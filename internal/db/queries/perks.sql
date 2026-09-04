-- Привилегии module queries (docs/specs/perks.md). The module owns perk /
-- perk_quota / perk_event / perk_snapshot; bank and bank_client are read-only
-- reference reads (ADR-0002 seam) — no cashback table is touched here, and
-- nothing outside the module reads these four.
--
-- Since 00025 a perk belongs to a держатель: `perk.bank_client_id` is the
-- module's anchor, the bank is read through it, and a window inherits both
-- from its perk.
--
-- Nothing in this file computes remaining. The math (effective size, consumed
-- across both levels, the discrepancy predicate) lives in internal/perks/
-- domain.go where it is pure and tested; these queries return the raw ledger
-- and let it do the arithmetic.

-- CreatePerk authorizes through the держатель, the CreateCard idiom: the
-- insert reads bank_client scoped by the session user, so a foreign or missing
-- client yields 0 rows → 404.
-- name: CreatePerk :one
insert into perk (user_id, bank_client_id, name, unit, note)
select cl.user_id, cl.id, sqlc.arg(name), sqlc.arg(unit), sqlc.narg(note)
from bank_client cl
where cl.id = sqlc.arg(bank_client_id)
  and cl.user_id = sqlc.arg(user_id)
returning *;

-- name: ListPerksForUser :many
select p.*, cl.bank_id, cl.label as client_label, b.name as bank_name
from perk p
         join bank_client cl on cl.id = p.bank_client_id
         join bank b on b.id = cl.bank_id
where p.user_id = $1
order by b.name, cl.label nulls first, p.name;

-- name: GetPerkForUser :one
select p.*, cl.bank_id, cl.label as client_label, b.name as bank_name
from perk p
         join bank_client cl on cl.id = p.bank_client_id
         join bank b on b.id = cl.bank_id
where p.id = $1
  and p.user_id = $2;

-- UpdatePerkForUser patches the three editable fields; a null argument leaves
-- its column alone, so the caller sends only what changed.
-- name: UpdatePerkForUser :one
update perk
set name = coalesce(sqlc.narg(name), name),
    unit = coalesce(sqlc.narg(unit), unit),
    note = case when sqlc.arg(set_note)::boolean then sqlc.narg(note) else note end
where id = sqlc.arg(id)
  and user_id = sqlc.arg(user_id)
returning *;

-- DeletePerkForUser leaves the 409 to the caller: perk_quota.perk_id is a plain
-- FK, so a perk with windows fails with 23503 rather than taking a ledger down
-- with it (spec invariant 6).
-- name: DeletePerkForUser :execrows
delete
from perk
where id = $1
  and user_id = $2;

-- CreatePerkQuota authorizes through the owning perk: 0 rows means the perk is
-- missing or someone else's. The держатель needs no checking — the window
-- inherits it (00025).
-- name: CreatePerkQuota :one
insert into perk_quota (perk_id, parent_quota_id, window_start, window_end, size, note)
select p.id, sqlc.narg(parent_quota_id), sqlc.arg(window_start), sqlc.arg(window_end), sqlc.arg(size), sqlc.narg(note)
from perk p
where p.id = sqlc.arg(perk_id)
  and p.user_id = sqlc.arg(user_id)
returning *;

-- name: GetPerkQuotaForUser :one
select q.*, p.user_id, p.bank_client_id, p.name as perk_name, p.unit
from perk_quota q
         join perk p on p.id = q.perk_id
where q.id = $1
  and p.user_id = $2;

-- name: ListPerkQuotasForPerk :many
-- PV-02's history: every window of one perk, newest first. Children sort
-- directly under their parent's window, so the screen can render the tree
-- without a second pass.
select q.*, cl.label as client_label, b.name as bank_name
from perk_quota q
         join perk p on p.id = q.perk_id
         join bank_client cl on cl.id = p.bank_client_id
         join bank b on b.id = cl.bank_id
where q.perk_id = $1
  and p.user_id = $2
order by coalesce(q.parent_quota_id, q.id), q.parent_quota_id nulls first, q.window_start desc;

-- ListActivePerkQuotasForUser drives PV-01. A window counts as active when it
-- contains the date, and so does a parent whose child is active — otherwise a
-- monthly window running past its annual pool's end (ВТБ's program year does
-- not follow Альфа's calendar year) would render as an orphan.
-- name: ListActivePerkQuotasForUser :many
select q.*,
       p.name    as perk_name,
       p.unit,
       p.note    as perk_note,
       cl.id     as client_id,
       cl.label  as client_label,
       b.id      as bank_id,
       b.name    as bank_name
from perk_quota q
         join perk p on p.id = q.perk_id
         join bank_client cl on cl.id = p.bank_client_id
         join bank b on b.id = cl.bank_id
where p.user_id = sqlc.arg(user_id)
  and (
    (q.window_start <= sqlc.arg(on_date) and sqlc.arg(on_date) <= q.window_end)
        or exists (select 1
                   from perk_quota c
                   where c.parent_quota_id = q.id
                     and c.window_start <= sqlc.arg(on_date)
                     and sqlc.arg(on_date) <= c.window_end)
    )
order by b.name, cl.label nulls first, p.name, q.parent_quota_id nulls first, q.window_start;

-- ListPerkEventsForQuotaTree returns the events of the given quotas AND of
-- their children: a leaf `use` burns both levels, so an annual window's
-- remaining counts uses recorded on months that have long since closed and are
-- nowhere near the active set. parent_quota_id rides along so the caller can
-- attribute each row to its level without a second lookup.
-- name: ListPerkEventsForQuotaTree :many
select e.id,
       e.quota_id,
       e.kind,
       e.qty,
       e.event_date,
       e.note,
       e.created_at,
       q.parent_quota_id
from perk_event e
         join perk_quota q on q.id = e.quota_id
where q.id = any (sqlc.arg(quota_ids)::bigint[])
   or q.parent_quota_id = any (sqlc.arg(quota_ids)::bigint[])
order by e.event_date, e.id;

-- name: ListPerkEventsForPerk :many
select e.id,
       e.quota_id,
       e.kind,
       e.qty,
       e.event_date,
       e.note,
       e.created_at,
       q.parent_quota_id
from perk_event e
         join perk_quota q on q.id = e.quota_id
         join perk p on p.id = q.perk_id
where q.perk_id = $1
  and p.user_id = $2
order by e.event_date, e.id;

-- ListLatestPerkSnapshots keeps one row per quota — the reading the discrepancy
-- badge is judged against. created_at breaks a same-day tie in insert order, so
-- correcting a mistyped snapshot is a matter of entering the right one.
-- name: ListLatestPerkSnapshots :many
select distinct on (s.quota_id) s.*
from perk_snapshot s
where s.quota_id = any (sqlc.arg(quota_ids)::bigint[])
order by s.quota_id, s.observed_on desc, s.created_at desc;

-- name: ListPerkSnapshotsForPerk :many
select s.*
from perk_snapshot s
         join perk_quota q on q.id = s.quota_id
         join perk p on p.id = q.perk_id
where q.perk_id = $1
  and p.user_id = $2
order by s.observed_on desc, s.created_at desc;

-- name: UpdatePerkQuotaForUser :one
-- `size` is patchable only while the window has no history (spec invariant 5);
-- the service checks that and passes null otherwise.
update perk_quota q
set size = coalesce(sqlc.narg(size), q.size),
    note = case when sqlc.arg(set_note)::boolean then sqlc.narg(note) else q.note end
from perk p
where p.id = q.perk_id
  and q.id = sqlc.arg(id)
  and p.user_id = sqlc.arg(user_id)
returning q.*;

-- CountPerkQuotaHistory answers «has this window been used yet?» — the gate on
-- editing `size` in place instead of dating a resize event.
-- name: CountPerkQuotaHistory :one
select (select count(*) from perk_event e where e.quota_id = $1)    as events,
       (select count(*) from perk_snapshot s where s.quota_id = $1) as snapshots;

-- name: DeletePerkQuotaForUser :execrows
delete
from perk_quota q using perk p
where p.id = q.perk_id
  and q.id = $1
  and p.user_id = $2;

-- CreatePerkEvent authorizes through the owning perk, the CreatePerkQuota
-- idiom: 0 rows means the quota is missing or someone else's.
-- name: CreatePerkEvent :one
insert into perk_event (quota_id, kind, qty, event_date, note)
select q.id, sqlc.arg(kind)::perk_event_kind, sqlc.arg(qty), sqlc.arg(event_date), sqlc.narg(note)
from perk_quota q
         join perk p on p.id = q.perk_id
where q.id = sqlc.arg(quota_id)
  and p.user_id = sqlc.arg(user_id)
returning *;

-- name: DeletePerkEventForUser :execrows
delete
from perk_event e using perk_quota q, perk p
where q.id = e.quota_id
  and p.id = q.perk_id
  and e.id = $1
  and p.user_id = $2;

-- name: CreatePerkSnapshot :one
insert into perk_snapshot (quota_id, observed_on, remaining, note)
select q.id, sqlc.arg(observed_on), sqlc.arg(remaining), sqlc.narg(note)
from perk_quota q
         join perk p on p.id = q.perk_id
where q.id = sqlc.arg(quota_id)
  and p.user_id = sqlc.arg(user_id)
returning *;

-- name: DeletePerkSnapshotForUser :execrows
delete
from perk_snapshot s using perk_quota q, perk p
where q.id = s.quota_id
  and p.id = q.perk_id
  and s.id = $1
  and p.user_id = $2;

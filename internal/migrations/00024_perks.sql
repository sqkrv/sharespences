-- Привилегии (docs/specs/perks.md): quota'd bank perks — такси compensations,
-- бизнес-залы, преференции — tracked as dated windows plus a ledger of events,
-- reconciled against snapshots of the bank's own counter.
--
-- The module replaces a hand-maintained table of 32 snapshot columns. What that
-- note proved about the domain, and what the schema therefore refuses to
-- assume (docs/knowledge/concepts/bank-perks.md):
--
-- * Windows are DATA, not derivation rules. ВТБ's monthly window ran 20th→19th
--   until 2026-07-31 and 1st→EOM after; Альфа's annual pool burns Dec 31. There
--   is no «resets on the 1st» anywhere here — both ends of every window are
--   typed in.
-- * Quota size is a condition outcome, not a constant. `size` is the size the
--   window OPENED at; every later change is a dated `resize` event, because the
--   history is the point (a month re-rated 15 → 12 «не выполнил условия» has to
--   stay legible two years later).
-- * The bank's counter is opaque and authoritative. perk_snapshot records what
--   the app displayed on a date; it never writes back. A snapshot that
--   disagrees with the computed remaining is a visible state, closed by an
--   explicit `adjust` event — never a silent recompute.
--
-- Nesting (the annual pool with a monthly sub-allowance inside it) is exactly
-- two levels, and a child belongs to its parent's perk and bank client. Both
-- are foreign keys rather than service promises: `is_child` and
-- `parent_is_root` are generated columns, and the composite self-FK targets
-- (id, is_child = false, perk_id, bank_client_id). A row naming a parent must
-- therefore match a row that is itself a root and carries the same perk and
-- client; a root leaves parent_quota_id null, which makes the FK's other
-- generated column null too and (MATCH SIMPLE) skips the check.
--
-- perk_quota.bank_client_id is a plain FK on purpose: deleting a bank client
-- that still has perk windows must fail with 23503, which bank-client-delete
-- already maps to 409. A cascade here would destroy a manual ledger the app
-- cannot reconstruct. Same reasoning one level up — perk_quota.perk_id does not
-- cascade, so perk-delete answers 409 while quotas exist (spec invariant 6).
-- Events and snapshots DO cascade from their quota: they are meaningless
-- without the window they were recorded against.

-- +goose Up
create table perk
(
    id         bigint generated always as identity primary key,
    user_id    uuid                     not null references "user" (id),
    bank_id    integer                  not null references bank (id),
    name       text                     not null,
    unit       text                     not null,
    note       text,
    created_at timestamp with time zone not null default now(),
    check (length(name) between 1 and 64),
    check (length(unit) between 1 and 32),
    unique (user_id, bank_id, name)
);

comment on column perk.unit is
    'the counted noun, singular: «поездка», «преференция», «проход» — rendered next to a number';

create table perk_quota
(
    id              bigint generated always as identity primary key,
    perk_id         bigint                   not null references perk (id),
    bank_client_id  bigint                   not null references bank_client (id),
    parent_quota_id bigint,
    window_start    date                     not null,
    window_end      date                     not null,
    size            integer                  not null,
    note            text,
    created_at      timestamp with time zone not null default now(),

    -- Derived halves of the nesting foreign key; see the header.
    is_child        boolean generated always as (parent_quota_id is not null) stored,
    parent_is_root  boolean generated always as (case when parent_quota_id is null then null else false end) stored,

    check (window_start <= window_end),
    check (size >= 0),
    unique (id, is_child, perk_id, bank_client_id),
    foreign key (parent_quota_id, parent_is_root, perk_id, bank_client_id)
        references perk_quota (id, is_child, perk_id, bank_client_id) on delete cascade
);

-- The overview reads a perk's windows for one client, newest first; the
-- history screen reads them for a perk across clients. Leading perk_id serves
-- both.
create index perk_quota_perk_client_window_idx on perk_quota (perk_id, bank_client_id, window_start desc);
-- Not covered by the index above: the FK check that runs on bank-client-delete,
-- and the child lookup the overview does per annual window.
create index perk_quota_bank_client_idx on perk_quota (bank_client_id);
create index perk_quota_parent_idx on perk_quota (parent_quota_id) where parent_quota_id is not null;

create type perk_event_kind as enum (
    'use',    -- a claim the bank compensated: the only kind that burns both levels
    'grant',  -- an allowance outside the schedule («подарили поездку»)
    'resize', -- the window re-rated: qty is the NEW absolute size, not a delta
    'adjust'  -- signed reconciliation against a snapshot; the note says why
    );

create table perk_event
(
    id         bigint generated always as identity primary key,
    quota_id   bigint                   not null references perk_quota (id) on delete cascade,
    kind       perk_event_kind          not null,
    qty        integer                  not null,
    event_date date                     not null,
    note       text,
    created_at timestamp with time zone not null default now(),
    -- qty means something different per kind, so the bound does too. An
    -- `adjust` of 0 is the one that would read as a recorded correction while
    -- changing nothing.
    check (case kind
               when 'use' then qty > 0
               when 'grant' then qty > 0
               when 'resize' then qty >= 0
               when 'adjust' then qty <> 0
        end)
);

create index perk_event_quota_date_idx on perk_event (quota_id, event_date);

create table perk_snapshot
(
    id          bigint generated always as identity primary key,
    quota_id    bigint                   not null references perk_quota (id) on delete cascade,
    observed_on date                     not null,
    remaining   integer                  not null,
    note        text,
    created_at  timestamp with time zone not null default now(),
    -- The bank's own counter floors at zero; a negative reading is a typo, and
    -- the app's own remaining is what goes negative (spec invariant 4).
    check (remaining >= 0)
);

-- Discrepancy is judged against the LATEST snapshot of a quota: newest first,
-- and created_at breaks a same-day tie in insert order.
create index perk_snapshot_quota_latest_idx on perk_snapshot (quota_id, observed_on desc, created_at desc);

-- +goose Down
drop table perk_snapshot;
drop table perk_event;
drop type perk_event_kind;
drop table perk_quota;
drop table perk;

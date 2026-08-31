-- Привилегии become flat: a perk belongs to a держатель, not to a bank.
--
-- 00024 modelled one perk per (пользователь, банк, название) with a quota
-- window per держатель inside it, on the reading that «Компенсация такси» is
-- one Альфа programme. The interface never managed to say that: PV-01 already
-- draws a card per держатель × привилегия, and the create form had to ask for
-- a держатель it could not store — then refuse a second держатель on the
-- unique name and explain the model in a paragraph.
--
-- The household this was built from is four Альфа accounts with the same perk.
-- Those are four rows, and each of them owns its own facts: Юля ran on 12 where
-- the others had 15, and the note that says «Alfa Only M» is true of one
-- держатель and not the next.
--
-- So `perk` carries `bank_client_id` and `bank_id` goes — the bank is whatever
-- the держатель's client says it is, and storing it twice only invites the two
-- to disagree. `perk_quota.bank_client_id` goes with it for the same reason:
-- the window's держатель is its perk's, and the composite nesting FK loses a
-- column it no longer needs to police.
--
-- The backfill splits every perk that spans держатели. A perk with no windows
-- has no держатель to infer, so it lands on the user's first client at that
-- bank — recoverable by renaming or deleting, unlike dropping the row.

-- A side effect worth naming: 00024 authorized a quota by держатель OWNER only,
-- so a perk at Альфа could take a window on a ВТБ client. With the perk owning
-- the client that is no longer expressible.

-- +goose Up
alter table perk
    add column bank_client_id bigint references bank_client (id),
    -- Dropped again at the end; it is how a clone remembers what it split from.
    add column split_from     bigint;

-- The clones repeat (user, bank, name), so the old key has to go first.
alter table perk
    drop constraint perk_user_id_bank_id_name_key;

create temporary table perk_split on commit drop as
select q.perk_id,
       q.bank_client_id,
       row_number() over (partition by q.perk_id order by min(q.id)) as n
from perk_quota q
group by q.perk_id, q.bank_client_id;

-- The держатель whose window came first keeps the original row.
update perk p
set bank_client_id = s.bank_client_id
from perk_split s
where s.perk_id = p.id
  and s.n = 1;

insert into perk (user_id, bank_id, name, unit, note, bank_client_id, split_from)
select p.user_id, p.bank_id, p.name, p.unit, p.note, s.bank_client_id, p.id
from perk_split s
         join perk p on p.id = s.perk_id
where s.n > 1;

update perk_quota q
set perk_id = clone.id
from perk clone
where clone.split_from = q.perk_id
  and clone.bank_client_id = q.bank_client_id;

-- Perks nobody ever opened a window on: attach to the user's first client at
-- that bank; drop only where the user has no client there at all, since the row
-- cannot be expressed in the new shape.
update perk p
set bank_client_id = (select cl.id
                      from bank_client cl
                      where cl.user_id = p.user_id
                        and cl.bank_id = p.bank_id
                      order by cl.id
                      limit 1)
where p.bank_client_id is null;

delete from perk where bank_client_id is null;

alter table perk
    alter column bank_client_id set not null,
    drop column bank_id,
    drop column split_from,
    add constraint perk_client_name_key unique (bank_client_id, name);
create index perk_user_idx on perk (user_id);

-- The window's держатель is its perk's now, so the nesting key drops a column.
alter table perk_quota
    drop constraint perk_quota_parent_quota_id_parent_is_root_perk_id_bank_cli_fkey,
    drop constraint perk_quota_id_is_child_perk_id_bank_client_id_key,
    drop column bank_client_id;
alter table perk_quota
    add constraint perk_quota_nesting_key unique (id, is_child, perk_id),
    add constraint perk_quota_parent_fkey foreign key (parent_quota_id, parent_is_root, perk_id)
        references perk_quota (id, is_child, perk_id) on delete cascade;

-- Both old indexes led with bank_client_id and went with the column.
create index perk_quota_perk_window_idx on perk_quota (perk_id, window_start desc);

-- +goose Down
-- Lossy: the split cannot be undone — держатели that were one perk stay
-- separate rows, and the clones' identities are gone.
alter table perk add column bank_id integer references bank (id);
update perk p set bank_id = (select cl.bank_id from bank_client cl where cl.id = p.bank_client_id);
alter table perk alter column bank_id set not null;
alter table perk_quota add column bank_client_id bigint references bank_client (id);
update perk_quota q set bank_client_id = (select p.bank_client_id from perk p where p.id = q.perk_id);
alter table perk_quota alter column bank_client_id set not null;

-- Catalog rows a user creates become private to that account.
--
-- bank_category and bank_category_alias are writable by any signed-in account
-- — the picker's «Добавить свою категорию», and the alias upsert that every
-- recorded offer performs — but were read by everyone. A typo, a joke title or
-- a wrong canonical mapping reached every account holding that bank, and the
-- alias upsert (on conflict do update) silently replaced the suggestion others
-- got for the same (bank, raw title). With open registration in production
-- that is a shared, unmoderated namespace.
--
-- A null owner means seed-managed and global; a non-null one means visible to
-- that account alone. Both uniques become NULLS NOT DISTINCT over the owner
-- column, which also frees a title the seed may ship later: under unique
-- (bank_id, title) a custom row squatted the name, and the seed's
-- `where not is_custom` guard then kept it un-refreshable forever.
--
-- bank_category_alias loses its primary key for the same reason — a key column
-- cannot be nullable — and keeps an equivalent unique constraint. Its leading
-- column is still bank_id, so the per-bank lookup keeps its index.
--
-- Existing bank_category rows are attributed where the evidence is
-- unambiguous: exactly one account cites the row in an offer. Rows cited by
-- several accounts, or by none, stay global for review in the operator
-- sidecar. Aliases are deliberately NOT attributed — that table has no
-- provenance, and privatising a seeded row would freeze a copy that later seed
-- corrections could never reach. Instead the seed now refreshes global aliases
-- (00019 pairs with the `do update` in internal/seed), so a mapping a user
-- overwrote before this migration is corrected on the next seed run.

-- +goose Up
alter table bank_category
    add column created_by uuid references "user" (id) on delete cascade;

alter table bank_category_alias
    add column user_id uuid references "user" (id) on delete cascade;

-- Catalog row → the offers citing it → the period's client → its owner.
update bank_category bc
set created_by = ev.user_id
from (select co.bank_category_id         as id,
             min(cl.user_id::text)::uuid as user_id
      from category_offer co
               join offer_period op on op.id = co.offer_period_id
               join bank_client cl on cl.id = op.bank_client_id
      where co.bank_category_id is not null
      group by co.bank_category_id
      having count(distinct cl.user_id) = 1) ev
where bc.id = ev.id
  and bc.is_custom;

alter table bank_category
    drop constraint bank_category_bank_id_title_key,
    add constraint bank_category_bank_id_title_created_by_key
        unique nulls not distinct (bank_id, title, created_by);

alter table bank_category_alias
    drop constraint bank_category_alias_pkey,
    add constraint bank_category_alias_bank_id_raw_title_user_id_key
        unique nulls not distinct (bank_id, raw_title, user_id);

-- +goose Down
-- Lossy, like 00007 and 00016. Merging the namespaces back cannot preserve
-- what the split allowed: two accounts' rows with one (bank, title). The
-- earliest row per pair survives and the rest are deleted — offers citing a
-- deleted row keep their raw_title snapshot and lose only the catalog link
-- (bank_category_id is ON DELETE SET NULL). Private aliases are dropped
-- outright; they are re-derived the next time an offer is saved.
delete
from bank_category bc
    using bank_category keep
where keep.bank_id = bc.bank_id
  and keep.title = bc.title
  and keep.id < bc.id;

delete
from bank_category_alias
where user_id is not null;

alter table bank_category
    drop constraint bank_category_bank_id_title_created_by_key,
    add constraint bank_category_bank_id_title_key unique (bank_id, title);

alter table bank_category_alias
    drop constraint bank_category_alias_bank_id_raw_title_user_id_key,
    add primary key (bank_id, raw_title);

alter table bank_category
    drop column created_by;

alter table bank_category_alias
    drop column user_id;

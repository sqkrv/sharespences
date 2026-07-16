-- First-class per-bank category catalog for the SPA's picker (owner
-- decisions 2026-07-16). bank_category_alias stays the free-text matching
-- mechanism; this table is what the picker LISTS — including special/
-- service rows, which the alias table can't hold (its canonical FK is NOT
-- NULL, but service rows get no canonical per the 2026-07-14 owner rule).
-- Seeded from docs/knowledge/concepts/categories-taxonomy.md «Bank
-- catalogs»; user-created rows carry is_custom = true.

-- +goose Up
alter table canonical_category
    add column emoji text; -- UI icon; null = generic fallback in the SPA

create table bank_category
(
    id                    bigint generated always as identity primary key,
    bank_id               integer             not null references bank (id),
    title                 text                not null,
    canonical_category_id bigint references canonical_category (id), -- null = special/service row
    kind                  cashback_offer_kind not null default 'regular', -- prefill hint for entry
    emoji                 text,                                           -- override; null = inherit canonical's
    is_custom             boolean             not null default false,
    active                boolean             not null default true,
    unique (bank_id, title)
);

-- Traceability from a recorded offer back to the catalog row it was picked
-- from. raw_title stays the self-sufficient snapshot: catalog deletion
-- never touches user history.
alter table category_offer
    add column bank_category_id bigint references bank_category (id) on delete set null;

-- +goose Down
alter table category_offer
    drop column bank_category_id;
drop table bank_category;
alter table canonical_category
    drop column emoji;

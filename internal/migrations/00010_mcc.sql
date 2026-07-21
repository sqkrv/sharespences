-- MCC module bones (owner decisions 2026-07-20): FLAT current state +
-- append-only change journal — not temporal intervals, not per-document
-- snapshots. The legacy `mcc` table (00001) becomes the live dictionary:
-- right shape, empty, and point_of_sale already references it for the
-- future crowd-source module. Legacy category / category_mcc / bank_mcc
-- stay dead. Seam note: bank_category_mcc FKs into cashback's
-- bank_category — read-only reference reads, same practice as
-- cashback.sql's bank joins (documented there).

-- +goose Up
create table bank_category_mcc
(
    bank_category_id bigint   not null references bank_category (id) on delete cascade,
    mcc_code         smallint not null references mcc (code),
    note             text, -- source footnote («покупки на сайте travel.alfabank.ru…»)
    primary key (bank_category_id, mcc_code)
);
create index idx_bank_category_mcc_code on bank_category_mcc (mcc_code);

-- `imported` is distinct from `added` so the news digest never renders the
-- baseline load as «bank added X today»; category_* events arrive with the
-- ADR-0004 parse pipeline.
create type mcc_change_action as enum
    ('imported', 'added', 'removed', 'category_added', 'category_removed');

-- The news-digest precursor. category_title is a self-sufficient snapshot
-- (00009 raw_title precedent): the journal survives catalog deletion.
-- mcc_code deliberately has NO FK — the journal may record codes before the
-- dictionary knows them. noted_at = when WE noticed, not the bank's
-- (unknowable) change date; provenance lives in source.
create table mcc_change
(
    id               bigint generated always as identity primary key,
    bank_id          integer           not null references bank (id),
    bank_category_id bigint references bank_category (id) on delete set null,
    category_title   text              not null,
    mcc_code         smallint, -- null for category_* events
    action           mcc_change_action not null,
    noted_at         timestamptz       not null default now(),
    source           text              not null,
    note             text
);
create index idx_mcc_change_noted_at on mcc_change (noted_at desc, id desc);

-- +goose Down
drop table mcc_change;
drop type mcc_change_action;
drop table bank_category_mcc;

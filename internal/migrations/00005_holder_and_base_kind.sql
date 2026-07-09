-- Owner decisions 2026-07-09:
-- 1. holder_label: whose plastic this is («Мама», «Стас»…) — the household
--    runs several family members' cards under one account. Display/grouping
--    only; upgrades to a держатель entity in the Groups/household era.
-- 2. kind=base: «За все покупки» granted outside the menu — slot-free,
--    non-colliding, shown in lookups as the fallback. Banks where the base
--    rate is a selectable slot choice enter it as a regular row instead.

-- +goose Up
alter table bank_card
    add column holder_label text;
comment on column bank_card.holder_label is
    'Family member holding this plastic; null = the account owner themselves';

alter type cashback_offer_kind add value if not exists 'base';

-- +goose Down
alter table bank_card
    drop column holder_label;
-- PostgreSQL cannot drop enum values; 'base' stays (harmless when unused).

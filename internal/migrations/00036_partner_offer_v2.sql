-- +goose Up
-- Партнёрки v2 (redesign 2026-08-06, spec amendment in docs/specs/cashback.md):
-- partner offers are promoted from record-only to ranked — the feed, the
-- category lookup and the точка продаж surface them, so the row needs to be
-- machine-rankable:
--
--   - scope_kind + canonical_category_id say WHERE the offer applies: a
--     merchant offer keeps merchant_title as both display text and the
--     name-matching key (no FK to point_of_sale — that table is import-owned
--     and per-location, a foreign key would be false precision), with the
--     canonical as an optional hint; a category-wide offer requires it.
--   - merchant_kind reuses point_of_sale_type (магазин / онлайн / приложение)
--     rather than inventing a parallel enum.
--   - currency_kind puts the offer into the ranking's currency group
--     (invariant 5: a points партнёрка never outranks a ruble row by number).
--     Nullable: pre-v2 rows may predate a resolvable program; null ranks in
--     the unknown group, honestly last.
--   - requires_activation + activated_at are two fields because one flag
--     cannot say «не требует» / «требует, не активировано» / «активировано»;
--     an unactivated offer still ranks, with a warning.
--   - ended_at records «Завершить» as an event; valid_to stays the recorded
--     bank term and is never rewritten (undo = clear ended_at).
create type partner_scope as enum ('merchant', 'category');

alter table partner_offer
    add column scope_kind            partner_scope not null default 'merchant',
    add column canonical_category_id bigint references canonical_category (id),
    add column merchant_kind         point_of_sale_type,
    add column currency_kind         cashback_currency_kind,
    add column requires_activation   boolean       not null default false,
    add column activated_at          timestamptz,
    add column ended_at              timestamptz;

alter table partner_offer
    add constraint partner_offer_category_scope_has_canonical
        check (scope_kind <> 'category' or canonical_category_id is not null);

-- Existing rows inherit the bank's program currency — the same bank-level
-- fallback the offer list already displays with.
update partner_offer po
set currency_kind = (select cp.currency_kind
                     from cashback_program cp
                     where cp.bank_id = po.bank_id
                     limit 1)
where currency_kind is null;

create index partner_offer_canonical_idx on partner_offer (canonical_category_id);

comment on column partner_offer.ended_at is
    'ended by the user («Завершить»); valid_to keeps the bank''s own term';

-- +goose Down
-- Lossy for the new fields, like 00007: the scope/currency/activation facts
-- have nowhere to go back to.
drop index partner_offer_canonical_idx;
alter table partner_offer
    drop constraint partner_offer_category_scope_has_canonical;
alter table partner_offer
    drop column scope_kind,
    drop column canonical_category_id,
    drop column merchant_kind,
    drop column currency_kind,
    drop column requires_activation,
    drop column activated_at,
    drop column ended_at;
drop type partner_scope;

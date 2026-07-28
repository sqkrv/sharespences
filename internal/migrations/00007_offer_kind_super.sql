-- Split the cashback_offer_kind «special» bucket (spec invariant 6): the
-- Альфа monthly барабан суперкэшбека is a full-period,
-- STACKING boost — a real «best card for такси» — unlike the time-boxed /
-- non-stacking bonuses (Пятница, колесо, timed flash, сервис-категории).
-- The new `super` kind ranks in lookup/overview like a regular, but like
-- `special` consumes no slot and raises no collision warning.
--
-- Enum-value additions in PostgreSQL are one-way: the Down cannot drop the
-- label, so it only demotes any `super` rows back to `special` to keep data
-- valid on rollback. Adding (not USING) the value is transaction-safe on
-- PG 12+, so this runs under goose's default transaction.

-- +goose Up
alter type cashback_offer_kind add value if not exists 'super';

-- +goose Down
update category_offer set kind = 'special' where kind = 'super';
-- (the 'super' enum label itself remains — PostgreSQL cannot drop it)

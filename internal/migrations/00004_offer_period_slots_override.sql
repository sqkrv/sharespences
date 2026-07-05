-- Owner feedback 2026-07-04: the number of selectable categories is NOT
-- constant per tier — banks vary it per period. tier.max_categories stays
-- the reference default; a period-level override wins when set.

-- +goose Up
alter table offer_period
    add column max_categories_override integer;
comment on column offer_period.max_categories_override is
    'Per-period slot count; null = use tier.max_categories (invariant 1 uses the effective value)';

-- +goose Down
alter table offer_period
    drop column max_categories_override;

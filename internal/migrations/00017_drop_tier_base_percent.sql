-- Drop program_tier.base_percent: it modelled a rate no bank actually pays.
--
-- The column was added with the original cashback schema (00003) as a
-- "headline %, offers override per row" hint. Six days later the model was
-- corrected: «За все покупки» is not an always-on rate but an ordinary
-- slot-consuming category row, mapped to canonical all-purchases and
-- displayed as «Остальное». `kind = base` was removed then; this column was
-- missed and kept standing.
--
-- Nothing computed with it — no ranking, cap math, collision check or
-- fallback ever read it. Its only consumer rendered «Базовая ставка 5%» on
-- the period screen, which asserted exactly the always-on semantics the
-- correction had ruled out: Альфа's real base-on-everything is 1%, and only
-- when a slot is spent on it. The seeded 5/7 values were the tier's headline
-- category rate, which cannot be authoritative anyway — percent is a
-- (client, month) attribute, the same category paying 7% for one client and
-- 10% for another in the same month.
--
-- No user-entered data is lost: the column was populated by the seed only,
-- and the per-tier headline rates remain recorded in the knowledge base.

-- +goose Up
alter table program_tier
    drop column base_percent;

-- +goose Down
-- Lossy: the values are seed-derived, so the column comes back empty and a
-- re-seed will not refill it (the seed no longer carries the numbers).
alter table program_tier
    add column base_percent numeric;

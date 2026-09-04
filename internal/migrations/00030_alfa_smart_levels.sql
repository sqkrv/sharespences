-- «Альфа-Смарт» has been two products since 2026-01, and the seed models one.
-- The bank's own subscription page states the split: a set of 9 привилегий at
-- 199 ₽/мес, and a set of 15 привилегий at 399 ₽/мес (личный) or 499 ₽
-- (семейный). Only the 15-privilege set carries the cashback privileges — the
-- fourth category slot, the extra барабан spin, and the 5 000 → 7 000 ₽ cap.
--
-- Modelled as two tiers. The existing row becomes «Альфа-Смарт M»: it already
-- carries the 7 000 ₽ / 4-slot terms, so every bank_client pointing at it keeps
-- exactly the terms it has today and nobody is silently downgraded. «Альфа-Смарт
-- S» is added by the seed with base-equivalent cashback terms.
--
-- This has to be a migration rather than a seed edit. program_tier is unique on
-- (program_id, name), so «Альфа-Смарт M» does not collide with «Альфа-Смарт» —
-- it is simply a different row. The seed would insert it and leave the original
-- untouched, and every bank_client still references the original: a tier whose
-- name is no longer in the seed literals, so nothing refreshes it and it drifts
-- forever. Rename in place first; the seed literals then match the row clients
-- already point at.
--
-- (This differs from 00012's problem, which was bank.name having no unique
-- constraint at all. Same remedy, different cause.)
--
-- Scoped to Альфа-Банк's programme by join — «Альфа-Смарт» is not a name any
-- other bank's tier could plausibly carry, but an unscoped update over a table
-- keyed only by name is the kind of thing that bites later.

-- +goose Up
update program_tier pt
set name = 'Альфа-Смарт M'
from cashback_program cp
         join bank b on b.id = cp.bank_id
where pt.program_id = cp.id
  and b.name = 'Альфа-Банк'
  and pt.name = 'Альфа-Смарт';

-- +goose Down
-- Lossy in one direction only: a client moved onto «Альфа-Смарт S» after the Up
-- has no pre-split equivalent, so the Down leaves them on a tier the seed will
-- no longer refresh. Merging S back into the single tier would be worse — it
-- would hand S clients the M cap.
update program_tier pt
set name = 'Альфа-Смарт'
from cashback_program cp
         join bank b on b.id = cp.bank_id
where pt.program_id = cp.id
  and b.name = 'Альфа-Банк'
  and pt.name = 'Альфа-Смарт M';

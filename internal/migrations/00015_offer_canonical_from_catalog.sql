-- Backfill for rows recorded before the offer inherited the catalog row's
-- canonical mapping. A category picked from the bank catalog reaches the
-- API as bank_category_id alone, and the offer's canonical_category_id
-- stayed null — which is the field lookup and the overview's «Категории»
-- cut key on, so those rows were recorded, selected, and invisible.
--
-- Only rows that carry no mapping of their own are touched, and only where
-- the catalog row has one: canonical-less catalog rows (Альфа-Тревел,
-- канальные) are deliberately unmapped and stay that way.

-- +goose Up
update category_offer co
set canonical_category_id = bc.canonical_category_id
from bank_category bc
where bc.id = co.bank_category_id
  and co.canonical_category_id is null
  and bc.canonical_category_id is not null;

-- +goose Down
-- Not reversible: the pre-backfill null is indistinguishable from a mapping
-- the user set by hand, so undoing this would drop good data.
select 1;

-- A journal action for codes a bank's own document names but the MCC
-- dictionary does not carry.
--
-- bank_category_mcc.mcc_code is a foreign key into mcc, so such a code cannot
-- be attached: the import drops it and prints one line. That guard is worth
-- keeping — it is what filters parser artefacts, such as a range that expands
-- across the unassigned gap between 4011 and 4111 — but the information was
-- ephemeral, surviving only in whatever terminal ran the import. A new code
-- appearing next quarter is exactly the case the monthly ritual must notice.
--
-- Journalled once per (bank, code): re-importing an unchanged document adds
-- nothing. mcc_change.mcc_code deliberately has no FK (00010), so a code the
-- dictionary lacks is recordable there by design.
--
-- Enum-value additions are one-way (00007 precedent): the Down deletes the
-- rows carrying the label but cannot drop the label itself.

-- +goose Up
alter type mcc_change_action add value if not exists 'code_unknown';

-- +goose Down
delete
from mcc_change
where action = 'code_unknown';
-- (the label remains — PostgreSQL cannot drop an enum value)

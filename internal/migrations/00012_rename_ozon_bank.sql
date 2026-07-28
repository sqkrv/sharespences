-- The bank's own brand is «Ozon Банк» (Latin
-- wordmark + Cyrillic «Банк»), not the fully-Russian «Озон Банк» the seed
-- shipped. This has to be a migration, not just a seed edit: `bank.name`
-- carries no unique constraint and seedBank inserts `where not exists
-- (select 1 from bank where name = $1)`, so a renamed literal alone would
-- add a SECOND bank row and orphan every bank_client, offer_period and
-- imported offer hanging off the old one. Rename in place first; the seed
-- literals then match the existing row.
--
-- Everything else that keys on the name (bank_category, aliases, colors,
-- the MCC membership CSV) resolves through `where b.name = $1` at seed
-- time — those are no-ops against a missing name rather than errors, so
-- the seed literals must move in the same commit as this migration.

-- +goose Up
update bank
set name = 'Ozon Банк'
where name = 'Озон Банк';

-- +goose Down
update bank
set name = 'Озон Банк'
where name = 'Ozon Банк';

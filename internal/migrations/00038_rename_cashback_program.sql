-- cashback_program.name «Кэшбэк» → «Кешбэк», aligning the program label with
-- the single spelling the rest of the app uses (2026-09-08).
--
-- A migration rather than a seed edit, for the reason 00012, 00030 and 00032
-- all had: the seed resolves a program by (bank_id, name) under a NOT EXISTS
-- guard, and the tier insert keys off cp.name as well. A renamed literal alone
-- would therefore insert a SECOND program per bank and leave every tier, client
-- link and period attached to an orphan the seed no longer refreshes.
--
-- Collision-free: cashback_program has been unique on (bank_id, name) since
-- 00003, and no bank carries a «Кешбэк» program yet.

-- +goose Up
update cashback_program
set name = 'Кешбэк'
where name = 'Кэшбэк';

-- +goose Down
update cashback_program
set name = 'Кэшбэк'
where name = 'Кешбэк';

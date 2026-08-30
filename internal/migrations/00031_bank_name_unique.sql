-- bank.name is how the seed resolves every bank it writes — programs, tiers,
-- catalogs, aliases, colors and the MCC membership CSV all key on `where
-- b.name = $1` — but nothing at the database level made it unique. The only
-- guard was seedBank's own `where not exists (select 1 from bank where name =
-- $1)`, so any insert outside that path could add a second row with the same
-- name, and every later name lookup would resolve to an arbitrary one of them.
--
-- The inconsistency is visible one level down: cashback_program is unique on
-- (bank_id, name) and program_tier on (program_id, name). bank, the root of
-- eleven foreign keys, had neither.
--
-- Making it unique also removes a race the NOT EXISTS guard cannot close: two
-- concurrent seed runs can both pass the check and both insert. With the
-- constraint in place seedBank switches to ON CONFLICT DO NOTHING.
--
-- ⚠️ This migration fails, deliberately and before changing anything, if
-- duplicates already exist. Merging two bank rows means re-pointing eleven FKs
-- and deciding which row's history is authoritative — an operator decision, not
-- something a migration should guess at. The exception names the offenders.

-- +goose Up
-- +goose StatementBegin
do $$
    declare
        dupes text;
    begin
        select string_agg(format('%L (%s rows)', name, n), ', ' order by name)
        into dupes
        from (select name, count(*) as n from bank group by name having count(*) > 1) d;

        if dupes is not null then
            raise exception 'bank.name has duplicates, cannot make it unique: %', dupes
                using hint = 'Merge the duplicate bank rows first: re-point the eleven FKs '
                             '(bank_client, cashback_program, bank_category, bank_card, category, '
                             'point_of_sale, …) at the surviving id, then delete the loser and re-run.';
        end if;
    end
$$;
-- +goose StatementEnd

alter table bank
    add constraint bank_name_key unique (name);

-- +goose Down
alter table bank
    drop constraint bank_name_key;

-- UUID primary keys switch from gen_random_uuid() (v4, uniformly random) to
-- uuidv7() (time-ordered). Random keys scatter B-tree inserts across the whole
-- index — write amplification, more WAL and more bloat as a table grows; v7
-- keys append, so inserts stay in the hot right-hand pages.
--
-- Defaults only. Existing rows keep their v4 ids, and the two forms coexist in
-- one column with no special handling: both are valid UUIDs, and nothing in the
-- schema, the API or the SPA parses a version out of an id.
--
-- transaction and receipt_position are the tables that will actually grow (the
-- ADR-0003 import lands every user's bank history in them) and both are still
-- empty, so they get the benefit from their first row. point_of_sale is listed
-- for consistency only: import-pos supplies the source site's own row UUID, so
-- the column default is never reached today.
--
-- Trade-off accepted: a v7 id embeds its creation time in milliseconds, so
-- whoever holds an id learns when the row was created. Ids appear in API paths,
-- but every one of these tables is scoped per account and answers 404 for a
-- foreign id, so this discloses to a party already holding the id. It is not a
-- privacy-policy edit — the policy describes what is collected, not the shape
-- of internal identifiers.
--
-- Requires PostgreSQL 18: uuidv7() is built in there and absent before it,
-- which is why the precondition below fails loudly rather than letting a
-- deploy discover it one ALTER at a time.

-- +goose Up
-- +goose StatementBegin
do
$$
    begin
        if current_setting('server_version_num')::int < 180000 then
            raise exception
                'migration 00022 needs PostgreSQL 18 for uuidv7(); server is %',
                current_setting('server_version');
        end if;
    end
$$;
-- +goose StatementEnd

alter table "user" alter column id set default uuidv7();
alter table attachment alter column id set default uuidv7();
alter table point_of_sale alter column id set default uuidv7();
alter table transaction alter column id set default uuidv7();
alter table receipt_position alter column id set default uuidv7();
alter table friend_invite alter column id set default uuidv7();

-- +goose Down
-- Lossless: defaults only, and rows written as v7 stay valid UUIDs.
alter table friend_invite alter column id set default gen_random_uuid();
alter table receipt_position alter column id set default gen_random_uuid();
alter table transaction alter column id set default gen_random_uuid();
alter table point_of_sale alter column id set default gen_random_uuid();
alter table attachment alter column id set default gen_random_uuid();
alter table "user" alter column id set default gen_random_uuid();

-- Where a point of sale came from.
--
-- The base has had exactly one origin so far — the 62 117-row mcc-codes.ru
-- scrape — and is about to have several: rows typed by a user, rows derived
-- from an opted-in user's own transactions, and the rows the admin sidecar
-- already writes today. Once they coexist, a row that cannot say where it
-- came from cannot be trusted, credited or corrected differently from its
-- neighbours.
--
-- Two counters on purpose. `confirmations` is mcc-codes.ru's own crowd
-- signal, imported verbatim and used to rank the merchant search; adding
-- this app's confirmations to it would average two populations counted
-- under different rules, irreversibly. Ours get their own column.
--
-- No default on `origin`. Every existing row is from the scrape, so the
-- backfill below is exact — but a permanent default would let a future
-- write path forget to declare itself and be quietly recorded as a scrape.
-- Both current writers (the import upsert, AdminCreatePOS) name their own.

-- +goose Up
create type point_of_sale_origin as enum (
    'mcc_codes',        -- the mcc-codes.ru scrape, imported by `import-pos`
    'user_manual',      -- typed by a user in the app
    'user_transaction', -- derived from an opted-in user's own operations
    'admin'             -- created or curated through the admin sidecar
    );

alter table point_of_sale
    add column origin             point_of_sale_origin,
    add column user_confirmations bigint not null default 0;

-- The whole base predates every other write path.
update point_of_sale
set origin = 'mcc_codes'
where origin is null;

alter table point_of_sale
    alter column origin set not null;

comment on column point_of_sale.origin is
    'where this row came from; deliberately without a default, so every write path states it';
comment on column point_of_sale.confirmations is
    'mcc-codes.ru''s own crowd counter, imported verbatim — never incremented by this app';
comment on column point_of_sale.user_confirmations is
    'confirmations from this app''s users (manual entry, opted-in transaction imports)';

-- +goose Down
-- Lossy in the same way 00007 and 00016 are: the origin of every non-scrape
-- row and every user confirmation is gone, and no re-import recovers them.
-- Prod is forward-only (ADR-0007); this exists for local iteration.
alter table point_of_sale
    drop column user_confirmations,
    drop column origin;

drop type point_of_sale_origin;

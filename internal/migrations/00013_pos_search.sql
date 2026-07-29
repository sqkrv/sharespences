-- Merchant search over point_of_sale (the mcc-codes.ru base, loaded via the
-- import-pos subcommand — the data itself never ships in the repo). Queries
-- use plain ILIKE; the trigram GIN indexes accelerate it transparently.
-- pg_trgm ships with the postgis/postgis image; creating an extension needs
-- superuser (true for compose, e2e testcontainers and prod today).

-- +goose Up
create extension if not exists pg_trgm;
create index idx_point_of_sale_name_trgm
    on point_of_sale using gin (name gin_trgm_ops);
create index idx_point_of_sale_merchant_title_trgm
    on point_of_sale using gin (merchant_title gin_trgm_ops);

-- +goose Down
-- The extension stays: shared-object semantics — another database object may
-- depend on it by the time this rolls back, and dropping it is never required
-- for correctness.
drop index idx_point_of_sale_merchant_title_trgm;
drop index idx_point_of_sale_name_trgm;

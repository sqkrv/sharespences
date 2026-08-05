-- The postgis/postgis image's init script installs four extensions into every
-- database it creates; the schema uses exactly one of them. postgis_topology,
-- postgis_tiger_geocoder and fuzzystrmatch are referenced by no migration, no
-- query and no seed — the tiger geocoder alone carries a schema of US Census
-- reference tables (addrfeat, county, tabblock…) with no bearing on this app.
-- Only postgis, btree_gist (00003) and pg_trgm (00013) are actually used.
--
-- Dropping them shrinks the schema and removes the objects that make a PostGIS
-- major upgrade awkward — the tiger geocoder is the usual reason a PostGIS
-- dump/restore or pg_upgrade needs hand-holding.
--
-- Order matters: postgis_tiger_geocoder depends on fuzzystrmatch. The schemas
-- outlive the extensions that created them, so they are dropped explicitly.
--
-- Version-agnostic — verified on PostgreSQL 16 and 18.

-- +goose Up
drop extension if exists postgis_tiger_geocoder cascade;
drop extension if exists postgis_topology cascade;
drop extension if exists fuzzystrmatch cascade;

drop schema if exists tiger cascade;
drop schema if exists tiger_data cascade;
drop schema if exists topology cascade;

-- +goose Down
-- Restores what the image would have installed. fuzzystrmatch first: the tiger
-- geocoder depends on it. Each extension recreates its own schema.
create extension if not exists fuzzystrmatch;
create extension if not exists postgis_topology;
create extension if not exists postgis_tiger_geocoder;

-- Radius search («Рядом» on CB-04) measures in metres; geometry(Point, 4326)
-- measures in degrees. The two read identically at the call site and differ
-- by three orders of magnitude: for a 1 421 km leg ST_Distance returns 23.0,
-- and ST_DWithin(location, p, 500) — which reads as «within 500 m» — matches
-- it. A geo query written against these columns would have looked correct and
-- returned the whole country.
--
-- geography(Point, 4326) takes and returns metres in ST_Distance/ST_DWithin,
-- so the same call means what it reads. Casting at query time
-- (location::geography) is the alternative, but it bypasses a plain GiST
-- index on the geometry column, so the type is the honest place to fix it.
--
-- Free to do now and not later: neither column has ever been written — no
-- import path fills them (the merchant scrape carries addresses, not
-- coordinates) and no query reads them. Once coordinates exist this stops
-- being a type change and becomes a data migration.
--
-- The dependent GiST index on transaction.location is rebuilt automatically
-- with the geography operator class; no explicit drop/create needed.

-- +goose Up
alter table point_of_sale
    alter column location type geography(Point, 4326) using location::geography;

alter table transaction
    alter column location type geography(Point, 4326) using location::geography;

-- +goose Down
-- Lossless in both directions: the cast is exact for points.
alter table transaction
    alter column location type geometry(Point, 4326) using location::geometry;

alter table point_of_sale
    alter column location type geometry(Point, 4326) using location::geometry;

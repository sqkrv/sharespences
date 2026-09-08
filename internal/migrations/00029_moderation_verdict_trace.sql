-- The reviewer's trace (Moderation-Module boards 1c/1d). A SEPARATE
-- migration rather than an amendment of 00028: 00028 was already applied
-- on dev databases when these columns were designed, and goose never
-- re-runs a recorded version — an in-place edit leaves every such
-- database silently missing the columns (the published-stream 500,
-- 2026-08-28).
--
-- moderation_note is the reject reason — a note for the OPERATOR, never
-- returned to the author. moderated_at is when the verdict fell, which
-- for a manual row is also its publish moment: the review stream orders
-- by coalesce(moderated_at, created_at).

-- +goose Up
alter table point_of_sale
    add column moderation_note text,
    add column moderated_at    timestamptz;

-- +goose Down
alter table point_of_sale
    drop column moderated_at,
    drop column moderation_note;

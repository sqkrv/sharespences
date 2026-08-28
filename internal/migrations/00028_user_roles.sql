-- Roles & PoS moderation (docs/specs/roles-moderation.md): elevated
-- permissions live in the app — moderators are app users with a role, not
-- sidecar operators (ADR-0008 addendum). Only `moderator` is wired today;
-- `admin` exists so future in-app admin features need no second migration.
-- Roles are read per-request at the gate, never frozen into the session,
-- so a demotion takes effect on the demoted user's next request.

-- +goose Up
create type user_role as enum ('user', 'moderator', 'admin');

alter table "user"
    add column role user_role not null default 'user';

-- The 5e submission status gains the reviewer's second verb. A rejected
-- row is kept (audit; the author may file a fresh submission) but becomes
-- invisible everywhere — search filters show approved rows plus the
-- author's own PENDING ones only.
alter type point_of_sale_status add value 'rejected';

-- The reviewer's trace (design 1c/1d): moderation_note is the reject
-- reason — a note for the OPERATOR, never returned to the author;
-- moderated_at is when the verdict fell, which for a manual row is also
-- its publish moment (the review stream orders by it).
alter table point_of_sale
    add column moderation_note text,
    add column moderated_at    timestamptz;

-- The moderation review stream: recently published non-scrape rows. The
-- scrape is 62k rows and grows only via import; everything else is what
-- moderators watch.
create index point_of_sale_reviewable_idx on point_of_sale (created_at desc)
    where origin <> 'mcc_codes';

-- +goose Down
drop index point_of_sale_reviewable_idx;
alter table point_of_sale
    drop column moderated_at,
    drop column moderation_note;
alter table "user"
    drop column role;
drop type user_role;
-- 'rejected' stays in point_of_sale_status: Postgres cannot drop an enum
-- label (00007 precedent) — this Down is lossy by necessity.

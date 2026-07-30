-- Friends & cashback sharing (docs/specs/friends-sharing.md): a mutual
-- friend graph (invite links + username заявки) and per-bank_client read
-- grants so friends can answer «чьей картой платить?» across the circle.
--
-- Design notes:
-- * friendship stores the canonical pair (user_lo < user_hi) — exactly one
--   row per pair, membership checks are (user_lo = $u or user_hi = $u).
-- * friend_cashback_share references the FRIENDSHIP, not the two users:
--   unfriending revokes grants in both directions by cascade, and a
--   re-friendship (new row, new id) resurrects nothing. Grant direction is
--   derived, never stored: owner = bank_client.user_id, grantee = the other
--   member of the pair.
-- * The bank_client FK cascades too — deleting a client must drop its mere
--   shares, not 409 (bank-client-delete maps 23503 to a conflict).
-- * friend_invite stores a SHA-256 token hash only; the plaintext exists
--   once, in the create response. Claim burns the row atomically via a
--   conditional update.

-- +goose Up
create type friend_request_status as enum ('pending', 'accepted', 'declined', 'cancelled');

create table friendship
(
    id         bigint generated always as identity primary key,
    user_lo    uuid                     not null references "user" (id),
    user_hi    uuid                     not null references "user" (id),
    created_at timestamp with time zone not null default now(),
    check (user_lo < user_hi),
    unique (user_lo, user_hi)
);
create index friendship_user_hi_idx on friendship (user_hi);

create table friend_request
(
    id           bigint generated always as identity primary key,
    from_user_id uuid                     not null references "user" (id),
    to_user_id   uuid                     not null references "user" (id),
    status       friend_request_status    not null default 'pending',
    created_at   timestamp with time zone not null default now(),
    responded_at timestamp with time zone,
    check (from_user_id <> to_user_id)
);
-- One LIVE заявка per pair, regardless of direction; history rows keep
-- their terminal status.
create unique index friend_request_pending_pair_key
    on friend_request (least(from_user_id, to_user_id), greatest(from_user_id, to_user_id))
    where status = 'pending';
create index friend_request_inbox_idx on friend_request (to_user_id) where status = 'pending';

create table friend_invite
(
    id                 uuid primary key                  default gen_random_uuid(),
    created_by_user_id uuid                     not null references "user" (id),
    token_hash         bytea                    not null unique,
    created_at         timestamp with time zone not null default now(),
    expires_at         timestamp with time zone not null,
    claimed_at         timestamp with time zone,
    claimed_by_user_id uuid references "user" (id)
);

create table friend_cashback_share
(
    bank_client_id bigint                   not null references bank_client (id) on delete cascade,
    friendship_id  bigint                   not null references friendship (id) on delete cascade,
    created_at     timestamp with time zone not null default now(),
    primary key (bank_client_id, friendship_id)
);
create index friend_cashback_share_friendship_idx on friend_cashback_share (friendship_id);

-- Exact-but-case-insensitive username search (friends-search): registration
-- never normalized case, so the unique(username) constraint is
-- case-sensitive and this index only accelerates the lookup — the query
-- prefers the exact-case row when two usernames differ only by case.
create index user_username_lower_idx on "user" (lower(username));

-- +goose Down
drop index user_username_lower_idx;
drop table friend_cashback_share;
drop table friend_invite;
drop table friend_request;
drop table friendship;
drop type friend_request_status;

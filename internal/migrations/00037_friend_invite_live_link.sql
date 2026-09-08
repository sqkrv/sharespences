-- Invite links become multi-use and re-showable (redesign 4e, 2026-08-12).
--
-- A claim no longer creates the friendship — it files an ordinary incoming
-- friend request (via_invite marks it, «пришла по твоей ссылке»). With that,
-- the link's only capability is what any user already has through
-- exact-login search, so the plaintext token may live at rest: it is what
-- lets the app show the live link again («ссылка одна и живая»). Rows
-- created before this migration keep a null token — they were shown once
-- and cannot be re-displayed; the next «Создать новую» replaces them.
--
-- claimed_at / claimed_by_user_id stop being written (multi-use has no
-- burn); existing burned rows keep their terminal values as history.

-- +goose Up
alter table friend_invite
    add column token text;

alter table friend_request
    add column via_invite boolean not null default false;

-- +goose Down
alter table friend_request
    drop column via_invite;

alter table friend_invite
    drop column token;

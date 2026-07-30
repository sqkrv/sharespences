-- Canonical identity fields: a username rule the database enforces.
--
-- Before this migration nothing normalized either field. The consequences were
-- live: `Alice` and `alice` could both register (unique(username) is
-- byte-exact) while friend search matched case-insensitively and broke the tie
-- arbitrarily, so a заявка could reach the wrong account; a Cyrillic «аlice»
-- sat visually identical beside a Latin «alice»; a login could hold spaces,
-- emoji or kilobytes of text; and `Foo@x.com` could register but not sign in,
-- because login matched the email exactly.
--
-- The rule (authoritative copy with its rationale: internal/auth/domain.go):
-- 3–32 characters, lowercase Latin letters and digits, «.» and «_» as interior
-- separators only — never leading, trailing, doubled or adjacent — and a
-- leading letter.
--
-- ⚠️ This migration ABORTS rather than mangle data, by design: the case fold
-- hits unique(username) if two accounts collide on lowercase, and the
-- constraints are validated against existing rows. Both need a human decision
-- (which of two accounts keeps the name), not a guess. Run the pre-flight below
-- first — it lists every row that would block, in one pass. Feed it on stdin
-- through a quoted heredoc rather than psql -c "…": every column here is a
-- boolean flag, and an interactive zsh expands the «!» of a `!~` operator as a
-- history reference before psql ever sees it.
--
--   docker compose exec -T db psql -U sharespences <<'SQL'
--   select id, username, email, display_name,
--          username <> lower(username)                                as username_case,
--          count(*) over (partition by lower(username)) > 1           as username_collision,
--          not (lower(username) ~ '^[a-z][a-z0-9]*([._][a-z0-9]+)*$') as username_format,
--          char_length(username) not between 3 and 32                 as username_length,
--          email <> lower(email)                                      as email_case,
--          count(*) over (partition by lower(email)) > 1              as email_collision,
--          char_length(display_name) not between 1 and 64             as display_name_length
--   from "user"
--   order by username;
--   SQL
--
-- Everything false on every row ⇒ the migration applies cleanly. Any true in a
-- *_collision column ⇒ merge or rename by hand first; any other true is fixed
-- in place by the update statements here (case) or needs a rename (format,
-- length). There is no rename endpoint, so a rename is an UPDATE: friendships
-- key on user_id and survive it — only future searches use the new login.

-- +goose Up
update "user" set username = lower(username) where username <> lower(username);
update "user" set email = lower(email) where email <> lower(email);

alter table "user"
    add constraint user_username_format
        check (username ~ '^[a-z][a-z0-9]*([._][a-z0-9]+)*$' and char_length(username) between 3 and 32),
    add constraint user_email_lower
        check (email = lower(email)),
    add constraint user_display_name_len
        check (char_length(display_name) between 1 and 64);

-- Dead with the case fold: the search query matches `username = $1` against the
-- canonical form, which unique(username) already indexes. Control characters in
-- display_name are scrubbed by the API rather than constrained here — that keeps
-- this migration's failure surface to the username rule it exists for.
drop index user_username_lower_idx;

-- +goose Down
-- Lossy, like 00007: the original letter case of usernames and emails is gone
-- and cannot be restored. Down only removes the enforcement.
alter table "user"
    drop constraint user_display_name_len,
    drop constraint user_email_lower,
    drop constraint user_username_format;

create index user_username_lower_idx on "user" (lower(username));

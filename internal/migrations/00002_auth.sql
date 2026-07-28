-- Minimal password auth: email/password now, magic links when the auth
-- spec is written; WebAuthn stays parked.
-- `sessions` is the alexedwards/scs pgxstore schema.
-- attachment.user_id: attachments are user-owned rows, scope them like
-- everything else (spec: user_id scoping on all user-owned rows).

-- +goose Up
alter table "user"
    add column password_hash text;

create table sessions
(
    token  text primary key,
    data   bytea       not null,
    expiry timestamptz not null
);
create index sessions_expiry_idx on sessions (expiry);

alter table attachment
    add column user_id uuid references "user" (id);

-- +goose Down
alter table attachment
    drop column user_id;
drop table sessions;
alter table "user"
    drop column password_hash;

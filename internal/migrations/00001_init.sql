-- Port of init-database.sql (the v1 Python schema, the project's real spec)
-- into goose. Deliberate omissions:
--   * legacy `cashback` table — superseded by the cashback module tables
--     (00003_cashback.sql);
--   * `passkey` table — WebAuthn auth is parked, not to be extended.
-- The destructive `drop owned by` prologue is gone: migrations own the schema.

-- +goose Up
create extension if not exists postgis;

create type payment_system as enum ('visa', 'mastercard', 'mir', 'unionpay', 'american_express');
create type transaction_status as enum ('hold', 'success');
create type transaction_direction as enum ('expense', 'income');
create type period as enum ('day', 'week', 'month', 'year');
create type point_of_sale_type as enum ('offline', 'online', 'app', 'other');

create domain money_type as numeric(19, 4);

create table "user"
(
    id           uuid primary key                  default gen_random_uuid(),
    username     text                     not null unique,
    display_name text                     not null,
    email        text                     not null unique,
    created_at   timestamp with time zone not null default now(),
    telegram_id  bigint unique
);

create table attachment
(
    id         uuid primary key default gen_random_uuid(),
    filename   text not null,
    media_type text
);

create table bank
(
    id            serial primary key,
    name          text not null,
    logo_filename text,
    color_hex     text check (color_hex ~* '^#[0-9a-f]{6}$')
);

create table bank_card
(
    id             serial primary key,
    bank_id        smallint       not null references bank (id),
    user_id        uuid           not null references "user" (id),
    last_4_digits  integer        not null,
    payment_system payment_system not null,
    image_filename text
);

create table category
(
    id                         serial primary key,
    bank_id                    smallint not null references bank (id),
    name                       text     not null,
    description                text,
    mcc_additional_description text,
    icon_filename              text,
    og_id                      integer
);

create table mcc
(
    code        smallint primary key,
    name        text not null,
    description text
);

create table bank_mcc
(
    bank_id  smallint not null references bank (id),
    mcc_code smallint not null references mcc (code),
    footnote text,
    primary key (bank_id, mcc_code)
);

create table category_mcc
(
    category_id integer  not null references category (id),
    mcc_code    smallint not null references mcc (code),
    primary key (category_id, mcc_code)
);

create table article
(
    id    serial primary key,
    title text not null,
    text  text not null
);

create table subscription
(
    id                  serial primary key,
    name                text       not null,
    price               money_type not null,
    recurrence_date     date       not null,
    recurrence_interval interval,
    bank_card_id        integer references bank_card (id),
    is_active           boolean default true,
    notes               text,
    icon_filename       text
);

create table subscription_member
(
    subscription_id integer                  not null references subscription (id),
    user_id         uuid                     not null references "user" (id),
    since           timestamp with time zone not null,
    is_payer        boolean                  not null default false,
    primary key (subscription_id, user_id)
);

create table point_of_sale
(
    id                uuid primary key     default gen_random_uuid(),
    name              text        not null,
    merchant_title    text,
    mcc_code          smallint references mcc (code),
    type              point_of_sale_type,
    address           text,
    confirmations     bigint,
    created_at        timestamptz not null default now(),
    last_confirmed_at timestamptz,
    location          geometry(Point, 4326)
);

create table transaction
(
    id                    uuid primary key default gen_random_uuid(),
    user_id               uuid                     not null references "user" (id),
    og_id                 text                     not null,
    timestamp             timestamp with time zone not null,
    title                 text                     not null,
    amount                money_type               not null,
    direction             transaction_direction    not null,
    bank_id               smallint references bank (id),
    merchandiser_logo_url text,
    bank_comment          text,
    mcc_code              smallint,
    category_id           integer references category (id),
    loyalty_amount        money_type,
    status                transaction_status       not null,
    location              geometry(Point, 4326),
    bank_card_id          integer references bank_card (id),
    subscription_id       integer references subscription (id),
    user_comment          text
);
create index idx_transaction_location on transaction using gist (location);

create table transaction_attachment
(
    transaction_id uuid not null references transaction (id),
    attachment_id  uuid not null references attachment (id),
    primary key (transaction_id, attachment_id)
);

create table receipt_position
(
    id             uuid primary key default gen_random_uuid(),
    transaction_id uuid       not null references transaction (id),
    name           text       not null,
    quantity       real       not null,
    total          money_type not null
);

create table transaction_user
(
    transaction_id     uuid    not null references transaction (id),
    user_id            uuid    not null references "user" (id),
    position_id        uuid references receipt_position (id),
    equal_distribution boolean not null,
    primary key (transaction_id, user_id)
);

-- +goose Down
drop table transaction_user;
drop table receipt_position;
drop table transaction_attachment;
drop table transaction;
drop table point_of_sale;
drop table subscription_member;
drop table subscription;
drop table article;
drop table category_mcc;
drop table bank_mcc;
drop table mcc;
drop table category;
drop table bank_card;
drop table bank;
drop table attachment;
drop table "user";
drop domain money_type;
drop type point_of_sale_type;
drop type period;
drop type transaction_direction;
drop type transaction_status;
drop type payment_system;

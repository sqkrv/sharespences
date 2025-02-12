drop owned by sharespences cascade;

create extension if not exists postgis;

create type payment_system as enum ('visa', 'mastercard', 'mir', 'unionpay', 'american_express');
create type transaction_status as enum ('hold', 'success');
create type transaction_direction as enum ('expense', 'income');

create domain money_type as numeric(19, 4);

create table "user"
(
    id           uuid primary key                  default gen_random_uuid(),
    username     text                     not null unique,
    display_name text                     not null,
    email        text                     not null unique,
    created_at   timestamp with time zone not null default now()
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
    logo_filename text
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

create table cashback
(
    id             uuid primary key          default gen_random_uuid(),
    user_id        uuid             not null references "user" (id),
    category_id    integer          not null references category (id),
    start_date     date             not null,
    end_date       date             not null,
    percentage     double precision not null,
    super_cashback boolean          not null default false
);
comment on column cashback.start_date is 'When cashback takes effect';
comment on column cashback.end_date is 'When cashback loses effect';
comment on column cashback.super_cashback is 'Whether cashback is super (e.g. gotten from the fortune wheel) and adds up with another cashback by the category. Currently applies only to Alfa-Bank';

create table mcc_code
(
    code        smallint primary key,
    name        text not null,
    description text
);

create table bank_mcc
(
    bank_id  smallint not null references bank (id),
    mcc_code smallint not null references mcc_code (code),
    footnote text,
    primary key (bank_id, mcc_code)
);

create table category_mcc
(
    category_id integer  not null references category (id),
    mcc_code    smallint not null references mcc_code (code),
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
    id   serial primary key,
    name text not null
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

create table passkey
(
    id         text primary key, -- Base64URL encoded CredentialID
    user_id    uuid not null references "user" (id),
    name       text not null,
    public_key text not null     -- Base64URL encoded PublicKey
);
comment on column passkey.id is 'Base64URL encoded CredentialID';
comment on column passkey.public_key is 'Base64URL encoded PublicKey';

create table subscription_member
(
    subscription_id integer                  not null references subscription (id),
    user_id         uuid                     not null references "user" (id),
    since           timestamp with time zone not null default now(),
    primary key (subscription_id, user_id)
);

-- Cashback module v1 tables, per docs/specs/cashback.md (private meta-repo).
-- Seam decision (spec left it open): the cashback module OWNS the bank_card
-- extension (program_tier_id) and may read bank / bank_card as reference
-- data; no other module reads cashback tables.

-- +goose Up
create extension if not exists btree_gist;

create type cashback_period_type as enum ('calendar_month', 'quarter', 'week', 'rolling');
create type cashback_selection_mode as enum ('atomic', 'incremental');
create type cashback_currency_kind as enum ('rub', 'points');
create type cashback_cap_scope as enum ('total', 'per_category', 'both');
create type cashback_offer_kind as enum ('regular', 'special');

-- One per bank (rarely >1).
create table cashback_program
(
    id                  bigint generated always as identity primary key,
    bank_id             integer                 not null references bank (id),
    name                text                    not null,
    period_type         cashback_period_type    not null,
    selection_mode      cashback_selection_mode not null,
    currency_kind       cashback_currency_kind  not null,
    points_label        text,     -- 'Баллы Плюс', 'баллы МКБ'
    selection_opens_day integer,  -- 25 (Альфа-Банк/Озон), 26 (ВТБ); passive display only
    notes               text,
    unique (bank_id, name)
);

-- Client level, often a PAID subscription — decides caps/slots/%.
-- Caps are static reference values in v1 (remaining-cap tracking deferred).
create table program_tier
(
    id                   bigint generated always as identity primary key,
    program_id           bigint             not null references cashback_program (id),
    name                 text               not null, -- 'Стандартный', 'Альфа-Смарт', 'Alfa Only', 'Привилегия'…
    is_paid_subscription boolean            not null default false,
    cap_value            numeric,                     -- null = unknown (Газпромбанк)
    cap_scope            cashback_cap_scope not null default 'total',
    cap_per_category     numeric,                     -- when cap_scope ∈ {per_category, both}
    max_categories       integer,
    base_percent         numeric,                     -- headline %, offers override per row
    notes                text,
    unique (program_id, name)
);

-- Tier is mutable (user can sub/unsub a client level): card → tier FK.
alter table bank_card
    add column program_tier_id bigint references program_tier (id);

-- Cross-bank category identity for overlap detection.
create table canonical_category
(
    id       bigint generated always as identity primary key,
    slug     text not null unique, -- 'supermarkets'
    title_ru text not null         -- «Супермаркеты»
);

-- «Продукты» (Озон) → supermarkets. One mapping per (bank, raw title).
create table bank_category_alias
(
    canonical_category_id bigint  not null references canonical_category (id),
    bank_id               integer not null references bank (id),
    raw_title             text    not null,
    primary key (bank_id, raw_title)
);

-- One card × one selection period. Invariant 4 (ranges never overlap per
-- card) is enforced twice: in the service and by the exclusion constraint.
create table offer_period
(
    id           bigint generated always as identity primary key,
    card_id      integer not null references bank_card (id),
    period_start date    not null,
    period_end   date    not null,
    check (period_start <= period_end),
    unique (card_id, period_start),
    constraint offer_period_no_overlap
        exclude using gist (card_id with =, daterange(period_start, period_end, '[]') with &&)
);

-- Screenshot evidence (bank-app menus).
create table offer_period_attachment
(
    offer_period_id bigint not null references offer_period (id),
    attachment_id   uuid   not null references attachment (id),
    primary key (offer_period_id, attachment_id)
);

-- One row of the bank's offered menu, exactly as the bank shows it.
create table category_offer
(
    id                    bigint generated always as identity primary key,
    offer_period_id       bigint              not null references offer_period (id),
    raw_title             text                not null,
    canonical_category_id bigint references canonical_category (id),
    percent               numeric,
    kind                  cashback_offer_kind not null default 'regular', -- special = барабан/пятница/колесо rows, record-only
    notes                 text
);

-- Dated event, not a flag — supports incremental banks (Яндекс Пэй).
-- An offer is selected at most once.
create table selection
(
    id                bigint generated always as identity primary key,
    category_offer_id bigint      not null unique references category_offer (id),
    selected_at       timestamptz not null
);

-- Record-only in v1.
create table partner_offer
(
    id             bigint generated always as identity primary key,
    user_id        uuid    not null references "user" (id),
    bank_id        integer not null references bank (id),
    card_id        integer references bank_card (id),
    merchant_title text    not null,
    percent        numeric,
    valid_from     date,
    valid_to       date,
    cap_value      numeric,
    notes          text
);

create table partner_offer_attachment
(
    partner_offer_id bigint not null references partner_offer (id),
    attachment_id    uuid   not null references attachment (id),
    primary key (partner_offer_id, attachment_id)
);

-- +goose Down
drop table partner_offer_attachment;
drop table partner_offer;
drop table selection;
drop table category_offer;
drop table offer_period_attachment;
drop table offer_period;
drop table bank_category_alias;
drop table canonical_category;
alter table bank_card
    drop column program_tier_id;
drop table program_tier;
drop table cashback_program;
drop type cashback_offer_kind;
drop type cashback_cap_scope;
drop type cashback_currency_kind;
drop type cashback_selection_mode;
drop type cashback_period_type;

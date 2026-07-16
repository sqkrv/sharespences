-- Program-level facts the S3b «Можно выбрать» flow needs (spec S3b, owner
-- 2026-07-16): whether a category can be ADDED mid-period, and when a fresh
-- pick starts paying. Neither is derivable from selection_mode — Альфа is
-- atomic yet allows mid-month adds while a slot is free, while ВТБ/Озон
-- (also atomic) lock after the one-shot confirmation; МКБ charges for a
-- mid-quarter change AND activates it next day.

-- +goose Up
create type cashback_mid_period_add as enum ('allowed', 'locked_after_first', 'paid', 'unknown');
create type cashback_activation as enum ('immediate', 'next_day', 'unknown');

alter table cashback_program
    add column mid_period_add cashback_mid_period_add not null default 'unknown',
    add column activation     cashback_activation     not null default 'unknown';

-- +goose Down
alter table cashback_program
    drop column mid_period_add,
    drop column activation;
drop type cashback_activation;
drop type cashback_mid_period_add;

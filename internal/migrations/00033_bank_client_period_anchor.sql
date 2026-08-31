-- Two banks anchor the cashback period to the client's own billing cycle rather
-- than the calendar: Ozon Банк («3000₽ за расчетный период (у всех свой)») and
-- Совкомбанк (the период runs from the card's issue date). cashback_program
-- .period_type is a property of the PROGRAM, so it cannot express a start day
-- that differs per client, and Ozon currently ships `calendar_month`, which is
-- simply wrong for it.
--
-- ADR-0009 §2: the anchor is a per-client HINT, not a constraint. offer_period
-- already stores explicit period_start/period_end, so a shifted cycle was always
-- representable — the only thing broken was the default the app pre-fills when
-- the user opens «Новый период». period_type keeps saying how long a period is;
-- the anchor says which day it starts on.
--
-- Nullable, so null = follow the programme's period_type, which is what every
-- existing client wants and means no backfill. Deliberately NOT a new
-- `billing_cycle` enum label: 00007 established that Postgres cannot drop an
-- enum label, making that direction one-way, and a nullable column carries the
-- same information reversibly.
--
-- Enforcement of period sanity stays where it already is — the no-overlap
-- exclusion constraint from 00006 — because a hint that lies must never be able
-- to create a broken period.

-- +goose Up
alter table bank_client
    add column period_anchor_day smallint
        check (period_anchor_day between 1 and 31);

comment on column bank_client.period_anchor_day is
    'Day of month the client''s cashback period starts on. Null = follow cashback_program.period_type (the calendar). A pre-fill hint for new periods, never a constraint — see ADR-0009 §2.';

-- +goose Down
alter table bank_client
    drop column period_anchor_day;

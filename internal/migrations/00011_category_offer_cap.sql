-- Observed in the ВТБ menu: specific categories can carry their
-- own КБ cap for the period («Театры и кино — кешбэк до 5 000 ₽») while the
-- program-wide cap keeps burning. Static display only in v1 — no spend
-- tracking (per the spec's OUT list); shown offer-cap-first over the tier cap.

-- +goose Up
alter table category_offer
    add column cap_value numeric;
comment on column category_offer.cap_value is
    'Per-offer КБ cap for the period (ВТБ «Кешбэк до N ₽» rows); null = tier cap applies; static display, no tracking';

-- +goose Down
alter table category_offer
    drop column cap_value;

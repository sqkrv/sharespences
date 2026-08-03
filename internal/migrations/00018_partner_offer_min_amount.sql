-- +goose Up
-- Minimum purchase amount a partner offer requires («от 3 000 ₽»). Sits
-- beside cap_value, which bounds the payout — this bounds the qualifying
-- purchase, and the two are independent: an offer can have either, both or
-- neither. Static display like cap_value; nothing tracks spend against it.
--
-- numeric, not money_type, to match the sibling percent/cap_value columns on
-- this table. Both map to decimal.Decimal through the sqlc overrides.
alter table partner_offer
    add column min_amount numeric;

comment on column partner_offer.min_amount is
    'minimum qualifying purchase, e.g. «при заказе от 2 000 ₽»; display only';

-- +goose Down
alter table partner_offer
    drop column min_amount;

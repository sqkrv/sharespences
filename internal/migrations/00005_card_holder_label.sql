-- Owner decision 2026-07-09: holder_label — whose plastic this is
-- («Мама», «Стас»…): the household runs several family members' cards
-- under one account. Display/grouping only; upgrades to a держатель
-- entity in the Groups/household era.
-- (An earlier draft also added a kind=base enum value here; the owner
-- corrected the semantics — «За все покупки» is an ordinary selectable
-- category — so no schema change for it is needed.)

-- +goose Up
alter table bank_card
    add column holder_label text;
comment on column bank_card.holder_label is
    'Family member holding this plastic; null = the account owner themselves';

-- +goose Down
alter table bank_card
    drop column holder_label;

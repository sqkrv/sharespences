-- Bank-client re-keying: КБ selections apply to
-- the BANK CLIENT (person × bank), not the plastic — all of a client's cards
-- share the selection, and program tiers are client-level subscriptions
-- (docs/knowledge/concepts/cashback-mechanics.md). bank_client absorbs
-- держатель (label) + program_tier_id from bank_card; offer_period and
-- partner_offer re-key from card to client; bank_card is fully normalized
-- down to the plastic itself (client, last 4, payment system, image).
--
-- `unique nulls not distinct` pins PostgreSQL ≥ 15 (compose/e2e ship 16):
-- label NULL = the account owner, and there is exactly one self-relationship
-- per (user, bank).

-- +goose Up
create table bank_client
(
    id              bigint generated always as identity primary key,
    user_id         uuid    not null references "user" (id),
    bank_id         integer not null references bank (id),
    label           text, -- держатель («Мама», «Стас»); null = the account owner
    program_tier_id bigint references program_tier (id),
    unique nulls not distinct (user_id, bank_id, label)
);

-- Backfill 1: clients from the card fleet. Cards of one (user, bank, holder)
-- group could disagree on tier; max() is an arbitrary-but-deterministic pick.
insert into bank_client (user_id, bank_id, label, program_tier_id)
select bc.user_id, bc.bank_id::integer, bc.holder_label, max(bc.program_tier_id)
from bank_card bc
group by bc.user_id, bc.bank_id, bc.holder_label;

-- Backfill 2: card → client.
alter table bank_card
    add column bank_client_id bigint references bank_client (id);
update bank_card bc
set bank_client_id = cl.id
from bank_client cl
where cl.user_id = bc.user_id
  and cl.bank_id = bc.bank_id
  and cl.label is not distinct from bc.holder_label;
alter table bank_card
    alter column bank_client_id set not null;

-- Backfill 3: offer_period / partner_offer re-key through the card's client.
alter table offer_period
    add column bank_client_id bigint references bank_client (id);
update offer_period op
set bank_client_id = bc.bank_client_id
from bank_card bc
where bc.id = op.card_id;
alter table offer_period
    alter column bank_client_id set not null;

alter table partner_offer
    add column bank_client_id bigint references bank_client (id);
update partner_offer po
set bank_client_id = bc.bank_client_id
from bank_card bc
where bc.id = po.card_id;
alter table partner_offer
    drop column card_id;

-- Dedupe: two cards of one client may have carried overlapping periods (the
-- exact case the re-keying fixes — the menu entered twice). The period with
-- more category_offer rows wins; ties break to the lower id. Anything this
-- pass misses fails the exclusion-constraint add below — loudly, never
-- silently.
-- +goose StatementBegin
do
$$
    declare
        loser  record;
        beaten boolean;
    begin
        for loser in
            select op.id, op.bank_client_id, op.period_start, op.period_end,
                   (select count(*) from category_offer co where co.offer_period_id = op.id) as n
            from offer_period op
            order by op.id
            loop
                select exists (
                    select 1
                    from offer_period o2
                    where o2.bank_client_id = loser.bank_client_id
                      and o2.id <> loser.id
                      and daterange(o2.period_start, o2.period_end, '[]') &&
                          daterange(loser.period_start, loser.period_end, '[]')
                      and ((select count(*) from category_offer co where co.offer_period_id = o2.id) > loser.n
                        or ((select count(*) from category_offer co where co.offer_period_id = o2.id) = loser.n
                            and o2.id < loser.id))
                ) into beaten;
                if beaten then
                    raise notice 'bank-client re-key: dropping offer_period % (client %, % .. %) — a richer/older sibling period wins',
                        loser.id, loser.bank_client_id, loser.period_start, loser.period_end;
                    delete
                    from selection s using category_offer co
                    where co.offer_period_id = loser.id
                      and s.category_offer_id = co.id;
                    delete from category_offer where offer_period_id = loser.id;
                    delete from offer_period_attachment where offer_period_id = loser.id;
                    delete from offer_period where id = loser.id;
                end if;
            end loop;
    end
$$;
-- +goose StatementEnd

-- Constraints: invariant 4 (no overlapping periods) is now per client.
-- btree_gist for `bigint with =` exists since 00003.
alter table offer_period
    add constraint offer_period_client_start_key unique (bank_client_id, period_start);
alter table offer_period
    drop constraint offer_period_no_overlap;
alter table offer_period
    add constraint offer_period_no_overlap
        exclude using gist (bank_client_id with =, daterange(period_start, period_end, '[]') with &&);
alter table offer_period
    drop column card_id; -- drops unique(card_id, period_start) + its FK too

-- bank_card slims to the plastic itself (full normalization):
-- bank/user derive via bank_client; держатель and tier live on the client.
alter table bank_card
    drop column holder_label,
    drop column program_tier_id,
    drop column bank_id,
    drop column user_id;

-- +goose Down
-- Best-effort, LOSSY inverse: the dedupe above cannot be undone, and each
-- offer_period is re-attached to the client's lowest-id card.
alter table bank_card
    add column holder_label text,
    add column program_tier_id bigint references program_tier (id),
    add column bank_id smallint references bank (id),
    add column user_id uuid references "user" (id);
update bank_card bc
set holder_label    = cl.label,
    program_tier_id = cl.program_tier_id,
    bank_id         = cl.bank_id::smallint,
    user_id         = cl.user_id
from bank_client cl
where cl.id = bc.bank_client_id;
alter table bank_card
    alter column bank_id set not null,
    alter column user_id set not null;

alter table offer_period
    add column card_id integer references bank_card (id);
update offer_period op
set card_id = (select min(bc.id)
               from bank_card bc
               where bc.bank_client_id = op.bank_client_id);
alter table offer_period
    alter column card_id set not null;
alter table offer_period
    drop constraint offer_period_no_overlap;
alter table offer_period
    add constraint offer_period_no_overlap
        exclude using gist (card_id with =, daterange(period_start, period_end, '[]') with &&);
alter table offer_period
    add constraint offer_period_card_id_period_start_key unique (card_id, period_start);
alter table offer_period
    drop constraint offer_period_client_start_key;
alter table offer_period
    drop column bank_client_id;

alter table partner_offer
    add column card_id integer references bank_card (id);
update partner_offer po
set card_id = (select min(bc.id)
               from bank_card bc
               where bc.bank_client_id = po.bank_client_id);
alter table partner_offer
    drop column bank_client_id;

alter table bank_card
    drop column bank_client_id;
drop table bank_client;

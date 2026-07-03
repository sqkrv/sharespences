-- name: ListBanks :many
select *
from bank
order by id;

-- name: GetBankByName :one
select *
from bank
where name = $1;

-- name: CreateCard :one
insert into bank_card (bank_id, user_id, last_4_digits, payment_system, program_tier_id)
values ($1, $2, $3, $4, $5)
returning *;

-- name: ListCardsForUser :many
select bc.*, b.name as bank_name
from bank_card bc
         join bank b on b.id = bc.bank_id
where bc.user_id = $1
order by bc.id;

-- name: GetCardForUser :one
select bc.*, b.name as bank_name
from bank_card bc
         join bank b on b.id = bc.bank_id
where bc.id = $1
  and bc.user_id = $2;

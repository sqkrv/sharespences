-- name: ListBanks :many
select *
from bank
order by id;

-- name: GetBankByName :one
select *
from bank
where name = $1;

-- name: CreateBankClient :one
insert into bank_client (user_id, bank_id, label, program_tier_id)
values ($1, $2, $3, $4)
returning *;

-- name: UpdateBankClientForUser :one
update bank_client
set label           = $3,
    program_tier_id = $4
where id = $1
  and user_id = $2
returning *;

-- name: ListBankClientsForUser :many
select cl.*, b.name as bank_name
from bank_client cl
         join bank b on b.id = cl.bank_id
where cl.user_id = $1
order by cl.id;

-- name: GetBankClientForUser :one
select cl.*, b.name as bank_name
from bank_client cl
         join bank b on b.id = cl.bank_id
where cl.id = $1
  and cl.user_id = $2;

-- CreateCard authorizes and derives in one statement: inserting through a
-- select on the user's own bank_client returns 0 rows (pgx.ErrNoRows → 404)
-- when the client does not exist or belongs to someone else.
-- name: CreateCard :one
insert into bank_card (bank_client_id, last_4_digits, payment_system)
select cl.id, sqlc.arg(last_4_digits)::integer, sqlc.arg(payment_system)::payment_system
from bank_client cl
where cl.id = sqlc.arg(bank_client_id)
  and cl.user_id = sqlc.arg(user_id)
returning *;

-- name: UpdateCardForUser :one
update bank_card bc
set last_4_digits  = $3,
    payment_system = $4
from bank_client cl
where bc.id = $1
  and cl.id = bc.bank_client_id
  and cl.user_id = $2
returning bc.*;

-- name: ListCardsForUser :many
select bc.*, cl.bank_id, b.name as bank_name
from bank_card bc
         join bank_client cl on cl.id = bc.bank_client_id
         join bank b on b.id = cl.bank_id
where cl.user_id = $1
order by bc.id;

-- name: GetCardForUser :one
select bc.*, cl.bank_id, b.name as bank_name
from bank_card bc
         join bank_client cl on cl.id = bc.bank_client_id
         join bank b on b.id = cl.bank_id
where bc.id = $1
  and cl.user_id = $2;

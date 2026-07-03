-- name: CreateUser :one
insert into "user" (username, display_name, email, password_hash)
values ($1, $2, $3, $4)
returning *;

-- name: GetUserByEmail :one
select *
from "user"
where email = $1;

-- name: GetUserByID :one
select *
from "user"
where id = $1;

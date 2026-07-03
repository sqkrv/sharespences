-- name: CreateAttachment :one
insert into attachment (id, filename, media_type, user_id)
values ($1, $2, $3, $4)
returning *;

-- name: GetAttachmentForUser :one
select *
from attachment
where id = $1
  and user_id = $2;

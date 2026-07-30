-- Friends module queries. The module owns friendship / friend_request /
-- friend_invite / friend_cashback_share; joins to "user" and bank_client are
-- read-only reference reads (ADR-0002 seam). The shared-picture read path
-- stays in cashback.sql (cashback is the only reader of cashback tables) —
-- this file only resolves WHICH clients are visible to whom.

-- name: GetUserByUsernameCI :one
-- Exact match, case-insensitive (user_username_lower_idx). username is
-- unique only case-sensitively, so two rows can differ by case alone —
-- the exact-case row wins then.
select *
from "user"
where lower(username) = lower(sqlc.arg(username))
order by (username = sqlc.arg(username)) desc
limit 1;

-- name: CreateFriendship :one
insert into friendship (user_lo, user_hi)
values ($1, $2)
returning *;

-- name: GetFriendshipByPair :one
select *
from friendship
where user_lo = $1
  and user_hi = $2;

-- name: DeleteFriendshipByPair :execrows
delete
from friendship
where user_lo = $1
  and user_hi = $2;

-- name: ListFriendsForUser :many
select f.id         as friendship_id,
       f.created_at as since,
       u.id         as user_id,
       u.username,
       u.display_name
from friendship f
         join "user" u on u.id = case when f.user_lo = sqlc.arg(user_id) then f.user_hi else f.user_lo end
where f.user_lo = sqlc.arg(user_id)
   or f.user_hi = sqlc.arg(user_id)
order by u.display_name, u.username;

-- name: CreateFriendRequest :one
insert into friend_request (from_user_id, to_user_id)
values ($1, $2)
returning *;

-- name: GetPendingRequestBetween :one
select *
from friend_request
where status = 'pending'
  and ((from_user_id = $1 and to_user_id = $2) or (from_user_id = $2 and to_user_id = $1));

-- name: ListPendingRequestsForUser :many
select fr.id,
       fr.from_user_id,
       fr.to_user_id,
       fr.created_at,
       fu.username     as from_username,
       fu.display_name as from_display_name,
       tu.username     as to_username,
       tu.display_name as to_display_name
from friend_request fr
         join "user" fu on fu.id = fr.from_user_id
         join "user" tu on tu.id = fr.to_user_id
where fr.status = 'pending'
  and (fr.from_user_id = sqlc.arg(user_id) or fr.to_user_id = sqlc.arg(user_id))
order by fr.created_at desc;

-- name: SetRequestStatusForRecipient :one
update friend_request
set status       = $3,
    responded_at = now()
where id = $1
  and to_user_id = $2
  and status = 'pending'
returning *;

-- name: CancelRequestForSender :execrows
update friend_request
set status       = 'cancelled',
    responded_at = now()
where id = $1
  and from_user_id = $2
  and status = 'pending';

-- name: CreateFriendInvite :one
insert into friend_invite (created_by_user_id, token_hash, expires_at)
values ($1, $2, $3)
returning *;

-- name: ListLiveInvitesForUser :many
select *
from friend_invite
where created_by_user_id = $1
  and claimed_at is null
  and expires_at > now()
order by created_at desc;

-- name: DeleteInviteForUser :execrows
delete
from friend_invite
where id = $1
  and created_by_user_id = $2;

-- name: GetInviteByTokenHash :one
select *
from friend_invite
where token_hash = $1;

-- ClaimInvite is the atomic burn (invariant 5): the conditional update
-- either claims a live, unexpired invite or matches nothing — the service
-- distinguishes burned from expired via GetInviteByTokenHash afterwards.
-- name: ClaimInvite :one
update friend_invite
set claimed_at         = now(),
    claimed_by_user_id = $2
where token_hash = $1
  and claimed_at is null
  and expires_at > now()
returning *;

-- name: CreateShare :exec
insert into friend_cashback_share (bank_client_id, friendship_id)
values ($1, $2)
on conflict do nothing;

-- name: DeleteShare :execrows
delete
from friend_cashback_share
where bank_client_id = $1
  and friendship_id = $2;

-- name: ListSharesForOwner :many
-- Grants the user has issued: which of their clients each friend sees.
select s.bank_client_id,
       f.id                                                       as friendship_id,
       case when f.user_lo = sqlc.arg(owner_id) then f.user_hi else f.user_lo end as friend_user_id
from friend_cashback_share s
         join friendship f on f.id = s.friendship_id
         join bank_client cl on cl.id = s.bank_client_id
where cl.user_id = sqlc.arg(owner_id);

-- name: ListSharedWithViewer :many
-- Everything friends currently share with the viewer. The friendship join
-- is belt and braces over the FK cascade (invariant 2); grant direction is
-- derived, so the viewer's own clients are excluded explicitly.
select cl.id      as bank_client_id,
       cl.user_id as owner_user_id,
       u.username,
       u.display_name
from friend_cashback_share s
         join friendship f on f.id = s.friendship_id
         join bank_client cl on cl.id = s.bank_client_id
         join "user" u on u.id = cl.user_id
where (f.user_lo = sqlc.arg(viewer_id) or f.user_hi = sqlc.arg(viewer_id))
  and cl.user_id <> sqlc.arg(viewer_id)
order by u.display_name, cl.id;

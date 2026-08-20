-- MCC module queries. Seam note: joins to bank / bank_category /
-- canonical_category are the module's read-only reference reads (same seam
-- practice as cashback.sql); no other module touches mcc /
-- bank_category_mcc / mcc_change. Seed writes via pool.Exec (seed.go
-- precedent).

-- name: GetMCC :one
select code, name, description
from mcc
where code = $1;

-- name: SearchMCC :many
select code, name, description
from mcc
where case
          when sqlc.arg(is_numeric)::bool
              then lpad(code::text, 4, '0') like sqlc.arg(query)::text || '%'
          else name ilike '%' || sqlc.arg(query)::text || '%'
    end
order by code
limit sqlc.arg(max_rows);

-- name: ResolveMCC :many
select b.id     as bank_id,
       b.name   as bank_name,
       b.color_hex,
       bc.id    as bank_category_id,
       bc.title,
       bc.kind,
       bc.emoji as bank_emoji,
       cc.emoji as canonical_emoji,
       cc.slug  as canonical_slug,
       cc.title_ru as canonical_title,
       bcm.note
from bank_category_mcc bcm
         join bank_category bc on bc.id = bcm.bank_category_id
         join bank b on b.id = bc.bank_id
         left join canonical_category cc on cc.id = bc.canonical_category_id
where bcm.mcc_code = $1
  and bc.active
order by b.name, bc.title;

-- name: SearchMerchants :many
-- Pending user submissions are visible to their author only (5e): the общий
-- каталог serves approved rows. Rejected rows are invisible to everyone,
-- the author included (roles-moderation invariant 3).
select id,
       name,
       merchant_title,
       mcc_code,
       coalesce(type::text, '')::text as pos_type,
       address,
       confirmations,
       user_confirmations,
       last_confirmed_at,
       status,
       origin::text as origin
from point_of_sale
where mcc_code is not null -- a merchant row without an MCC answers nothing here
  and (status = 'approved' or (author_user_id = sqlc.arg(user_id)::uuid and status = 'pending'))
  and (name ilike '%' || sqlc.arg(query)::text || '%'
    or merchant_title ilike '%' || sqlc.arg(query)::text || '%')
order by confirmations desc nulls last, last_confirmed_at desc nulls last, name
limit sqlc.arg(max_rows);

-- name: FindSimilarPointsOfSale :many
-- The 5e duplicate net: same MCC and either name contains the other —
-- «Хлебник» must catch a new «Пекарня Хлебник» before a copy is created.
select id, name, merchant_title, mcc_code
from point_of_sale
where mcc_code = sqlc.arg(mcc_code)
  and (status = 'approved' or (author_user_id = sqlc.arg(user_id)::uuid and status = 'pending'))
  and (name ilike '%' || sqlc.arg(name)::text || '%'
    or sqlc.arg(name)::text ilike '%' || name || '%')
order by confirmations desc nulls last, name
limit 3;

-- name: CreateUserPointOfSale :one
insert into point_of_sale (name, merchant_title, mcc_code, type, address, status, author_user_id, origin)
values ($1, $2, $3, $4, $5, 'pending', $6, 'user_manual')
returning *;

-- Moderation (roles-moderation.md). The queue is ANONYMOUS by column
-- selection: author_user_id is deliberately never selected here — the
-- moderator-facing DTO cannot carry what the query never returns
-- (invariant 1). Writes are scoped to non-scrape rows: the 62k imported
-- rows are the operator's domain (sidecar), not the moderators'.

-- name: ModerationListPendingPOS :many
select id,
       name,
       merchant_title,
       mcc_code,
       coalesce(type::text, '')::text as pos_type,
       address,
       origin::text                   as origin,
       created_at,
       count(*) over ()::bigint       as total
from point_of_sale
where status = 'pending'
order by created_at, id
limit sqlc.arg(max_rows) offset sqlc.arg(skip);

-- name: ModerationListPublishedPOS :many
-- The review stream: recently published non-scrape rows — what keeps the
-- instant-publish path (user_transaction) supervised after the fact.
select id,
       name,
       merchant_title,
       mcc_code,
       coalesce(type::text, '')::text as pos_type,
       address,
       origin::text                   as origin,
       created_at,
       count(*) over ()::bigint       as total
from point_of_sale
where status = 'approved'
  and origin <> 'mcc_codes'
order by created_at desc, id
limit sqlc.arg(max_rows) offset sqlc.arg(skip);

-- name: ModerationApprovePOS :execrows
update point_of_sale
set status = 'approved'
where id = $1
  and status = 'pending';

-- name: ModerationRejectPOS :execrows
-- Reject doubles as the review stream's prune: a published non-scrape row
-- can be pulled back. Rejected rows are kept for audit.
update point_of_sale
set status = 'rejected'
where id = $1
  and status in ('pending', 'approved')
  and origin <> 'mcc_codes';

-- name: ListMCCChanges :many
select mc.id,
       mc.bank_id,
       b.name as bank_name,
       mc.bank_category_id,
       mc.category_title,
       mc.mcc_code,
       mc.action,
       mc.noted_at,
       mc.source,
       mc.note
from mcc_change mc
         join bank b on b.id = mc.bank_id
order by mc.noted_at desc, mc.id desc
limit $1;

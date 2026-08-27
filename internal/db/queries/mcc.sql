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
where bcm.mcc_code = sqlc.arg(mcc_code)
  and bc.active
  and (bc.created_by is null or bc.created_by = sqlc.arg(user_id)::uuid)
order by b.name, bc.title;

-- name: SearchMerchants :many
-- Pending user submissions are visible to their author only (5e): the общий
-- каталог serves approved rows. Rejected rows are invisible to everyone,
-- the author included (roles-moderation invariant 3).
-- Every word of the query must appear somewhere in the row, in any order:
-- «доставка яндекс» and «яндекс доставка» are the same question. The words
-- arrive already wrapped in %…% (the service builds them).
--
-- total_rows rides along as a window count over the whole match set: the
-- caller pages with offset, and «яндекс» matches 500+ rows — without the
-- count the list would silently end at the page size, which is exactly the
-- bug this replaced (a row found by «яндекс доставка» was missing from
-- «яндекс», buried past row 20 by the confirmations order).
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
       origin::text as origin,
       count(*) over ()::bigint as total_rows
from point_of_sale
where mcc_code is not null -- a merchant row without an MCC answers nothing here
  and (status = 'approved' or (author_user_id = sqlc.arg(user_id)::uuid and status = 'pending'))
  -- Point-of-sale type filter: empty means «any». The same merchant is often
  -- a different MCC at the till than in its app, so «где я плачу» is a real
  -- question the base can answer.
  and (sqlc.arg(pos_type)::text = '' or coalesce(type::text, '') = sqlc.arg(pos_type)::text)
  -- The first word, per column and without coalesce, is the clause the two
  -- gin_trgm_ops indexes can serve: a BitmapOr over name/merchant_title
  -- instead of a 62k-row scan (11 ms vs 115 ms on the live base). It is
  -- implied by the ALL below, so it changes no result — only the plan.
  and (name ilike sqlc.arg(head)::text or merchant_title ilike sqlc.arg(head)::text)
  -- Every word, against the two fields joined. Matching the concatenation is
  -- the same as matching either column, because the words come from a
  -- whitespace split: a spaceless pattern cannot straddle the joining space.
  and name || ' ' || coalesce(merchant_title, '') ilike all (sqlc.arg(patterns)::text[])
-- id last: offset paging needs a total order, or a row can repeat or vanish
-- between pages when confirmations tie (they tie constantly — most rows sit
-- at 0).
order by confirmations desc nulls last, last_confirmed_at desc nulls last, name, id
limit sqlc.arg(max_rows) offset sqlc.arg(skip_rows);

-- name: ListMerchantsByCode :many
-- «Точки с кодом NNNN» (13a): the known points carrying a code,
-- confirmations first — the search's visibility rule applies.
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
where mcc_code = sqlc.arg(mcc_code)
  and (status = 'approved' or (author_user_id = sqlc.arg(user_id)::uuid and status = 'pending'))
order by confirmations desc nulls last, name, id
limit sqlc.arg(max_rows);

-- name: GetPointOfSale :one
-- The «О точке» card (8b): one row by id — approved, or the caller's own
-- pending submission (the same visibility rule the search applies).
select id,
       name,
       merchant_title,
       mcc_code,
       coalesce(type::text, '')::text as pos_type,
       address,
       confirmations,
       last_confirmed_at,
       status
from point_of_sale
where id = $1
  and (status = 'approved' or (author_user_id = sqlc.arg(user_id)::uuid and status = 'pending'))
  -- Point-of-sale type filter: empty means «any». The same merchant is often
  -- a different MCC at the till than in its app, so «где я плачу» is a real
  -- question the base can answer.
  and (sqlc.arg(pos_type)::text = '' or coalesce(type::text, '') = sqlc.arg(pos_type)::text);

-- name: FindSimilarPointsOfSale :many
-- The 5e duplicate net: same MCC and either name contains the other —
-- «Хлебник» must catch a new «Пекарня Хлебник» before a copy is created.
select id, name, merchant_title, mcc_code
from point_of_sale
where mcc_code = sqlc.arg(mcc_code)
  and (status = 'approved' or (author_user_id = sqlc.arg(user_id)::uuid and status = 'pending'))
  -- Point-of-sale type filter: empty means «any». The same merchant is often
  -- a different MCC at the till than in its app, so «где я плачу» is a real
  -- question the base can answer.
  and (sqlc.arg(pos_type)::text = '' or coalesce(type::text, '') = sqlc.arg(pos_type)::text)
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

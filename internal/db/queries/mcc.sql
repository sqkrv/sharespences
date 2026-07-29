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
select id,
       name,
       merchant_title,
       mcc_code,
       coalesce(type::text, '')::text as pos_type,
       address,
       confirmations,
       last_confirmed_at
from point_of_sale
where mcc_code is not null -- a merchant row without an MCC answers nothing here
  and (name ilike '%' || sqlc.arg(query)::text || '%'
    or merchant_title ilike '%' || sqlc.arg(query)::text || '%')
order by confirmations desc nulls last, last_confirmed_at desc nulls last, name
limit sqlc.arg(max_rows);

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

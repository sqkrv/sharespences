-- Admin sidecar queries (ADR-0008). Operator tooling spans module tables by
-- nature — the seam rule is relaxed here the same way it is for seed and
-- import-pos. Write queries exist only for what seed does NOT own: custom
-- bank_category rows (is_custom guard in SQL), the MCC dictionary and
-- category↔MCC links outside the seed CSVs (coverage guard in the service),
-- the mcc_change journal, and point_of_sale. `location` (PostGIS geometry)
-- is deliberately never selected or written — v1 has no map widget.

-- name: AdminCounts :one
select (select count(*) from bank)                                  as banks,
       (select count(*) from canonical_category)                    as canonical_categories,
       (select count(*) from bank_category)                         as bank_categories,
       (select count(*) from bank_category where is_custom)         as custom_bank_categories,
       (select count(*) from bank_category_alias)                   as aliases,
       (select count(*) from cashback_program)                      as programs,
       (select count(*) from program_tier)                          as tiers,
       (select count(*) from mcc)                                   as mcc_codes,
       (select count(*) from bank_category_mcc)                     as mcc_links,
       (select count(*) from mcc_change)                            as mcc_changes,
       (select count(*) from point_of_sale)                         as points_of_sale,
       (select count(*) from "user")                                as users,
       (select count(*) from attachment)                            as attachments;

-- name: AdminListAliases :many
select a.bank_id,
       b.name      as bank_name,
       a.raw_title,
       a.canonical_category_id,
       cc.slug     as canonical_slug,
       cc.title_ru as canonical_title
from bank_category_alias a
         join bank b on b.id = a.bank_id
         join canonical_category cc on cc.id = a.canonical_category_id
order by cc.slug, b.name, a.raw_title;

-- name: AdminListBankCategories :many
select bc.id,
       bc.bank_id,
       b.name      as bank_name,
       bc.title,
       bc.canonical_category_id,
       cc.slug     as canonical_slug,
       cc.title_ru as canonical_title,
       bc.kind,
       bc.emoji,
       bc.is_custom,
       bc.active,
       (select count(*) from bank_category_mcc m where m.bank_category_id = bc.id) as mcc_count
from bank_category bc
         join bank b on b.id = bc.bank_id
         left join canonical_category cc on cc.id = bc.canonical_category_id
where bc.bank_id = sqlc.narg(bank_id) or sqlc.narg(bank_id) is null
order by b.name, bc.kind, bc.title;

-- name: AdminGetBankCategoryWithBank :one
select bc.id, bc.bank_id, b.name as bank_name, bc.title, bc.is_custom
from bank_category bc
         join bank b on b.id = bc.bank_id
where bc.id = $1;

-- name: AdminUpdateCustomBankCategory :one
update bank_category
set title                 = $2,
    canonical_category_id = $3,
    kind                  = $4,
    emoji                 = $5,
    active                = $6
where id = $1
  and is_custom
returning *;

-- name: AdminDeleteCustomBankCategory :execrows
delete
from bank_category
where id = $1
  and is_custom;

-- name: AdminListMCC :many
select code,
       name,
       description,
       count(*) over ()::bigint as total
from mcc
where sqlc.arg(query)::text = ''
   or name ilike '%' || sqlc.arg(query)::text || '%'
   or lpad(code::text, 4, '0') like sqlc.arg(query)::text || '%'
order by code
limit sqlc.arg(max_rows) offset sqlc.arg(skip);

-- name: AdminCreateMCC :one
insert into mcc (code, name, description)
values ($1, $2, $3)
returning code, name, description;

-- name: AdminUpdateMCC :one
update mcc
set name        = $2,
    description = $3
where code = $1
returning code, name, description;

-- name: AdminDeleteMCC :execrows
delete
from mcc
where code = $1;

-- name: AdminListBankCategoryMCC :many
select bcm.mcc_code,
       m.name as mcc_name,
       bcm.note
from bank_category_mcc bcm
         join mcc m on m.code = bcm.mcc_code
where bcm.bank_category_id = $1
order by bcm.mcc_code;

-- name: AdminUpsertBankCategoryMCC :exec
insert into bank_category_mcc (bank_category_id, mcc_code, note)
values ($1, $2, $3)
on conflict (bank_category_id, mcc_code) do update set note = excluded.note;

-- name: AdminDeleteBankCategoryMCC :execrows
delete
from bank_category_mcc
where bank_category_id = $1
  and mcc_code = $2;

-- name: AdminListMCCChanges :many
select mc.id,
       mc.bank_id,
       b.name as bank_name,
       mc.bank_category_id,
       mc.category_title,
       mc.mcc_code,
       mc.action,
       mc.noted_at,
       mc.source,
       mc.note,
       count(*) over ()::bigint as total
from mcc_change mc
         join bank b on b.id = mc.bank_id
order by mc.noted_at desc, mc.id desc
limit sqlc.arg(max_rows) offset sqlc.arg(skip);

-- name: AdminCreateMCCChange :one
insert into mcc_change (bank_id, bank_category_id, category_title, mcc_code, action, source, note)
values ($1, $2, $3, $4, $5, $6, $7)
returning id;

-- name: AdminDeleteMCCChange :execrows
delete
from mcc_change
where id = $1;

-- name: AdminSearchPOS :many
select id,
       name,
       merchant_title,
       mcc_code,
       type,
       address,
       confirmations,
       created_at,
       last_confirmed_at,
       count(*) over ()::bigint as total
from point_of_sale
where sqlc.arg(query)::text = ''
   or name ilike '%' || sqlc.arg(query)::text || '%'
   or merchant_title ilike '%' || sqlc.arg(query)::text || '%'
order by confirmations desc nulls last, name, id
limit sqlc.arg(max_rows) offset sqlc.arg(skip);

-- name: AdminCreatePOS :one
insert into point_of_sale (name, merchant_title, mcc_code, type, address)
values ($1, $2, $3, $4, $5)
returning id;

-- name: AdminUpdatePOS :one
update point_of_sale
set name              = $2,
    merchant_title    = $3,
    mcc_code          = $4,
    type              = $5,
    address           = $6
where id = $1
returning id;

-- name: AdminDeletePOS :execrows
delete
from point_of_sale
where id = $1;

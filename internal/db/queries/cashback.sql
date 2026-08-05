-- Cashback module queries. Seam note: joins to bank / bank_client are the
-- module's read-only reference reads (decided at skeleton, see
-- 00003_cashback.sql header; re-keyed card→client in 00006); no other
-- module touches cashback tables.

-- name: ListPrograms :many
select cp.*, b.name as bank_name
from cashback_program cp
         join bank b on b.id = cp.bank_id
order by cp.id;

-- name: GetProgram :one
select *
from cashback_program
where id = $1;

-- name: UpdateProgram :one
update cashback_program
set name                = $2,
    period_type         = $3,
    selection_mode      = $4,
    currency_kind       = $5,
    points_label        = $6,
    selection_opens_day = $7,
    notes               = $8
where id = $1
returning *;

-- name: DeleteProgram :execrows
delete
from cashback_program
where id = $1;

-- name: ListTiersForProgram :many
select *
from program_tier
where program_id = $1
order by id;

-- name: GetTier :one
select *
from program_tier
where id = $1;

-- name: UpdateTier :one
update program_tier
set name                 = $2,
    is_paid_subscription = $3,
    cap_value            = $4,
    cap_scope            = $5,
    cap_per_category     = $6,
    max_categories       = $7,
    notes                = $8
where id = $1
returning *;

-- name: DeleteTier :execrows
delete
from program_tier
where id = $1;

-- name: ListCanonicalCategories :many
select *
from canonical_category
order by slug;

-- name: GetCanonicalCategoryBySlug :one
select *
from canonical_category
where slug = $1;

-- name: CreateCanonicalCategory :one
insert into canonical_category (slug, title_ru, emoji)
values ($1, $2, $3)
returning *;

-- name: ListBankCategories :many
select bc.*,
       cc.slug     as canonical_slug,
       cc.title_ru as canonical_title_ru,
       cc.emoji    as canonical_emoji
from bank_category bc
         left join canonical_category cc on cc.id = bc.canonical_category_id
where bc.bank_id = $1
  and bc.active
order by bc.kind, bc.title;

-- name: GetBankCategory :one
select *
from bank_category
where id = $1;

-- name: CreateBankCategory :one
insert into bank_category (bank_id, title, canonical_category_id, kind, emoji, is_custom)
values ($1, $2, $3, $4, $5, true)
returning *;

-- name: ListAliasesForBank :many
select *
from bank_category_alias
where bank_id = $1;

-- name: UpsertAlias :exec
insert into bank_category_alias (canonical_category_id, bank_id, raw_title)
values ($1, $2, $3)
on conflict (bank_id, raw_title) do update set canonical_category_id = excluded.canonical_category_id;

-- name: CreateOfferPeriod :one
insert into offer_period (bank_client_id, period_start, period_end)
values ($1, $2, $3)
returning *;

-- name: ListPeriodRangesForClient :many
select period_start, period_end
from offer_period
where bank_client_id = $1;

-- name: GetOfferPeriodForUser :one
select op.*, cl.bank_id, cl.label as holder_label, cl.program_tier_id, b.name as bank_name
from offer_period op
         join bank_client cl on cl.id = op.bank_client_id
         join bank b on b.id = cl.bank_id
where op.id = $1
  and cl.user_id = $2;

-- name: ListOfferPeriodsForUser :many
select op.*, cl.bank_id, cl.label as holder_label, b.name as bank_name
from offer_period op
         join bank_client cl on cl.id = op.bank_client_id
         join bank b on b.id = cl.bank_id
where cl.user_id = $1
order by op.period_start desc, op.id;

-- name: AttachToOfferPeriod :exec
insert into offer_period_attachment (offer_period_id, attachment_id)
values ($1, $2)
on conflict do nothing;

-- name: ListOfferPeriodAttachments :many
select a.*
from attachment a
         join offer_period_attachment opa on opa.attachment_id = a.id
where opa.offer_period_id = $1;

-- name: DetachFromOfferPeriod :execrows
delete
from offer_period_attachment
where offer_period_id = $1
  and attachment_id = $2;

-- DeleteAttachmentIfOrphan removes the attachment row once nothing links
-- to it (period or partner joins); the caller then removes the disk file.
-- name: DeleteAttachmentIfOrphan :execrows
delete
from attachment a
where a.id = $1
  and a.user_id = $2
  and not exists (select 1 from offer_period_attachment where attachment_id = $1)
  and not exists (select 1 from partner_offer_attachment where attachment_id = $1);

-- name: CreateCategoryOffer :one
insert into category_offer (offer_period_id, raw_title, canonical_category_id, percent, kind, notes, bank_category_id, cap_value)
values ($1, $2, $3, $4, $5, $6, $7, $8)
returning *;

-- name: UpdateCategoryOfferForUser :one
update category_offer co
set raw_title             = $3,
    canonical_category_id = $4,
    percent               = $5,
    kind                  = $6,
    notes                 = $7,
    bank_category_id      = $8,
    cap_value             = $9
from offer_period op,
     bank_client cl
where co.id = $1
  and op.id = co.offer_period_id
  and cl.id = op.bank_client_id
  and cl.user_id = $2
returning co.*;

-- name: DeleteSelectionByOffer :exec
delete
from selection
where category_offer_id = $1;

-- name: DeleteCategoryOffer :exec
delete
from category_offer
where id = $1;

-- name: SetOfferPeriodMaxOverride :one
update offer_period op
set max_categories_override = $3
from bank_client cl
where op.id = $1
  and cl.id = op.bank_client_id
  and cl.user_id = $2
returning op.*;

-- name: ListOfferIDsForPeriod :many
select id
from category_offer
where offer_period_id = $1;

-- name: DeleteOfferPeriodAttachments :exec
delete
from offer_period_attachment
where offer_period_id = $1;

-- name: DeleteOfferPeriod :exec
delete
from offer_period
where id = $1;

-- name: ListOffersForPeriod :many
select co.*, s.id as selection_id, s.selected_at
from category_offer co
         left join selection s on s.category_offer_id = co.id
where co.offer_period_id = $1
order by co.id;

-- name: GetOfferWithContextForUser :one
select co.*,
       op.bank_client_id,
       op.period_start,
       op.period_end,
       op.max_categories_override,
       cl.bank_id,
       cl.program_tier_id,
       (s.id is not null)::bool as already_selected
from category_offer co
         join offer_period op on op.id = co.offer_period_id
         join bank_client cl on cl.id = op.bank_client_id
         left join selection s on s.category_offer_id = co.id
where co.id = $1
  and cl.user_id = $2;

-- name: CountRegularSelectionsInPeriod :one
select count(*)
from selection s
         join category_offer co on co.id = s.category_offer_id
where co.offer_period_id = $1
  and co.kind = 'regular';

-- name: CreateSelection :one
insert into selection (category_offer_id, selected_at)
values ($1, $2)
returning *;

-- name: DeleteSelectionForUser :execrows
delete
from selection s
    using category_offer co, offer_period op, bank_client cl
where s.id = $1
  and co.id = s.category_offer_id
  and op.id = co.offer_period_id
  and cl.id = op.bank_client_id
  and cl.user_id = $2;

-- ListUserOffers is the helper/lookup workhorse: every menu row of the
-- user's bank clients with its period, client, tier-cap and program-currency
-- context, plus whether it is selected. Filtering (overlap, currency, kind,
-- date) is domain logic in Go — data volume is personal-app sized.
-- name: ListUserOffers :many
select co.id                     as category_offer_id,
       co.raw_title,
       co.canonical_category_id,
       co.percent,
       co.kind,
       co.cap_value              as offer_cap_value,
       op.id                     as offer_period_id,
       op.bank_client_id,
       op.period_start,
       op.period_end,
       op.max_categories_override,
       cl.label                  as holder_label,
       b.name                    as bank_name,
       pt.cap_value,
       pt.cap_scope              as tier_cap_scope,
       pt.cap_per_category,
       pt.max_categories,
       cp.currency_kind          as program_currency_kind,
       cp.points_label,
       -- Program-level policy reached via the BANK, not the tier: a client
       -- without a tier still falls under the bank's program (S3b verdicts
       -- must not degrade to 'unknown' just because no tier is set).
       -- coalesce: banks without a seeded program (Сбербанк/Т-Банк) → unknown.
       coalesce((select cpb.mid_period_add
                 from cashback_program cpb
                 where cpb.bank_id = cl.bank_id
                 limit 1), 'unknown')::cashback_mid_period_add as mid_period_add,
       coalesce((select cpb.activation
                 from cashback_program cpb
                 where cpb.bank_id = cl.bank_id
                 limit 1), 'unknown')::cashback_activation     as activation,
       (s.id is not null)::bool  as selected,
       s.selected_at
from category_offer co
         join offer_period op on op.id = co.offer_period_id
         join bank_client cl on cl.id = op.bank_client_id
         join bank b on b.id = cl.bank_id
         left join selection s on s.category_offer_id = co.id
         left join program_tier pt on pt.id = cl.program_tier_id
         left join cashback_program cp on cp.id = pt.program_id
where cl.user_id = $1;

-- ListOffersForClients is ListUserOffers keyed by explicit client ids
-- instead of the owner — the friends-sharing read path
-- (docs/specs/friends-sharing.md). The row shape is kept identical on
-- purpose: entryOf and the period filters reuse verbatim. The caller is
-- responsible for passing only GRANTED client ids (the friends module
-- resolves them); this module stays the only reader of cashback tables.
-- name: ListOffersForClients :many
select co.id                     as category_offer_id,
       co.raw_title,
       co.canonical_category_id,
       co.percent,
       co.kind,
       co.cap_value              as offer_cap_value,
       op.id                     as offer_period_id,
       op.bank_client_id,
       op.period_start,
       op.period_end,
       op.max_categories_override,
       cl.label                  as holder_label,
       b.name                    as bank_name,
       pt.cap_value,
       pt.cap_scope              as tier_cap_scope,
       pt.cap_per_category,
       pt.max_categories,
       cp.currency_kind          as program_currency_kind,
       cp.points_label,
       coalesce((select cpb.mid_period_add
                 from cashback_program cpb
                 where cpb.bank_id = cl.bank_id
                 limit 1), 'unknown')::cashback_mid_period_add as mid_period_add,
       coalesce((select cpb.activation
                 from cashback_program cpb
                 where cpb.bank_id = cl.bank_id
                 limit 1), 'unknown')::cashback_activation     as activation,
       (s.id is not null)::bool  as selected,
       s.selected_at
from category_offer co
         join offer_period op on op.id = co.offer_period_id
         join bank_client cl on cl.id = op.bank_client_id
         join bank b on b.id = cl.bank_id
         left join selection s on s.category_offer_id = co.id
         left join program_tier pt on pt.id = cl.program_tier_id
         left join cashback_program cp on cp.id = pt.program_id
where op.bank_client_id = any (sqlc.arg(client_ids)::bigint[]);

-- name: CreatePartnerOffer :one
insert into partner_offer (user_id, bank_id, bank_client_id, merchant_title, percent,
                           valid_from, valid_to, cap_value, notes)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
returning *;

-- name: ListPartnerOffersForUser :many
select po.*, b.name as bank_name
from partner_offer po
         join bank b on b.id = po.bank_id
where po.user_id = $1
order by po.id;

-- name: DeletePartnerOfferForUser :execrows
delete
from partner_offer
where id = $1
  and user_id = $2;

-- name: AttachToPartnerOffer :exec
insert into partner_offer_attachment (partner_offer_id, attachment_id)
values ($1, $2)
on conflict do nothing;

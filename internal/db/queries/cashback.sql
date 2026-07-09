-- Cashback module queries. Seam note: joins to bank / bank_card are the
-- module's read-only reference reads (decided at skeleton, see
-- 00003_cashback.sql header); no other module touches cashback tables.

-- name: CreateProgram :one
insert into cashback_program (bank_id, name, period_type, selection_mode, currency_kind,
                              points_label, selection_opens_day, notes)
values ($1, $2, $3, $4, $5, $6, $7, $8)
returning *;

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

-- name: CreateTier :one
insert into program_tier (program_id, name, is_paid_subscription, cap_value, cap_scope,
                          cap_per_category, max_categories, base_percent, notes)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
returning *;

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
    base_percent         = $8,
    notes                = $9
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
insert into canonical_category (slug, title_ru)
values ($1, $2)
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
insert into offer_period (card_id, period_start, period_end)
values ($1, $2, $3)
returning *;

-- name: ListPeriodRangesForCard :many
select period_start, period_end
from offer_period
where card_id = $1;

-- name: GetOfferPeriodForUser :one
select op.*, bc.bank_id, bc.last_4_digits, bc.program_tier_id, b.name as bank_name
from offer_period op
         join bank_card bc on bc.id = op.card_id
         join bank b on b.id = bc.bank_id
where op.id = $1
  and bc.user_id = $2;

-- name: ListOfferPeriodsForUser :many
select op.*, bc.bank_id, bc.last_4_digits, b.name as bank_name
from offer_period op
         join bank_card bc on bc.id = op.card_id
         join bank b on b.id = bc.bank_id
where bc.user_id = $1
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
insert into category_offer (offer_period_id, raw_title, canonical_category_id, percent, kind, notes)
values ($1, $2, $3, $4, $5, $6)
returning *;

-- name: UpdateCategoryOfferForUser :one
update category_offer co
set raw_title             = $3,
    canonical_category_id = $4,
    percent               = $5,
    kind                  = $6,
    notes                 = $7
from offer_period op,
     bank_card bc
where co.id = $1
  and op.id = co.offer_period_id
  and bc.id = op.card_id
  and bc.user_id = $2
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
from bank_card bc
where op.id = $1
  and bc.id = op.card_id
  and bc.user_id = $2
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
       op.card_id,
       op.period_start,
       op.period_end,
       op.max_categories_override,
       bc.bank_id,
       bc.program_tier_id,
       (s.id is not null)::bool as already_selected
from category_offer co
         join offer_period op on op.id = co.offer_period_id
         join bank_card bc on bc.id = op.card_id
         left join selection s on s.category_offer_id = co.id
where co.id = $1
  and bc.user_id = $2;

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
    using category_offer co, offer_period op, bank_card bc
where s.id = $1
  and co.id = s.category_offer_id
  and op.id = co.offer_period_id
  and bc.id = op.card_id
  and bc.user_id = $2;

-- ListUserOffers is the helper/lookup workhorse: every menu row of the
-- user's cards with its period, card, tier-cap and program-currency context,
-- plus whether it is selected. Filtering (overlap, currency, kind, date) is
-- domain logic in Go — data volume is personal-app sized.
-- name: ListUserOffers :many
select co.id                     as category_offer_id,
       co.raw_title,
       co.canonical_category_id,
       co.percent,
       co.kind,
       op.id                     as offer_period_id,
       op.card_id,
       op.period_start,
       op.period_end,
       op.max_categories_override,
       bc.last_4_digits,
       b.name                    as bank_name,
       pt.cap_value,
       pt.cap_scope              as tier_cap_scope,
       pt.cap_per_category,
       pt.max_categories,
       cp.currency_kind          as program_currency_kind,
       cp.points_label,
       (s.id is not null)::bool  as selected,
       s.selected_at
from category_offer co
         join offer_period op on op.id = co.offer_period_id
         join bank_card bc on bc.id = op.card_id
         join bank b on b.id = bc.bank_id
         left join selection s on s.category_offer_id = co.id
         left join program_tier pt on pt.id = bc.program_tier_id
         left join cashback_program cp on cp.id = pt.program_id
where bc.user_id = $1;

-- name: CreatePartnerOffer :one
insert into partner_offer (user_id, bank_id, card_id, merchant_title, percent,
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

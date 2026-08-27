-- PostgreSQL does not index a foreign key column automatically, so every read
-- that filters or joins on one of these seq-scans the child table, and every
-- delete of a parent row scans the child to check the constraint.
--
-- These five are the ones on live query paths (audit 2026-08-06):
--
--   category_offer.offer_period_id  — ListOffersForPeriod, ListOfferIDsForPeriod,
--       ListUserOffers, ListOffersForClients. category_offer is the table that
--       grows fastest (one row per menu row, per bank client, per month), and
--       every period read walked all of it.
--   partner_offer.user_id           — ListPartnerOffersForUser.
--   partner_offer.bank_client_id    — the joins, and the FK check that runs on
--       DeleteBankClientForUser.
--   bank_card.bank_client_id        — ListCardsForUser, and the same FK check.
--   offer_period_attachment.attachment_id, partner_offer_attachment.attachment_id
--       — both primary keys lead with the parent id, so DeleteAttachmentIfOrphan
--       (which looks an attachment up in both) had no usable index either way.
--
-- Not included, and deliberately: bank_category.created_by and
-- bank_category_alias.user_id are already covered by the leading bank_id of
-- their unique indexes, and attachment.user_id is only ever read alongside the
-- primary key. The schema-only tables (transaction, subscription,
-- receipt_position, category) are left alone until they have queries.
--
-- Plain CREATE INDEX rather than CONCURRENTLY: these tables hold hundreds of
-- rows, and goose runs a migration in one transaction, which CONCURRENTLY
-- cannot join.

-- +goose Up
create index if not exists category_offer_period_idx on category_offer (offer_period_id);
create index if not exists partner_offer_user_idx on partner_offer (user_id);
create index if not exists partner_offer_client_idx on partner_offer (bank_client_id);
create index if not exists bank_card_client_idx on bank_card (bank_client_id);
create index if not exists offer_period_attachment_file_idx on offer_period_attachment (attachment_id);
create index if not exists partner_offer_attachment_file_idx on partner_offer_attachment (attachment_id);

-- +goose Down
drop index if exists partner_offer_attachment_file_idx;
drop index if exists offer_period_attachment_file_idx;
drop index if exists bank_card_client_idx;
drop index if exists partner_offer_client_idx;
drop index if exists partner_offer_user_idx;
drop index if exists category_offer_period_idx;

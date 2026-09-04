package cashback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/vision"
)

// ErrNotFound covers rows that don't exist or belong to another user —
// scoping never reveals which.
var ErrNotFound = errors.New("не найдено")

// ErrBankCategoryExists — the bank already has a catalog row with this title.
var ErrBankCategoryExists = errors.New("категория с таким названием уже есть у этого банка")

// ErrBankCategoryWrongBank — the referenced catalog row belongs to another
// bank than the offer period's.
var ErrBankCategoryWrongBank = errors.New("категория из каталога другого банка")

// Service wires the domain rules to storage. Reference reads of bank /
// bank_client are the seam decided at skeleton time (00003_cashback.sql,
// re-keyed card→client in 00006). RemoveAttachmentFile is injected at
// assembly (the attachment module owns the disk store); called after an
// orphaned attachment row is deleted.
type Service struct {
	Q                    *db.Queries
	RemoveAttachmentFile func(id uuid.UUID) error
	// ReadAttachmentFile opens an attachment's stored bytes — injected
	// like RemoveAttachmentFile so this module never imports the
	// attachment store (ADR-0002 seam). Callers must have user-scoped
	// the attachment row first; the disk accessor itself carries no auth.
	ReadAttachmentFile func(id uuid.UUID) (io.ReadCloser, error)
	// Vision is the screenshot recognizer's model backend; nil = the
	// feature is off and recognition endpoints answer 503.
	Vision vision.Backend
	// ListSharedWithMe resolves the viewer's friends and their granted
	// client ids — injected from the friends module at assembly (ADR-0002
	// seam, same idiom as the attachment funcs). Nil = the friends feature
	// is absent; friend views are empty, lookup stays personal.
	ListSharedWithMe func(ctx context.Context, viewerID uuid.UUID) ([]SharedFriend, error)
	// MCCMemberships resolves an MCC code to the bank catalog rows that hold
	// it (bank_category_mcc) — injected from the mcc module at assembly
	// (ADR-0002 seam: values cross, tables don't). Nil = MCC boards answer
	// empty.
	MCCMemberships func(ctx context.Context, userID uuid.UUID, code int16) ([]MembershipRow, error)

	recognitions recognitionStore
}

// MembershipRow is one bank catalog row holding an MCC — the value shape the
// mcc module hands across the seam.
type MembershipRow struct {
	BankCategoryID int64
	// BankID and CanonicalCategoryID let the board match a menu row that was
	// never linked to a catalog row — see LookupByMCC.
	BankID              int32
	CanonicalCategoryID *int64
}

// clientLabel names a bank client for display: «Альфа-Банк» for the account owner's
// own relationship, «Альфа-Банк · Мама» for a держатель.
func clientLabel(bankName string, label *string) string {
	if label == nil || *label == "" {
		return bankName
	}
	return fmt.Sprintf("%s · %s", bankName, *label)
}

func holderOf(h *string) string {
	if h == nil {
		return ""
	}
	return *h
}

// capNote renders the static cap reference the helper and warnings display,
// e.g. «лимит 1500₽/кат, всего 3000₽» (Озон), «лимит 7000₽» (Альфа-Смарт).
func capNote(offerCap, capValue, capPerCategory *decimal.Decimal, scope db.NullCashbackCapScope, currency db.NullCashbackCurrencyKind, pointsLabel *string) string {
	unit := "₽"
	if currency.Valid && currency.CashbackCurrencyKind == db.CashbackCurrencyKindPoints {
		unit = " баллов"
		if pointsLabel != nil {
			unit = " " + *pointsLabel
		}
	}
	// A per-offer cap (ВТБ «Кешбэк до N ₽» rows) wins over the tier cap.
	if offerCap != nil {
		return fmt.Sprintf("лимит %s%s", offerCap.String(), unit)
	}
	if !scope.Valid {
		return ""
	}
	switch scope.CashbackCapScope {
	case db.CashbackCapScopePerCategory:
		if capPerCategory == nil {
			return ""
		}
		return fmt.Sprintf("лимит %s%s/кат", capPerCategory.String(), unit)
	case db.CashbackCapScopeBoth:
		if capPerCategory == nil || capValue == nil {
			return ""
		}
		return fmt.Sprintf("лимит %s%s/кат, всего %s%s", capPerCategory.String(), unit, capValue.String(), unit)
	default: // total
		if capValue == nil {
			return ""
		}
		return fmt.Sprintf("лимит %s%s", capValue.String(), unit)
	}
}

func currencyOf(row db.ListUserOffersRow) CurrencyKind {
	if !row.ProgramCurrencyKind.Valid {
		return CurrencyUnknown
	}
	return CurrencyKind(row.ProgramCurrencyKind.CashbackCurrencyKind)
}

func rowRange(start, end time.Time) DateRange {
	return DateRange{Start: start, End: end}
}

// entryOf maps a ListUserOffers row into a lookup entry (shared by lookup,
// overview and the base-rate fallback).
func entryOf(o db.ListUserOffersRow) LookupEntry {
	var capScope CapScope
	if o.TierCapScope.Valid {
		capScope = CapScope(o.TierCapScope.CashbackCapScope)
	}
	var pointsLabel string
	if o.PointsLabel != nil {
		pointsLabel = *o.PointsLabel
	}
	emoji := ""
	if o.BankCategoryEmoji != nil {
		emoji = *o.BankCategoryEmoji
	} else if o.CanonicalEmoji != nil {
		emoji = *o.CanonicalEmoji
	}
	return LookupEntry{
		ClientID:       o.BankClientID,
		ClientLabel:    clientLabel(o.BankName, o.HolderLabel),
		HolderLabel:    holderOf(o.HolderLabel),
		BankName:       o.BankName,
		RawTitle:       o.RawTitle,
		Emoji:          emoji,
		Percent:        o.Percent,
		CurrencyKind:   currencyOf(o),
		Kind:           OfferKind(o.Kind),
		Period:         rowRange(o.PeriodStart, o.PeriodEnd),
		CapValue:       o.CapValue,
		CapPerCategory: o.CapPerCategory,
		CapScope:       capScope,
		OfferCapValue:  o.OfferCapValue,
		PointsLabel:    pointsLabel,
	}
}

// activeSelectionOf maps a selected ListUserOffers row into the domain view.
func activeSelectionOf(row db.ListUserOffersRow) ActiveSelection {
	return ActiveSelection{
		ClientID:            row.BankClientID,
		ClientLabel:         clientLabel(row.BankName, row.HolderLabel),
		HolderLabel:         holderOf(row.HolderLabel),
		BankName:            row.BankName,
		CanonicalCategoryID: row.CanonicalCategoryID,
		Period:              rowRange(row.PeriodStart, row.PeriodEnd),
		Kind:                OfferKind(row.Kind),
		Percent:             row.Percent,
		CurrencyKind:        currencyOf(row),
		CapNote:             capNote(row.OfferCapValue, row.CapValue, row.CapPerCategory, row.TierCapScope, row.ProgramCurrencyKind, row.PointsLabel),
	}
}

// AssertOwnsClient rejects a bank_client_id belonging to another account.
//
// Every other reference to a bank client reaches the service through a
// user-scoped lookup, but a partner offer takes it from the request body, where
// nothing upstream has scoped it. Unchecked, an offer files itself against a
// stranger's client: invisible to them (their own list is scoped by user_id)
// and enough to make DeleteBankClientForUser answer 409 forever, citing history
// they cannot see. nil is the ordinary «not tied to a client» case.
func (s *Service) AssertOwnsClient(ctx context.Context, userID uuid.UUID, clientID *int64) error {
	if clientID == nil {
		return nil
	}
	if _, err := s.Q.GetBankClientForUser(ctx, db.GetBankClientForUserParams{ID: *clientID, UserID: userID}); err != nil {
		return notFound(err)
	}
	return nil
}

// CreateOfferPeriod enforces invariant 4 in the service; the DB exclusion
// constraint backstops races.
func (s *Service) CreateOfferPeriod(ctx context.Context, userID uuid.UUID, clientID int64, start, end time.Time, attachmentIDs []uuid.UUID) (db.OfferPeriod, error) {
	if _, err := s.Q.GetBankClientForUser(ctx, db.GetBankClientForUserParams{ID: clientID, UserID: userID}); err != nil {
		return db.OfferPeriod{}, notFound(err)
	}
	ranges, err := s.Q.ListPeriodRangesForClient(ctx, clientID)
	if err != nil {
		return db.OfferPeriod{}, err
	}
	existing := make([]DateRange, len(ranges))
	for i, r := range ranges {
		existing[i] = rowRange(r.PeriodStart, r.PeriodEnd)
	}
	if err := ValidateNewPeriod(rowRange(start, end), existing); err != nil {
		return db.OfferPeriod{}, err
	}
	// Ownership of the screenshots is checked BEFORE the insert. The check
	// needs nothing from the new row, and doing it afterwards left a period
	// behind on every 404 — the retry then answered 409 for an overlap with
	// the row the failed attempt had just created.
	for _, aid := range attachmentIDs {
		if _, err := s.Q.GetAttachmentForUser(ctx, db.GetAttachmentForUserParams{ID: aid, UserID: &userID}); err != nil {
			return db.OfferPeriod{}, notFound(err)
		}
	}
	period, err := s.Q.CreateOfferPeriod(ctx, db.CreateOfferPeriodParams{BankClientID: clientID, PeriodStart: start, PeriodEnd: end})
	if err != nil {
		if isPgCode(err, "23P01") || isPgCode(err, "23505") {
			return db.OfferPeriod{}, ErrPeriodOverlap
		}
		return db.OfferPeriod{}, err
	}
	for _, aid := range attachmentIDs {
		if err := s.Q.AttachToOfferPeriod(ctx, db.AttachToOfferPeriodParams{OfferPeriodID: period.ID, AttachmentID: aid}); err != nil {
			return db.OfferPeriod{}, err
		}
	}
	return period, nil
}

// AttachScreenshot links an uploaded attachment to an existing period
// (2026-07-09: screenshots must be editable after creation, not only
// at «Новый период»).
func (s *Service) AttachScreenshot(ctx context.Context, userID uuid.UUID, periodID int64, attachmentID uuid.UUID) error {
	if _, err := s.Q.GetOfferPeriodForUser(ctx, db.GetOfferPeriodForUserParams{ID: periodID, UserID: userID}); err != nil {
		return notFound(err)
	}
	uid := userID
	if _, err := s.Q.GetAttachmentForUser(ctx, db.GetAttachmentForUserParams{ID: attachmentID, UserID: &uid}); err != nil {
		return notFound(err)
	}
	return s.Q.AttachToOfferPeriod(ctx, db.AttachToOfferPeriodParams{OfferPeriodID: periodID, AttachmentID: attachmentID})
}

// DetachScreenshot unlinks a screenshot from the period; when nothing else
// references the attachment, its row and disk file are removed too.
func (s *Service) DetachScreenshot(ctx context.Context, userID uuid.UUID, periodID int64, attachmentID uuid.UUID) error {
	if _, err := s.Q.GetOfferPeriodForUser(ctx, db.GetOfferPeriodForUserParams{ID: periodID, UserID: userID}); err != nil {
		return notFound(err)
	}
	n, err := s.Q.DetachFromOfferPeriod(ctx, db.DetachFromOfferPeriodParams{OfferPeriodID: periodID, AttachmentID: attachmentID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.reclaimAttachments(ctx, userID, attachmentID)
}

// reclaimAttachments drops the row and the disk file of every id nothing links
// to any more. Policy §7.4 promises uploaded files go with the record they were
// attached to, so every unlink path has to end here — an attachment left behind
// is reachable through its content URL forever with nothing left to delete it.
func (s *Service) reclaimAttachments(ctx context.Context, userID uuid.UUID, ids ...uuid.UUID) error {
	uid := userID
	for _, id := range ids {
		orphaned, err := s.Q.DeleteAttachmentIfOrphan(ctx, db.DeleteAttachmentIfOrphanParams{ID: id, UserID: &uid})
		if err != nil {
			return err
		}
		if orphaned > 0 && s.RemoveAttachmentFile != nil {
			// The row is gone; a stale file is a cleanup nit, not a failure.
			_ = s.RemoveAttachmentFile(id)
		}
	}
	return nil
}

// DetachPartnerScreenshot unlinks a screenshot from a partner offer, reclaiming
// it when nothing else references it — the mirror of DetachScreenshot.
func (s *Service) DetachPartnerScreenshot(ctx context.Context, userID uuid.UUID, offerID int64, attachmentID uuid.UUID) error {
	if _, err := s.Q.GetPartnerOfferForUser(ctx, db.GetPartnerOfferForUserParams{ID: offerID, UserID: userID}); err != nil {
		return notFound(err)
	}
	n, err := s.Q.DetachFromPartnerOffer(ctx, db.DetachFromPartnerOfferParams{
		PartnerOfferID: offerID, AttachmentID: attachmentID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.reclaimAttachments(ctx, userID, attachmentID)
}

// DeletePartnerOffer removes the offer with its screenshot links, reclaiming
// any attachment left with nothing pointing at it. The link rows have to go
// first: partner_offer_attachment carries a plain foreign key, so deleting the
// offer underneath it raises 23503 and surfaced as a 500.
func (s *Service) DeletePartnerOffer(ctx context.Context, userID uuid.UUID, offerID int64) error {
	if _, err := s.Q.GetPartnerOfferForUser(ctx, db.GetPartnerOfferForUserParams{ID: offerID, UserID: userID}); err != nil {
		return notFound(err)
	}
	atts, err := s.Q.ListPartnerOfferAttachments(ctx, offerID)
	if err != nil {
		return err
	}
	if err := s.Q.DeletePartnerOfferAttachments(ctx, offerID); err != nil {
		return err
	}
	n, err := s.Q.DeletePartnerOfferForUser(ctx, db.DeletePartnerOfferForUserParams{ID: offerID, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	ids := make([]uuid.UUID, len(atts))
	for i, a := range atts {
		ids[i] = a.ID
	}
	return s.reclaimAttachments(ctx, userID, ids...)
}

// SuggestAlias implements the S1 pre-suggestion for a raw menu title on the
// entry screen of one offer period.
func (s *Service) SuggestAlias(ctx context.Context, userID uuid.UUID, offerPeriodID int64, rawTitle string) (*db.CanonicalCategory, error) {
	period, err := s.Q.GetOfferPeriodForUser(ctx, db.GetOfferPeriodForUserParams{ID: offerPeriodID, UserID: userID})
	if err != nil {
		return nil, notFound(err)
	}
	aliases, err := s.Q.ListAliasesForBank(ctx, db.ListAliasesForBankParams{BankID: int32(period.BankID), UserID: userID})
	if err != nil {
		return nil, err
	}
	domainAliases := make([]Alias, len(aliases))
	for i, a := range aliases {
		domainAliases[i] = Alias{CanonicalCategoryID: a.CanonicalCategoryID, RawTitle: a.RawTitle}
	}
	id, ok := SuggestCanonical(rawTitle, domainAliases)
	if !ok {
		return nil, nil
	}
	cats, err := s.Q.ListCanonicalCategories(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range cats {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, nil
}

// ListBankCategories returns one bank's picker catalog (active rows with
// resolved canonical info): the seeded rows every account shares plus the
// caller's own custom ones.
func (s *Service) ListBankCategories(ctx context.Context, userID uuid.UUID, bankID int32) ([]db.ListBankCategoriesRow, error) {
	return s.Q.ListBankCategories(ctx, db.ListBankCategoriesParams{BankID: bankID, UserID: userID})
}

// CreateBankCategory adds a custom row to a bank's picker catalog — the
// escape hatch for a category the bank introduced before the seed learned
// it. Canonical mapping is optional (special/service rows stay
// canonical-less by design; unmapped rows keep the S3 warning badge).
//
// The row belongs to its author and nobody else sees it: the catalog is a
// shared namespace on an installation with open registration, so a typo or a
// joke title would otherwise reach every account holding that bank. A title
// the seed ships later coexists with it (00019).
func (s *Service) CreateBankCategory(ctx context.Context, userID uuid.UUID, bankID int32, title string, canonicalID *int64, kind OfferKind, emoji *string) (db.BankCategory, error) {
	// The unique constraint only rejects a second row of the caller's own, so
	// a title already in their catalog — theirs OR seeded — is rejected here.
	// Two concurrent creates can still slip past to the constraint below.
	n, err := s.Q.CountVisibleBankCategoriesWithTitle(ctx, db.CountVisibleBankCategoriesWithTitleParams{
		BankID: bankID,
		Title:  title,
		UserID: userID,
	})
	if err != nil {
		return db.BankCategory{}, err
	}
	if n > 0 {
		return db.BankCategory{}, ErrBankCategoryExists
	}
	bc, err := s.Q.CreateBankCategory(ctx, db.CreateBankCategoryParams{
		BankID:              bankID,
		Title:               title,
		CanonicalCategoryID: canonicalID,
		Kind:                db.CashbackOfferKind(kind),
		Emoji:               emoji,
		CreatedBy:           userID,
	})
	if err != nil {
		if isPgCode(err, "23505") {
			return db.BankCategory{}, ErrBankCategoryExists
		}
		return db.BankCategory{}, err
	}
	return bc, nil
}

// firstNonNil is the explicit-wins rule for optional mappings: what the
// caller sent, else what the catalog row supplies.
func firstNonNil[T any](explicit, fallback *T) *T {
	if explicit != nil {
		return explicit
	}
	return fallback
}

// resolveBankCategory validates that a referenced catalog row is visible to
// the caller and belongs to the given bank (a picker pick can't attach
// another bank's row, nor another account's private one — that reads as
// «not found», which is also what keeps the id from probing for existence),
// and returns the canonical mapping that row carries.
//
// A catalog pick reaches the API as bank_category_id ALONE — both the picker
// and the recognizer's draft do that — while every read path (lookup,
// overview) keys on the offer's own canonical_category_id. Without the
// inheritance below, a committed and selected row stays invisible to «Какой
// картой?» and to the overview's «Категории» cut, and nothing warns: the
// unmapped badge is suppressed for catalog rows precisely because the
// catalog is supposed to hold the mapping (report 2026-07-30). Rows that are
// canonical-less by design (Альфа-Тревел, канальные) return nil and stay so.
func (s *Service) resolveBankCategory(ctx context.Context, userID uuid.UUID, bankCategoryID *int64, bankID int32) (*int64, error) {
	if bankCategoryID == nil {
		return nil, nil
	}
	bc, err := s.Q.GetBankCategory(ctx, db.GetBankCategoryParams{ID: *bankCategoryID, UserID: userID})
	if err != nil {
		return nil, notFound(err)
	}
	if bc.BankID != bankID {
		return nil, ErrBankCategoryWrongBank
	}
	return bc.CanonicalCategoryID, nil
}

// CreateCategoryOffer records one menu row. A provided canonical mapping is
// remembered as a bank alias (S1: unknown titles create the mapping inline).
func (s *Service) CreateCategoryOffer(ctx context.Context, userID uuid.UUID, offerPeriodID int64, rawTitle string, canonicalID *int64, percent *decimal.Decimal, kind OfferKind, notes *string, bankCategoryID *int64, capValue *decimal.Decimal) (db.CategoryOffer, error) {
	period, err := s.Q.GetOfferPeriodForUser(ctx, db.GetOfferPeriodForUserParams{ID: offerPeriodID, UserID: userID})
	if err != nil {
		return db.CategoryOffer{}, notFound(err)
	}
	inherited, err := s.resolveBankCategory(ctx, userID, bankCategoryID, int32(period.BankID))
	if err != nil {
		return db.CategoryOffer{}, err
	}
	offer, err := s.Q.CreateCategoryOffer(ctx, db.CreateCategoryOfferParams{
		OfferPeriodID:       offerPeriodID,
		RawTitle:            rawTitle,
		CanonicalCategoryID: firstNonNil(canonicalID, inherited),
		Percent:             percent,
		Kind:                db.CashbackOfferKind(kind),
		Notes:               notes,
		BankCategoryID:      bankCategoryID,
		CapValue:            capValue,
	})
	if err != nil {
		return db.CategoryOffer{}, err
	}
	if canonicalID != nil {
		if err := s.Q.UpsertAlias(ctx, db.UpsertAliasParams{
			CanonicalCategoryID: *canonicalID,
			BankID:              int32(period.BankID),
			RawTitle:            rawTitle,
			UserID:              userID,
		}); err != nil {
			return db.CategoryOffer{}, err
		}
	}
	return offer, nil
}

// effectiveMax resolves invariant 1's limit: the period-level override wins
// over the tier default (2026-07-04: slot counts vary between
// periods); nil = no limit known, no slot check.
func (s *Service) effectiveMax(ctx context.Context, override *int32, tierID *int64) (*int32, error) {
	if override != nil {
		return override, nil
	}
	if tierID == nil {
		return nil, nil
	}
	tier, err := s.Q.GetTier(ctx, *tierID)
	if err != nil {
		return nil, err
	}
	return tier.MaxCategories, nil
}

// CreateSelection enforces invariants 1 and 2 (hard rejects) and records the
// dated selection event. Cross-client duplicates never block here — the
// entry screen surfaces them via HelperContext (invariant 3).
func (s *Service) CreateSelection(ctx context.Context, userID uuid.UUID, categoryOfferID int64, selectedAt time.Time, backfill bool) (db.Selection, error) {
	offer, err := s.Q.GetOfferWithContextForUser(ctx, db.GetOfferWithContextForUserParams{ID: categoryOfferID, UserID: userID})
	if err != nil {
		return db.Selection{}, notFound(err)
	}
	maxCategories, err := s.effectiveMax(ctx, offer.MaxCategoriesOverride, offer.ProgramTierID)
	if err != nil {
		return db.Selection{}, err
	}
	count, err := s.Q.CountRegularSelectionsInPeriod(ctx, offer.OfferPeriodID)
	if err != nil {
		return db.Selection{}, err
	}
	if err := ValidateSelection(SelectionCheck{
		Period:               rowRange(offer.PeriodStart, offer.PeriodEnd),
		SelectedAt:           selectedAt,
		OfferKind:            OfferKind(offer.Kind),
		AlreadySelected:      offer.AlreadySelected,
		MaxCategories:        maxCategories,
		RegularSelectedCount: int(count),
		BackfillOverride:     backfill,
	}); err != nil {
		return db.Selection{}, err
	}
	sel, err := s.Q.CreateSelection(ctx, db.CreateSelectionParams{CategoryOfferID: categoryOfferID, SelectedAt: selectedAt})
	if err != nil {
		if isPgCode(err, "23505") {
			return db.Selection{}, ErrAlreadySelected
		}
		return db.Selection{}, err
	}
	return sel, nil
}

// UpdateCategoryOffer replaces the mutable fields of a menu row (feedback
// 2026-07-04: entered rows must be correctable — a row mapped to a
// canonical category after the fact starts appearing in lookups). A newly
// set canonical mapping is remembered as a bank alias, like on create.
func (s *Service) UpdateCategoryOffer(ctx context.Context, userID uuid.UUID, offerID int64, rawTitle string, canonicalID *int64, percent *decimal.Decimal, kind OfferKind, notes *string, bankCategoryID *int64, capValue *decimal.Decimal) (db.CategoryOffer, error) {
	ctxRow, err := s.Q.GetOfferWithContextForUser(ctx, db.GetOfferWithContextForUserParams{ID: offerID, UserID: userID})
	if err != nil {
		return db.CategoryOffer{}, notFound(err)
	}
	inherited, err := s.resolveBankCategory(ctx, userID, bankCategoryID, int32(ctxRow.BankID))
	if err != nil {
		return db.CategoryOffer{}, err
	}
	offer, err := s.Q.UpdateCategoryOfferForUser(ctx, db.UpdateCategoryOfferForUserParams{
		ID:                  offerID,
		UserID:              userID,
		RawTitle:            rawTitle,
		CanonicalCategoryID: firstNonNil(canonicalID, inherited),
		Percent:             percent,
		Kind:                db.CashbackOfferKind(kind),
		Notes:               notes,
		BankCategoryID:      bankCategoryID,
		CapValue:            capValue,
	})
	if err != nil {
		return db.CategoryOffer{}, notFound(err)
	}
	if canonicalID != nil {
		if err := s.Q.UpsertAlias(ctx, db.UpsertAliasParams{
			CanonicalCategoryID: *canonicalID,
			BankID:              int32(ctxRow.BankID),
			RawTitle:            rawTitle,
			UserID:              userID,
		}); err != nil {
			return db.CategoryOffer{}, err
		}
	}
	return offer, nil
}

// DeleteCategoryOffer removes a menu row together with its selection.
func (s *Service) DeleteCategoryOffer(ctx context.Context, userID uuid.UUID, offerID int64) error {
	if _, err := s.Q.GetOfferWithContextForUser(ctx, db.GetOfferWithContextForUserParams{ID: offerID, UserID: userID}); err != nil {
		return notFound(err)
	}
	if err := s.Q.DeleteSelectionByOffer(ctx, offerID); err != nil {
		return err
	}
	return s.Q.DeleteCategoryOffer(ctx, offerID)
}

// SetPeriodMaxOverride sets (or clears, with nil) the period's slot count.
func (s *Service) SetPeriodMaxOverride(ctx context.Context, userID uuid.UUID, periodID int64, value *int32) (db.OfferPeriod, error) {
	p, err := s.Q.SetOfferPeriodMaxOverride(ctx, db.SetOfferPeriodMaxOverrideParams{
		ID: periodID, UserID: userID, MaxCategoriesOverride: value,
	})
	if err != nil {
		return db.OfferPeriod{}, notFound(err)
	}
	return p, nil
}

// DeleteOfferPeriod removes a period with everything under it — menu rows,
// their selections, attachment links, and any screenshot the unlink leaves
// with nothing pointing at it (policy §7.4).
func (s *Service) DeleteOfferPeriod(ctx context.Context, userID uuid.UUID, periodID int64) error {
	if _, err := s.Q.GetOfferPeriodForUser(ctx, db.GetOfferPeriodForUserParams{ID: periodID, UserID: userID}); err != nil {
		return notFound(err)
	}
	// Read the links before they are deleted — afterwards nothing can name them.
	atts, err := s.Q.ListOfferPeriodAttachments(ctx, periodID)
	if err != nil {
		return err
	}
	offerIDs, err := s.Q.ListOfferIDsForPeriod(ctx, periodID)
	if err != nil {
		return err
	}
	for _, oid := range offerIDs {
		if err := s.Q.DeleteSelectionByOffer(ctx, oid); err != nil {
			return err
		}
		if err := s.Q.DeleteCategoryOffer(ctx, oid); err != nil {
			return err
		}
	}
	if err := s.Q.DeleteOfferPeriodAttachments(ctx, periodID); err != nil {
		return err
	}
	if err := s.Q.DeleteOfferPeriod(ctx, periodID); err != nil {
		return err
	}
	ids := make([]uuid.UUID, len(atts))
	for i, a := range atts {
		ids[i] = a.ID
	}
	return s.reclaimAttachments(ctx, userID, ids...)
}

func (s *Service) DeleteSelection(ctx context.Context, userID uuid.UUID, selectionID int64) error {
	n, err := s.Q.DeleteSelectionForUser(ctx, db.DeleteSelectionForUserParams{ID: selectionID, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// HelperRow is the helper panel's view of one menu row (S1).
type HelperRow struct {
	Offer       db.ListOffersForPeriodRow
	Collisions  []Collision
	Comparisons []OfferView
}

// HelperContextResult answers GET /cashback/helper-context.
// MaxCategories is the EFFECTIVE limit (period override, else tier).
type HelperContextResult struct {
	Period        db.GetOfferPeriodForUserRow
	SlotsUsed     int
	MaxCategories *int32
	Override      *int32
	Rows          []HelperRow
}

// HelperContext builds the entry-screen panel: unfilled-slot tracking,
// cross-client duplicate warnings and same-currency comparisons per menu
// row. Comparisons pool = same canonical category rows on the user's OTHER
// bank clients with overlapping periods (so «Супермаркеты 5%» can be judged
// against the other clients' offers of the same category), same currency
// only.
func (s *Service) HelperContext(ctx context.Context, userID uuid.UUID, offerPeriodID int64) (HelperContextResult, error) {
	period, err := s.Q.GetOfferPeriodForUser(ctx, db.GetOfferPeriodForUserParams{ID: offerPeriodID, UserID: userID})
	if err != nil {
		return HelperContextResult{}, notFound(err)
	}
	res := HelperContextResult{Period: period, Override: period.MaxCategoriesOverride}
	res.MaxCategories, err = s.effectiveMax(ctx, period.MaxCategoriesOverride, period.ProgramTierID)
	if err != nil {
		return HelperContextResult{}, err
	}
	rows, err := s.Q.ListOffersForPeriod(ctx, offerPeriodID)
	if err != nil {
		return HelperContextResult{}, err
	}
	all, err := s.Q.ListUserOffers(ctx, userID)
	if err != nil {
		return HelperContextResult{}, err
	}

	var selectedElsewhere []ActiveSelection
	for _, o := range all {
		if o.Selected {
			selectedElsewhere = append(selectedElsewhere, activeSelectionOf(o))
		}
	}
	periodRange := rowRange(period.PeriodStart, period.PeriodEnd)
	thisCurrency := CurrencyUnknown
	for _, o := range all {
		if o.OfferPeriodID == offerPeriodID {
			thisCurrency = currencyOf(o)
			break
		}
	}

	for _, row := range rows {
		if row.Kind == db.CashbackOfferKindRegular && row.SelectionID != nil {
			res.SlotsUsed++
		}
		hr := HelperRow{Offer: row}
		candidate := CandidateSelection{
			ClientID:            period.BankClientID,
			CanonicalCategoryID: row.CanonicalCategoryID,
			Period:              periodRange,
			Kind:                OfferKind(row.Kind),
		}
		hr.Collisions = DetectCollisions(candidate, selectedElsewhere)

		if row.CanonicalCategoryID != nil {
			candidateView := OfferView{
				OfferID:      row.ID,
				RawTitle:     row.RawTitle,
				Percent:      row.Percent,
				Kind:         OfferKind(row.Kind),
				CurrencyKind: thisCurrency,
				ClientID:     period.BankClientID,
				BankName:     period.BankName,
				ClientLabel:  clientLabel(period.BankName, period.HolderLabel),
			}
			var pool []OfferView
			for _, o := range all {
				if o.BankClientID == period.BankClientID ||
					o.CanonicalCategoryID == nil ||
					*o.CanonicalCategoryID != *row.CanonicalCategoryID ||
					!periodRange.Overlaps(rowRange(o.PeriodStart, o.PeriodEnd)) {
					continue
				}
				pool = append(pool, OfferView{
					OfferID:      o.CategoryOfferID,
					RawTitle:     o.RawTitle,
					Percent:      o.Percent,
					Kind:         OfferKind(o.Kind),
					CurrencyKind: currencyOf(o),
					ClientID:     o.BankClientID,
					BankName:     o.BankName,
					ClientLabel:  clientLabel(o.BankName, o.HolderLabel),
				})
			}
			hr.Comparisons = ComparableOffers(candidateView, pool)
		}
		res.Rows = append(res.Rows, hr)
	}
	return res, nil
}

// OverviewCategoryGroup is one row of the «Категории» cut: the category and
// its best active card. «Best» = first by the domain ranking (rubles group
// before points, percent desc within a group) — deliberately NOT a numeric
// cross-currency comparison (invariant 5); rubles win by list position only.
// BankStackEntry is one logo in a feed row's overlap stack (9a): a bank
// where the category exists this month, in rank order. Friend marks let the
// client drop friends' banks when the toggle hides them.
type BankStackEntry struct {
	BankName string
	Friend   bool
}

// bankStackOf builds the stack: selected entries first (rank order), then
// still-available menu rows; one entry per bank. A bank seen as a friend's
// AND the viewer's own keeps its rank position but counts as own — the
// friends toggle must not hide the viewer's own presence.
func bankStackOf(ranked []LookupEntry, avail []AvailableEntry) []BankStackEntry {
	idx := make(map[string]int, len(ranked)+len(avail))
	var out []BankStackEntry
	add := func(bank string, friend bool) {
		if bank == "" {
			return
		}
		if i, ok := idx[bank]; ok {
			if !friend {
				out[i].Friend = false
			}
			return
		}
		idx[bank] = len(out)
		out = append(out, BankStackEntry{BankName: bank, Friend: friend})
	}
	for _, e := range ranked {
		add(e.BankName, e.FriendName != "")
	}
	for _, e := range avail {
		add(e.Entry.BankName, false)
	}
	return out
}

type OverviewCategoryGroup struct {
	CategoryID int64
	Slug       string
	TitleRu    string
	Emoji      string // canonical category icon for the list (2026-07-27)
	// Best is the viewer's own winner; nil when only a friend covers the
	// category. FriendBest is set only when a friend's card outranks every
	// own one or fills such a hole (redesign 2026-08-06) — hiding friends
	// falls the row back to Best instead of dropping it.
	Best       *LookupEntry
	FriendBest *LookupEntry
	// Available is the best S3b «можно выбрать» row, populated only while
	// the viewer has no own selection for the category — the feed's dashed
	// state («только то, что есть или реально доступно»); once something is
	// selected the fuller available list stays a lookup concern.
	Available *AvailableEntry
	// FriendAvailable is the same dashed state on a friend's shared card: a
	// menu row they have not picked and still have room for. Display only —
	// it carries no offer id, because picking is the owner's action. Kept
	// apart from Available for the reason Best/FriendBest are kept apart:
	// a row the viewer cannot act on must never displace one they can.
	FriendAvailable *AvailableEntry
	OthersCount     int // other OWN cards beyond Best; friends never counted
	// BankStack: every bank where the category exists this month, rank
	// order — the feed row's overlap logos (9a).
	BankStack []BankStackEntry
}

// OverviewSelectedRow is a selected menu row shown as a chip on a card.
type OverviewSelectedRow struct {
	OfferID  int64
	RawTitle string
	Kind     OfferKind
	Percent  *decimal.Decimal
	Emoji    string // canonical category icon (2e v3); "" for canonical-less rows
}

// OverviewClientCard is one plastic of the client, shown as a chip
// («··1234») — any of them pays with the client's shared selection.
type OverviewClientCard struct {
	CardID        int32
	Last4Digits   int32
	PaymentSystem string
}

// OverviewClient is one row of the «Карты» cut: a bank client (person ×
// bank) with its plastics. Period is nil when the client has no
// offer_period covering the date («нет периода», the design's dashed card
// with «Добавить»).
type OverviewClient struct {
	ClientID     int64
	BankID       int32
	BankName     string
	HolderLabel  *string
	Cards        []OverviewClientCard
	TierName     *string
	IsPaidTier   bool
	CapValue     *decimal.Decimal
	CapScope     CapScope
	CapPerCat    *decimal.Decimal
	CurrencyKind CurrencyKind
	PointsLabel  string
	// MidPeriodAdd/Activation are the two policy axes that actually govern
	// «can I still pick this right now?» (migration 00008). They replaced
	// SelectionMode here: atomic|incremental described how the picker submits,
	// which no screen and no rule ever needed.
	MidPeriodAdd  string
	Activation    string
	PeriodID      *int64
	PeriodStart   *time.Time
	PeriodEnd     *time.Time
	SlotsUsed     int
	MaxCategories *int32 // effective: period override, else tier
	Selected      []OverviewSelectedRow
	Specials      []OverviewSelectedRow
	// SelectionOpensDay is the program's ритуал date («выбор с 25-го»), shown
	// per bank on CB-09.b — the aggregate on OverviewResult answers «when is
	// the next one anywhere», which is a different question.
	SelectionOpensDay *int32
	// Pending is the period this client still has to fill, when its window is
	// already open (domain PendingMenu). Nil = nothing to do.
	Pending *DateRange
	// Partners are the client's партнёрки as gold chips (v2, 3c): every
	// status — the SPA shows alive ones inline and folds ended/expired into
	// a collapsed group, so past offers keep a home after CB-05 dissolves.
	// A bank-level offer (no client) hangs off the bank's first client.
	Partners []OverviewPartnerChip
}

// OverviewPartnerChip is one партнёрка on a bank client card.
type OverviewPartnerChip struct {
	ID            int64
	MerchantTitle string
	Percent       *decimal.Decimal
	CurrencyKind  CurrencyKind
	ValidTo       *time.Time
	Status        string
}

// OverviewBase is the «Остальное» row: the best base-rate card («За все
// покупки» granted rows plus regular rows mapped to all-purchases).
type OverviewBase struct {
	Emoji       string // all-purchases icon — keeps the list's icon column aligned
	Best        LookupEntry
	OthersCount int
	BankStack   []BankStackEntry // banks with a selected base row, rank order (9a)
}

// emojiOf unwraps a canonical category's optional UI icon (seeded from the
// knowledge taxonomy; empty means «no icon», the frontend falls back).
func emojiOf(c db.CanonicalCategory) string {
	if c.Emoji == nil {
		return ""
	}
	return *c.Emoji
}

// OverviewResult answers GET /cashback/overview: the design's two cuts of
// the same month (screens 01/02), plus the passive «selection opens» day.
// OverviewPartnerRow is one партнёрка in the feed: its rankable entry plus
// the lifecycle facts the row states («по 31.08», требует активации).
type OverviewPartnerRow struct {
	Entry   LookupEntry
	ValidTo *time.Time
	Status  string
}

type OverviewResult struct {
	Categories []OverviewCategoryGroup
	Base       *OverviewBase
	Clients    []OverviewClient
	// SingleBank is the «Только в одном банке» tail (redesign 2026-08-06):
	// selected canonical-less rows — a bank's own service categories that
	// cannot group across banks. Ranked for a stable order, shown collapsed.
	SingleBank []LookupEntry
	// Partners are the alive партнёрки active on the date, ranked by the
	// same key as category rows (currency group → percent desc) so the SPA
	// can interleave without ever putting points above rubles (invariant 5).
	// They live outside Categories: a merchant offer is its own row, named
	// by the merchant, alive even when no month menu is entered.
	Partners          []OverviewPartnerRow
	SelectionOpensDay *int32 // earliest across the user's clients' programs
}

// fallbackEntries picks the selected rows that answer «а если категория не
// выбрана нигде?»: ordinary regular rows mapped to canonical all-purchases
// («За все покупки» — a category like any other, 2026-07-09; it pays
// only when no other selected category matches, which is why it doubles as
// the «Остальное» display). exceptCat skips rows already listed as the
// looked-up category itself.
func fallbackEntries(offers []db.ListUserOffersRow, allPurposesID *int64, exceptCat *int64, build func(db.ListUserOffersRow) LookupEntry) []LookupEntry {
	var out []LookupEntry
	for _, o := range offers {
		if !o.Selected || allPurposesID == nil || o.CanonicalCategoryID == nil {
			continue
		}
		if *o.CanonicalCategoryID != *allPurposesID || OfferKind(o.Kind) != OfferRegular {
			continue
		}
		if exceptCat != nil && *o.CanonicalCategoryID == *exceptCat {
			continue
		}
		out = append(out, build(o))
	}
	return out
}

// Overview builds both cuts for the date. No spend model, no remaining-cap
// math — everything here is recorded selections plus configured tier data.
func (s *Service) Overview(ctx context.Context, userID uuid.UUID, onDate time.Time) (OverviewResult, error) {
	offers, err := s.Q.ListUserOffers(ctx, userID)
	if err != nil {
		return OverviewResult{}, err
	}
	clients, err := s.Q.ListBankClientsForUser(ctx, userID)
	if err != nil {
		return OverviewResult{}, err
	}
	// Periods come from the period list, NOT from the offers join — a
	// freshly created period has no menu rows yet and would otherwise be
	// invisible here while still blocking re-creation with a 409 overlap
	// (bug report 2026-07-22).
	periods, err := s.Q.ListOfferPeriodsForUser(ctx, userID)
	if err != nil {
		return OverviewResult{}, err
	}
	cards, err := s.Q.ListCardsForUser(ctx, userID)
	if err != nil {
		return OverviewResult{}, err
	}
	cardsByClient := make(map[int64][]OverviewClientCard, len(clients))
	for _, c := range cards {
		cardsByClient[c.BankClientID] = append(cardsByClient[c.BankClientID], OverviewClientCard{
			CardID: c.ID, Last4Digits: c.Last4Digits, PaymentSystem: string(c.PaymentSystem),
		})
	}
	cats, err := s.Q.ListCanonicalCategories(ctx)
	if err != nil {
		return OverviewResult{}, err
	}
	catByID := make(map[int64]db.CanonicalCategory, len(cats))
	for _, c := range cats {
		catByID[c.ID] = c
	}

	var res OverviewResult

	var allPurposesID *int64
	for _, c := range cats {
		if c.Slug == "all-purchases" {
			id := c.ID
			allPurposesID = &id
		}
	}

	// --- «Категории»: group active selections by canonical category.
	// all-purchases rows are routed to the «Остальное» base row instead. ---
	byCat := make(map[int64][]LookupEntry)
	for _, o := range offers {
		if !o.Selected || o.CanonicalCategoryID == nil {
			continue
		}
		if allPurposesID != nil && *o.CanonicalCategoryID == *allPurposesID {
			continue
		}
		byCat[*o.CanonicalCategoryID] = append(byCat[*o.CanonicalCategoryID], entryOf(o))
	}
	// Friends enter the feed rankings (redesign 2026-08-06) — surfaced only
	// when they win or fill a hole (SplitFeedWinner). Their all-purchases
	// rows stay out: base rates are personal, «Остальное» is own-only.
	friendCats, err := s.friendEntriesByCategory(ctx, userID)
	if err != nil {
		return OverviewResult{}, err
	}
	for catID, entries := range friendCats {
		if allPurposesID != nil && catID == *allPurposesID {
			continue
		}
		byCat[catID] = append(byCat[catID], entries...)
	}
	// S3b «можно выбрать» per category, for the feed's dashed rows: menu
	// rows sitting unselected in an active period, verdict-first ranked —
	// the same construction Lookup does for one category.
	regCount := make(map[int64]int) // offer_period_id → selected regular rows
	for _, o := range offers {
		if o.Selected && OfferKind(o.Kind) == OfferRegular {
			regCount[o.OfferPeriodID]++
		}
	}
	availByCat := make(map[int64][]AvailableEntry)
	for _, o := range offers {
		if o.Selected || o.CanonicalCategoryID == nil {
			continue
		}
		if allPurposesID != nil && *o.CanonicalCategoryID == *allPurposesID {
			continue
		}
		kind := OfferKind(o.Kind)
		if kind == OfferSpecial || !rowRange(o.PeriodStart, o.PeriodEnd).Contains(onDate) {
			continue
		}
		max := o.MaxCategoriesOverride
		if max == nil {
			max = o.MaxCategories
		}
		verdict := AssessAvailability(AvailabilityCheck{
			Kind:                 kind,
			Policy:               MidPeriodAddPolicy(o.MidPeriodAdd),
			HasRegularSelection:  regCount[o.OfferPeriodID] > 0,
			MaxCategories:        max,
			RegularSelectedCount: regCount[o.OfferPeriodID],
		})
		if !verdict.Pickable() {
			continue
		}
		availByCat[*o.CanonicalCategoryID] = append(availByCat[*o.CanonicalCategoryID], AvailableEntry{
			Entry:      entryOf(o),
			OfferID:    o.CategoryOfferID,
			Verdict:    verdict,
			Activation: ActivationKind(o.Activation),
		})
	}
	friendAvailByCat, err := s.friendAvailableByCategory(ctx, userID, onDate)
	if err != nil {
		return OverviewResult{}, err
	}
	// «За все покупки» belongs to the Base row, not to the category list —
	// the viewer's own available rows drop it above for the same reason, and
	// a friend's copy of it would reintroduce the row the feed avoids.
	if allPurposesID != nil {
		delete(friendAvailByCat, *allPurposesID)
	}
	seenCats := make(map[int64]bool, len(byCat))
	for catID, entries := range byCat {
		seenCats[catID] = true
		ranked := RankActiveSelections(onDate, entries)
		cat, ok := catByID[catID]
		if !ok {
			continue
		}
		// All three kinds rank (invariant 6 amendment, 2026-07-27): the
		// best card may be a барабан or a спец — the frontend marks it.
		own, friendBest := SplitFeedWinner(ranked.Ranked)
		g := OverviewCategoryGroup{
			CategoryID: catID,
			Slug:       cat.Slug,
			TitleRu:    cat.TitleRu,
			Emoji:      emojiOf(cat),
			Best:       own,
			FriendBest: friendBest,
			BankStack:  bankStackOf(ranked.Ranked, availByCat[catID]),
		}
		// A friend's unpicked row is not gated on the viewer's own state, and
		// that is the one place it parts company with Available. Available is
		// a call to action and rightly disappears once the viewer has picked;
		// this is inventory — CB-01 exists to show what cashback is out there,
		// while choosing a card to pay with happens through search
		// (merchant/category/MCC), not here.
		if fa := RankAvailable(friendAvailByCat[catID]); len(fa) > 0 {
			g.FriendAvailable = &fa[0]
		}
		if own == nil {
			// No own selection — the dashed state stays reachable even when
			// a friend fills the hole (it is what the row falls back to).
			if avail := RankAvailable(availByCat[catID]); len(avail) > 0 {
				g.Available = &avail[0]
			}
			if friendBest == nil && g.Available == nil && g.FriendAvailable == nil {
				continue // nothing active
			}
		}
		ownCount := 0
		for _, e := range ranked.Ranked {
			if e.FriendName == "" {
				ownCount++
			}
		}
		if own != nil {
			ownCount--
		}
		g.OthersCount = ownCount
		res.Categories = append(res.Categories, g)
	}
	// Categories with nothing selected anywhere but something offered: the
	// feed's dashed «можно выбрать» rows (redesign, ТУР 1 «решено»: only
	// what exists or is genuinely available — but available IS a row).
	for catID, avails := range availByCat {
		if seenCats[catID] {
			continue
		}
		seenCats[catID] = true
		cat, ok := catByID[catID]
		if !ok {
			continue
		}
		ranked := RankAvailable(avails)
		g := OverviewCategoryGroup{
			CategoryID: catID,
			Slug:       cat.Slug,
			TitleRu:    cat.TitleRu,
			Emoji:      emojiOf(cat),
			Available:  &ranked[0],
			BankStack:  bankStackOf(nil, ranked),
		}
		if fa := RankAvailable(friendAvailByCat[catID]); len(fa) > 0 {
			g.FriendAvailable = &fa[0]
		}
		res.Categories = append(res.Categories, g)
	}
	// Categories only a friend offers: nothing of the viewer's own is in play
	// this month, but a friend has a row they have not picked and can. No
	// BankStack — the logos say «where you have this category», and a
	// friend's bank is not the viewer's.
	for catID, favails := range friendAvailByCat {
		if seenCats[catID] {
			continue
		}
		seenCats[catID] = true
		cat, ok := catByID[catID]
		if !ok {
			continue
		}
		ranked := RankAvailable(favails)
		res.Categories = append(res.Categories, OverviewCategoryGroup{
			CategoryID:      catID,
			Slug:            cat.Slug,
			TitleRu:         cat.TitleRu,
			Emoji:           emojiOf(cat),
			FriendAvailable: &ranked[0],
		})
	}
	// Sort: rub before points; then percent desc; then title. The key is the
	// row's displayed winner — the friend when one is surfaced, the dashed
	// available row when nothing is selected at all.
	winnerOf := func(g OverviewCategoryGroup) *LookupEntry {
		if g.FriendBest != nil {
			return g.FriendBest
		}
		if g.Best != nil {
			return g.Best
		}
		if g.Available != nil {
			return &g.Available.Entry
		}
		if g.FriendAvailable != nil {
			return &g.FriendAvailable.Entry
		}
		return &LookupEntry{}
	}
	sort.SliceStable(res.Categories, func(i, j int) bool {
		a, b := winnerOf(res.Categories[i]), winnerOf(res.Categories[j])
		ca := map[CurrencyKind]int{CurrencyRub: 0, CurrencyPoints: 1}[a.CurrencyKind]
		cb := map[CurrencyKind]int{CurrencyRub: 0, CurrencyPoints: 1}[b.CurrencyKind]
		if ca != cb {
			return ca < cb
		}
		if c := cmpPercentDesc(a.Percent, b.Percent); c != 0 {
			return c < 0
		}
		return res.Categories[i].TitleRu < res.Categories[j].TitleRu
	})

	// «Только в одном банке»: selected canonical-less rows — bank-own
	// service categories («Альфа-Тревел», «ЖКУ») that no canonical groups.
	// Unmapped rows used to be invisible here; the redesign shows them as a
	// collapsed tail instead of dropping them from the feed.
	var singles []LookupEntry
	for _, o := range offers {
		if o.Selected && o.CanonicalCategoryID == nil {
			singles = append(singles, entryOf(o))
		}
	}
	res.SingleBank = RankActiveSelections(onDate, singles).Ranked

	// Партнёрки (v2): alive offers active on the date become their own
	// merchant-named feed rows, ranked by the category-row key. They ignore
	// periods entirely — the «жив без меню месяца» promise holds by shape.
	partners, err := s.Q.ListPartnerOffersForUser(ctx, userID)
	if err != nil {
		return OverviewResult{}, err
	}
	partnerByID := make(map[int64]db.ListPartnerOffersForUserRow, len(partners))
	var partnerEntries []LookupEntry
	for _, p := range partners {
		if !alivePartner(p) {
			continue
		}
		partnerByID[p.ID] = p
		partnerEntries = append(partnerEntries, partnerEntryOf(p))
	}
	for _, e := range RankActiveSelections(onDate, partnerEntries).Ranked {
		p := partnerByID[e.PartnerID]
		res.Partners = append(res.Partners, OverviewPartnerRow{
			Entry:   e,
			ValidTo: p.ValidTo,
			Status:  PartnerStatus(onDate, p.ValidFrom, p.ValidTo, p.EndedAt),
		})
	}

	// «Остальное»: best selected «За все покупки» across clients.
	fb := RankActiveSelections(onDate, fallbackEntries(offers, allPurposesID, nil, entryOf))
	if len(fb.Ranked) > 0 {
		base := OverviewBase{Best: fb.Ranked[0], OthersCount: len(fb.Ranked) - 1, BankStack: bankStackOf(fb.Ranked, nil)}
		if allPurposesID != nil {
			base.Emoji = emojiOf(catByID[*allPurposesID])
		}
		res.Base = &base
	}

	// --- «Карты»: every bank client with its plastics, and the client's
	// active period when one exists (all its cards share it). ---
	// A bank-level партнёрка (no client) hangs off the bank's first client,
	// so it renders exactly once.
	firstClientOfBank := make(map[int32]int64, len(clients))
	for _, c := range clients {
		if _, ok := firstClientOfBank[c.BankID]; !ok {
			firstClientOfBank[c.BankID] = c.ID
		}
	}
	// Programs by bank: the ритуал date belongs to the bank's program, not to
	// the client's tier, so a client with no tier set still gets its «выбор с
	// 25-го» and its unfilled-menu mark (CB-09.b).
	programs, err := s.Q.ListPrograms(ctx)
	if err != nil {
		return OverviewResult{}, err
	}
	programByBank := make(map[int32]db.ListProgramsRow, len(programs))
	for _, p := range programs {
		if _, taken := programByBank[p.BankID]; !taken {
			programByBank[p.BankID] = p
		}
	}
	// Every recorded period with its menu-row count — PendingMenu needs both,
	// and both are already loaded for the month cut.
	fillByClient := make(map[int64][]PeriodFill)
	for _, p := range periods {
		fill := PeriodFill{Range: rowRange(p.PeriodStart, p.PeriodEnd)}
		for _, o := range offers {
			if o.OfferPeriodID == p.ID {
				fill.Offers++
			}
		}
		fillByClient[p.BankClientID] = append(fillByClient[p.BankClientID], fill)
	}
	for _, client := range clients {
		oc := OverviewClient{
			ClientID:     client.ID,
			BankID:       client.BankID,
			BankName:     client.BankName,
			HolderLabel:  client.Label,
			Cards:        cardsByClient[client.ID],
			CurrencyKind: CurrencyUnknown,
		}
		if client.ProgramTierID != nil {
			tier, err := s.Q.GetTier(ctx, *client.ProgramTierID)
			if err != nil {
				return OverviewResult{}, err
			}
			oc.TierName = &tier.Name
			oc.IsPaidTier = tier.IsPaidSubscription
			oc.CapValue = tier.CapValue
			oc.CapScope = CapScope(tier.CapScope)
			oc.CapPerCat = tier.CapPerCategory
			oc.MaxCategories = tier.MaxCategories
			program, err := s.Q.GetProgram(ctx, tier.ProgramID)
			if err != nil {
				return OverviewResult{}, err
			}
			oc.CurrencyKind = CurrencyKind(program.CurrencyKind)
			if program.PointsLabel != nil {
				oc.PointsLabel = *program.PointsLabel
			}
			oc.MidPeriodAdd = string(program.MidPeriodAdd)
			oc.Activation = string(program.Activation)
		}
		if program, ok := programByBank[client.BankID]; ok {
			oc.SelectionOpensDay = program.SelectionOpensDay
			if program.SelectionOpensDay != nil &&
				(res.SelectionOpensDay == nil || *program.SelectionOpensDay < *res.SelectionOpensDay) {
				res.SelectionOpensDay = program.SelectionOpensDay
			}
			// The mark is about today, not about the month being viewed: a
			// user browsing July must still see that September is open and
			// empty (owner 2026-08-27).
			oc.Pending = PendingMenu(time.Now(), PeriodType(program.PeriodType), program.SelectionOpensDay, fillByClient[client.ID])
		}
		// Invariant 4 guarantees at most one period per client covers a date.
		for _, p := range periods {
			if p.BankClientID != client.ID || !rowRange(p.PeriodStart, p.PeriodEnd).Contains(onDate) {
				continue
			}
			id, start, end := p.ID, p.PeriodStart, p.PeriodEnd
			oc.PeriodID, oc.PeriodStart, oc.PeriodEnd = &id, &start, &end
			if p.MaxCategoriesOverride != nil {
				oc.MaxCategories = p.MaxCategoriesOverride
			}
			break
		}
		for _, o := range offers {
			if o.BankClientID != client.ID || !rowRange(o.PeriodStart, o.PeriodEnd).Contains(onDate) {
				continue
			}
			if !o.Selected {
				continue
			}
			row := OverviewSelectedRow{OfferID: o.CategoryOfferID, RawTitle: o.RawTitle, Kind: OfferKind(o.Kind), Percent: o.Percent}
			if o.CanonicalCategoryID != nil {
				row.Emoji = emojiOf(catByID[*o.CanonicalCategoryID])
			}
			// Only regular fills a slot and shows as a chosen (mint) chip;
			// granted super/special go to the gold bonus chips (no slot).
			if OfferKind(o.Kind) != OfferRegular {
				oc.Specials = append(oc.Specials, row)
			} else {
				oc.SlotsUsed++
				oc.Selected = append(oc.Selected, row)
			}
		}
		for _, p := range partners {
			owns := (p.BankClientID != nil && *p.BankClientID == client.ID) ||
				(p.BankClientID == nil && p.BankID == client.BankID && firstClientOfBank[p.BankID] == client.ID)
			if !owns {
				continue
			}
			chip := OverviewPartnerChip{
				ID: p.ID, MerchantTitle: p.MerchantTitle, Percent: p.Percent,
				CurrencyKind: CurrencyUnknown, ValidTo: p.ValidTo,
				Status: PartnerStatus(onDate, p.ValidFrom, p.ValidTo, p.EndedAt),
			}
			if p.CurrencyKind.Valid {
				chip.CurrencyKind = CurrencyKind(p.CurrencyKind.CashbackCurrencyKind)
			}
			oc.Partners = append(oc.Partners, chip)
		}
		res.Clients = append(res.Clients, oc)
	}
	return res, nil
}

// LookupResultView answers S3. Partner offers rank since партнёрки v2
// (2026-08-06): a canonical-scoped/hinted партнёрка enters Ranked as
// kind=partner; the legacy substring footnote survives only for rows
// without a canonical, until their owner maps them.
type LookupResultView struct {
	Category  db.CanonicalCategory
	Ranked    []LookupEntry    // regular + super + special + partner, marked by kind
	Fallback  []LookupEntry    // selected «За все покупки» — pays when nothing ranks
	Available []AvailableEntry // S3b: offered-but-unselected rows still pickable
	Blocked   []AvailableEntry // offered-but-unselected rows this period can no longer take
	Partner   []db.ListPartnerOffersForUserRow
}

// partnerEntryOf turns one partner_offer row into a rankable entry. Caps
// stay on the row's own fields (cap bounds the payout in the offer's OWN
// currency); nil currency lands in the unknown group, honestly last.
func partnerEntryOf(p db.ListPartnerOffersForUserRow) LookupEntry {
	e := LookupEntry{
		BankName:        p.BankName,
		RawTitle:        p.MerchantTitle,
		Percent:         p.Percent,
		CurrencyKind:    CurrencyUnknown,
		Kind:            OfferPartner,
		Period:          PartnerPeriod(p.ValidFrom, p.ValidTo),
		OfferCapValue:   p.CapValue,
		PartnerID:       p.ID,
		PartnerScope:    PartnerScope(p.ScopeKind),
		NeedsActivation: p.RequiresActivation && p.ActivatedAt == nil,
	}
	if p.CurrencyKind.Valid {
		e.CurrencyKind = CurrencyKind(p.CurrencyKind.CashbackCurrencyKind)
	}
	if e.CurrencyKind == CurrencyPoints && p.PointsLabel != nil {
		e.PointsLabel = *p.PointsLabel
	}
	if p.BankClientID != nil {
		e.ClientID = *p.BankClientID
	}
	if p.HolderLabel != nil {
		e.HolderLabel = *p.HolderLabel
		e.ClientLabel = *p.HolderLabel
	}
	return e
}

// alivePartner: not ended by the user. The date filter itself is the
// ranking's period check (PartnerPeriod covers open bounds).
func alivePartner(p db.ListPartnerOffersForUserRow) bool {
	return p.EndedAt == nil
}

func (s *Service) Lookup(ctx context.Context, userID uuid.UUID, categorySlug string, onDate time.Time) (LookupResultView, error) {
	cat, err := s.Q.GetCanonicalCategoryBySlug(ctx, categorySlug)
	if err != nil {
		return LookupResultView{}, notFound(err)
	}
	all, err := s.Q.ListUserOffers(ctx, userID)
	if err != nil {
		return LookupResultView{}, err
	}
	var entries []LookupEntry
	for _, o := range all {
		if !o.Selected || o.CanonicalCategoryID == nil || *o.CanonicalCategoryID != cat.ID {
			continue
		}
		entries = append(entries, entryOf(o))
	}
	// Friends' shared selections rank alongside own cards (FR-S4); they
	// never reach Available or the fallback below — both stay personal.
	friendEntries, err := s.friendLookupEntries(ctx, userID, cat.ID)
	if err != nil {
		return LookupResultView{}, err
	}
	entries = append(entries, friendEntries...)
	// Партнёрки of this canonical rank too (v2): merchant-scoped ones carry
	// their scope so the UI states «только в „…“» — ranked, but marked.
	partners, err := s.Q.ListPartnerOffersForUser(ctx, userID)
	if err != nil {
		return LookupResultView{}, err
	}
	for _, p := range partners {
		if !alivePartner(p) || p.CanonicalCategoryID == nil || *p.CanonicalCategoryID != cat.ID {
			continue
		}
		entries = append(entries, partnerEntryOf(p))
	}
	ranked := RankActiveSelections(onDate, entries)

	// S3b «Можно выбрать»: menu rows of this category sitting in an active
	// period WITHOUT a selection. regular+super only (special never ranks);
	// each row gets a fact-based verdict instead of a dead end.
	regCount := make(map[int64]int) // offer_period_id → selected regular rows
	for _, o := range all {
		if o.Selected && OfferKind(o.Kind) == OfferRegular {
			regCount[o.OfferPeriodID]++
		}
	}
	var available, blocked []AvailableEntry
	for _, o := range all {
		if o.Selected || o.CanonicalCategoryID == nil || *o.CanonicalCategoryID != cat.ID {
			continue
		}
		kind := OfferKind(o.Kind)
		if kind == OfferSpecial || !rowRange(o.PeriodStart, o.PeriodEnd).Contains(onDate) {
			continue
		}
		max := o.MaxCategoriesOverride
		if max == nil {
			max = o.MaxCategories
		}
		verdict := AssessAvailability(AvailabilityCheck{
			Kind:                 kind,
			Policy:               MidPeriodAddPolicy(o.MidPeriodAdd),
			HasRegularSelection:  regCount[o.OfferPeriodID] > 0,
			MaxCategories:        max,
			RegularSelectedCount: regCount[o.OfferPeriodID],
		})
		row := AvailableEntry{
			Entry:      entryOf(o),
			OfferID:    o.CategoryOfferID,
			Verdict:    verdict,
			Activation: ActivationKind(o.Activation),
		}
		if !verdict.Pickable() {
			blocked = append(blocked, row)
			continue
		}
		available = append(available, row)
	}

	// «За все покупки» answers the lookup when nothing ranks — and is worth
	// showing alongside even when something does (it pays only when no other
	// selected category matches).
	var allPurposesID *int64
	if ap, err := s.Q.GetCanonicalCategoryBySlug(ctx, "all-purchases"); err == nil {
		allPurposesID = &ap.ID
	}
	catID := cat.ID
	fb := RankActiveSelections(onDate, fallbackEntries(all, allPurposesID, &catID, entryOf))
	fallback := fb.Ranked

	// Legacy footnote — only canonical-less партнёрки still substring-match
	// here (deprecated: mapping the row promotes it into Ranked above).
	var footnote []db.ListPartnerOffersForUserRow
	needle := NormalizeTitle(cat.TitleRu)
	for _, p := range partners {
		if !alivePartner(p) || p.CanonicalCategoryID != nil {
			continue
		}
		if p.ValidFrom != nil && dateOnly(onDate).Before(dateOnly(*p.ValidFrom)) {
			continue
		}
		if p.ValidTo != nil && dateOnly(onDate).After(dateOnly(*p.ValidTo)) {
			continue
		}
		hay := NormalizeTitle(p.MerchantTitle)
		if p.Notes != nil {
			hay += " " + NormalizeTitle(*p.Notes)
		}
		if needle != "" && strings.Contains(hay, needle) {
			footnote = append(footnote, p)
		}
	}
	return LookupResultView{
		Category: cat, Ranked: ranked.Ranked,
		Fallback: fallback, Available: RankAvailable(available), Blocked: RankAvailable(blocked), Partner: footnote,
	}, nil
}

// MatchPartnerOffers answers the точка продаж: alive offers whose merchant
// title (or notes) match the queried name, normalized both directions —
// honest name-based matching, never presented as a merchant link (the POS
// base is import-owned and per-location; a FK would be false precision).
func (s *Service) MatchPartnerOffers(ctx context.Context, userID uuid.UUID, query string, onDate time.Time) ([]LookupEntry, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	partners, err := s.Q.ListPartnerOffersForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	needle := NormalizeTitle(query)
	var entries []LookupEntry
	for _, p := range partners {
		if !alivePartner(p) {
			continue
		}
		hay := NormalizeTitle(p.MerchantTitle)
		if p.Notes != nil {
			hay += " " + NormalizeTitle(*p.Notes)
		}
		if !strings.Contains(hay, needle) && !strings.Contains(needle, NormalizeTitle(p.MerchantTitle)) {
			continue
		}
		entries = append(entries, partnerEntryOf(p))
	}
	return RankActiveSelections(onDate, entries).Ranked, nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// MCCBoard is the exact answer for one code (10b variant 3, 2026-08-27):
// every entry got here through a bank's OWN category holding the code —
// bank_category_mcc membership — never through a canonical guess. Banks
// without ingested MCC memberships fall to Base («Кешбек на всё»), honestly:
// approximate ranking on an MCC-driven screen was rejected outright.
// Exclusion lists («код исключён банком», 13c) are a recorded follow-up —
// they need their own model and a per-bank rules ingestion first.
type MCCBoard struct {
	Ranked    []LookupEntry    // selected rows, own + friends', stacked супер folded in
	Available []AvailableEntry // exact-matched menu rows still pickable («свободный слот»)
	Blocked   []AvailableEntry // exact-matched menu rows this period can no longer take
	// FriendAvailable is the same «в меню, но не выбрано» fact on a friend's
	// shared card, and it is actionable in the way that matters here: the
	// friend can still pick it if asked. Kept apart from Available because
	// the viewer cannot pick it themselves — no offer id travels with it.
	FriendAvailable []AvailableEntry
	Base            []LookupEntry // clients whose only answer is the selected base row
}

// friendRowKey collapses a friend's same menu row across the two periods the
// share window can span.
type friendRowKey struct {
	clientID int64
	title    string
}

// LookupByMCC builds the board for one code on one date. Friends ride the
// same exact filter over their shared rows — invariants 4 (no caps) and 8
// (the share window) hold exactly as in friendEntriesByCategory.
func (s *Service) LookupByMCC(ctx context.Context, userID uuid.UUID, code int16, onDate time.Time) (MCCBoard, error) {
	var board MCCBoard
	if s.MCCMemberships == nil {
		return board, nil
	}
	members, err := s.MCCMemberships(ctx, userID, code)
	if err != nil {
		return board, err
	}
	// Two ways a menu row can be «the bank's category for this code».
	//
	// Exact: the row was entered through the picker and points at the catalog
	// row (bank_category_id) that carries the code. That is the strong form
	// and stays the primary one.
	//
	// By canonical, WITHIN THE SAME BANK: a row entered by title — a custom
	// category, a bank whose catalog the picker had to fall back from, an
	// import — has no catalog link, and requiring one made it invisible here
	// no matter how well it mapped (report 2026-08-28: «for every PoS it
	// showed no cards»). If the bank counts 5411 in its «Продукты», and the
	// user's selected row IS «Продукты» by canonical identity, the bank's
	// answer for that code is known. Staying inside one bank is what keeps
	// this honest: it never borrows another bank's category set.
	matched := make(map[int64]bool, len(members))
	matchedCanon := make(map[int32]map[int64]bool)
	for _, m := range members {
		matched[m.BankCategoryID] = true
		if m.CanonicalCategoryID == nil {
			continue
		}
		if matchedCanon[m.BankID] == nil {
			matchedCanon[m.BankID] = make(map[int64]bool)
		}
		matchedCanon[m.BankID][*m.CanonicalCategoryID] = true
	}
	// countsForCode reports whether this menu row is the bank's category for
	// the looked-up code, by either route.
	countsForCode := func(bankID int32, bankCategoryID, canonicalID *int64) bool {
		if bankCategoryID != nil && matched[*bankCategoryID] {
			return true
		}
		return canonicalID != nil && matchedCanon[bankID][*canonicalID]
	}

	offers, err := s.Q.ListUserOffers(ctx, userID)
	if err != nil {
		return board, err
	}
	regCount := make(map[int64]int) // offer_period_id → selected regular rows
	for _, o := range offers {
		if o.Selected && OfferKind(o.Kind) == OfferRegular {
			regCount[o.OfferPeriodID]++
		}
	}

	var entries []LookupEntry
	var avail, blocked []AvailableEntry
	covered := make(map[int64]bool) // client ids answering above the base fold
	for _, o := range offers {
		if !countsForCode(o.BankID, o.BankCategoryID, o.CanonicalCategoryID) {
			continue
		}
		if !rowRange(o.PeriodStart, o.PeriodEnd).Contains(onDate) {
			continue
		}
		if o.Selected {
			entries = append(entries, entryOf(o))
			covered[o.BankClientID] = true
			continue
		}
		if OfferKind(o.Kind) == OfferSpecial {
			continue
		}
		max := o.MaxCategoriesOverride
		if max == nil {
			max = o.MaxCategories
		}
		verdict := AssessAvailability(AvailabilityCheck{
			Kind:                 OfferKind(o.Kind),
			Policy:               MidPeriodAddPolicy(o.MidPeriodAdd),
			HasRegularSelection:  regCount[o.OfferPeriodID] > 0,
			MaxCategories:        max,
			RegularSelectedCount: regCount[o.OfferPeriodID],
		})
		row := AvailableEntry{
			Entry:      entryOf(o),
			OfferID:    o.CategoryOfferID,
			Verdict:    verdict,
			Activation: ActivationKind(o.Activation),
		}
		if !verdict.Pickable() {
			// The bank does count this code — in a category this period can
			// no longer take. Shown apart, and deliberately NOT «covered»:
			// the client's honest answer here stays its base row.
			blocked = append(blocked, row)
			continue
		}
		avail = append(avail, row)
		covered[o.BankClientID] = true
	}

	friends, rows, err := s.sharedRows(ctx, userID)
	if err != nil {
		return board, err
	}
	if len(rows) > 0 {
		window := FriendShareWindow(time.Now())
		owner := make(map[int64]*SharedFriend)
		for i := range friends {
			for _, id := range friends[i].BankClientIDs {
				owner[id] = &friends[i]
			}
		}
		// The friend's own slot arithmetic decides whether an unpicked row of
		// theirs is still takeable.
		friendBest := make(map[friendRowKey]AvailableEntry)
		friendRegCount := make(map[int64]int)
		for _, r := range rows {
			if r.Selected && OfferKind(r.Kind) == OfferRegular {
				friendRegCount[r.OfferPeriodID]++
			}
		}
		for _, r := range rows {
			if !countsForCode(r.BankID, r.BankCategoryID, r.CanonicalCategoryID) {
				continue
			}
			if !rowRange(r.PeriodStart, r.PeriodEnd).Overlaps(window) {
				continue
			}
			f := owner[r.BankClientID]
			if f == nil {
				continue
			}
			e := entryOf(db.ListUserOffersRow(r))
			e.CapValue, e.CapPerCategory, e.OfferCapValue, e.CapScope = nil, nil, nil, ""
			e.FriendName = f.DisplayName
			e.FriendUsername = f.Username
			if r.Selected {
				entries = append(entries, e)
				continue
			}
			// Unpicked, and the bank counts this code in it. Worth showing
			// even though the viewer cannot act: asking the friend to pick it
			// is the whole point of sharing a menu.
			if OfferKind(r.Kind) == OfferSpecial {
				continue
			}
			max := r.MaxCategoriesOverride
			if max == nil {
				max = r.MaxCategories
			}
			verdict := AssessAvailability(AvailabilityCheck{
				Kind:                 OfferKind(r.Kind),
				Policy:               MidPeriodAddPolicy(r.MidPeriodAdd),
				HasRegularSelection:  friendRegCount[r.OfferPeriodID] > 0,
				MaxCategories:        max,
				RegularSelectedCount: friendRegCount[r.OfferPeriodID],
			})
			if !verdict.Pickable() {
				continue // a dead end for them too — nothing to ask for
			}
			// The share window spans two months, so the same row can be
			// unpicked in both this period and the next one. They are one
			// fact to the viewer — «Марина может это выбрать» — and two rows
			// differing only by a period the board never shows read as a
			// duplicate. Keep the better offer per client and title.
			key := friendRowKey{clientID: r.BankClientID, title: r.RawTitle}
			if prev, seen := friendBest[key]; seen {
				if cmpPercentDesc(e.Percent, prev.Entry.Percent) >= 0 {
					continue
				}
			}
			friendBest[key] = AvailableEntry{
				Entry: e, Verdict: verdict, Activation: ActivationKind(r.Activation),
			}
		}

		for _, row := range friendBest {
			board.FriendAvailable = append(board.FriendAvailable, row)
		}
	}
	// The viewer's own three lists are built from `offers` above and have
	// nothing to do with sharing, so they are assigned outside the friends
	// block — inside it, a viewer with no shared rows got an empty board.
	board.Ranked = RankActiveSelections(onDate, entries).Ranked
	board.Available = RankAvailable(avail)
	board.Blocked = RankAvailable(blocked)
	board.FriendAvailable = RankAvailable(board.FriendAvailable)

	var allPurposesID *int64
	if ap, err := s.Q.GetCanonicalCategoryBySlug(ctx, "all-purchases"); err == nil {
		allPurposesID = &ap.ID
	}
	for _, e := range RankActiveSelections(onDate, fallbackEntries(offers, allPurposesID, nil, entryOf)).Ranked {
		if !covered[e.ClientID] {
			board.Base = append(board.Base, e)
		}
	}
	return board, nil
}

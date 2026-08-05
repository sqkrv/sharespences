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

	recognitions recognitionStore
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
	return LookupEntry{
		ClientID:       o.BankClientID,
		ClientLabel:    clientLabel(o.BankName, o.HolderLabel),
		HolderLabel:    holderOf(o.HolderLabel),
		BankName:       o.BankName,
		RawTitle:       o.RawTitle,
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
	period, err := s.Q.CreateOfferPeriod(ctx, db.CreateOfferPeriodParams{BankClientID: clientID, PeriodStart: start, PeriodEnd: end})
	if err != nil {
		if isPgCode(err, "23P01") || isPgCode(err, "23505") {
			return db.OfferPeriod{}, ErrPeriodOverlap
		}
		return db.OfferPeriod{}, err
	}
	for _, aid := range attachmentIDs {
		if _, err := s.Q.GetAttachmentForUser(ctx, db.GetAttachmentForUserParams{ID: aid, UserID: &userID}); err != nil {
			return db.OfferPeriod{}, notFound(err)
		}
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
	uid := userID
	orphaned, err := s.Q.DeleteAttachmentIfOrphan(ctx, db.DeleteAttachmentIfOrphanParams{ID: attachmentID, UserID: &uid})
	if err != nil {
		return err
	}
	if orphaned > 0 && s.RemoveAttachmentFile != nil {
		if err := s.RemoveAttachmentFile(attachmentID); err != nil {
			// The row is gone; a stale file is a cleanup nit, not a failure.
			return nil
		}
	}
	return nil
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

// DeleteOfferPeriod removes a period with everything under it (menu rows,
// their selections, attachment links — the attachment files stay).
func (s *Service) DeleteOfferPeriod(ctx context.Context, userID uuid.UUID, periodID int64) error {
	if _, err := s.Q.GetOfferPeriodForUser(ctx, db.GetOfferPeriodForUserParams{ID: periodID, UserID: userID}); err != nil {
		return notFound(err)
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
	return s.Q.DeleteOfferPeriod(ctx, periodID)
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
type OverviewCategoryGroup struct {
	CategoryID  int64
	Slug        string
	TitleRu     string
	Emoji       string // canonical category icon for the list (2026-07-27)
	Best        LookupEntry
	OthersCount int
}

// OverviewSelectedRow is a selected menu row shown as a chip on a card.
type OverviewSelectedRow struct {
	OfferID  int64
	RawTitle string
	Kind     OfferKind
	Percent  *decimal.Decimal
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
}

// OverviewBase is the «Остальное» row: the best base-rate card («За все
// покупки» granted rows plus regular rows mapped to all-purchases).
type OverviewBase struct {
	Emoji       string // all-purchases icon — keeps the list's icon column aligned
	Best        LookupEntry
	OthersCount int
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
type OverviewResult struct {
	Categories        []OverviewCategoryGroup
	Base              *OverviewBase
	Clients           []OverviewClient
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
	for catID, entries := range byCat {
		ranked := RankActiveSelections(onDate, entries)
		cat, ok := catByID[catID]
		if !ok {
			continue
		}
		if len(ranked.Ranked) == 0 {
			continue // nothing active
		}
		// All three kinds rank (invariant 6 amendment, 2026-07-27): the
		// best card may be a барабан or a спец — the frontend marks it.
		res.Categories = append(res.Categories, OverviewCategoryGroup{
			CategoryID:  catID,
			Slug:        cat.Slug,
			TitleRu:     cat.TitleRu,
			Emoji:       emojiOf(cat),
			Best:        ranked.Ranked[0],
			OthersCount: len(ranked.Ranked) - 1,
		})
	}
	// Sort: rub before points; then percent desc; then title.
	sort.SliceStable(res.Categories, func(i, j int) bool {
		a, b := res.Categories[i].Best, res.Categories[j].Best
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

	// «Остальное»: best selected «За все покупки» across clients.
	fb := RankActiveSelections(onDate, fallbackEntries(offers, allPurposesID, nil, entryOf))
	if len(fb.Ranked) > 0 {
		base := OverviewBase{Best: fb.Ranked[0], OthersCount: len(fb.Ranked) - 1}
		if allPurposesID != nil {
			base.Emoji = emojiOf(catByID[*allPurposesID])
		}
		res.Base = &base
	}

	// --- «Карты»: every bank client with its plastics, and the client's
	// active period when one exists (all its cards share it). ---
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
			if program.SelectionOpensDay != nil &&
				(res.SelectionOpensDay == nil || *program.SelectionOpensDay < *res.SelectionOpensDay) {
				res.SelectionOpensDay = program.SelectionOpensDay
			}
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
			// Only regular fills a slot and shows as a chosen (mint) chip;
			// granted super/special go to the gold bonus chips (no slot).
			if OfferKind(o.Kind) != OfferRegular {
				oc.Specials = append(oc.Specials, row)
			} else {
				oc.SlotsUsed++
				oc.Selected = append(oc.Selected, row)
			}
		}
		res.Clients = append(res.Clients, oc)
	}
	return res, nil
}

// LookupResultView answers S3, with partner offers as an unranked footnote
// (matched by merchant/notes text against the category title).
type LookupResultView struct {
	Category  db.CanonicalCategory
	Ranked    []LookupEntry    // regular + super + special, marked by kind (amendment 2026-07-27)
	Fallback  []LookupEntry    // selected «За все покупки» — pays when nothing ranks
	Available []AvailableEntry // S3b: offered-but-unselected rows with verdicts
	Partner   []db.ListPartnerOffersForUserRow
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
	var available []AvailableEntry
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
		available = append(available, AvailableEntry{
			Entry:   entryOf(o),
			OfferID: o.CategoryOfferID,
			Verdict: AssessAvailability(AvailabilityCheck{
				Kind:                 kind,
				Policy:               MidPeriodAddPolicy(o.MidPeriodAdd),
				HasRegularSelection:  regCount[o.OfferPeriodID] > 0,
				MaxCategories:        max,
				RegularSelectedCount: regCount[o.OfferPeriodID],
			}),
			Activation: ActivationKind(o.Activation),
		})
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

	partners, err := s.Q.ListPartnerOffersForUser(ctx, userID)
	if err != nil {
		return LookupResultView{}, err
	}
	var footnote []db.ListPartnerOffersForUserRow
	needle := NormalizeTitle(cat.TitleRu)
	for _, p := range partners {
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
		Fallback: fallback, Available: RankAvailable(available), Partner: footnote,
	}, nil
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

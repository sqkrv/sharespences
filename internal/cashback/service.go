package cashback

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/sqkrv/sharespences/internal/db"
)

// ErrNotFound covers rows that don't exist or belong to another user —
// scoping never reveals which.
var ErrNotFound = errors.New("cashback: not found")

// Service wires the domain rules to storage. Reference reads of bank /
// bank_card are the seam decided at skeleton time (00003_cashback.sql).
// RemoveAttachmentFile is injected at assembly (the attachment module owns
// the disk store); called after an orphaned attachment row is deleted.
type Service struct {
	Q                    *db.Queries
	RemoveAttachmentFile func(id uuid.UUID) error
}

func cardLabel(bankName string, last4 int32) string {
	return fmt.Sprintf("%s ··%04d", bankName, last4)
}

func holderOf(h *string) string {
	if h == nil {
		return ""
	}
	return *h
}

// capNote renders the static cap reference the helper and warnings display,
// e.g. «лимит 1500₽/кат, всего 3000₽» (Озон), «лимит 7000₽» (Альфа-Смарт).
func capNote(capValue, capPerCategory *decimal.Decimal, scope db.NullCashbackCapScope, currency db.NullCashbackCurrencyKind, pointsLabel *string) string {
	unit := "₽"
	if currency.Valid && currency.CashbackCurrencyKind == db.CashbackCurrencyKindPoints {
		unit = " баллов"
		if pointsLabel != nil {
			unit = " " + *pointsLabel
		}
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
		CardID:         int64(o.CardID),
		CardLabel:      cardLabel(o.BankName, o.Last4Digits),
		HolderLabel:    holderOf(o.HolderLabel),
		BankName:       o.BankName,
		Percent:        o.Percent,
		CurrencyKind:   currencyOf(o),
		Kind:           OfferKind(o.Kind),
		Period:         rowRange(o.PeriodStart, o.PeriodEnd),
		CapValue:       o.CapValue,
		CapPerCategory: o.CapPerCategory,
		CapScope:       capScope,
		PointsLabel:    pointsLabel,
	}
}

// activeSelectionOf maps a selected ListUserOffers row into the domain view.
func activeSelectionOf(row db.ListUserOffersRow) ActiveSelection {
	return ActiveSelection{
		CardID:              int64(row.CardID),
		CardLabel:           cardLabel(row.BankName, row.Last4Digits),
		HolderLabel:         holderOf(row.HolderLabel),
		BankName:            row.BankName,
		CanonicalCategoryID: row.CanonicalCategoryID,
		Period:              rowRange(row.PeriodStart, row.PeriodEnd),
		Kind:                OfferKind(row.Kind),
		Percent:             row.Percent,
		CurrencyKind:        currencyOf(row),
		CapNote:             capNote(row.CapValue, row.CapPerCategory, row.TierCapScope, row.ProgramCurrencyKind, row.PointsLabel),
	}
}

// CreateOfferPeriod enforces invariant 4 in the service; the DB exclusion
// constraint backstops races.
func (s *Service) CreateOfferPeriod(ctx context.Context, userID uuid.UUID, cardID int32, start, end time.Time, attachmentIDs []uuid.UUID) (db.OfferPeriod, error) {
	if _, err := s.Q.GetCardForUser(ctx, db.GetCardForUserParams{ID: cardID, UserID: userID}); err != nil {
		return db.OfferPeriod{}, notFound(err)
	}
	ranges, err := s.Q.ListPeriodRangesForCard(ctx, cardID)
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
	period, err := s.Q.CreateOfferPeriod(ctx, db.CreateOfferPeriodParams{CardID: cardID, PeriodStart: start, PeriodEnd: end})
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
// (owner 2026-07-09: screenshots must be editable after creation, not only
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
	aliases, err := s.Q.ListAliasesForBank(ctx, int32(period.BankID))
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

// CreateCategoryOffer records one menu row. A provided canonical mapping is
// remembered as a bank alias (S1: unknown titles create the mapping inline).
func (s *Service) CreateCategoryOffer(ctx context.Context, userID uuid.UUID, offerPeriodID int64, rawTitle string, canonicalID *int64, percent *decimal.Decimal, kind OfferKind, notes *string) (db.CategoryOffer, error) {
	period, err := s.Q.GetOfferPeriodForUser(ctx, db.GetOfferPeriodForUserParams{ID: offerPeriodID, UserID: userID})
	if err != nil {
		return db.CategoryOffer{}, notFound(err)
	}
	offer, err := s.Q.CreateCategoryOffer(ctx, db.CreateCategoryOfferParams{
		OfferPeriodID:       offerPeriodID,
		RawTitle:            rawTitle,
		CanonicalCategoryID: canonicalID,
		Percent:             percent,
		Kind:                db.CashbackOfferKind(kind),
		Notes:               notes,
	})
	if err != nil {
		return db.CategoryOffer{}, err
	}
	if canonicalID != nil {
		if err := s.Q.UpsertAlias(ctx, db.UpsertAliasParams{
			CanonicalCategoryID: *canonicalID,
			BankID:              int32(period.BankID),
			RawTitle:            rawTitle,
		}); err != nil {
			return db.CategoryOffer{}, err
		}
	}
	return offer, nil
}

// effectiveMax resolves invariant 1's limit: the period-level override wins
// over the tier default (owner feedback 2026-07-04: slot counts vary between
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
// dated selection event. Cross-card duplicates never block here — the entry
// screen surfaces them via HelperContext (invariant 3).
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

// UpdateCategoryOffer replaces the mutable fields of a menu row (owner
// feedback 2026-07-04: entered rows must be correctable — a row mapped to a
// canonical category after the fact starts appearing in lookups). A newly
// set canonical mapping is remembered as a bank alias, like on create.
func (s *Service) UpdateCategoryOffer(ctx context.Context, userID uuid.UUID, offerID int64, rawTitle string, canonicalID *int64, percent *decimal.Decimal, kind OfferKind, notes *string) (db.CategoryOffer, error) {
	ctxRow, err := s.Q.GetOfferWithContextForUser(ctx, db.GetOfferWithContextForUserParams{ID: offerID, UserID: userID})
	if err != nil {
		return db.CategoryOffer{}, notFound(err)
	}
	offer, err := s.Q.UpdateCategoryOfferForUser(ctx, db.UpdateCategoryOfferForUserParams{
		ID:                  offerID,
		UserID:              userID,
		RawTitle:            rawTitle,
		CanonicalCategoryID: canonicalID,
		Percent:             percent,
		Kind:                db.CashbackOfferKind(kind),
		Notes:               notes,
	})
	if err != nil {
		return db.CategoryOffer{}, notFound(err)
	}
	if canonicalID != nil {
		if err := s.Q.UpsertAlias(ctx, db.UpsertAliasParams{
			CanonicalCategoryID: *canonicalID,
			BankID:              int32(ctxRow.BankID),
			RawTitle:            rawTitle,
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
// cross-card duplicate warnings and same-currency comparisons per menu row.
// Comparisons pool = same canonical category rows on the user's OTHER cards
// with overlapping periods (so «Супермаркеты 5%» can be judged against the
// other cards' offers of the same category), same currency only.
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
			CardID:              int64(period.CardID),
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
				CardID:       int64(period.CardID),
				BankName:     period.BankName,
				CardLabel:    cardLabel(period.BankName, period.Last4Digits),
			}
			var pool []OfferView
			for _, o := range all {
				if int64(o.CardID) == int64(period.CardID) ||
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
					CardID:       int64(o.CardID),
					BankName:     o.BankName,
					CardLabel:    cardLabel(o.BankName, o.Last4Digits),
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
	Best        LookupEntry
	OthersCount int
}

// OverviewSelectedRow is a selected menu row shown as a chip on a card.
type OverviewSelectedRow struct {
	OfferID  int64
	RawTitle string
	Percent  *decimal.Decimal
}

// OverviewCard is one row of the «Карты» cut. Period is nil when the card
// has no offer_period covering the date («нет периода», the design's dashed
// card with «Добавить»).
type OverviewCard struct {
	CardID        int32
	BankID        int16
	BankName      string
	Last4Digits   int32
	HolderLabel   *string
	TierName      *string
	IsPaidTier    bool
	CapValue      *decimal.Decimal
	CapScope      CapScope
	CapPerCat     *decimal.Decimal
	CurrencyKind  CurrencyKind
	PointsLabel   string
	SelectionMode string
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
	Best        LookupEntry
	OthersCount int
}

// OverviewResult answers GET /cashback/overview: the design's two cuts of
// the same month (screens 01/02), plus the passive «selection opens» day.
type OverviewResult struct {
	Categories        []OverviewCategoryGroup
	Base              *OverviewBase
	Cards             []OverviewCard
	SelectionOpensDay *int32 // earliest across the user's cards' programs
}

// fallbackEntries picks the selected rows that answer «а если категория не
// выбрана нигде?»: ordinary regular rows mapped to canonical all-purchases
// («За все покупки» — a category like any other, owner 2026-07-09; it pays
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
	cards, err := s.Q.ListCardsForUser(ctx, userID)
	if err != nil {
		return OverviewResult{}, err
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
		if len(ranked.Ranked) == 0 {
			continue // only specials or nothing active — not a lookup answer (invariant 6)
		}
		cat, ok := catByID[catID]
		if !ok {
			continue
		}
		res.Categories = append(res.Categories, OverviewCategoryGroup{
			CategoryID:  catID,
			Slug:        cat.Slug,
			TitleRu:     cat.TitleRu,
			Best:        ranked.Ranked[0],
			OthersCount: len(ranked.Ranked) - 1,
		})
	}
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

	// «Остальное»: best selected «За все покупки» across cards.
	fb := RankActiveSelections(onDate, fallbackEntries(offers, allPurposesID, nil, entryOf))
	if len(fb.Ranked) > 0 {
		res.Base = &OverviewBase{Best: fb.Ranked[0], OthersCount: len(fb.Ranked) - 1}
	}

	// --- «Карты»: every card, with its active period when one exists. ---
	for _, card := range cards {
		oc := OverviewCard{
			CardID:       card.ID,
			BankID:       card.BankID,
			BankName:     card.BankName,
			Last4Digits:  card.Last4Digits,
			HolderLabel:  card.HolderLabel,
			CurrencyKind: CurrencyUnknown,
		}
		if card.ProgramTierID != nil {
			tier, err := s.Q.GetTier(ctx, *card.ProgramTierID)
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
			oc.SelectionMode = string(program.SelectionMode)
			if program.SelectionOpensDay != nil &&
				(res.SelectionOpensDay == nil || *program.SelectionOpensDay < *res.SelectionOpensDay) {
				res.SelectionOpensDay = program.SelectionOpensDay
			}
		}
		for _, o := range offers {
			if o.CardID != card.ID || !rowRange(o.PeriodStart, o.PeriodEnd).Contains(onDate) {
				continue
			}
			if oc.PeriodID == nil {
				id, start, end := o.OfferPeriodID, o.PeriodStart, o.PeriodEnd
				oc.PeriodID, oc.PeriodStart, oc.PeriodEnd = &id, &start, &end
				if o.MaxCategoriesOverride != nil {
					oc.MaxCategories = o.MaxCategoriesOverride
				}
			}
			if !o.Selected {
				continue
			}
			row := OverviewSelectedRow{OfferID: o.CategoryOfferID, RawTitle: o.RawTitle, Percent: o.Percent}
			if OfferKind(o.Kind) == OfferSpecial {
				oc.Specials = append(oc.Specials, row)
			} else {
				oc.SlotsUsed++
				oc.Selected = append(oc.Selected, row)
			}
		}
		res.Cards = append(res.Cards, oc)
	}
	return res, nil
}

// LookupResultView answers S3, with partner offers as an unranked footnote
// (matched by merchant/notes text against the category title).
type LookupResultView struct {
	Category db.CanonicalCategory
	Ranked   []LookupEntry
	Special  []LookupEntry
	Fallback []LookupEntry // selected «За все покупки» — pays when nothing ranks
	Partner  []db.ListPartnerOffersForUserRow
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
	ranked := RankActiveSelections(onDate, entries)

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
	return LookupResultView{Category: cat, Ranked: ranked.Ranked, Special: ranked.Special, Fallback: fallback, Partner: footnote}, nil
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

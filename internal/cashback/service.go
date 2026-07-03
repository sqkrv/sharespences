package cashback

import (
	"context"
	"errors"
	"fmt"
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
type Service struct {
	Q *db.Queries
}

func cardLabel(bankName string, last4 int32) string {
	return fmt.Sprintf("%s ··%04d", bankName, last4)
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

// activeSelectionOf maps a selected ListUserOffers row into the domain view.
func activeSelectionOf(row db.ListUserOffersRow) ActiveSelection {
	return ActiveSelection{
		CardID:              int64(row.CardID),
		CardLabel:           cardLabel(row.BankName, row.Last4Digits),
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

// CreateSelection enforces invariants 1 and 2 (hard rejects) and records the
// dated selection event. Cross-card duplicates never block here — the entry
// screen surfaces them via HelperContext (invariant 3).
func (s *Service) CreateSelection(ctx context.Context, userID uuid.UUID, categoryOfferID int64, selectedAt time.Time, backfill bool) (db.Selection, error) {
	offer, err := s.Q.GetOfferWithContextForUser(ctx, db.GetOfferWithContextForUserParams{ID: categoryOfferID, UserID: userID})
	if err != nil {
		return db.Selection{}, notFound(err)
	}
	var maxCategories *int32
	if offer.ProgramTierID != nil {
		tier, err := s.Q.GetTier(ctx, *offer.ProgramTierID)
		if err != nil {
			return db.Selection{}, err
		}
		maxCategories = tier.MaxCategories
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
type HelperContextResult struct {
	Period        db.GetOfferPeriodForUserRow
	SlotsUsed     int
	MaxCategories *int32
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
	res := HelperContextResult{Period: period}
	if period.ProgramTierID != nil {
		tier, err := s.Q.GetTier(ctx, *period.ProgramTierID)
		if err != nil {
			return HelperContextResult{}, err
		}
		res.MaxCategories = tier.MaxCategories
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

// LookupResultView answers S3, with partner offers as an unranked footnote
// (matched by merchant/notes text against the category title).
type LookupResultView struct {
	Category db.CanonicalCategory
	Ranked   []LookupEntry
	Special  []LookupEntry
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
		var capScope CapScope
		if o.TierCapScope.Valid {
			capScope = CapScope(o.TierCapScope.CashbackCapScope)
		}
		var pointsLabel string
		if o.PointsLabel != nil {
			pointsLabel = *o.PointsLabel
		}
		entries = append(entries, LookupEntry{
			CardID:         int64(o.CardID),
			CardLabel:      cardLabel(o.BankName, o.Last4Digits),
			BankName:       o.BankName,
			Percent:        o.Percent,
			CurrencyKind:   currencyOf(o),
			Kind:           OfferKind(o.Kind),
			Period:         rowRange(o.PeriodStart, o.PeriodEnd),
			CapValue:       o.CapValue,
			CapPerCategory: o.CapPerCategory,
			CapScope:       capScope,
			PointsLabel:    pointsLabel,
		})
	}
	ranked := RankActiveSelections(onDate, entries)

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
	return LookupResultView{Category: cat, Ranked: ranked.Ranked, Special: ranked.Special, Partner: footnote}, nil
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

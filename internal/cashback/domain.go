// Package cashback implements the КБ (cashback) module: recording per-card
// offer menus and selections, the constraint helper, and the category-level
// lookup. Domain rules follow docs/specs/cashback.md (private meta-repo);
// invariant numbers in comments refer to its "Invariants" section.
package cashback

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// CurrencyKind is the currency a program pays КБ in. Helper comparisons and
// lookup ranking never cross currency kinds (invariant 5).
type CurrencyKind string

const (
	CurrencyRub    CurrencyKind = "rub"
	CurrencyPoints CurrencyKind = "points"
	// CurrencyUnknown marks cards without a resolvable program (no tier set).
	// Never compared with anything; listed as its own last group in lookups.
	CurrencyUnknown CurrencyKind = "unknown"
)

// OfferKind separates regular menu rows from special bonus-mechanic rows
// (барабан суперкэшбека, Альфа-Пятница, колесо фортуны) and the base
// «За все покупки» rate. Special rows are record-only: excluded from helper
// math and lookup ranking (invariant 6); base rows are the granted-outside-
// the-menu fallback: slot-free and non-colliding like special, but INCLUDED
// in lookups as the fallback answer (owner decisions 2026-07-03/2026-07-09).
// Where a bank makes the base rate a selectable slot choice, the row is
// entered as regular instead.
type OfferKind string

const (
	OfferRegular OfferKind = "regular"
	OfferSpecial OfferKind = "special"
	OfferBase    OfferKind = "base"
)

// PeriodType mirrors cashback_program.period_type.
type PeriodType string

const (
	PeriodCalendarMonth PeriodType = "calendar_month"
	PeriodQuarter       PeriodType = "quarter"
	PeriodWeek          PeriodType = "week"
	PeriodRolling       PeriodType = "rolling"
)

// SelectionMode mirrors cashback_program.selection_mode.
type SelectionMode string

const (
	SelectionAtomic      SelectionMode = "atomic"
	SelectionIncremental SelectionMode = "incremental"
)

// CapScope mirrors program_tier.cap_scope.
type CapScope string

const (
	CapTotal       CapScope = "total"
	CapPerCategory CapScope = "per_category"
	CapBoth        CapScope = "both"
)

var (
	// ErrInvalidPeriod — period range has End before Start.
	ErrInvalidPeriod = errors.New("cashback: invalid period range")
	// ErrPeriodOverlap — invariant 4: offer_period ranges for one card never overlap.
	ErrPeriodOverlap = errors.New("cashback: offer periods overlap for this card")
	// ErrSlotsExhausted — invariant 1: selections per period ≤ tier.max_categories.
	ErrSlotsExhausted = errors.New("cashback: tier category slots exhausted")
	// ErrOutsidePeriod — invariant 2: selected_at date outside the offer's period.
	ErrOutsidePeriod = errors.New("cashback: selection date outside offer period")
	// ErrAlreadySelected — an offer is selected at most once.
	ErrAlreadySelected = errors.New("cashback: offer already selected")
)

// Date builds a calendar date (UTC midnight). Period bounds are dates, not
// instants; keep them constructed through this helper.
func Date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// DateRange is an inclusive [Start, End] calendar-date range
// (МКБ quarter: 01.01–31.03; months: 01.07–31.07).
type DateRange struct {
	Start time.Time
	End   time.Time
}

// dateOnly truncates t to its calendar date in t's own location,
// re-anchored to UTC so all range math compares plain dates.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Valid reports whether Start ≤ End.
func (r DateRange) Valid() bool {
	return !dateOnly(r.End).Before(dateOnly(r.Start))
}

// Contains reports whether the calendar date of t (in t's own location)
// falls inside the range, boundaries inclusive.
func (r DateRange) Contains(t time.Time) bool {
	d := dateOnly(t)
	return !d.Before(dateOnly(r.Start)) && !d.After(dateOnly(r.End))
}

// Overlaps reports whether two inclusive ranges share at least one day.
// Overlap is computed on date ranges regardless of period kind, so a МКБ
// quarter collides with an Альфа-Банк month (S5).
func (r DateRange) Overlaps(o DateRange) bool {
	return !dateOnly(r.Start).After(dateOnly(o.End)) && !dateOnly(o.Start).After(dateOnly(r.End))
}

// ValidateNewPeriod enforces invariant 4 for a card's new offer_period
// against its existing period ranges.
func ValidateNewPeriod(candidate DateRange, existing []DateRange) error {
	if !candidate.Valid() {
		return ErrInvalidPeriod
	}
	for _, e := range existing {
		if candidate.Overlaps(e) {
			return ErrPeriodOverlap
		}
	}
	return nil
}

// SelectionCheck carries everything needed to validate one new selection
// event against invariants 1 and 2.
type SelectionCheck struct {
	// Period is the offer's offer_period range.
	Period DateRange
	// SelectedAt is the selection event timestamp; invariant 2 compares its
	// calendar date (in its own location) against Period.
	SelectedAt time.Time
	// OfferKind of the offer being selected. Special selections are recorded
	// but never consume slots.
	OfferKind OfferKind
	// AlreadySelected — this offer already has a selection (UNIQUE in schema).
	AlreadySelected bool
	// MaxCategories is tier.max_categories; nil = unknown/unset → no slot check.
	MaxCategories *int32
	// RegularSelectedCount is the number of existing kind=regular selections
	// in this offer_period.
	RegularSelectedCount int
	// BackfillOverride skips the period-containment check (invariant 2's
	// manual override for entering history).
	BackfillOverride bool
}

// ValidateSelection returns nil if the selection may be recorded, or one of
// ErrAlreadySelected, ErrOutsidePeriod, ErrSlotsExhausted (hard rejects).
// Cross-card duplicates are NOT checked here — they are warnings, never
// blocks (invariant 3); see DetectCollisions.
func ValidateSelection(c SelectionCheck) error {
	if c.AlreadySelected {
		return ErrAlreadySelected
	}
	if !c.BackfillOverride && !c.Period.Contains(c.SelectedAt) {
		return ErrOutsidePeriod
	}
	if c.OfferKind == OfferRegular && c.MaxCategories != nil &&
		c.RegularSelectedCount >= int(*c.MaxCategories) {
		return ErrSlotsExhausted
	}
	return nil
}

// CandidateSelection identifies the offer a user is about to select, for
// collision warnings.
type CandidateSelection struct {
	CardID              int64
	CanonicalCategoryID *int64
	Period              DateRange
	Kind                OfferKind
}

// ActiveSelection is an existing selection on some card of the same user,
// with display fields the warning message needs.
type ActiveSelection struct {
	CardID              int64
	CardLabel           string // «Альфа-Банк ··1234»
	HolderLabel         string // whose plastic («Мама»); empty = the owner
	BankName            string
	CanonicalCategoryID *int64
	Period              DateRange
	Kind                OfferKind
	Percent             *decimal.Decimal
	CurrencyKind        CurrencyKind
	CapNote             string // static cap info for display, e.g. «лимит 1500₽/кат»
}

// Collision is a cross-card duplicate warning (invariant 3): advisory only.
type Collision struct {
	Other ActiveSelection
}

// DetectCollisions returns a warning per existing selection of the same
// canonical category on a DIFFERENT card with an overlapping period.
// Only regular offers participate: special is excluded from helper math
// (invariant 6), base rates exist on every bank by design (nothing to
// warn about); offers without a canonical mapping cannot collide.
func DetectCollisions(candidate CandidateSelection, others []ActiveSelection) []Collision {
	if candidate.CanonicalCategoryID == nil || candidate.Kind != OfferRegular {
		return nil
	}
	var out []Collision
	for _, o := range others {
		switch {
		case o.Kind != OfferRegular,
			o.CanonicalCategoryID == nil,
			*o.CanonicalCategoryID != *candidate.CanonicalCategoryID,
			o.CardID == candidate.CardID,
			!candidate.Period.Overlaps(o.Period):
			continue
		}
		out = append(out, Collision{Other: o})
	}
	return out
}

// cmpPercentDesc orders percents descending with unknown (nil) last.
func cmpPercentDesc(a, b *decimal.Decimal) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	default:
		return b.Cmp(*a)
	}
}

// OfferView is a menu row as the helper compares them.
type OfferView struct {
	OfferID      int64
	RawTitle     string
	Percent      *decimal.Decimal
	Kind         OfferKind
	CurrencyKind CurrencyKind
	CardID       int64
	CardLabel    string
	BankName     string
}

// ComparableOffers returns the pool rows the helper may show side by side
// with the candidate: same currency_kind only (invariant 5), kind=regular
// only (invariant 6), the candidate itself excluded. Result is sorted by
// percent descending (unknown percent last), then by bank and card label.
// A special candidate has no comparisons at all.
func ComparableOffers(candidate OfferView, pool []OfferView) []OfferView {
	if candidate.Kind == OfferSpecial {
		return nil
	}
	var out []OfferView
	for _, o := range pool {
		if o.OfferID == candidate.OfferID || o.Kind == OfferSpecial ||
			o.CurrencyKind != candidate.CurrencyKind {
			continue
		}
		out = append(out, o)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if c := cmpPercentDesc(out[i].Percent, out[j].Percent); c != 0 {
			return c < 0
		}
		if out[i].BankName != out[j].BankName {
			return out[i].BankName < out[j].BankName
		}
		return out[i].CardLabel < out[j].CardLabel
	})
	return out
}

// LookupEntry is one card's selection of the looked-up category (S3), with
// static tier-cap reference data passed through for display.
type LookupEntry struct {
	CardID         int64
	CardLabel      string
	HolderLabel    string // whose plastic («Мама»); empty = the owner
	BankName       string
	Percent        *decimal.Decimal
	CurrencyKind   CurrencyKind
	Kind           OfferKind
	Period         DateRange
	CapValue       *decimal.Decimal
	CapPerCategory *decimal.Decimal
	CapScope       CapScope
	PointsLabel    string // 'Баллы Плюс', 'баллы МКБ'; empty for rubles
}

// LookupResult is the S3 answer: ranked regular selections, special offers
// listed separately unranked (invariant 6), and base rates as the fallback
// («Остальное» — pays when no selected category matches).
type LookupResult struct {
	Ranked   []LookupEntry
	Special  []LookupEntry
	Fallback []LookupEntry
}

// RankActiveSelections filters entries to those whose period covers onDate,
// then ranks regular ones: grouped by currency (rub before points — groups
// are never compared to each other, invariant 5), percent descending within
// a group (unknown percent last), ties by bank name then card label.
// Special entries active on the date go to Special in input order.
func RankActiveSelections(onDate time.Time, entries []LookupEntry) LookupResult {
	var res LookupResult
	for _, e := range entries {
		if !e.Period.Contains(onDate) {
			continue
		}
		switch e.Kind {
		case OfferSpecial:
			res.Special = append(res.Special, e)
			continue
		case OfferBase:
			res.Fallback = append(res.Fallback, e)
			continue
		}
		res.Ranked = append(res.Ranked, e)
	}
	currencyOrder := func(k CurrencyKind) int {
		switch k {
		case CurrencyRub:
			return 0
		case CurrencyPoints:
			return 1
		default:
			return 2
		}
	}
	entryLess := func(s []LookupEntry) func(i, j int) bool {
		return func(i, j int) bool {
			a, b := s[i], s[j]
			if ca, cb := currencyOrder(a.CurrencyKind), currencyOrder(b.CurrencyKind); ca != cb {
				return ca < cb
			}
			if c := cmpPercentDesc(a.Percent, b.Percent); c != 0 {
				return c < 0
			}
			if a.BankName != b.BankName {
				return a.BankName < b.BankName
			}
			return a.CardLabel < b.CardLabel
		}
	}
	sort.SliceStable(res.Ranked, entryLess(res.Ranked))
	sort.SliceStable(res.Fallback, entryLess(res.Fallback))
	return res
}

// NormalizeTitle canonicalizes a bank's raw category title for alias
// matching: lower-case, trimmed, inner whitespace collapsed, ё→е.
func NormalizeTitle(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	return strings.Join(strings.Fields(s), " ")
}

// Alias is one bank_category_alias row (already filtered to a bank).
type Alias struct {
	CanonicalCategoryID int64
	RawTitle            string
}

// SuggestCanonical proposes a canonical category for a raw menu title by
// normalized-equality against known aliases. Returns (0, false) when the
// title is unknown (S1: the UI then offers to create a new alias inline).
func SuggestCanonical(rawTitle string, aliases []Alias) (int64, bool) {
	want := NormalizeTitle(rawTitle)
	for _, a := range aliases {
		if NormalizeTitle(a.RawTitle) == want {
			return a.CanonicalCategoryID, true
		}
	}
	return 0, false
}

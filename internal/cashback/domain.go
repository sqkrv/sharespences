// Package cashback implements the КБ (cashback) module: recording offer
// menus and selections per bank client (a person's relationship with one
// bank — all of the client's cards share the selection), the constraint
// helper, and the category-level lookup. Domain rules follow
// docs/specs/cashback.md (private meta-repo); invariant numbers in comments
// refer to its "Invariants" section.
package cashback

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"golang.org/x/text/unicode/norm"
)

// CurrencyKind is the currency a program pays КБ in. Helper comparisons and
// lookup ranking never cross currency kinds (invariant 5).
type CurrencyKind string

const (
	CurrencyRub    CurrencyKind = "rub"
	CurrencyPoints CurrencyKind = "points"
	// CurrencyUnknown marks bank clients without a resolvable program (no
	// tier set). Never compared with anything; listed as its own last group
	// in lookups.
	CurrencyUnknown CurrencyKind = "unknown"
)

// OfferKind separates three shapes of menu row (spec invariant 6):
//
//   - regular — a chosen menu category: consumes a tier slot, collides
//     across clients, ranks in lookup.
//   - super   — a full-period STACKING bonus that is a genuine best-card
//     candidate (the Альфа monthly барабан суперкэшбека: +1 category, whole
//     period, stacks with the monthly pick). Ranks like a regular, но — like
//     special — is granted, not chosen: no slot, no collision warning
//     (2026-07-15).
//   - special — a time-boxed / non-stacking / channel bonus (Альфа-Пятница,
//     Яндекс колесо, timed flash): granted, not chosen — no slot, no
//     collision, never a comparison candidate, never offered in S3b. It DOES
//     rank in lookup/overview since the invariant-6 amendment (2026-07-27:
//     a 7% special must not hide below a 5% regular); the UI
//     marks it «спец» with the offer's raw title and a «проверь условие»
//     caveat, because its condition (пятница, только в сервисе) is not
//     modelled.
//
// «За все покупки» is an ORDINARY regular row (2026-07-09): it takes a
// slot and collides like any category; it merely pays only when no other
// selected category matches — a display concern (the «Остальное» fallback),
// not a kind.
type OfferKind string

const (
	OfferRegular OfferKind = "regular"
	OfferSuper   OfferKind = "super"
	OfferSpecial OfferKind = "special"
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
	ErrInvalidPeriod = errors.New("конец периода раньше начала")
	// ErrPeriodOverlap — invariant 4: offer_period ranges for one bank client never overlap.
	ErrPeriodOverlap = errors.New("период пересекается с другим периодом этого банка")
	// ErrSlotsExhausted — invariant 1: selections per period ≤ tier.max_categories.
	ErrSlotsExhausted = errors.New("все слоты категорий по тарифу заняты")
	// ErrOutsidePeriod — invariant 2: selected_at date outside the offer's period.
	ErrOutsidePeriod = errors.New("дата выбора вне периода")
	// ErrAlreadySelected — an offer is selected at most once.
	ErrAlreadySelected = errors.New("категория уже выбрана")
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

// ValidateNewPeriod enforces invariant 4 for a bank client's new
// offer_period against its existing period ranges.
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
// Cross-client duplicates are NOT checked here — they are warnings, never
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
	ClientID            int64
	CanonicalCategoryID *int64
	Period              DateRange
	Kind                OfferKind
}

// ActiveSelection is an existing selection on some bank client of the same
// user, with display fields the warning message needs.
type ActiveSelection struct {
	ClientID            int64
	ClientLabel         string // «Альфа-Банк» / «Альфа-Банк · Мама»
	HolderLabel         string // держатель («Мама»); empty = the owner
	BankName            string
	CanonicalCategoryID *int64
	Period              DateRange
	Kind                OfferKind
	Percent             *decimal.Decimal
	CurrencyKind        CurrencyKind
	CapNote             string // static cap info for display, e.g. «лимит 1500₽/кат»
}

// Collision is a cross-client duplicate warning (invariant 3): advisory
// only. Same person duplicating a category across two banks, or two people
// at one bank, is deliberate and legal — both caps get filled.
type Collision struct {
	Other ActiveSelection
}

// DetectCollisions returns a warning per existing selection of the same
// canonical category on a DIFFERENT bank client with an overlapping period.
// Only regular offers collide: super and special are granted, not chosen, so
// a duplicate warning on them is noise (invariant 6). Offers without a
// canonical mapping cannot collide.
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
			o.ClientID == candidate.ClientID,
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
	ClientID     int64
	ClientLabel  string
	BankName     string
}

// ComparableOffers returns the pool rows the helper may show side by side
// with the candidate: same currency_kind only (invariant 5), kind=regular
// only (invariant 6 — granted super/special are not menu alternatives), the
// candidate itself excluded. Result is sorted by percent descending (unknown
// percent last), then by bank and client label. A super or special candidate
// has no comparisons at all.
func ComparableOffers(candidate OfferView, pool []OfferView) []OfferView {
	if candidate.Kind != OfferRegular {
		return nil
	}
	var out []OfferView
	for _, o := range pool {
		if o.OfferID == candidate.OfferID || o.Kind != OfferRegular ||
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
		return out[i].ClientLabel < out[j].ClientLabel
	})
	return out
}

// LookupEntry is one bank client's selection of the looked-up category (S3),
// with static tier-cap reference data passed through for display.
type LookupEntry struct {
	ClientID       int64
	ClientLabel    string
	HolderLabel    string // держатель («Мама»); empty = the owner
	BankName       string
	RawTitle       string // the bank's own menu title — names the mechanic on marked super/special rows («Пятница»)
	Percent        *decimal.Decimal
	CurrencyKind   CurrencyKind
	Kind           OfferKind
	Period         DateRange
	CapValue       *decimal.Decimal
	CapPerCategory *decimal.Decimal
	CapScope       CapScope
	OfferCapValue  *decimal.Decimal // per-offer cap (ВТБ «Кешбэк до N ₽» rows); wins over the tier cap in display
	PointsLabel    string           // 'Баллы Плюс', 'баллы МКБ'; empty for rubles
	// FriendName/FriendUsername mark a friend's shared card
	// (docs/specs/friends-sharing.md); empty = the viewer's own card. Friend
	// entries rank alongside own ones but never enter Available or the
	// fallback, and their cap fields are cleared before they get here
	// (invariant 4 — caps never serialize to a viewer).
	FriendName     string
	FriendUsername string
}

// MidPeriodAddPolicy mirrors cashback_program.mid_period_add (2026-07-16):
// whether a category can be ADDED mid-period. Deliberately not
// derived from SelectionMode — Альфа is atomic yet allows adds while a slot
// is free; ВТБ/Озон (also atomic) lock after the one-shot confirmation.
type MidPeriodAddPolicy string

const (
	AddAllowed          MidPeriodAddPolicy = "allowed"
	AddLockedAfterFirst MidPeriodAddPolicy = "locked_after_first"
	AddPaid             MidPeriodAddPolicy = "paid"
	AddUnknown          MidPeriodAddPolicy = "unknown"
)

// ActivationKind mirrors cashback_program.activation: when a fresh pick
// starts paying. МКБ = next_day, so «выбери перед покупкой» must warn there.
type ActivationKind string

const (
	ActivationImmediate ActivationKind = "immediate"
	ActivationNextDay   ActivationKind = "next_day"
	ActivationUnknown   ActivationKind = "unknown"
)

// AvailabilityVerdict is S3b's honest answer per offered-but-unselected row —
// a fact-based state, never a guess (spec S3b, 2026-07-16).
type AvailabilityVerdict string

const (
	AvailFree      AvailabilityVerdict = "free"       // pick it now
	AvailPaid      AvailabilityVerdict = "paid"       // bank charges for the change (МКБ)
	AvailLocked    AvailabilityVerdict = "locked"     // one-shot selection already confirmed
	AvailSlotsFull AvailabilityVerdict = "slots_full" // no free regular slot
	AvailUnknown   AvailabilityVerdict = "unknown"    // program policy not gathered
)

// AvailabilityCheck is the input for one offered-but-unselected menu row.
type AvailabilityCheck struct {
	Kind                 OfferKind
	Policy               MidPeriodAddPolicy
	HasRegularSelection  bool   // the client already confirmed picks this period
	MaxCategories        *int32 // effective limit (override ?? tier); nil = unknown → no slot check
	RegularSelectedCount int
}

// AssessAvailability decides whether the row can still be picked. super
// (барабан) is granted, slot-free and never locked — always markable. For
// regular rows the slot check comes first (invariant 1 would hard-reject the
// selection anyway), then the program's mid-period policy.
func AssessAvailability(c AvailabilityCheck) AvailabilityVerdict {
	if c.Kind == OfferSuper {
		return AvailFree
	}
	if c.MaxCategories != nil && c.RegularSelectedCount >= int(*c.MaxCategories) {
		return AvailSlotsFull
	}
	switch c.Policy {
	case AddAllowed:
		return AvailFree
	case AddPaid:
		return AvailPaid
	case AddLockedAfterFirst:
		if c.HasRegularSelection {
			return AvailLocked
		}
		return AvailFree
	default:
		return AvailUnknown
	}
}

// AvailableEntry is one S3b row: the menu offer, its verdict and the
// program's activation timing (next_day must be warned about).
type AvailableEntry struct {
	Entry      LookupEntry // carries the row's raw title
	OfferID    int64
	Verdict    AvailabilityVerdict
	Activation ActivationKind
}

// verdictOrder: actionable first (free, paid, unknown are things the user can
// still do), blocked after (slots_full before locked — freeing a slot is an
// action, a one-shot lock is final).
func verdictOrder(v AvailabilityVerdict) int {
	switch v {
	case AvailFree:
		return 0
	case AvailPaid:
		return 1
	case AvailUnknown:
		return 2
	case AvailSlotsFull:
		return 3
	default: // AvailLocked
		return 4
	}
}

// RankAvailable orders S3b rows: verdict actionability, then the ranked
// ordering within (currency group rub→points, percent desc with unknown
// last, bank name, client label).
func RankAvailable(entries []AvailableEntry) []AvailableEntry {
	out := append([]AvailableEntry(nil), entries...)
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
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if va, vb := verdictOrder(a.Verdict), verdictOrder(b.Verdict); va != vb {
			return va < vb
		}
		if ca, cb := currencyOrder(a.Entry.CurrencyKind), currencyOrder(b.Entry.CurrencyKind); ca != cb {
			return ca < cb
		}
		if c := cmpPercentDesc(a.Entry.Percent, b.Entry.Percent); c != 0 {
			return c < 0
		}
		if a.Entry.BankName != b.Entry.BankName {
			return a.Entry.BankName < b.Entry.BankName
		}
		return a.Entry.ClientLabel < b.Entry.ClientLabel
	})
	return out
}

// LookupResult is the S3 answer: the ranked selections. All three kinds rank
// since the invariant-6 amendment (2026-07-27) — kind rides along on
// each entry so the UI can mark барабан/спец rows.
type LookupResult struct {
	Ranked []LookupEntry
}

// RankActiveSelections filters entries to those whose period covers onDate,
// then ranks them: grouped by currency (rub before points — groups are never
// compared to each other, invariant 5), percent descending within a group
// (unknown percent last), ties by bank name then client label. Kind does not
// affect the order (amendment 2026-07-27); it only reaches the UI as a mark.
func RankActiveSelections(onDate time.Time, entries []LookupEntry) LookupResult {
	var res LookupResult
	for _, e := range entries {
		if !e.Period.Contains(onDate) {
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
			// On equal percent the own card wins — the app never sends the
			// user to a friend for nothing (friends-sharing invariant 7).
			if ownA, ownB := a.FriendName == "", b.FriendName == ""; ownA != ownB {
				return ownA
			}
			if a.BankName != b.BankName {
				return a.BankName < b.BankName
			}
			return a.ClientLabel < b.ClientLabel
		}
	}
	sort.SliceStable(res.Ranked, entryLess(res.Ranked))
	return res
}

// homoglyphs maps the Latin letters that are pixel-identical to Cyrillic
// ones onto their Cyrillic twins. Real Альфа menu titles mix them into
// Cyrillic words («Кафе и pестораны», «Цвeты»). Applied to both sides of
// every comparison, so genuinely-Latin titles still match each other.
//
// The set is the full uppercase-identical alphabet (А В Е К М Н О Р С Т У Х),
// not just the letters that also look identical in lowercase: NormalizeTitle
// lower-cases before folding, so an uppercase Latin «Т» reaches this replacer
// as «t» with its origin already erased. Screenshot extraction returns
// exactly that — «T-Страхование» with a Latin T (recognizer benchmark,
// 2026-07-27) — and a title-cased word is where a stray Latin letter is
// most likely to sit.
var homoglyphs = strings.NewReplacer(
	"a", "а", "b", "в", "c", "с", "e", "е", "h", "н", "k", "к",
	"m", "м", "o", "о", "p", "р", "t", "т", "x", "х", "y", "у",
)

// NormalizeTitle canonicalizes a bank's raw category title for alias
// matching: NFC-composed (Альфа sends «й» decomposed), lower-case, trimmed,
// inner whitespace collapsed, ё→е, Latin homoglyphs folded to Cyrillic.
// The normalized form is compared, never stored or displayed.
func NormalizeTitle(s string) string {
	s = norm.NFC.String(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	s = homoglyphs.Replace(s)
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

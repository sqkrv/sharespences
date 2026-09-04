// Package perks owns Привилегии (docs/specs/perks.md): quota'd bank perks —
// такси compensations, бизнес-залы, преференции — as dated windows plus a
// ledger of events, reconciled against snapshots of the bank's own counter.
//
// This file is the whole of the module's arithmetic and its invariants, with
// no storage in sight. The counters are DERIVED on every read (never stored):
// a bank's counter is opaque and authoritative, so the app's number is only
// ever a claim about it, and the moment the two disagree the disagreement is
// what the user needs to see — not a silently corrected total.
package perks

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// User-facing sentinel errors. Russian at the source and without a package
// prefix: the text lands verbatim in the response detail.
var (
	// ErrNotFound covers rows that don't exist and rows belonging to another
	// user — scoping never reveals which (spec invariant 1).
	ErrNotFound          = errors.New("не найдено")
	ErrPerkHasQuotas     = errors.New("у привилегии есть периоды — сначала удали их")
	ErrPerkExists        = errors.New("такая привилегия у этого банка уже есть")
	ErrNameLength        = errors.New("название: от 1 до 64 символов")
	ErrUnitLength        = errors.New("единица: от 1 до 32 символов")
	ErrWindowOrder       = errors.New("начало периода не может быть позже конца")
	ErrSizeNegative      = errors.New("размер не может быть отрицательным")
	ErrNestingTooDeep    = errors.New("вложенность только двухуровневая: у периода-родителя не может быть своего родителя")
	ErrChildOutsideParen = errors.New("период должен помещаться внутрь родительского")
	ErrChildMismatch     = errors.New("период и родительский период должны принадлежать одной привилегии")
	ErrSizeLocked        = errors.New("у периода уже есть история — меняйте размер событием «resize»")
	ErrQtyPositive       = errors.New("количество должно быть больше нуля")
	ErrQtyNonNegative    = errors.New("новый размер не может быть отрицательным")
	ErrQtyNonZero        = errors.New("корректировка на ноль ничего не меняет")
	ErrRemainingNegative = errors.New("остаток по счётчику банка не может быть отрицательным")
	ErrUnknownKind       = errors.New("неизвестный вид события")
)

// Kind is what a ledger row does to its window. The DB has the same four
// labels as an enum; the two are cast at the storage seam rather than unified,
// exactly as cashback keeps its own OfferKind (CLAUDE.md, known traps).
type Kind string

const (
	// KindUse is a claim the bank compensated — the only kind that burns two
	// levels, because a monthly allowance is spent out of the annual pool.
	KindUse Kind = "use"
	// KindGrant is an allowance that arrived outside the schedule («подарили
	// поездку»). It lands on the window it was given for and nowhere else:
	// whether the bank also charged the pool is not guessable, so the next
	// snapshot answers it and an adjust records the answer.
	KindGrant Kind = "grant"
	// KindResize is a re-rating: qty is the window's NEW absolute size, not a
	// delta. Balance tiers move a quota between months and the bank re-rates
	// the annual pool mid-year, so «size» is a dated fact, not a constant.
	KindResize Kind = "resize"
	// KindAdjust is signed reconciliation against a snapshot — the one
	// operation that closes a discrepancy, always with a note saying why.
	KindAdjust Kind = "adjust"
)

// ValidKind reports whether s is one of the four labels. The HTTP layer's enum
// tag rejects the rest first; this keeps the domain honest on its own terms.
func ValidKind(s string) bool {
	switch Kind(s) {
	case KindUse, KindGrant, KindResize, KindAdjust:
		return true
	}
	return false
}

// Quota is the arithmetic's view of a window: an identity, a level, and the
// size the window opened at. Dates and labels live in the DB row.
type Quota struct {
	ID       int64
	ParentID *int64
	Size     int
}

// IsChild reports whether the quota is a monthly sub-allowance rather than a
// pool of its own.
func (q Quota) IsChild() bool { return q.ParentID != nil }

// Event is one ledger row. ParentID is the parent of the quota the row sits
// on, carried alongside so a use can be attributed to both its levels without
// a second lookup.
type Event struct {
	ID       int64
	QuotaID  int64
	ParentID *int64
	Kind     Kind
	Qty      int
	Date     time.Time
}

// Snapshot is what the bank's app displayed on a date.
type Snapshot struct {
	ID         int64
	QuotaID    int64
	ObservedOn time.Time
	Remaining  int
}

// onOrBefore is the ledger's cutoff everywhere: an event dated d counts on d.
func onOrBefore(event, asOf time.Time) bool { return !event.After(asOf) }

// EffectiveSize is the window's size as of a date: the last re-rating (or the
// size it opened at) plus everything granted or adjusted on it.
//
// Grants and adjusts stack on top of a resize regardless of their order in
// time — a gift is an extra allowance, and withdrawing the scheduled quota
// («не выполнил условия», resize 0) does not take the gift back with it. When
// the bank does take it back, that is an adjust of its own, with the note that
// explains it.
func EffectiveSize(q Quota, events []Event, asOf time.Time) int {
	size := q.Size
	var resizeAt time.Time
	var resizeID int64
	var extra int
	for _, e := range events {
		if e.QuotaID != q.ID || !onOrBefore(e.Date, asOf) {
			continue
		}
		switch e.Kind {
		case KindResize:
			// Latest wins; same-day re-ratings fall back to insert order.
			if resizeID == 0 || e.Date.After(resizeAt) || (e.Date.Equal(resizeAt) && e.ID > resizeID) {
				size, resizeAt, resizeID = e.Qty, e.Date, e.ID
			}
		case KindGrant, KindAdjust:
			extra += e.Qty
		case KindUse:
		}
	}
	return size + extra
}

// Consumed is what has been claimed against the window as of a date. A use
// recorded on a monthly window burns the annual pool too, so a pool counts its
// own uses and its children's; a monthly window counts only its own.
func Consumed(q Quota, events []Event, asOf time.Time) int {
	var used int
	for _, e := range events {
		if e.Kind != KindUse || !onOrBefore(e.Date, asOf) {
			continue
		}
		own := e.QuotaID == q.ID
		viaChild := e.ParentID != nil && *e.ParentID == q.ID
		if own || viaChild {
			used += e.Qty
		}
	}
	return used
}

// Remaining is the app's claim about the bank's counter.
//
// It may go negative, and nothing in the module refuses an operation for
// «insufficient quota» (spec invariant 4). A negative number is not a bug to
// be clamped away: it means the ledger and the bank have drifted, which is the
// state the user opened the screen to find out about.
func Remaining(q Quota, events []Event, asOf time.Time) int {
	return EffectiveSize(q, events, asOf) - Consumed(q, events, asOf)
}

// Discrepancy is a snapshot that disagrees with the ledger. Delta is the
// bank's number minus the computed one: negative means the bank has already
// spent something the app has not recorded.
type Discrepancy struct {
	SnapshotID int64
	Delta      int
	Computed   int
	Bank       int
	ObservedOn time.Time
}

// CheckSnapshot compares the latest snapshot of a window against the ledger as
// it stood on the day that snapshot was taken. nil means «no reading yet» or
// «they agree» — the badge is absent in both cases.
//
// Reconciling against the snapshot's own date, not today, is what makes the
// badge stable: a use recorded after the reading is not a disagreement, it is
// simply later news.
func CheckSnapshot(q Quota, events []Event, snap *Snapshot) *Discrepancy {
	if snap == nil {
		return nil
	}
	computed := Remaining(q, events, snap.ObservedOn)
	if computed == snap.Remaining {
		return nil
	}
	return &Discrepancy{
		SnapshotID: snap.ID,
		Delta:      snap.Remaining - computed, Computed: computed,
		Bank:       snap.Remaining, ObservedOn: snap.ObservedOn,
	}
}

// NormalizeName trims a perk's name or unit. Deliberately nothing more: the
// uniqueness rule is one perk name per bank per user, a list short enough that
// the user reads it, so the homoglyph folding the cashback catalog needs for
// alias matching would only surprise someone naming a perk in Latin.
func NormalizeName(s string) string { return strings.TrimSpace(s) }

// ValidatePerkName bounds a perk's name. Runes, not bytes: every real name
// here is Cyrillic, where a byte bound would cut the limit in half.
func ValidatePerkName(name string) error {
	if n := utf8.RuneCountInString(name); n < 1 || n > 64 {
		return ErrNameLength
	}
	return nil
}

// ValidatePerkUnit bounds the counted noun.
func ValidatePerkUnit(unit string) error {
	if n := utf8.RuneCountInString(unit); n < 1 || n > 32 {
		return ErrUnitLength
	}
	return nil
}

// ValidatePerk checks the two typed fields of a perk definition.
func ValidatePerk(name, unit string) error {
	if err := ValidatePerkName(name); err != nil {
		return err
	}
	return ValidatePerkUnit(unit)
}

// Window is a quota's dated span. Both ends are typed in by the user, never
// derived: ВТБ ran its monthly window 20th→19th until 2026-07-31 and 1st→EOM
// after, so a rule that computes either end is already wrong about the recent
// past.
type Window struct {
	Start time.Time
	End   time.Time
}

// Contains reports whether the window covers a date, both ends inclusive.
func (w Window) Contains(d time.Time) bool {
	return !d.Before(w.Start) && !d.After(w.End)
}

// Within reports whether w fits entirely inside outer.
func (w Window) Within(outer Window) bool {
	return !w.Start.Before(outer.Start) && !w.End.After(outer.End)
}

// ValidateWindow checks a window's own shape.
func ValidateWindow(w Window, size int) error {
	if w.End.Before(w.Start) {
		return ErrWindowOrder
	}
	if size < 0 {
		return ErrSizeNegative
	}
	return nil
}

// ParentQuota is what a child needs to know about its prospective parent.
type ParentQuota struct {
	Quota
	Window Window
	PerkID int64
}

// ValidateChild enforces spec invariant 2 — the shape of a two-level quota.
// The DB backstops both through a composite self-FK (00024, narrowed by 00025
// once the держатель moved onto the perk); this is where they get a Russian
// sentence instead of a constraint name.
func ValidateChild(w Window, perkID int64, parent ParentQuota) error {
	if parent.IsChild() {
		return ErrNestingTooDeep
	}
	if parent.PerkID != perkID {
		return ErrChildMismatch
	}
	if !w.Within(parent.Window) {
		return ErrChildOutsideParen
	}
	return nil
}

// ValidateEventQty gives each kind the bound its qty actually means. An adjust
// of zero is the one worth naming: it would read as a recorded correction
// while changing nothing.
func ValidateEventQty(kind Kind, qty int) error {
	switch kind {
	case KindUse, KindGrant:
		if qty <= 0 {
			return ErrQtyPositive
		}
	case KindResize:
		if qty < 0 {
			return ErrQtyNonNegative
		}
	case KindAdjust:
		if qty == 0 {
			return ErrQtyNonZero
		}
	default:
		return ErrUnknownKind
	}
	return nil
}

// ValidateSnapshot checks a reading of the bank's counter. The bank's own
// display floors at zero, so a negative reading is a typo — it is the app's
// remaining that is allowed to go negative, not the bank's.
func ValidateSnapshot(remaining int) error {
	if remaining < 0 {
		return ErrRemainingNegative
	}
	return nil
}

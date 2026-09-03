package perks

import (
	"errors"
	"testing"
	"time"
)

func date(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func win(start, end string) Window { return Window{Start: date(start), End: date(end)} }

// The wallet's actual shape (docs/knowledge/concepts/bank-perks.md): an annual
// pool of 15 rides with a monthly sub-allowance of 3 inside it.
func alfaYear() (Quota, Quota) {
	annual := Quota{ID: 1, Size: 15}
	monthly := Quota{ID: 2, ParentID: &annual.ID, Size: 3}
	return annual, monthly
}

func use(id, quota int64, parent *int64, qty int, on string) Event {
	return Event{ID: id, QuotaID: quota, ParentID: parent, Kind: KindUse, Qty: qty, Date: date(on)}
}

func TestRemainingBurnsBothLevels(t *testing.T) {
	annual, monthly := alfaYear()
	// PV-S2: two compensated rides on the monthly window.
	events := []Event{
		use(1, monthly.ID, monthly.ParentID, 1, "2026-08-04"),
		use(2, monthly.ID, monthly.ParentID, 1, "2026-08-11"),
	}
	asOf := date("2026-08-31")

	if got := Remaining(monthly, events, asOf); got != 1 {
		t.Errorf("monthly remaining = %d, want 1 (3 − 2)", got)
	}
	if got := Remaining(annual, events, asOf); got != 13 {
		t.Errorf("annual remaining = %d, want 13 (15 − 2): a leaf use burns the pool too", got)
	}
	if got := Consumed(annual, events, asOf); got != 2 {
		t.Errorf("annual consumed = %d, want 2", got)
	}
}

func TestUseOnPoolDirectly(t *testing.T) {
	// A bank with no sub-limit takes uses on the pool itself.
	pool := Quota{ID: 7, Size: 12}
	events := []Event{use(1, pool.ID, nil, 3, "2026-03-02")}
	if got := Remaining(pool, events, date("2026-03-31")); got != 9 {
		t.Errorf("remaining = %d, want 9", got)
	}
}

func TestSiblingUsesDoNotLeak(t *testing.T) {
	annual, august := alfaYear()
	julyID := int64(3)
	july := Quota{ID: julyID, ParentID: &annual.ID, Size: 3}
	events := []Event{
		use(1, july.ID, july.ParentID, 3, "2026-07-15"),
		use(2, august.ID, august.ParentID, 1, "2026-08-04"),
	}
	asOf := date("2026-08-31")

	if got := Remaining(august, events, asOf); got != 2 {
		t.Errorf("august remaining = %d, want 2 — July's uses are its own", got)
	}
	if got := Remaining(annual, events, asOf); got != 11 {
		t.Errorf("annual remaining = %d, want 11 (15 − 4): the pool counts every child", got)
	}
}

func TestAsOfCutoffIsInclusive(t *testing.T) {
	_, monthly := alfaYear()
	events := []Event{use(1, monthly.ID, monthly.ParentID, 1, "2026-08-10")}

	if got := Remaining(monthly, events, date("2026-08-10")); got != 2 {
		t.Errorf("remaining on the event's own day = %d, want 2 — the cutoff includes it", got)
	}
	if got := Remaining(monthly, events, date("2026-08-09")); got != 3 {
		t.Errorf("remaining the day before = %d, want 3 — later events are invisible", got)
	}
}

// PV-S3: Альфа gifted a ride and the month rendered as x/4.
func TestGrantRaisesOnlyItsOwnWindow(t *testing.T) {
	annual, monthly := alfaYear()
	grant := Event{ID: 1, QuotaID: monthly.ID, ParentID: monthly.ParentID, Kind: KindGrant, Qty: 1, Date: date("2026-09-29")}
	asOf := date("2026-09-30")

	if got := EffectiveSize(monthly, []Event{grant}, asOf); got != 4 {
		t.Errorf("monthly size = %d, want 4 (3 + подарок)", got)
	}
	if got := EffectiveSize(annual, []Event{grant}, asOf); got != 15 {
		t.Errorf("annual size = %d, want 15 — whether the bank also charged the pool is not guessed", got)
	}
}

// PV-S5: «не выполнил условия» — the month is re-rated to nothing.
func TestResizeSetsAbsoluteSize(t *testing.T) {
	annual, _ := alfaYear()
	events := []Event{
		{ID: 1, QuotaID: annual.ID, Kind: KindResize, Qty: 12, Date: date("2026-04-01")},
		use(2, annual.ID, nil, 2, "2026-05-06"),
	}
	asOf := date("2026-06-01")

	if got := EffectiveSize(annual, events, asOf); got != 12 {
		t.Errorf("effective size = %d, want 12 — resize is absolute, not a delta", got)
	}
	if got := Remaining(annual, events, asOf); got != 10 {
		t.Errorf("remaining = %d, want 10", got)
	}
	// Before the re-rating landed, the window was still 15.
	if got := EffectiveSize(annual, events, date("2026-03-31")); got != 15 {
		t.Errorf("effective size before the resize = %d, want 15", got)
	}
}

func TestLatestResizeWins(t *testing.T) {
	annual, _ := alfaYear()
	events := []Event{
		{ID: 1, QuotaID: annual.ID, Kind: KindResize, Qty: 12, Date: date("2026-04-01")},
		{ID: 2, QuotaID: annual.ID, Kind: KindResize, Qty: 2, Date: date("2026-06-01")},
	}
	if got := EffectiveSize(annual, events, date("2026-12-31")); got != 2 {
		t.Errorf("effective size = %d, want 2 — the later re-rating wins", got)
	}
	// Same day, two re-ratings: insert order breaks the tie.
	same := []Event{
		{ID: 5, QuotaID: annual.ID, Kind: KindResize, Qty: 2, Date: date("2026-06-01")},
		{ID: 6, QuotaID: annual.ID, Kind: KindResize, Qty: 30, Date: date("2026-06-01")},
	}
	if got := EffectiveSize(annual, same, date("2026-06-01")); got != 30 {
		t.Errorf("effective size = %d, want 30 — the row entered last wins a same-day tie", got)
	}
}

func TestGrantSurvivesALaterResize(t *testing.T) {
	_, monthly := alfaYear()
	events := []Event{
		{ID: 1, QuotaID: monthly.ID, ParentID: monthly.ParentID, Kind: KindGrant, Qty: 1, Date: date("2026-08-05")},
		{ID: 2, QuotaID: monthly.ID, ParentID: monthly.ParentID, Kind: KindResize, Qty: 0, Date: date("2026-08-20")},
	}
	// Withdrawing the scheduled allowance does not take the gift back; when
	// the bank does, that is an adjust of its own.
	if got := EffectiveSize(monthly, events, date("2026-08-31")); got != 1 {
		t.Errorf("effective size = %d, want 1 — the gift stacks on the re-rated base", got)
	}
}

func TestRemainingGoesNegative(t *testing.T) {
	_, monthly := alfaYear()
	events := []Event{use(1, monthly.ID, monthly.ParentID, 5, "2026-08-04")}
	// Spec invariant 4: nothing is refused for «insufficient quota».
	if got := Remaining(monthly, events, date("2026-08-31")); got != -2 {
		t.Errorf("remaining = %d, want −2 — drift is shown, not clamped", got)
	}
}

func TestCheckSnapshot(t *testing.T) {
	annual, _ := alfaYear()
	events := []Event{use(1, annual.ID, nil, 4, "2026-05-06")}

	t.Run("no reading yet", func(t *testing.T) {
		if d := CheckSnapshot(annual, events, nil); d != nil {
			t.Errorf("got %+v, want nil", d)
		}
	})

	t.Run("agrees", func(t *testing.T) {
		snap := &Snapshot{QuotaID: annual.ID, ObservedOn: date("2026-05-10"), Remaining: 11}
		if d := CheckSnapshot(annual, events, snap); d != nil {
			t.Errorf("got %+v, want nil — 15 − 4 = 11", d)
		}
	})

	// PV-S3: computed says 11, the bank shows 10.
	t.Run("disagrees", func(t *testing.T) {
		snap := &Snapshot{QuotaID: annual.ID, ObservedOn: date("2026-05-10"), Remaining: 10}
		d := CheckSnapshot(annual, events, snap)
		if d == nil {
			t.Fatal("want a discrepancy, got nil")
		}
		if d.Delta != -1 || d.Computed != 11 || d.Bank != 10 {
			t.Errorf("got %+v, want delta −1, computed 11, bank 10", d)
		}
	})

	t.Run("adjust closes it", func(t *testing.T) {
		closed := append([]Event{}, events...)
		closed = append(closed, Event{
			ID: 2, QuotaID: annual.ID, Kind: KindAdjust, Qty: -1, Date: date("2026-05-10"),
		})
		snap := &Snapshot{QuotaID: annual.ID, ObservedOn: date("2026-05-10"), Remaining: 10}
		if d := CheckSnapshot(annual, closed, snap); d != nil {
			t.Errorf("got %+v, want nil — the adjust reconciles the gap", d)
		}
	})

	// The badge is judged as of the reading's own date, so recording a use
	// afterwards is later news, not a disagreement.
	t.Run("a later use does not reopen it", func(t *testing.T) {
		later := append([]Event{}, events...)
		later = append(later, use(3, annual.ID, nil, 1, "2026-05-20"))
		snap := &Snapshot{QuotaID: annual.ID, ObservedOn: date("2026-05-10"), Remaining: 11}
		if d := CheckSnapshot(annual, later, snap); d != nil {
			t.Errorf("got %+v, want nil", d)
		}
	})
}

func TestValidateChild(t *testing.T) {
	annualID := int64(1)
	parent := ParentQuota{
		Quota:  Quota{ID: annualID, Size: 15},
		Window: win("2026-01-01", "2026-12-31"), PerkID: 10,
	}

	cases := []struct {
		name   string
		w      Window
		perk   int64
		parent ParentQuota
		want   error
	}{
		{"inside", win("2026-08-01", "2026-08-31"), 10, parent, nil},
		{"flush with both ends", win("2026-01-01", "2026-12-31"), 10, parent, nil},
		{"starts before the pool", win("2025-12-20", "2026-01-19"), 10, parent, ErrChildOutsideParen},
		{"ends after the pool", win("2026-12-20", "2027-01-19"), 10, parent, ErrChildOutsideParen},
		// The держатель moved onto the perk (00025), so a foreign perk is the
		// only mismatch left to catch here.
		{"another perk", win("2026-08-01", "2026-08-31"), 11, parent, ErrChildMismatch},
		{"parent is itself a child", win("2026-08-01", "2026-08-31"), 10, ParentQuota{
			Quota:  Quota{ID: 2, ParentID: &annualID, Size: 3},
			Window: win("2026-01-01", "2026-12-31"), PerkID: 10,
		}, ErrNestingTooDeep},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateChild(c.w, c.perk, c.parent); !errors.Is(err, c.want) {
				t.Errorf("ValidateChild = %v, want %v", err, c.want)
			}
		})
	}
}

func TestValidateWindow(t *testing.T) {
	if err := ValidateWindow(win("2026-08-31", "2026-08-01"), 3); !errors.Is(err, ErrWindowOrder) {
		t.Errorf("reversed window = %v, want ErrWindowOrder", err)
	}
	// ВТБ's window ran 20th→19th across a month boundary for nine months.
	if err := ValidateWindow(win("2026-06-20", "2026-07-19"), 2); err != nil {
		t.Errorf("cross-month window = %v, want nil", err)
	}
	if err := ValidateWindow(win("2026-08-01", "2026-08-31"), -1); !errors.Is(err, ErrSizeNegative) {
		t.Errorf("negative size = %v, want ErrSizeNegative", err)
	}
	// A window re-rated to nothing is a legitimate size.
	if err := ValidateWindow(win("2026-08-01", "2026-08-31"), 0); err != nil {
		t.Errorf("zero size = %v, want nil", err)
	}
}

func TestValidateEventQty(t *testing.T) {
	cases := []struct {
		kind Kind
		qty  int
		want error
	}{
		{KindUse, 1, nil},
		{KindUse, 0, ErrQtyPositive},
		{KindUse, -1, ErrQtyPositive},
		{KindGrant, 1, nil},
		{KindGrant, 0, ErrQtyPositive},
		{KindResize, 0, nil},
		{KindResize, 12, nil},
		{KindResize, -1, ErrQtyNonNegative},
		{KindAdjust, -1, nil},
		{KindAdjust, 2, nil},
		{KindAdjust, 0, ErrQtyNonZero},
		{Kind("burn"), 1, ErrUnknownKind},
	}
	for _, c := range cases {
		if err := ValidateEventQty(c.kind, c.qty); !errors.Is(err, c.want) {
			t.Errorf("ValidateEventQty(%q, %d) = %v, want %v", c.kind, c.qty, err, c.want)
		}
	}
}

func TestValidatePerk(t *testing.T) {
	if err := ValidatePerk("Компенсация такси", "поездка"); err != nil {
		t.Errorf("valid perk = %v, want nil", err)
	}
	if err := ValidatePerk("", "поездка"); !errors.Is(err, ErrNameLength) {
		t.Errorf("empty name = %v, want ErrNameLength", err)
	}
	// Bounds count runes, not bytes — every real name here is Cyrillic.
	long := ""
	for range 65 {
		long += "я"
	}
	if err := ValidatePerk(long, "поездка"); !errors.Is(err, ErrNameLength) {
		t.Errorf("65 runes = %v, want ErrNameLength", err)
	}
	if err := ValidatePerk(long[:len(long)-2], "поездка"); err != nil {
		t.Errorf("64 runes = %v, want nil", err)
	}
	if err := ValidatePerk("Преференции", ""); !errors.Is(err, ErrUnitLength) {
		t.Errorf("empty unit = %v, want ErrUnitLength", err)
	}
}

func TestValidateSnapshot(t *testing.T) {
	if err := ValidateSnapshot(0); err != nil {
		t.Errorf("zero = %v, want nil", err)
	}
	if err := ValidateSnapshot(-1); !errors.Is(err, ErrRemainingNegative) {
		t.Errorf("negative reading = %v, want ErrRemainingNegative", err)
	}
}

func TestWindowContains(t *testing.T) {
	w := win("2026-08-01", "2026-08-31")
	for _, c := range []struct {
		d    string
		want bool
	}{
		{"2026-07-31", false}, {"2026-08-01", true},
		{"2026-08-15", true}, {"2026-08-31", true}, {"2026-09-01", false},
	} {
		if got := w.Contains(date(c.d)); got != c.want {
			t.Errorf("Contains(%s) = %v, want %v", c.d, got, c.want)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	if got := NormalizeName("  Компенсация такси \n"); got != "Компенсация такси" {
		t.Errorf("NormalizeName = %q", got)
	}
}

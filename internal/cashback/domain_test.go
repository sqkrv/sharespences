package cashback

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func pct(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func catID(id int64) *int64 { return &id }

func maxCats(n int32) *int32 { return &n }

var (
	july2026   = DateRange{Start: Date(2026, time.July, 1), End: Date(2026, time.July, 31)}
	august2026 = DateRange{Start: Date(2026, time.August, 1), End: Date(2026, time.August, 31)}
	june2026   = DateRange{Start: Date(2026, time.June, 1), End: Date(2026, time.June, 30)}
	q3_2026    = DateRange{Start: Date(2026, time.July, 1), End: Date(2026, time.September, 30)}
)

// Bank clients (person × bank): the same user's relationships with three
// banks. Cards do not appear here — selections are keyed by client.
const (
	clientAlfa int64 = 1
	clientOzon int64 = 2
	clientMKB  int64 = 3
)

// supermarkets is the canonical category shared by Альфа-Банк «Супермаркеты»
// and Озон «Продукты» in the E2E scenario.
const supermarketsID int64 = 10

func alfaSupermarkets() ActiveSelection {
	return ActiveSelection{
		ClientID:            clientAlfa,
		ClientLabel:         "Альфа-Банк",
		BankName:            "Альфа-Банк",
		CanonicalCategoryID: catID(supermarketsID),
		Period:              july2026,
		Kind:                OfferRegular,
		Percent:             pct("5"),
		CurrencyKind:        CurrencyRub,
		CapNote:             "лимит 7000₽",
	}
}

// TestDetectCollisions_MainScenario is the spec's E2E step 3 at unit level:
// Супермаркеты already selected on Альфа-Банк; selecting Озон «Продукты»
// (mapped to the same canonical category, overlapping July periods) must
// produce a warning naming Альфа-Банк — a warning, never a block.
func TestDetectCollisions_MainScenario(t *testing.T) {
	candidate := CandidateSelection{
		ClientID:            clientOzon,
		CanonicalCategoryID: catID(supermarketsID),
		Period:              july2026,
		Kind:                OfferRegular,
	}

	got := DetectCollisions(candidate, []ActiveSelection{alfaSupermarkets()})

	if len(got) != 1 {
		t.Fatalf("DetectCollisions() returned %d collisions, want exactly 1", len(got))
	}
	if got[0].Other.BankName != "Альфа-Банк" {
		t.Errorf("collision names bank %q, want %q", got[0].Other.BankName, "Альфа-Банк")
	}
	if got[0].Other.CapNote != "лимит 7000₽" {
		t.Errorf("collision cap note %q, want pass-through %q", got[0].Other.CapNote, "лимит 7000₽")
	}
}

func TestDetectCollisions(t *testing.T) {
	base := CandidateSelection{
		ClientID:            clientOzon,
		CanonicalCategoryID: catID(supermarketsID),
		Period:              july2026,
		Kind:                OfferRegular,
	}

	tests := []struct {
		name      string
		candidate CandidateSelection
		others    []ActiveSelection
		want      int
	}{
		{
			name:      "same category, different client, overlapping period → warn",
			candidate: base,
			others:    []ActiveSelection{alfaSupermarkets()},
			want:      1,
		},
		{
			name:      "different canonical category → no warning",
			candidate: base,
			others: []ActiveSelection{func() ActiveSelection {
				s := alfaSupermarkets()
				s.CanonicalCategoryID = catID(99)
				return s
			}()},
			want: 0,
		},
		{
			name: "same client → no warning (a second card of the same client is still the same client)",
			candidate: CandidateSelection{
				ClientID:            clientAlfa,
				CanonicalCategoryID: catID(supermarketsID),
				Period:              july2026,
				Kind:                OfferRegular,
			},
			others: []ActiveSelection{alfaSupermarkets()},
			want:   0,
		},
		{
			name: "non-overlapping periods → no warning",
			candidate: CandidateSelection{
				ClientID:            clientOzon,
				CanonicalCategoryID: catID(supermarketsID),
				Period:              august2026,
				Kind:                OfferRegular,
			},
			others: []ActiveSelection{func() ActiveSelection {
				s := alfaSupermarkets()
				s.Period = june2026
				return s
			}()},
			want: 0,
		},
		{
			name: "quarter (МКБ) overlaps month → warn (S5: overlap on date ranges)",
			candidate: CandidateSelection{
				ClientID:            clientMKB,
				CanonicalCategoryID: catID(supermarketsID),
				Period:              q3_2026,
				Kind:                OfferRegular,
			},
			others: []ActiveSelection{alfaSupermarkets()},
			want:   1,
		},
		{
			name: "already-entered NEXT period collides too (S1)",
			candidate: CandidateSelection{
				ClientID:            clientOzon,
				CanonicalCategoryID: catID(supermarketsID),
				Period:              august2026,
				Kind:                OfferRegular,
			},
			others: []ActiveSelection{func() ActiveSelection {
				s := alfaSupermarkets()
				s.Period = august2026
				return s
			}()},
			want: 1,
		},
		{
			name:      "candidate without canonical mapping → no warning",
			candidate: CandidateSelection{ClientID: clientOzon, Period: july2026, Kind: OfferRegular},
			others:    []ActiveSelection{alfaSupermarkets()},
			want:      0,
		},
		{
			name:      "other without canonical mapping → no warning",
			candidate: base,
			others: []ActiveSelection{func() ActiveSelection {
				s := alfaSupermarkets()
				s.CanonicalCategoryID = nil
				return s
			}()},
			want: 0,
		},
		{
			name: "special candidate → excluded from helper math (invariant 6)",
			candidate: CandidateSelection{
				ClientID:            clientOzon,
				CanonicalCategoryID: catID(supermarketsID),
				Period:              july2026,
				Kind:                OfferSpecial,
			},
			others: []ActiveSelection{alfaSupermarkets()},
			want:   0,
		},
		{
			name:      "special existing selection → excluded (invariant 6)",
			candidate: base,
			others: []ActiveSelection{func() ActiveSelection {
				s := alfaSupermarkets()
				s.Kind = OfferSpecial
				return s
			}()},
			want: 0,
		},
		{
			name:      "two colliding clients → two warnings",
			candidate: base,
			others: []ActiveSelection{alfaSupermarkets(), func() ActiveSelection {
				s := alfaSupermarkets()
				s.ClientID = clientMKB
				s.BankName = "МКБ"
				s.Period = q3_2026
				return s
			}()},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectCollisions(tt.candidate, tt.others)
			if len(got) != tt.want {
				t.Errorf("DetectCollisions() = %d collisions, want %d", len(got), tt.want)
			}
		})
	}
}

// TestValidateSelection covers invariants 1 and 2 (hard rejects) plus the
// offer-selected-at-most-once rule. E2E step 4 (5th selection on a 4-slot
// tier) is the "slots exhausted" case.
func TestValidateSelection(t *testing.T) {
	tests := []struct {
		name    string
		check   SelectionCheck
		wantErr error
	}{
		{
			name: "slot available → ok",
			check: SelectionCheck{
				Period:               july2026,
				SelectedAt:           Date(2026, time.July, 10),
				OfferKind:            OfferRegular,
				MaxCategories:        maxCats(4),
				RegularSelectedCount: 3,
			},
			wantErr: nil,
		},
		{
			name: "slots exhausted → hard reject (invariant 1, E2E step 4)",
			check: SelectionCheck{
				Period:               july2026,
				SelectedAt:           Date(2026, time.July, 10),
				OfferKind:            OfferRegular,
				MaxCategories:        maxCats(4),
				RegularSelectedCount: 4,
			},
			wantErr: ErrSlotsExhausted,
		},
		{
			name: "no tier limit set → no slot check",
			check: SelectionCheck{
				Period:               july2026,
				SelectedAt:           Date(2026, time.July, 10),
				OfferKind:            OfferRegular,
				MaxCategories:        nil,
				RegularSelectedCount: 12,
			},
			wantErr: nil,
		},
		{
			name: "special selection never consumes slots (owner decision)",
			check: SelectionCheck{
				Period:               july2026,
				SelectedAt:           Date(2026, time.July, 10),
				OfferKind:            OfferSpecial,
				MaxCategories:        maxCats(4),
				RegularSelectedCount: 4,
			},
			wantErr: nil,
		},
		{
			name: "selected_at before period → hard reject (invariant 2)",
			check: SelectionCheck{
				Period:        july2026,
				SelectedAt:    Date(2026, time.June, 28),
				OfferKind:     OfferRegular,
				MaxCategories: maxCats(4),
			},
			wantErr: ErrOutsidePeriod,
		},
		{
			name: "selected_at after period → hard reject (invariant 2)",
			check: SelectionCheck{
				Period:        july2026,
				SelectedAt:    Date(2026, time.August, 5),
				OfferKind:     OfferRegular,
				MaxCategories: maxCats(4),
			},
			wantErr: ErrOutsidePeriod,
		},
		{
			name: "last day of period, late evening → ok (inclusive end)",
			check: SelectionCheck{
				Period:        july2026,
				SelectedAt:    time.Date(2026, time.July, 31, 23, 59, 0, 0, time.UTC),
				OfferKind:     OfferRegular,
				MaxCategories: maxCats(4),
			},
			wantErr: nil,
		},
		{
			name: "first day of period, midnight → ok (inclusive start)",
			check: SelectionCheck{
				Period:        july2026,
				SelectedAt:    Date(2026, time.July, 1),
				OfferKind:     OfferRegular,
				MaxCategories: maxCats(4),
			},
			wantErr: nil,
		},
		{
			name: "backfill override skips containment (invariant 2 override)",
			check: SelectionCheck{
				Period:           june2026,
				SelectedAt:       Date(2026, time.July, 20),
				OfferKind:        OfferRegular,
				MaxCategories:    maxCats(4),
				BackfillOverride: true,
			},
			wantErr: nil,
		},
		{
			name: "backfill override does NOT bypass slot limit",
			check: SelectionCheck{
				Period:               june2026,
				SelectedAt:           Date(2026, time.July, 20),
				OfferKind:            OfferRegular,
				MaxCategories:        maxCats(3),
				RegularSelectedCount: 3,
				BackfillOverride:     true,
			},
			wantErr: ErrSlotsExhausted,
		},
		{
			name: "offer already selected → reject even with free slots",
			check: SelectionCheck{
				Period:          july2026,
				SelectedAt:      Date(2026, time.July, 10),
				OfferKind:       OfferRegular,
				AlreadySelected: true,
				MaxCategories:   maxCats(4),
			},
			wantErr: ErrAlreadySelected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSelection(tt.check)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateSelection() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateNewPeriod covers invariant 4: one bank client's offer_period
// ranges never overlap. Boundaries are inclusive dates.
func TestValidateNewPeriod(t *testing.T) {
	tests := []struct {
		name      string
		candidate DateRange
		existing  []DateRange
		wantErr   error
	}{
		{"first period of a client", july2026, nil, nil},
		{"adjacent months do not overlap", august2026, []DateRange{july2026}, nil},
		{"identical range", july2026, []DateRange{july2026}, ErrPeriodOverlap},
		{"quarter overlaps its months", q3_2026, []DateRange{july2026}, ErrPeriodOverlap},
		{
			"sharing a single boundary day overlaps (inclusive ranges)",
			DateRange{Start: Date(2026, time.July, 31), End: Date(2026, time.August, 30)},
			[]DateRange{july2026},
			ErrPeriodOverlap,
		},
		{
			"end before start is invalid",
			DateRange{Start: Date(2026, time.July, 31), End: Date(2026, time.July, 1)},
			nil,
			ErrInvalidPeriod,
		},
		{"overlap with any of several", june2026, []DateRange{august2026, june2026}, ErrPeriodOverlap},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNewPeriod(tt.candidate, tt.existing)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateNewPeriod() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDateRange(t *testing.T) {
	if !july2026.Valid() {
		t.Error("july2026.Valid() = false, want true")
	}
	if (DateRange{Start: Date(2026, time.July, 2), End: Date(2026, time.July, 1)}).Valid() {
		t.Error("inverted range reported valid")
	}

	containsCases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"start boundary", Date(2026, time.July, 1), true},
		{"end boundary", Date(2026, time.July, 31), true},
		{"end boundary with time-of-day", time.Date(2026, time.July, 31, 18, 30, 0, 0, time.UTC), true},
		{"day before", Date(2026, time.June, 30), false},
		{"day after", Date(2026, time.August, 1), false},
	}
	for _, tt := range containsCases {
		if got := july2026.Contains(tt.t); got != tt.want {
			t.Errorf("july2026.Contains(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}

	overlapsCases := []struct {
		name string
		a, b DateRange
		want bool
	}{
		{"disjoint months", june2026, august2026, false},
		{"identical", july2026, july2026, true},
		{"quarter × month", q3_2026, july2026, true},
		{"adjacent months", july2026, august2026, false},
		{"shared boundary day", DateRange{Start: Date(2026, time.June, 30), End: Date(2026, time.July, 1)}, july2026, true},
	}
	for _, tt := range overlapsCases {
		if got := tt.a.Overlaps(tt.b); got != tt.want {
			t.Errorf("Overlaps(%s) = %v, want %v", tt.name, got, tt.want)
		}
		if got := tt.b.Overlaps(tt.a); got != tt.want {
			t.Errorf("Overlaps(%s, reversed) = %v, want %v — must be symmetric", tt.name, got, tt.want)
		}
	}
}

// TestComparableOffers covers invariants 5 (same currency only) and 6
// (special rows excluded), plus the descending-percent ordering the entry
// screen shows.
func TestComparableOffers(t *testing.T) {
	candidate := OfferView{
		OfferID: 1, RawTitle: "Продукты", Percent: pct("5"),
		Kind: OfferRegular, CurrencyKind: CurrencyRub, ClientID: clientOzon,
		ClientLabel: "Ozon Банк", BankName: "Ozon Банк",
	}
	pool := []OfferView{
		candidate, // itself — must be excluded
		{OfferID: 2, RawTitle: "Супермаркеты", Percent: pct("7"), Kind: OfferRegular, CurrencyKind: CurrencyRub, ClientID: clientAlfa, BankName: "Альфа-Банк"},
		{OfferID: 3, RawTitle: "Рестораны", Percent: pct("3"), Kind: OfferRegular, CurrencyKind: CurrencyRub, ClientID: clientAlfa, BankName: "Альфа-Банк"},
		{OfferID: 4, RawTitle: "Супермаркеты", Percent: pct("10"), Kind: OfferRegular, CurrencyKind: CurrencyPoints, ClientID: 4, BankName: "Яндекс Пэй"},
		{OfferID: 5, RawTitle: "Барабан суперкэшбека", Percent: pct("100"), Kind: OfferSpecial, CurrencyKind: CurrencyRub, ClientID: clientAlfa, BankName: "Альфа-Банк"},
		{OfferID: 6, RawTitle: "Загадка", Percent: nil, Kind: OfferRegular, CurrencyKind: CurrencyRub, ClientID: clientMKB, BankName: "МКБ"},
	}

	got := ComparableOffers(candidate, pool)

	wantIDs := []int64{2, 3, 6} // 7% first, 3% next, unknown percent last
	if len(got) != len(wantIDs) {
		t.Fatalf("ComparableOffers() returned %d offers, want %d (rub+regular only, self excluded)", len(got), len(wantIDs))
	}
	for i, id := range wantIDs {
		if got[i].OfferID != id {
			t.Errorf("ComparableOffers()[%d].OfferID = %d, want %d (order: percent desc, unknown last)", i, got[i].OfferID, id)
		}
	}

	special := candidate
	special.Kind = OfferSpecial
	if got := ComparableOffers(special, pool); len(got) != 0 {
		t.Errorf("special candidate got %d comparisons, want 0 (invariant 6)", len(got))
	}
}

// TestRankActiveSelections covers S3: lookup «какой картой платить?».
// E2E step 5 shape: supermarkets on July 15 → Альфа-Банк and Озон ranked
// together (same currency), points entries grouped separately, inactive
// periods filtered. Since the invariant-6 amendment (2026-07-27)
// specials rank too — by percent, keeping their kind so the UI can mark them.
func TestRankActiveSelections(t *testing.T) {
	entries := []LookupEntry{
		{ClientID: clientOzon, BankName: "Ozon Банк", Percent: pct("5"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026, CapValue: pct("3000"), CapPerCategory: pct("1500"), CapScope: CapBoth},
		{ClientID: clientAlfa, BankName: "Альфа-Банк", Percent: pct("5"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026, CapValue: pct("7000"), CapScope: CapTotal},
		{ClientID: 5, BankName: "ВТБ", Percent: pct("15"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: june2026, CapValue: pct("3000"), CapScope: CapTotal}, // expired
		{ClientID: 4, BankName: "Яндекс Пэй", Percent: pct("10"), CurrencyKind: CurrencyPoints, Kind: OfferRegular, Period: july2026, PointsLabel: "Баллы Плюс"},
		{ClientID: clientAlfa, BankName: "Альфа-Банк", RawTitle: "Пятница", Percent: pct("100"), CurrencyKind: CurrencyRub, Kind: OfferSpecial, Period: july2026},
	}

	got := RankActiveSelections(Date(2026, time.July, 15), entries)

	if len(got.Ranked) != 4 {
		t.Fatalf("Ranked has %d entries, want 4 (expired ВТБ filtered, special ranked in)", len(got.Ranked))
	}
	// The 100% special leads the rub group (amendment 2026-07-27: a better
	// offer is never buried below a worse one), keeping kind + raw title.
	if got.Ranked[0].Kind != OfferSpecial || got.Ranked[0].RawTitle != "Пятница" {
		t.Errorf("Ranked[0] = %s/%q, want the special «Пятница» ranked first by percent", got.Ranked[0].Kind, got.Ranked[0].RawTitle)
	}
	// Rub group first (Ozon Банк vs Альфа-Банк tie on 5% → bank-name order),
	// then points. The Latin «O» of «Ozon Банк» sorts before Cyrillic «А»
	// (and Russian collation agrees), so the 2026-07-28 rename flipped this
	// pair — the tie-break only needs to be deterministic, not alphabetical
	// in one script.
	if got.Ranked[1].BankName != "Ozon Банк" || got.Ranked[2].BankName != "Альфа-Банк" {
		t.Errorf("rub group order = [%s, %s], want [Ozon Банк, Альфа-Банк] (tie on 5%% → bank name)",
			got.Ranked[1].BankName, got.Ranked[2].BankName)
	}
	if got.Ranked[3].CurrencyKind != CurrencyPoints {
		t.Errorf("Ranked[3].CurrencyKind = %s, want points grouped after rub", got.Ranked[3].CurrencyKind)
	}
	// Look the entry up by bank rather than by rank: this asserts the cap
	// passes through, and must not silently start checking a different
	// bank's entry when the ordering shifts (it did — see the tie-break
	// note above).
	var alfa *LookupEntry
	for i := range got.Ranked {
		// Альфа-Банк appears twice here (the regular and «Пятница») — the
		// cap belongs to the regular one.
		if got.Ranked[i].BankName == "Альфа-Банк" && got.Ranked[i].Kind == OfferRegular {
			alfa = &got.Ranked[i]
			break
		}
	}
	if alfa == nil || alfa.CapValue == nil || !alfa.CapValue.Equal(decimal.RequireFromString("7000")) {
		t.Errorf("Альфа-Банк entry must pass through its static 7000₽ cap (E2E step 5)")
	}

	empty := RankActiveSelections(Date(2026, time.July, 15), nil)
	if len(empty.Ranked) != 0 {
		t.Errorf("no entries → empty result (S3: «нет активных выборов»), got %+v", empty)
	}

	higherFirst := RankActiveSelections(Date(2026, time.July, 15), []LookupEntry{
		{ClientID: 1, BankName: "А", Percent: pct("3"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026},
		{ClientID: 2, BankName: "Б", Percent: pct("7"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026},
	})
	if len(higherFirst.Ranked) != 2 || !higherFirst.Ranked[0].Percent.Equal(decimal.RequireFromString("7")) {
		t.Errorf("higher percent must rank first within a currency group")
	}
}

// TestAssessAvailability covers S3b verdicts (2026-07-16): honest
// «можно ли выбрать прямо сейчас» per offered-but-unselected menu row.
func TestAssessAvailability(t *testing.T) {
	tests := []struct {
		name  string
		check AvailabilityCheck
		want  AvailabilityVerdict
	}{
		{
			// барабан is granted: no slot, no lock — markable even on a
			// one-shot bank with a full menu.
			"super bypasses slots and locks",
			AvailabilityCheck{Kind: OfferSuper, Policy: AddLockedAfterFirst, HasRegularSelection: true, MaxCategories: maxCats(3), RegularSelectedCount: 3},
			AvailFree,
		},
		{
			"allowed with a free slot (Альфа)",
			AvailabilityCheck{Kind: OfferRegular, Policy: AddAllowed, MaxCategories: maxCats(5), RegularSelectedCount: 4},
			AvailFree,
		},
		{
			"slots full wins regardless of policy",
			AvailabilityCheck{Kind: OfferRegular, Policy: AddAllowed, MaxCategories: maxCats(5), RegularSelectedCount: 5},
			AvailSlotsFull,
		},
		{
			"one-shot bank before any pick is still open (ВТБ до выбора)",
			AvailabilityCheck{Kind: OfferRegular, Policy: AddLockedAfterFirst, HasRegularSelection: false, MaxCategories: maxCats(3), RegularSelectedCount: 0},
			AvailFree,
		},
		{
			"one-shot bank after the pick is locked (ВТБ после выбора)",
			AvailabilityCheck{Kind: OfferRegular, Policy: AddLockedAfterFirst, HasRegularSelection: true, MaxCategories: maxCats(3), RegularSelectedCount: 1},
			AvailLocked,
		},
		{
			"paid change (МКБ)",
			AvailabilityCheck{Kind: OfferRegular, Policy: AddPaid, HasRegularSelection: true, MaxCategories: maxCats(3), RegularSelectedCount: 1},
			AvailPaid,
		},
		{
			"unknown policy (Газпромбанк)",
			AvailabilityCheck{Kind: OfferRegular, Policy: AddUnknown},
			AvailUnknown,
		},
		{
			"unknown slot limit skips the slot check",
			AvailabilityCheck{Kind: OfferRegular, Policy: AddAllowed, MaxCategories: nil, RegularSelectedCount: 99},
			AvailFree,
		},
	}
	for _, tt := range tests {
		if got := AssessAvailability(tt.check); got != tt.want {
			t.Errorf("%s: AssessAvailability() = %s, want %s", tt.name, got, tt.want)
		}
	}
}

// TestRankAvailable: actionable verdicts first, then the ranked ordering
// (currency group, percent desc) within.
func TestRankAvailable(t *testing.T) {
	mk := func(v AvailabilityVerdict, pctStr string, cur CurrencyKind, bank string) AvailableEntry {
		return AvailableEntry{
			Entry:   LookupEntry{BankName: bank, Percent: pct(pctStr), CurrencyKind: cur, Kind: OfferRegular, Period: july2026},
			Verdict: v,
		}
	}
	got := RankAvailable([]AvailableEntry{
		mk(AvailLocked, "10", CurrencyRub, "ВТБ"),
		mk(AvailFree, "5", CurrencyRub, "А"),
		mk(AvailFree, "8", CurrencyRub, "Б"),
		mk(AvailFree, "7", CurrencyPoints, "Яндекс Пэй"),
		mk(AvailPaid, "9", CurrencyRub, "МКБ"),
		mk(AvailSlotsFull, "12", CurrencyRub, "В"),
	})
	wantOrder := []string{"8", "5", "7", "9", "12", "10"} // free rub desc, free points, paid, slots_full, locked
	if len(got) != len(wantOrder) {
		t.Fatalf("RankAvailable() len = %d, want %d", len(got), len(wantOrder))
	}
	for i, w := range wantOrder {
		if got[i].Entry.Percent.String() != w {
			t.Errorf("RankAvailable()[%d].Percent = %s, want %s", i, got[i].Entry.Percent.String(), w)
		}
	}
}

// TestOfferSuper covers the super kind (2026-07-15, spec invariant 6):
// the Альфа monthly барабан ranks like a regular (unlike special), but is
// granted, not chosen — no slot, no collision, not a comparison candidate.
func TestOfferSuper(t *testing.T) {
	// Ranks, and outranks a lower regular in the same category/currency.
	res := RankActiveSelections(Date(2026, time.July, 15), []LookupEntry{
		{ClientID: clientAlfa, BankName: "Альфа-Банк", Percent: pct("7"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026},
		{ClientID: clientAlfa, BankName: "Альфа-Банк", Percent: pct("15"), CurrencyKind: CurrencyRub, Kind: OfferSuper, Period: july2026},
	})
	if len(res.Ranked) != 2 {
		t.Fatalf("super must rank: got Ranked=%d, want 2", len(res.Ranked))
	}
	if res.Ranked[0].Kind != OfferSuper || !res.Ranked[0].Percent.Equal(decimal.RequireFromString("15")) {
		t.Errorf("super 15%% must rank above the regular 7%%, got %s %v", res.Ranked[0].Kind, res.Ranked[0].Percent)
	}

	// No collision: neither as a candidate nor as an existing selection.
	superCand := CandidateSelection{ClientID: clientOzon, CanonicalCategoryID: catID(supermarketsID), Period: july2026, Kind: OfferSuper}
	if got := DetectCollisions(superCand, []ActiveSelection{alfaSupermarkets()}); len(got) != 0 {
		t.Errorf("super candidate must raise no collision, got %d", len(got))
	}
	regCand := CandidateSelection{ClientID: clientOzon, CanonicalCategoryID: catID(supermarketsID), Period: july2026, Kind: OfferRegular}
	grantedSuper := alfaSupermarkets()
	grantedSuper.Kind = OfferSuper
	if got := DetectCollisions(regCand, []ActiveSelection{grantedSuper}); len(got) != 0 {
		t.Errorf("a regular must not collide against a granted super, got %d", len(got))
	}

	// No slot: a super selection is allowed even at the regular cap.
	if err := ValidateSelection(SelectionCheck{
		Period: july2026, SelectedAt: Date(2026, time.July, 10), OfferKind: OfferSuper,
		MaxCategories: maxCats(3), RegularSelectedCount: 3,
	}); err != nil {
		t.Errorf("super consumes no slot even at the regular cap, got %v", err)
	}

	// Not a menu alternative: a super candidate has no comparisons.
	if got := ComparableOffers(OfferView{OfferID: 1, Kind: OfferSuper, CurrencyKind: CurrencyRub}, nil); got != nil {
		t.Errorf("super candidate has no comparisons, got %v", got)
	}
}

// TestOfferSpecialRanks is the regression guard for the 2026-07-27
// report: Озон такси 5% (regular, selected) was answered as «лучшая карта»
// while an Альфа 7% special sat below under «вне рейтинга». Specials now
// rank by percent — but stay granted, not chosen (no slot, no collision, no
// comparisons), so the UI marks them instead of the ranking hiding them.
func TestOfferSpecialRanks(t *testing.T) {
	res := RankActiveSelections(Date(2026, time.July, 15), []LookupEntry{
		{ClientID: clientOzon, BankName: "Ozon Банк", RawTitle: "Такси", Percent: pct("5"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026},
		{ClientID: clientAlfa, BankName: "Альфа-Банк", RawTitle: "Барабан суперкэшбека", Percent: pct("7"), CurrencyKind: CurrencyRub, Kind: OfferSpecial, Period: july2026},
	})
	if len(res.Ranked) != 2 {
		t.Fatalf("special must rank: got Ranked=%d, want 2", len(res.Ranked))
	}
	if res.Ranked[0].BankName != "Альфа-Банк" || res.Ranked[0].Kind != OfferSpecial {
		t.Errorf("special 7%% must rank above the regular 5%%, got %s/%s", res.Ranked[0].BankName, res.Ranked[0].Kind)
	}

	// Currency segregation still wins over percent (invariant 5): a 100%
	// points special never outranks a rub row.
	mixed := RankActiveSelections(Date(2026, time.July, 15), []LookupEntry{
		{ClientID: 4, BankName: "Яндекс Пэй", Percent: pct("100"), CurrencyKind: CurrencyPoints, Kind: OfferSpecial, Period: july2026},
		{ClientID: clientOzon, BankName: "Ozon Банк", Percent: pct("1"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026},
	})
	if mixed.Ranked[0].CurrencyKind != CurrencyRub {
		t.Errorf("rub group still leads a points special (invariant 5), got %s first", mixed.Ranked[0].CurrencyKind)
	}

	// Still granted, not chosen: no slot, no collision, no comparisons.
	if err := ValidateSelection(SelectionCheck{
		Period: july2026, SelectedAt: Date(2026, time.July, 10), OfferKind: OfferSpecial,
		MaxCategories: maxCats(3), RegularSelectedCount: 3,
	}); err != nil {
		t.Errorf("special consumes no slot even at the regular cap, got %v", err)
	}
	specCand := CandidateSelection{ClientID: clientOzon, CanonicalCategoryID: catID(supermarketsID), Period: july2026, Kind: OfferSpecial}
	if got := DetectCollisions(specCand, []ActiveSelection{alfaSupermarkets()}); len(got) != 0 {
		t.Errorf("special candidate must raise no collision, got %d", len(got))
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Продукты", "продукты"},
		{"  Продукты  ", "продукты"},
		{"Кафе  и   Рестораны", "кафе и рестораны"},
		{"ВСЁ ДЛЯ ДОМА", "все для дома"},
		{"Fast Food", "fаsт fооd"}, // a/o/t fold to Cyrillic homoglyphs
		{"Такси\tи каршеринг", "такси и каршеринг"},
		// Альфа sends «й»/«ё» NFD-decomposed (base letter + combining mark).
		{"Чай и кофе", "чай и кофе"},
		{"Вёрный", "верный"},
		// Real Альфа titles mix pixel-identical Latin letters into Cyrillic
		// words; expected values below are pure Cyrillic.
		{"Кафе и pестораны", "кафе и рестораны"},
		{"Цвeты", "цветы"},
		// Genuinely-Latin titles fold too — consistently on both comparison
		// sides, so they keep matching each other.
		{"Duty Free", "duту frее"},
		// Uppercase-only homoglyphs: the Latin letter is lower-cased before
		// folding, so the fold set must cover т/к/м/н/в too. Escapes are
		// explicit because the whole point is a character that renders
		// identically to its Cyrillic twin. «T-Страхование» with a Latin T
		// is what the screenshot recognizer actually returned (2026-07-27).
		{"T-Страхование", "т-страхование"}, // Latin T
		{"Kаршеринг", "каршеринг"},         // Latin K
		{"Мосэнерго", "мосэнерго"},         // Cyrillic М — control case
		{"Mосэнерго", "мосэнерго"},         // Latin M
		{"Hоутбуки", "ноутбуки"},           // Latin H
		{"Bсе покупки", "все покупки"},     // Latin B
	}
	for _, tt := range tests {
		if got := NormalizeTitle(tt.in); got != tt.want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestSuggestCanonical covers the S1 alias pre-suggestion: Озон «Продукты»
// maps to supermarkets; unknown titles report no match so the UI can offer
// creating a new alias inline.
func TestSuggestCanonical(t *testing.T) {
	aliases := []Alias{
		{CanonicalCategoryID: supermarketsID, RawTitle: "Продукты"},
		{CanonicalCategoryID: 20, RawTitle: "Кафе и рестораны"},
	}

	tests := []struct {
		name   string
		raw    string
		wantID int64
		wantOK bool
	}{
		{"exact match", "Продукты", supermarketsID, true},
		{"case/space-insensitive match", "  ПРОДУКТЫ ", supermarketsID, true},
		{"collapsed whitespace match", "Кафе  и  рестораны", 20, true},
		{"unknown title → no suggestion", "Аптеки", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := SuggestCanonical(tt.raw, aliases)
			if id != tt.wantID || ok != tt.wantOK {
				t.Errorf("SuggestCanonical(%q) = (%d, %v), want (%d, %v)", tt.raw, id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

// TestRankActiveSelectionsWithFriends covers the friends-sharing merge
// (FR-S4): friend entries rank by percent alongside own ones, and on equal
// percent the own card wins — the app never sends the user to a friend for
// nothing (friends-sharing invariant 7).
func TestRankActiveSelectionsWithFriends(t *testing.T) {
	entries := []LookupEntry{
		{ClientID: 1, BankName: "ВТБ", Percent: pct("5"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026},
		{ClientID: 2, BankName: "Альфа-Банк", Percent: pct("7"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026, FriendName: "Стас", FriendUsername: "stas"},
		{ClientID: 3, BankName: "Ozon Банк", Percent: pct("5"), CurrencyKind: CurrencyRub, Kind: OfferRegular, Period: july2026, FriendName: "Стас", FriendUsername: "stas"},
	}

	got := RankActiveSelections(Date(2026, time.July, 15), entries)
	if len(got.Ranked) != 3 {
		t.Fatalf("Ranked has %d entries, want 3", len(got.Ranked))
	}
	// The friend's 7% beats the own 5% — a better answer is never buried.
	if got.Ranked[0].FriendName != "Стас" || got.Ranked[0].BankName != "Альфа-Банк" {
		t.Errorf("Ranked[0] = %s/%q, want the friend's 7%% Альфа-Банк first", got.Ranked[0].BankName, got.Ranked[0].FriendName)
	}
	// 5% tie: own ВТБ before the friend's Ozon Банк, even though «Ozon Банк»
	// sorts before «ВТБ» by name — own-first outranks the bank-name order.
	if got.Ranked[1].FriendName != "" || got.Ranked[1].BankName != "ВТБ" {
		t.Errorf("Ranked[1] = %s/%q, want the own 5%% card on the tie", got.Ranked[1].BankName, got.Ranked[1].FriendName)
	}
	if got.Ranked[2].FriendName != "Стас" {
		t.Errorf("Ranked[2].FriendName = %q, want the friend's 5%% card last", got.Ranked[2].FriendName)
	}
}

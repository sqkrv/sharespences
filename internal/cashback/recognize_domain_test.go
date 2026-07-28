package cashback

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/sqkrv/sharespences/internal/vision"
)

func TestParseExtractedNumber(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"7", "7", true},
		{"1.5", "1.5", true},
		{"1,5", "1.5", true},   // RU keyboard / model comma decimal
		{"1%", "1", true},      // the model ignores «digits only» (run 2)
		{"1.5%", "1.5", true},
		{"2 000 ₽", "2000", true},       // ASCII space
		{"2 000", "2000", true},    // NBSP
		{"5 000 ₽", "5000", true},  // narrow NBSP
		{"до 5000", "5000", true},
		{"7.", "7", true},
		{"", "", false},
		{"пять", "", false},
	}
	for _, tc := range cases {
		got, ok := parseExtractedNumber(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseExtractedNumber(%q) = %q,%v want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func reading(screen vision.Screen, slots *vision.Slots) *vision.Reading {
	return &vision.Reading{Screen: screen, Slots: slots}
}

func menuImage(rows ...vision.Row) RecognizedImage {
	return RecognizedImage{AttachmentID: uuid.New(), Reading: reading(vision.Screen{ScreenType: "menu", Rows: rows}, nil)}
}

func row(percent, title string) vision.Row {
	return vision.Row{Percent: percent, Title: title, State: "unchecked"}
}

func hasNote(notes []string, frag string) bool {
	for _, n := range notes {
		if strings.Contains(n, frag) {
			return true
		}
	}
	return false
}

// Scroll overlap collapses: the same normalized title across images is
// one row; Latin homoglyphs and case fold into the same key.
func TestBuildDraftMergesScrollOverlap(t *testing.T) {
	draft := BuildDraft([]RecognizedImage{
		menuImage(row("5", "Супермаркеты"), row("7", "Такси")),
		menuImage(row("7", "Tакси"), row("3", "Аптеки")), // Latin «T»
	}, nil, nil, "Альфа-Банк", nil)
	if len(draft.Rows) != 3 {
		t.Fatalf("rows = %d (%+v), want 3", len(draft.Rows), draft.Rows)
	}
	taxi := draft.Rows[1]
	if taxi.RawTitle != "Такси" || len(taxi.SourceImages) != 2 {
		t.Fatalf("taxi = %+v, want merged from images 1 and 2", taxi)
	}
	if taxi.Percent == nil || *taxi.Percent != "7" || taxi.NeedsReview {
		t.Fatalf("taxi = %+v, want clean percent 7", taxi)
	}
}

// Same title + different percent = surfaced conflict, never auto-resolved.
func TestBuildDraftSurfacesPercentConflict(t *testing.T) {
	draft := BuildDraft([]RecognizedImage{
		menuImage(row("5", "Такси")),
		menuImage(row("7", "Такси")),
	}, nil, nil, "", nil)
	r := draft.Rows[0]
	if r.Percent != nil || !r.NeedsReview {
		t.Fatalf("row = %+v, want nil percent + needs_review", r)
	}
	if len(r.ConflictPercents) != 2 || r.ConflictPercents[0] != "5" || r.ConflictPercents[1] != "7" {
		t.Fatalf("conflicts = %v, want [5 7]", r.ConflictPercents)
	}
}

// «120%» prefills AND flags — never silently dropped (plan phase 2 step 1).
func TestBuildDraftFlagsOutOfRangePercent(t *testing.T) {
	draft := BuildDraft([]RecognizedImage{menuImage(row("120", "Ошибка"))}, nil, nil, "", nil)
	r := draft.Rows[0]
	if r.Percent == nil || *r.Percent != "120" || !r.NeedsReview {
		t.Fatalf("row = %+v, want percent 120 prefetched with needs_review", r)
	}
}

func TestBuildDraftKeepsUnparseableRaw(t *testing.T) {
	draft := BuildDraft([]RecognizedImage{menuImage(row("пять", "Кафе"))}, nil, nil, "", nil)
	r := draft.Rows[0]
	if r.Percent != nil || r.RawPercent != "пять" || !r.NeedsReview {
		t.Fatalf("row = %+v, want nil percent, raw kept, needs_review", r)
	}
}

func TestBuildDraftNormalizesCosmetics(t *testing.T) {
	img := menuImage(vision.Row{Percent: "1,5%", Title: "Все покупки", Cap: "2 000 ₽", State: "unchecked"})
	draft := BuildDraft([]RecognizedImage{img}, nil, nil, "", nil)
	r := draft.Rows[0]
	if r.Percent == nil || *r.Percent != "1.5" || r.CapValue == nil || *r.CapValue != "2000" {
		t.Fatalf("row = %+v, want 1.5 / 2000", r)
	}
	if r.RawPercent != "1,5%" || r.RawCap != "2 000 ₽" {
		t.Fatalf("raw strings not kept verbatim: %+v", r)
	}
	if r.NeedsReview {
		t.Fatalf("cosmetic normalisation must not flag review: %+v", r)
	}
}

// Two барабан cards → ONE kind=super row, latest wins, pre-ticked
// (invariants 5 and 5a).
func TestBuildDraftWheelLatestWins(t *testing.T) {
	wheel := func(percent, title string) RecognizedImage {
		return RecognizedImage{AttachmentID: uuid.New(), Reading: reading(vision.Screen{
			ScreenType: "wheel_result",
			Rows:       []vision.Row{{Percent: percent, Title: title, State: "unknown"}},
		}, nil)}
	}
	draft := BuildDraft([]RecognizedImage{wheel("5", "Такси"), wheel("7", "Рестораны")}, nil, nil, "", nil)
	if len(draft.Rows) != 1 {
		t.Fatalf("rows = %+v, want exactly one super row", draft.Rows)
	}
	r := draft.Rows[0]
	if r.Kind != "super" || r.RawTitle != "Рестораны" || r.Percent == nil || *r.Percent != "7" || !r.Checked {
		t.Fatalf("row = %+v, want latest wheel (Рестораны 7) as pre-ticked super", r)
	}
}

func TestBuildDraftChecksStateHint(t *testing.T) {
	img := menuImage(vision.Row{Percent: "5", Title: "Аптеки", State: "checked"}, row("3", "Кафе"))
	draft := BuildDraft([]RecognizedImage{img}, nil, nil, "", nil)
	if !draft.Rows[0].Checked || draft.Rows[1].Checked {
		t.Fatalf("rows = %+v, want only Аптеки pre-ticked", draft.Rows)
	}
}

// row_kind ≠ category never prefills (invariant 2, owner decision 2 —
// slot modifiers are ignored, not harvested), but is not silently lost.
func TestBuildDraftDropsNonCategoryRows(t *testing.T) {
	img := menuImage(
		vision.Row{Percent: "1", Title: "4 категории кешбэка вместо 3", State: "unchecked", RowKind: "slot_modifier"},
		row("5", "Супермаркеты"),
	)
	draft := BuildDraft([]RecognizedImage{img}, nil, nil, "", nil)
	if len(draft.Rows) != 1 || draft.Rows[0].RawTitle != "Супермаркеты" {
		t.Fatalf("rows = %+v, want the slot modifier dropped", draft.Rows)
	}
	if !hasNote(draft.Notes, "4 категории кешбэка вместо 3") {
		t.Fatalf("notes = %v, want the dropped row named", draft.Notes)
	}
}

func TestBuildDraftSlotGrammar(t *testing.T) {
	total := RecognizedImage{AttachmentID: uuid.New(), Reading: reading(
		vision.Screen{ScreenType: "menu", Rows: []vision.Row{row("5", "Такси")}},
		&vision.Slots{SourceText: "Выберите 4 категории", SlotCount: 4},
	)}
	if got, _ := resolveSlots([]RecognizedImage{total}); got == nil || *got != 4 {
		t.Fatalf("total grammar = %v, want 4", got)
	}

	remaining := RecognizedImage{AttachmentID: uuid.New(), Reading: reading(
		vision.Screen{ScreenType: "menu", Rows: []vision.Row{
			{Percent: "5", Title: "Такси", State: "checked"},
			{Percent: "3", Title: "Кафе", State: "checked"},
			{Percent: "2", Title: "Аптеки", State: "unchecked"},
		}},
		&vision.Slots{SourceText: "Выберите ещё 2 категории", SlotCount: 2},
	)}
	if got, _ := resolveSlots([]RecognizedImage{remaining}); got == nil || *got != 4 {
		t.Fatalf("remaining grammar = %v, want 2 + 2 checked = 4", got)
	}

	english := RecognizedImage{AttachmentID: uuid.New(), Reading: reading(
		vision.Screen{ScreenType: "menu", Rows: []vision.Row{{Percent: "5", Title: "Taxi", State: "checked"}}},
		&vision.Slots{SourceText: "Select 4 more out of 14", SlotCount: 4},
	)}
	// The pool size (14) must never leak into the count (spec: a Яндекс
	// screenshot must not prefill max_categories = 14).
	if got, _ := resolveSlots([]RecognizedImage{english}); got == nil || *got != 5 {
		t.Fatalf("remaining-of-pool = %v, want 4 + 1 checked = 5", got)
	}
}

func TestBuildDraftSlotDisagreementSurfaces(t *testing.T) {
	imgAt := func(count int) RecognizedImage {
		return RecognizedImage{AttachmentID: uuid.New(), Reading: reading(
			vision.Screen{ScreenType: "menu"},
			&vision.Slots{SourceText: "Выберите категории", SlotCount: count},
		)}
	}
	draft := BuildDraft([]RecognizedImage{imgAt(4), imgAt(5)}, nil, nil, "", nil)
	if draft.SlotCount != nil {
		t.Fatalf("slot = %v, want nil on disagreement", *draft.SlotCount)
	}
	if len(draft.SlotCandidates) != 2 || draft.SlotCandidates[0].Value != 4 || draft.SlotCandidates[1].Value != 5 {
		t.Fatalf("candidates = %+v", draft.SlotCandidates)
	}
}

func TestBuildDraftCatalogMatching(t *testing.T) {
	canonical := int64(31)
	catalog := []CatalogRow{
		{ID: 1, Title: "Такси", CanonicalCategoryID: &canonical, Kind: OfferRegular},
		{ID: 2, Title: "Альфа-Пятница", Kind: OfferSpecial},
	}
	aliases := []Alias{{CanonicalCategoryID: 42, RawTitle: "Продукты"}}
	img := menuImage(
		vision.Row{Percent: "7", Title: "Tакси", State: "unchecked", CatalogMatch: "Такси"}, // model matched
		vision.Row{Percent: "5", Title: "альфа-пятница", State: "unchecked"},                // Go-side title match
		row("3", "Продукты"),  // alias hit
		row("2", "Новая категория"), // genuinely new
	)
	draft := BuildDraft([]RecognizedImage{img}, catalog, aliases, "", nil)

	taxi := draft.Rows[0]
	if taxi.BankCategoryID == nil || *taxi.BankCategoryID != 1 || taxi.CatalogTitle == nil || *taxi.CatalogTitle != "Такси" {
		t.Fatalf("taxi = %+v, want bank_category 1 via the model's catalog_match", taxi)
	}
	if taxi.RawTitle != "Tакси" {
		t.Fatalf("raw_title = %q — must stay verbatim as shown (invariant 3)", taxi.RawTitle)
	}
	if taxi.CanonicalCategoryID != nil {
		t.Fatalf("taxi = %+v — catalog matches send bank_category_id ALONE, no alias write", taxi)
	}

	friday := draft.Rows[1]
	if friday.BankCategoryID == nil || *friday.BankCategoryID != 2 || friday.Kind != "special" {
		t.Fatalf("friday = %+v, want catalog row 2 with its special kind", friday)
	}

	products := draft.Rows[2]
	if products.CanonicalCategoryID == nil || *products.CanonicalCategoryID != 42 || products.BankCategoryID != nil {
		t.Fatalf("products = %+v, want canonical 42 from the existing alias", products)
	}

	fresh := draft.Rows[3]
	if fresh.BankCategoryID != nil || fresh.CanonicalCategoryID != nil {
		t.Fatalf("fresh = %+v, want no mapping for a new title", fresh)
	}
}

// A catalog_match naming a title the catalog lacks is a model
// hallucination — dropped, then ordinary matching proceeds.
func TestBuildDraftBogusCatalogMatchIgnored(t *testing.T) {
	img := menuImage(vision.Row{Percent: "5", Title: "Кафе", State: "unchecked", CatalogMatch: "Несуществующая"})
	draft := BuildDraft([]RecognizedImage{img}, []CatalogRow{{ID: 9, Title: "Кафе"}}, nil, "", nil)
	r := draft.Rows[0]
	if r.BankCategoryID == nil || *r.BankCategoryID != 9 || r.CatalogTitle == nil || *r.CatalogTitle != "Кафе" {
		t.Fatalf("row = %+v, want the direct title match to win over the bogus catalog_match", r)
	}
}

func TestBuildDraftCompletenessWarning(t *testing.T) {
	no, yes := false, true
	headless := RecognizedImage{AttachmentID: uuid.New(), Reading: reading(vision.Screen{
		ScreenType: "menu", Rows: []vision.Row{row("5", "Такси")}, HasHeader: &no, HasFooterButton: &no,
	}, nil)}
	draft := BuildDraft([]RecognizedImage{headless}, nil, nil, "", nil)
	if !hasNote(draft.Notes, "часть меню не попала") {
		t.Fatalf("notes = %v, want the completeness warning", draft.Notes)
	}

	withHeader := RecognizedImage{AttachmentID: uuid.New(), Reading: reading(vision.Screen{
		ScreenType: "menu", Rows: []vision.Row{row("5", "Такси")}, HasHeader: &yes,
	}, nil)}
	draft = BuildDraft([]RecognizedImage{headless, withHeader}, nil, nil, "", nil)
	if hasNote(draft.Notes, "часть меню не попала") {
		t.Fatalf("notes = %v, header present — no warning", draft.Notes)
	}
}

func TestBuildDraftSummaryHint(t *testing.T) {
	img := RecognizedImage{AttachmentID: uuid.New(), Reading: reading(vision.Screen{
		ScreenType: "summary", Rows: []vision.Row{{Percent: "5", Title: "Такси", State: "checked"}},
	}, nil)}
	draft := BuildDraft([]RecognizedImage{img}, nil, nil, "", nil)
	if len(draft.Rows) != 1 {
		t.Fatalf("rows = %+v — a summary still prefills its rows (never rejected)", draft.Rows)
	}
	if !hasNote(draft.Notes, "уже выбранных категорий") {
		t.Fatalf("notes = %v, want the summary hint", draft.Notes)
	}
}

// The warning fires on positive evidence only: the guess has to look like
// a DIFFERENT known bank. Anything unreadable is silence by design — the
// model's bank field is measurably unreliable, and the owner hit a false
// positive on the first real run («ozon банк» vs a client «Озон Банк»).
func TestBuildDraftBankMismatch(t *testing.T) {
	seeded := []string{"Альфа-Банк", "ВТБ", "Ozon Банк", "Яндекс Пэй", "Газпромбанк", "МКБ", "Сбербанк", "Т-Банк"}
	others := func(chosen string) []string {
		var out []string
		for _, b := range seeded {
			if b != chosen {
				out = append(out, b)
			}
		}
		return out
	}
	draftFor := func(guess, chosen string) RecognitionDraftDTO {
		img := RecognizedImage{AttachmentID: uuid.New(), Reading: reading(vision.Screen{
			ScreenType: "menu", Bank: guess, Rows: []vision.Row{row("5", "Такси")},
		}, nil)}
		return BuildDraft([]RecognizedImage{img}, nil, nil, chosen, others(chosen))
	}

	quiet := []struct{ guess, chosen, why string }{
		{"ozon банк", "Ozon Банк", "the reported false positive — Latin/Cyrillic mix of the same name"},
		{"Ozon Bank", "Ozon Банк", "fully-Latin reading of the same bank"},
		{"Альфа Банк", "Альфа-Банк", "hyphen only"},
		{"T-BANK", "Т-Банк", "Latin transliteration — unreadable, must stay silent, not warn"},
		{"SELECT PLUSES", "Альфа-Банк", "a header mistaken for a bank name (measured in run 3)"},
		{"", "Альфа-Банк", "no guess at all"},
		{"ВТБ Путешествия", "ВТБ", "a row title containing the right bank"},
	}
	for _, c := range quiet {
		if d := draftFor(c.guess, c.chosen); hasNote(d.Notes, "другого банка") {
			t.Errorf("guess %q vs %q warned (%s): %v", c.guess, c.chosen, c.why, d.Notes)
		}
	}

	loud := []struct{ guess, chosen, want string }{
		{"Альфа Банк", "Т-Банк", "Альфа-Банк"},
		{"Ozon Банк", "Альфа-Банк", "Ozon Банк"},
		{"Газпромбанк", "МКБ", "Газпромбанк"},
	}
	for _, c := range loud {
		d := draftFor(c.guess, c.chosen)
		if !hasNote(d.Notes, "другого банка") {
			t.Errorf("guess %q vs %q should warn: %v", c.guess, c.chosen, d.Notes)
		}
		if !hasNote(d.Notes, c.want) {
			t.Errorf("guess %q vs %q should name %q: %v", c.guess, c.chosen, c.want, d.Notes)
		}
	}

	// Without a bank list there is nothing to be positive about.
	img := RecognizedImage{AttachmentID: uuid.New(), Reading: reading(vision.Screen{
		ScreenType: "menu", Bank: "Альфа Банк", Rows: []vision.Row{row("5", "Такси")},
	}, nil)}
	if d := BuildDraft([]RecognizedImage{img}, nil, nil, "Т-Банк", nil); hasNote(d.Notes, "другого банка") {
		t.Errorf("no bank list must mean no warning: %v", d.Notes)
	}
}

func TestBuildDraftNotRelevantAndSkipped(t *testing.T) {
	skipped := RecognizedImage{AttachmentID: uuid.New(), SkipNote: "не удалось декодировать (HEIC)"}
	irrelevant := RecognizedImage{AttachmentID: uuid.New(), Reading: reading(vision.Screen{
		ScreenType: "not_relevant", Rows: []vision.Row{row("5", "Мусор")},
	}, nil)}
	draft := BuildDraft([]RecognizedImage{skipped, irrelevant, menuImage(row("5", "Такси"))}, nil, nil, "", nil)
	if len(draft.Rows) != 1 {
		t.Fatalf("rows = %+v, want only the menu row", draft.Rows)
	}
	if !draft.Images[0].Skipped || draft.Images[0].Note == "" {
		t.Fatalf("images[0] = %+v, want skipped with note", draft.Images[0])
	}
	if !draft.Images[1].Skipped || draft.Images[1].ScreenType != "not_relevant" {
		t.Fatalf("images[1] = %+v", draft.Images[1])
	}
}

package mcc

import (
	"strings"
	"testing"

	"github.com/sqkrv/sharespences/internal/cashback"
)

func parseOK(t *testing.T, doc string) *Snapshot {
	t.Helper()
	s, err := ParseSnapshot([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const snapshotHead = `"schema_version": 2, "bank": "Тестбанк", "captured_at": "2026-09-01",
	"source": {"file": "t.pdf", "sha256": "abcdef0123456789"}`

func TestParseSnapshotValidation(t *testing.T) {
	cases := map[string]string{
		"schema_version 1":       `{"schema_version": 1, "bank": "Б", "captured_at": "д", "source": {"file": "f", "sha256": "s"}}`,
		"bank, captured_at":      `{"schema_version": 2, "source": {"file": "f", "sha256": "s"}}`,
		"bad MCC":                `{` + snapshotHead + `, "categories": [{"title": "АЗС", "mcc": ["55A1"]}]}`,
		"want 4 digits":          `{` + snapshotHead + `, "categories": [{"title": "АЗС", "mcc": ["742"]}]}`,
		"empty title":            `{` + snapshotHead + `, "categories": [{"title": "", "mcc": ["5541"]}]}`,
		"exclusions.mcc":         `{` + snapshotHead + `, "exclusions": {"mcc": ["48"]}}`,
		"dictionary entry":       `{` + snapshotHead + `, "dictionary": [{"mcc": "5968", "name": ""}]}`,
	}
	for wantErr, doc := range cases {
		if _, err := ParseSnapshot([]byte(doc)); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("want error containing %q, got %v", wantErr, err)
		}
	}
	s := parseOK(t, `{`+snapshotHead+`, "categories": [{"title": "Животные", "mcc": ["0742"]}]}`)
	if got := mustCode(s.Categories[0].MCC[0]); got != 742 {
		t.Fatalf("zero-padded code parsed to %d", got)
	}
	if FormatCode(742) != "0742" {
		t.Fatalf("FormatCode(742) = %q", FormatCode(742))
	}
}

func azs(id int64) CatalogEntry { return CatalogEntry{ID: id, BankID: 1, Title: "АЗС"} }

func known(codes ...int16) map[int16]bool {
	m := map[int16]bool{}
	for _, c := range codes {
		m[c] = true
	}
	return m
}

func TestPlanMembershipDiff(t *testing.T) {
	s := parseOK(t, `{`+snapshotHead+`, "categories": [
		{"title": "АЗС", "mcc": ["5541", "5542"],
		 "qualified": [{"mcc": "3990", "resolves_to": ["Яндекс Заправки"], "when": "только МИР"}]}]}`)
	in := PlanInput{
		Catalog: []CatalogEntry{azs(10), {ID: 11, BankID: 1, Title: "Такси"}},
		Membership: map[int64]map[int16]string{
			10: {5542: "", 5983: ""}, // 5983 must be removed, 5541+3990 added
			11: {4121: ""},           // Такси is not named by the snapshot — untouched
		},
		KnownCodes: known(5541, 5542, 5983, 3990, 4121),
	}
	plan, err := PlanImport(s, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Adds) != 2 || plan.Adds[0].Code != 3990 || plan.Adds[1].Code != 5541 {
		t.Fatalf("adds = %+v", plan.Adds)
	}
	if want := "Яндекс Заправки — только МИР"; plan.Adds[0].Note != want {
		t.Fatalf("qualified note = %q, want %q", plan.Adds[0].Note, want)
	}
	if len(plan.Removes) != 1 || plan.Removes[0].Code != 5983 {
		t.Fatalf("removes = %+v", plan.Removes)
	}
	if plan.Baseline[10] {
		t.Fatal("populated category must not be baseline")
	}
}

func TestPlanBaselineAndIdempotency(t *testing.T) {
	s := parseOK(t, `{`+snapshotHead+`, "categories": [{"title": "АЗС", "mcc": ["5541"]}]}`)
	in := PlanInput{Catalog: []CatalogEntry{azs(10)}, KnownCodes: known(5541),
		Membership: map[int64]map[int16]string{}}
	plan, err := PlanImport(s, in)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Baseline[10] {
		t.Fatal("empty category must journal as imported")
	}
	// second run: state now matches the snapshot
	in.Membership = map[int64]map[int16]string{10: {5541: ""}}
	plan, err = PlanImport(s, in)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Empty() {
		t.Fatalf("re-import must be a no-op, got %+v", plan)
	}
}

func TestPlanTitleResolution(t *testing.T) {
	// homoglyph/case variants resolve; a code-carrying unknown fails; a bare
	// unknown lands in NewTitles unless already journaled.
	s := parseOK(t, `{`+snapshotHead+`, "categories": [
		{"title": "АЗC", "mcc": ["5541"]},
		{"title": "Дикси", "mcc": []},
		{"title": "Zolla", "mcc": []}]}`) // АЗC ends with Latin C
	in := PlanInput{
		Catalog:         []CatalogEntry{azs(10)},
		Membership:      map[int64]map[int16]string{},
		KnownCodes:      known(5541),
		JournaledTitles: map[string]bool{cashback.NormalizeTitle("Zolla"): true},
	}
	plan, err := PlanImport(s, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Adds) != 1 || plan.Adds[0].Category.ID != 10 {
		t.Fatalf("homoglyph title did not resolve: %+v", plan.Adds)
	}
	if len(plan.NewTitles) != 1 || plan.NewTitles[0] != "Дикси" {
		t.Fatalf("NewTitles = %v (Zolla is already journaled)", plan.NewTitles)
	}

	s = parseOK(t, `{`+snapshotHead+`, "categories": [{"title": "Неизвестная", "mcc": ["5541"]}]}`)
	if _, err := PlanImport(s, in); err == nil || !strings.Contains(err.Error(), "Неизвестная") {
		t.Fatalf("code-carrying unknown title must fail, got %v", err)
	}

	in.Catalog = append(in.Catalog, CatalogEntry{ID: 12, BankID: 1, Title: "азс"})
	s = parseOK(t, `{`+snapshotHead+`, "categories": [{"title": "АЗС", "mcc": ["5541"]}]}`)
	if _, err := PlanImport(s, in); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous resolution must fail, got %v", err)
	}
}

func TestPlanDictionaryAndSkips(t *testing.T) {
	s := parseOK(t, `{`+snapshotHead+`,
		"categories": [{"title": "АЗС", "mcc": ["5541", "5968", "4011"]}],
		"dictionary": [{"mcc": "5968", "name": "Оплата подписок"}, {"mcc": "5541", "name": "уже есть"}]}`)
	in := PlanInput{Catalog: []CatalogEntry{azs(10)}, KnownCodes: known(5541),
		Membership: map[int64]map[int16]string{}}
	plan, err := PlanImport(s, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.DictionaryAdds) != 1 || plan.DictionaryAdds[0].MCC != "5968" {
		t.Fatalf("dictionary adds = %+v (known codes must not re-add)", plan.DictionaryAdds)
	}
	if len(plan.Adds) != 2 { // 5541 + 5968 (made known by the dictionary section)
		t.Fatalf("adds = %+v", plan.Adds)
	}
	if got := plan.SkippedCodes["АЗС"]; len(got) != 1 || got[0] != "4011" {
		t.Fatalf("skipped = %v", plan.SkippedCodes)
	}
}

func TestPlanExclusionsPerKindPresence(t *testing.T) {
	// The snapshot syncs only the kinds it carries: classes stay untouched
	// when only mcc/qualified are present, and one qualified code holds
	// several rows distinguished by note.
	s := parseOK(t, `{`+snapshotHead+`, "exclusions": {
		"mcc": ["4829"],
		"qualified": [{"mcc": "3990", "when": "курьер"}, {"mcc": "3990", "when": "штрафы"}]}}`)
	in := PlanInput{
		Membership: map[int64]map[int16]string{},
		KnownCodes: known(),
		Exclusions: map[string]map[string]string{
			"mcc":   {ExclusionIdentity("5933", ""): ""}, // will be removed
			"class": {ExclusionIdentity("СБП", ""): ""},  // kind absent from snapshot — kept
		},
	}
	plan, err := PlanImport(s, in)
	if err != nil {
		t.Fatal(err)
	}
	var adds, removes []string
	for _, e := range plan.ExclusionAdds {
		adds = append(adds, e.Kind+":"+e.Value)
	}
	for _, e := range plan.ExclusionRemoves {
		removes = append(removes, e.Kind+":"+e.Value)
	}
	if want := []string{"mcc:4829", "mcc_qualified:3990", "mcc_qualified:3990"}; strings.Join(adds, " ") != strings.Join(want, " ") {
		t.Fatalf("exclusion adds = %v", adds)
	}
	if want := []string{"mcc:5933"}; strings.Join(removes, " ") != strings.Join(want, " ") {
		t.Fatalf("exclusion removes = %v (class kind must be untouched)", removes)
	}
	if plan.ExclusionBaseline["mcc"] {
		t.Fatal("mcc kind had rows — not a baseline")
	}
	if !plan.ExclusionBaseline["mcc_qualified"] {
		t.Fatal("first qualified rows must journal excluded_imported")
	}
	// same snapshot over the resulting state → no-op
	in.Exclusions = map[string]map[string]string{
		"mcc":           {ExclusionIdentity("4829", ""): ""},
		"mcc_qualified": {ExclusionIdentity("3990", "курьер"): "курьер", ExclusionIdentity("3990", "штрафы"): "штрафы"},
		"class":         {ExclusionIdentity("СБП", ""): ""},
	}
	plan, err = PlanImport(s, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ExclusionAdds) != 0 || len(plan.ExclusionRemoves) != 0 {
		t.Fatalf("re-import must be a no-op: +%v −%v", plan.ExclusionAdds, plan.ExclusionRemoves)
	}
}

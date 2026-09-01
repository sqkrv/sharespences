package mcc

// Canonical snapshot (ADR-0004, schema v2): the stable boundary between the
// per-bank document parsers (meta-repo utils/) and this import. Pure layer —
// parsing, validation and diff planning; the DB side lives in
// snapshot_import.go.
//
// Semantics the planner encodes (ADR-0004 §6):
//   - scoped to the snapshot's bank, and within it to the catalog rows the
//     snapshot NAMES — a catalog row absent from the document is untouched
//     (absence in one document proves nothing; the same scoping rule the
//     seed's membership refresh used);
//   - a snapshot carries only the sections its source document is
//     authoritative for: a nil section — and, inside exclusions, a nil
//     kind — is untouched;
//   - an unresolved title THAT CARRIES CODES fails the whole import: it
//     means the picker catalog needs updating first (catalog and MCC data
//     stay in lockstep by construction). An unresolved title with no codes
//     is reported and journaled as category_added, but blocks nothing — it
//     carries no membership data;
//   - ecosystem/conditional codes arrive as `qualified` and land as plain
//     membership with the condition folded into the row note (flat model);
//   - codes missing from the mcc dictionary are skipped and counted, never
//     stubbed (import-pos precedent) — the snapshot's own `dictionary`
//     section is applied first, so glossed appendix codes do land.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sqkrv/sharespences/internal/cashback"
)

// Snapshot mirrors the schema the utils/ parsers emit.
type Snapshot struct {
	SchemaVersion int                 `json:"schema_version"`
	Bank          string              `json:"bank"`
	CapturedAt    string              `json:"captured_at"`
	Source        SnapshotSource      `json:"source"`
	Note          string              `json:"note,omitempty"`
	Categories    []SnapshotCategory  `json:"categories,omitempty"`
	Exclusions    *SnapshotExclusions `json:"exclusions,omitempty"`
	Dictionary    []SnapshotGloss     `json:"dictionary,omitempty"`
}

type SnapshotSource struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url,omitempty"`
}

type SnapshotCategory struct {
	Title     string              `json:"title"`
	Note      string              `json:"note,omitempty"`
	MCC       []string            `json:"mcc"`
	Qualified []SnapshotQualified `json:"qualified,omitempty"`
}

type SnapshotQualified struct {
	MCC        string   `json:"mcc"`
	ResolvesTo []string `json:"resolves_to,omitempty"`
	When       string   `json:"when,omitempty"`
}

// SnapshotExclusions uses pointer slices so an absent kind (key not in the
// JSON) is distinguishable from an explicitly empty one: only present kinds
// are synced.
type SnapshotExclusions struct {
	MCC         *[]string            `json:"mcc,omitempty"`
	Qualified   *[]SnapshotQualified `json:"qualified,omitempty"`
	Classes     *[]string            `json:"classes,omitempty"`
	Descriptors *[]string            `json:"descriptors,omitempty"`
}

type SnapshotGloss struct {
	MCC  string `json:"mcc"`
	Name string `json:"name"`
}

var snapshotCode = regexp.MustCompile(`^\d{4}$`)

// ParseSnapshot decodes and validates a snapshot document.
func ParseSnapshot(data []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	if s.SchemaVersion != 2 {
		return nil, fmt.Errorf("snapshot: schema_version %d, this build imports 2", s.SchemaVersion)
	}
	if s.Bank == "" || s.Source.File == "" || s.Source.SHA256 == "" || s.CapturedAt == "" {
		return nil, fmt.Errorf("snapshot: bank, captured_at and source file+sha256 are required")
	}
	check := func(code, where string) error {
		if !snapshotCode.MatchString(code) {
			return fmt.Errorf("snapshot: bad MCC %q in %s (want 4 digits, zero-padded)", code, where)
		}
		return nil
	}
	for _, c := range s.Categories {
		if c.Title == "" {
			return nil, fmt.Errorf("snapshot: category with empty title")
		}
		for _, code := range c.MCC {
			if err := check(code, "«"+c.Title+"»"); err != nil {
				return nil, err
			}
		}
		for _, q := range c.Qualified {
			if err := check(q.MCC, "«"+c.Title+"» qualified"); err != nil {
				return nil, err
			}
		}
	}
	if e := s.Exclusions; e != nil {
		if e.MCC != nil {
			for _, code := range *e.MCC {
				if err := check(code, "exclusions.mcc"); err != nil {
					return nil, err
				}
			}
		}
		if e.Qualified != nil {
			for _, q := range *e.Qualified {
				if err := check(q.MCC, "exclusions.qualified"); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, g := range s.Dictionary {
		if err := check(g.MCC, "dictionary"); err != nil {
			return nil, err
		}
		if g.Name == "" {
			return nil, fmt.Errorf("snapshot: dictionary entry %s has no name", g.MCC)
		}
	}
	return &s, nil
}

// SourceID is the journal `source` string: the immutable identity of the
// parsed document.
func (s *Snapshot) SourceID() string {
	sha := s.Source.SHA256
	if len(sha) > 12 {
		sha = sha[:12]
	}
	return fmt.Sprintf("import: %s sha256:%s (captured %s)", s.Source.File, sha, s.CapturedAt)
}

// CatalogEntry is one global bank_category row of the snapshot's bank.
type CatalogEntry struct {
	ID     int64
	BankID int32
	Title  string
}

// PlanInput is the current DB state the planner diffs against, already
// restricted to the snapshot's bank (catalog: global rows only).
type PlanInput struct {
	Catalog        []CatalogEntry
	Membership     map[int64]map[int16]string // category id → code → note ("" = null)
	KnownCodes     map[int16]bool             // mcc dictionary
	Exclusions     map[string]map[string]string // kind → identity key → note
	JournaledTitles map[string]bool           // normalized titles already journaled category_added
}

// ExclusionIdentity keys a bank_exclusion row within a kind: the note is
// part of the identity because one qualified code carries several rows with
// different conditions (ГПБ's 3990).
func ExclusionIdentity(value, note string) string { return value + "\x00" + note }

// MembershipChange is one planned ±code on a resolved catalog row.
type MembershipChange struct {
	Category CatalogEntry
	Code     int16
	Note     string
}

// ExclusionChange is one planned ±row in bank_exclusion.
type ExclusionChange struct {
	Kind  string
	Value string
	Note  string
}

// Plan is everything an import run would write, in deterministic order.
type Plan struct {
	SourceID string

	DictionaryAdds []SnapshotGloss // codes the dictionary lacks

	Adds        []MembershipChange
	Removes     []MembershipChange
	NoteUpdates []MembershipChange
	// Baseline categories journal `imported` instead of `added`: their
	// membership was empty before this run (seed precedent — the digest
	// must not render a first load as «bank added N codes today»).
	Baseline map[int64]bool

	// NewTitles: snapshot rows with no codes and no catalog row — reported
	// and journaled category_added (once), never a failure.
	NewTitles []string

	// SkippedCodes: title → codes absent from the dictionary even after
	// DictionaryAdds; counted, not imported.
	SkippedCodes map[string][]string

	ExclusionAdds    []ExclusionChange
	ExclusionRemoves []ExclusionChange
	// ExclusionBaseline marks kinds that were empty before this run —
	// their adds journal `excluded_imported`.
	ExclusionBaseline map[string]bool
}

// Empty reports whether applying the plan would write nothing.
func (p *Plan) Empty() bool {
	return len(p.DictionaryAdds) == 0 && len(p.Adds) == 0 && len(p.Removes) == 0 &&
		len(p.NoteUpdates) == 0 && len(p.NewTitles) == 0 &&
		len(p.ExclusionAdds) == 0 && len(p.ExclusionRemoves) == 0
}

func qualifiedNote(q SnapshotQualified) string {
	parts := make([]string, 0, 2)
	if len(q.ResolvesTo) > 0 {
		parts = append(parts, strings.Join(q.ResolvesTo, ", "))
	}
	if q.When != "" {
		parts = append(parts, q.When)
	}
	return strings.Join(parts, " — ")
}

func mustCode(s string) int16 {
	n, _ := strconv.ParseInt(s, 10, 16) // validated by ParseSnapshot
	return int16(n)
}

// PlanImport diffs a parsed snapshot against the current state. It fails on
// an unresolved title that carries codes and on ambiguous title resolution;
// everything else lands in the plan.
func PlanImport(s *Snapshot, in PlanInput) (*Plan, error) {
	byNorm := map[string][]CatalogEntry{}
	for _, c := range in.Catalog {
		k := cashback.NormalizeTitle(c.Title)
		byNorm[k] = append(byNorm[k], c)
	}

	plan := &Plan{
		SourceID:          s.SourceID(),
		Baseline:          map[int64]bool{},
		SkippedCodes:      map[string][]string{},
		ExclusionBaseline: map[string]bool{},
	}

	known := make(map[int16]bool, len(in.KnownCodes))
	for c := range in.KnownCodes {
		known[c] = true
	}
	for _, g := range s.Dictionary {
		if code := mustCode(g.MCC); !known[code] {
			known[code] = true
			plan.DictionaryAdds = append(plan.DictionaryAdds, g)
		}
	}
	sort.Slice(plan.DictionaryAdds, func(i, j int) bool {
		return plan.DictionaryAdds[i].MCC < plan.DictionaryAdds[j].MCC
	})

	var unresolved, ambiguous []string
	for _, cat := range s.Categories {
		matches := byNorm[cashback.NormalizeTitle(cat.Title)]
		hasCodes := len(cat.MCC) > 0 || len(cat.Qualified) > 0
		switch {
		case len(matches) > 1:
			ambiguous = append(ambiguous, cat.Title)
			continue
		case len(matches) == 0 && hasCodes:
			unresolved = append(unresolved, cat.Title)
			continue
		case len(matches) == 0:
			if !in.JournaledTitles[cashback.NormalizeTitle(cat.Title)] {
				plan.NewTitles = append(plan.NewTitles, cat.Title)
			}
			continue
		}
		row := matches[0]

		want := map[int16]string{}
		for _, code := range cat.MCC {
			want[mustCode(code)] = ""
		}
		for _, q := range cat.Qualified { // a qualified entry wins over a plain duplicate
			want[mustCode(q.MCC)] = qualifiedNote(q)
		}
		for code := range want {
			if !known[code] {
				plan.SkippedCodes[cat.Title] = append(plan.SkippedCodes[cat.Title], FormatCode(code))
				delete(want, code)
			}
		}
		sort.Strings(plan.SkippedCodes[cat.Title])

		have := in.Membership[row.ID]
		if len(have) == 0 && len(want) > 0 {
			plan.Baseline[row.ID] = true
		}
		for code, note := range want {
			cur, ok := have[code]
			switch {
			case !ok:
				plan.Adds = append(plan.Adds, MembershipChange{row, code, note})
			case cur != note:
				plan.NoteUpdates = append(plan.NoteUpdates, MembershipChange{row, code, note})
			}
		}
		for code := range have {
			if _, ok := want[code]; !ok {
				plan.Removes = append(plan.Removes, MembershipChange{row, code, ""})
			}
		}
	}
	if len(ambiguous) > 0 {
		return nil, fmt.Errorf("import: ambiguous titles (several catalog rows normalize equal): %s",
			strings.Join(ambiguous, ", "))
	}
	if len(unresolved) > 0 {
		return nil, fmt.Errorf("import: no catalog row for titles carrying MCC data: %s — add the picker rows first (catalog and MCC data ship in lockstep)",
			strings.Join(unresolved, ", "))
	}
	sortMembership(plan.Adds)
	sortMembership(plan.Removes)
	sortMembership(plan.NoteUpdates)

	if s.Exclusions != nil {
		planExclusions(plan, "mcc", codesAsExclusions(s.Exclusions.MCC), in)
		planExclusions(plan, "mcc_qualified", qualifiedAsExclusions(s.Exclusions.Qualified), in)
		planExclusions(plan, "class", textAsExclusions("class", s.Exclusions.Classes), in)
		planExclusions(plan, "descriptor", textAsExclusions("descriptor", s.Exclusions.Descriptors), in)
	}
	return plan, nil
}

func sortMembership(list []MembershipChange) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Category.Title != list[j].Category.Title {
			return list[i].Category.Title < list[j].Category.Title
		}
		return list[i].Code < list[j].Code
	})
}

func codesAsExclusions(codes *[]string) []ExclusionChange {
	if codes == nil {
		return nil
	}
	out := make([]ExclusionChange, 0, len(*codes))
	for _, c := range *codes {
		out = append(out, ExclusionChange{Kind: "mcc", Value: c})
	}
	return out
}

func qualifiedAsExclusions(quals *[]SnapshotQualified) []ExclusionChange {
	if quals == nil {
		return nil
	}
	out := make([]ExclusionChange, 0, len(*quals))
	for _, q := range *quals {
		out = append(out, ExclusionChange{Kind: "mcc_qualified", Value: q.MCC, Note: qualifiedNote(q)})
	}
	return out
}

func textAsExclusions(kind string, values *[]string) []ExclusionChange {
	if values == nil {
		return nil
	}
	out := make([]ExclusionChange, 0, len(*values))
	for _, v := range *values {
		out = append(out, ExclusionChange{Kind: kind, Value: v})
	}
	return out
}

// planExclusions diffs one kind; a nil `want` (kind absent from the
// snapshot) is untouched by contract, so callers pass nil through.
func planExclusions(plan *Plan, kind string, want []ExclusionChange, in PlanInput) {
	if want == nil {
		return
	}
	desired := map[string]ExclusionChange{}
	for _, e := range want {
		desired[ExclusionIdentity(e.Value, e.Note)] = e
	}
	have := in.Exclusions[kind]
	if len(have) == 0 && len(desired) > 0 {
		plan.ExclusionBaseline[kind] = true
	}
	for key, e := range desired {
		if _, ok := have[key]; !ok {
			plan.ExclusionAdds = append(plan.ExclusionAdds, e)
		}
	}
	for key := range have {
		if _, ok := desired[key]; !ok {
			value, note, _ := strings.Cut(key, "\x00")
			plan.ExclusionRemoves = append(plan.ExclusionRemoves, ExclusionChange{Kind: kind, Value: value, Note: note})
		}
	}
	sortExclusions(plan.ExclusionAdds)
	sortExclusions(plan.ExclusionRemoves)
}

func sortExclusions(list []ExclusionChange) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Kind != list[j].Kind {
			return list[i].Kind < list[j].Kind
		}
		if list[i].Value != list[j].Value {
			return list[i].Value < list[j].Value
		}
		return list[i].Note < list[j].Note
	})
}

// Render writes the human-readable plan — the ADR's review gate — through
// logf (developer diagnostics, English by convention).
func (p *Plan) Render(bank string, logf func(format string, args ...any)) {
	logf("%s — %s", bank, p.SourceID)
	if len(p.DictionaryAdds) > 0 {
		codes := make([]string, len(p.DictionaryAdds))
		for i, g := range p.DictionaryAdds {
			codes[i] = g.MCC
		}
		logf("dictionary: +%d (%s)", len(codes), strings.Join(codes, ", "))
	}
	byCat := map[string][3][]string{}
	collect := func(list []MembershipChange, slot int) {
		for _, m := range list {
			entry := byCat[m.Category.Title]
			entry[slot] = append(entry[slot], FormatCode(m.Code))
			byCat[m.Category.Title] = entry
		}
	}
	collect(p.Adds, 0)
	collect(p.Removes, 1)
	collect(p.NoteUpdates, 2)
	titles := make([]string, 0, len(byCat))
	for t := range byCat {
		titles = append(titles, t)
	}
	sort.Strings(titles)
	for _, t := range titles {
		e := byCat[t]
		parts := []string{}
		if len(e[0]) > 0 {
			parts = append(parts, "+"+strings.Join(e[0], ", "))
		}
		if len(e[1]) > 0 {
			parts = append(parts, "−"+strings.Join(e[1], ", "))
		}
		if len(e[2]) > 0 {
			parts = append(parts, fmt.Sprintf("%d note updates", len(e[2])))
		}
		logf("«%s»: %s", t, strings.Join(parts, "  "))
	}
	for _, e := range p.ExclusionAdds {
		logf("exclusions[%s] + %s%s", e.Kind, e.Value, noteSuffix(e.Note))
	}
	for _, e := range p.ExclusionRemoves {
		logf("exclusions[%s] − %s%s", e.Kind, e.Value, noteSuffix(e.Note))
	}
	if len(p.NewTitles) > 0 {
		logf("titles with no catalog row (no codes — journaled, not imported): %s",
			strings.Join(p.NewTitles, ", "))
	}
	for _, t := range sortedKeys(p.SkippedCodes) {
		logf("«%s»: skipped, not in dictionary: %s", t, strings.Join(p.SkippedCodes[t], ", "))
	}
	if p.Empty() {
		logf("no changes — snapshot already applied")
	}
}

func noteSuffix(note string) string {
	if note == "" {
		return ""
	}
	if r := []rune(note); len(r) > 60 {
		note = string(r[:60]) + "…"
	}
	return " (" + note + ")"
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

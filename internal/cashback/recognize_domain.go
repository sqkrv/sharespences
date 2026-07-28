// Screenshot-recognizer domain logic: merging per-image VLM readings into
// the prefill draft (docs/specs/cashback-recognizer.md). Pure functions,
// no I/O — the service layer feeds it vision.Reading values and catalog
// snapshots. The recognizer is a PREFILLER, not an authority: nothing
// here is committed without the user picking it on the review screen, so
// everything doubtful is surfaced, never resolved silently.
package cashback

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/text/unicode/norm"

	"github.com/sqkrv/sharespences/internal/vision"
)

// RecognizedImage is one uploaded screenshot's outcome, in upload order.
type RecognizedImage struct {
	AttachmentID uuid.UUID
	Reading      *vision.Reading // nil when the image was skipped
	SkipNote     string          // why, when Reading is nil
}

// CatalogRow is the bank's known picker row (bank_category), the
// constrained vocabulary the draft maps against.
type CatalogRow struct {
	ID                  int64
	Title               string
	CanonicalCategoryID *int64
	Kind                OfferKind
}

// RecognitionRowDTO is one prefilled row of the draft — exactly the
// fields the existing POST /cashback/category-offers accepts, plus the
// review metadata. Number normalisation lives HERE and nowhere else
// (plan contract C5): the Raw* fields keep the model's verbatim strings
// auditable, the parsed fields feed the form.
type RecognitionRowDTO struct {
	RawTitle string `json:"raw_title"`
	// Percent is the normalized figure («1,5%» → «1.5»); nil when the
	// model's string was unparseable or images disagreed.
	Percent    *string `json:"percent,omitempty"`
	RawPercent string  `json:"raw_percent,omitempty"`
	CapValue   *string `json:"cap_value,omitempty"`
	RawCap     string  `json:"raw_cap,omitempty"`
	Kind       string  `json:"kind" enum:"regular,super,special"`
	// BankCategoryID is set when the row matched the bank's catalog —
	// the commit sends it ALONE, and no alias is written from a model
	// match (owner decision 2026-07-28: aliases are bank-global; only an
	// explicit user choice may create one).
	BankCategoryID *int64  `json:"bank_category_id,omitempty"`
	CatalogTitle   *string `json:"catalog_title,omitempty"`
	// CanonicalCategoryID is set only from an EXISTING alias hit —
	// committing it re-upserts the same value, a no-op.
	CanonicalCategoryID *int64 `json:"canonical_category_id,omitempty"`
	// Checked is a pre-tick hint (checkbox state, or a барабан row —
	// invariant 5a). The user's picks stay authoritative.
	Checked          bool     `json:"checked"`
	NeedsReview      bool     `json:"needs_review"`
	ReviewNotes      []string `json:"review_notes,omitempty"`
	ConflictPercents []string `json:"conflict_percents,omitempty"`
	ConflictCaps     []string `json:"conflict_caps,omitempty"`
	SourceImages     []int    `json:"source_images"` // 1-based upload indices
}

// SlotCandidateDTO is one image's slot-count reading after grammar
// resolution. More than one distinct candidate = disagreement — surfaced,
// the user picks (spec §6).
type SlotCandidateDTO struct {
	Value       int    `json:"value"`
	SourceText  string `json:"source_text,omitempty" doc:"not always verbatim — evidence is the number, never the quote"`
	SourceImage int    `json:"source_image"`
}

// RecognitionImageDTO is the per-screenshot outcome shown on review.
type RecognitionImageDTO struct {
	AttachmentID uuid.UUID `json:"attachment_id"`
	ScreenType   string    `json:"screen_type,omitempty"`
	Skipped      bool      `json:"skipped,omitempty"`
	Note         string    `json:"note,omitempty"`
}

// RecognitionDraftDTO is the whole draft — the payload the review screen
// prefills and the commit replays through the four existing endpoints.
type RecognitionDraftDTO struct {
	Rows []RecognitionRowDTO `json:"rows"`
	// SlotCount is the resolved max-categories prefill; nil when unread
	// or when images disagree (then SlotCandidates has them all).
	SlotCount      *int                  `json:"slot_count,omitempty"`
	SlotCandidates []SlotCandidateDTO    `json:"slot_candidates,omitempty"`
	PeriodTexts    []string              `json:"period_texts,omitempty" doc:"cross-check hints from screen headers («на август»)"`
	BankGuesses    []string              `json:"bank_guesses,omitempty" doc:"never trusted — the user's bank client is authoritative"`
	Notes          []string              `json:"notes,omitempty"`
	Images         []RecognitionImageDTO `json:"images"`
}

// numberRe finds the first numeric run, comma or dot decimals.
var numberRe = regexp.MustCompile(`\d+(?:[.,]\d+)?`)

// spaceStripper removes the three spaces RU bank UIs (and the model)
// put inside figures: ASCII, NBSP, narrow NBSP («2 000 ₽»).
var spaceStripper = strings.NewReplacer(" ", "", " ", "", " ", "")

// parseExtractedNumber normalizes a model figure string: strip spaces,
// take the first number, comma→dot. ok=false when no number is present.
func parseExtractedNumber(s string) (string, bool) {
	m := numberRe.FindString(spaceStripper.Replace(s))
	if m == "" {
		return "", false
	}
	return strings.TrimSuffix(strings.ReplaceAll(m, ",", "."), "."), true
}

// percentInRange is the 0–100 sanity check that drives needs_review —
// the row still prefills (an out-of-range «120» is shown AND flagged,
// never dropped).
func percentInRange(s string) bool {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return false
	}
	return !d.IsNegative() && d.LessThanOrEqual(decimal.NewFromInt(100))
}

// slotIsRemaining classifies the slot reading's grammar: «Выберите ещё 5
// категорий» / «Select 4 more» state what REMAINS, so the effective total
// adds the already-checked rows (spec §6; run-4 measured the model can
// read the word but not do the arithmetic).
func slotIsRemaining(sourceText string) bool {
	t := strings.ToLower(sourceText)
	return strings.Contains(t, "ещё") || strings.Contains(t, "еще") || strings.Contains(t, " more")
}

// draftRow is the merge accumulator for one (kind, normalized title).
type draftRow struct {
	row      RecognitionRowDTO
	percents []string // distinct parsed percents seen
	caps     []string // distinct parsed caps seen
}

// BuildDraft merges per-image readings into the prefill draft:
// same normalized title across images collapses (scroll overlap), value
// disagreements surface as conflicts, барабан results collapse to one
// pre-ticked kind=super row with the latest screenshot winning.
// otherBanks are the other known bank names, used only to decide whether
// the screenshots positively belong to a DIFFERENT bank (see
// mismatchedBank); pass nil to disable that warning entirely.
func BuildDraft(images []RecognizedImage, catalog []CatalogRow, aliases []Alias, bankName string, otherBanks []string) RecognitionDraftDTO {
	draft := RecognitionDraftDTO{Images: make([]RecognitionImageDTO, 0, len(images))}

	type key struct {
		kind string
		norm string
	}
	var order []key
	rows := map[key]*draftRow{}
	var lastWheel *RecognizedImage
	var lastWheelIndex int
	sawMenuOrSummary, sawHeaderOrFooter, sawSummary := false, false, false

	addRow := func(imgIndex int, r vision.Row, kind string, preTicked bool) {
		if r.RowKind != "" && r.RowKind != "category" {
			draft.Notes = append(draft.Notes, fmt.Sprintf("строка «%s» пропущена (%s — не категория)", r.Title, r.RowKind))
			return
		}
		k := key{kind: kind, norm: NormalizeTitle(r.Title)}
		dr, seen := rows[k]
		if !seen {
			dr = &draftRow{row: RecognitionRowDTO{RawTitle: r.Title, Kind: kind}}
			rows[k] = dr
			order = append(order, k)
		}
		dr.row.SourceImages = append(dr.row.SourceImages, imgIndex)
		if r.CatalogMatch != "" && dr.row.CatalogTitle == nil {
			m := r.CatalogMatch
			dr.row.CatalogTitle = &m
		}
		if preTicked || r.State == "checked" {
			dr.row.Checked = true
		}
		dr.row.RawPercent = firstNonEmptyStr(dr.row.RawPercent, r.Percent)
		if p, ok := parseExtractedNumber(r.Percent); ok {
			if !containsStr(dr.percents, p) {
				dr.percents = append(dr.percents, p)
			}
		}
		dr.row.RawCap = firstNonEmptyStr(dr.row.RawCap, r.Cap)
		if c, ok := parseExtractedNumber(r.Cap); ok {
			if !containsStr(dr.caps, c) {
				dr.caps = append(dr.caps, c)
			}
		}
	}

	for i, img := range images {
		n := i + 1
		imgDTO := RecognitionImageDTO{AttachmentID: img.AttachmentID}
		if img.Reading == nil {
			imgDTO.Skipped = true
			imgDTO.Note = img.SkipNote
			draft.Images = append(draft.Images, imgDTO)
			continue
		}
		screen := img.Reading.Screen
		imgDTO.ScreenType = screen.ScreenType
		if screen.Bank != "" && !containsStr(draft.BankGuesses, screen.Bank) {
			draft.BankGuesses = append(draft.BankGuesses, screen.Bank)
		}
		if screen.PeriodText != "" && !containsStr(draft.PeriodTexts, screen.PeriodText) {
			draft.PeriodTexts = append(draft.PeriodTexts, screen.PeriodText)
		}

		switch screen.ScreenType {
		case "wheel_result":
			// Latest wins: a re-spin replaces the result, it never adds
			// a second барабан offer (invariant 5).
			lastWheel, lastWheelIndex = &images[i], n
			imgDTO.Note = "экран барабана суперкэшбека"
		case "not_relevant":
			imgDTO.Skipped = true
			imgDTO.Note = "экран не похож на выбор категорий — пропущен"
		default: // menu, summary, partial scrolls — all prefill (spec §2)
			sawMenuOrSummary = true
			if (screen.HasHeader != nil && *screen.HasHeader) || (screen.HasFooterButton != nil && *screen.HasFooterButton) {
				sawHeaderOrFooter = true
			}
			if screen.ScreenType == "summary" {
				sawSummary = true
			}
			for _, r := range screen.Rows {
				addRow(n, r, string(OfferRegular), false)
			}
		}
		draft.Images = append(draft.Images, imgDTO)
	}

	if lastWheel != nil {
		for _, r := range lastWheel.Reading.Screen.Rows {
			// Pre-ticked (invariant 5a): the барабан is granted, not
			// chosen — a screenshot of the result is evidence it is
			// active. Still a default, not a write.
			addRow(lastWheelIndex, r, string(OfferSuper), true)
		}
	}

	// Resolve values, conflicts and catalog matches per merged row.
	for _, k := range order {
		dr := rows[k]
		row := &dr.row
		switch len(dr.percents) {
		case 0:
			if row.RawPercent != "" {
				row.NeedsReview = true
				row.ReviewNotes = append(row.ReviewNotes, fmt.Sprintf("процент не распознан: «%s»", row.RawPercent))
			}
		case 1:
			p := dr.percents[0]
			row.Percent = &p
			if !percentInRange(p) {
				row.NeedsReview = true
				row.ReviewNotes = append(row.ReviewNotes, fmt.Sprintf("процент вне 0–100: %s", p))
			}
		default:
			row.NeedsReview = true
			row.ConflictPercents = dr.percents
			row.ReviewNotes = append(row.ReviewNotes, "скриншоты расходятся в проценте: "+strings.Join(dr.percents, " / "))
		}
		switch len(dr.caps) {
		case 0:
			if row.RawCap != "" {
				row.NeedsReview = true
				row.ReviewNotes = append(row.ReviewNotes, fmt.Sprintf("лимит не распознан: «%s»", row.RawCap))
			}
		case 1:
			c := dr.caps[0]
			row.CapValue = &c
		default:
			row.NeedsReview = true
			row.ConflictCaps = dr.caps
			row.ReviewNotes = append(row.ReviewNotes, "скриншоты расходятся в лимите: "+strings.Join(dr.caps, " / "))
		}
		matchCatalog(row, k.norm, catalog, aliases)
		draft.Rows = append(draft.Rows, *row)
	}

	draft.SlotCount, draft.SlotCandidates = resolveSlots(images)

	if sawSummary {
		draft.Notes = append(draft.Notes, "похоже на экран уже выбранных категорий — в меню могло быть больше строк")
	}
	if sawMenuOrSummary && !sawHeaderOrFooter {
		draft.Notes = append(draft.Notes, "возможно, часть меню не попала: ни заголовок, ни нижняя кнопка не видны")
	}
	if other := mismatchedBank(draft.BankGuesses, bankName, otherBanks); other != "" {
		draft.Notes = append(draft.Notes, fmt.Sprintf("похоже, это скриншоты другого банка (%s) — проверь, тот ли клиент выбран", other))
	}
	return draft
}

// matchCatalog resolves the constrained vocabulary: the model's
// catalog_match first, then a deterministic normalized-title match, then
// the existing alias table. All exact normalized equality — fuzzy
// matching would silently rewrite what the user sees.
func matchCatalog(row *RecognitionRowDTO, norm string, catalog []CatalogRow, aliases []Alias) {
	tryTitle := func(want string) bool {
		for i := range catalog {
			if NormalizeTitle(catalog[i].Title) == want {
				row.BankCategoryID = &catalog[i].ID
				row.CatalogTitle = &catalog[i].Title
				if k := catalog[i].Kind; k != "" && row.Kind == string(OfferRegular) {
					row.Kind = string(k)
				}
				return true
			}
		}
		return false
	}
	if row.CatalogTitle != nil && tryTitle(NormalizeTitle(*row.CatalogTitle)) {
		return
	}
	row.CatalogTitle = nil // the model named a title the catalog lacks
	if tryTitle(norm) {
		return
	}
	if id, ok := SuggestCanonical(row.RawTitle, aliases); ok {
		row.CanonicalCategoryID = &id
	}
}

// resolveSlots reconciles per-image slot readings (spec §6): total is
// direct; remaining adds that screen's checked rows. One distinct value
// prefills; disagreement surfaces every candidate and prefills nothing.
func resolveSlots(images []RecognizedImage) (*int, []SlotCandidateDTO) {
	var candidates []SlotCandidateDTO
	distinct := map[int]bool{}
	for i, img := range images {
		if img.Reading == nil || img.Reading.Slots == nil {
			continue
		}
		s := img.Reading.Slots
		value := s.SlotCount
		if slotIsRemaining(s.SourceText) {
			for _, r := range img.Reading.Screen.Rows {
				if r.State == "checked" {
					value++
				}
			}
		}
		candidates = append(candidates, SlotCandidateDTO{Value: value, SourceText: s.SourceText, SourceImage: i + 1})
		distinct[value] = true
	}
	if len(distinct) == 1 {
		v := candidates[0].Value
		return &v, candidates
	}
	return nil, candidates
}

// canonBank normalizes a bank name for comparison: NFC, lower-case,
// letters and digits only. It deliberately does NOT use NormalizeTitle —
// that folds Latin homoglyphs into Cyrillic, which is right for category
// titles («Tакси») and wrong for bank names, where whole Latin words are
// legitimate: the fold turns «bank» into «ваnк», so «T-BANK» did not match
// «Т-Банк» and «ozon банк» did not match «Ozon Банк».
func canonBank(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(norm.NFC.String(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bankNamesOverlap is containment with a length floor, so a two-letter
// fragment cannot match half the bank list.
func bankNamesOverlap(a, b string) bool {
	if utf8.RuneCountInString(a) < 3 || utf8.RuneCountInString(b) < 3 {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

// mismatchedBank reports which OTHER known bank the screenshots appear to
// belong to, or "" for no warning. It fires only on positive evidence.
//
// The first version warned whenever a guess failed to match the chosen
// bank, which made «не смог прочитать» indistinguishable from «это другой
// банк» — and the model's bank field is measurably unreliable (it returned
// «Альфа Банк» for both Т-Банк and МКБ screens, and row titles as bank
// names). The owner hit the false positive on the first real run: «ozon
// банк» against a client named «Озон Банк». Silence on an unreadable guess
// is the deliberate trade — a missed mismatch costs a prefill the user
// reviews anyway, while a crying-wolf warning trains people to ignore the
// one that matters. The user picks the bank client; this only cross-checks
// (spec decision 4).
func mismatchedBank(guesses []string, bankName string, otherBanks []string) string {
	want := canonBank(bankName)
	for _, g := range guesses {
		if bankNamesOverlap(canonBank(g), want) {
			return "" // a guess confirms the chosen bank — nothing to warn about
		}
	}
	for _, g := range guesses {
		got := canonBank(g)
		for _, other := range otherBanks {
			o := canonBank(other)
			if o == want {
				continue
			}
			if bankNamesOverlap(got, o) {
				return other
			}
		}
	}
	return ""
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

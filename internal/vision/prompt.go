package vision

import (
	"encoding/json"
	"strings"
)

// PromptVersion identifies the prompt+schema revision stored alongside
// results. Bump on ANY change here, and re-run the eval harness
// (docs/research/recognizer-eval, private meta-repo) before shipping —
// the numbers below were measured against this exact wording:
// qwen3-vl:4b on the 13-image set: 93/93 rows, 93/93 percent, 4/4 caps,
// zero hallucinations (run 5, prod GPU).
const PromptVersion = "2026-07-28"

// rowPromptBase is bench.py ROW_PROMPT, verbatim.
const rowPromptBase = `You are reading a screenshot from a Russian bank's mobile app — the screen where a user picks cashback categories for a period.

Extract, as JSON:

- screen_type:
  - "menu" — the picker with the offered category rows
  - "summary" — ONLY already-selected categories are listed, no unselected ones
  - "wheel_result" — a single large prize card with one big percentage (Альфа барабан суперкэшбека)
  - "not_relevant" — anything else
- bank — the bank's name as shown
- period_text — the month or end date in the header, verbatim ("на август", "май", "до 31.12"). Omit if absent.
- rows — EVERY category row visible, top to bottom. Per row:
  - percent — the number ("7", "1.5")
  - title — the category name EXACTLY as printed, without the leading percentage
  - cap — the number from a "Кешбэк до N ₽" chip. Omit if absent.
  - state — "checked" if its checkbox/circle is ticked or filled, else "unchecked", else "unknown"

Rules:
- Transcribe titles verbatim. Do not translate, correct or normalise them.
- Include merchant/brand rows (Tasty Coffee, М.Косметик, MODI) — they are ordinary rows.
- Do NOT include section headers, footer buttons, links, or non-category rows
  (e.g. "4 категории кешбэка вместо 3" is a slot modifier; a prize draw is not a category).
- Report only what is visible. Never invent a row.
`

// rowPromptExtra is the delta over the benchmarked prompt: has_header /
// has_footer_button feed the completeness warning (spec §7), row_kind is
// the safety net behind invariant 2 (a non-category row must never
// prefill), catalog_match is the constrained-vocabulary lever (spec §3).
const rowPromptExtra = `
Also report, top-level:
- has_header — true if the picker's own header/title area is visible in this screenshot
- has_footer_button — true if a footer confirm button («Выбрать», «Продолжить») is visible

If a non-category row does end up in rows, mark it with row_kind: "slot_modifier", "mechanic" or "section_header"; ordinary rows may omit row_kind or use "category".
`

const vocabularyHeader = `
Known catalog titles for this bank:
`

const vocabularyFooter = `For each row whose category is one of the known titles above, also set catalog_match to that exact known title; omit catalog_match for rows not in the list. title itself must STAY verbatim as printed on screen.
`

// RowPrompt builds the row-pass prompt, injecting the bank's known
// catalog titles as a constrained vocabulary when available.
func RowPrompt(catalog []string) string {
	if len(catalog) == 0 {
		return rowPromptBase + rowPromptExtra
	}
	var b strings.Builder
	b.WriteString(rowPromptBase)
	b.WriteString(rowPromptExtra)
	b.WriteString(vocabularyHeader)
	for _, title := range catalog {
		b.WriteString("- ")
		b.WriteString(title)
		b.WriteByte('\n')
	}
	b.WriteString(vocabularyFooter)
	return b.String()
}

// SlotPrompt is bench.py SLOT_PROMPT, verbatim.
const SlotPrompt = `Look ONLY at the header, subtitle and footer button of this cashback category picker.
How many categories may the user select in total this period?
The wording varies — read it literally:
  "Выберите 4 категории"        -> 4
  "Выберите ещё 5 категорий"    -> 5 (how many REMAIN; add any already-ticked rows)
  "Выбрано 0 из 5"              -> 5
  "Выбрано 0 из 3 категорий"    -> 3
  "Select 4 more out of 14"     -> 4  (14 is the menu size, NOT the answer)
If the screen states no such number at all, answer 0.
Answer with the quoted source text and the number.`

// noReason is the rung-3 suffix (bench.py NO_REASON): grammar off, prose
// forbidden.
const noReason = "\n\nRespond with ONLY the JSON object. No reasoning, no explanation, " +
	"no <think> tags, no prose before or after it."

// RowSchema is the benchmarked ROW_SCHEMA plus the optional additions
// above (absent from required, so constrained decoding is not forced to
// emit them). additionalProperties:false is required by the hosted
// backend's structured outputs and harmless for Ollama's grammar.
var RowSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "screen_type": {"type": "string", "enum": ["menu", "summary", "wheel_result", "not_relevant"]},
    "bank": {"type": "string"},
    "period_text": {"type": "string"},
    "has_header": {"type": "boolean"},
    "has_footer_button": {"type": "boolean"},
    "rows": {"type": "array", "items": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "percent": {"type": "string"},
        "title": {"type": "string"},
        "cap": {"type": "string"},
        "state": {"type": "string", "enum": ["unchecked", "checked", "unknown"]},
        "catalog_match": {"type": "string"},
        "row_kind": {"type": "string", "enum": ["category", "slot_modifier", "mechanic", "section_header"]}
      },
      "required": ["percent", "title", "state"]}}
  },
  "required": ["screen_type", "bank", "rows"]
}`)

// SlotSchema is bench.py SLOT_SCHEMA, verbatim plus additionalProperties.
var SlotSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "source_text": {"type": "string"},
    "slot_count": {"type": "integer"}
  },
  "required": ["source_text", "slot_count"]
}`)

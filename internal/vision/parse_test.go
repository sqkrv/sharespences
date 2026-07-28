package vision

import (
	"encoding/json"
	"testing"
)

// One case per measured failure shape from the eval harness, plus the
// deliberate improvement over it (string-aware brace scan).
func TestExtractJSON(t *testing.T) {
	obj := `{"screen_type":"menu","rows":[{"percent":"5","title":"Супермаркеты"}]}`
	cases := []struct {
		name    string
		content string
		think   string
		want    string // "" = expect nil
	}{
		{"plain object", obj, "", obj},
		{"prose before and after", "Вот результат:\n" + obj + "\nНадеюсь, помог.", "", obj},
		{"fenced json", "```json\n" + obj + "\n```", "", obj},
		{"fenced bare", "```\n" + obj + "\n```", "", obj},
		// The MacBook qwen3-vl:2b shape: empty content, the object lives
		// in the thinking channel.
		{"json in thinking", "", "Let me read the rows... " + obj, obj},
		{"leaked closed think block", "<think>reasoning here</think>\n" + obj, "", obj},
		// The sqserver qwen3-vl:2b shape: 6–9k chars of reasoning, never
		// terminated, no object at all.
		{"unterminated think prose", "<think>The screen shows a bank app with категории", "", ""},
		{"unterminated think then thinking has json", "<think>endless prose", obj, obj},
		// Deliberate improvement over bench.py: a brace inside a string
		// must not break the balance count.
		{"brace in title", `{"rows":[{"title":"Кафе {и} бары","percent":"7"}]}`, "", `{"rows":[{"title":"Кафе {и} бары","percent":"7"}]}`},
		{"escaped quote in title", `{"title":"Кафе \"У Ашота\" {x}"}`, "", `{"title":"Кафе \"У Ашота\" {x}"}`},
		{"malformed", `{"rows": [`, "", ""},
		{"invalid inside balanced braces", `{"rows": nope}`, "", ""},
		{"no json at all", "не могу распознать изображение", "", ""},
		{"empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractJSON(tc.content, tc.think)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("want nil, got %s", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want %s, got nil", tc.want)
			}
			if string(got) != tc.want {
				t.Fatalf("want %s, got %s", tc.want, got)
			}
			var v map[string]any // extracted object must round-trip
			if err := json.Unmarshal(got, &v); err != nil {
				t.Fatalf("extracted JSON does not unmarshal: %v", err)
			}
		})
	}
}

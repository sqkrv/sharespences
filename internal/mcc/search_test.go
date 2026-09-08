package mcc

import (
	"reflect"
	"testing"
)

// SearchPatterns is what makes word order irrelevant, and what keeps a user's
// «%» a character rather than a wildcard.
func TestSearchPatterns(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []string
	}{
		{name: "one word", in: "яндекс", want: []string{"%яндекс%"}},
		{name: "two words — one pattern each, order preserved but irrelevant to the AND",
			in: "яндекс доставка", want: []string{"%яндекс%", "%доставка%"}},
		{name: "extra whitespace collapses", in: "  яндекс   доставка \n", want: []string{"%яндекс%", "%доставка%"}},
		{name: "wildcards are literals", in: "100%_скидка", want: []string{`%100\%\_скидка%`}},
		{name: "backslash is escaped first", in: `a\b`, want: []string{`%a\\b%`}},
		{name: "blank query matches nothing", in: "   ", want: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SearchPatterns(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SearchPatterns(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

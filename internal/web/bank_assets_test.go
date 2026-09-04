// Guards the two ways the bank-logo wiring silently breaks. Both are silent by
// construction: a missing logo is a SUPPORTED state (the badge falls back to a
// two-letter chip), so neither a mistyped slug nor a bank absent from the map
// produces an error anywhere — the bank simply never shows its mark, and only a
// human looking at the right screen notices.
//
// It has happened three times: the 00032 rename of Сбербанк to СберБанк left
// BANK_SLUG on the old name; «Совкомбанк» and «МТС Деньги» were guessed as
// sovcombank/mts-dengi against files named sovkom/mtsmoney; and «Яндекс Про»
// was guessed as yandex-pro against yandexpro.
//
// Lives in Go rather than beside the TSX so it runs in `go test ./...`, which
// is what CI gates on.
package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	uiPath     = "../../web/src/components/ui.tsx"
	seedPath   = "../seed/seed.go"
	assetsPath = "../../web/src/assets/banks"
)

// bankSlugs parses the BANK_SLUG literal: bank.name -> asset slug.
func bankSlugs(t *testing.T) map[string]string {
	t.Helper()
	src, err := os.ReadFile(uiPath)
	if err != nil {
		t.Skipf("no SPA sources here: %v", err)
	}
	start := strings.Index(string(src), "const BANK_SLUG")
	if start < 0 {
		t.Fatal("BANK_SLUG not found — the map was renamed and this guard is now blind")
	}
	body := string(src)[start:]
	body = body[:strings.Index(body, "};")]

	entry := regexp.MustCompile(`(?m)^\s*"?([^":\n/]+?)"?\s*:\s*"([a-z0-9-]+)"`)
	out := map[string]string{}
	for _, m := range entry.FindAllStringSubmatch(body, -1) {
		out[strings.TrimSpace(m[1])] = m[2]
	}
	if len(out) == 0 {
		t.Fatal("parsed zero BANK_SLUG entries — the literal's shape changed")
	}
	return out
}

func TestEveryBankSlugHasAnAssetFile(t *testing.T) {
	for bank, slug := range bankSlugs(t) {
		found := false
		for _, ext := range []string{".svg", ".png"} {
			if _, err := os.Stat(filepath.Join(assetsPath, slug+ext)); err == nil {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s maps to %q but no %s.{svg,png} exists — the bank will render "+
				"a two-letter chip forever and nothing will report it", bank, slug, slug)
		}
	}
}

func TestEverySeededBankHasASlug(t *testing.T) {
	src, err := os.ReadFile(seedPath)
	if err != nil {
		t.Skipf("no seed sources here: %v", err)
	}
	// The seed's bank names are the keys of bankColors — every bank that gets a
	// color is a bank the UI renders.
	body := string(src)
	start := strings.Index(body, "var bankColors")
	if start < 0 {
		t.Fatal("bankColors not found — the seed's shape changed and this guard is blind")
	}
	body = body[start:]
	body = body[:strings.Index(body, "\n}")]

	slugs := bankSlugs(t)
	entry := regexp.MustCompile(`"([^"]+)":\s*"#[0-9A-Fa-f]{6}"`)
	for _, m := range entry.FindAllStringSubmatch(body, -1) {
		if _, ok := slugs[m[1]]; !ok {
			t.Errorf("bank %q is seeded with a brand color but has no BANK_SLUG entry — "+
				"it can never pick up a logo, even once the file lands", m[1])
		}
	}
}

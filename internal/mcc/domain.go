// Package mcc implements the MCC module: the code dictionary, per-bank
// category→MCC membership (flat current state), and the append-only change
// journal (pipeline design in ADR-0004,
// meta-repo). Resolution answers «каким банковским категориям принадлежит
// этот MCC» and hands deduped canonical slugs to the cashback lookup.
package mcc

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/sqkrv/sharespences/internal/db"
)

// ErrNotFound — the code is not in the dictionary.
var ErrNotFound = errors.New("код не найден в справочнике")

// ErrBadCode — the input is not a 3-4 digit MCC.
var ErrBadCode = errors.New("код должен быть числом из 3–4 цифр")

// FormatCode renders a code the way banks print them: zero-padded to four
// digits («0742»).
func FormatCode(code int16) string {
	return fmt.Sprintf("%04d", code)
}

// ParseCode accepts 3-4 digit strings (leading zeros allowed).
func ParseCode(s string) (int16, error) {
	if len(s) < 3 || len(s) > 4 {
		return 0, ErrBadCode
	}
	n, err := strconv.ParseInt(s, 10, 16)
	if err != nil || n < 0 {
		return 0, ErrBadCode
	}
	return int16(n), nil
}

// CanonicalRef is one distinct canonical category among a code's per-bank
// resolutions — the hand-off into the cashback category lookup.
type CanonicalRef struct {
	Slug  string
	Title string
}

// DedupCanonicals collapses per-bank resolution rows to distinct canonical
// categories, first-appearance order; canonical-less (special) rows are
// skipped — they never route through the category lookup.
func DedupCanonicals(rows []db.ResolveMCCRow) []CanonicalRef {
	var out []CanonicalRef
	seen := map[string]bool{}
	for _, r := range rows {
		if r.CanonicalSlug == nil || seen[*r.CanonicalSlug] {
			continue
		}
		seen[*r.CanonicalSlug] = true
		title := ""
		if r.CanonicalTitle != nil {
			title = *r.CanonicalTitle
		}
		out = append(out, CanonicalRef{Slug: *r.CanonicalSlug, Title: title})
	}
	return out
}

package mcc

import (
	"errors"
	"testing"

	"github.com/sqkrv/sharespences/internal/db"
)

func TestFormatCode(t *testing.T) {
	tests := []struct {
		in   int16
		want string
	}{
		{742, "0742"},
		{5411, "5411"},
		{1, "0001"},
	}
	for _, tt := range tests {
		if got := FormatCode(tt.in); got != tt.want {
			t.Errorf("FormatCode(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseCode(t *testing.T) {
	ok := []struct {
		in   string
		want int16
	}{
		{"0742", 742},
		{"742", 742},
		{"5411", 5411},
	}
	for _, tt := range ok {
		got, err := ParseCode(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseCode(%q) = (%d, %v), want (%d, nil)", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"", "12", "12345", "abcd", "12a4", "-123"} {
		if _, err := ParseCode(in); !errors.Is(err, ErrBadCode) {
			t.Errorf("ParseCode(%q) err = %v, want ErrBadCode", in, err)
		}
	}
}

func strp(s string) *string { return &s }

func TestDedupCanonicals(t *testing.T) {
	rows := []db.ResolveMCCRow{
		{BankName: "Альфа-Банк", CanonicalSlug: strp("supermarkets"), CanonicalTitle: strp("Супермаркеты")},
		{BankName: "Альфа-Банк", CanonicalSlug: nil}, // special row — skipped
		{BankName: "ВТБ", CanonicalSlug: strp("supermarkets"), CanonicalTitle: strp("Супермаркеты")},
		{BankName: "ВТБ", CanonicalSlug: strp("health"), CanonicalTitle: strp("Здоровье")},
	}
	got := DedupCanonicals(rows)
	if len(got) != 2 || got[0].Slug != "supermarkets" || got[1].Slug != "health" {
		t.Fatalf("DedupCanonicals = %+v, want [supermarkets health] in first-appearance order", got)
	}
}

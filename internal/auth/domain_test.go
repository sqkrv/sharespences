package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeUsername(t *testing.T) {
	cases := []struct{ in, want string }{
		{"anna", "anna"},
		{"ANNA", "anna"},                         // mobile keyboards autocapitalize
		{"  anna  ", "anna"},                     // whitespace was significant before
		{"\tanna\n", "anna"},                     // any Unicode space, not just ' '
		{"@anna", "anna"},                        // the UI renders @login — that's what gets copied
		{"  @Anna ", "anna"},                     // the whole chain at once
		{"@@anna", "@anna"},                      // only one @ comes off; the rest must fail validation
		{"anna@example.com", "anna@example.com"}, // an inner @ is not a prefix
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := NormalizeUsername(c.in); got != c.want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateUsername(t *testing.T) {
	// Validation runs on the normalized form, so uppercase input here is a
	// caller bug rather than a user typo — it must still be rejected.
	valid := []string{
		"ann", // the 3-char floor
		"anna",
		"an1",
		"a1b2c3",
		"ivan.petrov",
		"ivan_petrov",
		"a.b",
		"a_b",
		"a1.b2_c3",
		strings.Repeat("a", 32), // the ceiling
	}
	for _, s := range valid {
		if err := ValidateUsername(s); err != nil {
			t.Errorf("ValidateUsername(%q) = %v, want nil", s, err)
		}
	}

	invalid := []struct{ in, why string }{
		{"", "empty"},
		{"an", "shorter than 3"},
		{strings.Repeat("a", 33), "longer than 32"},
		{"Anna", "not normalized"},
		{"1anna", "leading digit — keeps the all-numeric namespace free"},
		{"123", "all digits"},
		{"_anna", "leading separator"},
		{".anna", "leading separator"},
		{"anna_", "trailing separator"},
		{"anna.", "trailing separator — ambiguous at the end of a sentence"},
		{"ivan..petrov", "doubled separator"},
		{"ivan__petrov", "doubled separator"},
		{"ivan._petrov", "adjacent separators"},
		{"ivan-petrov", "hyphen is not a separator here"},
		{"ivan petrov", "space"},
		{"anna!", "punctuation"},
		{"anna@example.com", "@ inside"},
		{"аnna", "Cyrillic а — the homoglyph this rule exists to kill"},
		{"аня", "all Cyrillic"},
		{"anna\nadmin", "newline must not slip past the anchors"},
		{"admin", "reserved"},
		{"support", "reserved"},
		{"sharespences", "reserved — the brand itself"},
	}
	for _, c := range invalid {
		if err := ValidateUsername(c.in); err == nil {
			t.Errorf("ValidateUsername(%q) = nil, want an error (%s)", c.in, c.why)
		}
	}
}

func TestValidateUsernameReservedIsDistinct(t *testing.T) {
	// The two rejections read differently to the user: one is «fix your input»,
	// the other «pick another name».
	if err := ValidateUsername("admin"); !errors.Is(err, ErrUsernameReserved) {
		t.Errorf("ValidateUsername(admin) = %v, want ErrUsernameReserved", err)
	}
	if err := ValidateUsername("an"); !errors.Is(err, ErrUsernameFormat) {
		t.Errorf("ValidateUsername(an) = %v, want ErrUsernameFormat", err)
	}
}

func TestNormalizeDisplayName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Аня", "Аня"},
		{"  Аня Иванова  ", "Аня Иванова"},
		{"Аня\nИванова", "Аня Иванова"},    // newlines would break the friends list layout
		{"Аня\t\tИванова", "Аня Иванова"},  // runs collapse to one space
		{"Аня\u200bИванова", "АняИванова"}, // zero-width chars are dropped, not spaced
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeDisplayName(c.in); got != c.want {
			t.Errorf("NormalizeDisplayName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateDisplayName(t *testing.T) {
	if err := ValidateDisplayName(""); err == nil {
		t.Error("ValidateDisplayName(empty) = nil, want an error")
	}
	// The cap counts runes, not bytes: a Cyrillic name is two bytes per letter,
	// so a byte-based cap would halve it.
	if err := ValidateDisplayName(strings.Repeat("я", 64)); err != nil {
		t.Errorf("ValidateDisplayName(64 Cyrillic runes) = %v, want nil", err)
	}
	if err := ValidateDisplayName(strings.Repeat("я", 65)); err == nil {
		t.Error("ValidateDisplayName(65 runes) = nil, want an error")
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"foo@example.com", "foo@example.com"},
		{"Foo@Example.COM", "foo@example.com"},
		{"  foo@example.com\n", "foo@example.com"},
	}
	for _, c := range cases {
		if got := NormalizeEmail(c.in); got != c.want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

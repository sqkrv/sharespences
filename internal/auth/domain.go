package auth

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Identity rules for the three fields a person is known by. Pure functions,
// no DB: the service normalizes first and validates the normalized form, so
// what reaches the database is already the canonical value and every reader
// can compare it with plain equality.
//
// The username is the one string this app asks people to dictate to each other
// («скажи мне свой логин» — friend search matches it exactly and nothing else),
// which is what the rule is shaped around:
//   - lowercase-only, folded rather than rejected — mobile keyboards
//     autocapitalize the first letter, and «Anna» must not become a second
//     account beside «anna»;
//   - Latin letters and digits only, so two visually identical logins cannot
//     exist (Cyrillic «аlice» beside Latin «alice» was an impersonation vector
//     in a module whose whole point is sharing financial data with a named
//     person);
//   - «.» and «_» as interior separators, never leading, trailing, doubled or
//     adjacent — «ivan.» and «ivan..petrov» would be dictated identically to
//     «ivan» and «ivan.petrov»;
//   - a leading letter, which keeps the all-numeric namespace free of collisions
//     with the numeric and UUID identifiers users already carry.
const (
	UsernameMinLen    = 3
	UsernameMaxLen    = 32
	DisplayNameMaxLen = 64
)

// usernamePattern is also mirrored in the SPA as the field's HTML `pattern`
// (web/src/lib.ts) for instant feedback. This side is authoritative.
const usernamePattern = `^[a-z][a-z0-9]*([._][a-z0-9]+)*$`

var usernameRe = regexp.MustCompile(usernamePattern)

// reservedUsernames cannot be registered. Two reasons, both concrete: a заявка
// arriving from «@support» or «@sharespences» is a phishing lever in a feature
// that asks people to grant access to their cards, and a handful of the words
// are values that read as absent rather than as a name once they reach JSON or
// a URL. Matching is on the whole login only — «admin_ivan» is a fine name.
var reservedUsernames = map[string]bool{
	"admin": true, "administrator": true, "moderator": true, "support": true,
	"help": true, "official": true, "team": true, "staff": true,
	"security": true, "system": true, "service": true, "bot": true,
	"sharespences": true, "api": true, "root": true, "me": true,
	"null": true, "undefined": true, "anonymous": true, "deleted": true,
}

var (
	ErrUsernameFormat = fmt.Errorf(
		"логин: от %d до %d символов — строчные латинские буквы и цифры, «.» и «_» внутри, начинается с буквы",
		UsernameMinLen, UsernameMaxLen)
	ErrUsernameReserved = errors.New("этот логин зарезервирован, выберите другой")
	ErrDisplayNameLen   = fmt.Errorf("имя: от 1 до %d символов", DisplayNameMaxLen)
)

// NormalizeUsername canonicalizes what a person typed or pasted. It strips one
// leading «@» because every screen renders the login as «@anna» — that prefix
// is part of what gets copied off the screen and dictated, not part of the name.
func NormalizeUsername(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "@")
	return strings.ToLower(s)
}

// ValidateUsername checks the normalized form. Uppercase input therefore fails:
// callers normalize first.
func ValidateUsername(s string) error {
	if n := utf8.RuneCountInString(s); n < UsernameMinLen || n > UsernameMaxLen {
		return ErrUsernameFormat
	}
	if !usernameRe.MatchString(s) {
		return ErrUsernameFormat
	}
	if reservedUsernames[s] {
		return ErrUsernameReserved
	}
	return nil
}

// NormalizeDisplayName flattens whitespace and drops anything non-printing.
// The name is rendered inline next to the login in the friends list and in
// CB-06's cards, where a newline or a control character breaks the layout
// rather than the meaning.
func NormalizeDisplayName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsPrint(r):
			if space && b.Len() > 0 {
				b.WriteRune(' ')
			}
			space = false
			b.WriteRune(r)
		}
		// Non-printing and non-space runes (control chars, zero-width joiners)
		// are dropped outright — they neither separate words nor show up.
	}
	return b.String()
}

// ValidateDisplayName bounds the normalized name in runes: the field is
// Russian, so a byte cap would silently halve it.
func ValidateDisplayName(s string) error {
	if n := utf8.RuneCountInString(s); n < 1 || n > DisplayNameMaxLen {
		return ErrDisplayNameLen
	}
	return nil
}

// NormalizeEmail lowercases the whole address. The domain is case-insensitive
// by RFC 5321 and every provider treats the local part that way too, so
// registering «Foo@x.com» and then signing in as «foo@x.com» must not fail —
// it did, because login matched exactly.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

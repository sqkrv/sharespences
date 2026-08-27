package cashback

import (
	"strings"
	"testing"
)

// RU keyboards produce «1,5»; the API must treat the comma as a decimal
// separator (bug report 2026-07-24, ВТБ 1.5% «Все остальные покупки»).
func TestStrToDecComma(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"1,5", "1.5"},
		{"1.5", "1.5"},
		{"5", "5"},
	} {
		d, err := strToDec(&tc.in, "percent")
		if err != nil {
			t.Fatalf("strToDec(%q): %v", tc.in, err)
		}
		if d.String() != tc.want {
			t.Fatalf("strToDec(%q) = %s, want %s", tc.in, d, tc.want)
		}
	}

	bad := "1,5,0"
	if _, err := strToDec(&bad, "percent"); err == nil {
		t.Fatal("strToDec(\"1,5,0\"): want error")
	}
	if d, err := strToDec(nil, "percent"); d != nil || err != nil {
		t.Fatalf("strToDec(nil) = %v, %v, want nil, nil", d, err)
	}
}

// A money field must not be able to hand pgx something whose decimal expansion
// is enormous: the driver materialises it as a big.Int while encoding the
// statement, and no timeout bounds that. `1e-200000` measured 1.6 s of server
// time for a 40-byte request body — and was accepted.
func TestStrToDecRejectsUnboundedValues(t *testing.T) {
	for _, in := range []string{
		"1e-200000",              // huge negative exponent
		"1e200000",               // huge positive exponent
		"1E999999999",            // shopspring accepts up to MaxInt32
		strings.Repeat("9", 500), // no exponent — the digits themselves
	} {
		if d, err := strToDec(&in, "percent"); err == nil {
			t.Fatalf("strToDec(%.20q…) = %v, want an error", in, d)
		}
	}

	// The bound must not touch anything a person would type, including the
	// trailing zeros a bank's own screen shows.
	for _, in := range []string{"5", "1.5", "0.0001", "12,75", "100", "1.0000", "-3.5"} {
		if _, err := strToDec(&in, "percent"); err != nil {
			t.Fatalf("strToDec(%q): %v, want it accepted", in, err)
		}
	}
}

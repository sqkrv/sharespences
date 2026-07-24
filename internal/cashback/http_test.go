package cashback

import "testing"

// RU keyboards produce «1,5»; the API must treat the comma as a decimal
// separator (owner bug report 2026-07-24, ВТБ 1.5% «Все остальные покупки»).
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

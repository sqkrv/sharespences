package seed

import "testing"

func TestSeededMCCCodes(t *testing.T) {
	codes, err := SeededMCCCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) == 0 {
		t.Fatal("empty coverage set")
	}
	// 5411 (grocery stores) is the canonical always-present dictionary code.
	if !codes[5411] {
		t.Error("5411 missing from the seeded dictionary set")
	}
}

func TestSeededMembershipKeys(t *testing.T) {
	keys, err := SeededMembershipKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) == 0 {
		t.Fatal("empty membership set")
	}
	for k := range keys {
		if k[0] == "" || k[1] == "" {
			t.Fatalf("blank bank or title in membership key %q", k)
		}
	}
}

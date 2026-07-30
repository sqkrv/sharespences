package friends

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestCanonPair(t *testing.T) {
	a := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	lo, hi := CanonPair(a, b)
	if lo != a || hi != b {
		t.Fatalf("CanonPair(a,b) = (%s,%s), want (a,b)", lo, hi)
	}
	lo2, hi2 := CanonPair(b, a)
	if lo2 != lo || hi2 != hi {
		t.Fatalf("CanonPair is not symmetric: (%s,%s) vs (%s,%s)", lo2, hi2, lo, hi)
	}
	if bytes.Compare(lo[:], hi[:]) >= 0 {
		t.Fatalf("lo (%s) not < hi (%s)", lo, hi)
	}
}

func TestOtherMember(t *testing.T) {
	a := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	c := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	if got, err := OtherMember(a, b, a); err != nil || got != b {
		t.Fatalf("OtherMember(a,b,a) = (%s,%v), want (b,nil)", got, err)
	}
	if got, err := OtherMember(a, b, b); err != nil || got != a {
		t.Fatalf("OtherMember(a,b,b) = (%s,%v), want (a,nil)", got, err)
	}
	if _, err := OtherMember(a, b, c); err == nil {
		t.Fatal("OtherMember with a stranger: want error (corrupt grant), got nil")
	}
}

func TestTransitionRequest(t *testing.T) {
	cases := []struct {
		name   string
		status RequestStatus
		action RequestAction
		role   RequestRole
		want   RequestStatus
		err    error
	}{
		{"recipient accepts pending", RequestPending, ActionAccept, RoleRecipient, RequestAccepted, nil},
		{"recipient declines pending", RequestPending, ActionDecline, RoleRecipient, RequestDeclined, nil},
		{"sender cancels pending", RequestPending, ActionCancel, RoleSender, RequestCancelled, nil},
		// Wrong role must look like a missing row — no probe signal.
		{"sender cannot accept", RequestPending, ActionAccept, RoleSender, "", ErrNotFound},
		{"sender cannot decline", RequestPending, ActionDecline, RoleSender, "", ErrNotFound},
		{"recipient cannot cancel", RequestPending, ActionCancel, RoleRecipient, "", ErrNotFound},
		// Terminal rows are history: nothing transitions out of them.
		{"accepted is terminal", RequestAccepted, ActionAccept, RoleRecipient, "", ErrNotFound},
		{"declined is terminal", RequestDeclined, ActionCancel, RoleSender, "", ErrNotFound},
		{"cancelled is terminal", RequestCancelled, ActionDecline, RoleRecipient, "", ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TransitionRequest(tc.status, tc.action, tc.role)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("TransitionRequest(%s,%s,%s) = (%q,%v), want (%q,%v)",
					tc.status, tc.action, tc.role, got, err, tc.want, tc.err)
			}
		})
	}
}

func TestInviteToken(t *testing.T) {
	tok1, hash1, err := NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	tok2, hash2, err := NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok1 == tok2 {
		t.Fatal("two invite tokens are identical")
	}
	if bytes.Equal(hash1, hash2) {
		t.Fatal("two invite token hashes are identical")
	}
	// The storage hash must be recomputable from the plaintext alone —
	// that's the whole claim path.
	if !bytes.Equal(HashInviteToken(tok1), hash1) {
		t.Fatal("HashInviteToken(token) != stored hash")
	}
	if len(hash1) != 32 {
		t.Fatalf("hash length = %d, want 32 (SHA-256)", len(hash1))
	}
	// The token must never equal its own storage form.
	if tok1 == string(hash1) {
		t.Fatal("token equals its hash")
	}
}

// Package friends owns the mutual friend graph (заявки + one-shot invite
// links) and the per-bank_client cashback read grants
// (docs/specs/friends-sharing.md). Cashback stays the only reader of
// cashback tables — this module only resolves WHO sees WHICH clients; the
// shared-picture read path is injected the other way at assembly.
package friends

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// User-facing sentinel errors (Russian at the source, no package prefix —
// the text lands in the response detail).
var (
	// ErrNotFound covers rows that don't exist or belong to another user —
	// scoping never reveals which (spec invariant 1).
	ErrNotFound       = errors.New("не найдено")
	ErrSelfFriendship = errors.New("нельзя добавить в друзья самого себя")
	ErrAlreadyFriends = errors.New("вы уже друзья")
	ErrRequestExists  = errors.New("заявка уже отправлена")
	ErrInviteBurned   = errors.New("приглашение уже использовано")
	ErrInviteExpired  = errors.New("срок действия приглашения истёк")
	ErrRateLimited    = errors.New("слишком много запросов — попробуй позже")
)

// CanonPair orders two user ids into the canonical friendship pair
// (user_lo < user_hi, matching the table's CHECK). Callers reject a == b
// (self-friendship) before pairing.
func CanonPair(a, b uuid.UUID) (lo, hi uuid.UUID) {
	if bytes.Compare(a[:], b[:]) < 0 {
		return a, b
	}
	return b, a
}

// OtherMember returns the friendship member that isn't `member`. An id
// outside the pair is a corrupt grant (spec invariant 2) — an internal
// error, not a user-facing one.
func OtherMember(lo, hi, member uuid.UUID) (uuid.UUID, error) {
	switch member {
	case lo:
		return hi, nil
	case hi:
		return lo, nil
	}
	return uuid.UUID{}, fmt.Errorf("user %s is not a member of friendship (%s, %s)", member, lo, hi)
}

// Request lifecycle. The service enforces role scoping in SQL as well
// (0 rows → ErrNotFound); this table is the single written-down source of
// which transitions exist.
type RequestStatus string

const (
	RequestPending   RequestStatus = "pending"
	RequestAccepted  RequestStatus = "accepted"
	RequestDeclined  RequestStatus = "declined"
	RequestCancelled RequestStatus = "cancelled"
)

type RequestAction string

const (
	ActionAccept  RequestAction = "accept"
	ActionDecline RequestAction = "decline"
	ActionCancel  RequestAction = "cancel"
)

type RequestRole string

const (
	RoleSender    RequestRole = "sender"
	RoleRecipient RequestRole = "recipient"
)

// TransitionRequest returns the status a заявка moves to, or ErrNotFound:
// terminal rows are history, and a wrong-role action must look exactly like
// a missing row (invariant 1 — no probe signal).
func TransitionRequest(status RequestStatus, action RequestAction, role RequestRole) (RequestStatus, error) {
	if status != RequestPending {
		return "", ErrNotFound
	}
	switch {
	case action == ActionAccept && role == RoleRecipient:
		return RequestAccepted, nil
	case action == ActionDecline && role == RoleRecipient:
		return RequestDeclined, nil
	case action == ActionCancel && role == RoleSender:
		return RequestCancelled, nil
	}
	return "", ErrNotFound
}

// InviteTTL bounds how long an unclaimed invite link stays claimable.
const InviteTTL = 7 * 24 * time.Hour

const inviteTokenBytes = 32

// NewInviteToken generates an invite token and its storage hash. The
// plaintext exists exactly once — in the create response (invariant 5);
// only the SHA-256 lands in the DB.
func NewInviteToken() (token string, hash []byte, err error) {
	raw := make([]byte, inviteTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashInviteToken(token), nil
}

// HashInviteToken maps a plaintext token to its storage form.
func HashInviteToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

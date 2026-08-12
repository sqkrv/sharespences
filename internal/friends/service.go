package friends

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sqkrv/sharespences/internal/auth"
	"github.com/sqkrv/sharespences/internal/db"
)

// Service wires the friend graph to storage. Pool exists for the two
// multi-statement flows (accept, invite claim) where a half-applied state
// would burn an invite or accept a заявка without creating the friendship —
// everything else follows the house single-statement style.
type Service struct {
	Q    *db.Queries
	Pool *pgxpool.Pool
}

// RequestOutcome tells the caller whether SendRequest created a pending
// заявка or collapsed a mutual pair into a friendship (auto-accept).
type RequestOutcome struct {
	Accepted bool
	Request  *db.FriendRequest
}

// Search resolves an exact username. The input goes through the same
// normalizer registration used (case folded, «@» prefix stripped — the login is
// rendered as «@anna» everywhere, so that is what people paste), which reduces
// the match to plain equality against the stored canonical form.
func (s *Service) Search(ctx context.Context, username string) (db.User, error) {
	u, err := s.Q.GetUserByUsername(ctx, auth.NormalizeUsername(username))
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, ErrNotFound
	}
	return u, err
}

// SendRequest creates a pending заявка to `username`, or — when the reverse
// заявка is already pending — accepts it (invariant 6: mutual pending
// requests collapse into a friendship, no dead-lock of two rows).
func (s *Service) SendRequest(ctx context.Context, userID uuid.UUID, username string) (RequestOutcome, error) {
	target, err := s.Search(ctx, username)
	if err != nil {
		return RequestOutcome{}, err
	}
	return s.sendRequestTo(ctx, userID, target.ID, false)
}

// sendRequestTo is the заявка core shared by SendRequest and ClaimInvite —
// the invite path differs only in how the target was found and in the
// via_invite marker its заявка carries.
func (s *Service) sendRequestTo(ctx context.Context, userID, targetID uuid.UUID, viaInvite bool) (RequestOutcome, error) {
	if targetID == userID {
		return RequestOutcome{}, ErrSelfFriendship
	}
	lo, hi := CanonPair(userID, targetID)
	if _, err := s.Q.GetFriendshipByPair(ctx, db.GetFriendshipByPairParams{UserLo: lo, UserHi: hi}); err == nil {
		return RequestOutcome{}, ErrAlreadyFriends
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return RequestOutcome{}, err
	}

	pending, err := s.Q.GetPendingRequestBetween(ctx, db.GetPendingRequestBetweenParams{FromUserID: userID, ToUserID: targetID})
	switch {
	case err == nil && pending.FromUserID == userID:
		return RequestOutcome{}, ErrRequestExists
	case err == nil: // reverse pending — auto-accept
		if err := s.acceptTx(ctx, pending.ID, userID); err != nil {
			return RequestOutcome{}, err
		}
		return RequestOutcome{Accepted: true}, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return RequestOutcome{}, err
	}

	req, err := s.Q.CreateFriendRequest(ctx, db.CreateFriendRequestParams{FromUserID: userID, ToUserID: targetID, ViaInvite: viaInvite})
	if err != nil {
		// The pending-pair partial unique index closes the race between the
		// check above and this insert.
		if isPgCode(err, "23505") {
			return RequestOutcome{}, ErrRequestExists
		}
		return RequestOutcome{}, err
	}
	return RequestOutcome{Request: &req}, nil
}

// acceptTx marks a pending заявка accepted and creates the friendship — one
// transaction, so neither half exists without the other.
func (s *Service) acceptTx(ctx context.Context, requestID int64, recipientID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.Q.WithTx(tx)

	req, err := q.SetRequestStatusForRecipient(ctx, db.SetRequestStatusForRecipientParams{
		ID: requestID, ToUserID: recipientID, Status: db.FriendRequestStatusAccepted,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	lo, hi := CanonPair(req.FromUserID, req.ToUserID)
	if _, err := q.CreateFriendship(ctx, db.CreateFriendshipParams{UserLo: lo, UserHi: hi}); err != nil {
		if isPgCode(err, "23505") {
			return ErrAlreadyFriends
		}
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Accept(ctx context.Context, userID uuid.UUID, requestID int64) error {
	return s.acceptTx(ctx, requestID, userID)
}

func (s *Service) Decline(ctx context.Context, userID uuid.UUID, requestID int64) error {
	_, err := s.Q.SetRequestStatusForRecipient(ctx, db.SetRequestStatusForRecipientParams{
		ID: requestID, ToUserID: userID, Status: db.FriendRequestStatusDeclined,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Service) Cancel(ctx context.Context, userID uuid.UUID, requestID int64) error {
	n, err := s.Q.CancelRequestForSender(ctx, db.CancelRequestForSenderParams{ID: requestID, FromUserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListRequests(ctx context.Context, userID uuid.UUID) ([]db.ListPendingRequestsForUserRow, error) {
	return s.Q.ListPendingRequestsForUser(ctx, userID)
}

func (s *Service) ListFriends(ctx context.Context, userID uuid.UUID) ([]db.ListFriendsForUserRow, error) {
	return s.Q.ListFriendsForUser(ctx, userID)
}

// Unfriend deletes the friendship; grants in both directions go with it by
// FK cascade (invariant 3 — re-friending resurrects nothing).
func (s *Service) Unfriend(ctx context.Context, userID, otherID uuid.UUID) error {
	lo, hi := CanonPair(userID, otherID)
	n, err := s.Q.DeleteFriendshipByPair(ctx, db.DeleteFriendshipByPairParams{UserLo: lo, UserHi: hi})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateInvite mints the invite link. Multi-use and re-showable since 00025:
// a claim only files a friend request, so the token may live at rest — that
// is what lets the app display the live link again. One live invite per
// user: the previous link is revoked in the same transaction — «Создать
// новую: старая перестанет работать сразу».
func (s *Service) CreateInvite(ctx context.Context, userID uuid.UUID) (db.FriendInvite, string, error) {
	token, hash, err := NewInviteToken()
	if err != nil {
		return db.FriendInvite{}, "", err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return db.FriendInvite{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.Q.WithTx(tx)

	if err := q.DeleteUnclaimedInvitesForUser(ctx, userID); err != nil {
		return db.FriendInvite{}, "", err
	}
	inv, err := q.CreateFriendInvite(ctx, db.CreateFriendInviteParams{
		CreatedByUserID: userID, TokenHash: hash, Token: &token, ExpiresAt: time.Now().Add(InviteTTL),
	})
	if err != nil {
		return db.FriendInvite{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.FriendInvite{}, "", err
	}
	return inv, token, nil
}

// ListInvites returns the live link (at most one by construction). Rows
// minted before 00025 carry no stored token — the client shows «Создать
// новую» for them, the only path that ever could recover a lost link.
func (s *Service) ListInvites(ctx context.Context, userID uuid.UUID) ([]db.FriendInvite, error) {
	return s.Q.ListLiveInvitesForUser(ctx, userID)
}

func (s *Service) DeleteInvite(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	n, err := s.Q.DeleteInviteForUser(ctx, db.DeleteInviteForUserParams{ID: id, CreatedByUserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimInvite resolves a live link into an incoming friend request to the
// inviter (4e: «переход сам по себе не делает другом — человек падает во
// входящие»). Multi-use: nothing burns, the link keeps working until it
// expires or is replaced. The mutual-pending collapse still applies — if the
// inviter already sent this user a заявка, the claim accepts it.
func (s *Service) ClaimInvite(ctx context.Context, userID uuid.UUID, token string) (db.User, RequestOutcome, error) {
	inv, err := s.Q.GetInviteByTokenHash(ctx, HashInviteToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, RequestOutcome{}, ErrNotFound
	}
	if err != nil {
		return db.User{}, RequestOutcome{}, err
	}
	// Pre-00025 rows were one-shot and may sit in the burned terminal state.
	if inv.ClaimedAt != nil {
		return db.User{}, RequestOutcome{}, ErrInviteBurned
	}
	if !inv.ExpiresAt.After(time.Now()) {
		return db.User{}, RequestOutcome{}, ErrInviteExpired
	}
	inviter, err := s.Q.GetUserByID(ctx, inv.CreatedByUserID)
	if err != nil {
		return db.User{}, RequestOutcome{}, err
	}
	res, err := s.sendRequestTo(ctx, userID, inv.CreatedByUserID, true)
	if err != nil {
		return inviter, RequestOutcome{}, err
	}
	return inviter, res, nil
}

// SetSharing toggles one grant: friendUserID sees (or stops seeing)
// bankClientID. Idempotent both ways. A client that isn't the caller's and
// a user that isn't their friend answer the same ErrNotFound — no probe
// signal (invariant 1).
func (s *Service) SetSharing(ctx context.Context, userID uuid.UUID, bankClientID int64, friendUserID uuid.UUID, shared bool) error {
	if _, err := s.Q.GetBankClientForUser(ctx, db.GetBankClientForUserParams{ID: bankClientID, UserID: userID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	lo, hi := CanonPair(userID, friendUserID)
	f, err := s.Q.GetFriendshipByPair(ctx, db.GetFriendshipByPairParams{UserLo: lo, UserHi: hi})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if shared {
		return s.Q.CreateShare(ctx, db.CreateShareParams{BankClientID: bankClientID, FriendshipID: f.ID})
	}
	_, err = s.Q.DeleteShare(ctx, db.DeleteShareParams{BankClientID: bankClientID, FriendshipID: f.ID})
	return err
}

// ListSharing returns the grants the user has issued.
func (s *Service) ListSharing(ctx context.Context, userID uuid.UUID) ([]db.ListSharesForOwnerRow, error) {
	return s.Q.ListSharesForOwner(ctx, userID)
}

// SharedFriendView is one friend with the client ids they granted the
// viewer (possibly none). The cashback module receives this via a function
// value injected at assembly — never a package import (ADR-0002).
type SharedFriendView struct {
	UserID        uuid.UUID
	Username      string
	DisplayName   string
	BankClientIDs []int64
}

// SharedWithMe resolves every friend of the viewer plus what each one
// currently shares.
func (s *Service) SharedWithMe(ctx context.Context, viewerID uuid.UUID) ([]SharedFriendView, error) {
	friendRows, err := s.Q.ListFriendsForUser(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	shareRows, err := s.Q.ListSharedWithViewer(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	clientsByOwner := make(map[uuid.UUID][]int64)
	for _, r := range shareRows {
		clientsByOwner[r.OwnerUserID] = append(clientsByOwner[r.OwnerUserID], r.BankClientID)
	}
	out := make([]SharedFriendView, len(friendRows))
	for i, f := range friendRows {
		out[i] = SharedFriendView{
			UserID: f.UserID, Username: f.Username, DisplayName: f.DisplayName,
			BankClientIDs: clientsByOwner[f.UserID],
		}
	}
	return out, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

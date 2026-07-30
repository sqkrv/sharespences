package friends

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

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

// Search resolves an exact username (case-insensitive; exact case wins).
func (s *Service) Search(ctx context.Context, username string) (db.User, error) {
	u, err := s.Q.GetUserByUsernameCI(ctx, username)
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
	if target.ID == userID {
		return RequestOutcome{}, ErrSelfFriendship
	}
	lo, hi := CanonPair(userID, target.ID)
	if _, err := s.Q.GetFriendshipByPair(ctx, db.GetFriendshipByPairParams{UserLo: lo, UserHi: hi}); err == nil {
		return RequestOutcome{}, ErrAlreadyFriends
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return RequestOutcome{}, err
	}

	pending, err := s.Q.GetPendingRequestBetween(ctx, db.GetPendingRequestBetweenParams{FromUserID: userID, ToUserID: target.ID})
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

	req, err := s.Q.CreateFriendRequest(ctx, db.CreateFriendRequestParams{FromUserID: userID, ToUserID: target.ID})
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

// CreateInvite mints a one-shot invite link. The plaintext token is
// returned exactly once; only its hash is stored (invariant 5). One live
// invite per user: the previous unclaimed link is revoked in the same
// transaction — «потерял ссылку → создай новую» is the whole recovery
// story, so the old one must die the moment the new one exists.
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
		CreatedByUserID: userID, TokenHash: hash, ExpiresAt: time.Now().Add(InviteTTL),
	})
	if err != nil {
		return db.FriendInvite{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.FriendInvite{}, "", err
	}
	return inv, token, nil
}

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

// ClaimInvite burns a live invite and creates the friendship, returning the
// inviter (for «теперь вы друзья с X»). Already-friends leaves the token
// unburned — the link isn't wasted (invariant 5).
func (s *Service) ClaimInvite(ctx context.Context, userID uuid.UUID, token string) (db.User, error) {
	hash := HashInviteToken(token)
	inv, err := s.Q.GetInviteByTokenHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, ErrNotFound
	}
	if err != nil {
		return db.User{}, err
	}
	if inv.CreatedByUserID == userID {
		return db.User{}, ErrSelfFriendship
	}
	lo, hi := CanonPair(userID, inv.CreatedByUserID)
	if _, err := s.Q.GetFriendshipByPair(ctx, db.GetFriendshipByPairParams{UserLo: lo, UserHi: hi}); err == nil {
		return db.User{}, ErrAlreadyFriends
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return db.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.Q.WithTx(tx)

	if _, err := q.ClaimInvite(ctx, db.ClaimInviteParams{TokenHash: hash, ClaimedByUserID: &userID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The conditional burn matched nothing: the pre-fetched row says
			// which terminal state it hit; a lost race reads as burned.
			if inv.ClaimedAt != nil {
				return db.User{}, ErrInviteBurned
			}
			if !inv.ExpiresAt.After(time.Now()) {
				return db.User{}, ErrInviteExpired
			}
			return db.User{}, ErrInviteBurned
		}
		return db.User{}, err
	}
	if _, err := q.CreateFriendship(ctx, db.CreateFriendshipParams{UserLo: lo, UserHi: hi}); err != nil {
		if isPgCode(err, "23505") {
			return db.User{}, ErrAlreadyFriends
		}
		return db.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.User{}, err
	}
	return s.Q.GetUserByID(ctx, inv.CreatedByUserID)
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

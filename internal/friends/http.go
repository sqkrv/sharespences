package friends

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/sqkrv/sharespences/internal/auth"
)

func httpErr(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return huma.Error404NotFound("не найдено")
	case errors.Is(err, ErrSelfFriendship):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.Is(err, ErrAlreadyFriends), errors.Is(err, ErrRequestExists), errors.Is(err, ErrInviteBurned):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, ErrInviteExpired):
		return huma.Error410Gone(err.Error())
	case errors.Is(err, ErrRateLimited):
		return huma.Error429TooManyRequests(err.Error())
	}
	return err
}

// rateLimiter is a minimal per-user token bucket guarding the enumeration
// surface (search + request-create): exact-match search with minimal fields
// blunts probing, this bounds its rate. In-memory only — a restart resets
// it, which is fine for the guard's purpose.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[uuid.UUID]*bucket
	rate    float64 // tokens per second
	burst   float64
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(perMinute, burst float64) *rateLimiter {
	return &rateLimiter{buckets: make(map[uuid.UUID]*bucket), rate: perMinute / 60, burst: burst}
}

func (rl *rateLimiter) allow(id uuid.UUID) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	// Backstop against unbounded growth: full-and-stale buckets carry no
	// state worth keeping.
	if len(rl.buckets) > 4096 {
		for k, b := range rl.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(rl.buckets, k)
			}
		}
	}
	b, ok := rl.buckets[id]
	if !ok {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[id] = b
	}
	b.tokens = min(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// FriendDTO is one row of the friends list.
type FriendDTO struct {
	UserID      uuid.UUID `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Since       time.Time `json:"since"`
}

// FoundUserDTO is the deliberately minimal search hit (no email, no ids
// beyond the one a заявка needs).
type FoundUserDTO struct {
	UserID      uuid.UUID `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
}

// RequestDTO names the OTHER side of a pending заявка: the sender on
// incoming rows, the recipient on outgoing ones.
type RequestDTO struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

type InviteDTO struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RegisterHTTP mounts the friend-graph operations.
func RegisterHTTP(api huma.API, s *Service) {
	// Sized for a human adding friends, not a script walking usernames.
	limiter := newRateLimiter(10, 20)

	huma.Register(api, huma.Operation{
		OperationID: "friends-list", Method: http.MethodGet,
		Path: "/api/v1/friends", Summary: "List friends", Tags: []string{"friends"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []FriendDTO }, error) {
		rows, err := s.ListFriends(ctx, auth.UserID(ctx))
		if err != nil {
			return nil, httpErr(err)
		}
		out := make([]FriendDTO, len(rows))
		for i, r := range rows {
			out[i] = FriendDTO{UserID: r.UserID, Username: r.Username, DisplayName: r.DisplayName, Since: r.Since}
		}
		return &struct{ Body []FriendDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-remove", Method: http.MethodDelete,
		Path: "/api/v1/friends/{userId}", Summary: "Remove a friend (revokes shares both ways)", Tags: []string{"friends"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		UserID uuid.UUID `path:"userId"`
	}) (*struct{}, error) {
		if err := s.Unfriend(ctx, auth.UserID(ctx), in.UserID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-search", Method: http.MethodGet,
		Path: "/api/v1/friends/search", Summary: "Find a user by exact username", Tags: []string{"friends"},
	}, func(ctx context.Context, in *struct {
		Username string `query:"username" minLength:"1"`
	}) (*struct{ Body FoundUserDTO }, error) {
		if !limiter.allow(auth.UserID(ctx)) {
			return nil, httpErr(ErrRateLimited)
		}
		u, err := s.Search(ctx, in.Username)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body FoundUserDTO }{FoundUserDTO{UserID: u.ID, Username: u.Username, DisplayName: u.DisplayName}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-request-create", Method: http.MethodPost,
		Path: "/api/v1/friends/requests", Summary: "Send a friend request", Tags: []string{"friends"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			Username string `json:"username" minLength:"1"`
		}
	}) (*struct {
		Body struct {
			Status string `json:"status" enum:"pending,accepted" doc:"accepted — встречная заявка уже ждала, вы сразу друзья"`
		}
	}, error) {
		if !limiter.allow(auth.UserID(ctx)) {
			return nil, httpErr(ErrRateLimited)
		}
		res, err := s.SendRequest(ctx, auth.UserID(ctx), in.Body.Username)
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				Status string `json:"status" enum:"pending,accepted" doc:"accepted — встречная заявка уже ждала, вы сразу друзья"`
			}
		}{}
		out.Body.Status = "pending"
		if res.Accepted {
			out.Body.Status = "accepted"
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-request-list", Method: http.MethodGet,
		Path: "/api/v1/friends/requests", Summary: "Pending friend requests (incoming + outgoing)", Tags: []string{"friends"},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Incoming []RequestDTO `json:"incoming"`
			Outgoing []RequestDTO `json:"outgoing"`
		}
	}, error) {
		userID := auth.UserID(ctx)
		rows, err := s.ListRequests(ctx, userID)
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				Incoming []RequestDTO `json:"incoming"`
				Outgoing []RequestDTO `json:"outgoing"`
			}
		}{}
		out.Body.Incoming = []RequestDTO{}
		out.Body.Outgoing = []RequestDTO{}
		for _, r := range rows {
			if r.ToUserID == userID {
				out.Body.Incoming = append(out.Body.Incoming, RequestDTO{
					ID: r.ID, Username: r.FromUsername, DisplayName: r.FromDisplayName, CreatedAt: r.CreatedAt,
				})
			} else {
				out.Body.Outgoing = append(out.Body.Outgoing, RequestDTO{
					ID: r.ID, Username: r.ToUsername, DisplayName: r.ToDisplayName, CreatedAt: r.CreatedAt,
				})
			}
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-request-accept", Method: http.MethodPost,
		Path: "/api/v1/friends/requests/{id}/accept", Summary: "Accept a friend request", Tags: []string{"friends"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.Accept(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-request-decline", Method: http.MethodPost,
		Path: "/api/v1/friends/requests/{id}/decline", Summary: "Decline a friend request", Tags: []string{"friends"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.Decline(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-request-cancel", Method: http.MethodDelete,
		Path: "/api/v1/friends/requests/{id}", Summary: "Cancel an outgoing friend request", Tags: []string{"friends"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.Cancel(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-invite-create", Method: http.MethodPost,
		Path: "/api/v1/friends/invites", Summary: "Create a one-shot invite link", Tags: []string{"friends"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			ID        uuid.UUID `json:"id"`
			URL       string    `json:"url" doc:"путь ссылки-приглашения; хост добавляет клиент"`
			Token     string    `json:"token" doc:"показывается только здесь — хранится лишь хэш"`
			ExpiresAt time.Time `json:"expires_at"`
		}
	}, error) {
		inv, token, err := s.CreateInvite(ctx, auth.UserID(ctx))
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				ID        uuid.UUID `json:"id"`
				URL       string    `json:"url" doc:"путь ссылки-приглашения; хост добавляет клиент"`
				Token     string    `json:"token" doc:"показывается только здесь — хранится лишь хэш"`
				ExpiresAt time.Time `json:"expires_at"`
			}
		}{}
		out.Body.ID = inv.ID
		out.Body.URL = "/friends/join/" + token
		out.Body.Token = token
		out.Body.ExpiresAt = inv.ExpiresAt
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-invite-list", Method: http.MethodGet,
		Path: "/api/v1/friends/invites", Summary: "List live invite links", Tags: []string{"friends"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []InviteDTO }, error) {
		rows, err := s.ListInvites(ctx, auth.UserID(ctx))
		if err != nil {
			return nil, httpErr(err)
		}
		out := make([]InviteDTO, len(rows))
		for i, r := range rows {
			out[i] = InviteDTO{ID: r.ID, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt}
		}
		return &struct{ Body []InviteDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-invite-delete", Method: http.MethodDelete,
		Path: "/api/v1/friends/invites/{id}", Summary: "Revoke an invite link", Tags: []string{"friends"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID uuid.UUID `path:"id"`
	}) (*struct{}, error) {
		if err := s.DeleteInvite(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "friends-invite-claim", Method: http.MethodPost,
		Path: "/api/v1/friends/invites/claim", Summary: "Claim an invite link", Tags: []string{"friends"},
		// Token in the body, not the path — keeps it out of URL logs.
	}, func(ctx context.Context, in *struct {
		Body struct {
			Token string `json:"token" minLength:"1"`
		}
	}) (*struct{ Body FoundUserDTO }, error) {
		inviter, err := s.ClaimInvite(ctx, auth.UserID(ctx), in.Body.Token)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body FoundUserDTO }{FoundUserDTO{
			UserID: inviter.ID, Username: inviter.Username, DisplayName: inviter.DisplayName,
		}}, nil
	})
}

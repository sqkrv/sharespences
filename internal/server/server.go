// Package server assembles the modular monolith per ADR-0002: chi router,
// huma (OpenAPI 3.1) JSON API under /api/v1, scs session auth, module
// registrations. The SPA will be embedded here later (go:embed) — v1 of the
// cashback module ships the API surface first.
package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sqkrv/sharespences/internal/attach"
	"github.com/sqkrv/sharespences/internal/auth"
	"github.com/sqkrv/sharespences/internal/cashback"
	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/friends"
	"github.com/sqkrv/sharespences/internal/i18n"
	"github.com/sqkrv/sharespences/internal/mcc"
	"github.com/sqkrv/sharespences/internal/perks"
	"github.com/sqkrv/sharespences/internal/vision"
	"github.com/sqkrv/sharespences/internal/web"
)

const sessionUserKey = "user_id"

type Config struct {
	Pool           *pgxpool.Pool
	AttachmentsDir string
	// Vision is the screenshot-recognizer model backend; nil turns the
	// feature off (recognition endpoints answer 503, manual entry keeps
	// working). Backends never dial at construction, so the openapi CI
	// gate stays network-free.
	Vision vision.Backend
	// Version identifies the running build — ADR-0006 CalVer (`v2026.7.1`),
	// injected at link time. Empty means an unstamped build.
	Version string
	// Docs serves huma's built-in API reference at /docs. Off by default: the
	// page loads Stoplight Elements from unpkg.com, and the privacy policy
	// (§2.4) states the service loads no third-party front-end resources.
	// Its CSP also runs that script same-origin with connect-src 'self', so a
	// hijacked CDN response could call the API as whoever opened the page.
	// The machine-readable /openapi.json stays available either way.
	Docs bool
	// InsecureCookie drops the Secure flag from the session cookie. Secure is
	// the default because the app is served over HTTPS in production and the
	// cookie must never travel in clear; this escape hatch exists for local
	// development over plain http, where Safari (unlike Chrome and Firefox)
	// refuses to store a Secure cookie even on localhost.
	InsecureCookie bool
}

// version reported when the binary was built without -ldflags.
const devVersion = "dev"

// New builds the full HTTP handler (sessions wrapped around chi+huma,
// embedded SPA as the catch-all). Sessions wrap ONLY /api/: scs stamps
// Vary: Cookie on every wrapped response (breaks service-worker cache
// matching, docs/specs/pwa.md) and costs a DB session lookup per request —
// static assets and the SPA need neither.
func New(cfg Config) http.Handler {
	r, sm, _ := build(cfg)
	withSessions := sm.LoadAndSave(r)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			withSessions.ServeHTTP(w, req)
			return
		}
		r.ServeHTTP(w, req)
	})
}

// OpenAPI returns the API description as JSON — consumed by
// `sharespences openapi` for frontend type generation (web/, see spec).
func OpenAPI(cfg Config) ([]byte, error) {
	_, _, api := build(cfg)
	return api.OpenAPI().MarshalJSON()
}

func build(cfg Config) (chi.Router, *scs.SessionManager, huma.API) {
	// The API answers in the interface language (Russian only for now) —
	// this switches huma's own validation vocabulary and
	// error titles; module messages are written in Russian at their source.
	i18n.Install()

	q := db.New(cfg.Pool)

	sm := scs.New()
	sm.Store = pgxstore.New(cfg.Pool)
	sm.Lifetime = 30 * 24 * time.Hour
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	// scs defaults Secure to false, which would let the session cookie ride a
	// plaintext request — a redirect, a typed http:// URL, a stale link.
	sm.Cookie.Secure = !cfg.InsecureCookie

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(limitUploadBody)

	apiVersion := cfg.Version
	if apiVersion == "" {
		apiVersion = devVersion
	}
	humaCfg := huma.DefaultConfig("Sharespences API", apiVersion)
	if !cfg.Docs {
		// Empty DocsPath skips the docs route entirely (huma api.go).
		humaCfg.DocsPath = ""
	}
	api := humachi.New(r, humaCfg)
	api.UseMiddleware(requireSession(api, sm))

	authSvc := &auth.Service{Q: q}
	store := &attach.Store{Q: q, Dir: cfg.AttachmentsDir}
	cbSvc := &cashback.Service{Q: q, RemoveAttachmentFile: store.Remove, ReadAttachmentFile: store.Open, Vision: cfg.Vision}
	mccSvc := &mcc.Service{Q: q}
	frSvc := &friends.Service{Q: q, Pool: cfg.Pool}
	// Привилегии is self-contained: it reads bank and bank_client as reference
	// data and owns everything else it touches, so it needs no seam wiring.
	pvSvc := &perks.Service{Q: q}
	// The one function value crossing the friends→cashback seam (ADR-0002:
	// injected here at the composition root, never a package import).
	cbSvc.ListSharedWithMe = func(ctx context.Context, viewerID uuid.UUID) ([]cashback.SharedFriend, error) {
		rows, err := frSvc.SharedWithMe(ctx, viewerID)
		if err != nil {
			return nil, err
		}
		out := make([]cashback.SharedFriend, len(rows))
		for i, r := range rows {
			out[i] = cashback.SharedFriend{
				UserID: r.UserID, Username: r.Username, DisplayName: r.DisplayName,
				BankClientIDs: r.BankClientIDs,
			}
		}
		return out, nil
	}

	registerVersion(api, apiVersion)
	registerAuth(api, sm, authSvc)
	registerBanks(api, q)
	registerAttachments(api, store)
	cashback.RegisterHTTP(api, cbSvc)
	mcc.RegisterHTTP(api, mccSvc)
	friends.RegisterHTTP(api, frSvc)
	perks.RegisterHTTP(api, pvSvc)

	// Raw attachment bytes; outside the JSON API (and its OpenAPI doc).
	r.Get("/api/v1/attachments/{id}/content", func(w http.ResponseWriter, req *http.Request) {
		userID, ok := sessionUser(sm, req.Context())
		if !ok {
			http.Error(w, "нужно войти", http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(chi.URLParam(req, "id"))
		if err != nil {
			http.Error(w, "некорректный идентификатор файла", http.StatusBadRequest)
			return
		}
		a, err := store.Get(req.Context(), userID, id)
		if err != nil {
			http.Error(w, "не найдено", http.StatusNotFound)
			return
		}
		f, err := os.Open(store.Path(a.ID))
		if err != nil {
			http.Error(w, "файл не найден на диске", http.StatusInternalServerError)
			return
		}
		defer func() { _ = f.Close() }()
		// The stored media type is whatever the upload declared — huma's
		// validator trusts the part's header (see the upload op), so this is a
		// wrong-file guard, not a verified type. Serving it back from the app's
		// own origin therefore needs the two headers that keep a mislabelled
		// file from becoming active content: nosniff stops the browser
		// second-guessing the type, and anything that is not a known-safe image
		// downloads instead of rendering.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mediaType := "application/octet-stream"
		if a.MediaType != nil {
			mediaType = *a.MediaType
		}
		w.Header().Set("Content-Type", mediaType)
		if inlineImageTypes[mediaType] {
			w.Header().Set("Content-Disposition", "inline")
		} else {
			w.Header().Set("Content-Disposition", "attachment")
		}
		_, _ = io.Copy(w, f)
	})

	// Embedded SPA: catch-all for everything the API and docs don't claim.
	r.Handle("/*", web.Handler())

	return r, sm, api
}

// inlineImageTypes are the uploaded types the SPA renders in place (the
// Lightbox, W-04). Everything else the allowlist admits — application/pdf —
// is served as a download: a PDF opened inline runs its own scripting engine
// on this origin.
var inlineImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/heic": true,
}

func sessionUser(sm *scs.SessionManager, ctx context.Context) (uuid.UUID, bool) {
	idStr := sm.GetString(ctx, sessionUserKey)
	if idStr == "" {
		return uuid.UUID{}, false
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.UUID{}, false
	}
	return id, true
}

// requireSession guards every operation except register/login and stores the
// user id where auth.UserID finds it.
func requireSession(api huma.API, sm *scs.SessionManager) func(huma.Context, func(huma.Context)) {
	public := map[string]bool{
		"/api/v1/auth/register": true,
		"/api/v1/auth/login":    true,
		"/api/v1/version":       true,
	}
	return func(ctx huma.Context, next func(huma.Context)) {
		if public[ctx.Operation().Path] {
			next(ctx)
			return
		}
		id, ok := sessionUser(sm, ctx.Context())
		if !ok {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "нужно войти")
			return
		}
		next(huma.WithValue(ctx, auth.ContextKey, id))
	}
}

type UserDTO struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
}

func userDTO(u db.User) UserDTO {
	return UserDTO{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Email: u.Email}
}

func registerAuth(api huma.API, sm *scs.SessionManager, svc *auth.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "auth-register", Method: http.MethodPost,
		Path: "/api/v1/auth/register", Summary: "Register (and sign in)", Tags: []string{"auth"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		// The identity fields are normalized before validation (case folded,
		// whitespace trimmed), so the schema carries loose bounds as a body
		// guard only — auth's domain owns the real rule and its Russian
		// wording. Tighter bounds here would reject values that normalize to
		// something perfectly valid.
		Body struct {
			Username    string `json:"username" minLength:"1" maxLength:"64" doc:"3–32 символа: строчные латинские буквы и цифры, «.» и «_» внутри, начинается с буквы"`
			DisplayName string `json:"display_name" minLength:"1" maxLength:"256" doc:"отображаемое имя, до 64 символов"`
			Email       string `json:"email" format:"email"`
			Password    string `json:"password" minLength:"8"`
		}
	}) (*struct{ Body UserDTO }, error) {
		u, err := svc.Register(ctx, in.Body.Username, in.Body.DisplayName, in.Body.Email, in.Body.Password)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrEmailTaken):
				return nil, huma.Error409Conflict("почта или имя пользователя уже заняты")
			case errors.Is(err, auth.ErrUsernameFormat), errors.Is(err, auth.ErrUsernameReserved),
				errors.Is(err, auth.ErrDisplayNameLen):
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			return nil, err
		}
		if err := sm.RenewToken(ctx); err != nil {
			return nil, err
		}
		sm.Put(ctx, sessionUserKey, u.ID.String())
		return &struct{ Body UserDTO }{userDTO(u)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-login", Method: http.MethodPost,
		Path: "/api/v1/auth/login", Summary: "Sign in with email + password", Tags: []string{"auth"},
	}, func(ctx context.Context, in *struct {
		Body struct {
			Email    string `json:"email" format:"email"`
			Password string `json:"password" minLength:"1"`
		}
	}) (*struct{ Body UserDTO }, error) {
		u, err := svc.Login(ctx, in.Body.Email, in.Body.Password)
		if err != nil {
			return nil, huma.Error401Unauthorized("неверная почта или пароль")
		}
		if err := sm.RenewToken(ctx); err != nil {
			return nil, err
		}
		sm.Put(ctx, sessionUserKey, u.ID.String())
		return &struct{ Body UserDTO }{userDTO(u)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-logout", Method: http.MethodPost,
		Path: "/api/v1/auth/logout", Summary: "Sign out", Tags: []string{"auth"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		if err := sm.Destroy(ctx); err != nil {
			return nil, err
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-me", Method: http.MethodGet,
		Path: "/api/v1/auth/me", Summary: "Current user", Tags: []string{"auth"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body UserDTO }, error) {
		u, err := svc.Q.GetUserByID(ctx, auth.UserID(ctx))
		if err != nil {
			return nil, huma.Error401Unauthorized("нужно войти")
		}
		return &struct{ Body UserDTO }{userDTO(u)}, nil
	})
}

type BankDTO struct {
	ID       int32   `json:"id"`
	Name     string  `json:"name"`
	ColorHex *string `json:"color_hex,omitempty" doc:"brand color for UI tinting (seeded from the knowledge base)"`
}

// BankClientDTO is a person's relationship with one bank — держатель + tier
// live here; КБ selections are keyed by it (all the client's cards share
// them).
type BankClientDTO struct {
	ID            int64   `json:"id"`
	BankID        int32   `json:"bank_id"`
	BankName      string  `json:"bank_name,omitempty"`
	Label         *string `json:"label,omitempty" doc:"держатель («Мама»); omit for yourself"`
	ProgramTierID *int64  `json:"program_tier_id,omitempty"`
}

type CardDTO struct {
	ID            int32  `json:"id"`
	BankClientID  int64  `json:"bank_client_id"`
	BankID        int32  `json:"bank_id,omitempty"`
	BankName      string `json:"bank_name,omitempty"`
	Last4Digits   int32  `json:"last_4_digits"`
	PaymentSystem string `json:"payment_system"`
}

// VersionDTO identifies the running build. `api` is the path-versioned
// contract (ADR-0006 decision 2): the tag never promises API compatibility,
// so the two are reported separately and mean different things.
type VersionDTO struct {
	Version string `json:"version"`
	API     string `json:"api"`
}

// registerVersion answers «which build am I on?» — the question ADR-0006
// exists to make answerable for a user who is not the author. Public: it is
// not a secret, and a bug report may come from someone who cannot sign in.
func registerVersion(api huma.API, version string) {
	huma.Register(api, huma.Operation{
		OperationID: "meta-version", Method: http.MethodGet,
		Path: "/api/v1/version", Summary: "Running build version", Tags: []string{"meta"},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body VersionDTO }, error) {
		return &struct{ Body VersionDTO }{VersionDTO{Version: version, API: "v1"}}, nil
	})
}

func registerBanks(api huma.API, q *db.Queries) {
	huma.Register(api, huma.Operation{
		OperationID: "bank-list", Method: http.MethodGet,
		Path: "/api/v1/banks", Summary: "List banks", Tags: []string{"banks"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []BankDTO }, error) {
		rows, err := q.ListBanks(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]BankDTO, len(rows))
		for i, b := range rows {
			out[i] = BankDTO{ID: b.ID, Name: b.Name, ColorHex: b.ColorHex}
		}
		return &struct{ Body []BankDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "bank-client-create", Method: http.MethodPost,
		Path: "/api/v1/bank-clients", Summary: "Add a bank client (person × bank)", Tags: []string{"banks"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			BankID        int32   `json:"bank_id"`
			Label         *string `json:"label,omitempty" doc:"держатель («Мама»); omit for yourself"`
			ProgramTierID *int64  `json:"program_tier_id,omitempty"`
		}
	}) (*struct{ Body BankClientDTO }, error) {
		c, err := q.CreateBankClient(ctx, db.CreateBankClientParams{
			UserID: auth.UserID(ctx), BankID: in.Body.BankID,
			Label: in.Body.Label, ProgramTierID: in.Body.ProgramTierID,
		})
		if err != nil {
			if isPgCode(err, "23505") {
				return nil, huma.Error409Conflict("такой держатель у этого банка уже добавлен")
			}
			return nil, err
		}
		return &struct{ Body BankClientDTO }{BankClientDTO{
			ID: c.ID, BankID: c.BankID, Label: c.Label, ProgramTierID: c.ProgramTierID,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "bank-client-update", Method: http.MethodPut,
		Path: "/api/v1/bank-clients/{id}", Summary: "Edit a bank client (держатель, tier)", Tags: []string{"banks"},
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			Label         *string `json:"label,omitempty"`
			ProgramTierID *int64  `json:"program_tier_id,omitempty"`
		}
	}) (*struct{ Body BankClientDTO }, error) {
		c, err := q.UpdateBankClientForUser(ctx, db.UpdateBankClientForUserParams{
			ID: in.ID, UserID: auth.UserID(ctx),
			Label: in.Body.Label, ProgramTierID: in.Body.ProgramTierID,
		})
		if err != nil {
			if isPgCode(err, "23505") {
				return nil, huma.Error409Conflict("такой держатель у этого банка уже добавлен")
			}
			return nil, huma.Error404NotFound("не найдено")
		}
		return &struct{ Body BankClientDTO }{BankClientDTO{
			ID: c.ID, BankID: c.BankID, Label: c.Label, ProgramTierID: c.ProgramTierID,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "bank-client-list", Method: http.MethodGet,
		Path: "/api/v1/bank-clients", Summary: "List the user's bank clients", Tags: []string{"banks"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []BankClientDTO }, error) {
		rows, err := q.ListBankClientsForUser(ctx, auth.UserID(ctx))
		if err != nil {
			return nil, err
		}
		out := make([]BankClientDTO, len(rows))
		for i, c := range rows {
			out[i] = BankClientDTO{
				ID: c.ID, BankID: c.BankID, BankName: c.BankName,
				Label: c.Label, ProgramTierID: c.ProgramTierID,
			}
		}
		return &struct{ Body []BankClientDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "card-create", Method: http.MethodPost,
		Path: "/api/v1/cards", Summary: "Add a card to a bank client", Tags: []string{"banks"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			BankClientID  int64  `json:"bank_client_id"`
			Last4Digits   int32  `json:"last_4_digits" minimum:"0" maximum:"9999"`
			PaymentSystem string `json:"payment_system" enum:"visa,mastercard,mir,unionpay,american_express"`
		}
	}) (*struct{ Body CardDTO }, error) {
		c, err := q.CreateCard(ctx, db.CreateCardParams{
			BankClientID: in.Body.BankClientID, UserID: auth.UserID(ctx),
			Last4Digits:   in.Body.Last4Digits,
			PaymentSystem: db.PaymentSystem(in.Body.PaymentSystem),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, huma.Error404NotFound("банк не найден")
			}
			return nil, err
		}
		return &struct{ Body CardDTO }{CardDTO{
			ID: c.ID, BankClientID: c.BankClientID, Last4Digits: c.Last4Digits,
			PaymentSystem: string(c.PaymentSystem),
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "card-update", Method: http.MethodPut,
		Path: "/api/v1/cards/{id}", Summary: "Edit a card (last 4, payment system)", Tags: []string{"banks"},
	}, func(ctx context.Context, in *struct {
		ID   int32 `path:"id"`
		Body struct {
			Last4Digits   int32  `json:"last_4_digits" minimum:"0" maximum:"9999"`
			PaymentSystem string `json:"payment_system" enum:"visa,mastercard,mir,unionpay,american_express"`
		}
	}) (*struct{ Body CardDTO }, error) {
		c, err := q.UpdateCardForUser(ctx, db.UpdateCardForUserParams{
			ID: in.ID, UserID: auth.UserID(ctx),
			Last4Digits:   in.Body.Last4Digits,
			PaymentSystem: db.PaymentSystem(in.Body.PaymentSystem),
		})
		if err != nil {
			return nil, huma.Error404NotFound("не найдено")
		}
		return &struct{ Body CardDTO }{CardDTO{
			ID: c.ID, BankClientID: c.BankClientID, Last4Digits: c.Last4Digits,
			PaymentSystem: string(c.PaymentSystem),
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "card-delete", Method: http.MethodDelete,
		Path: "/api/v1/cards/{id}", Summary: "Delete a card", Tags: []string{"banks"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int32 `path:"id"`
	}) (*struct{}, error) {
		n, err := q.DeleteCardForUser(ctx, db.DeleteCardForUserParams{ID: in.ID, UserID: auth.UserID(ctx)})
		if err != nil {
			if isPgCode(err, "23503") {
				return nil, huma.Error409Conflict("карта используется в других записях")
			}
			return nil, err
		}
		if n == 0 {
			return nil, huma.Error404NotFound("не найдено")
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "bank-client-delete", Method: http.MethodDelete,
		Path: "/api/v1/bank-clients/{id}", Summary: "Delete a bank client (with its cards)", Tags: []string{"banks"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		n, err := q.DeleteBankClientForUser(ctx, db.DeleteBankClientForUserParams{ID: in.ID, UserID: auth.UserID(ctx)})
		if err != nil {
			if isPgCode(err, "23503") {
				return nil, huma.Error409Conflict("у банка есть история — периоды КБ, партнёрские предложения или периоды привилегий; сначала удали её")
			}
			return nil, err
		}
		if n == 0 {
			return nil, huma.Error404NotFound("не найдено")
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "card-list", Method: http.MethodGet,
		Path: "/api/v1/cards", Summary: "List the user's cards", Tags: []string{"banks"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []CardDTO }, error) {
		rows, err := q.ListCardsForUser(ctx, auth.UserID(ctx))
		if err != nil {
			return nil, err
		}
		out := make([]CardDTO, len(rows))
		for i, c := range rows {
			out[i] = CardDTO{
				ID: c.ID, BankClientID: c.BankClientID, BankID: c.BankID, BankName: c.BankName,
				Last4Digits: c.Last4Digits, PaymentSystem: string(c.PaymentSystem),
			}
		}
		return &struct{ Body []CardDTO }{out}, nil
	})
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// maxUploadBytes caps one attachment upload. The cap has to live on the
// transport: huma applies Operation.MaxBodyBytes only on the JSON body path,
// while the multipart branch parses the body itself, and MultipartMaxMemory
// is a spill-to-disk threshold rather than a limit. Sized for phone
// screenshots and receipt photos (the picker corpus runs 230–430 KB).
const maxUploadBytes = 10 << 20 // 10 MiB

const uploadPath = "/api/v1/attachments"

// limitUploadBody bounds the request body on the one route that streams
// bytes to disk. Over the limit, MaxBytesReader makes the read fail and the
// multipart parse returns 422 rather than filling the disk.
func limitUploadBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost && req.URL.Path == uploadPath {
			req.Body = http.MaxBytesReader(w, req.Body, maxUploadBytes)
		}
		next.ServeHTTP(w, req)
	})
}

func registerAttachments(api huma.API, store *attach.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "attachment-upload", Method: http.MethodPost,
		Path: uploadPath, Summary: "Upload a screenshot/evidence file", Tags: []string{"attachments"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		// contentType drives huma's MimeTypeValidator. It trusts the part's
		// declared Content-Type and only sniffs when the header is absent —
		// so this is a wrong-file guard, not a security boundary; the byte
		// cap above is what bounds the damage.
		RawBody huma.MultipartFormFiles[struct {
			File huma.FormFile `form:"file" contentType:"image/jpeg,image/png,image/webp,image/heic,application/pdf" required:"true"`
		}]
	}) (*struct {
		Body struct {
			ID       uuid.UUID `json:"id"`
			Filename string    `json:"filename"`
		}
	}, error) {
		data := in.RawBody.Data()
		a, err := store.Save(ctx, auth.UserID(ctx), data.File.Filename, data.File.ContentType, data.File)
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				ID       uuid.UUID `json:"id"`
				Filename string    `json:"filename"`
			}
		}{}
		out.Body.ID = a.ID
		out.Body.Filename = a.Filename
		return out, nil
	})
}

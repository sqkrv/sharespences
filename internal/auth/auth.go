// Package auth is the minimal password auth for cashback v1: email/password
// with argon2id hashes and
// DB-backed scs sessions. Magic links and token transport for mobile/bot
// clients wait for the auth spec; WebAuthn stays parked.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"

	"github.com/sqkrv/sharespences/internal/db"
)

var (
	ErrInvalidCredentials = errors.New("неверная почта или пароль")
	ErrEmailTaken         = errors.New("почта или имя пользователя уже заняты")
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword produces a PHC-formatted argon2id hash.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword checks password against a PHC argon2id hash.
func VerifyPassword(password, phc string) bool {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var mem, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, mem, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Service owns user registration and credential checks.
type Service struct {
	Q *db.Queries
}

// Register normalizes the identity fields before storing them (see domain.go):
// what lands in the database is the canonical form, which is what makes friend
// search a plain equality match.
func (s *Service) Register(ctx context.Context, username, displayName, email, password string) (db.User, error) {
	username = NormalizeUsername(username)
	if err := ValidateUsername(username); err != nil {
		return db.User{}, err
	}
	displayName = NormalizeDisplayName(displayName)
	if err := ValidateDisplayName(displayName); err != nil {
		return db.User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return db.User{}, err
	}
	u, err := s.Q.CreateUser(ctx, db.CreateUserParams{
		Username:     username,
		DisplayName:  displayName,
		Email:        NormalizeEmail(email),
		PasswordHash: &hash,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return db.User{}, ErrEmailTaken
		}
		return db.User{}, err
	}
	return u, nil
}

// dummyHash is verified against when the email is unknown, so that a miss
// costs the same ~100 ms of argon2 as a wrong password. Skipping the hash
// made account existence measurable from the response time alone — registration
// is open, so anyone can ask whether a given address has an account here.
// Generated once at init from a random password nothing can supply.
var dummyHash = func() string {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic(err) // crypto/rand failing at startup is not recoverable
	}
	h, err := HashPassword(string(secret))
	if err != nil {
		panic(err)
	}
	return h
}()

func (s *Service) Login(ctx context.Context, email, password string) (db.User, error) {
	u, err := s.Q.GetUserByEmail(ctx, NormalizeEmail(email))
	if err != nil || u.PasswordHash == nil {
		VerifyPassword(password, dummyHash)
		return db.User{}, ErrInvalidCredentials
	}
	if !VerifyPassword(password, *u.PasswordHash) {
		return db.User{}, ErrInvalidCredentials
	}
	return u, nil
}

func isUniqueViolation(err error) bool {
	// pgconn.PgError code 23505; matched by string to avoid importing pgconn here.
	return err != nil && strings.Contains(err.Error(), "23505")
}

type ctxKey struct{}

// ContextKey is the context key the session middleware stores the user id
// under (usable with huma.WithValue, which needs the key itself).
var ContextKey any = ctxKey{}

// WithUserID stores the authenticated user in a request context.
func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// UserID returns the authenticated user id; the zero UUID means unauthenticated.
func UserID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(ctxKey{}).(uuid.UUID)
	return id
}

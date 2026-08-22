// Package auth implements local-user authentication for the Go control plane:
// bcrypt password hashing, HMAC-signed session tokens, first-run admin setup,
// and an HTTP middleware that gates the API (setup-mode until the first user
// exists, then token-required). Tokens are stateless (HMAC); logout is
// client-side.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/flowforge/flowforge/internal/store"
	"github.com/flowforge/flowforge/internal/util"
)

const secretSetting = "auth_secret"

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

// HashPassword returns a bcrypt hash of the password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword reports whether the password matches the hash.
func VerifyPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// ValidCredentials enforces basic username/password rules.
func ValidCredentials(username, password string) bool {
	return usernameRe.MatchString(username) && len(password) >= 6
}

// Claims is the token payload.
type Claims struct {
	Sub string `json:"sub"` // user id
	U   string `json:"u"`   // username
	R   string `json:"r"`   // role
	Exp int64  `json:"exp"` // unix expiry
}

func serverSecret(s *store.Store) string {
	if v, ok, _ := s.GetSetting(secretSetting); ok && v != "" {
		return v
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	sec := base64.StdEncoding.EncodeToString(b)
	_ = s.SetSetting(secretSetting, sec)
	return sec
}

// MakeToken returns an HMAC-signed token valid for 7 days.
func MakeToken(s *store.Store, u store.UserRow) (string, error) {
	claims := Claims{Sub: u.ID, U: u.Username, R: u.Role, Exp: time.Now().Add(7 * 24 * time.Hour).Unix()}
	body, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(serverSecret(s)))
	mac.Write([]byte(enc))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return enc + "." + sig, nil
}

// ParseToken validates the signature and expiry.
func ParseToken(s *store.Store, token string) (*Claims, bool) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, false
	}
	mac := hmac.New(sha256.New, []byte(serverSecret(s)))
	mac.Write([]byte(parts[0]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return nil, false
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, false
	}
	var c Claims
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, false
	}
	if c.Exp > 0 && time.Now().Unix() > c.Exp {
		return nil, false
	}
	return &c, true
}

// Status reports whether first-run setup is needed and whether auth is required.
type Status struct {
	SetupRequired bool `json:"setupRequired"`
	AuthRequired  bool `json:"authRequired"`
}

// StatusOf derives the auth status from the user count.
func StatusOf(s *store.Store) Status {
	n, _ := s.CountUsers()
	return Status{SetupRequired: n == 0, AuthRequired: n > 0}
}

type claimsKey struct{}

// ClaimsFrom extracts the token claims from the request context (nil if absent).
func ClaimsFrom(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey{}).(*Claims)
	return c
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func isPublic(path string) bool {
	return path == "/api/v1/health" || strings.HasPrefix(path, "/api/v1/auth/")
}

// Wrap gates the API. mode "off" disables auth (dev / Node parity); "auto"
// (default) enables setup-mode gating then token-required access. A valid token
// is always attached to the context when present (so /auth/me works).
func Wrap(s *store.Store, mode string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == "off" {
			next.ServeHTTP(w, r)
			return
		}
		var claims *Claims
		if tok := bearer(r); tok != "" {
			claims, _ = ParseToken(s, tok)
		}
		if claims != nil {
			r = r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims))
		}
		if isPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		n, _ := s.CountUsers()
		if n == 0 {
			// Setup mode: the app is locked until an admin is created.
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "setup required", "setupRequired": true})
			return
		}
		if claims == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CreateUser persists a new user with a hashed password.
func CreateUser(s *store.Store, username, password, role string) (store.UserRow, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return store.UserRow{}, err
	}
	u := store.UserRow{ID: "u-" + util.UID(), Username: username, PasswordHash: hash, Role: role, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := s.AddUser(u); err != nil {
		return store.UserRow{}, err
	}
	return u, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

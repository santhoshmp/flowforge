package auth

// SEC-01b/c/d: token integrity + middleware gating internals. Feature F-SEC.
// (The HTTP-level flow lives in api/auth_test.go; these pin the primitives.)

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flowforge/flowforge/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// SEC-01b: make → parse roundtrip; tampered body/signature rejected.
func TestSEC01b_TokenTamperRejection(t *testing.T) {
	s := testStore(t)
	u := store.UserRow{ID: "u-1", Username: "ada", Role: "admin"}
	tok, err := MakeToken(s, u)
	if err != nil {
		t.Fatal(err)
	}

	claims, ok := ParseToken(s, tok)
	if !ok || claims.Sub != "u-1" || claims.U != "ada" || claims.R != "admin" {
		t.Fatalf("roundtrip failed: %+v ok=%v", claims, ok)
	}

	// Tamper with the claims body (keep a syntactically valid shape).
	parts := strings.SplitN(tok, ".", 2)
	body, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	m["r"] = "superadmin"
	forged, _ := json.Marshal(m)
	forgedTok := base64.RawURLEncoding.EncodeToString(forged) + "." + parts[1]
	if _, ok := ParseToken(s, forgedTok); ok {
		t.Fatal("forged claims must not verify")
	}

	// Flip a signature byte.
	sig := []byte(parts[1])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	if _, ok := ParseToken(s, parts[0]+"."+string(sig)); ok {
		t.Fatal("tampered signature must not verify")
	}

	// Garbage shapes.
	for _, bad := range []string{"", "only", "a.b.c"} {
		if _, ok := ParseToken(s, bad); ok {
			t.Fatalf("garbage token %q verified", bad)
		}
	}

	// A token minted against a different store (different HMAC secret) fails.
	s2 := testStore(t)
	if _, ok := ParseToken(s2, tok); ok {
		t.Fatal("cross-server token must not verify")
	}
}

// SEC-01c: expired tokens are rejected.
func TestSEC01c_TokenExpiry(t *testing.T) {
	s := testStore(t)
	// Craft claims that are already expired, signed with the real server secret.
	enc := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u-1","u":"ada","r":"admin","exp":1000}`))
	mac := hmac.New(sha256.New, []byte(serverSecret(s)))
	mac.Write([]byte(enc))
	tok := enc + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if _, ok := ParseToken(s, tok); ok {
		t.Fatal("expired token must not verify")
	}
	// Same token with a future exp verifies (control).
	enc2 := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u-1","u":"ada","r":"admin","exp":` + jsonInt64(time.Now().Add(time.Hour).Unix()) + `}`))
	mac2 := hmac.New(sha256.New, []byte(serverSecret(s)))
	mac2.Write([]byte(enc2))
	if _, ok := ParseToken(s, enc2+"."+base64.RawURLEncoding.EncodeToString(mac2.Sum(nil))); !ok {
		t.Fatal("future-exp token should verify")
	}
}

func jsonInt64(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// SEC-01d: Wrap gating in every mode against a probe handler.
func TestSEC01d_WrapModes(t *testing.T) {
	probe := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(299) }) // sentinel status
	hit := func(mode, path, bearer string) int {
		s := testStore(t)
		h := Wrap(s, mode, probe)
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	// mode=off: everything passes through.
	if c := hit("off", "/api/v1/workflows", ""); c != 299 {
		t.Fatalf("off mode should pass through, got %d", c)
	}

	// auto + no users: setup lock (403) on app routes, but /health + /auth/* stay open.
	if c := hit("auto", "/api/v1/workflows", ""); c != http.StatusForbidden {
		t.Fatalf("setup mode should 403 app routes, got %d", c)
	}
	for _, pub := range []string{"/api/v1/health", "/api/v1/auth/status", "/api/v1/auth/login"} {
		if c := hit("auto", pub, ""); c != 299 {
			t.Fatalf("public path %s should pass in setup mode, got %d", pub, c)
		}
	}

	// auto + a user: app routes need a valid token.
	s := testStore(t)
	if err := s.AddUser(store.UserRow{ID: "u-1", Username: "ada", PasswordHash: "x", Role: "admin", CreatedAt: "t"}); err != nil {
		t.Fatal(err)
	}
	tok, _ := MakeToken(s, store.UserRow{ID: "u-1", Username: "ada", Role: "admin"})
	do := func(path, bearer string) int {
		h := Wrap(s, "auto", probe)
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := do("/api/v1/workflows", ""); c != http.StatusUnauthorized {
		t.Fatalf("missing token should 401, got %d", c)
	}
	if c := do("/api/v1/workflows", "not-a-token"); c != http.StatusUnauthorized {
		t.Fatalf("invalid token should 401, got %d", c)
	}
	if c := do("/api/v1/workflows", tok); c != 299 {
		t.Fatalf("valid token should pass, got %d", c)
	}
	if c := do("/api/v1/health", ""); c != 299 {
		t.Fatalf("health stays public with users, got %d", c)
	}
}

// SEC-01e: credential rules + bcrypt verify roundtrip.
func TestSEC01e_Credentials(t *testing.T) {
	if ValidCredentials("ab", "secret123") {
		t.Error("short username accepted")
	}
	if ValidCredentials("bad name!", "secret123") {
		t.Error("invalid chars accepted")
	}
	if ValidCredentials("ada", "short") {
		t.Error("short password accepted")
	}
	if !ValidCredentials("ada.lovelace-1", "secret123") {
		t.Error("valid credentials rejected")
	}
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "secret123") {
		t.Error("correct password failed verify")
	}
	if VerifyPassword(hash, "wrong") {
		t.Error("wrong password verified")
	}
	if hash == "secret123" || strings.Contains(hash, "secret123") {
		t.Error("bcrypt hash leaks plaintext")
	}
}

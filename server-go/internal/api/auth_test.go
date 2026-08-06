package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowforge/flowforge/internal/store"
)

// P2 auth flow (feature F-SEC). Mode "auto": setup-mode gating then token-required.

func newAuthServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.SeedIfEmpty(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return httptest.NewServer(New(st, "auto")), st
}

func TestAuth_SetupModeThenLoginAndProtected(t *testing.T) {
	hs, st := newAuthServer(t)
	defer hs.Close()
	defer st.Close()

	// 1. Initially in setup mode.
	if code, b := req(t, hs, "GET", "/api/v1/auth/status", nil); code != 200 {
		t.Fatalf("status %d %s", code, b)
	}

	// 2. App routes are locked until an admin exists.
	if code, _ := req(t, hs, "GET", "/api/v1/bootstrap", nil); code != 403 {
		t.Fatalf("setup mode should lock app routes, got %d", code)
	}

	// 3. First-run setup creates the admin and returns a token.
	code, b := req(t, hs, "POST", "/api/v1/auth/setup", map[string]string{"username": "admin", "password": "secret123"})
	if code != 200 {
		t.Fatalf("setup %d %s", code, b)
	}
	setup := asMap(b)
	token, _ := setup["token"].(string)
	if token == "" {
		t.Fatalf("no token in setup response")
	}

	// 4. Setup cannot run twice.
	if code, _ := req(t, hs, "POST", "/api/v1/auth/setup", map[string]string{"username": "other", "password": "secret123"}); code != 409 {
		t.Fatalf("second setup should be 409, got %d", code)
	}

	// 5. Without a token, app routes are 401.
	if code, _ := req(t, hs, "GET", "/api/v1/bootstrap", nil); code != 401 {
		t.Fatalf("want 401 without token, got %d", code)
	}

	// 6. With the token, app routes work.
	authReq(t, hs, "GET", "/api/v1/bootstrap", token, nil)

	// 7. /auth/me returns the admin.
	code, b = authReq(t, hs, "GET", "/api/v1/auth/me", token, nil)
	if asMap(b)["username"] != "admin" {
		t.Fatalf("me mismatch: %s", b)
	}

	// 8. Bad password -> 401; good password -> token.
	if code, _ := req(t, hs, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "wrong"}); code != 401 {
		t.Fatalf("bad login should be 401, got %d", code)
	}
	code, b = req(t, hs, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "secret123"})
	if code != 200 || asMap(b)["token"] == nil {
		t.Fatalf("login failed: %d %s", code, b)
	}

	// 9. Setup rejects weak credentials (still 0 users is false now, so this is 409;
	//    verify credential validation indirectly via a fresh server).
	hs2, st2 := newAuthServer(t)
	defer hs2.Close()
	defer st2.Close()
	if code, _ := req(t, hs2, "POST", "/api/v1/auth/setup", map[string]string{"username": "a", "password": "short"}); code != 400 {
		t.Fatalf("weak creds should be 400, got %d", code)
	}
}

// authReq issues a request with a Bearer token and returns (status, body).
func authReq(t *testing.T, hs *httptest.Server, method, path, token string, body any) (int, []byte) {
	t.Helper()
	var rdr = func() *bytes.Reader {
		if body == nil {
			return bytes.NewReader(nil)
		}
		b, _ := json.Marshal(body)
		return bytes.NewReader(b)
	}()
	r, err := http.NewRequest(method, hs.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

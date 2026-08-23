// Package e2e drives the actual flowforge binary: CLI subcommands and a real
// `serve` process (HTTP + scheduler + SQLite on disk). This is the product
// test — if this suite is green, a user's first hour works.
//
// E2E-01..08 (see docs/test-strategy.md). The binary is built once in
// TestMain; `go test ./e2e` takes ~15-25s, so it is kept out of the unit
// packages.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "flowforge-e2e-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempdir:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	bin = filepath.Join(dir, "flowforge.exe")
	if runtimeGOOS() != "windows" {
		bin = filepath.Join(dir, "flowforge")
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/flowforge")
	build.Dir = mustRepoServerGo()
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s\n", err, out)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func mustRepoServerGo() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Clean(filepath.Join(wd, ".."))
}

func runtimeGOOS() string {
	if os.PathSeparator == '\\' {
		return "windows"
	}
	return "linux"
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// E2E-01: version prints the build stamp.
func TestE2E01_Version(t *testing.T) {
	out, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v\n%s", err, out)
	}
	if !strings.Contains(out, "flowforge") {
		t.Fatalf("unexpected version output: %q", out)
	}
}

// E2E-02: validate accepts a gallery artifact and rejects a broken one.
func TestE2E02_ValidateCLI(t *testing.T) {
	good := filepath.Join(mustRepoServerGo(), "internal", "templates", "gallery", "vendor-invoice-approval.flow.yaml")
	out, err := run(t, "validate", good)
	if err != nil {
		t.Fatalf("validate good: %v\n%s", err, out)
	}
	if !strings.Contains(out, "valid") {
		t.Fatalf("output: %q", out)
	}

	bad := filepath.Join(t.TempDir(), "bad.flow.yaml")
	if err := os.WriteFile(bad, []byte("apiVersion: flowforge/v1\nkind: Workflow\nmetadata:\n  name: Bad!\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "validate", bad)
	if err == nil {
		t.Fatalf("invalid artifact accepted: %q", out)
	}

	// run prints the execution plan for a valid artifact.
	out, err = run(t, "run", good)
	if err != nil || !strings.Contains(out, "plan:") {
		t.Fatalf("run: %v %q", err, out)
	}
}

// E2E-03: connectors CLI lists built-ins; connector validate checks a drop-in.
func TestE2E03_ConnectorsCLI(t *testing.T) {
	out, err := run(t, "connectors")
	if err != nil {
		t.Fatalf("connectors: %v\n%s", err, out)
	}
	for _, id := range []string{"http-json", "slack-webhook", "smtp"} {
		if !strings.Contains(out, id) {
			t.Fatalf("built-in %s missing from listing", id)
		}
	}

	dir := t.TempDir()
	conn := filepath.Join(dir, "my-conn")
	if err := os.MkdirAll(conn, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: my-conn\nname: My\nversion: 1.0.0\nexecutor: http\nhttp:\n  url: \"${params.url}\"\n"
	if err := os.WriteFile(filepath.Join(conn, "connector.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, "connector", "validate", conn)
	if err != nil || !strings.Contains(out, "valid") {
		t.Fatalf("connector validate: %v %q", err, out)
	}
	// Broken manifest fails.
	if err := os.WriteFile(filepath.Join(conn, "connector.yaml"), []byte("id: x\nname: X\nversion: 1\nexecutor: http\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = run(t, "connector", "validate", conn); err == nil {
		t.Fatal("invalid connector accepted")
	}
}

// E2E-04: artifact signing roundtrip through the CLI (keygen → sign → verify
// → tamper rejected).
func TestE2E04_SigningCLI(t *testing.T) {
	dir := t.TempDir()
	if out, err := run(t, "keygen", dir); err != nil || !strings.Contains(out, "private key") {
		t.Fatalf("keygen: %v %q", err, out)
	}
	// keygen refuses to overwrite.
	if _, err := run(t, "keygen", dir); err == nil {
		t.Fatal("keygen must refuse to overwrite existing keys")
	}

	artifact := filepath.Join(dir, "flow.flow.yaml")
	if err := os.WriteFile(artifact, []byte("apiVersion: flowforge/v1\nkind: Workflow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, "sign", artifact, "--key", filepath.Join(dir, "flowforge.key")); err != nil || !strings.Contains(out, "signed") {
		t.Fatalf("sign: %v %q", err, out)
	}
	if out, err := run(t, "verify", artifact, "--key", filepath.Join(dir, "flowforge.key.pub")); err != nil || !strings.Contains(out, "verified") {
		t.Fatalf("verify: %v %q", err, out)
	}
	if err := os.WriteFile(artifact, []byte("apiVersion: flowforge/v1\nkind: Workflow\n# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "verify", artifact, "--key", filepath.Join(dir, "flowforge.key.pub")); err == nil {
		t.Fatal("tampered artifact verified")
	}
}

// ---- serve: the full product loop over real HTTP ------------------------------

type server struct {
	base string
	tok  string
	dir  string
	proc *exec.Cmd
	t    *testing.T
}

func startServe(t *testing.T) *server {
	t.Helper()
	dir := t.TempDir()
	port := freePort(t)

	cmd := exec.Command(bin, "serve")
	cmd.Dir = dir // DB, secrets, and keys land in the temp dir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		"DB_PATH=flowforge.db",
		"FLOWFORGE_SECRETS_FILE=flowforge.secrets",
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &server{base: fmt.Sprintf("http://127.0.0.1:%d", port), dir: dir, proc: cmd, t: t}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	waitHealthy(t, s)
	return s
}

func (s *server) call(method, path string, body any, authed bool) (int, []byte) {
	s.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.base+path, rdr)
	if err != nil {
		s.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authed {
		req.Header.Set("Authorization", "Bearer "+s.tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// pollInstance GETs until the status matches or the deadline passes.
func (s *server) pollInstance(id string, want map[string]bool, timeout time.Duration) map[string]any {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, b := s.call("GET", "/api/v1/executions/"+id, nil, true)
		inst := map[string]any{}
		_ = json.Unmarshal(b, &inst)
		if status, _ := inst["status"].(string); want[status] {
			return inst
		}
		time.Sleep(300 * time.Millisecond)
	}
	s.t.Fatalf("instance %s did not reach %v within %s", id, want, timeout)
	return nil
}

// E2E-05: first-run auth — setup lock, admin creation, 401s, health public.
func TestE2E05_AuthGatingOverHTTP(t *testing.T) {
	s := startServe(t)

	// Pre-setup: app routes are locked with the setup signal.
	code, b := s.call("GET", "/api/v1/bootstrap", nil, false)
	if code != 403 || !strings.Contains(string(b), "setupRequired") {
		t.Fatalf("setup lock: %d %s", code, b)
	}
	// Health stays public.
	if code, _ := s.call("GET", "/api/v1/health", nil, false); code != 200 {
		t.Fatal("health must stay public")
	}

	// Create the admin (first-run setup).
	code, b = s.call("POST", "/api/v1/auth/setup", map[string]string{"username": "e2e-admin", "password": "e2e-pass-1"}, false)
	if code != 200 {
		t.Fatalf("setup: %d %s", code, b)
	}
	s.tok, _ = mapTok(b)

	// Second setup 409; wrong login 401; app routes need the token.
	if code, _ = s.call("POST", "/api/v1/auth/setup", map[string]string{"username": "x", "password": "y"}, false); code != 409 {
		t.Fatalf("second setup: %d", code)
	}
	if code, _ = s.call("POST", "/api/v1/auth/login", map[string]string{"username": "e2e-admin", "password": "wrong"}, false); code != 401 {
		t.Fatalf("bad login: %d", code)
	}
	if code, _ = s.call("GET", "/api/v1/workflows", nil, false); code != 401 {
		t.Fatalf("unauthenticated: %d", code)
	}
	if code, _ = s.call("GET", "/api/v1/workflows", nil, true); code != 200 {
		t.Fatalf("authenticated: %d", code)
	}
	// /auth/me reflects the session.
	_, b = s.call("GET", "/api/v1/auth/me", nil, true)
	if !strings.Contains(string(b), "e2e-admin") {
		t.Fatalf("me: %s", b)
	}
}

func mapTok(b []byte) (string, bool) {
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	tok, _ := m["token"].(string)
	return tok, tok != ""
}

// E2E-06: the UI is embedded and served (index + SPA fallback).
func TestE2E06_EmbeddedUI(t *testing.T) {
	s := startServe(t)
	_, b := s.call("POST", "/api/v1/auth/setup", map[string]string{"username": "ui-admin", "password": "ui-pass-1"}, false)
	s.tok, _ = mapTok(b)
	if s.tok == "" {
		t.Fatal("no token from setup")
	}

	resp, err := http.Get(s.base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "<div id=\"root\"") {
		t.Fatalf("UI index not served: %d %.120s", resp.StatusCode, body)
	}
	// SPA fallback for a client-side route.
	resp2, err := http.Get(s.base + "/some/deep/route")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("SPA fallback: %d", resp2.StatusCode)
	}
}

// E2E-07: FULL loop on a live server with the real scheduler: template →
// approve → run → WAIT (engine advances by itself) → approve task → COMPLETE.
func TestE2E07_FullLoopOnLiveServer(t *testing.T) {
	s := startServe(t)
	_, b := s.call("POST", "/api/v1/auth/setup", map[string]string{"username": "loop-admin", "password": "loop-pass-1"}, false)
	s.tok, _ = mapTok(b)
	if s.tok == "" {
		t.Fatal("no token from setup")
	}

	// Connectors + templates + secrets are live over HTTP.
	code, b := s.call("GET", "/api/v1/connectors", nil, true)
	if code != 200 || !strings.Contains(string(b), "http-json") {
		t.Fatalf("connectors over HTTP: %d", code)
	}
	code, b = s.call("GET", "/api/v1/templates", nil, true)
	if code != 200 || !strings.Contains(string(b), "vendor-invoice-approval") {
		t.Fatalf("templates over HTTP: %d", code)
	}
	if code, _ = s.call("PUT", "/api/v1/secrets", map[string]string{"name": "E2E_PROBE", "value": "s3cret"}, true); code != 200 {
		t.Fatal("secrets PUT failed")
	}
	_, b = s.call("GET", "/api/v1/secrets", nil, true)
	if !strings.Contains(string(b), "E2E_PROBE") || strings.Contains(string(b), "s3cret") {
		t.Fatalf("secrets listing must show names only: %s", b)
	}

	// Template → draft → approve → run ABOVE threshold.
	code, b = s.call("POST", "/api/v1/templates/vendor-invoice-approval/instantiate", nil, true)
	if code != 200 {
		t.Fatalf("instantiate: %d %s", code, b)
	}
	wf := map[string]any{}
	_ = json.Unmarshal(b, &wf)
	wfID := wf["id"].(string)

	if code, _ = s.call("POST", "/api/v1/workflows/"+wfID+"/approve", nil, true); code != 200 {
		t.Fatal("approve failed")
	}
	code, b = s.call("POST", "/api/v1/workflows/"+wfID+"/executions", map[string]any{
		"entity": "E2E-INV-001", "input": map[string]any{"total": 50000},
	}, true)
	if code != 200 {
		t.Fatalf("run: %d %s", code, b)
	}
	inst := map[string]any{}
	_ = json.Unmarshal(b, &inst)
	instID := inst["id"].(string)

	// The scheduler advances the engine by itself — no test-side ticking.
	waiting := s.pollInstance(instID, map[string]bool{"waiting": true}, 20*time.Second)
	if waiting["waitingOn"] != "Cost-Center Manager" {
		t.Fatalf("waitingOn = %v, want Cost-Center Manager", waiting["waitingOn"])
	}

	if code, _ = s.call("POST", "/api/v1/executions/"+instID+"/approve", nil, true); code != 200 {
		t.Fatal("task approve failed")
	}
	done := s.pollInstance(instID, map[string]bool{"completed": true}, 20*time.Second)
	if done["endedAt"] == nil || done["endedAt"] == "" {
		t.Fatalf("completed instance must record endedAt: %v", done["endedAt"])
	}

	// Restart durability: the completed instance + workflow survive a fresh
	// process on the same DB dir.
	_ = s.proc.Process.Kill()
	_, _ = s.proc.Process.Wait()

	cmd2 := exec.Command(bin, "serve")
	cmd2.Dir = s.dir
	port2 := freePort(t)
	cmd2.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port2),
		"DB_PATH=flowforge.db",
		"FLOWFORGE_SECRETS_FILE=flowforge.secrets",
	)
	if err := cmd2.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd2.Process.Kill(); _, _ = cmd2.Process.Wait() })
	s2 := &server{base: fmt.Sprintf("http://127.0.0.1:%d", port2), dir: s.dir, proc: cmd2, t: t}
	waitHealthy(t, s2)

	_, b = s2.call("POST", "/api/v1/auth/login", map[string]string{"username": "loop-admin", "password": "loop-pass-1"}, false)
	s2.tok, _ = mapTok(b)
	if s2.tok == "" {
		t.Fatal("login after restart failed")
	}
	code, b = s2.call("GET", "/api/v1/executions/"+instID, nil, true)
	if code != 200 || !strings.Contains(string(b), "completed") {
		t.Fatalf("instance did not survive restart: %d %s", code, b)
	}
	code, b = s2.call("GET", "/api/v1/workflows/"+wfID, nil, true)
	if code != 200 || !strings.Contains(string(b), "Acme") && !strings.Contains(string(b), "Vendor") {
		t.Fatalf("workflow did not survive restart: %d %s", code, b)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitHealthy(t *testing.T, s *server) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(s.base + "/api/v1/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("server did not become healthy")
}

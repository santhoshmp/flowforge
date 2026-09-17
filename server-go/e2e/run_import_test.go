package e2e

// E2E-08/09: the standalone runner and artifact import on the built binary.
// Features F-RUNNER + F-IMPORT.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var e2eArtifact = `apiVersion: flowforge/v1
kind: Workflow
metadata:
  name: e2e-standalone
  version: 1
  createdBy: e2e
spec:
  description: threshold routing
  trigger:
    event: invoice.created
  steps:
    - id: amount_check
      type: condition
      name: Over 100?
      params:
        expression: total > 100
        on_false: auto_approve
    - id: mgr
      type: human.approval
      name: Manager approval
      params:
        approver: Manager
`

// runIn runs the binary with CWD=dir (so DB files land in the temp dir).
func runIn(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// exitCode extracts the process exit code from an exec error (0 if err nil).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

// serveOn starts `flowforge serve` on port with the given data dir.
func serveOn(t *testing.T, dir string, port int) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin, "serve")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		"DB_PATH=flowforge.db",
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd
}

func writeArtifact(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "standalone.flow.yaml")
	if err := os.WriteFile(p, []byte(e2eArtifact), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// E2E-08: `flowforge run` executes standalone — completed (auto-approve,
// exit 0), waiting (no resolver, exit 3), and --plan preview.
func TestE2E08_StandaloneRun(t *testing.T) {
	dir := t.TempDir()
	art := writeArtifact(t, dir)
	in := filepath.Join(dir, "input.json")
	_ = os.WriteFile(in, []byte(`{"input": {"total": 500}}`), 0o644)

	// Above threshold with --auto-approve -> completed, exit 0.
	out, err := runIn(t, dir, "run", art, "--input", in, "--auto-approve")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "completed") || !strings.Contains(out, "auto-approved Manager") {
		t.Fatalf("unexpected output:\n%s", out)
	}

	// Above threshold without a resolver -> waiting, exit 3.
	out, err = runIn(t, dir, "run", art, "--input", in)
	if err == nil || !strings.Contains(out, "waiting") {
		t.Fatalf("expected waiting + non-zero exit:\n%s err=%v", out, err)
	}
	if code := exitCode(err); code != 3 {
		t.Fatalf("waiting exit code = %d, want 3", code)
	}

	// --plan stays a preview.
	out, err = runIn(t, dir, "run", art, "--plan")
	if err != nil || !strings.Contains(out, "plan:") {
		t.Fatalf("plan: %v\n%s", err, out)
	}
}

// E2E-09: `flowforge import` loads an artifact into a DB; a server on the
// same DB serves it (draft, refusable until approved).
func TestE2E09_ImportThenServe(t *testing.T) {
	dir := t.TempDir()
	art := writeArtifact(t, dir)

	out, err := runIn(t, dir, "import", art)
	if err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if !strings.Contains(out, "imported") {
		t.Fatalf("unexpected import output:\n%s", out)
	}
	// Grab the printed id (…id wf-XXXX …).
	var wfID string
	for _, f := range strings.Fields(out) {
		if strings.HasPrefix(f, "wf-") {
			wfID = strings.Trim(f, ",()")
		}
	}
	if wfID == "" {
		t.Fatalf("no workflow id in output:\n%s", out)
	}

	// Serve on the same DB (import wrote flowforge.db inside dir because
	// the command ran with CWD=dir).
	port := freePort(t)
	_ = serveOn(t, dir, port)
	s := &server{base: fmt.Sprintf("http://127.0.0.1:%d", port), dir: dir, t: t}
	waitHealthy(t, s)

	_, b := s.call("POST", "/api/v1/auth/setup", map[string]string{"username": "imp-admin", "password": "imp-pass-1"}, false)
	s.tok, _ = mapTok(b)

	code, b := s.call("GET", "/api/v1/workflows/"+wfID, nil, true)
	if code != 200 || !strings.Contains(string(b), "E2e Standalone") {
		t.Fatalf("imported workflow not served: %d %s", code, b)
	}
	wf := map[string]any{}
	_ = json.Unmarshal(b, &wf)
	if wf["status"] != "draft" {
		t.Fatalf("import must leave a draft, got %v", wf["status"])
	}
	if code, _ = s.call("POST", "/api/v1/workflows/"+wfID+"/executions", nil, true); code != 400 {
		t.Fatal("imported draft must not execute before approval")
	}
}

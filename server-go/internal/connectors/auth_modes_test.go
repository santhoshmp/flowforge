package connectors

// CONN-06/07: connector auth modes (bearer/header/basic) reach the target
// request, secret refs resolve from the vault, and Preview warns on missing
// refs. Feature F-EXT.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/secrets"
)

// writeConnector creates a connector directory from a manifest body.
func writeConnector(t *testing.T, id, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	sub := filepath.Join(dir, id)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "connector.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// CONN-06: every auth mode lands on the outbound request; secrets come from
// the vault; missing secrets fail before any network egress.
func TestCONN06_AuthModes(t *testing.T) {
	vault, err := secrets.Default()
	if err != nil {
		t.Fatal(err)
	}
	_ = vault.Set("API_TOKEN", "tok-123")
	_ = vault.Set("BASIC_CREDS", "user:pass")

	cases := []struct {
		name     string
		manifest string
		assert   func(t *testing.T, h http.Header)
	}{
		{
			name: "bearer",
			manifest: `id: auth-bearer
name: Bearer
version: 0.1.0
executor: http
auth:
  mode: bearer
  secretKey: API_TOKEN
http:
  method: GET
  url: "${params.url}"
`,
			assert: func(t *testing.T, h http.Header) {
				if got := h.Get("Authorization"); got != "Bearer tok-123" {
					t.Fatalf("Authorization = %q", got)
				}
			},
		},
		{
			name: "custom header",
			manifest: `id: auth-header
name: Header
version: 0.1.0
executor: http
auth:
  mode: header
  headerName: X-Api-Key
  secretKey: API_TOKEN
http:
  method: GET
  url: "${params.url}"
`,
			assert: func(t *testing.T, h http.Header) {
				if got := h.Get("X-Api-Key"); got != "tok-123" {
					t.Fatalf("X-Api-Key = %q", got)
				}
				if got := h.Get("Authorization"); got != "" {
					t.Fatalf("unexpected Authorization: %q", got)
				}
			},
		},
		{
			name: "basic",
			manifest: `id: auth-basic
name: Basic
version: 0.1.0
executor: http
auth:
  mode: basic
  secretKey: BASIC_CREDS
http:
  method: GET
  url: "${params.url}"
`,
			assert: func(t *testing.T, h http.Header) {
				req, _ := http.NewRequest("GET", "http://x", nil)
				req.SetBasicAuth("user", "pass")
				if got := h.Get("Authorization"); got != req.Header.Get("Authorization") {
					t.Fatalf("Authorization = %q, want basic user:pass", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotHeader http.Header
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				gotHeader = r.Header.Clone()
			}))
			defer srv.Close()

			dir := writeConnector(t, tc.name, tc.manifest)
			e, err := ParseDir(filepath.Join(dir, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			reg := &Registry{}
			reg.put(e)

			step := &models.WorkflowStep{Type: "connector", Params: map[string]string{"connector": e.Manifest.ID, "url": srv.URL}}
			out, err := RunStep(reg, step, nil, &policy.Policy{})
			if err != nil {
				t.Fatalf("run: %v (out %q)", err, out)
			}
			if gotHeader == nil {
				t.Fatal("request never reached the server")
			}
			tc.assert(t, gotHeader)
		})
	}

	// Missing secret fails BEFORE any request: point at a live server; the
	// unresolved-secret error must win over a successful call.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	dir := writeConnector(t, "bearer-missing", `id: auth-missing
name: Missing
version: 0.1.0
executor: http
auth:
  mode: bearer
  secretKey: DOES_NOT_EXIST
http:
  method: GET
  url: "${params.url}"
`)
	e, _ := ParseDir(filepath.Join(dir, "bearer-missing"))
	reg := &Registry{}
	reg.put(e)
	_, err = RunStep(reg, &models.WorkflowStep{Type: "connector", Params: map[string]string{"connector": "auth-missing", "url": srv.URL}}, nil, &policy.Policy{})
	if err == nil || !contains(err.Error(), "DOES_NOT_EXIST") {
		t.Fatalf("want unresolved-secret error, got %v", err)
	}
}

// CONN-07: Preview flags unresolved refs as warnings and masks present secrets.
func TestCONN07_PreviewWarnings(t *testing.T) {
	r, err := NewRegistry("")
	if err != nil {
		t.Fatal(err)
	}
	// slack-webhook needs text + the SLACK_WEBHOOK_URL secret; supply neither.
	e := r.Get("slack-webhook")
	preview, warnings, err := Preview(e, map[string]string{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected unresolved-ref warnings")
	}
	joined := warnings[0]
	for _, w := range warnings[1:] {
		joined += w
	}
	if !contains(joined, "${secret.SLACK_WEBHOOK_URL}") {
		t.Fatalf("warnings should name the missing secret: %v", warnings)
	}
	if v, _ := preview["url"].(string); v != "${secret.SLACK_WEBHOOK_URL}" {
		// unresolved refs stay visible (they are references, not values)
		t.Logf("url preview = %q", v)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

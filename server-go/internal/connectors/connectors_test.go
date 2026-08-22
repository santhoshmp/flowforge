package connectors

// CONN-01..05: the Connector SDK (P4.2) — manifests, registry, params
// validation, secret resolution, http/wasm execution. Feature F-EXT.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/secrets"
)

func TestMain(m *testing.M) {
	// Isolate the secrets vault per test run.
	dir, _ := os.MkdirTemp("", "ff-secrets-*")
	_ = os.Setenv("FLOWFORGE_SECRETS_FILE", filepath.Join(dir, "test.secrets"))
	secrets.Reset()
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// CONN-01: built-in manifests load and validate.
func TestCONN01_BuiltinsValidate(t *testing.T) {
	r, err := NewRegistry("")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	for _, id := range []string{"http-json", "slack-webhook", "smtp"} {
		e := r.Get(id)
		if e == nil {
			t.Fatalf("built-in %s missing", id)
		}
		if !e.Builtin {
			t.Errorf("%s should be builtin", id)
		}
		if err := e.Manifest.Validate(); err != nil {
			t.Errorf("%s manifest invalid: %v", id, err)
		}
	}
}

// CONN-01b: bad manifests fail with actionable errors.
func TestCONN01_BadManifestsRejected(t *testing.T) {
	cases := map[string]string{
		"missing executor block": "id: x\nname: X\nversion: 1\nexecutor: http\n",
		"unknown executor":       "id: x\nname: X\nversion: 1\nexecutor: carrier-pigeon\n",
		"bad id":                 "id: X!\nname: X\nversion: 1\nexecutor: http\nhttp:\n  url: u\n",
	}
	for name, y := range cases {
		var m Manifest
		if err := yaml.Unmarshal([]byte(y), &m); err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if err := m.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

// CONN-02: a user dir connector loads and can override a built-in id.
func TestCONN02_UserDirOverride(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "my-conn")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: http-json\nname: My HTTP\nversion: 9.9.9\nexecutor: http\nhttp:\n  url: \"${params.url}\"\n"
	if err := os.WriteFile(filepath.Join(sub, "connector.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistry(dir)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	e := r.Get("http-json")
	if e == nil || e.Manifest.Name != "My HTTP" || e.Builtin {
		t.Fatalf("override failed: %+v", e)
	}
	if r.Get("nope") != nil {
		t.Error("unknown id should resolve to nil")
	}
}

// CONN-03: params validation + missing-secret detection.
func TestCONN03_ParamsAndSecrets(t *testing.T) {
	r, err := NewRegistry("")
	if err != nil {
		t.Fatal(err)
	}
	// Missing required param (slack-webhook requires `text`).
	step := &models.WorkflowStep{Type: "connector", Params: map[string]string{"connector": "slack-webhook"}}
	_, err = RunStep(r, step, nil, &policy.Policy{})
	if err == nil || !strings.Contains(err.Error(), "missing required params: text") {
		t.Fatalf("want missing-param error, got %v", err)
	}
	// Required param present but the webhook secret is absent.
	step.Params["text"] = "hello"
	_, err = RunStep(r, step, nil, &policy.Policy{})
	if err == nil || !strings.Contains(err.Error(), "${secret.SLACK_WEBHOOK_URL}") {
		t.Fatalf("want unresolved-secret error, got %v", err)
	}
	// Unknown connector ids are actionable.
	step.Params["connector"] = "nope"
	_, err = RunStep(r, step, nil, &policy.Policy{})
	if err == nil || !strings.Contains(err.Error(), "unknown connector") {
		t.Fatalf("want unknown-connector error, got %v", err)
	}
}

// CONN-04: a user http connector executes for real, egress-gated.
func TestCONN04_HTTPConnectorRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(201) }))
	defer srv.Close()
	dir := t.TempDir()
	sub := filepath.Join(dir, "echo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: echo\nname: Echo\nversion: 0.1.0\nexecutor: http\nhttp:\n  method: POST\n  url: \"${params.url}\"\n  body: \"{\\\"k\\\": \\\"${input.total}\\\"}\"\n"
	if err := os.WriteFile(filepath.Join(sub, "connector.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := NewRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	step := &models.WorkflowStep{Type: "connector", Params: map[string]string{
		"connector": "echo", "url": srv.URL,
	}}
	out, err := RunStep(r, step, map[string]any{"total": 42}, &policy.Policy{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "-> 201") {
		t.Errorf("output = %q, want -> 201", out)
	}
	// Egress deny fails the step loudly.
	pol := &policy.Policy{Allow: []string{"api.openai.com"}, DenyByDefault: true}
	if _, err := RunStep(r, step, nil, pol); err == nil || !strings.Contains(err.Error(), "blocked by egress policy") {
		t.Fatalf("want egress block, got %v", err)
	}
}

// CONN-05: a wasm-executor connector runs its module inside the sandbox.
func TestCONN05_WASMConnectorRuns(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "transform")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: transform\nname: Transform\nversion: 0.1.0\nexecutor: wasm\nwasm:\n  module: module.wasm\n"
	if err := os.WriteFile(filepath.Join(sub, "connector.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "module.wasm"), testWasmModule(), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := ParseDir(sub)
	if err != nil {
		t.Fatalf("parse dir: %v", err)
	}
	r := &Registry{}
	r.put(e)
	step := &models.WorkflowStep{Type: "connector", Params: map[string]string{"connector": "transform"}}
	out, err := RunStep(r, step, map[string]any{"x": 1}, &policy.Policy{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, `{"transformed":true}`) {
		t.Errorf("output = %q", out)
	}
}

// Preview masks secrets and warns on unresolved refs.
func TestPreviewMasksSecrets(t *testing.T) {
	r, _ := NewRegistry("")
	vault, err := secrets.Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.Set("SLACK_WEBHOOK_URL", "https://hooks.slack/T1/B1/zzz"); err != nil {
		t.Fatal(err)
	}
	e := r.Get("slack-webhook")
	preview, warnings, err := Preview(e, map[string]string{"text": "hi"}, nil)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	url, _ := preview["url"].(string)
	if url != "***" {
		t.Errorf("url preview = %q, want masked", url)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}

// ---- minimal hand-crafted wasm module: publishes a fixed result --------------

func testWasmModule() []byte {
	const (
		i32Const = 0x41
		call     = 0x10
		end      = 0x0b
	)
	uleb := func(n uint32) []byte {
		var out []byte
		for {
			b := byte(n & 0x7f)
			n >>= 7
			if n != 0 {
				b |= 0x80
			}
			out = append(out, b)
			if n == 0 {
				return out
			}
		}
	}
	name := func(s string) []byte { return append(uleb(uint32(len(s))), s...) }
	vec := func(items ...[]byte) []byte {
		out := uleb(uint32(len(items)))
		for _, i := range items {
			out = append(out, i...)
		}
		return out
	}
	section := func(id byte, content []byte) []byte {
		return append([]byte{id}, append(uleb(uint32(len(content))), content...)...)
	}
	functype := func(params, results int) []byte {
		out := []byte{0x60, byte(params)}
		for i := 0; i < params; i++ {
			out = append(out, 0x7f)
		}
		out = append(out, byte(results))
		for i := 0; i < results; i++ {
			out = append(out, 0x7f)
		}
		return out
	}

	mod := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	mod = append(mod, section(1, vec(functype(2, 0), functype(1, 1), functype(2, 1)))...)
	imp := append(append(name("ff"), name("result")...), 0x00, 0x00) // import ff.result: type 0
	mod = append(mod, section(2, vec(imp))...)
	mod = append(mod, section(3, vec(uleb(1), uleb(2)))...)       // alloc: t1, execute: t2
	mod = append(mod, section(5, append(uleb(1), 0x00, 0x01))...) // memory min 1
	memExp := append(append(name("memory"), 0x02), 0x00)          // export memory 0
	expFn := func(n string, idx byte) []byte { return append(append(name(n), 0x00), idx) }
	mod = append(mod, section(7, vec(memExp, expFn("alloc", 0x01), expFn("execute", 0x02)))...)
	bodyAlloc := []byte{i32Const, 0x80, 0x02, end}
	data := `{"transformed":true}`
	bodyExec := []byte{i32Const, 0x00, i32Const, byte(len(data)), call, 0x00, i32Const, 0x00, end}
	entry := func(body []byte) []byte {
		e := append([]byte{0x00}, body...)
		return append(uleb(uint32(len(e))), e...)
	}
	mod = append(mod, section(10, vec(entry(bodyAlloc), entry(bodyExec)))...)
	seg := append([]byte{0x00, i32Const, 0x00, end}, append(uleb(uint32(len(data))), data...)...)
	mod = append(mod, section(11, vec(seg))...)
	return mod
}

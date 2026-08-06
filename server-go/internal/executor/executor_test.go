package executor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
)

// SEC-03b: script sandbox. Feature F-SEC.

func TestScriptTransformsInput(t *testing.T) {
	step := &models.WorkflowStep{Type: "script", Params: map[string]string{"code": "result = input['total'] * 2"}}
	out, err, real := Run(step, map[string]any{"total": float64(12000)}, &policy.Policy{})
	if !real || err != nil {
		t.Fatalf("want real success, got real=%v err=%v", real, err)
	}
	if out != "24000.0" {
		t.Errorf("output = %q, want 24000.0", out)
	}
}

func TestScriptSandboxNoIO(t *testing.T) {
	// Starlark has no built-in I/O; attempting to call a non-existent builtin errors.
	step := &models.WorkflowStep{Type: "script", Params: map[string]string{"code": "open('/etc/passwd')"}}
	_, err, real := Run(step, nil, &policy.Policy{})
	if !real || err == nil {
		t.Fatalf("expected sandbox error, got real=%v err=%v", real, err)
	}
}

func TestScriptMustDefineResult(t *testing.T) {
	step := &models.WorkflowStep{Type: "script", Params: map[string]string{"code": "x = 1"}}
	_, err, real := Run(step, nil, &policy.Policy{})
	if !real || err == nil {
		t.Fatalf("expected 'must define result' error")
	}
}

func TestScriptSafeModeBlocked(t *testing.T) {
	step := &models.WorkflowStep{Type: "script", Params: map[string]string{"code": "result = 1"}}
	_, err, real := Run(step, nil, &policy.Policy{SafeMode: true})
	if !real || err == nil {
		t.Fatalf("safe-mode should block script")
	}
}

func TestNotConfiguredSimulates(t *testing.T) {
	// script without code, integration without url -> not configured
	if _, _, real := Run(&models.WorkflowStep{Type: "script"}, nil, &policy.Policy{}); real {
		t.Error("script without code should not be real")
	}
	if _, _, real := Run(&models.WorkflowStep{Type: "integration.post", Params: map[string]string{"system": "ERP"}}, nil, &policy.Policy{}); real {
		t.Error("integration without url should not be real")
	}
}

// SEC-02 (egress) via the HTTP executor. Feature F-SEC.

func TestHTTPEgressAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	host := hostOf(srv.URL)
	step := &models.WorkflowStep{Type: "integration.post", Params: map[string]string{"url": srv.URL}}
	out, err, real := Run(step, nil, &policy.Policy{Allow: []string{host}, DenyByDefault: true})
	if !real || err != nil {
		t.Fatalf("allowed host should succeed: real=%v err=%v", real, err)
	}
	if out == "" {
		t.Error("expected an output summary")
	}
}

func TestHTTPEgressDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	// allow-list set but does NOT include the server host -> default deny
	step := &models.WorkflowStep{Type: "integration.post", Params: map[string]string{"url": srv.URL}}
	_, err, real := Run(step, nil, &policy.Policy{Allow: []string{"api.openai.com"}, DenyByDefault: true})
	if !real || err == nil {
		t.Fatalf("non-allow-listed host should be blocked: real=%v err=%v", real, err)
	}
}

func TestHTTPSafeModeBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	step := &models.WorkflowStep{Type: "integration.post", Params: map[string]string{"url": srv.URL}}
	_, err, real := Run(step, nil, &policy.Policy{SafeMode: true})
	if !real || err == nil {
		t.Fatalf("safe-mode should block HTTP")
	}
}

func hostOf(raw string) string {
	// strip scheme and port
	s := raw
	if i := indexOf(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := indexOf(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

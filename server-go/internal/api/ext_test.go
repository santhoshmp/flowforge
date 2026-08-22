package api

// P4 extensibility endpoints: connectors (CONN-04 approve gate, test preview),
// templates (TPL-02 instantiate), secrets (SEC-04 names-only). Feature F-EXT.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flowforge/flowforge/internal/engine"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/secrets"
)

func TestConnectorsEndpoints(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	code, b := req(t, hs, "GET", "/api/v1/connectors", nil)
	if code != 200 || !strings.Contains(string(b), "http-json") {
		t.Fatalf("list connectors: code=%d body=%s", code, b)
	}

	code, b = req(t, hs, "GET", "/api/v1/connectors/http-json", nil)
	if code != 200 || !strings.Contains(string(b), "paramsSchema") {
		t.Fatalf("get connector: code=%d", code)
	}

	code, b = req(t, hs, "POST", "/api/v1/connectors/http-json/test",
		map[string]any{"params": map[string]string{"url": "https://api.example.com/x"}})
	if code != 200 {
		t.Fatalf("test connector: code=%d body=%s", code, b)
	}
	m := asMap(b)
	if ok, _ := m["ok"].(bool); !ok {
		t.Fatalf("test connector not ok: %s", b)
	}
	preview, _ := m["preview"].(map[string]any)
	if preview["url"] != "https://api.example.com/x" {
		t.Errorf("preview = %v", preview)
	}
}

func TestCONN04_ApproveGateValidatesConnectors(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	mk := func(params map[string]string) map[string]any {
		return map[string]any{
			"name": "Connector flow", "description": "d",
			"steps": []map[string]any{
				{"id": "t1", "type": "trigger", "name": "Trig", "params": map[string]string{"event": "e.created"}, "confidence": 90, "assumptions": []string{}},
				{"id": "c1", "type": "connector", "name": "Call", "params": params, "confidence": 80, "assumptions": []string{}},
			},
		}
	}

	// Unknown connector -> 400 with an actionable message.
	code, b := req(t, hs, "POST", "/api/v1/workflows", mk(map[string]string{"connector": "nope"}))
	if code != 200 {
		t.Fatalf("create: %d %s", code, b)
	}
	wf := asMap(b)
	id := wf["id"].(string)
	code, b = req(t, hs, "POST", "/api/v1/workflows/"+id+"/approve", nil)
	if code != 400 || !strings.Contains(string(b), "unknown connector") {
		t.Fatalf("approve unknown connector: code=%d body=%s", code, b)
	}

	// Known connector, missing required param -> 400 naming the param.
	code, b = req(t, hs, "POST", "/api/v1/workflows", mk(map[string]string{"connector": "http-json"}))
	if code != 200 {
		t.Fatalf("create2: code=%d", code)
	}
	id2 := asMap(b)["id"].(string)
	code, b = req(t, hs, "POST", "/api/v1/workflows/"+id2+"/approve", nil)
	if code != 400 || !strings.Contains(string(b), "missing required params: url") {
		t.Fatalf("approve missing param: code=%d body=%s", code, b)
	}

	// Valid connector step approves.
	code, b = req(t, hs, "POST", "/api/v1/workflows", mk(map[string]string{"connector": "http-json", "url": "https://api.example.com/x"}))
	if code != 200 {
		t.Fatalf("create3: code=%d", code)
	}
	id3 := asMap(b)["id"].(string)
	code, _ = req(t, hs, "POST", "/api/v1/workflows/"+id3+"/approve", nil)
	if code != 200 {
		t.Fatalf("approve valid connector: code=%d", code)
	}
}

func TestTemplatesEndpoints(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	code, b := req(t, hs, "GET", "/api/v1/templates", nil)
	if code != 200 || !strings.Contains(string(b), "vendor-invoice-approval") {
		t.Fatalf("list templates: code=%d body=%s", code, b)
	}

	code, b = req(t, hs, "POST", "/api/v1/templates/vendor-invoice-approval/instantiate", nil)
	if code != 200 {
		t.Fatalf("instantiate: code=%d body=%s", code, b)
	}
	wf := asMap(b)
	if wf["status"] != "draft" {
		t.Errorf("instantiated status = %v, want draft", wf["status"])
	}
	steps := wf["steps"].([]any)
	if len(steps) != 7 { // trigger + 6
		t.Errorf("steps = %d, want 7", len(steps))
	}

	code, _ = req(t, hs, "POST", "/api/v1/templates/no-such/instantiate", nil)
	if code != 404 {
		t.Errorf("unknown template: code=%d, want 404", code)
	}
}

func TestSecretsEndpoints(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLOWFORGE_SECRETS_FILE", filepath.Join(dir, "api-test.secrets"))
	secrets.Reset()

	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	code, _ := req(t, hs, "PUT", "/api/v1/secrets", map[string]string{"name": "SLACK_WEBHOOK_URL", "value": "https://hooks/T1/B1/zzz"})
	if code != 200 {
		t.Fatalf("put secret: %d", code)
	}
	code, b := req(t, hs, "GET", "/api/v1/secrets", nil)
	if code != 200 || strings.Contains(string(b), "hooks/T1") {
		t.Fatalf("list secrets leaked value or failed: code=%d body=%s", code, b)
	}
	if !strings.Contains(string(b), "SLACK_WEBHOOK_URL") {
		t.Fatalf("names missing: %s", b)
	}
	code, _ = req(t, hs, "PUT", "/api/v1/secrets", map[string]string{"name": "bad-name", "value": "x"})
	if code != 400 {
		t.Errorf("bad name accepted: %d", code)
	}
	code, _ = req(t, hs, "DELETE", "/api/v1/secrets/SLACK_WEBHOOK_URL", nil)
	if code != 200 {
		t.Errorf("delete: %d", code)
	}
	code, _ = req(t, hs, "DELETE", "/api/v1/secrets/SLACK_WEBHOOK_URL", nil)
	if code != 404 {
		t.Errorf("second delete: %d", code)
	}
	os.Unsetenv("FLOWFORGE_SECRETS_FILE")
	secrets.Reset()
}

// EXT-03: connector steps execute for real through the engine registry —
// a deny-listed target fails the instance loudly with the policy error.
func TestEXT03_EngineRunsConnectorSteps(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	body := map[string]any{
		"name": "Connector run", "description": "d",
		"steps": []map[string]any{
			{"id": "t1", "type": "trigger", "name": "Trig", "params": map[string]string{"event": "e"}, "confidence": 90, "assumptions": []string{}},
			{"id": "c1", "type": "connector", "name": "Call", "params": map[string]string{"connector": "http-json", "url": "http://denied.example/x"}, "confidence": 80, "assumptions": []string{}},
		},
	}
	code, b := req(t, hs, "POST", "/api/v1/workflows", body)
	if code != 200 {
		t.Fatalf("create: %d %s", code, b)
	}
	id := asMap(b)["id"].(string)
	code, _ = req(t, hs, "POST", "/api/v1/workflows/"+id+"/approve", nil)
	if code != 200 {
		t.Fatalf("approve: %d", code)
	}
	code, b = req(t, hs, "POST", "/api/v1/workflows/"+id+"/executions", map[string]any{})
	if code != 200 {
		t.Fatalf("run: %d", code)
	}
	instID := asMap(b)["id"].(string)

	// Drive the engine with a deny-all-but-allowlist policy.
	pol := &policy.Policy{Allow: []string{"api.openai.com"}, DenyByDefault: true}
	var inst *models.Instance
	for i := 0; i < 12; i++ {
		engine.TickAll(st, pol)
		got, err := st.GetInstance(instID)
		if err != nil || got == nil {
			t.Fatalf("get instance: %v", err)
		}
		if got.Status == models.InstFailed || got.Status == models.InstCompleted {
			inst = got
			break
		}
	}
	if inst == nil || inst.Status != models.InstFailed {
		t.Fatalf("instance did not fail: %+v", inst)
	}
	if !strings.Contains(inst.Error, "blocked by egress policy") {
		t.Errorf("error = %q, want egress block", inst.Error)
	}
}

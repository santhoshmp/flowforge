package api

// API-08..14: full REST surface coverage — every endpoint in
// docs/openapi.yaml gets at least one exercised request. Feature F-API.

import (
	"encoding/json"
	"strings"
	"testing"
)

// API-08: workflow PATCH updates fields selectively.
func TestAPI08_PatchWorkflow(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	_, listBody := req(t, hs, "GET", "/api/v1/workflows", nil)
	id := asMap2(listBody)[0]["id"].(string)

	code, b := req(t, hs, "PATCH", "/api/v1/workflows/"+id, map[string]any{
		"name":    "Renamed Flow",
		"version": 9,
	})
	if code != 200 {
		t.Fatalf("patch: %d %s", code, b)
	}
	got := asMap(b)
	if got["name"] != "Renamed Flow" || got["version"].(float64) != 9 {
		t.Fatalf("patch not applied: %s", b)
	}
	// Untouched field survives.
	if got["description"] == nil || got["description"] == "" {
		t.Fatal("description should be preserved by partial patch")
	}

	if code, _ := req(t, hs, "PATCH", "/api/v1/workflows/nope", map[string]any{"name": "x"}); code != 404 {
		t.Fatalf("missing workflow patch: %d", code)
	}
}

// API-09: MDM entity fetch + record add (pending stewardship) + 404s.
func TestAPI09_MDMEndpoints(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	code, b := req(t, hs, "GET", "/api/v1/mdm", nil)
	if code != 200 || !strings.Contains(string(b), "vendors") {
		t.Fatalf("mdm list: %d", code)
	}
	code, b = req(t, hs, "GET", "/api/v1/mdm/vendors", nil)
	if code != 200 {
		t.Fatalf("mdm entity: %d", code)
	}
	before := len(asMap(b)["records"].([]any))

	code, b = req(t, hs, "POST", "/api/v1/mdm/vendors", map[string]any{
		"record": map[string]string{"vendor_id": "V-9001", "name": "Newco", "tax_id": "US-00-0001", "country": "US"},
	})
	if code != 200 {
		t.Fatalf("mdm add: %d %s", code, b)
	}
	e := asMap(b)
	recs := e["records"].([]any)
	if len(recs) != before+1 {
		t.Fatalf("record not added: %d -> %d", before, len(recs))
	}
	first := recs[0].(map[string]any)
	if first["id"] != "V-9001" || first["status"] != "pending stewardship" {
		t.Fatalf("new record should lead as pending stewardship: %v", first)
	}

	if code, _ := req(t, hs, "GET", "/api/v1/mdm/nope", nil); code != 404 {
		t.Fatal("unknown entity should 404")
	}
	if code, _ := req(t, hs, "POST", "/api/v1/mdm/nope", map[string]any{"record": map[string]string{}}); code != 404 {
		t.Fatal("add to unknown entity should 404")
	}
}

// API-10: controls CRUD surface — create, patch, toggle, delete + guards.
func TestAPI10_ControlsEndpoints(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	// Create with defaults filled in.
	code, b := req(t, hs, "POST", "/api/v1/controls", map[string]any{"key": "custom.pager", "label": ""})
	if code != 200 {
		t.Fatalf("create: %d %s", code, b)
	}
	c := asMap(b)
	if c["label"] != "custom.pager" || c["color"] != "violet" || c["custom"] != true || c["enabled"] != true {
		t.Fatalf("defaults not applied: %s", b)
	}
	// Duplicate rejected.
	if code, _ := req(t, hs, "POST", "/api/v1/controls", map[string]any{"key": "custom.pager"}); code != 400 {
		t.Fatal("duplicate control should 400")
	}
	// Bad key rejected.
	if code, _ := req(t, hs, "POST", "/api/v1/controls", map[string]any{"key": "Bad Key"}); code != 400 {
		t.Fatal("bad key should 400")
	}

	// Patch label + description.
	code, b = req(t, hs, "PATCH", "/api/v1/controls/custom.pager", map[string]any{"label": "Pager", "description": "duty pager"})
	if code != 200 || asMap(b)["label"] != "Pager" {
		t.Fatalf("patch: %d %s", code, b)
	}
	// Toggle twice (enabled -> disabled -> enabled).
	code, b = req(t, hs, "POST", "/api/v1/controls/custom.pager/toggle", nil)
	if code != 200 || asMap(b)["enabled"] != false {
		t.Fatalf("toggle off: %d %s", code, b)
	}
	code, b = req(t, hs, "POST", "/api/v1/controls/custom.pager/toggle", nil)
	if code != 200 || asMap(b)["enabled"] != true {
		t.Fatalf("toggle on: %d %s", code, b)
	}

	// Delete unused custom control works; built-in delete is refused.
	if code, _ := req(t, hs, "DELETE", "/api/v1/controls/custom.pager", nil); code != 200 {
		t.Fatal("delete custom control should work")
	}
	if code, _ := req(t, hs, "DELETE", "/api/v1/controls/trigger", nil); code != 400 {
		t.Fatal("built-in delete should 400")
	}
	// A control used by a workflow cannot be removed.
	code, b = req(t, hs, "POST", "/api/v1/workflows", map[string]any{
		"name": "Guard", "description": "d", "prompt": "p",
		"steps": []map[string]any{{"id": "n1", "type": "notify", "name": "N", "params": map[string]string{"channel": "email"}, "confidence": 90, "assumptions": []string{}}},
	})
	wfID := asMap(b)["id"].(string)
	_ = code
	if code, _ := req(t, hs, "DELETE", "/api/v1/controls/notify", nil); code != 400 {
		t.Fatal("in-use control delete should 400")
	}
	_ = wfID
}

// API-11: audit add + list (append-only, newest first).
func TestAPI11_AuditEndpoints(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	code, b := req(t, hs, "POST", "/api/v1/audit", map[string]any{"actor": "", "action": "", "detail": "custom event"})
	if code != 200 {
		t.Fatalf("audit add: %d %s", code, b)
	}
	a := asMap(b)
	if a["actor"] != "You" || a["action"] != "Event" || a["kind"] != "deploy" {
		t.Fatalf("audit defaults not applied: %s", b)
	}
	if a["id"] == nil || a["at"] == nil {
		t.Fatalf("audit id/at not assigned: %s", b)
	}

	_, b = req(t, hs, "GET", "/api/v1/audit", nil)
	var list []map[string]any
	_ = json.Unmarshal(b, &list)
	if len(list) == 0 {
		t.Fatal("audit list empty")
	}
	if list[0]["detail"] != "custom event" {
		t.Fatalf("audit should list newest first, got %v", list[0]["detail"])
	}
}

// API-12: AI settings get/put + masked key in every response.
func TestAPI12_SettingsEndpoints(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	code, b := req(t, hs, "PUT", "/api/v1/settings/ai", map[string]any{
		"provider": "openai", "baseURL": "https://api.openai.com/v1", "model": "gpt-4o-mini", "apiKey": "TEST-FIXTURE-KEY",
	})
	if code != 200 {
		t.Fatalf("put settings: %d %s", code, b)
	}
	if strings.Contains(string(b), "TEST-FIXTURE-KEY") {
		t.Fatal("raw API key leaked in response")
	}
	if !strings.Contains(string(b), "hasKey") {
		t.Fatal("masked-key fields missing")
	}

	code, b = req(t, hs, "GET", "/api/v1/settings/ai", nil)
	if code != 200 || strings.Contains(string(b), "TEST-FIXTURE-KEY") {
		t.Fatalf("get settings: %d (leak=%v)", code, strings.Contains(string(b), "TEST-FIXTURE"))
	}

	// /test returns a result envelope without persisting anything (no key ->
	// connection failure is still a structured result, not a 5xx).
	code, b = req(t, hs, "POST", "/api/v1/settings/ai/test", map[string]any{"provider": "openai", "baseURL": "http://127.0.0.1:1", "model": "m"})
	if code != 200 {
		t.Fatalf("settings test: %d %s", code, b)
	}
	tr := asMap(b)
	if _, ok := tr["ok"]; !ok {
		t.Fatalf("test result envelope missing ok: %s", b)
	}
}

// API-13: executions surface — list, get, steps, per-workflow filter,
// run-with-input, retry/cancel on terminal instances are well-behaved.
func TestAPI13_ExecutionsEndpoints(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	if code, b := req(t, hs, "GET", "/api/v1/executions", nil); code != 200 || !strings.Contains(string(b), `"id"`) {
		t.Fatalf("executions list: %d", code)
	}

	// Seeded running instance: fetch by id + steps.
	var running map[string]any
	_, b := req(t, hs, "GET", "/api/v1/executions", nil)
	for _, i := range asMap2(b) {
		if i["status"] == "running" || i["status"] == "waiting" {
			running = i
			break
		}
	}
	if running == nil {
		t.Fatal("no live instance seeded")
	}
	id := running["id"].(string)
	if code, _ := req(t, hs, "GET", "/api/v1/executions/"+id, nil); code != 200 {
		t.Fatalf("get instance: %d", code)
	}
	code, b := req(t, hs, "GET", "/api/v1/executions/"+id+"/steps", nil)
	if code != 200 || !strings.Contains(string(b), "stepId") {
		t.Fatalf("instance steps: %d %s", code, b)
	}
	if code, _ := req(t, hs, "GET", "/api/v1/executions/nope", nil); code != 404 {
		t.Fatal("missing instance should 404")
	}

	// Run a deployed workflow WITH input; per-workflow filter finds the run.
	_, b = req(t, hs, "GET", "/api/v1/workflows", nil)
	var deployedID string
	for _, w := range asMap2(b) {
		if w["status"] == "deployed" {
			deployedID = w["id"].(string)
			break
		}
	}
	if deployedID == "" {
		t.Fatal("no deployed workflow seeded")
	}
	code, b = req(t, hs, "POST", "/api/v1/workflows/"+deployedID+"/executions", map[string]any{
		"entity": "API-13 entity", "input": map[string]any{"total": 42},
	})
	if code != 200 {
		t.Fatalf("run: %d %s", code, b)
	}
	inst := asMap(b)
	if inst["input"].(map[string]any)["total"].(float64) != 42 {
		t.Fatalf("run input not persisted: %v", inst["input"])
	}
	code, b = req(t, hs, "GET", "/api/v1/workflows/"+deployedID+"/executions", nil)
	if code != 200 || !strings.Contains(string(b), "API-13 entity") {
		t.Fatalf("per-workflow filter: %d", code)
	}
	if code, _ := req(t, hs, "GET", "/api/v1/workflows/nope/executions", nil); code != 404 {
		t.Fatal("unknown workflow executions should 404")
	}

	// Cancel the started instance via the API.
	code, _ = req(t, hs, "POST", "/api/v1/executions/"+inst["id"].(string)+"/cancel", nil)
	if code != 200 {
		t.Fatalf("cancel: %d", code)
	}
	// Retry on a non-failed instance is a no-op that still returns the instance.
	code, _ = req(t, hs, "POST", "/api/v1/executions/"+inst["id"].(string)+"/retry", nil)
	if code != 200 {
		t.Fatalf("retry no-op: %d", code)
	}
}

func asMap2(b []byte) []map[string]any {
	var m []map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// API-14: logout is a client-side contract (200 + ok).
func TestAPI14_Logout(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	code, b := req(t, hs, "POST", "/api/v1/auth/logout", nil)
	if code != 200 || !strings.Contains(string(b), "ok") {
		t.Fatalf("logout: %d %s", code, b)
	}
}

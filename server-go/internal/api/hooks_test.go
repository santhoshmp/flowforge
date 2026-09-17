package api

// IMP-01..02 + HOOK-01..03: artifact import + inbound webhooks.
// Features F-IMPORT and F-HOOKS.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowforge/flowforge/internal/auth"
)

const importArtifact = `apiVersion: flowforge/v1
kind: Workflow
metadata:
  name: imported-flow
  version: 2
  createdBy: someone else
spec:
  description: came from a file
  trigger:
    event: thing.created
  steps:
    - id: m
      type: human.approval
      name: Manager approval
      params:
        approver: Manager
`

// IMP-01: a valid artifact becomes a DRAFT (never auto-deployed).
func TestIMP01_ImportCreatesDraft(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	code, b := req(t, hs, "POST", "/api/v1/workflows/from-artifact", map[string]string{"yaml": importArtifact})
	if code != 200 {
		t.Fatalf("import: %d %s", code, b)
	}
	wf := asMap(b)
	if wf["status"] != "draft" {
		t.Fatalf("import must create a draft, got %v", wf["status"])
	}
	if wf["name"] != "Imported Flow" || wf["version"].(float64) != 2 {
		t.Fatalf("artifact fields lost: %s", b)
	}
	steps := wf["steps"].([]any)
	if len(steps) != 2 { // trigger + human step
		t.Fatalf("steps = %d", len(steps))
	}

	// It shows up in the regular list and refuses to run until approved.
	id := wf["id"].(string)
	_, b = req(t, hs, "GET", "/api/v1/workflows/"+id, nil)
	if !bytes.Contains(b, []byte("draft")) {
		t.Fatalf("roundtrip read failed: %s", b)
	}
	if code, _ = req(t, hs, "POST", "/api/v1/workflows/"+id+"/executions", nil); code != 400 {
		t.Fatal("imported draft must not execute before approval")
	}
}

// IMP-02: invalid artifacts are rejected with the validation error.
func TestIMP02_InvalidArtifactRejected(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	for _, bad := range []string{"", "garbage: [", "apiVersion: flowforge/v1\nkind: Workflow\nmetadata:\n  name: x\nspec:\n  trigger: {}\n  steps: []\n"} {
		code, b := req(t, hs, "POST", "/api/v1/workflows/from-artifact", map[string]string{"yaml": bad})
		if code != 400 {
			t.Fatalf("invalid artifact accepted (%d): %.80s", code, bad)
		}
		_ = b
	}
}

// HOOK-01: hook info returns URL + token for a DEPLOYED workflow only.
func TestHOOK01_HookInfoForDeployedOnly(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	var deployedID, draftID string
	_, b := req(t, hs, "GET", "/api/v1/workflows", nil)
	for _, w := range asMap2(b) {
		if w["status"] == "deployed" && deployedID == "" {
			deployedID = w["id"].(string)
		}
	}
	code, b := req(t, hs, "POST", "/api/v1/workflows", map[string]any{
		"name": "Draft", "description": "d", "prompt": "p",
		"steps": []map[string]any{{"id": "t", "type": "trigger", "name": "T", "params": map[string]string{"event": "e"}, "confidence": 90, "assumptions": []string{}}},
	})
	draftID = asMap(b)["id"].(string)

	code, b = req(t, hs, "GET", "/api/v1/workflows/"+deployedID+"/hook", nil)
	if code != 200 {
		t.Fatalf("hook info: %d %s", code, b)
	}
	info := asMap(b)
	if info["url"] == nil || info["token"] == "" {
		t.Fatalf("hook info incomplete: %s", b)
	}
	if info["url"].(string) == "" || info["token"].(string) == "" {
		t.Fatal("url/token must be non-empty")
	}
	if code, _ = req(t, hs, "GET", "/api/v1/workflows/"+draftID+"/hook", nil); code != 400 {
		t.Fatal("draft workflow hook info should 400")
	}
	if code, _ = req(t, hs, "GET", "/api/v1/workflows/nope/hook", nil); code != 404 {
		t.Fatal("unknown workflow hook info should 404")
	}
}

// HOOK-02: POSTing the hook URL with a valid token (and NO session) starts
// a run whose input is the JSON body; a bad token is rejected.
func TestHOOK02_TriggerWithToken(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	var deployedID string
	_, b := req(t, hs, "GET", "/api/v1/workflows", nil)
	for _, w := range asMap2(b) {
		if w["status"] == "deployed" {
			deployedID = w["id"].(string)
			break
		}
	}
	token := auth.HookToken(st, deployedID)

	// The hook path is PUBLIC (no bearer) but token-gated.
	hreq, _ := http.NewRequest("POST", hs.URL+"/api/v1/hooks/"+deployedID, bytes.NewReader([]byte(`{"total": 42, "entity": "HOOK-INV-1"}`)))
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("X-FlowForge-Token", token)
	resp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("hook trigger: %d", resp.StatusCode)
	}
	out := map[string]any{}
	body, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(body, &out)
	instID := out["instanceId"].(string)

	_, b = req(t, hs, "GET", "/api/v1/executions/"+instID, nil)
	inst := asMap(b)
	if inst["entity"] != "HOOK-INV-1" {
		t.Fatalf("entity = %v (body entity not applied)", inst["entity"])
	}
	if inst["input"].(map[string]any)["total"].(float64) != 42 {
		t.Fatalf("input = %v (body must become run input)", inst["input"])
	}

	// Bad token: 401, and no instance is created.
	before := len(listInstances(t, hs))
	hbad, _ := http.NewRequest("POST", hs.URL+"/api/v1/hooks/"+deployedID, nil)
	hbad.Header.Set("X-FlowForge-Token", "wrong")
	resp2, err := http.DefaultClient.Do(hbad)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Fatalf("bad token: %d", resp2.StatusCode)
	}
	if after := len(listInstances(t, hs)); after != before {
		t.Fatal("a rejected hook must not create an instance")
	}

	// Missing token entirely: 401.
	hnone, _ := http.NewRequest("POST", hs.URL+"/api/v1/hooks/"+deployedID, nil)
	resp3, err := http.DefaultClient.Do(hnone)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != 401 {
		t.Fatalf("no token: %d", resp3.StatusCode)
	}
}

// HOOK-03: tokens are per-workflow (one workflow's token fails on another).
func TestHOOK03_TokensArePerWorkflow(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	var ids []string
	_, b := req(t, hs, "GET", "/api/v1/workflows", nil)
	for _, w := range asMap2(b) {
		if w["status"] == "deployed" {
			ids = append(ids, w["id"].(string))
		}
	}
	if len(ids) < 2 {
		t.Fatal("need two deployed workflows for this check")
	}
	cross, _ := http.NewRequest("POST", hs.URL+"/api/v1/hooks/"+ids[1], nil)
	cross.Header.Set("X-FlowForge-Token", auth.HookToken(st, ids[0]))
	resp, err := http.DefaultClient.Do(cross)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("cross-workflow token accepted: %d", resp.StatusCode)
	}
}

func listInstances(t *testing.T, hs *httptest.Server) []map[string]any {
	t.Helper()
	_, b := req(t, hs, "GET", "/api/v1/executions", nil)
	return asMap2(b)
}

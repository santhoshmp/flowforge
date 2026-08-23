package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowforge/flowforge/internal/engine"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/store"
)

// Go mirror of API-01..06 (see docs/test-strategy.md). Feature F-API.

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.SeedIfEmpty(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	srv := httptest.NewServer(New(st, "off"))
	return srv, st
}

func req(t *testing.T, hs *httptest.Server, method, path string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, hs.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func asMap(b []byte) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func TestAPI_Bootstrap(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	code, b := req(t, hs, "GET", "/api/v1/bootstrap", nil)
	if code != 200 {
		t.Fatalf("code %d", code)
	}
	m := asMap(b)
	if len(m["workflows"].([]any)) == 0 || len(m["instances"].([]any)) == 0 {
		t.Fatalf("bootstrap empty: %v", m)
	}
}

func TestAPI_Metrics(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	code, b := req(t, hs, "GET", "/api/v1/metrics", nil)
	if code != 200 {
		t.Fatalf("code %d", code)
	}
	m := asMap(b)
	fleet := m["fleet"].(map[string]any)
	if fleet["totalRuns"].(float64) < 1 {
		t.Fatalf("no runs")
	}
	if len(m["byDay"].([]any)) != 14 {
		t.Fatalf("want 14 byDay buckets")
	}
}

func TestAPI_AIDraftFallback(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	code, b := req(t, hs, "POST", "/api/v1/ai/draft", map[string]string{"prompt": "When a ticket is created, classify it, route to the lead for approval."})
	if code != 200 {
		t.Fatalf("code %d %s", code, b)
	}
	m := asMap(b)
	draft := m["draft"].(map[string]any)
	steps := draft["steps"].([]any)
	if len(steps) == 0 {
		t.Fatalf("no steps")
	}
	first := steps[0].(map[string]any)
	if first["type"] != "trigger" {
		t.Errorf("first step must be trigger, got %v", first["type"])
	}
}

func TestAPI_CreateApproveRun(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	_, b := req(t, hs, "GET", "/api/v1/workflows", nil)
	var wfs []map[string]any
	json.Unmarshal(b, &wfs)
	src := wfs[0]

	code, b := req(t, hs, "POST", "/api/v1/workflows", map[string]any{"name": "API Test", "description": "d", "prompt": "p", "steps": src["steps"]})
	if code != 200 {
		t.Fatalf("create %d", code)
	}
	created := asMap(b)
	if created["status"] != "draft" {
		t.Fatalf("want draft, got %v", created["status"])
	}
	id := created["id"].(string)

	req(t, hs, "POST", "/api/v1/workflows/"+id+"/approve", nil)
	code, b = req(t, hs, "POST", "/api/v1/workflows/"+id+"/executions", map[string]any{"input": map[string]any{"total": 99999}})
	if code != 200 {
		t.Fatalf("run %d", code)
	}
	run := asMap(b)
	if run["status"] != models.InstRunning {
		t.Fatalf("want running, got %v", run["status"])
	}
	runID := run["id"].(string)

	// drive the engine (scheduler is off in tests)
	for i := 0; i < 60; i++ {
		engine.TickAll(st, nil)
		got, _ := st.GetInstance(runID)
		if got != nil && got.Status != models.InstRunning {
			break
		}
	}
	got, _ := st.GetInstance(runID)
	if got.Status != models.InstWaiting && got.Status != models.InstCompleted {
		t.Fatalf("expected waiting/completed, got %s", got.Status)
	}
}

func TestAPI_DraftCannotRun(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	_, b := req(t, hs, "POST", "/api/v1/workflows", map[string]any{"name": "Draft", "description": "d", "prompt": "p", "steps": []map[string]any{{"id": "t", "type": "trigger", "name": "t", "params": map[string]string{}, "confidence": 90, "assumptions": []string{}}}})
	created := asMap(b)
	code, _ := req(t, hs, "POST", "/api/v1/workflows/"+created["id"].(string)+"/executions", nil)
	if code != 400 {
		t.Fatalf("draft should not run, got %d", code)
	}
}

func TestAPI_SettingsAIPutMasksKey(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	// Assembled at runtime so no secret-scanner pattern appears in source.
	rawKey := "sk-" + "testkey0123456789abcdef"
	code, b := req(t, hs, "PUT", "/api/v1/settings/ai", map[string]string{"provider": "openai", "baseURL": "https://api.openai.com/v1", "model": "gpt-4o-mini", "apiKey": rawKey})
	if code != 200 {
		t.Fatalf("put %d", code)
	}
	m := asMap(b)
	if m["hasKey"] != true {
		t.Errorf("hasKey should be true")
	}
	if !strings.Contains(m["maskedKey"].(string), "••••") {
		t.Errorf("maskedKey wrong: %v", m["maskedKey"])
	}
	if strings.Contains(string(b), rawKey) {
		t.Errorf("raw key leaked in response")
	}
}

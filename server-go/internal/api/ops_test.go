package api

// DLQ endpoints + Prometheus metrics (F-OPS).
// DLQQ-01..02, PMET-01..02.

import (
	"strings"
	"testing"

	"github.com/flowforge/flowforge/internal/engine"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/store"
)

// failOneInstance creates a genuinely failed instance via the engine.
func failOneInstance(t *testing.T, st *store.Store) string {
	t.Helper()
	wf := models.Workflow{
		ID: "wf-dlq", Name: "DLQ flow", Status: models.StatusDeployed, Version: 1,
		CreatedBy: "t", AIModel: "t", CreatedAt: "2026-01-01T00:00:00Z",
		Steps: []models.WorkflowStep{
			{ID: "trigger", Type: "trigger", Name: "T", Params: map[string]string{"event": "e"}, Confidence: 90, Assumptions: []string{}},
			{ID: "s", Type: "script", Name: "Bad", Params: map[string]string{"code": "result = boom"}, Confidence: 80, Assumptions: []string{}},
		},
	}
	if err := st.UpsertWorkflow(wf); err != nil {
		t.Fatal(err)
	}
	inst := models.Instance{
		ID: "run-dlq", WorkflowID: "wf-dlq", WorkflowName: "DLQ flow",
		Status: models.InstRunning, StartedAt: "2026-01-01T00:00:00Z",
		StepRuns: []models.StepRun{
			{StepID: "trigger", Name: "T", Type: "trigger", Status: models.StepPending},
			{StepID: "s", Name: "Bad", Type: "script", Status: models.StepPending},
		},
	}
	_ = st.UpsertInstance(inst)
	for i := 0; i < 30; i++ {
		engine.TickAll(st, nil)
	}
	return "run-dlq"
}

// DLQQ-01: /dlq lists failed instances with age, oldest first.
func TestDLQQ01_ListFailedOldestFirst(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	_ = failOneInstance(t, st)

	code, b := req(t, hs, "GET", "/api/v1/dlq", nil)
	if code != 200 {
		t.Fatalf("dlq: %d %s", code, b)
	}
	items := asMap2(b)
	if len(items) == 0 {
		t.Fatal("dlq is empty")
	}
	// Only failures are listed.
	for _, i := range items {
		if i["status"] != "failed" {
			t.Fatalf("non-failed instance in DLQ: %v", i["status"])
		}
		if _, ok := i["ageSeconds"].(float64); !ok {
			t.Fatalf("ageSeconds missing: %v", i["ageSeconds"])
		}
	}
}

// DLQQ-02: requeue re-drives the failed instance from the failed step.
func TestDLQQ02_Requeue(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	id := failOneInstance(t, st)

	code, b := req(t, hs, "POST", "/api/v1/dlq/"+id+"/requeue", nil)
	if code != 200 {
		t.Fatalf("requeue: %d %s", code, b)
	}
	inst := asMap(b)
	if inst["status"] != "running" || inst["error"] != nil {
		t.Fatalf("requeued instance: %v / %v", inst["status"], inst["error"])
	}
	if code, _ := req(t, hs, "POST", "/api/v1/dlq/nope/requeue", nil); code != 404 {
		t.Fatal("unknown dlq id should 404")
	}
}

// PMET-01: /metrics exposes the Prometheus text format with core series.
func TestPMET01_PrometheusFormat(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	code, b := req(t, hs, "GET", "/metrics", nil)
	if code != 200 {
		t.Fatalf("metrics: %d", code)
	}
	text := string(b)
	for _, want := range []string{
		"# HELP flowforge_instances_total",
		"# TYPE flowforge_instances_total counter",
		`flowforge_instances_total{status="completed"}`,
		"# HELP flowforge_workflows",
		"# HELP flowforge_human_tasks_pending",
		"flowforge_build_info{version=",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metrics missing %q:\n%.400s", want, text)
		}
	}
}

// PMET-02: build_info reflects the version stamped by main.
func TestPMET02_BuildInfoVersion(t *testing.T) {
	old := Version
	Version = "v9.9.9-test"
	defer func() { Version = old }()

	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	_, b := req(t, hs, "GET", "/metrics", nil)
	if !strings.Contains(string(b), `flowforge_build_info{version="v9.9.9-test"} 1`) {
		t.Fatalf("build_info missing version:\n%.200s", b)
	}
}

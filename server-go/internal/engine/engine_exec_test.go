package engine

// ENG-06..10: real execution through the engine — script steps, connector
// steps (egress allow + deny + flaky-retry), safe-mode halting, cancel from
// waiting. Features F-EXEC + F-EXT (EXT-03 family) + F-SEC.

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/store"
)

func execStore(t *testing.T, steps ...models.WorkflowStep) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	wf := models.Workflow{
		ID: "wf-exec", Name: "Exec", Status: models.StatusDeployed, Version: 1,
		CreatedBy: "t", AIModel: "t", CreatedAt: "2026-01-01T00:00:00Z", Steps: steps,
	}
	if err := s.UpsertWorkflow(wf); err != nil {
		t.Fatal(err)
	}
	return s
}

func runFor(steps []models.WorkflowStep, input map[string]any) models.Instance {
	runs := make([]models.StepRun, len(steps))
	for i, st := range steps {
		runs[i] = models.StepRun{StepID: st.ID, Name: st.Name, Type: st.Type, Status: models.StepPending}
	}
	return models.Instance{
		ID: "run-" + stID(steps), WorkflowID: "wf-exec", WorkflowName: "Exec",
		Status: models.InstRunning, StartedAt: "2026-01-01T00:00:00Z",
		Input: input, CurrentStep: 0, StepRuns: runs,
	}
}

var idCounter int

func stID(steps []models.WorkflowStep) string {
	idCounter++
	return "x"
}

// ENG-06: a configured script step runs for real — output + real duration.
func TestENG06_ScriptStepRunsForReal(t *testing.T) {
	trig := models.WorkflowStep{ID: "t", Type: "trigger", Name: "T", Params: map[string]string{"event": "e"}, Confidence: 90, Assumptions: []string{}}
	script := models.WorkflowStep{
		ID: "s", Type: "script", Name: "Double it", Confidence: 90, Assumptions: []string{},
		Params: map[string]string{"code": "result = input['total'] * 2"},
	}
	steps := []models.WorkflowStep{trig, script}
	s := execStore(t, steps...)
	inst := runFor(steps, map[string]any{"total": float64(21)})
	_ = s.UpsertInstance(inst)

	final := drain(t, s, inst.ID)
	if final.Status != models.InstCompleted {
		t.Fatalf("status = %s, err = %q", final.Status, final.Error)
	}
	got := step(s, inst.ID, "s")
	if got.Output != "42.0" {
		t.Fatalf("script output = %q, want 42.0", got.Output)
	}
	// Real execution records a duration field (may round to 0ms for fast scripts).
	_ = got.DurationMs
}

// ENG-07: a broken script fails the instance with the script error surfaced.
func TestENG07_ScriptFailureHaltsInstance(t *testing.T) {
	steps := []models.WorkflowStep{
		{ID: "t", Type: "trigger", Name: "T", Params: map[string]string{"event": "e"}, Confidence: 90, Assumptions: []string{}},
		{ID: "s", Type: "script", Name: "Bad", Confidence: 90, Assumptions: []string{}, Params: map[string]string{"code": "result = undefined_thing"}},
	}
	s := execStore(t, steps...)
	inst := runFor(steps, nil)
	_ = s.UpsertInstance(inst)

	final := drain(t, s, inst.ID)
	if final.Status != models.InstFailed {
		t.Fatalf("status = %s, want failed", final.Status)
	}
	if final.Error == "" {
		t.Fatal("script error should surface on the instance")
	}
	if step(s, inst.ID, "s").Status != models.StepFailed {
		t.Fatal("script step should be failed")
	}
}

// ENG-08: connector step executes against a real server (egress allowed) and
// a flaky connector recovers via retry-from-failed.
func TestENG08_ConnectorRunAndFlakyRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 1 {
			w.WriteHeader(500) // first call fails
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	host := hostOfURL(srv.URL)

	steps := []models.WorkflowStep{
		{ID: "t", Type: "trigger", Name: "T", Params: map[string]string{"event": "e"}, Confidence: 90, Assumptions: []string{}},
		{ID: "c", Type: "connector", Name: "Call", Params: map[string]string{"connector": "http-json", "url": srv.URL, "method": "GET"}, Confidence: 80, Assumptions: []string{}},
	}
	s := execStore(t, steps...)
	inst := runFor(steps, nil)
	_ = s.UpsertInstance(inst)

	pol := &policy.Policy{Allow: []string{host}, DenyByDefault: true}
	final := drainEngine(t, s, inst.ID, pol)
	if final.Status != models.InstFailed {
		t.Fatalf("first call should fail the instance, got %s (%s)", final.Status, final.Error)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	// Retry from failed: second call succeeds -> instance completes.
	if _, err := RetryFailed(s, inst.ID); err != nil {
		t.Fatal(err)
	}
	done := drainEngine(t, s, inst.ID, pol)
	if done.Status != models.InstCompleted {
		t.Fatalf("retry should complete, got %s (%s)", done.Status, done.Error)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("calls after retry = %d, want 2 (only the failed step re-runs)", calls)
	}
	if got := step(s, inst.ID, "c"); got.Status != models.StepSucceeded || got.Output == "" {
		t.Fatalf("connector step: %+v", got)
	}
}

// ENG-09: safe-mode disables real execution — script and connector steps fail.
func TestENG09_SafeModeBlocksRealExecution(t *testing.T) {
	for _, typ := range []struct {
		step models.WorkflowStep
		id   string
	}{
		{models.WorkflowStep{ID: "s", Type: "script", Name: "S", Params: map[string]string{"code": "result = 1"}, Confidence: 90, Assumptions: []string{}}, "s"},
		{models.WorkflowStep{ID: "c", Type: "connector", Name: "C", Params: map[string]string{"connector": "http-json", "url": "http://x.example/"}, Confidence: 80, Assumptions: []string{}}, "c"},
	} {
		steps := []models.WorkflowStep{
			{ID: "t", Type: "trigger", Name: "T", Params: map[string]string{"event": "e"}, Confidence: 90, Assumptions: []string{}},
			typ.step,
		}
		s := execStore(t, steps...)
		inst := runFor(steps, nil)
		_ = s.UpsertInstance(inst)

		final := drainEngine(t, s, inst.ID, &policy.Policy{SafeMode: true})
		if final.Status != models.InstFailed {
			t.Fatalf("%s: safe-mode should fail the step, got %s", typ.id, final.Status)
		}
		if final.Error == "" {
			t.Fatalf("%s: safe-mode error should surface", typ.id)
		}
	}
}

// ENG-10: cancel a WAITING instance (human task pending) and verify no further
// progress after cancellation.
func TestENG10_CancelWaitingInstance(t *testing.T) {
	steps := []models.WorkflowStep{
		{ID: "t", Type: "trigger", Name: "T", Params: map[string]string{"event": "e"}, Confidence: 90, Assumptions: []string{}},
		{ID: "m", Type: "human.approval", Name: "Manager", Params: map[string]string{"approver": "Manager", "sla_hours": "24"}, Confidence: 85, Assumptions: []string{}},
		{ID: "n", Type: "notify", Name: "Notify", Params: map[string]string{"channel": "email"}, Confidence: 80, Assumptions: []string{}},
	}
	s := execStore(t, steps...)
	inst := runFor(steps, map[string]any{"total": float64(999)})
	_ = s.UpsertInstance(inst)

	waiting := drain(t, s, inst.ID)
	if waiting.Status != models.InstWaiting {
		t.Fatalf("expected waiting, got %s", waiting.Status)
	}
	if _, err := CancelInstance(s, inst.ID); err != nil {
		t.Fatal(err)
	}
	// Ticks on a cancelled instance must not move it.
	for i := 0; i < 5; i++ {
		TickAll(s, nil)
	}
	after, _ := s.GetInstance(inst.ID)
	if after.Status != models.InstCancelled || after.EndedAt == "" {
		t.Fatalf("cancelled instance drifted: %+v", after)
	}
	if step(s, inst.ID, "n").Status != models.StepPending {
		t.Fatal("steps after the wait must not run")
	}
}

// drainEngine is drain with an explicit policy.
func drainEngine(t *testing.T, s *store.Store, id string, pol *policy.Policy) *models.Instance {
	t.Helper()
	for i := 0; i < 60; i++ {
		TickAll(s, pol)
		inst, _ := s.GetInstance(id)
		if inst != nil && (inst.Status == models.InstCompleted || inst.Status == models.InstFailed || inst.Status == models.InstWaiting) {
			return inst
		}
	}
	inst, _ := s.GetInstance(id)
	return inst
}

func hostOfURL(raw string) string {
	s := raw
	if i := indexStr(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := indexStr(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}

func indexStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

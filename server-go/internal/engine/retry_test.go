package engine

// RTY-01..05: per-step automatic retries (params.retries / retry_delay).

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/santhoshmp/flowforge/internal/models"
	"github.com/santhoshmp/flowforge/internal/store"
)

func retryStore(t *testing.T, step models.WorkflowStep) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	wf := models.Workflow{
		ID: "wf-retry", Name: "Retry", Status: models.StatusDeployed, Version: 1,
		CreatedBy: "t", AIModel: "t", CreatedAt: "2026-01-01T00:00:00Z",
		Steps: []models.WorkflowStep{
			{ID: "trigger", Type: "trigger", Name: "T", Params: map[string]string{"event": "e"}, Confidence: 90, Assumptions: []string{}},
			step,
		},
	}
	if err := s.UpsertWorkflow(wf); err != nil {
		t.Fatal(err)
	}
	runs := []models.StepRun{
		{StepID: "trigger", Name: "T", Type: "trigger", Status: models.StepPending},
		{StepID: step.ID, Name: step.Name, Type: step.Type, Status: models.StepPending},
	}
	_ = s.UpsertInstance(models.Instance{
		ID: "run-retry", WorkflowID: "wf-retry", WorkflowName: "Retry",
		Status: models.InstRunning, StartedAt: "2026-01-01T00:00:00Z", StepRuns: runs,
	})
	return s
}

func drainN(t *testing.T, s *store.Store, id string, n int) *models.Instance {
	t.Helper()
	for i := 0; i < n; i++ {
		TickAll(s, nil)
	}
	inst, _ := s.GetInstance(id)
	return inst
}

// RTY-01: exhausting retries fails the instance with the attempt count.
func TestRTY01_ExhaustedRetriesFail(t *testing.T) {
	s := retryStore(t, models.WorkflowStep{
		ID: "s", Type: "script", Name: "Bad", Params: map[string]string{
			"code": "result = boom", "retries": "2",
		}, Confidence: 90, Assumptions: []string{},
	})
	inst := drainN(t, s, "run-retry", 40)
	if inst.Status != models.InstFailed {
		t.Fatalf("status = %s, want failed", inst.Status)
	}
	if got := inst.StepRuns[1].Attempts; got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if !contains(inst.Error, "after 2 retries") {
		t.Fatalf("error = %q", inst.Error)
	}
	// The transient "retry N/N" note is replaced by the final error on the
	// last attempt; attempts count proves both retries were used.
	if !contains(inst.StepRuns[1].Note, "script error") {
		t.Fatalf("final note = %q", inst.StepRuns[1].Note)
	}
}

// RTY-02: a transient failure recovers within the retry budget.
func TestRTY02_TransientFailureRecovers(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(503) // transient
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := retryStore(t, models.WorkflowStep{
		ID: "c", Type: "connector", Name: "Call", Params: map[string]string{
			"connector": "http-json", "url": srv.URL, "method": "GET", "retries": "3",
		}, Confidence: 80, Assumptions: []string{},
	})
	inst := drainN(t, s, "run-retry", 40)
	if inst.Status != models.InstCompleted {
		t.Fatalf("status = %s (err=%q)", inst.Status, inst.Error)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("calls = %d, want 2 (first fails, retry succeeds)", calls)
	}
	if inst.StepRuns[1].Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", inst.StepRuns[1].Attempts)
	}
	if !contains(inst.StepRuns[1].Note, "retry 1/3") {
		t.Fatalf("note = %q", inst.StepRuns[1].Note)
	}
}

// RTY-03: retry_delay parks the step until the backoff elapses.
func TestRTY03_BackoffGate(t *testing.T) {
	s := retryStore(t, models.WorkflowStep{
		ID: "s", Type: "script", Name: "Bad", Params: map[string]string{
			"code": "result = boom", "retries": "1", "retry_delay": "3600",
		}, Confidence: 90, Assumptions: []string{},
	})
	inst := drainN(t, s, "run-retry", 30)
	if inst.Status != models.InstRunning {
		t.Fatalf("status = %s, want running (parked in backoff)", inst.Status)
	}
	step := inst.StepRuns[1]
	if step.Status != models.StepPending || step.NextAttemptAt == "" {
		t.Fatalf("step should be pending with a backoff gate: %+v", step)
	}
	if !contains(step.Note, "retry 1/1") {
		t.Fatalf("note = %q", step.Note)
	}
	// The failing step is NOT re-executed while parked (still exactly one
	// script failure recorded in the note; attempts stays 1).
	if step.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1 while parked", step.Attempts)
	}
}

// RTY-04: without a retries param, failure is immediate (legacy behavior).
func TestRTY04_NoRetriesParamFailsFast(t *testing.T) {
	s := retryStore(t, models.WorkflowStep{
		ID: "s", Type: "script", Name: "Bad", Params: map[string]string{
			"code": "result = boom",
		}, Confidence: 90, Assumptions: []string{},
	})
	inst := drainN(t, s, "run-retry", 40)
	if inst.Status != models.InstFailed || inst.StepRuns[1].Attempts != 0 {
		t.Fatalf("instance = %s attempts = %d", inst.Status, inst.StepRuns[1].Attempts)
	}
}

// RTY-05: absurd retry budgets are clamped (robustness, not trust).
func TestRTY05_BudgetClamped(t *testing.T) {
	if got := stepRetries(&models.WorkflowStep{Params: map[string]string{"retries": "99"}}); got != 10 {
		t.Fatalf("retries clamped to %d, want 10", got)
	}
	if got := stepRetryDelay(&models.WorkflowStep{Params: map[string]string{"retry_delay": "99999"}}); got != 3600 {
		t.Fatalf("delay clamped to %d, want 3600", got)
	}
	if got := stepRetries(nil); got != 0 || stepRetryDelay(nil) != 0 {
		t.Fatal("nil step must mean no retries/delay")
	}
}

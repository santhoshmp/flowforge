package engine

import (
	"testing"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/store"
)

// Go mirror of ENG-01..05 (see docs/test-strategy.md). Feature F-EXEC.

func sampleStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	wf := models.Workflow{
		ID: "wf-test", Name: "Test", Status: models.StatusDeployed, Version: 1,
		CreatedBy: "tester", AIModel: "test", CreatedAt: "2026-01-01T00:00:00Z",
		Steps: []models.WorkflowStep{
			{ID: "trig", Type: "trigger", Name: "Record received", Params: map[string]string{"event": "x.created"}, Confidence: 95, Assumptions: []string{}},
			{ID: "cond", Type: "condition", Name: "Amount over 100?", Params: map[string]string{"expression": "total > 100", "on_false": "auto_approve"}, Confidence: 90, Assumptions: []string{}},
			{ID: "mgr", Type: "human.approval", Name: "Manager approval", Params: map[string]string{"approver": "Manager", "sla_hours": "24"}, Confidence: 85, Assumptions: []string{}},
			{ID: "esc", Type: "human.approval", Name: "Escalation", Params: map[string]string{"approver": "VP", "condition": "previous_step.sla_breached"}, Confidence: 70, Assumptions: []string{}},
			{ID: "post", Type: "integration.post", Name: "Post to ERP", Params: map[string]string{"system": "ERP"}, Confidence: 80, Assumptions: []string{}},
		},
	}
	if err := s.UpsertWorkflow(wf); err != nil {
		t.Fatalf("upsert wf: %v", err)
	}
	return s
}

func newRun(wfID, wfName string, total float64) models.Instance {
	return models.Instance{
		ID: "run-" + utilTestUID(), WorkflowID: wfID, WorkflowName: wfName, Status: models.InstRunning,
		Entity: "REC-1", StartedAt: "2026-01-01T00:00:00Z",
		Input:       map[string]any{"total": total},
		CurrentStep: 0,
		StepRuns: []models.StepRun{
			{StepID: "trig", Name: "Record received", Type: "trigger", Status: models.StepPending},
			{StepID: "cond", Name: "Amount over 100?", Type: "condition", Status: models.StepPending},
			{StepID: "mgr", Name: "Manager approval", Type: "human.approval", Status: models.StepPending},
			{StepID: "esc", Name: "Escalation", Type: "human.approval", Status: models.StepPending},
			{StepID: "post", Name: "Post to ERP", Type: "integration.post", Status: models.StepPending},
		},
	}
}

func utilTestUID() string { return "abc1" }

// drive ticks until pred(inst) is true (or max ticks).
func drive(t *testing.T, s *store.Store, id string, pred func(*models.Instance) bool, max int) *models.Instance {
	t.Helper()
	for i := 0; i < max; i++ {
		TickAll(s, nil)
		inst, _ := s.GetInstance(id)
		if inst != nil && pred(inst) {
			return inst
		}
	}
	inst, _ := s.GetInstance(id)
	return inst
}
func drain(t *testing.T, s *store.Store, id string) *models.Instance {
	return drive(t, s, id, func(i *models.Instance) bool {
		return i.Status == models.InstCompleted || i.Status == models.InstFailed || i.Status == models.InstWaiting
	}, 60)
}

func step(s *store.Store, instID, stepID string) models.StepRun {
	i, _ := s.GetInstance(instID)
	for _, r := range i.StepRuns {
		if r.StepID == stepID {
			return r
		}
	}
	return models.StepRun{}
}

func TestENG01_WaitsAtAboveThreshold(t *testing.T) {
	s := sampleStore(t)
	defer s.Close()
	inst := newRun("wf-test", "Test", 500)
	if err := s.UpsertInstance(inst); err != nil {
		t.Fatal(err)
	}
	cur := drain(t, s, inst.ID)
	if cur.Status != models.InstWaiting || cur.WaitingOn != "Manager" {
		t.Fatalf("expected waiting on Manager, got %s/%q", cur.Status, cur.WaitingOn)
	}
}

func TestENG02_CompletesAfterApprovalEscalationSkips(t *testing.T) {
	s := sampleStore(t)
	defer s.Close()
	inst := newRun("wf-test", "Test", 500)
	_ = s.UpsertInstance(inst)
	drain(t, s, inst.ID)
	if _, err := ApproveWaiting(s, inst.ID); err != nil {
		t.Fatal(err)
	}
	final := drain(t, s, inst.ID)
	if final.Status != models.InstCompleted {
		t.Fatalf("expected completed, got %s", final.Status)
	}
	if step(s, inst.ID, "esc").Status != models.StepSkipped {
		t.Errorf("escalation should be skipped")
	}
	if step(s, inst.ID, "post").Status != models.StepSucceeded {
		t.Errorf("post should succeed")
	}
	if final.EndedAt == "" {
		t.Errorf("endedAt should be set")
	}
}

func TestENG03_AutoApprovesBelowThreshold(t *testing.T) {
	s := sampleStore(t)
	defer s.Close()
	inst := newRun("wf-test", "Test", 10) // <= 100 -> auto-approve
	_ = s.UpsertInstance(inst)
	final := drain(t, s, inst.ID)
	if final.Status != models.InstCompleted {
		t.Fatalf("expected completed (never waited), got %s", final.Status)
	}
	mgr := step(s, inst.ID, "mgr")
	if mgr.Status != models.StepSucceeded || mgr.Output == "" {
		t.Errorf("manager should auto-approve: %+v", mgr)
	}
	if !contains(mgr.Output, "auto-approved") {
		t.Errorf("manager output should mention auto-approved: %q", mgr.Output)
	}
}

func TestENG04_RetryFromFailedNoRerun(t *testing.T) {
	s := sampleStore(t)
	defer s.Close()
	inst := newRun("wf-test", "Test", 500)
	inst.Status = models.InstFailed
	inst.Error = "boom"
	inst.CurrentStep = 4 // post
	inst.StepRuns[0].Status = models.StepSucceeded
	inst.StepRuns[1].Status = models.StepSucceeded
	inst.StepRuns[2].Status = models.StepSucceeded
	inst.StepRuns[3].Status = models.StepSkipped
	inst.StepRuns[4].Status = models.StepFailed
	_ = s.UpsertInstance(inst)
	if _, err := RetryFailed(s, inst.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetInstance(inst.ID)
	if after.Status != models.InstRunning || after.Error != "" {
		t.Fatalf("retry should resume: %+v", after)
	}
	if after.StepRuns[0].Status != models.StepSucceeded {
		t.Errorf("completed steps must be preserved")
	}
	final := drain(t, s, inst.ID)
	if final.Status != models.InstCompleted {
		t.Errorf("expected completed after retry, got %s", final.Status)
	}
}

func TestENG05_CancelSetsEndedAt(t *testing.T) {
	s := sampleStore(t)
	defer s.Close()
	inst := newRun("wf-test", "Test", 500)
	_ = s.UpsertInstance(inst)
	TickAll(s, nil)
	if _, err := CancelInstance(s, inst.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetInstance(inst.ID)
	if after.Status != models.InstCancelled || after.EndedAt == "" {
		t.Fatalf("expected cancelled + endedAt: %+v", after)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && containsStr(s, sub)))
}
func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

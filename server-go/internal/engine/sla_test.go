package engine

// SLA-01..05: real SLA breaches — waiting approvals time out, escalation
// steps (condition previous_step.sla_breached) fire, non-breached waits and
// the auto-approve path stay untouched.

import (
	"testing"
	"time"

	"github.com/santhoshmp/flowforge/internal/models"
	"github.com/santhoshmp/flowforge/internal/store"
)

func slaStore(t *testing.T, slaHours string, withEscalation bool) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	steps := []models.WorkflowStep{
		{ID: "trigger", Type: "trigger", Name: "T", Params: map[string]string{"event": "leave.requested"}, Confidence: 90, Assumptions: []string{}},
		{ID: "mgr", Type: "human.approval", Name: "Manager approval", Params: map[string]string{"approver": "Reporting Manager", "sla_hours": slaHours}, Confidence: 90, Assumptions: []string{}},
	}
	if withEscalation {
		steps = append(steps,
			models.WorkflowStep{ID: "esc", Type: "human.approval", Name: "Escalation", Params: map[string]string{"approver": "HR Escalation", "condition": "previous_step.sla_breached"}, Confidence: 90, Assumptions: []string{}},
		)
	}
	steps = append(steps,
		models.WorkflowStep{ID: "post", Type: "notify", Name: "Notify", Params: map[string]string{"channel": "email"}, Confidence: 80, Assumptions: []string{}},
	)
	if err := s.UpsertWorkflow(models.Workflow{
		ID: "wf-sla", Name: "SLA", Status: models.StatusDeployed, Version: 1,
		CreatedBy: "t", AIModel: "t", CreatedAt: "2026-01-01T00:00:00Z", Steps: steps,
	}); err != nil {
		t.Fatal(err)
	}
	runs := []models.StepRun{
		{StepID: "trigger", Name: "T", Type: "trigger", Status: models.StepPending},
		{StepID: "mgr", Name: "Manager approval", Type: "human.approval", Status: models.StepPending},
	}
	if withEscalation {
		runs = append(runs, models.StepRun{StepID: "esc", Name: "Escalation", Type: "human.approval", Status: models.StepPending})
	}
	runs = append(runs, models.StepRun{StepID: "post", Name: "Notify", Type: "notify", Status: models.StepPending})
	_ = s.UpsertInstance(models.Instance{
		ID: "run-sla", WorkflowID: "wf-sla", WorkflowName: "SLA", Status: models.InstRunning,
		StartedAt: "2026-01-01T00:00:00Z", StepRuns: runs,
	})
	return s
}

// SLA-01: a breached wait skips the approval and the escalation step fires,
// re-waiting on the escalation approver.
func TestSLA01_BreachEscalates(t *testing.T) {
	s := slaStore(t, "24", true)
	t0 := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)

	// Drive to waiting on the manager.
	for i := 0; i < 10; i++ {
		TickAllAt(s, nil, t0)
	}
	inst, _ := s.GetInstance("run-sla")
	if inst.Status != models.InstWaiting || inst.WaitingOn != "Reporting Manager" {
		t.Fatalf("precondition: %+v", inst)
	}

	// 25 hours later: breach -> skip + escalation fires.
	t1 := t0.Add(25 * time.Hour)
	for i := 0; i < 8; i++ {
		TickAllAt(s, nil, t1)
	}
	inst, _ = s.GetInstance("run-sla")
	mgr := stepOf(inst, "mgr")
	if mgr.Status != models.StepSkipped || mgr.Note != "SLA breached after 24h — escalated" {
		t.Fatalf("manager step: %+v", mgr)
	}
	esc := stepOf(inst, "esc")
	if esc.Status != models.StepWaiting || esc.Note != "Waiting on HR Escalation" {
		t.Fatalf("escalation step should wait on the escalator: %+v", esc)
	}
	if inst.Status != models.InstWaiting || inst.WaitingOn != "HR Escalation" {
		t.Fatalf("should wait on the escalator: %s / %q", inst.Status, inst.WaitingOn)
	}
}

// SLA-02: inside the SLA window the wait is untouched.
func TestSLA02_WithinWindowStillWaiting(t *testing.T) {
	s := slaStore(t, "24", true)
	t0 := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		TickAllAt(s, nil, t0)
	}
	for i := 0; i < 5; i++ {
		TickAllAt(s, nil, t0.Add(23*time.Hour))
	}
	inst, _ := s.GetInstance("run-sla")
	if inst.Status != models.InstWaiting || inst.WaitingOn != "Reporting Manager" {
		t.Fatalf("premature breach: %+v", inst)
	}
	if m := stepOf(inst, "mgr"); m.Status != models.StepWaiting {
		t.Fatalf("manager step: %+v", m)
	}
}

// SLA-03: no escalation step after a breach — the run simply continues.
func TestSLA03_BreachWithoutEscalationStep(t *testing.T) {
	s := slaStore(t, "24", false)
	t0 := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		TickAllAt(s, nil, t0)
	}
	for i := 0; i < 8; i++ {
		TickAllAt(s, nil, t0.Add(25*time.Hour))
	}
	inst, _ := s.GetInstance("run-sla")
	if inst.Status != models.InstCompleted {
		t.Fatalf("status = %s (run should continue past the breached gate)", inst.Status)
	}
	if m := stepOf(inst, "mgr"); m.Status != models.StepSkipped {
		t.Fatalf("manager step: %+v", m)
	}
}

// SLA-04: fractional-hour SLAs (demo pace) work.
func TestSLA04_FractionalHours(t *testing.T) {
	s := slaStore(t, "0.02", true) // ~72 seconds
	t0 := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		TickAllAt(s, nil, t0)
	}
	if inst, _ := s.GetInstance("run-sla"); inst.Status != models.InstWaiting {
		t.Fatal("precondition: waiting")
	}
	for i := 0; i < 8; i++ {
		TickAllAt(s, nil, t0.Add(90*time.Second))
	}
	inst, _ := s.GetInstance("run-sla")
	if stepOf(inst, "mgr").Status != models.StepSkipped {
		t.Fatalf("fractional SLA did not breach: %+v", stepOf(inst, "mgr"))
	}
}

// SLA-05: no SLA configured -> waits forever (no breach).
func TestSLA05_NoSLANeverBreaches(t *testing.T) {
	s := slaStore(t, "", true)
	t0 := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		TickAllAt(s, nil, t0)
	}
	for i := 0; i < 5; i++ {
		TickAllAt(s, nil, t0.Add(30*24*time.Hour))
	}
	inst, _ := s.GetInstance("run-sla")
	if inst.Status != models.InstWaiting || inst.WaitingOn != "Reporting Manager" {
		t.Fatalf("breached without an SLA: %+v", inst)
	}
}

func stepOf(inst *models.Instance, id string) models.StepRun {
	for _, r := range inst.StepRuns {
		if r.StepID == id {
			return r
		}
	}
	return models.StepRun{}
}

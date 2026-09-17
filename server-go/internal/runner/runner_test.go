package runner

// RUN-01..05: the standalone runner (F-RUNNER). Artifacts run headlessly on
// the durable engine with the same executors/policy as `serve`.

import (
	"errors"
	"strings"
	"testing"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/store"
)

const invoiceArtifact = `apiVersion: flowforge/v1
kind: Workflow
metadata:
  name: standalone-invoice
  version: 1
  createdBy: runner test
spec:
  description: condition + human approval + post
  trigger:
    event: invoice.created
  steps:
    - id: amount_check
      type: condition
      name: Over 100?
      params:
        expression: total > 100
        on_false: auto_approve
    - id: mgr
      type: human.approval
      name: Manager approval
      params:
        approver: Manager
        sla_hours: "24"
    - id: post
      type: integration.post
      name: Post to ERP
      params:
        system: ERP
`

func run(t *testing.T, input map[string]any, opts Options) (*models.Instance, error) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	opts.Input = input
	return Run(s, invoiceArtifact, opts)
}

// RUN-01: above threshold with auto-approve completes; escalation-free walk.
func TestRUN01_CompletesWithAutoApprove(t *testing.T) {
	var log []string
	inst, err := run(t, map[string]any{"total": float64(500)}, Options{
		AutoApprove: true,
		Progress:    func(l string) { log = append(log, l) },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if inst.Status != models.InstCompleted {
		t.Fatalf("status = %s (err=%q)", inst.Status, inst.Error)
	}
	joined := strings.Join(log, "\n")
	if !strings.Contains(joined, "auto-approved Manager") {
		t.Errorf("progress should show the auto-approval:\n%s", joined)
	}
	if !strings.Contains(joined, "ok") {
		t.Errorf("progress should report step success:\n%s", joined)
	}
	if inst.EndedAt == "" {
		t.Error("completed run must record endedAt")
	}
}

// RUN-02: without a resolver the run stops at the human task (ErrWaiting).
func TestRUN02_StopsWaitingWithoutResolver(t *testing.T) {
	inst, err := run(t, map[string]any{"total": float64(500)}, Options{})
	if !errors.Is(err, ErrWaiting) {
		t.Fatalf("want ErrWaiting, got %v", err)
	}
	if inst == nil || inst.Status != models.InstWaiting || inst.WaitingOn != "Manager" {
		t.Fatalf("instance: %+v", inst)
	}
}

// RUN-03: below threshold the condition auto-approves — no resolver needed.
func TestRUN03_ConditionAutoApprovePath(t *testing.T) {
	inst, err := run(t, map[string]any{"total": float64(5)}, Options{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if inst.Status != models.InstCompleted {
		t.Fatalf("below-threshold run should complete unattended, got %s (%s)", inst.Status, inst.Error)
	}
	for _, r := range inst.StepRuns {
		if r.StepID == "mgr" && r.Status != models.StepSucceeded {
			t.Fatalf("manager should auto-approve: %+v", r)
		}
	}
}

// RUN-04: interactive resolver approves mid-run.
func TestRUN04_InteractiveResolver(t *testing.T) {
	inst, err := run(t, map[string]any{"total": float64(999)}, Options{
		Interactive: func(approver string) bool { return approver == "Manager" },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if inst.Status != models.InstCompleted {
		t.Fatalf("status = %s", inst.Status)
	}
}

// RUN-05: invalid artifacts are rejected before anything executes.
func TestRUN05_InvalidArtifactRejected(t *testing.T) {
	s, _ := store.Open(":memory:")
	defer s.Close()
	for _, bad := range []string{"", "not yaml: a flowforge artifact", "apiVersion: flowforge/v1\nkind: Workflow\n"} {
		if _, err := Run(s, bad, Options{}); err == nil {
			t.Fatalf("invalid artifact accepted: %q", bad)
		}
	}
}

// RUN-06: real script execution runs through the runner under a permissive
// policy, and a deny-egress policy fails the run loudly.
func TestRUN06_RealExecutionHonorsPolicy(t *testing.T) {
	const scriptArtifact = `apiVersion: flowforge/v1
kind: Workflow
metadata:
  name: script-flow
  version: 1
  createdBy: runner test
spec:
  description: script step
  trigger:
    event: manual
  steps:
    - id: calc
      type: script
      name: Double it
      params:
        code: "result = input['total'] * 2"
`
	s1, _ := store.Open(":memory:")
	defer s1.Close()
	inst, err := Run(s1, scriptArtifact, Options{Input: map[string]any{"total": float64(21)}})
	if err != nil || inst.Status != models.InstCompleted {
		t.Fatalf("script run: %v / %s (%s)", err, inst.Status, inst.Error)
	}
	if inst.StepRuns[1].Output != "42.0" {
		t.Fatalf("script output = %q", inst.StepRuns[1].Output)
	}

	// Safe-mode fails the same run.
	s2, _ := store.Open(":memory:")
	defer s2.Close()
	inst2, err := Run(s2, scriptArtifact, Options{
		Input:  map[string]any{"total": float64(21)},
		Policy: &policy.Policy{SafeMode: true},
	})
	if err != nil || inst2.Status != models.InstFailed {
		t.Fatalf("safe-mode should fail the script step: %v / %s", err, inst2.Status)
	}
	if !strings.Contains(inst2.Error, "safe-mode") {
		t.Fatalf("error = %q", inst2.Error)
	}
}

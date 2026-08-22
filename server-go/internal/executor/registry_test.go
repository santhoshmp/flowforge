package executor

// EXT-01/02: the step-executor registry (P4.1). Feature F-EXT.
// (The connector executor's registration via internal/connectors is covered
// by the connectors suite; this file covers the registry mechanics.)

import (
	"errors"
	"testing"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
)

func TestEXT01_Dispatch(t *testing.T) {
	// Known types dispatch to their executors.
	if e := ForType("script"); e == nil || e.Name() != "script" {
		t.Errorf("script executor = %v", e)
	}
	if e := ForType("integration.post"); e == nil || e.Name() != "integration.http" {
		t.Errorf("integration executor = %v", e)
	}
	// Unhandled types simulate (no executor).
	for _, ty := range []string{"trigger", "notify", "human.approval", "mdm.lookup"} {
		if e := ForType(ty); e != nil {
			t.Errorf("type %s should have no executor, got %s", ty, e.Name())
		}
		_, _, real := Run(&models.WorkflowStep{Type: ty}, nil, &policy.Policy{})
		if real {
			t.Errorf("type %s should simulate", ty)
		}
	}
}

// extTestExecutor is a test-only executor for a synthetic type.
type extTestExecutor struct{ ran *bool }

func (extTestExecutor) Name() string                         { return "test.custom" }
func (extTestExecutor) Matches(t string) bool                { return t == "test.custom-only" }
func (extTestExecutor) Configured(*models.WorkflowStep) bool { return true }
func (e extTestExecutor) Run(*models.WorkflowStep, map[string]any, *policy.Policy) (string, error) {
	*e.ran = true
	return "custom executed", nil
}

func TestEXT02_RegisterAndOverride(t *testing.T) {
	ran := false
	Register(extTestExecutor{ran: &ran})
	if e := ForType("test.custom-only"); e == nil || e.Name() != "test.custom" {
		t.Fatalf("registered executor not found")
	}
	out, err, real := Run(&models.WorkflowStep{Type: "test.custom-only"}, nil, &policy.Policy{})
	if !real || err != nil || out != "custom executed" || !ran {
		t.Fatalf("run = %q err=%v real=%v ran=%v", out, err, real, ran)
	}

	// Later registration wins: a second executor for the same type overrides.
	overridden := errors.New("overridden")
	Register(overrideExecutor{err: overridden})
	_, err, _ = Run(&models.WorkflowStep{Type: "test.custom-only"}, nil, &policy.Policy{})
	if err == nil || err.Error() != "overridden" {
		t.Fatalf("override failed: %v", err)
	}
}

type overrideExecutor struct{ err error }

func (overrideExecutor) Name() string                         { return "test.override" }
func (overrideExecutor) Matches(t string) bool                { return t == "test.custom-only" }
func (overrideExecutor) Configured(*models.WorkflowStep) bool { return true }
func (e overrideExecutor) Run(*models.WorkflowStep, map[string]any, *policy.Policy) (string, error) {
	return "", e.err
}

package api

// API-15/16: full product lifecycles through the public REST surface with the
// real durable engine (driven synchronously, like the scheduler does).
// Feature F-API + F-EXEC + F-EXT.

import (
	"testing"

	"github.com/flowforge/flowforge/internal/engine"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/store"
)

// driveInst ticks the engine (permissive policy) until pred or max.
func driveInst(t *testing.T, st *store.Store, id string, pred func(*models.Instance) bool, max int) *models.Instance {
	t.Helper()
	for i := 0; i < max; i++ {
		engine.TickAll(st, nil)
		inst, _ := st.GetInstance(id)
		if inst != nil && pred(inst) {
			return inst
		}
	}
	inst, _ := st.GetInstance(id)
	return inst
}

// API-15: template â†’ edit â†’ approve â†’ run â†’ WAIT â†’ human-approve â†’ COMPLETE.
// The flagship describeâ†’approveâ†’runâ†’track loop, end to end.
func TestAPI15_FullHappyPathLifecycle(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	// 1. Start from a template (P4.4).
	code, b := req(t, hs, "POST", "/api/v1/templates/vendor-invoice-approval/instantiate", nil)
	if code != 200 {
		t.Fatalf("instantiate: %d %s", code, b)
	}
	wf := asMap(b)
	id := wf["id"].(string)
	if wf["status"] != "draft" {
		t.Fatalf("template must instantiate as draft, got %v", wf["status"])
	}

	// 2. Human edits the draft (rename + bump version).
	code, b = req(t, hs, "PATCH", "/api/v1/workflows/"+id, map[string]any{"name": "Acme Invoice Flow", "version": 2})
	if code != 200 || asMap(b)["name"] != "Acme Invoice Flow" {
		t.Fatalf("edit: %d %s", code, b)
	}

	// 3. Draft cannot run yet.
	if code, _ := req(t, hs, "POST", "/api/v1/workflows/"+id+"/executions", nil); code != 400 {
		t.Fatal("draft execution must be refused")
	}

	// 4. Approve & deploy.
	if code, _ := req(t, hs, "POST", "/api/v1/workflows/"+id+"/approve", nil); code != 200 {
		t.Fatal("approve failed")
	}

	// 5. Run with input above the $10K threshold -> must reach a human wait.
	code, b = req(t, hs, "POST", "/api/v1/workflows/"+id+"/executions", map[string]any{
		"entity": "INV-777 Â· Acme Corp (V-10293)",
		"input":  map[string]any{"total": 24000},
	})
	if code != 200 {
		t.Fatalf("run: %d %s", code, b)
	}
	inst := asMap(b)
	instID := inst["id"].(string)

	waiting := driveInst(t, st, instID, func(i *models.Instance) bool { return i.Status == models.InstWaiting }, 40)
	if waiting.Status != models.InstWaiting || waiting.WaitingOn != "Cost-Center Manager" {
		t.Fatalf("expected waiting on Cost-Center Manager, got %s/%q", waiting.Status, waiting.WaitingOn)
	}

	// 6. Resolve the human task through the API.
	code, b = req(t, hs, "POST", "/api/v1/executions/"+instID+"/approve", nil)
	if code != 200 {
		t.Fatalf("task approve: %d %s", code, b)
	}

	// 7. Runs to completion; escalation skipped; ERP step last.
	final := driveInst(t, st, instID, func(i *models.Instance) bool { return i.Status == models.InstCompleted }, 40)
	if final.Status != models.InstCompleted {
		t.Fatalf("lifecycle did not complete: %s (%s)", final.Status, final.Error)
	}
	if final.EndedAt == "" {
		t.Fatal("completed instance must record endedAt")
	}
	for _, r := range final.StepRuns {
		if r.Type == "human.approval" && r.Note != "" {
			t.Fatalf("waiting note should clear after approval: %+v", r)
		}
	}

	// 8. The audit trail captured the loop (create from template, approval, run, task).
	_, b = req(t, hs, "GET", "/api/v1/audit", nil)
	audit := string(b)
	for _, want := range []string{"Workflow created from template", "Approved & deployed", "Execution started", "Human task approved"} {
		if !containsStr(audit, want) {
			t.Fatalf("audit trail missing %q", want)
		}
	}
}

// API-16: script-step failure surfaces on the instance; retry endpoint
// resumes; below-threshold input auto-approves (condition drives the flow).
func TestAPI16_FailureRetryAndAutoApprove(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	mk := func(steps []map[string]any) string {
		code, b := req(t, hs, "POST", "/api/v1/workflows", map[string]any{
			"name": "Retry flow", "description": "d", "prompt": "p", "steps": steps,
		})
		if code != 200 {
			t.Fatalf("create: %d %s", code, b)
		}
		id := asMap(b)["id"].(string)
		if code, _ := req(t, hs, "POST", "/api/v1/workflows/"+id+"/approve", nil); code != 200 {
			t.Fatal("approve failed")
		}
		return id
	}
	trigger := map[string]any{"id": "t", "type": "trigger", "name": "T", "params": map[string]string{"event": "e"}, "confidence": 90, "assumptions": []string{}}
	cond := map[string]any{"id": "c", "type": "condition", "name": "Over 100?", "params": map[string]string{"expression": "total > 100", "on_false": "auto_approve"}, "confidence": 90, "assumptions": []string{}}
	mgr := map[string]any{"id": "m", "type": "human.approval", "name": "Mgr", "params": map[string]string{"approver": "Manager"}, "confidence": 85, "assumptions": []string{}}
	badScript := map[string]any{"id": "s", "type": "script", "name": "S", "params": map[string]string{"code": "result = boom"}, "confidence": 80, "assumptions": []string{}}

	// A: failing script -> instance failed with the script error; retry keeps
	// the failed status until the step is fixed (retry alone re-runs the same
	// failing step) â€” assert the error path honestly.
	wfA := mk([]map[string]any{trigger, badScript})
	code, b := req(t, hs, "POST", "/api/v1/workflows/"+wfA+"/executions", map[string]any{"input": map[string]any{}})
	if code != 200 {
		t.Fatalf("run A: %d", code)
	}
	idA := asMap(b)["id"].(string)
	failed := driveInst(t, st, idA, func(i *models.Instance) bool { return i.Status == models.InstFailed }, 40)
	if failed.Status != models.InstFailed || failed.Error == "" {
		t.Fatalf("script failure not surfaced: %+v", failed)
	}
	if code, _ := req(t, hs, "POST", "/api/v1/executions/"+idA+"/retry", nil); code != 200 {
		t.Fatal("retry endpoint failed")
	}
	again := driveInst(t, st, idA, func(i *models.Instance) bool { return i.Status == models.InstFailed }, 40)
	if again.Status != models.InstFailed {
		t.Fatalf("retrying an unfixed step stays failed: %+v", again)
	}

	// B: condition below threshold -> the human step auto-approves; no wait.
	wfB := mk([]map[string]any{trigger, cond, mgr})
	code, b = req(t, hs, "POST", "/api/v1/workflows/"+wfB+"/executions", map[string]any{"input": map[string]any{"total": 5}})
	if code != 200 {
		t.Fatalf("run B: %d", code)
	}
	idB := asMap(b)["id"].(string)
	final := driveInst(t, st, idB, func(i *models.Instance) bool { return i.Status == models.InstCompleted }, 40)
	if final.Status != models.InstCompleted {
		t.Fatalf("auto-approve path did not complete: %s (%s)", final.Status, final.Error)
	}
	for _, r := range final.StepRuns {
		if r.StepID == "m" && r.Status != models.StepSucceeded {
			t.Fatalf("manager should auto-approve below threshold: %+v", r)
		}
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

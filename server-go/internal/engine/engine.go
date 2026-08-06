// Package engine is the durable execution engine for the Go control plane.
// It mirrors server/src/engine.ts: TickAll advances every running instance by
// one persisted transition; human approvals wait; conditions evaluate run
// input; SLA-escalation steps skip. The scheduler calls TickAll on an interval;
// tests call it directly for determinism.
package engine

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flowforge/flowforge/internal/executor"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/store"
	"github.com/flowforge/flowforge/internal/util"
)

// runTicks counts how many ticks the current step has been "running" before it
// resolves (lost on restart; a running step simply re-runs — demo-safe).
var runTicks = map[string]int{}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

func audit(actor, action, detail, kind string) models.AuditEntry {
	return models.AuditEntry{ID: util.UID(), At: nowISO(), Actor: actor, Action: action, Detail: detail, Kind: kind}
}

func durationTicks(t string) int {
	switch t {
	case "trigger", "condition":
		return 1
	case "ai.extract", "ai.classify":
		return 3
	case "human.approval":
		return 2
	default:
		return 2
	}
}

func approverOf(s *models.WorkflowStep) string {
	if s != nil {
		if a := s.Params["approver"]; a != "" {
			return a
		}
	}
	return "approver"
}

var condRe = regexp.MustCompile(`^([A-Za-z0-9_.]+)\s*>\s*([0-9.]+)`)

// evalCondition evaluates a "var > N" expression against the run input,
// falling back to a "total" field. Returns (result, ok).
func evalCondition(expr string, input map[string]any) (bool, bool) {
	m := condRe.FindStringSubmatch(expr)
	if m == nil {
		return false, false
	}
	var raw any
	if v, ok := input[m[1]]; ok {
		raw = v
	} else if v, ok := input["total"]; ok {
		raw = v
	}
	val, ok := toFloat(raw)
	if !ok {
		return false, false
	}
	threshold, _ := strconv.ParseFloat(m[2], 64)
	return val > threshold, true
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func stepOutput(cur *models.StepRun, wfStep *models.WorkflowStep) string {
	switch cur.Type {
	case "trigger":
		return "record received"
	case "ai.extract":
		return "fields extracted · confidence 0.97"
	case "ai.classify":
		return "classified: standard"
	case "mdm.validate":
		return "matched golden record"
	case "mdm.lookup":
		return "entity resolved"
	case "condition":
		if wfStep != nil {
			return wfStep.Params["expression"] + " → true"
		}
		return "condition → true"
	case "notify":
		ch := "email"
		if wfStep != nil && wfStep.Params["channel"] != "" {
			ch = wfStep.Params["channel"]
		}
		return "sent via " + ch
	case "integration.post":
		sys := "target"
		if wfStep != nil && wfStep.Params["system"] != "" {
			sys = wfStep.Params["system"]
		}
		return "posted to " + sys + " · 200 OK"
	case "integration.http":
		return "HTTP 200"
	default:
		return "done"
	}
}

// TickAll advances every running instance by exactly one transition. A nil
// policy is treated as permissive (scripts/HTTP allowed, egress unrestricted).
func TickAll(s *store.Store, pol *policy.Policy) {
	if pol == nil {
		pol = &policy.Policy{}
	}
	insts, err := s.ListInstances()
	if err != nil {
		return
	}
	for i := range insts {
		if insts[i].Status == models.InstRunning {
			tickInstance(s, pol, &insts[i])
		}
	}
}

// runReal executes a configured script/integration step for real; returns
// real=false when the step isn't configured for real execution (simulate).
func runReal(wfStep *models.WorkflowStep, input map[string]any, pol *policy.Policy) (out string, err error, real bool, dms int) {
	if !(strings.HasPrefix(wfStep.Type, "integration.") || wfStep.Type == "script") {
		return "", nil, false, 0
	}
	start := time.Now()
	out, err, real = executor.Run(wfStep, input, pol)
	return out, err, real, int(time.Since(start).Milliseconds())
}

func tickInstance(s *store.Store, pol *policy.Policy, inst *models.Instance) {
	wf, err := s.GetWorkflow(inst.WorkflowID)
	if err != nil || wf == nil {
		return
	}
	stepByID := map[string]*models.WorkflowStep{}
	for i := range wf.Steps {
		stepByID[wf.Steps[i].ID] = &wf.Steps[i]
	}
	runs := append([]models.StepRun(nil), inst.StepRuns...)

	idx := -1
	for i := range runs {
		if runs[i].Status == models.StepRunning || runs[i].Status == models.StepPending {
			idx = i
			break
		}
	}
	if idx == -1 {
		inst.Status = models.InstCompleted
		inst.CurrentStep = len(runs)
		inst.EndedAt = nowISO()
		inst.StepRuns = runs
		_ = s.UpsertInstance(*inst)
		_ = s.AddAudit(audit("system", "Instance completed", inst.ID+" · "+inst.WorkflowName, "execution"))
		return
	}

	cur := &runs[idx]
	wfStep := stepByID[cur.StepID]

	// pending -> running
	if cur.Status == models.StepPending {
		cur.Status = models.StepRunning
		cur.StartedAt = "now"
		inst.CurrentStep = idx
		inst.StepRuns = runs
		_ = s.UpsertInstance(*inst)
		return
	}

	// running -> maybe resolve
	key := inst.ID + ":" + cur.StepID
	runTicks[key]++
	if runTicks[key] < durationTicks(cur.Type) {
		inst.CurrentStep = idx
		inst.StepRuns = runs
		_ = s.UpsertInstance(*inst)
		return
	}
	delete(runTicks, key)

	// resolve
	switch {
	case wfStep != nil && wfStep.Params["condition"] == "previous_step.sla_breached":
		cur.Status = models.StepSkipped
		cur.Output = "no SLA breach — skipped"
		cur.DurationMs = 5
	case cur.Type == "human.approval":
		if inst.AutoApprove {
			cur.Status = models.StepSucceeded
			cur.DurationMs = 1200
			cur.Output = "auto-approved — condition below threshold"
			inst.AutoApprove = false
		} else {
			approver := approverOf(wfStep)
			cur.Status = models.StepWaiting
			note := "Waiting on " + approver
			if wfStep != nil && wfStep.Params["sla_hours"] != "" {
				note += " · SLA " + wfStep.Params["sla_hours"] + "h"
			}
			cur.Note = note
			inst.Status = models.InstWaiting
			inst.WaitingOn = approver
			inst.CurrentStep = idx
			inst.StepRuns = runs
			_ = s.UpsertInstance(*inst)
			_ = s.AddAudit(audit("system", "Instance waiting", inst.ID+" waiting on "+approver, "execution"))
			return
		}
	case cur.Type == "condition" && wfStep != nil && wfStep.Params["expression"] != "":
		expr := wfStep.Params["expression"]
		cur.Status = models.StepSucceeded
		cur.DurationMs = 18
		if res, ok := evalCondition(expr, inst.Input); ok && !res {
			cur.Output = expr + " → false · auto-approve path"
			inst.AutoApprove = true
		} else {
			cur.Output = expr + " → true"
		}
	default:
		out, rerr, real, dms := runReal(wfStep, inst.Input, pol)
		if real && rerr != nil {
			// Real execution attempted and failed (e.g., blocked by policy, network) -> halt.
			cur.Status = models.StepFailed
			cur.Note = rerr.Error()
			inst.Status = models.InstFailed
			inst.Error = rerr.Error()
			inst.CurrentStep = idx
			inst.StepRuns = runs
			_ = s.UpsertInstance(*inst)
			_ = s.AddAudit(audit("system", "Step failed", inst.ID+" · "+cur.Name+" — "+rerr.Error(), "execution"))
			return
		}
		cur.Status = models.StepSucceeded
		if real {
			cur.DurationMs = dms
			cur.Output = out
		} else {
			if strings.HasPrefix(cur.Type, "ai.") {
				cur.DurationMs = 2100
			} else {
				cur.DurationMs = 120 + idx*60
			}
			cur.Output = stepOutput(cur, wfStep)
		}
	}

	inst.CurrentStep = idx
	inst.StepRuns = runs
	_ = s.UpsertInstance(*inst)
}

// ApproveWaiting resolves the current waiting human task and resumes the instance.
func ApproveWaiting(s *store.Store, id string) (*models.Instance, error) {
	inst, err := s.GetInstance(id)
	if err != nil || inst == nil || inst.Status != models.InstWaiting {
		return inst, err
	}
	if inst.CurrentStep < len(inst.StepRuns) && inst.StepRuns[inst.CurrentStep].Status == models.StepWaiting {
		cur := &inst.StepRuns[inst.CurrentStep]
		cur.Status = models.StepSucceeded
		cur.DurationMs = 420000
		cur.Output = "approved by " + inst.WaitingOn + " (simulated)"
		cur.Note = ""
	}
	inst.Status = models.InstRunning
	inst.WaitingOn = ""
	if err := s.UpsertInstance(*inst); err != nil {
		return nil, err
	}
	_ = s.AddAudit(audit("You", "Human task approved", id+" — execution resumed", "execution"))
	return inst, nil
}

// RetryFailed resumes a failed instance from the failed step.
func RetryFailed(s *store.Store, id string) (*models.Instance, error) {
	inst, err := s.GetInstance(id)
	if err != nil || inst == nil || inst.Status != models.InstFailed {
		return inst, err
	}
	if inst.CurrentStep < len(inst.StepRuns) && inst.StepRuns[inst.CurrentStep].Status == models.StepFailed {
		inst.StepRuns[inst.CurrentStep].Status = models.StepPending
		inst.StepRuns[inst.CurrentStep].Note = ""
	}
	inst.Status = models.InstRunning
	inst.Error = ""
	inst.EndedAt = ""
	if err := s.UpsertInstance(*inst); err != nil {
		return nil, err
	}
	_ = s.AddAudit(audit("You", "Retried from failed step", id+" — resume without re-running completed steps", "execution"))
	return inst, nil
}

// CancelInstance cancels a running/waiting instance.
func CancelInstance(s *store.Store, id string) (*models.Instance, error) {
	inst, err := s.GetInstance(id)
	if err != nil || inst == nil {
		return inst, err
	}
	inst.Status = models.InstCancelled
	inst.EndedAt = nowISO()
	if err := s.UpsertInstance(*inst); err != nil {
		return nil, err
	}
	_ = s.AddAudit(audit("You", "Instance cancelled", id, "execution"))
	return inst, nil
}

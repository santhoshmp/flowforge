// Package engine is the durable execution engine for the Go control plane.
// It mirrors server/src/engine.ts: TickAll advances every running instance by
// one persisted transition; human approvals wait; conditions evaluate run
// input; SLA-escalation steps skip. The scheduler calls TickAll on an interval;
// tests call it directly for determinism.
package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	// The connectors package registers the `connector` step executor with the
	// executor registry on init; importing it here makes connector execution a
	// guaranteed part of the engine, not an accident of route wiring.
	_ "github.com/santhoshmp/flowforge/internal/connectors"
	"github.com/santhoshmp/flowforge/internal/executor"
	"github.com/santhoshmp/flowforge/internal/models"
	"github.com/santhoshmp/flowforge/internal/policy"
	"github.com/santhoshmp/flowforge/internal/store"
	"github.com/santhoshmp/flowforge/internal/util"
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
// Waiting instances are included so their SLA breaches can fire.
func TickAll(s *store.Store, pol *policy.Policy) {
	TickAllAt(s, pol, time.Now())
}

// TickAllAt is TickAll with an explicit clock (deterministic tests).
func TickAllAt(s *store.Store, pol *policy.Policy, now time.Time) {
	if pol == nil {
		pol = &policy.Policy{}
	}
	insts, err := s.ListInstances()
	if err != nil {
		return
	}
	for i := range insts {
		if insts[i].Status == models.InstRunning || insts[i].Status == models.InstWaiting {
			tickInstance(s, pol, &insts[i], now)
		}
	}
}

// runReal executes a configured step for real via the executor registry
// (script sandbox, egress-gated HTTP, connectors); returns real=false when no
// executor handles the type or the step isn't configured (simulate).
func runReal(wfStep *models.WorkflowStep, input map[string]any, pol *policy.Policy) (out string, err error, real bool, dms int) {
	start := time.Now()
	out, err, real = executor.Run(wfStep, input, pol)
	return out, err, real, int(time.Since(start).Milliseconds())
}

func tickInstance(s *store.Store, pol *policy.Policy, inst *models.Instance, now time.Time) {
	// Waiting instances: only SLA breaches can move them (approve/cancel come
	// through their own APIs). A breached approval is skipped and the run
	// continues — the escalation step (condition previous_step.sla_breached)
	// takes over if the workflow has one.
	if inst.Status == models.InstWaiting {
		tickWaiting(s, pol, inst, now)
		return
	}
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

	// Retry backoff gate: a step scheduled for a later attempt waits.
	if cur.NextAttemptAt != "" {
		if t, err := time.Parse(time.RFC3339, cur.NextAttemptAt); err == nil && time.Now().Before(t) {
			inst.CurrentStep = idx
			inst.StepRuns = runs
			_ = s.UpsertInstance(*inst)
			return
		}
		cur.NextAttemptAt = ""
	}

	// pending -> running
	if cur.Status == models.StepPending {
		cur.Status = models.StepRunning
		cur.StartedAt = now.UTC().Format(time.RFC3339)
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
	// An approval gated on a previous SLA breach: skipped when nothing
	// breached; when it DID breach, fall through to the human.approval
	// handling below so the escalation approver still gets the task.
	case wfStep != nil && wfStep.Params["condition"] == "previous_step.sla_breached" && !(idx > 0 && breached(runs[idx-1])):
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
			// Automatic retries (params.retries + optional retry_delay
			// seconds): the step goes back to pending with a backoff gate.
			if maxRetry := stepRetries(wfStep); cur.Attempts < maxRetry {
				cur.Attempts++
				cur.Status = models.StepPending
				cur.Output = ""
				cur.Note = fmt.Sprintf("retry %d/%d after failure: %s", cur.Attempts, maxRetry, rerr.Error())
				if delay := stepRetryDelay(wfStep); delay > 0 {
					cur.NextAttemptAt = time.Now().Add(time.Duration(delay) * time.Second).UTC().Format(time.RFC3339)
				}
				delete(runTicks, key) // restart duration ticks on the retry
				inst.Status = models.InstRunning
				inst.CurrentStep = idx
				inst.StepRuns = runs
				_ = s.UpsertInstance(*inst)
				return
			}
			// Real execution attempted and failed (e.g., blocked by policy, network) -> halt.
			cur.Status = models.StepFailed
			cur.Note = rerr.Error()
			inst.Status = models.InstFailed
			inst.Error = rerr.Error()
			if cur.Attempts > 0 {
				inst.Error = fmt.Sprintf("%s (after %d retries)", rerr.Error(), cur.Attempts)
			}
			inst.CurrentStep = idx
			inst.StepRuns = runs
			_ = s.UpsertInstance(*inst)
			_ = s.AddAudit(audit("system", "Step failed", inst.ID+" · "+cur.Name+" — "+rerr.Error(), "execution"))
			executor.SendFailureAlert(inst.WorkflowName, inst.ID, cur.Name, rerr.Error(), pol)
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

// tickWaiting checks a waiting instance for SLA breaches. Non-breached
// waits are left untouched (their transition comes from approve/cancel).
func tickWaiting(s *store.Store, _ *policy.Policy, inst *models.Instance, now time.Time) {
	for i := range inst.StepRuns {
		r := &inst.StepRuns[i]
		if r.Status != models.StepWaiting {
			continue
		}
		wf, err := s.GetWorkflow(inst.WorkflowID)
		if err != nil || wf == nil {
			return
		}
		var step *models.WorkflowStep
		for j := range wf.Steps {
			if wf.Steps[j].ID == r.StepID {
				step = &wf.Steps[j]
			}
		}
		if step == nil {
			return
		}
		hours := stepSLAHours(step)
		if hours <= 0 {
			return // no SLA on this gate
		}
		started, err := time.Parse(time.RFC3339, r.StartedAt)
		if err != nil {
			return
		}
		if now.Sub(started) < time.Duration(hours*float64(time.Hour)) {
			return // still inside the SLA window
		}
		// Breached: skip the approval, let the run continue (escalation step
		// with condition previous_step.sla_breached will now fire).
		waitingOn := inst.WaitingOn
		r.Status = models.StepSkipped
		r.Note = fmt.Sprintf("SLA breached after %gh — escalated", hours)
		r.Output = ""
		inst.Status = models.InstRunning
		inst.WaitingOn = ""
		inst.CurrentStep = i
		_ = s.UpsertInstance(*inst)
		_ = s.AddAudit(audit("system", "SLA breached", inst.ID+" · "+r.Name+" — waiting on "+waitingOn+" timed out", "execution"))
		return
	}
}

// breached reports whether a step run was skipped due to an SLA breach.
func breached(r models.StepRun) bool {
	return r.Status == models.StepSkipped && strings.Contains(r.Note, "SLA breached")
}

// stepSLAHours parses params.sla_hours as (possibly fractional) hours.
// Fractional values enable fast demo cycles ("0.02" ≈ 72 seconds); 0,
// missing, or invalid means no SLA.
func stepSLAHours(step *models.WorkflowStep) float64 {
	if step == nil {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(step.Params["sla_hours"]), 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

// stepRetries parses params.retries (max automatic retries; 0 = fail fast).
func stepRetries(step *models.WorkflowStep) int {
	if step == nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(step.Params["retries"]))
	if err != nil || n < 0 {
		return 0
	}
	if n > 10 {
		return 10 // sanity bound
	}
	return n
}

// stepRetryDelay parses params.retry_delay seconds (0 = retry next tick).
func stepRetryDelay(step *models.WorkflowStep) int {
	if step == nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(step.Params["retry_delay"]))
	if err != nil || n < 0 {
		return 0
	}
	if n > 3600 {
		return 3600 // sanity bound
	}
	return n
}

// Package demopack loads the Meridian Components demo organization into a
// FlowForge store: master data with governance stories, six departmental
// workflows (deployed), and two weeks of realistic run history so the
// dashboards, human-task queue, and audit trail are demo-ready.
//
// Load is idempotent (fixed record IDs), so `flowforge demo` is safe to run
// repeatedly. See demo-pack/ at the repo root and docs/demo-runbook.md.
package demopack

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/santhoshmp/flowforge/internal/models"
	"github.com/santhoshmp/flowforge/internal/spec"
	"github.com/santhoshmp/flowforge/internal/store"
)

//go:embed workflows
var workflowsFS embed.FS

// Summary describes what a Load applied.
type Summary struct {
	Org        string
	Workflows  int
	Runs       int
	MDMRecords int
	Audit      int
}

// approvedBy maps workflow name -> the named approver recorded on deploy.
var approvedBy = map[string]string{
	"vendor-invoice-approval":   "Priya Raman (CFO)",
	"purchase-order-approval":   "Priya Raman (CFO)",
	"employee-onboarding":       "Sofia Lindqvist (HR Business Partner)",
	"support-ticket-routing":    "Daniel Osei (Support Lead)",
	"supplier-master-change":    "Priya Raman (CFO)",
	"customer-contract-renewal": "Priya Raman (CFO)",
}

// Load seeds the demo organization into the store. `now` anchors the 14-day
// run history (tests pass a fixed time; the CLI passes time.Now()).
func Load(s *store.Store, now time.Time) (Summary, error) {
	sum := Summary{Org: Org}

	// 1. Master data (full replace per entity — record order is the story).
	for _, e := range MDM() {
		if err := s.UpsertMDM(e); err != nil {
			return sum, err
		}
		sum.MDMRecords += len(e.Records)
	}

	// 2. Workflows from the embedded flowforge/v1 artifacts.
	entries, err := workflowsFS.ReadDir("workflows")
	if err != nil {
		return sum, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	type loaded struct {
		wf   models.Workflow
		spec *spec.WorkflowSpec
	}
	var wfs []loaded
	for _, name := range names {
		raw, err := workflowsFS.ReadFile("workflows/" + name)
		if err != nil {
			return sum, err
		}
		sp, err := spec.ParseYAML(string(raw))
		if err != nil {
			return sum, fmt.Errorf("demo pack artifact %s: %v", name, err)
		}
		wf := toWorkflow(sp, approvedBy[sp.Metadata.Name], now)
		if err := s.UpsertWorkflow(wf); err != nil {
			return sum, err
		}
		sum.Workflows++
		wfs = append(wfs, loaded{wf: wf, spec: sp})
	}

	// 3. Run history + audit narrative. Existing audit rows (re-loads) are
	// skipped by id so Load stays idempotent — audit is append-only.
	existingAudit := map[string]bool{}
	if prior, err := s.ListAudit(); err == nil {
		for _, a := range prior {
			existingAudit[a.ID] = true
		}
	}
	auditN := 0
	for _, w := range wfs {
		for _, r := range historyFor(w.wf) {
			inst := buildInstance(w.wf, w.spec, r, now)
			if err := s.UpsertInstance(inst); err != nil {
				return sum, err
			}
			sum.Runs++
			for _, a := range runAudit(w.wf, inst, &auditN) {
				if existingAudit[a.ID] {
					continue
				}
				if err := s.AddAudit(a); err != nil {
					return sum, err
				}
				existingAudit[a.ID] = true
				sum.Audit++
			}
		}
	}
	return sum, nil
}

// toWorkflow converts a validated artifact into a deployed workflow.
func toWorkflow(sp *spec.WorkflowSpec, approver string, now time.Time) models.Workflow {
	wf := models.Workflow{
		ID:          "wf-demo-" + sp.Metadata.Name,
		Name:        displayName(sp.Metadata.Name),
		Description: sp.Spec.Description,
		Prompt:      "imported from the Meridian demo pack (" + sp.Metadata.Name + ".flow.yaml)",
		Status:      models.StatusDeployed,
		Version:     sp.Metadata.Version,
		ApprovedBy:  approver,
		CreatedBy:   "Meridian demo pack",
		AIModel:     sp.Metadata.AuthoredWith,
		CreatedAt:   now.AddDate(0, 0, -21).UTC().Format(time.RFC3339),
	}
	wf.Steps = append(wf.Steps, models.WorkflowStep{
		ID: "trigger", Type: "trigger", Name: "Trigger",
		Params: map[string]string{"event": sp.Spec.Trigger.Event},
	})
	for _, st := range sp.Spec.Steps {
		params := map[string]string{}
		for k, v := range st.Params {
			params[k] = v
		}
		wf.Steps = append(wf.Steps, models.WorkflowStep{
			ID: st.ID, Type: st.Type, Name: st.Name, Params: params,
			Confidence: 92, Assumptions: []string{},
		})
	}
	return wf
}

func displayName(kebab string) string {
	out := ""
	upper := true
	for _, c := range kebab {
		if c == '-' || c == '_' {
			out += " "
			upper = true
			continue
		}
		if upper && c >= 'a' && c <= 'z' {
			out += string(rune(c - 32))
			upper = false
		} else {
			out += string(c)
			upper = false
		}
	}
	return out
}

// historyRun describes one synthetic execution.
type historyRun struct {
	daysAgo   int
	status    string
	entity    string
	input     map[string]any
	waitingOn string // for waiting runs
	err       string // for failed runs
}

// historyFor returns a believable two-week story per workflow: mostly
// completed, one live human task, one failure with a retried follow-up.
func historyFor(wf models.Workflow) []historyRun {
	switch wf.ID {
	case "wf-demo-vendor-invoice-approval":
		return []historyRun{
			{daysAgo: 12, status: models.InstCompleted, entity: "INV-2026-0811 · Shenzhen Brightway Electronics (V-1001)", input: map[string]any{"total": float64(18400), "vendor_id": "V-1001"}},
			{daysAgo: 11, status: models.InstCompleted, entity: "INV-2026-0819 · Nordic Copper Works AB (V-1003)", input: map[string]any{"total": float64(4210), "vendor_id": "V-1003"}},
			{daysAgo: 9, status: models.InstFailed, entity: "INV-2026-0834 · Veloce Logistics GmbH (V-1006)", input: map[string]any{"total": float64(9900), "vendor_id": "V-1006"}, err: "blocked by egress policy: erp.meridian.internal/invoices"},
			{daysAgo: 9, status: models.InstCompleted, entity: "INV-2026-0834 · Veloce Logistics GmbH (V-1006) — retried", input: map[string]any{"total": float64(9900), "vendor_id": "V-1006"}},
			{daysAgo: 7, status: models.InstCompleted, entity: "INV-2026-0851 · TechSupply Global Pte Ltd (V-1002)", input: map[string]any{"total": float64(23750), "vendor_id": "V-1002"}},
			{daysAgo: 5, status: models.InstCompleted, entity: "INV-2026-0867 · Penang Polymers Sdn Bhd (V-1004)", input: map[string]any{"total": float64(7300), "vendor_id": "V-1004"}},
			{daysAgo: 2, status: models.InstCancelled, entity: "INV-2026-0880 · duplicate submission", input: map[string]any{"total": float64(6100)}},
			{daysAgo: 1, status: models.InstWaiting, entity: "INV-2026-0887 · Shenzhen Brightway Electronics (V-1001)", input: map[string]any{"total": float64(24500), "vendor_id": "V-1001"}, waitingOn: "Aisha Khan (Controller)"},
		}
	case "wf-demo-purchase-order-approval":
		return []historyRun{
			{daysAgo: 13, status: models.InstCompleted, entity: "PO-4471 · cable harnesses", input: map[string]any{"total": float64(6900)}},
			{daysAgo: 8, status: models.InstCompleted, entity: "PO-4489 · copper spreaders", input: map[string]any{"total": float64(15400)}},
			{daysAgo: 4, status: models.InstCompleted, entity: "PO-4502 · polymer housings", input: map[string]any{"total": float64(3200)}},
			{daysAgo: 1, status: models.InstWaiting, entity: "PO-4515 · MCU boards (rush)", input: map[string]any{"total": float64(11200)}, waitingOn: "Marcus Webb (Procurement Director)"},
		}
	case "wf-demo-employee-onboarding":
		return []historyRun{
			{daysAgo: 10, status: models.InstCompleted, entity: "E-4412 · Mei Lin Chong (Support Engineer)", input: map[string]any{"emp_id": "E-4412"}},
			{daysAgo: 3, status: models.InstCompleted, entity: "E-4415 · Arjun Nair (Quality Analyst)", input: map[string]any{"emp_id": "E-4415"}},
			{daysAgo: 0, status: models.InstWaiting, entity: "E-4417 · Jonas de Vries (Operations Planner)", input: map[string]any{"emp_id": "E-4417"}, waitingOn: "Sofia Lindqvist (HR Business Partner)"},
		}
	case "wf-demo-support-ticket-routing":
		return []historyRun{
			{daysAgo: 6, status: models.InstCompleted, entity: "TCK-9021 · latency spike, Halden Marine", input: map[string]any{"priority": "critical"}},
			{daysAgo: 6, status: models.InstCompleted, entity: "TCK-9024 · password resets, internal", input: map[string]any{"priority": "normal"}},
			{daysAgo: 2, status: models.InstCompleted, entity: "TCK-9061 · firmware query, Vestergaard", input: map[string]any{"priority": "normal"}},
			{daysAgo: 0, status: models.InstWaiting, entity: "TCK-9068 · line-down alarm, Nordwind line 3", input: map[string]any{"priority": "critical"}, waitingOn: "Daniel Osei (Support Lead)"},
		}
	case "wf-demo-supplier-master-change":
		return []historyRun{
			{daysAgo: 5, status: models.InstCompleted, entity: "V-1004 · bank account change", input: map[string]any{"vendor_id": "V-1004"}},
			{daysAgo: 1, status: models.InstWaiting, entity: "V-1012 · suspected duplicate of V-1001", input: map[string]any{"vendor_id": "V-1012"}, waitingOn: "Tomas Herrera (Data Steward)"},
		}
	case "wf-demo-customer-contract-renewal":
		return []historyRun{
			{daysAgo: 7, status: models.InstCompleted, entity: "C-2001 · Vestergaard Medical, 3yr", input: map[string]any{"renewal_value": float64(74000)}},
			{daysAgo: 2, status: models.InstCompleted, entity: "C-2003 · Halden Marine, 1yr", input: map[string]any{"renewal_value": float64(18500)}},
			{daysAgo: 0, status: models.InstWaiting, entity: "C-2002 · Nordwind Automotiv, 2yr", input: map[string]any{"renewal_value": float64(126000)}, waitingOn: "Priya Raman (CFO)"},
		}
	}
	return nil
}

// decisiveIndex is the step index a run stopped at: the matching human task
// for waiting runs, the first integration step for failed runs, the first
// approval for cancelled runs, and past-the-end for completed runs.
func decisiveIndex(wf models.Workflow, r historyRun) int {
	switch r.status {
	case models.InstWaiting:
		for i, st := range wf.Steps {
			if st.Type == "human.approval" && st.Params["approver"] == r.waitingOn && st.Params["condition"] == "" {
				return i
			}
		}
	case models.InstFailed:
		for i, st := range wf.Steps {
			if strings.HasPrefix(st.Type, "integration") {
				return i
			}
		}
	case models.InstCancelled:
		for i, st := range wf.Steps {
			if st.Type == "human.approval" {
				return i
			}
		}
	}
	return len(wf.Steps)
}

// buildInstance materializes one history run into a store-ready instance
// with step-level state consistent with the final status. Starts are pinned
// to midday UTC of their day so they always land inside the metrics
// 14-day window regardless of when the pack loads.
func buildInstance(wf models.Workflow, sp *spec.WorkflowSpec, r historyRun, now time.Time) models.Instance {
	day := now.UTC().AddDate(0, 0, -r.daysAgo)
	start := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.UTC)
	var end time.Time
	switch r.status {
	case models.InstCompleted:
		end = start.Add(38 * time.Minute)
	case models.InstFailed:
		end = start.Add(12 * time.Minute)
	case models.InstCancelled:
		end = start.Add(3 * time.Minute)
	}

	inst := models.Instance{
		ID:           "run-demo-" + fmt.Sprintf("%s-%02d-%04x", wf.ID[len("wf-demo-"):], r.daysAgo, fnv16(r.entity)),
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		Status:       r.status,
		Entity:       r.entity,
		StartedAt:    start.Format(time.RFC3339),
		Input:        r.input,
	}
	if !end.IsZero() {
		inst.EndedAt = end.Format(time.RFC3339)
	}
	inst.WaitingOn = r.waitingOn
	inst.Error = r.err

	decisive := decisiveIndex(wf, r)
	for i, st := range wf.Steps {
		run := models.StepRun{StepID: st.ID, Name: st.Name, Type: st.Type, Status: models.StepPending}
		isEscalation := st.Type == "human.approval" && st.Params["condition"] != ""
		switch {
		case i == 0:
			run.Status = models.StepSucceeded
			run.Output = "event received: " + sp.Spec.Trigger.Event
			run.DurationMs = 80 + i*37
		case i < decisive:
			if isEscalation && r.status == models.InstCompleted {
				run.Status = models.StepSkipped
			} else {
				run.Status = models.StepSucceeded
				run.DurationMs = 120 + i*53
				run.Output = stepOutput(st)
			}
		case i == decisive:
			switch r.status {
			case models.InstWaiting:
				run.Status = models.StepWaiting
				run.Note = "Waiting on " + r.waitingOn + " · SLA " + st.Params["sla_hours"] + "h"
				inst.CurrentStep = i
			case models.InstFailed:
				run.Status = models.StepFailed
				run.Output = r.err
				inst.CurrentStep = i
			case models.InstCancelled:
				run.Status = models.StepSkipped
			}
		}
		inst.StepRuns = append(inst.StepRuns, run)
	}
	return inst
}

// fnv16 derives a short stable id fragment from s (deterministic history ids).
func fnv16(s string) uint16 {
	var h uint16 = 0x811c
	for i := 0; i < len(s); i++ {
		h ^= uint16(s[i])
		h *= 0x9d1b
	}
	return h & 0xffff
}

func stepOutput(st models.WorkflowStep) string {
	switch st.Type {
	case "ai.extract":
		return "extracted 4 fields (confidence 0.93)"
	case "ai.classify":
		return "priority=normal (confidence 0.88)"
	case "mdm.validate":
		return "golden record matched"
	case "condition":
		return "branch: true"
	case "human.approval":
		return "approved by " + st.Params["approver"]
	case "notify":
		return "notification sent (" + st.Params["channel"] + ")"
	case "integration.post":
		return "POST " + st.Params["endpoint"] + " -> 201 created"
	case "wait":
		return "SLA window elapsed"
	}
	return "done"
}

// runAudit writes the audit narrative for a run.
func runAudit(wf models.Workflow, inst models.Instance, n *int) []models.AuditEntry {
	mk := func(at time.Time, actor, action, detail, kind string) models.AuditEntry {
		*n++
		return models.AuditEntry{
			ID: fmt.Sprintf("aud-demo-%s-%03d", wf.ID[len("wf-demo-"):], *n), At: at.Format(time.RFC3339),
			Actor: actor, Action: action, Detail: detail, Kind: kind,
		}
	}
	start, _ := time.Parse(time.RFC3339, inst.StartedAt)
	approver := firstApprover(wf)
	out := []models.AuditEntry{
		mk(start, "system", "Execution started", inst.WorkflowName+" · "+inst.Entity, "execution"),
	}
	switch inst.Status {
	case models.InstCompleted:
		out = append(out, mk(start.Add(20*time.Minute), approver, "Human task approved", inst.WorkflowName+" · "+inst.Entity, "approval"))
		out = append(out, mk(start.Add(38*time.Minute), "system", "Execution completed", inst.Entity+" — all steps succeeded", "execution"))
	case models.InstWaiting:
		out = append(out, mk(start.Add(2*time.Minute), inst.WaitingOn, "Task assigned", "waiting for approval — "+inst.Entity, "approval"))
	case models.InstFailed:
		out = append(out, mk(start.Add(12*time.Minute), "system", "Execution failed", inst.Error, "execution"))
	case models.InstCancelled:
		out = append(out, mk(start.Add(3*time.Minute), firstApprover(wf), "Execution cancelled", inst.Entity, "execution"))
	}
	return out
}

func firstApprover(wf models.Workflow) string {
	for _, st := range wf.Steps {
		if st.Type == "human.approval" && st.Params["condition"] == "" {
			return st.Params["approver"]
		}
	}
	return "Meridian operator"
}

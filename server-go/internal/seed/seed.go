// Package seed produces a deterministic, relationship-correct demo dataset.
// Every instance's step runs match its parent workflow's steps; entities are
// drawn from the MDM; audit entries reference real instances/workflows; and
// each workflow has balanced coverage (completed / failed / waiting / cancelled).
package seed

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/flowforge/flowforge/internal/models"
)

// Fixed seed for reproducible data across restarts and test runs.
var rng = rand.New(rand.NewSource(42))

func uid() string {
	const hex = "0123456789abcdef"
	b := make([]byte, 4)
	for i := range b {
		b[i] = hex[rng.Intn(16)]
	}
	return string(b)
}

func ri(min, max int) int { return min + rng.Intn(max-min+1) }
func pick(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[rng.Intn(len(s))]
}
func isoAgo(days, hours int) string {
	return time.Now().UTC().Add(-time.Duration(days)*24*time.Hour - time.Duration(hours)*time.Hour).Format(time.RFC3339)
}

// durFor returns a realistic per-step duration in ms.
func durFor(t string) int {
	switch t {
	case "trigger":
		return ri(80, 180)
	case "ai.extract", "ai.classify":
		return ri(1800, 2700)
	case "mdm.validate", "mdm.lookup":
		return ri(120, 280)
	case "condition":
		return ri(8, 40)
	case "human.approval":
		return ri(180000, 900000)
	case "notify":
		return ri(180, 650)
	case "integration.post", "integration.http":
		return ri(400, 2300)
	case "wait":
		return ri(1000, 5000)
	default:
		return ri(100, 400)
	}
}

func strOr(m map[string]string, k, d string) string {
	if v, ok := m[k]; ok && v != "" {
		return v
	}
	return d
}

func step(id, typ, name string, params map[string]string, conf int, assumptions ...string) models.WorkflowStep {
	if params == nil {
		params = map[string]string{}
	}
	if assumptions == nil {
		assumptions = []string{}
	}
	return models.WorkflowStep{ID: id, Type: typ, Name: name, Params: params, Confidence: conf, Assumptions: assumptions}
}

// ---- Workflow definitions (the 6 seeded workflows) --------------------------

func Workflows() []models.Workflow {
	invoice := []models.WorkflowStep{
		step("invoice_received", "trigger", "Invoice received", map[string]string{"event": "vendor_invoice.created", "source": "any"}, 95),
		step("extract_line_items", "ai.extract", "Extract line items", map[string]string{"fields": "line_items, vendor, total, currency", "model": "auto"}, 88, "Field list inferred from prompt."),
		step("validate_vendor", "mdm.validate", "Validate vendor against master", map[string]string{"entity": "vendors", "match_on": "vendor_id, tax_id", "on_mismatch": "route_to_steward"}, 91, "Assumed master data entity vendors."),
		step("amount_check", "condition", "Amount over 10000?", map[string]string{"expression": "total > 10000", "on_false": "auto_approve"}, 93),
		step("manager_approval", "human.approval", "Cost-Center Manager approval", map[string]string{"approver": "Cost-Center Manager", "resolve_via": "hr_hierarchy", "sla_hours": "48"}, 82, "Approver resolves via HR hierarchy."),
		step("vp_escalation", "human.approval", "Escalation to Finance VP", map[string]string{"approver": "Finance VP", "after_hours": "48", "condition": "previous_step.sla_breached"}, 78, "Escalation triggers on SLA breach."),
		step("post_to_erp", "integration.post", "Post to ERP", map[string]string{"system": "ERP", "endpoint": "erp.inbound.invoices", "mapping": "auto"}, 84, "Assumed ERP exposes standard inbound API."),
	}
	onboard := []models.WorkflowStep{
		step("req_submitted", "trigger", "Onboarding request submitted", map[string]string{"event": "onboarding.submitted"}, 94),
		step("validate_emp", "mdm.validate", "Validate employee", map[string]string{"entity": "employees", "match_on": "emp_id, email"}, 90, "Assumed master data entity employees."),
		step("mgr_approval", "human.approval", "Hiring Manager approval", map[string]string{"approver": "Hiring Manager", "sla_hours": "24"}, 85, "Approver resolves via HR hierarchy."),
		step("notify_it", "notify", "Notify IT", map[string]string{"channel": "slack", "recipients": "#it-provisioning"}, 88),
		step("post_workday", "integration.post", "Post to Workday", map[string]string{"system": "Workday", "endpoint": "workday.hires"}, 83, "Assumed Workday exposes standard API."),
	}
	po := []models.WorkflowStep{
		step("po_received", "trigger", "Purchase order received", map[string]string{"event": "purchase_order.created"}, 95),
		step("validate_vendor", "mdm.validate", "Validate vendor", map[string]string{"entity": "vendors", "match_on": "vendor_id"}, 91, "Assumed master data entity vendors."),
		step("amount_check", "condition", "Amount over 5000?", map[string]string{"expression": "total > 5000", "on_false": "auto_approve"}, 92),
		step("mgr_approval", "human.approval", "Procurement Manager approval", map[string]string{"approver": "Procurement Manager", "sla_hours": "24"}, 84, "Approver resolves via HR hierarchy."),
		step("notify_requester", "notify", "Notify requester", map[string]string{"channel": "email"}, 87),
		step("post_sap", "integration.post", "Post to SAP", map[string]string{"system": "SAP", "endpoint": "sap.inbound.orders"}, 83, "Assumed SAP exposes standard API."),
	}
	expense := []models.WorkflowStep{
		step("exp_received", "trigger", "Expense report submitted", map[string]string{"event": "expense.submitted"}, 95),
		step("extract_receipt", "ai.extract", "Extract receipt data", map[string]string{"fields": "amount, category, date, vendor", "model": "auto"}, 87, "Field list inferred."),
		step("amount_check", "condition", "Amount over 500?", map[string]string{"expression": "total > 500", "on_false": "auto_approve"}, 92),
		step("mgr_approval", "human.approval", "Manager approval", map[string]string{"approver": "Manager", "sla_hours": "48"}, 85, "Approver resolves via HR hierarchy."),
		step("post_finance", "integration.post", "Post to Finance", map[string]string{"system": "Finance", "endpoint": "finance.inbound.expenses"}, 84, "Assumed Finance exposes standard API."),
	}
	ticket := []models.WorkflowStep{
		step("ticket_created", "trigger", "Ticket created", map[string]string{"event": "support_ticket.created"}, 95),
		step("classify", "ai.classify", "Classify priority", map[string]string{"model": "auto"}, 86, "Model inferred."),
		step("priority_check", "condition", "Priority critical?", map[string]string{"expression": "priority == critical", "on_false": "standard_path"}, 80),
		step("lead_approval", "human.approval", "Support Lead approval", map[string]string{"approver": "Support Lead", "sla_hours": "4"}, 83, "Approver resolves via HR hierarchy."),
		step("notify_oncall", "notify", "Notify on-call", map[string]string{"channel": "slack", "recipients": "#on-call"}, 88),
		step("sla_wait", "wait", "Escalation SLA", map[string]string{"hours": "4"}, 78, "SLA window."),
	}
	leave := []models.WorkflowStep{
		step("leave_submitted", "trigger", "Leave request submitted", map[string]string{"event": "leave.submitted"}, 95),
		step("validate_emp", "mdm.validate", "Validate employee", map[string]string{"entity": "employees", "match_on": "emp_id"}, 90, "Assumed master data entity employees."),
		step("balance_check", "condition", "Balance available?", map[string]string{"expression": "balance > 0", "on_false": "deny"}, 88),
		step("mgr_approval", "human.approval", "Manager approval", map[string]string{"approver": "Manager", "sla_hours": "24"}, 85, "Approver resolves via HR hierarchy."),
		step("post_hris", "integration.post", "Post to HRIS", map[string]string{"system": "HRIS", "endpoint": "hris.inbound.leave"}, 84, "Assumed HRIS exposes standard API."),
	}
	return []models.Workflow{
		{ID: "wf-invoice", Name: "Vendor Invoice Approval", Description: "Extract, validate, approve and post vendor invoices over $10K.", Prompt: "When a vendor invoice over $10K arrives, extract line items, validate against the vendor master, route to the cost-center manager for approval, escalate to Finance VP after 48 hours, then post to the ERP.", Status: models.StatusDeployed, Version: 3, Steps: invoice, CreatedBy: "Priya N", ApprovedBy: "Ravi S", AIModel: "flowforge-author · gpt-4o-mini", CreatedAt: isoAgo(14, 0)},
		{ID: "wf-onboard", Name: "Employee Onboarding", Description: "Validate and provision new hires after manager approval.", Prompt: "When an onboarding request is submitted, validate against HR master, route to the hiring manager for approval, notify IT, then post to Workday.", Status: models.StatusDeployed, Version: 1, Steps: onboard, CreatedBy: "Dev K", ApprovedBy: "Ravi S", AIModel: "flowforge-author · gpt-4o-mini", CreatedAt: isoAgo(9, 0)},
		{ID: "wf-po", Name: "Purchase Order Approval", Description: "Validate vendor and approve purchase orders over $5K, then post to SAP.", Prompt: "When a purchase order over $5K is submitted, validate the vendor, route to the procurement manager for approval, then post to SAP.", Status: models.StatusDeployed, Version: 2, Steps: po, CreatedBy: "Aisha M", ApprovedBy: "Ravi S", AIModel: "flowforge-author · gpt-4o-mini", CreatedAt: isoAgo(7, 0)},
		{ID: "wf-expense", Name: "Expense Report Approval", Description: "Extract receipt data and approve expenses over $500.", Prompt: "When an expense report is submitted, extract receipt data, if over $500 route to manager for approval, then post to Finance.", Status: models.StatusDeployed, Version: 1, Steps: expense, CreatedBy: "Priya N", ApprovedBy: "Aisha M", AIModel: "flowforge-author · gpt-4o-mini", CreatedAt: isoAgo(5, 0)},
		{ID: "wf-ticket", Name: "Support Ticket Routing", Description: "Classify priority, route critical tickets to the support lead, escalate after 4h.", Prompt: "When a ticket is created, classify it, if critical notify on-call, route to the support lead for approval, escalate after 4 hours.", Status: models.StatusDeployed, Version: 1, Steps: ticket, CreatedBy: "Carlos R", ApprovedBy: "Dev K", AIModel: "flowforge-author · gpt-4o-mini", CreatedAt: isoAgo(3, 0)},
		{ID: "wf-leave", Name: "Leave Request Approval", Description: "Validate leave balance and route to manager for approval.", Prompt: "When a leave request is submitted, validate the employee balance, route to the manager for approval, then post to HRIS.", Status: models.StatusApproved, Version: 1, Steps: leave, CreatedBy: "Mei L", AIModel: "flowforge-author · gpt-4o-mini", CreatedAt: isoAgo(1, 0)},
	}
}

// ---- MDM (referenced by mdm.validate steps) ---------------------------------

func MDM() []models.MDMEntity {
	rec := func(id string, kv map[string]string) map[string]string {
		m := map[string]string{"id": id, "status": "golden"}
		for k, v := range kv {
			m[k] = v
		}
		return m
	}
	return []models.MDMEntity{
		{Key: "vendors", Label: "Vendors", Icon: "Building2", Fields: []string{"vendor_id", "name", "tax_id", "country", "status"}, Records: []map[string]string{
			rec("V-10293", map[string]string{"vendor_id": "V-10293", "name": "Acme Corp", "tax_id": "US-84-2210", "country": "US"}),
			rec("V-10877", map[string]string{"vendor_id": "V-10877", "name": "Globex Ltd", "tax_id": "UK-99-3311", "country": "UK"}),
			rec("V-11240", map[string]string{"vendor_id": "V-11240", "name": "Initech", "tax_id": "US-71-8842", "country": "US"}),
			{"id": "V-11301", "vendor_id": "V-11301", "name": "Umbrella Supplies", "tax_id": "DE-45-0091", "country": "DE", "status": "pending stewardship"},
		}},
		{Key: "customers", Label: "Customers", Icon: "Users", Fields: []string{"cust_id", "name", "segment", "country", "status"}, Records: []map[string]string{
			rec("C-3301", map[string]string{"cust_id": "C-3301", "name": "Stark Industries", "segment": "enterprise", "country": "US"}),
			rec("C-3302", map[string]string{"cust_id": "C-3302", "name": "Wayne Enterprises", "segment": "enterprise", "country": "US"}),
			rec("C-3307", map[string]string{"cust_id": "C-3307", "name": "Pied Piper", "segment": "smb", "country": "US"}),
		}},
		{Key: "products", Label: "Products", Icon: "Package", Fields: []string{"sku", "name", "category", "uom", "status"}, Records: []map[string]string{
			rec("SKU-100", map[string]string{"sku": "SKU-100", "name": "Cloud Credits 1K", "category": "software", "uom": "unit"}),
			rec("SKU-214", map[string]string{"sku": "SKU-214", "name": "Support Plan Gold", "category": "services", "uom": "year"}),
		}},
		{Key: "employees", Label: "Employees", Icon: "IdCard", Fields: []string{"emp_id", "name", "role", "manager", "status"}, Records: []map[string]string{
			rec("E-101", map[string]string{"emp_id": "E-101", "name": "Dana W", "role": "Cost-Center Manager", "manager": "Ravi S"}),
			rec("E-102", map[string]string{"emp_id": "E-102", "name": "Ravi S", "role": "Finance VP", "manager": "—"}),
			rec("E-114", map[string]string{"emp_id": "E-114", "name": "Priya N", "role": "Business Analyst", "manager": "Dev K"}),
			rec("E-118", map[string]string{"emp_id": "E-118", "name": "Aisha M", "role": "Procurement Manager", "manager": "Ravi S"}),
			rec("E-121", map[string]string{"emp_id": "E-121", "name": "Carlos R", "role": "Support Lead", "manager": "Dev K"}),
		}},
	}
}

// vendorPool / employeePool drawn from MDM for realistic entities.
var vendorPool = []struct{ id, name string }{
	{"V-10293", "Acme Corp"}, {"V-10877", "Globex Ltd"}, {"V-11240", "Initech"},
}
var employeePool = []struct{ id, name string }{
	{"E-101", "Dana W"}, {"E-114", "Priya N"}, {"E-118", "Aisha M"}, {"E-121", "Carlos R"},
}

func entityFor(wfID string) string {
	n := ri(1000, 9999)
	switch wfID {
	case "wf-invoice":
		v := vendorPool[rng.Intn(len(vendorPool))]
		return fmt.Sprintf("INV-%d · %s (%s)", n, v.name, v.id)
	case "wf-po":
		v := vendorPool[rng.Intn(len(vendorPool))]
		return fmt.Sprintf("PO-%d · %s (%s)", n, v.name, v.id)
	case "wf-expense":
		e := employeePool[rng.Intn(len(employeePool))]
		return fmt.Sprintf("EXP-%d · %s (%s)", n, e.name, e.id)
	case "wf-ticket":
		return fmt.Sprintf("TKT-%d · %s", n, pick([]string{"Login issue", "Billing error", "Bug report", "Feature request"}))
	case "wf-leave":
		e := employeePool[rng.Intn(len(employeePool))]
		return fmt.Sprintf("LV-%d · %s (%s)", n, e.name, e.id)
	case "wf-onboard":
		e := employeePool[rng.Intn(len(employeePool))]
		return fmt.Sprintf("ONB-%d · %s (%s)", n, e.name, e.id)
	default:
		return fmt.Sprintf("REC-%d", n)
	}
}

// stepOutput builds a realistic output string for a succeeded step.
func stepOutput(s models.WorkflowStep, entity string) string {
	switch s.Type {
	case "trigger":
		return entity + " received"
	case "ai.extract":
		return fmt.Sprintf("extracted %s · total $%d.%02d", strOr(s.Params, "fields", "key fields"), ri(2, 48), ri(10, 99))
	case "ai.classify":
		return "classified: " + pick([]string{"standard", "high", "critical"})
	case "mdm.validate":
		return "matched " + strOr(s.Params, "entity", "entity") + " · golden record"
	case "mdm.lookup":
		return "entity resolved"
	case "condition":
		return strOr(s.Params, "expression", "condition") + " → true"
	case "human.approval":
		return "approved by " + strOr(s.Params, "approver", "approver")
	case "notify":
		return "sent via " + strOr(s.Params, "channel", "email")
	case "integration.post":
		return "posted to " + strOr(s.Params, "system", "target") + " · 200 OK"
	case "integration.http":
		return "HTTP 200"
	case "wait":
		return "SLA window elapsed"
	default:
		return "done"
	}
}

func indexOfEscalation(steps []models.WorkflowStep) int {
	for i, s := range steps {
		if s.Params["condition"] == "previous_step.sla_breached" {
			return i
		}
	}
	return -1
}

// synthRuns builds step runs for a given terminal/waiting status, derived
// directly from the workflow's steps (same IDs, types, names, order).
func synthRuns(steps []models.WorkflowStep, status, entity string) ([]models.StepRun, int, string, string) {
	out := make([]models.StepRun, len(steps))
	escIdx := indexOfEscalation(steps)
	firstApproval := -1
	for i, s := range steps {
		if s.Type == "human.approval" {
			firstApproval = i
			break
		}
	}
	for i, s := range steps {
		out[i] = models.StepRun{StepID: s.ID, Name: s.Name, Type: s.Type, Status: models.StepPending}
	}
	var waitingOn, errMsg string
	current := 0

	switch status {
	case models.InstCompleted:
		for i, s := range steps {
			if i == escIdx {
				out[i].Status = models.StepSkipped
				out[i].Output = "no SLA breach — skipped"
				out[i].DurationMs = 5
				continue
			}
			out[i].Status = models.StepSucceeded
			out[i].DurationMs = durFor(s.Type)
			out[i].Output = stepOutput(s, entity)
		}
		current = len(steps)
	case models.InstFailed:
		failPool := []int{}
		for i, s := range steps {
			switch s.Type {
			case "integration.post", "integration.http", "ai.extract", "ai.classify", "mdm.validate":
				failPool = append(failPool, i)
			}
		}
		failIdx := len(steps) - 1
		if len(failPool) > 0 {
			failIdx = failPool[rng.Intn(len(failPool))]
		}
		for i, s := range steps {
			if i < failIdx {
				out[i].Status = models.StepSucceeded
				out[i].DurationMs = durFor(s.Type)
				out[i].Output = stepOutput(s, entity)
			} else if i == failIdx {
				out[i].Status = models.StepFailed
			}
		}
		current = failIdx
		errMsg = steps[failIdx].Name + " — " + pick([]string{"timeout", "connection refused", "upstream 500", "validation error"})
	case models.InstWaiting:
		wIdx := firstApproval
		if wIdx < 0 {
			wIdx = len(steps) - 1
		}
		for i, s := range steps {
			if i < wIdx {
				out[i].Status = models.StepSucceeded
				out[i].DurationMs = durFor(s.Type)
				out[i].Output = stepOutput(s, entity)
			} else if i == wIdx {
				out[i].Status = models.StepWaiting
				sla := strOr(steps[i].Params, "sla_hours", "")
				if sla != "" {
					out[i].Note = "Waiting on " + strOr(steps[i].Params, "approver", "approver") + " · SLA " + sla + "h"
				} else {
					out[i].Note = "Waiting on " + strOr(steps[i].Params, "approver", "approver")
				}
			}
		}
		current = wIdx
		waitingOn = strOr(steps[wIdx].Params, "approver", "approver")
	case models.InstCancelled:
		cIdx := ri(1, max(1, len(steps)-1))
		for i, s := range steps {
			if i < cIdx {
				out[i].Status = models.StepSucceeded
				out[i].DurationMs = durFor(s.Type)
				out[i].Output = stepOutput(s, entity)
			} else {
				out[i].Status = models.StepSkipped
				out[i].Output = "cancelled"
			}
		}
		current = cIdx
	}
	return out, current, waitingOn, errMsg
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// buildInstance constructs one instance with step runs derived from the workflow.
func buildInstance(w models.Workflow, status, startedAt string) models.Instance {
	entity := entityFor(w.ID)
	stepRuns, current, waitingOn, errMsg := synthRuns(w.Steps, status, entity)
	totalMs := 0
	for _, r := range stepRuns {
		totalMs += r.DurationMs
	}
	inst := models.Instance{
		ID: "run-" + uid(), WorkflowID: w.ID, WorkflowName: w.Name, Status: status,
		Entity: entity, StartedAt: startedAt, StepRuns: stepRuns, CurrentStep: current,
		WaitingOn: waitingOn, Error: errMsg,
	}
	if status == models.InstCompleted || status == models.InstFailed || status == models.InstCancelled {
		t, _ := time.Parse(time.RFC3339, startedAt)
		inst.EndedAt = t.Add(time.Duration(totalMs)*time.Millisecond + time.Duration(ri(1000, 60000))*time.Millisecond).Format(time.RFC3339)
	}
	return inst
}

// Instances generates a balanced, deterministic set: for each workflow —
// 3 completed, 1 failed, 1 waiting, 1 cancelled, spread over 14 days.
// Plus 2 live "running" instances for an active first load.
func Instances(workflows []models.Workflow) []models.Instance {
	var list []models.Instance
	for wi, w := range workflows {
		// 3 completed
		for i := 0; i < 3; i++ {
			list = append(list, buildInstance(w, models.InstCompleted, isoAgo(ri(1, 13), ri(0, 23))))
		}
		// 1 failed
		list = append(list, buildInstance(w, models.InstFailed, isoAgo(ri(0, 7), ri(0, 23))))
		// 1 waiting (except leave, which is only 'approved')
		if w.Status == models.StatusDeployed {
			list = append(list, buildInstance(w, models.InstWaiting, isoAgo(0, 0)))
		}
		// 1 cancelled
		list = append(list, buildInstance(w, models.InstCancelled, isoAgo(ri(0, 10), ri(0, 23))))
		_ = wi
	}
	// 2 live running instances for immediate activity
	list = append(list, buildInstance(workflows[0], models.InstRunning, isoAgo(0, 0)))
	list = append(list, buildInstance(workflows[2], models.InstRunning, isoAgo(0, 0)))
	return list
}

// Audit generates audit entries referencing the actual workflows and instances.
func Audit(workflows []models.Workflow, instances []models.Instance) []models.AuditEntry {
	var entries []models.AuditEntry

	// Per-workflow: draft generated + approved
	for _, w := range workflows {
		entries = append(entries, models.AuditEntry{
			ID: uid(), At: w.CreatedAt, Actor: "AI author", Action: "Draft generated",
			Detail: w.Name + " v" + strconv.Itoa(w.Version) + " · AI draft", Kind: "ai",
		})
		if w.ApprovedBy != "" {
			entries = append(entries, models.AuditEntry{
				ID: uid(), At: w.CreatedAt, Actor: w.ApprovedBy, Action: "Approved & deployed",
				Detail: w.Name + " v" + strconv.Itoa(w.Version) + " — " + strconv.Itoa(len(w.Steps)) + " steps", Kind: "approval",
			})
		}
	}

	// Instance lifecycle events (sample the most recent)
	recent := instances
	if len(recent) > 15 {
		recent = recent[:15]
	}
	for _, inst := range recent {
		entries = append(entries, models.AuditEntry{
			ID: uid(), At: inst.StartedAt, Actor: "You", Action: "Execution started",
			Detail: inst.ID + " · " + inst.WorkflowName, Kind: "execution",
		})
		switch inst.Status {
		case models.InstWaiting:
			entries = append(entries, models.AuditEntry{
				ID: uid(), At: inst.StartedAt, Actor: "system", Action: "Instance waiting",
				Detail: inst.ID + " waiting on " + inst.WaitingOn, Kind: "execution",
			})
		case models.InstCompleted:
			entries = append(entries, models.AuditEntry{
				ID: uid(), At: inst.EndedAt, Actor: "system", Action: "Instance completed",
				Detail: inst.ID + " · " + inst.WorkflowName, Kind: "execution",
			})
		case models.InstFailed:
			entries = append(entries, models.AuditEntry{
				ID: uid(), At: inst.EndedAt, Actor: "system", Action: "Step failed",
				Detail: inst.ID + " · " + inst.Error, Kind: "execution",
			})
		}
	}

	// Operational entries
	entries = append(entries,
		models.AuditEntry{ID: uid(), At: isoAgo(1, 3), Actor: "Aisha M", Action: "Control created", Detail: "custom.send_sms — available in the palette", Kind: "deploy"},
		models.AuditEntry{ID: uid(), At: isoAgo(1, 6), Actor: "Priya N", Action: "MDM record updated", Detail: "vendors/V-10293 Acme Corp — tax_id corrected by steward", Kind: "mdm"},
		models.AuditEntry{ID: uid(), At: isoAgo(0, 2), Actor: "You", Action: "AI model updated", Detail: "Ollama (Local) · llama3.1", Kind: "deploy"},
	)
	return entries
}

// Controls returns the built-in step controls.
func Controls() []models.ControlDef {
	meta := []struct{ key, label, color, icon string }{
		{"trigger", "Trigger", "emerald", "Zap"},
		{"ai.extract", "AI Extract", "violet", "Sparkles"},
		{"ai.classify", "AI Classify", "violet", "Sparkles"},
		{"mdm.lookup", "MDM Lookup", "amber", "Database"},
		{"mdm.validate", "MDM Validate", "amber", "ShieldCheck"},
		{"condition", "Condition", "sky", "GitBranch"},
		{"human.approval", "Human Approval", "rose", "UserCheck"},
		{"notify", "Notify", "indigo", "Bell"},
		{"integration.post", "Post to System", "cyan", "ArrowRightLeft"},
		{"integration.http", "HTTP Call", "cyan", "Globe"},
		{"script", "Script", "slate", "Code"},
		{"wait", "Wait / SLA", "orange", "Timer"},
	}
	out := make([]models.ControlDef, 0, len(meta))
	for _, m := range meta {
		out = append(out, models.ControlDef{Key: m.key, Label: m.label, Color: m.color, Icon: m.icon, Enabled: true})
	}
	return out
}

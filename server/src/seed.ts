import { STEP_META, type AuditEntry, type ControlDef, type Instance, type MDMEntity, type StepRun, type Workflow, type WorkflowStep } from './types.js';
import type { DB } from './db.js';

const uid = () => Math.random().toString(36).slice(2, 9);
const rand = (min: number, max: number) => Math.floor(Math.random() * (max - min + 1)) + min;
const pick = <T,>(arr: T[]): T => arr[Math.floor(Math.random() * arr.length)];

const VENDORS = ['Acme Corp', 'Globex Ltd', 'Initech', 'Umbrella Supplies', 'Stark Industries', 'Wayne Enterprises', 'Hooli', 'Pied Piper'];
const PEOPLE = ['Dana W', 'Priya N', 'Dev K', 'Ravi S', 'Aisha M', 'Carlos R', 'Mei L', 'Tom H'];

// ---- Step duration / output helpers --------------------------------------

function durFor(type: string): number {
  switch (type) {
    case 'trigger': return rand(80, 180);
    case 'ai.extract':
    case 'ai.classify': return rand(1800, 2700);
    case 'mdm.validate':
    case 'mdm.lookup': return rand(120, 280);
    case 'condition': return rand(8, 40);
    case 'human.approval': return rand(180000, 900000);
    case 'notify': return rand(180, 650);
    case 'integration.post':
    case 'integration.http': return rand(400, 2300);
    case 'wait': return rand(1000, 5000);
    default: return rand(100, 400);
  }
}

function outFor(type: string, params: Record<string, string>, entity: string): string {
  switch (type) {
    case 'trigger': return `${entity} received`;
    case 'ai.extract': return `extracted fields · total $${rand(2, 48)}.${rand(10, 99)}K`;
    case 'ai.classify': return `classified: ${pick(['standard', 'high', 'critical'])}`;
    case 'mdm.validate': return 'matched golden record';
    case 'mdm.lookup': return 'entity resolved';
    case 'condition': return `${params.expression ?? 'condition'} → ${Math.random() < 0.8 ? 'true' : 'false'}`;
    case 'human.approval': return `approved by ${params.approver ?? 'approver'}`;
    case 'notify': return `sent via ${params.channel ?? 'email'}`;
    case 'integration.post': return `posted to ${params.system ?? 'target'} · 200 OK`;
    case 'integration.http': return 'HTTP 200';
    case 'wait': return 'SLA window elapsed';
    default: return 'executed';
  }
}

// ---- Workflow definitions -------------------------------------------------

const invoiceSteps: WorkflowStep[] = [
  { id: 'invoice_received_t1', type: 'trigger', name: 'Invoice received', params: { event: 'vendor_invoice.created', source: 'any' }, confidence: 95, assumptions: [] },
  { id: 'extract_line_items_a1', type: 'ai.extract', name: 'Extract line items', params: { fields: 'line_items, vendor, total, currency', model: 'auto' }, confidence: 88, assumptions: [] },
  { id: 'validate_vendor_m1', type: 'mdm.validate', name: 'Validate vendor against master', params: { entity: 'vendors', match_on: 'vendor_id, tax_id', on_mismatch: 'route_to_steward' }, confidence: 91, assumptions: [] },
  { id: 'amount_check_c1', type: 'condition', name: 'Amount > $10,000?', params: { expression: 'total > 10000', on_false: 'auto_approve' }, confidence: 93, assumptions: [] },
  { id: 'manager_approval_h1', type: 'human.approval', name: 'Cost-Center Manager approval', params: { approver: 'Cost-Center Manager', resolve_via: 'hr_hierarchy', sla_hours: '48' }, confidence: 82, assumptions: [] },
  { id: 'vp_escalation_h2', type: 'human.approval', name: 'Escalation to Finance VP', params: { approver: 'Finance VP', after_hours: '48', condition: 'previous_step.sla_breached' }, confidence: 78, assumptions: [] },
  { id: 'post_to_erp_i1', type: 'integration.post', name: 'Post to ERP', params: { system: 'ERP', endpoint: 'erp.inbound.invoices', mapping: 'auto' }, confidence: 84, assumptions: [] },
];

const onboardSteps: WorkflowStep[] = [
  { id: 'req_submitted_t1', type: 'trigger', name: 'Onboarding request submitted', params: { event: 'onboarding.submitted' }, confidence: 94, assumptions: [] },
  { id: 'validate_emp_m1', type: 'mdm.validate', name: 'Validate employee', params: { entity: 'employees', match_on: 'emp_id, email' }, confidence: 90, assumptions: [] },
  { id: 'manager_approval_h1', type: 'human.approval', name: 'Hiring Manager approval', params: { approver: 'Hiring Manager', sla_hours: '24' }, confidence: 85, assumptions: [] },
  { id: 'notify_it_n1', type: 'notify', name: 'Notify IT', params: { channel: 'slack', recipients: '#it-provisioning' }, confidence: 88, assumptions: [] },
  { id: 'post_workday_i1', type: 'integration.post', name: 'Post to Workday', params: { system: 'Workday', endpoint: 'workday.hires' }, confidence: 83, assumptions: [] },
];

const poSteps: WorkflowStep[] = [
  { id: 'po_received_t1', type: 'trigger', name: 'Purchase order received', params: { event: 'purchase_order.created' }, confidence: 95, assumptions: [] },
  { id: 'validate_vendor_m1', type: 'mdm.validate', name: 'Validate vendor', params: { entity: 'vendors', match_on: 'vendor_id' }, confidence: 91, assumptions: [] },
  { id: 'amount_check_c1', type: 'condition', name: 'Amount > $5,000?', params: { expression: 'total > 5000', on_false: 'auto_approve' }, confidence: 92, assumptions: [] },
  { id: 'manager_approval_h1', type: 'human.approval', name: 'Procurement Manager approval', params: { approver: 'Procurement Manager', sla_hours: '24' }, confidence: 84, assumptions: [] },
  { id: 'notify_requester_n1', type: 'notify', name: 'Notify requester', params: { channel: 'email' }, confidence: 87, assumptions: [] },
  { id: 'post_sap_i1', type: 'integration.post', name: 'Post to SAP', params: { system: 'SAP', endpoint: 'sap.inbound.orders' }, confidence: 83, assumptions: [] },
];

const expenseSteps: WorkflowStep[] = [
  { id: 'exp_received_t1', type: 'trigger', name: 'Expense report submitted', params: { event: 'expense.submitted' }, confidence: 95, assumptions: [] },
  { id: 'extract_a1', type: 'ai.extract', name: 'Extract receipt data', params: { fields: 'amount, category, date, vendor', model: 'auto' }, confidence: 87, assumptions: [] },
  { id: 'amount_check_c1', type: 'condition', name: 'Amount > $500?', params: { expression: 'total > 500', on_false: 'auto_approve' }, confidence: 92, assumptions: [] },
  { id: 'manager_approval_h1', type: 'human.approval', name: 'Manager approval', params: { approver: 'Manager', sla_hours: '48' }, confidence: 85, assumptions: [] },
  { id: 'post_finance_i1', type: 'integration.post', name: 'Post to Finance', params: { system: 'Finance', endpoint: 'finance.inbound.expenses' }, confidence: 84, assumptions: [] },
];

const ticketSteps: WorkflowStep[] = [
  { id: 'ticket_created_t1', type: 'trigger', name: 'Ticket created', params: { event: 'support_ticket.created' }, confidence: 95, assumptions: [] },
  { id: 'classify_a1', type: 'ai.classify', name: 'Classify priority', params: { model: 'auto' }, confidence: 86, assumptions: [] },
  { id: 'priority_check_c1', type: 'condition', name: 'Priority critical?', params: { expression: 'priority == critical', on_false: 'standard_path' }, confidence: 80, assumptions: [] },
  { id: 'lead_approval_h1', type: 'human.approval', name: 'Support Lead approval', params: { approver: 'Support Lead', sla_hours: '4' }, confidence: 83, assumptions: [] },
  { id: 'notify_oncall_n1', type: 'notify', name: 'Notify on-call', params: { channel: 'slack', recipients: '#on-call' }, confidence: 88, assumptions: [] },
  { id: 'sla_wait_w1', type: 'wait', name: 'Escalation SLA', params: { hours: '4' }, confidence: 78, assumptions: [] },
];

const leaveSteps: WorkflowStep[] = [
  { id: 'leave_submitted_t1', type: 'trigger', name: 'Leave request submitted', params: { event: 'leave.submitted' }, confidence: 95, assumptions: [] },
  { id: 'validate_emp_m1', type: 'mdm.validate', name: 'Validate employee', params: { entity: 'employees', match_on: 'emp_id' }, confidence: 90, assumptions: [] },
  { id: 'balance_check_c1', type: 'condition', name: 'Balance available?', params: { expression: 'balance > 0', on_false: 'deny' }, confidence: 88, assumptions: [] },
  { id: 'manager_approval_h1', type: 'human.approval', name: 'Manager approval', params: { approver: 'Manager', sla_hours: '24' }, confidence: 85, assumptions: [] },
  { id: 'post_hris_i1', type: 'integration.post', name: 'Post to HRIS', params: { system: 'HRIS', endpoint: 'hris.inbound.leave' }, confidence: 84, assumptions: [] },
];

const seedWorkflows: Workflow[] = [
  { id: 'wf-invoice', name: 'Vendor Invoice Approval', description: 'Extract, validate, approve and post vendor invoices over $10K.', prompt: 'When a vendor invoice over $10K arrives, extract line items, validate against the vendor master, route to the cost-center manager for approval, escalate to Finance VP after 48 hours, then post to the ERP.', status: 'deployed', version: 3, steps: invoiceSteps, createdBy: 'Priya N', approvedBy: 'Ravi S', aiModel: 'flowforge-author · gpt-4o-mini', createdAt: isoAgo(14), runs: 0 },
  { id: 'wf-onboard', name: 'Employee Onboarding', description: 'Validate and provision new hires after manager approval.', prompt: 'When an employee onboarding request is submitted, validate against HR master, route to the hiring manager for approval, notify IT, then post to Workday.', status: 'deployed', version: 1, steps: onboardSteps, createdBy: 'Dev K', approvedBy: 'Ravi S', aiModel: 'flowforge-author · gpt-4o-mini', createdAt: isoAgo(9), runs: 0 },
  { id: 'wf-po', name: 'Purchase Order Approval', description: 'Validate vendor and approve purchase orders over $5K, then post to SAP.', prompt: 'When a purchase order over $5K is submitted, validate the vendor, route to the procurement manager for approval, then post to SAP.', status: 'deployed', version: 2, steps: poSteps, createdBy: 'Aisha M', approvedBy: 'Ravi S', aiModel: 'flowforge-author · gpt-4o-mini', createdAt: isoAgo(7), runs: 0 },
  { id: 'wf-expense', name: 'Expense Report Approval', description: 'Extract receipt data and approve expenses over $500.', prompt: 'When an expense report is submitted, extract receipt data, if amount over $500 route to manager for approval, then post to Finance.', status: 'deployed', version: 1, steps: expenseSteps, createdBy: 'Priya N', approvedBy: 'Aisha M', aiModel: 'flowforge-author · gpt-4o-mini', createdAt: isoAgo(5), runs: 0 },
  { id: 'wf-ticket', name: 'Support Ticket Routing', description: 'Classify priority, route critical tickets to the support lead, escalate after 4h.', prompt: 'When a support ticket is created, classify it with AI, if priority is critical notify on-call on Slack, route to the support lead for approval, escalate after 4 hours.', status: 'deployed', version: 1, steps: ticketSteps, createdBy: 'Carlos R', approvedBy: 'Dev K', aiModel: 'flowforge-author · gpt-4o-mini', createdAt: isoAgo(3), runs: 0 },
  { id: 'wf-leave', name: 'Leave Request Approval', description: 'Validate leave balance and route to manager for approval.', prompt: 'When a leave request is submitted, validate the employee balance, route to the manager for approval, then post to the HRIS.', status: 'approved', version: 1, steps: leaveSteps, createdBy: 'Mei L', aiModel: 'flowforge-author · gpt-4o-mini', createdAt: isoAgo(1), runs: 0 },
];

// ---- Execution synthesis --------------------------------------------------

function isoAgo(days: number, hours = 0): string {
  return new Date(Date.now() - days * 86400000 - hours * 3600000 - rand(0, 3600000)).toISOString();
}

function entityFor(wfId: string): string {
  const n = rand(1000, 9999);
  switch (wfId) {
    case 'wf-invoice': return `INV-${n} · ${pick(VENDORS)}`;
    case 'wf-po': return `PO-${n} · ${pick(VENDORS)}`;
    case 'wf-expense': return `EXP-${n} · ${pick(PEOPLE)}`;
    case 'wf-ticket': return `TKT-${n} · ${pick(['Login issue', 'Billing error', 'Bug report', 'Feature request'])}`;
    case 'wf-leave': return `LV-${n} · ${pick(PEOPLE)}`;
    case 'wf-onboard': return `ONB-${n} · ${pick(PEOPLE)}`;
    default: return `REC-${n}`;
  }
}

function synthRuns(wf: Workflow, status: Instance['status']): { stepRuns: StepRun[]; currentStep: number; waitingOn?: string; error?: string } {
  const steps = wf.steps;
  const escalationIdx = steps.findIndex((s) => s.params?.condition === 'previous_step.sla_breached');
  const firstApproval = steps.findIndex((s) => s.type === 'human.approval');
  const failPool = steps.map((s, i) => i).filter((i) => ['integration.post', 'integration.http', 'ai.extract', 'ai.classify', 'mdm.validate'].includes(steps[i].type));
  const out: StepRun[] = steps.map((s) => ({ stepId: s.id, name: s.name, type: s.type, status: 'pending' as StepRun['status'] }));

  if (status === 'completed') {
    steps.forEach((s, i) => {
      if (i === escalationIdx) { out[i].status = 'skipped'; out[i].output = 'no SLA breach — skipped'; out[i].durationMs = 5; return; }
      out[i].status = 'succeeded'; out[i].durationMs = durFor(s.type); out[i].output = outFor(s.type, s.params, '');
    });
    return { stepRuns: out, currentStep: steps.length };
  }

  if (status === 'failed') {
    const failIdx = pick(failPool.length ? failPool : [steps.length - 1]);
    steps.forEach((s, i) => {
      if (i < failIdx) { out[i].status = 'succeeded'; out[i].durationMs = durFor(s.type); out[i].output = outFor(s.type, s.params, ''); }
      else if (i === failIdx) { out[i].status = 'failed'; out[i].note = `${s.name} failed`; out[i].output = undefined; }
    });
    return { stepRuns: out, currentStep: failIdx, error: `${steps[failIdx].name} — ${pick(['timeout', 'connection refused', 'upstream 500', 'validation error'])}` };
  }

  if (status === 'waiting') {
    const wIdx = firstApproval >= 0 ? firstApproval : steps.length - 1;
    steps.forEach((s, i) => {
      if (i < wIdx) { out[i].status = 'succeeded'; out[i].durationMs = durFor(s.type); out[i].output = outFor(s.type, s.params, ''); }
      else if (i === wIdx) { out[i].status = 'waiting'; out[i].note = `Waiting on ${s.params?.approver ?? 'approver'}${s.params?.sla_hours ? ` · SLA ${s.params.sla_hours}h` : ''}`; }
    });
    return { stepRuns: out, currentStep: wIdx, waitingOn: steps[wIdx].params?.approver ?? 'approver' };
  }

  if (status === 'running') {
    const rIdx = rand(1, Math.max(1, steps.length - 2));
    steps.forEach((s, i) => {
      if (i < rIdx) { out[i].status = 'succeeded'; out[i].durationMs = durFor(s.type); out[i].output = outFor(s.type, s.params, ''); }
      else if (i === rIdx) { out[i].status = 'running'; out[i].startedAt = 'now'; }
    });
    return { stepRuns: out, currentStep: rIdx };
  }

  // cancelled
  const cIdx = rand(1, Math.max(1, steps.length - 1));
  steps.forEach((s, i) => {
    if (i < cIdx) { out[i].status = 'succeeded'; out[i].durationMs = durFor(s.type); out[i].output = outFor(s.type, s.params, ''); }
    else { out[i].status = 'skipped'; out[i].output = 'cancelled'; }
  });
  return { stepRuns: out, currentStep: cIdx };
}

function buildInstance(wf: Workflow, status: Instance['status'], startedAt: string): Instance {
  const entity = entityFor(wf.id);
  const { stepRuns, currentStep, waitingOn, error } = synthRuns(wf, status);
  const totalMs = stepRuns.reduce((a, s) => a + (s.durationMs ?? 0), 0);
  const endedAt = status === 'completed' || status === 'failed' || status === 'cancelled'
    ? new Date(new Date(startedAt).getTime() + totalMs + rand(1000, 60000)).toISOString()
    : undefined;
  return { id: `run-${uid().slice(0, 4)}`, workflowId: wf.id, workflowName: wf.name, status, entity, startedAt, endedAt, stepRuns, currentStep, waitingOn, error };
}

function genHistory(wf: Workflow, count: number): Instance[] {
  const out: Instance[] = [];
  for (let i = 0; i < count; i++) {
    const r = Math.random();
    const status: Instance['status'] = r < 0.72 ? 'completed' : r < 0.86 ? 'failed' : 'cancelled';
    out.push(buildInstance(wf, status, isoAgo(rand(0, 13), rand(0, 23))));
  }
  return out;
}

function genInstances(): Instance[] {
  const list: Instance[] = [];
  const counts: Record<string, number> = { 'wf-invoice': 10, 'wf-po': 6, 'wf-expense': 5, 'wf-ticket': 5, 'wf-onboard': 4, 'wf-leave': 4 };
  for (const wf of seedWorkflows) list.push(...genHistory(wf, counts[wf.id] ?? 4));
  // Live executions for an active Monitor on first load.
  const inv = seedWorkflows[0];
  list.push(buildInstance(inv, 'waiting', isoAgo(0, 0)));
  list.push(buildInstance(inv, 'running', isoAgo(0, 0)));
  list.push(buildInstance(seedWorkflows[2], 'running', isoAgo(0, 0)));
  // Reflect run counts on each workflow.
  for (const wf of seedWorkflows) wf.runs = list.filter((i) => i.workflowId === wf.id).length;
  return list;
}

const seedAudit: AuditEntry[] = [
  { id: uid(), at: isoAgo(0, 0), actor: 'system', action: 'Instance waiting', detail: 'Cost-Center Manager approval pending', kind: 'execution' },
  { id: uid(), at: isoAgo(0, 1), actor: 'Ravi S', action: 'Approved & deployed v3', detail: 'Vendor Invoice Approval — 7 steps, 2 human gates', kind: 'approval' },
  { id: uid(), at: isoAgo(0, 2), actor: 'AI author', action: 'Draft generated', detail: 'Purchase Order Approval · 89% avg confidence', kind: 'ai' },
  { id: uid(), at: isoAgo(1, 3), actor: 'Aisha M', action: 'Control created', detail: 'custom.send_sms — available in the palette', kind: 'deploy' },
  { id: uid(), at: isoAgo(1, 6), actor: 'Priya N', action: 'MDM record updated', detail: 'vendors/V-10293 Acme Corp — tax_id corrected by steward', kind: 'mdm' },
  { id: uid(), at: isoAgo(2, 4), actor: 'Dev K', action: 'Execution failed', detail: 'wf-expense · Post to Finance — timeout', kind: 'execution' },
  { id: uid(), at: isoAgo(3, 8), actor: 'system', action: 'Instance completed', detail: 'wf-onboard · 5 steps · 9.4m', kind: 'execution' },
];

const seedMDM: MDMEntity[] = [
  { key: 'vendors', label: 'Vendors', icon: 'Building2', fields: ['vendor_id', 'name', 'tax_id', 'country', 'status'], records: [
    { id: 'V-10293', vendor_id: 'V-10293', name: 'Acme Corp', tax_id: 'US-84-2210', country: 'US', status: 'golden' },
    { id: 'V-10877', vendor_id: 'V-10877', name: 'Globex Ltd', tax_id: 'UK-99-3311', country: 'UK', status: 'golden' },
    { id: 'V-11240', vendor_id: 'V-11240', name: 'Initech', tax_id: 'US-71-8842', country: 'US', status: 'golden' },
    { id: 'V-11301', vendor_id: 'V-11301', name: 'Umbrella Supplies', tax_id: 'DE-45-0091', country: 'DE', status: 'pending stewardship' },
  ] },
  { key: 'customers', label: 'Customers', icon: 'Users', fields: ['cust_id', 'name', 'segment', 'country', 'status'], records: [
    { id: 'C-3301', cust_id: 'C-3301', name: 'Stark Industries', segment: 'enterprise', country: 'US', status: 'golden' },
    { id: 'C-3302', cust_id: 'C-3302', name: 'Wayne Enterprises', segment: 'enterprise', country: 'US', status: 'golden' },
    { id: 'C-3307', cust_id: 'C-3307', name: 'Pied Piper', segment: 'smb', country: 'US', status: 'golden' },
  ] },
  { key: 'products', label: 'Products', icon: 'Package', fields: ['sku', 'name', 'category', 'uom', 'status'], records: [
    { id: 'SKU-100', sku: 'SKU-100', name: 'Cloud Credits 1K', category: 'software', uom: 'unit', status: 'golden' },
    { id: 'SKU-214', sku: 'SKU-214', name: 'Support Plan — Gold', category: 'services', uom: 'year', status: 'golden' },
  ] },
  { key: 'employees', label: 'Employees', icon: 'IdCard', fields: ['emp_id', 'name', 'role', 'manager', 'status'], records: [
    { id: 'E-101', emp_id: 'E-101', name: 'Dana W', role: 'Cost-Center Manager', manager: 'Ravi S', status: 'golden' },
    { id: 'E-102', emp_id: 'E-102', name: 'Ravi S', role: 'Finance VP', manager: '—', status: 'golden' },
    { id: 'E-114', emp_id: 'E-114', name: 'Priya N', role: 'Business Analyst', manager: 'Dev K', status: 'golden' },
  ] },
];

export function seedIfEmpty(d: DB) {
  if (d.listWorkflows().length === 0) seedWorkflows.forEach((w) => d.upsertWorkflow(w));
  if (d.listInstances().length === 0) genInstances().forEach((i) => d.upsertInstance(i));
  if (d.listAudit().length === 0) seedAudit.forEach((a) => d.addAudit(a));
  if (d.listMDM().length === 0) seedMDM.forEach((e) => d.upsertMDM(e));
  if (d.listControls().length === 0) {
    Object.entries(STEP_META).forEach(([key, m]) => d.upsertControl({ key, label: m.label, color: m.color, icon: m.icon, enabled: true }));
  }
}

// Re-exported for the metrics module / tests.
export { seedWorkflows };

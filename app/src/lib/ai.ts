import type { WorkflowStep, StepType } from './types';

// ---------------------------------------------------------------------------
// Mock AI authoring engine (prototype stand-in for the LLM layer).
// Parses natural language into a flowforge/v1 draft: typed steps, per-step
// confidence, and explicit assumptions the human reviewer must confirm.
// ---------------------------------------------------------------------------

const slug = (s: string) =>
  s.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 40);

const uid = () => Math.random().toString(36).slice(2, 8);

function extractAmount(text: string): string | null {
  const m = text.match(/\$\s?([\d,.]+)\s?(k|m|thousand|million)?/i);
  if (!m) return null;
  let n = parseFloat(m[1].replace(/,/g, ''));
  const unit = (m[2] || '').toLowerCase();
  if (unit === 'k' || unit === 'thousand') n *= 1000;
  if (unit === 'm' || unit === 'million') n *= 1000000;
  return String(n);
}

function extractHours(text: string): string | null {
  const m = text.match(/(\d+)\s?(hours?|hrs?|days?)/i);
  if (!m) return null;
  const n = parseInt(m[1], 10);
  return /day/i.test(m[2]) ? String(n * 24) : String(n);
}

function extractRole(text: string): string {
  const patterns = [
    /route(?:d)? to (?:the )?([a-z -]+?)(?: for approval| to approve| after| then| ,|\.|$)/i,
    /(?:the )?([a-z -]*?(?:manager|vp|director|head|lead|officer|controller|steward|admin|owner))(?: approves| for approval| to approve|,| after| then|\.|$)/i,
    /escalate(?:s|d)? to (?:the )?([a-z -]+?)(?: after| if| ,|\.|$)/i,
  ];
  for (const p of patterns) {
    const m = text.match(p);
    if (m && m[1].trim().length > 2) return titleCase(m[1].trim());
  }
  return 'Process Owner';
}

function titleCase(s: string) {
  return s.replace(/\w\S*/g, (w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase());
}

export interface GeneratedDraft {
  name: string;
  description: string;
  steps: WorkflowStep[];
  model: string;
  overallConfidence: number;
}

export function generateWorkflow(prompt: string): GeneratedDraft {
  const text = prompt.trim();
  const lower = text.toLowerCase();
  const steps: WorkflowStep[] = [];
  const add = (type: StepType, name: string, params: Record<string, string>, confidence: number, assumptions: string[] = []) =>
    steps.push({ id: `${slug(name)}_${uid()}`, type, name, params, confidence, assumptions });

  // --- Trigger -------------------------------------------------------------
  const triggerMatch = lower.match(/when (?:an? |the )?([a-z ]+?)(?: arrives| is (?:created|submitted|received)| is over|,|\.|$)/);
  const entity = triggerMatch ? titleCase(triggerMatch[1].trim()) : 'Record';
  const amount = extractAmount(text);
  add('trigger', `${entity} received`, {
    event: `${slug(entity)}.created`,
    source: lower.includes('email') ? 'email' : lower.includes('api') ? 'api' : 'any',
  }, 95, triggerMatch ? [] : [`No explicit trigger found — assumed "${entity} received".`]);

  // --- AI extraction --------------------------------------------------------
  if (/extract|parse|read|ocr|line.?item|capture/.test(lower)) {
    add('ai.extract', 'Extract data with AI', {
      fields: /line.?item/.test(lower) ? 'line_items, vendor, total, currency, due_date' : 'key fields',
      model: 'auto',
    }, 88, ['Field list inferred from prompt — confirm the exact fields to extract.']);
  }

  // --- MDM validation -------------------------------------------------------
  if (/valid|match|master|vendor|customer|mdm|verify/.test(lower)) {
    const entityRef = /vendor/.test(lower) ? 'vendors' : /customer/.test(lower) ? 'customers' : /product/.test(lower) ? 'products' : 'vendors';
    add('mdm.validate', `Validate against ${titleCase(entityRef)} master`, {
      entity: entityRef,
      match_on: entityRef === 'vendors' ? 'vendor_id, tax_id' : 'id, email',
      on_mismatch: 'route_to_steward',
    }, 91, [`Assumed master data entity "${entityRef}" — confirm the golden-record source.`]);
  }

  // --- Condition ------------------------------------------------------------
  if (amount || /if |over |exceed|greater|above/.test(lower)) {
    add('condition', `Amount check${amount ? ` > $${Number(amount).toLocaleString()}` : ''}`, {
      expression: amount ? `total > ${amount}` : 'total > threshold',
      on_false: 'auto_approve',
    }, amount ? 93 : 70, amount ? [] : ['No threshold found — using a placeholder, please set the limit.']);
  }

  // --- Human approval -------------------------------------------------------
  if (/approv|review|sign.?off|route/.test(lower)) {
    const role = extractRole(text);
    const hours = extractHours(text);
    add('human.approval', `Approval by ${role}`, {
      approver: role,
      resolve_via: 'hr_hierarchy',
      ...(hours ? { sla_hours: hours } : {}),
    }, 82, [`Approver "${role}" resolves via the HR hierarchy — confirm the resolution rule.`]);
  }

  // --- Escalation -----------------------------------------------------------
  if (/escalat/.test(lower)) {
    const escRole = (text.match(/escalate(?:s|d)? to (?:the )?([a-z -]+?)(?: after| if| ,|\.|$)/i)?.[1] || 'Finance VP').trim();
    const hours = extractHours(text);
    add('human.approval', `Escalation to ${titleCase(escRole)}`, {
      approver: titleCase(escRole),
      ...(hours ? { after_hours: hours } : {}),
      condition: 'previous_step.sla_breached',
    }, 78, ['Escalation triggers on SLA breach of the previous approval step.']);
  }

  // --- Notify ---------------------------------------------------------------
  if (/notif|inform|email|slack|teams|alert/.test(lower)) {
    add('notify', 'Notify stakeholders', {
      channel: /slack/.test(lower) ? 'slack' : /teams/.test(lower) ? 'teams' : 'email',
      recipients: 'requester, procurement',
    }, 86, []);
  }

  // --- Integration / post ---------------------------------------------------
  const sysMatch = lower.match(/post (?:it |the \w+ )?to (?:the )?([a-z0-9 ]+?)(?:\.|,| then| and|$)/);
  if (/post|sync|push|send to|update|erp|sap|salesforce|servicenow|workday/.test(lower)) {
    const system = sysMatch ? titleCase(sysMatch[1].trim()) : /erp/.test(lower) ? 'ERP' : 'Target system';
    add('integration.post', `Post to ${system}`, {
      system,
      endpoint: `${slug(system)}.inbound`,
      mapping: 'auto',
    }, 84, [`Assumed "${system}" exposes a standard inbound API — confirm the connector.`]);
  }

  // --- Fallback if nothing matched ------------------------------------------
  if (steps.length <= 1) {
    add('script', 'Process record', { runtime: 'javascript', hint: text.slice(0, 60) }, 60, [
      'Could not infer detailed steps from the prompt — this is a placeholder. Edit or re-describe.',
    ]);
    add('notify', 'Notify requester', { channel: 'email', recipients: 'requester' }, 65, ['Added a default notification step.']);
  }

  // --- Name -----------------------------------------------------------------
  const name = (() => {
    if (/invoice/.test(lower)) return 'Vendor Invoice Approval';
    if (/onboard/.test(lower)) return 'Employee Onboarding';
    if (/purchase|procurement/.test(lower)) return 'Purchase Request';
    if (/expense/.test(lower)) return 'Expense Approval';
    if (/ticket|incident|support/.test(lower)) return 'Support Ticket Routing';
    return `${entity} Workflow`;
  })();

  const overall = Math.round(steps.reduce((s, st) => s + st.confidence, 0) / steps.length);

  return {
    name,
    description: `Auto-generated from prompt: “${text.slice(0, 120)}${text.length > 120 ? '…' : ''}”`,
    steps,
    model: 'flowforge-author (mock) · local-capable',
    overallConfidence: overall,
  };
}

export const SAMPLE_PROMPTS = [
  'When a vendor invoice over $10K arrives, extract line items, validate against the vendor master, route to the cost-center manager for approval, escalate to Finance VP after 48 hours, then post to the ERP.',
  'When an employee onboarding request is submitted, validate the employee against the HR master, route to the hiring manager for approval, notify IT on Slack, then post to Workday.',
  'When a support ticket is created, classify it with AI, if priority is critical notify the on-call team on Slack, route to the support lead for approval, escalate to the support director after 4 hours.',
];

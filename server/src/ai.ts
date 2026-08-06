import OpenAI from 'openai';
import { STEP_META, type GeneratedDraft, type StepType, type WorkflowStep } from './types.js';
import { slug, titleCase, uid } from './util.js';
import { isLLMActive, type AIConfig } from './settings.js';

// ---------------------------------------------------------------------------
// AI authoring. Calls an OpenAI-compatible chat endpoint to turn a natural-
// language prompt into a typed flowforge/v1 draft (steps, per-step confidence,
// explicit assumptions). Falls back to a deterministic local generator if the
// model is unavailable, so the demo always works.
// ---------------------------------------------------------------------------

export function authoringModel(cfg: AIConfig): string {
  return isLLMActive(cfg) ? cfg.model : 'flowforge-author (deterministic · no model set)';
}

export const SAMPLE_PROMPTS = [
  'When a vendor invoice over $10K arrives, extract line items, validate against the vendor master, route to the cost-center manager for approval, escalate to Finance VP after 48 hours, then post to the ERP.',
  'When an employee onboarding request is submitted, validate the employee against the HR master, route to the hiring manager for approval, notify IT on Slack, then post to Workday.',
  'When a support ticket is created, classify it with AI, if priority is critical notify the on-call team on Slack, route to the support lead for approval, escalate to the support director after 4 hours.',
];

const ALLOWED = new Set<string>(Object.keys(STEP_META));

const TYPE_ALIASES: Record<string, string> = {
  trigger: 'trigger', start: 'trigger', when: 'trigger', event: 'trigger', webhook: 'trigger',
  'ai.extract': 'ai.extract', extract: 'ai.extract', ocr: 'ai.extract', parse: 'ai.extract',
  'ai.classify': 'ai.classify', classify: 'ai.classify', categorize: 'ai.classify',
  'mdm.lookup': 'mdm.lookup', lookup: 'mdm.lookup', resolve: 'mdm.lookup',
  'mdm.validate': 'mdm.validate', validate: 'mdm.validate', verify: 'mdm.validate', match: 'mdm.validate',
  condition: 'condition', branch: 'condition', if: 'condition', decision: 'condition', gate: 'condition',
  'human.approval': 'human.approval', approval: 'human.approval', approve: 'human.approval', human: 'human.approval', review: 'human.approval', signoff: 'human.approval',
  notify: 'notify', notification: 'notify', email: 'notify', slack: 'notify', teams: 'notify', send: 'notify', inform: 'notify', alert: 'notify',
  'integration.post': 'integration.post', post: 'integration.post', create: 'integration.post',
  'integration.http': 'integration.http', http: 'integration.http', api: 'integration.http', call: 'integration.http', request: 'integration.http',
  script: 'script', code: 'script', transform: 'script', compute: 'script',
  wait: 'wait', delay: 'wait', sla: 'wait', timer: 'wait', pause: 'wait',
};

function normalizeType(t: unknown): string {
  const k = String(t ?? '').trim().toLowerCase().replace(/\s+/g, '');
  if (ALLOWED.has(k)) return k;
  return TYPE_ALIASES[k] ?? 'script';
}

function toStringParams(raw: unknown): Record<string, string> {
  if (raw === null || typeof raw !== 'object') return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    if (v === undefined || v === null) continue;
    out[k] = typeof v === 'object' ? JSON.stringify(v) : String(v);
  }
  return out;
}

function clampConf(n: unknown): number {
  const v = typeof n === 'number' ? n : parseFloat(String(n));
  if (!Number.isFinite(v)) return 80;
  return Math.max(50, Math.min(99, Math.round(v)));
}

function toAssumptions(raw: unknown): string[] {
  return Array.isArray(raw) ? raw.map((x) => String(x)).filter(Boolean) : [];
}

function normalizeDraft(raw: any, prompt: string): GeneratedDraft {
  const rawSteps: any[] = Array.isArray(raw?.steps) ? raw.steps : [];
  const steps: WorkflowStep[] = rawSteps.map((s) => {
    const type = normalizeType(s?.type);
    return {
      id: `${slug(String(s?.name ?? type))}_${uid()}`,
      type,
      name: String(s?.name ?? STEP_META[type as StepType]?.label ?? type).slice(0, 80),
      params: toStringParams(s?.params),
      confidence: clampConf(s?.confidence),
      assumptions: toAssumptions(s?.assumptions),
    };
  });

  const triggers = steps.filter((s) => s.type === 'trigger');
  let trigger: WorkflowStep;
  if (triggers.length === 0) {
    const triggerMatch = prompt.toLowerCase().match(/when (?:an? |the )?([a-z ]+?)(?: arrives| is |,|\.|$)/);
    const entity = triggerMatch ? titleCase(triggerMatch[1].trim()) : 'Record';
    trigger = { id: `trigger_${uid()}`, type: 'trigger', name: `${entity} received`, params: { event: `${slug(entity)}.created`, source: 'any' }, confidence: 95, assumptions: [`No explicit trigger found — assumed "${entity} received".`] };
    steps.unshift(trigger);
  } else {
    trigger = triggers[0];
    if (triggers.length > 1) {
      const drop = new Set(triggers.slice(1).map((t) => t.id));
      for (let i = steps.length - 1; i >= 0; i--) if (drop.has(steps[i].id)) steps.splice(i, 1);
    }
    if (steps[0].id !== trigger.id) {
      const i = steps.findIndex((s) => s.id === trigger.id);
      if (i > 0) { steps.splice(i, 1); steps.unshift(trigger); }
    }
  }

  if (steps.length <= 1) {
    steps.push({ id: `script_${uid()}`, type: 'script', name: 'Process record', params: { runtime: 'javascript', hint: prompt.slice(0, 60) }, confidence: 60, assumptions: ['Could not infer detailed steps from the prompt — this is a placeholder. Edit or re-describe.'] });
  }

  const name = String(raw?.name || `${trigger.name.replace(/ received$/, '')} Workflow`).slice(0, 80);
  const description = String(raw?.description || `Auto-generated from prompt: “${prompt.slice(0, 120)}${prompt.length > 120 ? '…' : ''}”`);
  const overall = Math.round(steps.reduce((s, st) => s + st.confidence, 0) / steps.length);

  return { name, description, steps, model: '', overallConfidence: overall };
}

const SYSTEM_PROMPT = `You are FlowForge Author, an expert that turns a natural-language description of a business process into a structured workflow draft.

Output STRICT JSON only (no prose, no markdown) with this exact shape:
{
  "name": "short workflow name",
  "description": "one sentence",
  "steps": [
    { "type": "<stepType>", "name": "human readable", "params": { "key": "value" }, "confidence": <0-100>, "assumptions": ["any inferred decision a human must confirm"] }
  ]
}

Rules:
- The FIRST step MUST have type "trigger".
- Choose stepType from exactly: trigger, ai.extract, ai.classify, mdm.lookup, mdm.validate, condition, human.approval, notify, integration.post, integration.http, script, wait.
- "extract/parse/OCR" -> ai.extract; "validate/verify/match against master" -> mdm.validate; "route to <role> for approval / sign-off" -> human.approval with params { "approver": "<role>", "sla_hours": "<n>" }; "escalate to <role> after Xh" -> human.approval with params { "approver": "<role>", "after_hours": "<n>", "condition": "previous_step.sla_breached" }; "notify/email/slack/teams" -> notify with params { "channel": "email|slack|teams" }; "post to <system>" -> integration.post with params { "system": "<System>", "endpoint": "<system>.inbound" }; amount/threshold checks -> condition with params { "expression": "total > <number>", "on_false": "auto_approve" }.
- Set confidence (0-100) per step: high for explicit instructions, lower for inferred ones.
- Add an assumption string for anything you inferred (entity names, approver roles, thresholds, endpoints). Be specific.
- Keep params values as strings. 4-8 steps is ideal.`;

function extractJSON(content: string): any {
  const trimmed = content.trim();
  try {
    return JSON.parse(trimmed);
  } catch {
    /* try to locate a JSON object/array inside the text */
  }
  const start = trimmed.search(/[[{]/);
  if (start === -1) throw new Error('No JSON found in model output');
  const open = trimmed[start];
  const close = open === '[' ? ']' : '}';
  const end = trimmed.lastIndexOf(close);
  if (end > start) {
    return JSON.parse(trimmed.slice(start, end + 1));
  }
  throw new Error('Could not parse JSON from model output');
}

async function authorWithLLM(prompt: string, cfg: AIConfig): Promise<GeneratedDraft> {
  const client = new OpenAI({ apiKey: cfg.apiKey || 'local-no-key', baseURL: cfg.baseURL, maxRetries: 0 });
  const baseMessages = [
    { role: 'system' as const, content: SYSTEM_PROMPT },
    { role: 'user' as const, content: prompt },
  ];

  let content: string | undefined;
  // First attempt: JSON mode (supported by most OpenAI-compatible servers).
  try {
    const c1 = await client.chat.completions.create({ model: cfg.model, temperature: 0.3, response_format: { type: 'json_object' }, messages: baseMessages });
    content = c1.choices[0]?.message?.content ?? undefined;
  } catch {
    /* some local servers reject response_format — retry without it below */
  }
  // Second attempt: plain completion (broader compatibility for local LLMs).
  if (!content) {
    const c2 = await client.chat.completions.create({ model: cfg.model, temperature: 0.3, messages: baseMessages });
    content = c2.choices[0]?.message?.content ?? undefined;
  }
  if (!content) throw new Error('Empty completion from model');

  return normalizeDraft(extractJSON(content), prompt);
}

// ---- Deterministic fallback (ported from the prototype) --------------------

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

function authorDeterministic(prompt: string): GeneratedDraft {
  const text = prompt.trim();
  const lower = text.toLowerCase();
  const steps: WorkflowStep[] = [];
  const add = (type: StepType, name: string, params: Record<string, string>, confidence: number, assumptions: string[] = []) =>
    steps.push({ id: `${slug(name)}_${uid()}`, type, name, params, confidence, assumptions });

  const triggerMatch = lower.match(/when (?:an? |the )?([a-z ]+?)(?: arrives| is (?:created|submitted|received)| is over|,|\.|$)/);
  const entity = triggerMatch ? titleCase(triggerMatch[1].trim()) : 'Record';
  const amount = extractAmount(text);
  add('trigger', `${entity} received`, { event: `${slug(entity)}.created`, source: lower.includes('email') ? 'email' : lower.includes('api') ? 'api' : 'any' }, 95, triggerMatch ? [] : [`No explicit trigger found — assumed "${entity} received".`]);

  if (/extract|parse|read|ocr|line.?item|capture/.test(lower)) {
    add('ai.extract', 'Extract data with AI', { fields: /line.?item/.test(lower) ? 'line_items, vendor, total, currency, due_date' : 'key fields', model: 'auto' }, 88, ['Field list inferred from prompt — confirm the exact fields to extract.']);
  }
  if (/valid|match|master|vendor|customer|mdm|verify/.test(lower)) {
    const entityRef = /vendor/.test(lower) ? 'vendors' : /customer/.test(lower) ? 'customers' : /product/.test(lower) ? 'products' : 'vendors';
    add('mdm.validate', `Validate against ${titleCase(entityRef)} master`, { entity: entityRef, match_on: entityRef === 'vendors' ? 'vendor_id, tax_id' : 'id, email', on_mismatch: 'route_to_steward' }, 91, [`Assumed master data entity "${entityRef}" — confirm the golden-record source.`]);
  }
  if (amount || /if |over |exceed|greater|above/.test(lower)) {
    add('condition', `Amount check${amount ? ` > $${Number(amount).toLocaleString()}` : ''}`, { expression: amount ? `total > ${amount}` : 'total > threshold', on_false: 'auto_approve' }, amount ? 93 : 70, amount ? [] : ['No threshold found — using a placeholder, please set the limit.']);
  }
  if (/approv|review|sign.?off|route/.test(lower)) {
    const role = extractRole(text);
    const hours = extractHours(text);
    add('human.approval', `Approval by ${role}`, { approver: role, resolve_via: 'hr_hierarchy', ...(hours ? { sla_hours: hours } : {}) }, 82, [`Approver "${role}" resolves via the HR hierarchy — confirm the resolution rule.`]);
  }
  if (/escalat/.test(lower)) {
    const escRole = (text.match(/escalate(?:s|d)? to (?:the )?([a-z -]+?)(?: after| if| ,|\.|$)/i)?.[1] || 'Finance VP').trim();
    const hours = extractHours(text);
    add('human.approval', `Escalation to ${titleCase(escRole)}`, { approver: titleCase(escRole), ...(hours ? { after_hours: hours } : {}), condition: 'previous_step.sla_breached' }, 78, ['Escalation triggers on SLA breach of the previous approval step.']);
  }
  if (/notif|inform|email|slack|teams|alert/.test(lower)) {
    add('notify', 'Notify stakeholders', { channel: /slack/.test(lower) ? 'slack' : /teams/.test(lower) ? 'teams' : 'email', recipients: 'requester, procurement' }, 86, []);
  }
  if (/post|sync|push|send to|update|erp|sap|salesforce|servicenow|workday/.test(lower)) {
    const sysMatch = lower.match(/post (?:it |the \w+ )?to (?:the )?([a-z0-9 ]+?)(?:\.|,| then| and|$)/);
    const system = sysMatch ? titleCase(sysMatch[1].trim()) : /erp/.test(lower) ? 'ERP' : 'Target system';
    add('integration.post', `Post to ${system}`, { system, endpoint: `${slug(system)}.inbound`, mapping: 'auto' }, 84, [`Assumed "${system}" exposes a standard inbound API — confirm the connector.`]);
  }
  if (steps.length <= 1) {
    add('script', 'Process record', { runtime: 'javascript', hint: text.slice(0, 60) }, 60, ['Could not infer detailed steps from the prompt — this is a placeholder. Edit or re-describe.']);
    add('notify', 'Notify requester', { channel: 'email', recipients: 'requester' }, 65, ['Added a default notification step.']);
  }

  const name = /invoice/.test(lower) ? 'Vendor Invoice Approval' : /onboard/.test(lower) ? 'Employee Onboarding' : /purchase|procurement/.test(lower) ? 'Purchase Request' : /expense/.test(lower) ? 'Expense Approval' : /ticket|incident|support/.test(lower) ? 'Support Ticket Routing' : `${entity} Workflow`;
  const overall = Math.round(steps.reduce((s, st) => s + st.confidence, 0) / steps.length);
  return { name, description: `Auto-generated from prompt: “${text.slice(0, 120)}${text.length > 120 ? '…' : ''}”`, steps, model: '', overallConfidence: overall };
}

export interface DraftResult {
  draft: GeneratedDraft;
  source: 'llm' | 'fallback';
  model: string;
}

export async function generateDraft(prompt: string, cfg: AIConfig): Promise<DraftResult> {
  if (isLLMActive(cfg)) {
    try {
      const draft = await authorWithLLM(prompt, cfg);
      draft.model = cfg.model;
      return { draft, source: 'llm', model: cfg.model };
    } catch (e) {
      console.warn('[ai] LLM authoring failed, falling back to deterministic generator:', (e as Error).message);
    }
  }
  const draft = authorDeterministic(prompt);
  draft.model = 'flowforge-author (deterministic fallback)';
  return { draft, source: 'fallback', model: draft.model };
}

// ---- Connection test (used by Admin → AI config) --------------------------

export interface TestResult {
  ok: boolean;
  latencyMs: number;
  model?: string;
  models?: string[];
  modelAvailable?: boolean;
  note?: string;
  error?: string;
}

export async function testConnection(cfg: AIConfig): Promise<TestResult> {
  const client = new OpenAI({ apiKey: cfg.apiKey || 'local-no-key', baseURL: cfg.baseURL });
  const t0 = Date.now();
  try {
    const list = await client.models.list();
    const ids = (list.data ?? []).map((m: { id: string }) => m.id);
    const modelAvailable = ids.length === 0 ? true : ids.includes(cfg.model) || ids.some((id) => id.startsWith(cfg.model.split(':')[0]) || cfg.model.startsWith(id.split('-').slice(0, 2).join('-')));
    return { ok: true, latencyMs: Date.now() - t0, models: ids.slice(0, 50), model: cfg.model, modelAvailable };
  } catch {
    /* models.list not supported — try a minimal chat completion */
  }
  try {
    await client.chat.completions.create({ model: cfg.model, messages: [{ role: 'user', content: 'Reply with the single word: ok' }], max_tokens: 3 });
    return { ok: true, latencyMs: Date.now() - t0, model: cfg.model, note: 'Chat endpoint reachable (model list not supported by this server).' };
  } catch (e) {
    return { ok: false, latencyMs: Date.now() - t0, error: (e as Error).message };
  }
}

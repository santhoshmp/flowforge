import type { FastifyInstance } from 'fastify';
import type { DB } from './db.js';
import { uid } from './util.js';
import { generateDraft, SAMPLE_PROMPTS, authoringModel, testConnection } from './ai.js';
import { getAIConfig, publicAISettings, setAIConfig, type AIConfig } from './settings.js';
import { computeMetrics } from './metrics.js';
import { approveWaiting, retryFailed, cancelInstance } from './engine.js';
import type { AuditEntry, ControlDef, GeneratedDraft, Instance, MDMRecord, Workflow, WorkflowStep } from './types.js';

export function registerRoutes(app: FastifyInstance, d: DB) {
  // ---- Health & bootstrap -------------------------------------------------
  app.get('/api/v1/health', async () => ({ status: 'ok', model: authoringModel(getAIConfig(d)) }));

  app.get('/api/v1/bootstrap', async () => ({
    workflows: d.listWorkflows(),
    instances: d.listInstances(),
    audit: d.listAudit(),
    mdm: d.listMDM(),
    controls: d.listControls(),
  }));

  // ---- Metrics (tracking dashboard) ---------------------------------------
  app.get('/api/v1/metrics', async () => computeMetrics(d));

  // ---- AI authoring -------------------------------------------------------
  app.post('/api/v1/ai/draft', async (req, reply) => {
    const { prompt } = req.body as { prompt?: string };
    if (!prompt?.trim()) return reply.code(400).send({ error: 'prompt is required' });
    const cfg = getAIConfig(d);
    const result = await generateDraft(prompt.trim(), cfg);
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'AI author', action: 'Draft generated', detail: `${result.draft.name} · ${result.draft.overallConfidence}% avg confidence · model: ${result.model}`, kind: 'ai' });
    return { ...result, samplePrompts: SAMPLE_PROMPTS };
  });

  // ---- AI settings (Admin → model configuration) --------------------------
  app.get('/api/v1/settings/ai', async () => publicAISettings(d));

  app.put('/api/v1/settings/ai', async (req) => {
    const b = req.body as Partial<AIConfig>;
    const next = setAIConfig(d, b);
    const pub = publicAISettings(d);
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: 'AI model updated', detail: `${pub.activeLabel}`, kind: 'deploy' });
    return pub;
  });

  app.post('/api/v1/settings/ai/test', async (req) => {
    const b = req.body as Partial<AIConfig>;
    const cur = getAIConfig(d);
    const cfg: AIConfig = {
      provider: b.provider ?? cur.provider,
      baseURL: b.baseURL?.trim() || cur.baseURL,
      model: b.model?.trim() || cur.model,
      // For "test before save": if no key in the body, fall back to the stored one.
      apiKey: b.apiKey && b.apiKey.trim() ? b.apiKey.trim() : cur.apiKey,
    };
    return testConnection(cfg);
  });

  // ---- Workflows ----------------------------------------------------------
  app.get('/api/v1/workflows', async () => d.listWorkflows());

  app.post('/api/v1/workflows', async (req) => {
    const b = req.body as { name: string; description: string; prompt: string; steps: WorkflowStep[]; aiModel?: string };
    const wf: Workflow = {
      id: `wf-${uid()}`, name: b.name, description: b.description, prompt: b.prompt,
      status: 'draft', version: 1, steps: b.steps, createdBy: 'You',
      aiModel: b.aiModel ?? authoringModel(getAIConfig(d)), createdAt: 'just now', runs: 0,
    };
    d.upsertWorkflow(wf);
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: 'Workflow created', detail: `${wf.name} — draft saved (${wf.steps.length} steps)`, kind: 'deploy' });
    return wf;
  });

  app.get('/api/v1/workflows/:id', async (req, reply) => {
    const wf = d.getWorkflow((req.params as { id: string }).id);
    if (!wf) return reply.code(404).send({ error: 'workflow not found' });
    return wf;
  });

  app.patch('/api/v1/workflows/:id', async (req, reply) => {
    const { id } = req.params as { id: string };
    const cur = d.getWorkflow(id);
    if (!cur) return reply.code(404).send({ error: 'workflow not found' });
    const b = req.body as Partial<Pick<Workflow, 'name' | 'description' | 'steps' | 'version' | 'status'>>;
    d.patchWorkflow(id, b);
    return d.getWorkflow(id);
  });

  app.post('/api/v1/workflows/:id/approve', async (req, reply) => {
    const { id } = req.params as { id: string };
    const wf = d.getWorkflow(id);
    if (!wf) return reply.code(404).send({ error: 'workflow not found' });
    const next: Workflow = { ...wf, status: 'deployed', approvedBy: 'You (reviewer)' };
    d.upsertWorkflow(next);
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You (reviewer)', action: 'Approved & deployed', detail: `${wf.name} v${wf.version} — ${wf.steps.length} steps · AI draft by ${wf.aiModel}`, kind: 'approval' });
    return next;
  });

  app.get('/api/v1/workflows/:id/executions', async (req, reply) => {
    const { id } = req.params as { id: string };
    if (!d.getWorkflow(id)) return reply.code(404).send({ error: 'workflow not found' });
    return d.listInstances().filter((i) => i.workflowId === id);
  });

  app.post('/api/v1/workflows/:id/executions', async (req, reply) => {
    const { id } = req.params as { id: string };
    const wf = d.getWorkflow(id);
    if (!wf) return reply.code(404).send({ error: 'workflow not found' });
    if (wf.status === 'draft') return reply.code(400).send({ error: 'workflow must be approved before execution' });
    const { entity, input } = (req.body as { entity?: string; input?: Record<string, unknown> }) ?? {};
    const inst: Instance = {
      id: `run-${uid().slice(0, 4)}`, workflowId: id, workflowName: wf.name, status: 'running',
      entity: entity ?? `REC-${Math.floor(1000 + Math.random() * 9000)} · demo`,
      startedAt: new Date().toISOString(), currentStep: 0, input,
      stepRuns: wf.steps.map((s) => ({ stepId: s.id, name: s.name, type: s.type, status: 'pending' as const })),
    };
    d.upsertInstance(inst);
    d.patchWorkflow(id, { runs: wf.runs + 1 });
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: 'Execution started', detail: `${inst.id} · ${wf.name}`, kind: 'execution' });
    return inst;
  });

  // ---- Executions ---------------------------------------------------------
  app.get('/api/v1/executions', async () => d.listInstances());

  app.get('/api/v1/executions/:id', async (req, reply) => {
    const inst = d.getInstance((req.params as { id: string }).id);
    if (!inst) return reply.code(404).send({ error: 'execution not found' });
    return inst;
  });

  app.get('/api/v1/executions/:id/steps', async (req, reply) => {
    const inst = d.getInstance((req.params as { id: string }).id);
    if (!inst) return reply.code(404).send({ error: 'execution not found' });
    return inst.stepRuns;
  });

  app.post('/api/v1/executions/:id/approve', async (req, reply) => {
    const inst = approveWaiting(d, (req.params as { id: string }).id);
    if (!inst) return reply.code(404).send({ error: 'execution not found' });
    return inst;
  });

  app.post('/api/v1/executions/:id/retry', async (req, reply) => {
    const inst = retryFailed(d, (req.params as { id: string }).id);
    if (!inst) return reply.code(404).send({ error: 'execution not found' });
    return inst;
  });

  app.post('/api/v1/executions/:id/cancel', async (req, reply) => {
    const inst = cancelInstance(d, (req.params as { id: string }).id);
    if (!inst) return reply.code(404).send({ error: 'execution not found' });
    return inst;
  });

  // ---- MDM ----------------------------------------------------------------
  app.get('/api/v1/mdm', async () => d.listMDM());

  app.get('/api/v1/mdm/:entity', async (req, reply) => {
    const e = d.getMDM((req.params as { entity: string }).entity);
    if (!e) return reply.code(404).send({ error: 'entity not found' });
    return e;
  });

  app.post('/api/v1/mdm/:entity', async (req, reply) => {
    const key = (req.params as { entity: string }).entity;
    const e = d.getMDM(key);
    if (!e) return reply.code(404).send({ error: 'entity not found' });
    const rec: MDMRecord = (req.body as { record?: MDMRecord }).record ?? { id: '' };
    const id = rec.id || rec[e.fields[0]] || `X-${Math.floor(Math.random() * 9000 + 1000)}`;
    const record: MDMRecord = { ...rec, id, status: 'pending stewardship' };
    d.upsertMDM({ ...e, records: [record, ...e.records] });
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: 'MDM record created', detail: `${key}/${id} — pending stewardship`, kind: 'mdm' });
    return d.getMDM(key);
  });

  // ---- Controls -----------------------------------------------------------
  app.get('/api/v1/controls', async () => d.listControls());

  app.post('/api/v1/controls', async (req, reply) => {
    const b = req.body as ControlDef;
    const key = b.key?.trim();
    if (!/^[a-z][a-z0-9_.]*$/.test(key ?? '')) return reply.code(400).send({ error: 'key must be lowercase letters, numbers, dots or underscores' });
    if (d.listControls().some((c) => c.key === key)) return reply.code(400).send({ error: `control "${key}" already exists` });
    const ctrl: ControlDef = { key: key!, label: b.label?.trim() || key!, color: b.color || 'violet', icon: b.icon || 'Code', enabled: true, custom: true, description: b.description };
    d.upsertControl(ctrl);
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: 'Control created', detail: `${ctrl.label} (${ctrl.key}) — available in the palette`, kind: 'deploy' });
    return ctrl;
  });

  app.patch('/api/v1/controls/:key', async (req, reply) => {
    const { key } = req.params as { key: string };
    const cur = d.listControls().find((c) => c.key === key);
    if (!cur) return reply.code(404).send({ error: 'control not found' });
    const b = req.body as Partial<ControlDef>;
    const next: ControlDef = { ...cur, ...b, key: cur.key };
    d.upsertControl(next);
    return next;
  });

  app.post('/api/v1/controls/:key/toggle', async (req, reply) => {
    const { key } = req.params as { key: string };
    const cur = d.listControls().find((c) => c.key === key);
    if (!cur) return reply.code(404).send({ error: 'control not found' });
    const next: ControlDef = { ...cur, enabled: !cur.enabled };
    d.upsertControl(next);
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: next.enabled ? 'Control enabled' : 'Control disabled', detail: `${next.label} steps ${next.enabled ? 'available in the palette' : 'hidden from the palette; existing steps flagged'}`, kind: 'deploy' });
    return next;
  });

  app.delete('/api/v1/controls/:key', async (req, reply) => {
    const { key } = req.params as { key: string };
    const cur = d.listControls().find((c) => c.key === key);
    if (!cur) return reply.code(404).send({ error: 'control not found' });
    if (!cur.custom) return reply.code(400).send({ error: 'built-in controls cannot be removed' });
    const used = d.listWorkflows().reduce((n, w) => n + w.steps.filter((s) => s.type === key).length, 0);
    if (used > 0) return reply.code(400).send({ error: `control is used by ${used} step(s)` });
    d.deleteControl(key);
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: 'Control removed', detail: key, kind: 'deploy' });
    return { ok: true };
  });

  // ---- Audit --------------------------------------------------------------
  app.get('/api/v1/audit', async () => d.listAudit());

  app.post('/api/v1/audit', async (req) => {
    const b = req.body as Partial<AuditEntry>;
    const entry: AuditEntry = {
      id: uid(), at: new Date().toISOString(), actor: b.actor ?? 'You', action: b.action ?? 'Event',
      detail: b.detail ?? '', kind: b.kind ?? 'deploy',
    };
    d.addAudit(entry);
    return entry;
  });
}

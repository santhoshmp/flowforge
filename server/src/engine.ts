import { uid } from './util.js';
import type { DB } from './db.js';
import type { Instance, StepRun, WorkflowStep } from './types.js';

// ---------------------------------------------------------------------------
// Durable execution engine. A single scheduler tick advances every 'running'
// instance by ONE transition; each transition is persisted immediately so an
// execution survives a server restart and resumes exactly where it stopped.
// (This is the prototype stand-in for the Temporal interpreter in the design
// plan — same semantics, deliberately simple.)
// ---------------------------------------------------------------------------

// In-memory tick counters: how many ticks the current step has been 'running'
// before it resolves. Lost on restart; a running step simply re-runs. Demo-safe.
const runTicks = new Map<string, number>();
const TICK_MS = 850;

function durationTicks(type: string): number {
  switch (type) {
    case 'trigger':
    case 'condition':
      return 1;
    case 'ai.extract':
    case 'ai.classify':
      return 3;
    case 'human.approval':
      return 2;
    default:
      return 2;
  }
}

function stepOutput(cur: StepRun, wfStep: WorkflowStep | undefined, inst: Instance): string {
  switch (cur.type) {
    case 'trigger': return `${inst.entity ?? 'record'} received`;
    case 'ai.extract': return 'fields extracted · confidence 0.97';
    case 'ai.classify': return 'classified: standard';
    case 'mdm.validate': return 'matched golden record';
    case 'mdm.lookup': return 'entity resolved';
    case 'condition': return `${wfStep?.params.expression ?? 'condition'} → true`;
    case 'notify': return `sent via ${wfStep?.params.channel ?? 'email'}`;
    case 'integration.post': return `posted to ${wfStep?.params.system ?? 'target'} · 200 OK`;
    case 'integration.http': return 'HTTP 200';
    default: return 'done';
  }
}

// Advance one instance by a single transition. Returns the (mutated) instance.
function tickInstance(d: DB, inst: Instance): Instance {
  if (inst.status !== 'running') return inst;

  const wf = d.getWorkflow(inst.workflowId);
  const stepById = new Map((wf?.steps ?? []).map((s) => [s.id, s]));
  const stepRuns = inst.stepRuns.map((s) => ({ ...s }));

  const idx = stepRuns.findIndex((s) => s.status === 'running' || s.status === 'pending');
  if (idx === -1) {
    inst.status = 'completed';
    inst.currentStep = stepRuns.length;
    inst.endedAt = new Date().toISOString();
    d.upsertInstance(inst);
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'system', action: 'Instance completed', detail: `${inst.id} · ${inst.workflowName}`, kind: 'execution' });
    return inst;
  }

  const cur = stepRuns[idx];
  const wfStep = stepById.get(cur.stepId);

  // pending -> running (visible 'running' state for a tick)
  if (cur.status === 'pending') {
    cur.status = 'running';
    cur.startedAt = 'now';
    inst.currentStep = idx;
    inst.stepRuns = stepRuns;
    d.upsertInstance(inst);
    return inst;
  }

  // running -> maybe resolve
  const key = `${inst.id}:${cur.stepId}`;
  const ticks = (runTicks.get(key) ?? 0) + 1;
  runTicks.set(key, ticks);

  if (ticks < durationTicks(cur.type)) {
    inst.currentStep = idx;
    inst.stepRuns = stepRuns;
    d.upsertInstance(inst);
    return inst;
  }

  runTicks.delete(key);

  // Resolve the step. (SLA-breach escalation checks first so an escalation
  // human step skips instead of waiting when nothing breached.)
  if (wfStep?.params.condition === 'previous_step.sla_breached') {
    cur.status = 'skipped';
    cur.output = 'no SLA breach — skipped';
    cur.durationMs = 5;
  } else if (cur.type === 'human.approval') {
    // Auto-approve path: a prior condition evaluated false (below threshold).
    if (inst.autoApprove) {
      cur.status = 'succeeded';
      cur.durationMs = 1200;
      cur.output = 'auto-approved — condition below threshold';
      inst.autoApprove = false;
      inst.currentStep = idx;
      inst.stepRuns = stepRuns;
      d.upsertInstance(inst);
      return inst;
    }
    cur.status = 'waiting';
    const approver = wfStep?.params.approver ?? 'approver';
    const sla = wfStep?.params.sla_hours;
    cur.note = `Waiting on ${approver}${sla ? ` · SLA ${sla}h` : ''}`;
    inst.status = 'waiting';
    inst.waitingOn = approver;
    inst.currentStep = idx;
    inst.stepRuns = stepRuns;
    d.upsertInstance(inst);
    d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'system', action: 'Instance waiting', detail: `${inst.id} waiting on ${approver}`, kind: 'execution' });
    return inst;
  } else if (cur.type === 'condition' && wfStep?.params.expression) {
    // Evaluate "var > N" against the run input (e.g. total > 10000).
    const m = wfStep.params.expression.match(/([\w.]+)\s*>\s*([\d.]+)/);
    let result: boolean | null = null;
    if (m) {
      const val = Number(inst.input?.[m[1]] ?? (inst.input as Record<string, unknown> | undefined)?.total ?? NaN);
      const threshold = Number(m[2]);
      if (Number.isFinite(val)) result = val > threshold;
    }
    cur.status = 'succeeded';
    cur.durationMs = 18;
    if (result === false) {
      cur.output = `${wfStep.params.expression} → false · auto-approve path`;
      inst.autoApprove = true;
    } else {
      cur.output = `${wfStep.params.expression} → true`;
    }
  } else {
    cur.status = 'succeeded';
    cur.durationMs = cur.type.startsWith('ai.') ? 2100 : 120 + idx * 60;
    cur.output = stepOutput(cur, wfStep, inst);
  }

  inst.currentStep = idx;
  inst.stepRuns = stepRuns;
  d.upsertInstance(inst);
  return inst;
}

// Advance every running instance by exactly one transition. The scheduler
// calls this on an interval; tests call it directly for determinism.
export function tickAll(d: DB): void {
  for (const inst of d.listInstances()) {
    if (inst.status === 'running') tickInstance(d, inst);
  }
}

export function startScheduler(d: DB): () => void {
  const handle = setInterval(() => tickAll(d), TICK_MS);
  return () => clearInterval(handle);
}

// ---- Execution control (called by routes) ----------------------------------

export function approveWaiting(d: DB, instanceId: string): Instance | undefined {
  const inst = d.getInstance(instanceId);
  if (!inst || inst.status !== 'waiting') return inst;
  const stepRuns = inst.stepRuns.map((s) => ({ ...s }));
  const cur = stepRuns[inst.currentStep];
  if (cur && cur.status === 'waiting') {
    cur.status = 'succeeded';
    cur.durationMs = 420000;
    cur.output = `approved by ${inst.waitingOn ?? 'approver'} (simulated)`;
    cur.note = undefined;
  }
  inst.stepRuns = stepRuns;
  inst.status = 'running';
  inst.waitingOn = undefined;
  d.upsertInstance(inst);
  d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: 'Human task approved', detail: `${instanceId} — execution resumed`, kind: 'execution' });
  return inst;
}

export function retryFailed(d: DB, instanceId: string): Instance | undefined {
  const inst = d.getInstance(instanceId);
  if (!inst || inst.status !== 'failed') return inst;
  const stepRuns = inst.stepRuns.map((s) => ({ ...s }));
  const failed = stepRuns[inst.currentStep];
  if (failed && failed.status === 'failed') {
    failed.status = 'pending';
    failed.note = undefined;
  }
  inst.stepRuns = stepRuns;
  inst.status = 'running';
  inst.error = undefined;
  inst.endedAt = undefined;
  d.upsertInstance(inst);
  d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: 'Retried from failed step', detail: `${instanceId} — resume without re-running completed steps`, kind: 'execution' });
  return inst;
}

export function cancelInstance(d: DB, instanceId: string): Instance | undefined {
  const inst = d.getInstance(instanceId);
  if (!inst) return inst;
  inst.status = 'cancelled';
  inst.endedAt = new Date().toISOString();
  d.upsertInstance(inst);
  d.addAudit({ id: uid(), at: new Date().toISOString(), actor: 'You', action: 'Instance cancelled', detail: instanceId, kind: 'execution' });
  return inst;
}

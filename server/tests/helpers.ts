import { openDB, type DB } from '../src/db.js';
import { tickAll } from '../src/engine.js';
import type { Instance, StepStatus, Workflow, WorkflowStep } from '../src/types.js';

// Shared test helpers: in-memory DB, a deterministic sample workflow, a fresh
// running instance, and a synchronous "drive the engine N ticks" helper.

export function memDB(): DB {
  return openDB(':memory:');
}

let counter = 0;
const uid = () => `t${++counter}${Math.random().toString(36).slice(2, 5)}`;

export const sampleSteps: WorkflowStep[] = [
  { id: 'trig', type: 'trigger', name: 'Record received', params: { event: 'record.created' }, confidence: 95, assumptions: [] },
  { id: 'cond', type: 'condition', name: 'Amount > 100?', params: { expression: 'total > 100', on_false: 'auto_approve' }, confidence: 90, assumptions: [] },
  { id: 'mgr', type: 'human.approval', name: 'Manager approval', params: { approver: 'Manager', sla_hours: '24' }, confidence: 85, assumptions: [] },
  { id: 'esc', type: 'human.approval', name: 'Escalation', params: { approver: 'VP', condition: 'previous_step.sla_breached' }, confidence: 70, assumptions: [] },
  { id: 'post', type: 'integration.post', name: 'Post to ERP', params: { system: 'ERP', endpoint: 'erp.in' }, confidence: 80, assumptions: [] },
];

export function sampleWorkflow(overrides: Partial<Workflow> = {}): Workflow {
  return {
    id: 'wf-test', name: 'Test Workflow', description: 'd', prompt: 'p', status: 'deployed',
    version: 1, steps: sampleSteps, createdBy: 'tester', aiModel: 'test',
    createdAt: new Date().toISOString(), runs: 0, ...overrides,
  };
}

export function newRun(wf: Workflow, input?: Record<string, unknown>): Instance {
  return {
    id: `run-${uid()}`, workflowId: wf.id, workflowName: wf.name, status: 'running',
    entity: 'REC-1', startedAt: new Date().toISOString(), input,
    stepRuns: wf.steps.map((s) => ({ stepId: s.id, name: s.name, type: s.type, status: 'pending' as StepStatus })),
    currentStep: 0,
  };
}

// Advance the engine until `until` is true (or max ticks). Synchronous — the
// engine has no timers in tests (scheduler disabled).
export function drive(d: DB, instanceId: string, until: (i?: Instance) => boolean, max = 40): Instance | undefined {
  for (let i = 0; i < max; i++) {
    tickAll(d);
    const inst = d.getInstance(instanceId);
    if (until(inst)) return inst;
  }
  return d.getInstance(instanceId);
}

export function drain(d: DB, instanceId: string, max = 40): Instance | undefined {
  return drive(d, instanceId, (i) => i?.status === 'completed' || i?.status === 'failed' || i?.status === 'waiting', max);
}

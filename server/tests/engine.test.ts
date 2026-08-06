import { describe, it, expect } from 'vitest';
import { memDB, sampleWorkflow, newRun, drain, drive } from './helpers.js';
import { tickAll, approveWaiting, retryFailed, cancelInstance } from '../src/engine.js';
import type { StepStatus } from '../src/types.js';

// Scenario IDs: ENG-01..ENG-05 (see docs/test-strategy.md).
// Maps to traceability feature F-EXEC-*.

function fresh() {
  const d = memDB();
  d.upsertWorkflow(sampleWorkflow());
  return { d, wf: sampleWorkflow() };
}

describe('engine: execution lifecycle (ENG)', () => {
  it('ENG-01 runs to a human approval and waits when above threshold', () => {
    const { d, wf } = fresh();
    const inst = newRun(wf, { total: 500 }); // > 100 → approval path
    d.upsertInstance(inst);
    const cur = drain(d, inst.id);
    expect(cur?.status).toBe('waiting');
    expect(cur?.waitingOn).toBe('Manager');
    const mgr = cur?.stepRuns.find((s) => s.stepId === 'mgr');
    expect(mgr?.status).toBe('waiting');
  });

  it('ENG-02 completes after a waiting task is approved; escalation skips', () => {
    const { d, wf } = fresh();
    const inst = newRun(wf, { total: 500 });
    d.upsertInstance(inst);
    drive(d, inst.id, (i) => i?.status === 'waiting');
    approveWaiting(d, inst.id);
    const final = drain(d, inst.id);
    expect(final?.status).toBe('completed');
    expect(final?.stepRuns.find((s) => s.stepId === 'esc')?.status).toBe('skipped');
    expect(final?.stepRuns.find((s) => s.stepId === 'post')?.status).toBe('succeeded');
    expect(final?.endedAt).toBeTruthy();
  });

  it('ENG-03 auto-approves the manager step when the condition is below threshold', () => {
    const { d, wf } = fresh();
    const inst = newRun(wf, { total: 10 }); // <= 100 → auto-approve path
    d.upsertInstance(inst);
    const final = drain(d, inst.id);
    expect(final?.status).toBe('completed'); // never waited
    const mgr = final?.stepRuns.find((s) => s.stepId === 'mgr');
    expect(mgr?.status).toBe('succeeded');
    expect(mgr?.output).toMatch(/auto-approved/);
    const cond = final?.stepRuns.find((s) => s.stepId === 'cond');
    expect(cond?.output).toMatch(/false/);
  });

  it('ENG-04 retries from the failed step without re-running completed steps', () => {
    const { d, wf } = fresh();
    const inst = newRun(wf, { total: 500 });
    // Force a failure at the 'post' step (index 4), prior steps succeeded.
    inst.status = 'failed';
    inst.error = 'boom';
    inst.currentStep = 4;
    inst.stepRuns = wf.steps.map((s, i) => {
      const status: StepStatus = i < 4 ? 'succeeded' : i === 4 ? 'failed' : 'pending';
      return { stepId: s.id, name: s.name, type: s.type, status };
    });
    d.upsertInstance(inst);
    retryFailed(d, inst.id);
    const after = d.getInstance(inst.id);
    expect(after?.status).toBe('running');
    expect(after?.error).toBeUndefined();
    expect(after?.stepRuns[0].status).toBe('succeeded'); // completed steps preserved
    const final = drain(d, inst.id);
    expect(final?.status).toBe('completed');
  });

  it('ENG-05 cancels a running instance and records endedAt', () => {
    const { d, wf } = fresh();
    const inst = newRun(wf, { total: 500 });
    d.upsertInstance(inst);
    tickAll(d); // move it off pure pending
    cancelInstance(d, inst.id);
    const after = d.getInstance(inst.id);
    expect(after?.status).toBe('cancelled');
    expect(after?.endedAt).toBeTruthy();
  });
});

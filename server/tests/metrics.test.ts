import { describe, it, expect } from 'vitest';
import { memDB, sampleWorkflow, newRun, drain, drive } from './helpers.js';
import { computeMetrics } from '../src/metrics.js';

// Scenario IDs: MET-01 (see docs/test-strategy.md). Feature F-DASH.

describe('metrics: tracking dashboard (MET)', () => {
  it('MET-01 reports fleet counts, 14-day series, and per-workflow breakdown', () => {
    const d = memDB();
    const wf = sampleWorkflow();
    d.upsertWorkflow(wf);
    // 2 below threshold (auto-approve → completed), 1 above (waits)
    d.upsertInstance(newRun(wf, { total: 10 }));
    d.upsertInstance(newRun(wf, { total: 10 }));
    d.upsertInstance(newRun(wf, { total: 500 }));
    // Drive them to terminal/waiting states.
    drain(d, d.listInstances()[0].id);
    drain(d, d.listInstances()[1].id);
    drive(d, d.listInstances()[2].id, (i) => i?.status === 'waiting');

    const m = computeMetrics(d);
    expect(m.fleet.totalRuns).toBe(3);
    expect(m.fleet.completed).toBe(2);
    expect(m.fleet.waiting).toBe(1);
    expect(m.fleet.successRate).toBeGreaterThan(0);
    expect(m.byDay).toHaveLength(14);
    expect(m.workflows[0].id).toBe('wf-test');
    expect(m.workflows[0].runs).toBe(3);
    expect(m.workflows[0].completed).toBe(2);
    expect(m.workflows[0].waiting).toBe(1);
  });
});

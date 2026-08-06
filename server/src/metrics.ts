import type { DB } from './db.js';
import type { InstanceStatus } from './types.js';

// ---------------------------------------------------------------------------
// Aggregate metrics for the tracking dashboard: fleet KPIs, a 14-day
// timeseries, status mix, and per-workflow breakdowns. Computed from live
// instance data on each request.
// ---------------------------------------------------------------------------

const DAY_MS = 86400000;

function instanceDurationMs(inst: { startedAt: string; endedAt?: string; stepRuns: { durationMs?: number }[] }): number | undefined {
  if (inst.endedAt) {
    const ms = new Date(inst.endedAt).getTime() - new Date(inst.startedAt).getTime();
    if (Number.isFinite(ms) && ms >= 0) return ms;
  }
  const sum = inst.stepRuns.reduce((a, s) => a + (s.durationMs ?? 0), 0);
  return sum > 0 ? sum : undefined;
}

export interface WorkflowMetric {
  id: string;
  name: string;
  status: string;
  runs: number;
  completed: number;
  failed: number;
  running: number;
  waiting: number;
  cancelled: number;
  successRate: number | null;
  avgDurationMs: number | null;
  lastRunIso: string | null;
}

export interface DayBucket {
  date: string;
  completed: number;
  failed: number;
  running: number;
  waiting: number;
  cancelled: number;
  total: number;
}

export interface Metrics {
  fleet: {
    workflows: number;
    deployed: number;
    totalRuns: number;
    running: number;
    waiting: number;
    failed: number;
    completed: number;
    cancelled: number;
    successRate: number | null;
    avgDurationMs: number | null;
    humanTasksPending: number;
  };
  byDay: DayBucket[];
  statusMix: { status: string; count: number }[];
  workflows: WorkflowMetric[];
}

export function computeMetrics(d: DB): Metrics {
  const instances = d.listInstances();
  const workflows = d.listWorkflows();

  const count = (s: InstanceStatus) => instances.filter((i) => i.status === s).length;
  const completed = count('completed');
  const failed = count('failed');
  const running = count('running');
  const waiting = count('waiting');
  const cancelled = count('cancelled');

  const completedInst = instances.filter((i) => i.status === 'completed');
  const durations = completedInst.map(instanceDurationMs).filter((x): x is number => x != null);
  const avgDurationMs = durations.length ? Math.round(durations.reduce((a, b) => a + b, 0) / durations.length) : null;
  const successRate = completed + failed > 0 ? Math.round((completed / (completed + failed)) * 1000) / 10 : null;

  // Status mix (exclude nothing; show all five).
  const statusMix: { status: string; count: number }[] = (['completed', 'failed', 'running', 'waiting', 'cancelled'] as InstanceStatus[]).map((s) => ({ status: s, count: count(s) }));

  // 14-day timeseries bucketed by startedAt day.
  const byDayMap = new Map<string, DayBucket>();
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  for (let i = 13; i >= 0; i--) {
    const dt = new Date(today.getTime() - i * DAY_MS);
    const key = dt.toISOString().slice(0, 10);
    byDayMap.set(key, { date: key, completed: 0, failed: 0, running: 0, waiting: 0, cancelled: 0, total: 0 });
  }
  for (const inst of instances) {
    const key = inst.startedAt.slice(0, 10);
    const bucket = byDayMap.get(key);
    if (!bucket) continue;
    if (inst.status === 'completed') bucket.completed += 1;
    else if (inst.status === 'failed') bucket.failed += 1;
    else if (inst.status === 'running') bucket.running += 1;
    else if (inst.status === 'waiting') bucket.waiting += 1;
    else if (inst.status === 'cancelled') bucket.cancelled += 1;
    bucket.total += 1;
  }

  // Per-workflow breakdown.
  const wfMetrics: WorkflowMetric[] = workflows.map((w) => {
    const runs = instances.filter((i) => i.workflowId === w.id);
    const c = runs.filter((i) => i.status === 'completed').length;
    const f = runs.filter((i) => i.status === 'failed').length;
    const wfDurations = runs.filter((i) => i.status === 'completed').map(instanceDurationMs).filter((x): x is number => x != null);
    const lastRun = runs.map((i) => i.startedAt).sort()[runs.length - 1] ?? null;
    return {
      id: w.id, name: w.name, status: w.status, runs: runs.length,
      completed: c, failed: f, running: runs.filter((i) => i.status === 'running').length,
      waiting: runs.filter((i) => i.status === 'waiting').length, cancelled: runs.filter((i) => i.status === 'cancelled').length,
      successRate: c + f > 0 ? Math.round((c / (c + f)) * 1000) / 10 : null,
      avgDurationMs: wfDurations.length ? Math.round(wfDurations.reduce((a, b) => a + b, 0) / wfDurations.length) : null,
      lastRunIso: lastRun,
    };
  });

  return {
    fleet: {
      workflows: workflows.length,
      deployed: workflows.filter((w) => w.status === 'deployed').length,
      totalRuns: instances.length,
      running, waiting, failed, completed, cancelled,
      successRate, avgDurationMs, humanTasksPending: waiting,
    },
    byDay: Array.from(byDayMap.values()),
    statusMix,
    workflows: wfMetrics,
  };
}

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ResponsiveContainer, AreaChart, Area, XAxis, YAxis, Tooltip, CartesianGrid,
  PieChart, Pie, Cell, BarChart, Bar,
} from 'recharts';
import { Play, Activity, CheckCircle2, XCircle, Clock, Timer, Gauge, RotateCcw, Ban, UserCheck, Layers, Code2, Zap } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { api, type Metrics, type WorkflowMetric } from '@/lib/api';
import { useStore, fmtDur } from '@/lib/store';
import { StatusPill } from '@/components/step';
import { toast } from 'sonner';
import { cn, timeAgo } from '@/lib/utils';
import type { Instance, StepRun } from '@/lib/types';

// Tracking dashboard: fleet KPIs, execution trends, outcome mix, and a
// per-workflow tracker with live executions and invoke/action controls.

const SERIES: Record<string, string> = {
  completed: '#10b981', failed: '#f43f5e', waiting: '#f59e0b', running: '#6366f1', cancelled: '#94a3b8',
};
const PIE = [SERIES.completed, SERIES.failed, SERIES.running, SERIES.waiting, SERIES.cancelled];

interface StepPerf { name: string; type: string; avgMs: number; count: number; p95Ms: number }

function computeStepPerf(insts: Instance[]): StepPerf[] {
  const buckets = new Map<string, { name: string; type: string; durs: number[] }>();
  for (const inst of insts) {
    if (inst.status !== 'completed') continue;
    for (const sr of inst.stepRuns) {
      if (sr.durationMs == null) continue;
      const k = sr.stepId;
      const b = buckets.get(k) ?? { name: sr.name, type: sr.type, durs: [] };
      b.durs.push(sr.durationMs);
      buckets.set(k, b);
    }
  }
  return [...buckets.values()].map((b) => {
    const durs = b.durs.sort((a, z) => a - z);
    const avg = Math.round(durs.reduce((s, d) => s + d, 0) / durs.length);
    const p95 = durs[Math.min(durs.length - 1, Math.floor(durs.length * 0.95))];
    return { name: b.name, type: b.type, avgMs: avg, count: durs.length, p95Ms: p95 };
  });
}

function KPI({ icon: Icon, label, value, sub, tone }: { icon: typeof Activity; label: string; value: string; sub?: string; tone: string }) {
  return (
    <div className="rounded-xl border bg-card p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <Icon className={cn('h-4 w-4', tone)} />
        <span className="text-xs font-medium text-muted-foreground">{label}</span>
      </div>
      <div className="mt-2 text-2xl font-bold">{value}</div>
      {sub && <div className="text-[11px] text-muted-foreground mt-0.5">{sub}</div>}
    </div>
  );
}

export default function Dashboard({ onGoMonitor }: { onGoMonitor: () => void }) {
  const { instances, workflows, runWorkflow, approveTask, retryInstance, cancelInstance } = useStore();
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [metricsErr, setMetricsErr] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [totalInput, setTotalInput] = useState('24000');

  const loadMetrics = useCallback(async () => {
    try { setMetrics(await api.metrics()); setMetricsErr(null); }
    catch (e) { setMetricsErr(String((e as Error).message ?? e)); }
  }, []);
  useEffect(() => { loadMetrics(); const t = setInterval(loadMetrics, 3000); return () => clearInterval(t); }, [loadMetrics]);

  const selWf = workflows.find((w) => w.id === selectedId) ?? null;
  const selInstances = useMemo(
    () => (selectedId ? instances.filter((i) => i.workflowId === selectedId).sort((a, b) => b.startedAt.localeCompare(a.startedAt)) : []),
    [instances, selectedId],
  );
  const stepPerf = useMemo(() => computeStepPerf(selInstances), [selInstances]);
  const waitingForWf = (w: WorkflowMetric) => instances.filter((i) => i.workflowId === w.id && i.status === 'waiting').length;

  const startRun = async (id: string, withInput = false) => {
    const input = withInput && totalInput.trim() ? { total: Number(totalInput) } : undefined;
    try {
      await runWorkflow(id, undefined, input);
      toast.success('Execution started');
    } catch (e) {
      toast.error('Could not start', { description: String((e as Error).message ?? e) });
    }
  };

  const f = metrics?.fleet;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Tracking dashboard</h1>
          <p className="text-sm text-muted-foreground mt-1">Track every workflow — throughput, outcomes, bottlenecks, and live runs. Invoke and act from here.</p>
        </div>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" className="gap-1.5" onClick={loadMetrics}><Activity className="h-3.5 w-3.5" /> Refresh</Button>
        </div>
      </div>

      {metricsErr && (
        <div className="rounded-lg border border-rose-200 bg-rose-50 px-4 py-2.5 text-sm text-rose-700">
          Couldn't load dashboard data: {metricsErr}
        </div>
      )}
      {!metrics && !metricsErr && (
        <div className="rounded-lg border bg-card px-4 py-3 text-sm text-muted-foreground">Loading dashboard data…</div>
      )}

      {/* KPIs */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-6">
        <KPI icon={Layers} label="Total runs" value={f ? String(f.totalRuns) : '—'} sub={`${f?.deployed ?? 0} deployed workflows`} tone="text-violet-600" />
        <KPI icon={CheckCircle2} label="Success rate" value={f?.successRate != null ? `${f.successRate}%` : '—'} sub={`${f?.completed ?? 0} completed`} tone="text-emerald-600" />
        <KPI icon={Activity} label="Running" value={f ? String(f.running) : '—'} tone="text-indigo-600" />
        <KPI icon={Clock} label="Waiting (human)" value={f ? String(f.waiting) : '—'} sub="pending approvals" tone="text-amber-600" />
        <KPI icon={XCircle} label="Failed" value={f ? String(f.failed) : '—'} tone="text-rose-600" />
        <KPI icon={Timer} label="Avg duration" value={f?.avgDurationMs ? fmtDur(f.avgDurationMs) : '—'} sub="per completed run" tone="text-sky-600" />
      </div>

      {/* Charts */}
      <div className="grid gap-4 lg:grid-cols-3">
        <div className="rounded-xl border bg-card p-5 shadow-sm lg:col-span-2">
          <div className="flex items-center gap-2 mb-3">
            <Gauge className="h-4 w-4 text-violet-600" />
            <span className="text-sm font-semibold">Execution trends</span>
            <span className="text-[11px] text-muted-foreground">last 14 days</span>
          </div>
          <div className="h-[240px]">
            {metrics && (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={metrics.byDay} margin={{ top: 4, right: 8, left: -16, bottom: 0 }}>
                  <defs>
                    {(['completed', 'failed', 'waiting'] as const).map((k) => (
                      <linearGradient key={k} id={`g-${k}`} x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor={SERIES[k]} stopOpacity={0.5} />
                        <stop offset="95%" stopColor={SERIES[k]} stopOpacity={0.05} />
                      </linearGradient>
                    ))}
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-slate-100" vertical={false} />
                  <XAxis dataKey="date" tickFormatter={(v: string) => v.slice(5)} fontSize={10} tickLine={false} axisLine={false} />
                  <YAxis allowDecimals={false} fontSize={10} tickLine={false} axisLine={false} />
                  <Tooltip contentStyle={{ borderRadius: 8, fontSize: 11, border: '1px solid #e2e8f0' }} labelFormatter={(v: any) => v} />
                  <Area type="monotone" dataKey="completed" stroke={SERIES.completed} fill="url(#g-completed)" strokeWidth={2} />
                  <Area type="monotone" dataKey="failed" stroke={SERIES.failed} fill="url(#g-failed)" strokeWidth={2} />
                  <Area type="monotone" dataKey="waiting" stroke={SERIES.waiting} fill="url(#g-waiting)" strokeWidth={2} />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </div>
        </div>

        <div className="rounded-xl border bg-card p-5 shadow-sm">
          <div className="flex items-center gap-2 mb-3">
            <Zap className="h-4 w-4 text-violet-600" />
            <span className="text-sm font-semibold">Outcome mix</span>
          </div>
          <div className="h-[240px]">
            {metrics && (
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie data={metrics.statusMix.filter((s) => s.count > 0)} dataKey="count" nameKey="status" cx="50%" cy="50%" innerRadius={45} outerRadius={80} paddingAngle={2}>
                    {metrics.statusMix.filter((s) => s.count > 0).map((_, i) => <Cell key={i} fill={PIE[i % PIE.length]} />)}
                  </Pie>
                  <Tooltip contentStyle={{ borderRadius: 8, fontSize: 11, border: '1px solid #e2e8f0' }} />
                </PieChart>
              </ResponsiveContainer>
            )}
          </div>
          <div className="flex flex-wrap justify-center gap-x-3 gap-y-1 text-[10px] text-muted-foreground">
            {metrics?.statusMix.filter((s) => s.count > 0).map((s, i) => (
              <span key={s.status} className="flex items-center gap-1"><span className="h-2 w-2 rounded-full" style={{ background: PIE[i % PIE.length] }} />{s.status} {s.count}</span>
            ))}
          </div>
        </div>
      </div>

      {/* Workflow tracker */}
      <div className="rounded-xl border bg-card shadow-sm overflow-hidden">
        <div className="flex items-center gap-2 border-b px-5 py-3">
          <Layers className="h-4 w-4 text-violet-600" />
          <span className="text-sm font-semibold">Workflow tracker</span>
          <span className="text-[11px] text-muted-foreground">click a workflow to track its runs in detail</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-[11px] uppercase tracking-wide text-muted-foreground">
                <th className="px-5 py-2 font-medium">Workflow</th>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-3 py-2 font-medium text-right">Runs</th>
                <th className="px-3 py-2 font-medium">Success</th>
                <th className="px-3 py-2 font-medium text-right">Avg duration</th>
                <th className="px-3 py-2 font-medium">Last run</th>
                <th className="px-5 py-2 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {metrics?.workflows.map((w) => {
                const waiting = waitingForWf(w);
                return (
                  <tr key={w.id} className={cn('border-t hover:bg-muted/40 cursor-pointer', selectedId === w.id && 'bg-violet-50/50')} onClick={() => setSelectedId(selectedId === w.id ? null : w.id)}>
                    <td className="px-5 py-3 font-medium">{w.name}</td>
                    <td className="px-3 py-3"><StatusPill status={w.status} /></td>
                    <td className="px-3 py-3 text-right tabular-nums">{w.runs}</td>
                    <td className="px-3 py-3">
                      <div className="flex items-center gap-2">
                        <div className="h-1.5 w-20 overflow-hidden rounded-full bg-slate-100">
                          <div className={cn('h-full rounded-full', (w.successRate ?? 0) >= 90 ? 'bg-emerald-500' : (w.successRate ?? 0) >= 75 ? 'bg-amber-500' : 'bg-rose-500')} style={{ width: `${w.successRate ?? 0}%` }} />
                        </div>
                        <span className="text-[11px] tabular-nums text-muted-foreground">{w.successRate != null ? `${w.successRate}%` : '—'}</span>
                      </div>
                    </td>
                    <td className="px-3 py-3 text-right tabular-nums text-muted-foreground">{w.avgDurationMs ? fmtDur(w.avgDurationMs) : '—'}</td>
                    <td className="px-3 py-3 text-muted-foreground">{timeAgo(w.lastRunIso)}</td>
                    <td className="px-5 py-3 text-right" onClick={(e) => e.stopPropagation()}>
                      <div className="flex items-center justify-end gap-1.5">
                        {waiting > 0 && <Badge className="bg-amber-100 text-amber-700 border-amber-200 text-[10px]">{waiting} waiting</Badge>}
                        <Button size="sm" variant="outline" className="h-7 gap-1 text-xs" disabled={w.status === 'draft'} onClick={() => startRun(w.id)}>
                          <Play className="h-3 w-3" /> Run
                        </Button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* Drill-down */}
      {selWf && (
        <div className="rounded-xl border bg-card p-5 shadow-sm space-y-4">
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-sm font-semibold">{selWf.name}</span>
            <StatusPill status={selWf.status} />
            <span className="text-[11px] text-muted-foreground">{selWf.steps.length} steps</span>
            <div className="ml-auto flex items-center gap-2">
              <div className="flex items-center gap-1.5 rounded-lg border bg-muted/40 px-2 py-1">
                <span className="text-[10px] text-muted-foreground">total $</span>
                <Input value={totalInput} onChange={(e) => setTotalInput(e.target.value)} className="h-6 w-20 border-0 bg-transparent px-1 text-xs tabular-nums focus-visible:ring-0" />
              </div>
              <Button size="sm" variant="outline" className="h-7 gap-1 text-xs" disabled={selWf.status === 'draft'} onClick={() => startRun(selWf.id, true)}>
                <Play className="h-3 w-3" /> Run with input
              </Button>
              <Button size="sm" variant="ghost" className="h-7 gap-1 text-xs" onClick={onGoMonitor}><Code2 className="h-3 w-3" /> Executions</Button>
            </div>
          </div>
          <div className="rounded-lg bg-muted/50 border px-3 py-2 font-mono text-[11px] text-muted-foreground flex items-center gap-2">
            <Code2 className="h-3.5 w-3.5 shrink-0" />
            POST /api/v1/workflows/{selWf.id}/executions  <span className="text-slate-400">— actions: /executions/{'{id}'}/approve · /retry · /cancel</span>
          </div>

          <div className="grid gap-4 lg:grid-cols-5">
            {/* Recent executions */}
            <div className="lg:col-span-3">
              <div className="text-xs font-semibold text-muted-foreground mb-2">Recent executions ({selInstances.length})</div>
              <div className="space-y-1.5 max-h-[320px] overflow-y-auto pr-1">
                {selInstances.length === 0 && <div className="rounded-lg border border-dashed p-4 text-center text-xs text-muted-foreground">No executions yet — hit Run.</div>}
                {selInstances.slice(0, 30).map((i) => (
                  <ExecRow key={i.id} inst={i}
                    onApprove={async () => { await approveTask(i.id); toast.success('Approved — resumed'); }}
                    onRetry={async () => { await retryInstance(i.id); toast.success('Retrying from failed step'); }}
                    onCancel={async () => { await cancelInstance(i.id); toast.success('Cancelled'); }}
                  />
                ))}
              </div>
            </div>

            {/* Step performance */}
            <div className="lg:col-span-2">
              <div className="text-xs font-semibold text-muted-foreground mb-2">Step performance (completed runs)</div>
              {stepPerf.length === 0 ? (
                <div className="rounded-lg border border-dashed p-4 text-center text-xs text-muted-foreground">No completed runs yet.</div>
              ) : (
                <div className="h-[300px]">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={stepPerf} layout="vertical" margin={{ top: 0, right: 16, left: 0, bottom: 0 }}>
                      <CartesianGrid strokeDasharray="3 3" className="stroke-slate-100" horizontal={false} />
                      <XAxis type="number" fontSize={10} tickLine={false} axisLine={false} tickFormatter={(v: number) => fmtDur(v)} />
                      <YAxis type="category" dataKey="name" fontSize={10} tickLine={false} axisLine={false} width={110} />
                      <Tooltip contentStyle={{ borderRadius: 8, fontSize: 11, border: '1px solid #e2e8f0' }} formatter={(v: any) => fmtDur(Number(v))} />
                      <Bar dataKey="avgMs" name="avg" fill="#8b5cf6" radius={[0, 4, 4, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function ExecRow({ inst, onApprove, onRetry, onCancel }: { inst: Instance; onApprove: () => Promise<void>; onRetry: () => Promise<void>; onCancel: () => Promise<void> }) {
  return (
    <div className="flex items-center gap-3 rounded-lg border px-3 py-2">
      <StepStatusDot status={inst.status} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="font-mono text-[11px] text-muted-foreground">{inst.id}</span>
          <StatusPill status={inst.status} />
        </div>
        <div className="truncate text-[11px] text-muted-foreground">{inst.entity} · {timeAgo(inst.startedAt)}{inst.error ? ` · ${inst.error}` : ''}</div>
      </div>
      <div className="flex items-center gap-1">
        {inst.status === 'waiting' && <Button size="sm" variant="outline" className="h-7 gap-1 text-xs border-amber-300 text-amber-800" onClick={onApprove}><UserCheck className="h-3 w-3" /> Approve</Button>}
        {inst.status === 'failed' && <Button size="sm" variant="outline" className="h-7 gap-1 text-xs" onClick={onRetry}><RotateCcw className="h-3 w-3" /> Retry</Button>}
        {(inst.status === 'running' || inst.status === 'waiting') && <Button size="sm" variant="ghost" className="h-7 gap-1 text-xs text-rose-600" onClick={onCancel}><Ban className="h-3 w-3" /></Button>}
      </div>
    </div>
  );
}

function StepStatusDot({ status }: { status: StepRun['status'] | Instance['status'] }) {
  const map: Record<string, string> = {
    running: 'bg-indigo-500', waiting: 'bg-amber-500', failed: 'bg-rose-500', completed: 'bg-emerald-500', cancelled: 'bg-slate-400', succeeded: 'bg-emerald-500', skipped: 'bg-slate-300', pending: 'bg-slate-200',
  };
  return <span className={cn('h-2 w-2 shrink-0 rounded-full', map[status] ?? 'bg-slate-300')} />;
}

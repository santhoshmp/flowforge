import { useState } from 'react';
import { Play, UserCheck, RotateCcw, Ban, ArrowDown } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { useStore, fmtDur } from '@/lib/store';
import { StepIcon, StepStatusIcon, StatusPill } from '@/components/step';
import { toast } from 'sonner';
import { cn, timeAgo } from '@/lib/utils';
import type { Instance } from '@/lib/types';

export default function Monitor() {
  const { instances, workflows, runWorkflow, approveTask, retryInstance, cancelInstance } = useStore();
  const [selected, setSelected] = useState<Instance | null>(instances[0] ?? null);
  const sel = instances.find((i) => i.id === selected?.id) ?? instances[0];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Executions</h1>
          <p className="text-sm text-muted-foreground mt-1">Every step of every run — live. No black boxes.</p>
        </div>
        <div className="flex gap-2">
          {workflows.filter((w) => w.status === 'deployed').map((w) => (
            <Button key={w.id} size="sm" variant="outline" className="gap-1.5" onClick={async () => {
              const id = await runWorkflow(w.id);
              setSelected(null);
              setTimeout(() => setSelected({ id } as Instance), 50);
              toast.success(`Started ${w.name}`);
            }}>
              <Play className="h-3.5 w-3.5" /> Run: {w.name}
            </Button>
          ))}
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-5">
        {/* Instance list */}
        <div className="lg:col-span-2 space-y-2">
          {instances.map((inst) => (
            <button
              key={inst.id}
              onClick={() => setSelected(inst)}
              className={cn(
                'w-full rounded-lg border bg-card p-3 text-left transition-colors hover:border-indigo-300',
                sel?.id === inst.id && 'border-indigo-400 ring-1 ring-indigo-200',
              )}
            >
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono text-xs text-muted-foreground">{inst.id}</span>
                <StatusPill status={inst.status} />
              </div>
              <div className="mt-1 font-medium text-sm">{inst.workflowName}</div>
              <div className="mt-0.5 flex items-center justify-between text-[11px] text-muted-foreground">
                <span className="truncate">{inst.entity}</span>
                <span className="shrink-0 ml-2">{timeAgo(inst.startedAt)}</span>
              </div>
              {inst.status === 'waiting' && (
                <div className="mt-1.5 text-[11px] text-amber-700">⏳ waiting on {inst.waitingOn}</div>
              )}
            </button>
          ))}
        </div>

        {/* Timeline */}
        <div className="lg:col-span-3">
          {sel ? (
            <div className="rounded-xl border bg-card p-5 shadow-sm">
              <div className="flex flex-wrap items-center gap-3 border-b pb-3">
                <div>
                  <div className="font-semibold text-sm">{sel.workflowName}</div>
                  <div className="text-[11px] text-muted-foreground font-mono">{sel.id} · {sel.entity} · started {timeAgo(sel.startedAt)}</div>
                </div>
                <StatusPill status={sel.status} />
                <div className="ml-auto flex gap-2">
                  {sel.status === 'failed' && (
                    <Button size="sm" className="gap-1.5" onClick={() => { retryInstance(sel.id); toast.success('Resuming from the failed step — completed steps are not re-run'); }}>
                      <RotateCcw className="h-3.5 w-3.5" /> Retry from failed step
                    </Button>
                  )}
                  {(sel.status === 'running' || sel.status === 'waiting') && (
                    <Button size="sm" variant="outline" className="gap-1.5" onClick={() => cancelInstance(sel.id)}>
                      <Ban className="h-3.5 w-3.5" /> Cancel
                    </Button>
                  )}
                </div>
              </div>

              <div className="mt-4">
                {sel.stepRuns.map((sr, i) => (
                  <div key={sr.stepId}>
                    {i > 0 && <div className="ml-[26px] py-0.5"><ArrowDown className="h-3.5 w-3.5 text-slate-300" /></div>}
                    <div className={cn(
                      'flex items-start gap-3 rounded-lg border p-3',
                      sr.status === 'running' && 'border-indigo-300 bg-indigo-50/40',
                      sr.status === 'waiting' && 'border-amber-300 bg-amber-50/40',
                      sr.status === 'failed' && 'border-rose-300 bg-rose-50/40',
                      sr.status === 'pending' && 'opacity-60',
                    )}>
                      <StepIcon type={sr.type} />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium text-sm">{sr.name}</span>
                          {sr.durationMs != null && <Badge variant="outline" className="text-[10px]">{fmtDur(sr.durationMs)}</Badge>}
                        </div>
                        {sr.output && <div className="mt-0.5 text-[11px] text-muted-foreground">→ {sr.output}</div>}
                        {sr.note && <div className="mt-0.5 text-[11px] text-amber-700">{sr.note}</div>}
                        {sr.status === 'waiting' && (
                          <Button size="sm" variant="outline" className="mt-2 h-7 gap-1.5 text-xs border-amber-300 text-amber-800 hover:bg-amber-100"
                            onClick={() => { approveTask(sel.id); toast.success(`Approved as ${sel.waitingOn} — execution resumed`); }}>
                            <UserCheck className="h-3 w-3" /> Approve as {sel.waitingOn} (simulate)
                          </Button>
                        )}
                      </div>
                      <StepStatusIcon status={sr.status} />
                    </div>
                  </div>
                ))}
              </div>

              {sel.error && (
                <div className="mt-3 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
                  <span className="font-semibold">Error:</span> {sel.error} — retry resumes from this step only.
                </div>
              )}
            </div>
          ) : (
            <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">Select an execution to inspect its steps.</div>
          )}
        </div>
      </div>
    </div>
  );
}

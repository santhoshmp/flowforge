import { Play, Download, Package, Code2, Pencil } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useStore } from '@/lib/store';
import { StatusPill, StepIcon } from '@/components/step';
import { toYAML, download, runnerReadme } from '@/lib/dsl';
import { toast } from 'sonner';
import type { Workflow } from '@/lib/types';

export default function Workflows({ onGoMonitor, onEdit }: { onGoMonitor: () => void; onEdit: (wf: Workflow) => void }) {
  const { workflows, runWorkflow, logAudit } = useStore();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Workflows</h1>
        <p className="text-sm text-muted-foreground mt-1">Versioned, approved, and callable via API — or exportable as portable YAML.</p>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        {workflows.map((wf) => (
          <div key={wf.id} className="rounded-xl border bg-card p-5 shadow-sm flex flex-col">
            <div className="flex items-center gap-3">
              <span className="font-semibold text-sm">{wf.name}</span>
              <span className="text-[11px] text-muted-foreground">v{wf.version}</span>
              <StatusPill status={wf.status} />
              <span className="ml-auto text-[11px] text-muted-foreground">{wf.runs} runs</span>
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{wf.description}</p>

            <div className="mt-3 flex items-center gap-1.5 overflow-x-auto pb-1">
              {wf.steps.map((s) => (
                <div key={s.id} className="flex items-center gap-1.5 shrink-0">
                  <StepIcon type={s.type} className="h-6 w-6 rounded-md" />
                  <span className="text-slate-300 text-xs">→</span>
                </div>
              ))}
            </div>

            <div className="mt-3 rounded-lg bg-muted/60 border px-3 py-2 font-mono text-[11px] text-muted-foreground flex items-center gap-2">
              <Code2 className="h-3.5 w-3.5 shrink-0" />
              POST /api/v1/workflows/{wf.id}/executions
            </div>

            <div className="mt-1.5 text-[11px] text-muted-foreground">
              created by {wf.createdBy} · approved by {wf.approvedBy ?? '—'} · {wf.aiModel}
            </div>

            <div className="mt-4 flex flex-wrap gap-2 pt-2 border-t">
              <Button size="sm" className="gap-1.5" disabled={wf.status === 'draft'} onClick={() => { runWorkflow(wf.id); onGoMonitor(); }}>
                <Play className="h-3.5 w-3.5" /> Run
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={() => onEdit(wf)}>
                <Pencil className="h-3.5 w-3.5" /> Edit
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={() => {
                const fname = `${wf.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}.flow.yaml`;
                download(fname, toYAML(wf));
                logAudit('You', 'Exported workflow', `${fname}`, 'export');
                toast.success('Downloaded — runs anywhere with the standalone runner');
              }}>
                <Download className="h-3.5 w-3.5" /> .flow.yaml
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={() => {
                download(`${wf.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}-runner.md`, runnerReadme(wf), 'text/markdown');
                toast.success('Standalone runner notes downloaded');
              }}>
                <Package className="h-3.5 w-3.5" /> Runner
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

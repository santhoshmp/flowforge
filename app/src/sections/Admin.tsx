import { useState } from 'react';
import { Activity, AlertTriangle, CheckCircle2, Clock, FileText, Sparkles, ShieldCheck, Database, Download, UserCheck, Blocks, Plus, Pencil, Trash2 } from 'lucide-react';
import { useStore } from '@/lib/store';
import { StatusPill, StepIcon, COLOR_CLASSES, ICONS } from '@/components/step';
import AIConfigCard from '@/components/AIConfigCard';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { toast } from 'sonner';
import type { AuditEntry, ControlDef } from '@/lib/types';
import { cn, timeAgo } from '@/lib/utils';

const KIND_ICON: Record<AuditEntry['kind'], typeof Sparkles> = {
  ai: Sparkles, approval: ShieldCheck, deploy: CheckCircle2, execution: Activity, mdm: Database, export: Download,
};

const slugKey = (label: string) => label.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');

interface ControlForm { label: string; key: string; color: string; icon: string; description: string }
const EMPTY_FORM: ControlForm = { label: '', key: '', color: 'violet', icon: 'Code', description: '' };

export default function Admin() {
  const { instances, audit, workflows, controls, toggleControl, addControl, updateControl, removeControl, refresh } = useStore();
  const [dialog, setDialog] = useState<{ mode: 'add' } | { mode: 'edit'; original: ControlDef } | null>(null);
  const [form, setForm] = useState<ControlForm>(EMPTY_FORM);
  const usageCount = (t: string) => workflows.reduce((n, w) => n + w.steps.filter((s) => s.type === t).length, 0);

  const openAdd = () => { setForm(EMPTY_FORM); setDialog({ mode: 'add' }); };
  const openEdit = (c: ControlDef) => {
    setForm({ label: c.label, key: c.key, color: c.color, icon: c.icon, description: c.description ?? '' });
    setDialog({ mode: 'edit', original: c });
  };

  const saveDialog = () => {
    const label = form.label.trim();
    if (!label) { toast.error('Label is required'); return; }
    if (dialog?.mode === 'add') {
      const key = (form.key.trim() || slugKey(label));
      if (!/^[a-z][a-z0-9_.]*$/.test(key)) { toast.error('Key must be lowercase letters, numbers, dots or underscores'); return; }
      if (controls.some((c) => c.key === key)) { toast.error(`Control "${key}" already exists`); return; }
      addControl({ key, label, color: form.color, icon: form.icon, description: form.description.trim() || undefined, enabled: true, custom: true });
      toast.success(`Control "${label}" added — it's live in the palette`);
    } else if (dialog?.mode === 'edit') {
      updateControl(dialog.original.key, { label, color: form.color, icon: form.icon, description: form.description.trim() || undefined });
      toast.success('Control updated');
    }
    setDialog(null);
  };

  const counts = {
    running: instances.filter((i) => i.status === 'running').length,
    waiting: instances.filter((i) => i.status === 'waiting').length,
    failed: instances.filter((i) => i.status === 'failed').length,
    completed: instances.filter((i) => i.status === 'completed').length,
  };

  const stats = [
    { label: 'Running now', value: counts.running, icon: Activity, cls: 'text-indigo-600 bg-indigo-50 border-indigo-200' },
    { label: 'Waiting on humans', value: counts.waiting, icon: Clock, cls: 'text-amber-600 bg-amber-50 border-amber-200' },
    { label: 'Failed', value: counts.failed, icon: AlertTriangle, cls: 'text-rose-600 bg-rose-50 border-rose-200' },
    { label: 'Completed', value: counts.completed, icon: CheckCircle2, cls: 'text-emerald-600 bg-emerald-50 border-emerald-200' },
  ];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Admin Console</h1>
        <p className="text-sm text-muted-foreground mt-1">Fleet-wide state, human task queue, and a complete audit trail — including every AI draft and human approval.</p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {stats.map((s) => (
          <div key={s.label} className={cn('rounded-xl border p-4', s.cls.split(' ').slice(1).join(' '))}>
            <div className="flex items-center gap-2">
              <s.icon className={cn('h-4 w-4', s.cls.split(' ')[0])} />
              <span className="text-xs font-medium text-muted-foreground">{s.label}</span>
            </div>
            <div className="mt-2 text-3xl font-bold">{s.value}</div>
          </div>
        ))}
      </div>

      <AIConfigCard onSaved={refresh} />

      <div className="grid gap-6 lg:grid-cols-2">
        {/* Human task queue */}
        <div className="rounded-xl border bg-card p-5 shadow-sm">
          <div className="flex items-center gap-2 mb-3">
            <UserCheck className="h-4 w-4 text-amber-600" />
            <span className="font-semibold text-sm">Human task queue</span>
            <span className="ml-auto text-[11px] text-muted-foreground">{counts.waiting} pending</span>
          </div>
          {instances.filter((i) => i.status === 'waiting').length === 0 ? (
            <div className="rounded-lg border border-dashed p-6 text-center text-xs text-muted-foreground">No tasks waiting on people right now.</div>
          ) : (
            <div className="space-y-2">
              {instances.filter((i) => i.status === 'waiting').map((i) => (
                <div key={i.id} className="flex items-center gap-3 rounded-lg border border-amber-200 bg-amber-50/50 p-3">
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium truncate">{i.entity}</div>
                    <div className="text-[11px] text-muted-foreground">{i.workflowName} · waiting on <span className="font-medium text-amber-800">{i.waitingOn}</span></div>
                  </div>
                  <span className="text-[11px] text-muted-foreground shrink-0">SLA tracked</span>
                </div>
              ))}
            </div>
          )}

          <div className="mt-4 border-t pt-4">
            <div className="text-xs font-semibold mb-2 text-muted-foreground uppercase tracking-wide">Deployed workflows</div>
            <div className="space-y-1.5">
              {workflows.map((w) => (
                <div key={w.id} className="flex items-center gap-2 text-sm">
                  <span className="truncate flex-1">{w.name} <span className="text-muted-foreground text-xs">v{w.version}</span></span>
                  <span className="text-[11px] text-muted-foreground">{w.runs} runs</span>
                  <StatusPill status={w.status} />
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Audit log */}
        <div className="rounded-xl border bg-card p-5 shadow-sm">
          <div className="flex items-center gap-2 mb-3">
            <FileText className="h-4 w-4 text-slate-600" />
            <span className="font-semibold text-sm">Audit trail</span>
            <span className="ml-auto text-[11px] text-muted-foreground">immutable · exportable</span>
          </div>
          <div className="space-y-1 max-h-[420px] overflow-y-auto pr-1">
            {audit.map((a) => {
              const Icon = KIND_ICON[a.kind];
              return (
                <div key={a.id} className="flex items-start gap-3 rounded-lg px-2 py-2 hover:bg-muted/50">
                  <span className={cn('mt-0.5 inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md border',
                    a.kind === 'ai' ? 'bg-violet-50 text-violet-600 border-violet-200' :
                    a.kind === 'approval' ? 'bg-emerald-50 text-emerald-600 border-emerald-200' :
                    a.kind === 'execution' ? 'bg-indigo-50 text-indigo-600 border-indigo-200' :
                    a.kind === 'mdm' ? 'bg-amber-50 text-amber-600 border-amber-200' :
                    'bg-slate-50 text-slate-500 border-slate-200')}>
                    <Icon className="h-3 w-3" />
                  </span>
                  <div className="min-w-0">
                    <div className="text-xs"><span className="font-semibold">{a.action}</span> <span className="text-muted-foreground">by {a.actor}</span></div>
                    <div className="text-[11px] text-muted-foreground truncate">{a.detail}</div>
                    <div className="text-[10px] text-slate-400">{timeAgo(a.at)}</div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </div>

      {/* Step controls registry */}
      <div className="rounded-xl border bg-card p-5 shadow-sm">
        <div className="flex items-center gap-2 mb-1">
          <Blocks className="h-4 w-4 text-violet-600" />
          <span className="font-semibold text-sm">Step controls</span>
          <span className="ml-auto text-[11px] text-muted-foreground">disabled controls leave the palette and fail pre-flight checks</span>
        </div>
        <p className="text-[11px] text-muted-foreground mb-4">
          The building blocks authors can use in workflows. Add custom controls (e.g. a company-specific action), edit their appearance, or disable ones you don't want used.
        </p>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {controls.map((c) => {
            const used = usageCount(c.key);
            return (
              <div key={c.key} className={cn('group flex items-center gap-3 rounded-lg border p-3', !c.enabled && 'bg-slate-50 opacity-75')}>
                <StepIcon type={c.key} className="h-7 w-7" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5 text-xs font-semibold">
                    {c.label}
                    {c.custom && <span className="rounded bg-violet-100 px-1 text-[9px] font-medium text-violet-700">custom</span>}
                  </div>
                  <div className="text-[10px] text-muted-foreground">
                    <code>{c.key}</code>{used > 0 ? ` · used ${used}×` : ' · unused'}
                  </div>
                </div>
                <button onClick={() => openEdit(c)} className="rounded p-1 text-slate-400 opacity-0 transition-opacity hover:bg-slate-100 hover:text-slate-700 group-hover:opacity-100" title="Edit control">
                  <Pencil className="h-3.5 w-3.5" />
                </button>
                {c.custom && (
                  <button
                    onClick={() => {
                      if (used > 0) { toast.error(`Can't remove — ${used} step(s) still use this control`); return; }
                      removeControl(c.key);
                      toast.success('Custom control removed');
                    }}
                    className="rounded p-1 text-slate-400 opacity-0 transition-opacity hover:bg-rose-50 hover:text-rose-500 group-hover:opacity-100"
                    title="Remove custom control"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                )}
                <Switch checked={c.enabled} onCheckedChange={() => toggleControl(c.key)} />
              </div>
            );
          })}
          <button
            onClick={openAdd}
            className="flex items-center justify-center gap-2 rounded-lg border-2 border-dashed p-3 text-xs font-medium text-slate-500 transition-colors hover:border-violet-300 hover:bg-violet-50 hover:text-violet-700"
          >
            <Plus className="h-4 w-4" /> Add custom control
          </button>
        </div>
      </div>

      {/* Add / edit control dialog */}
      <Dialog open={!!dialog} onOpenChange={(o) => !o && setDialog(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{dialog?.mode === 'edit' ? `Edit control — ${dialog.original.key}` : 'Add custom control'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="flex items-center gap-3">
              <span className={cn('inline-flex h-9 w-9 items-center justify-center rounded-lg border', COLOR_CLASSES[form.color])}>
                {(() => { const Icon = ICONS[form.icon] ?? ICONS.Code; return <Icon className="h-4 w-4" />; })()}
              </span>
              <div className="flex-1">
                <label className="text-xs font-medium text-muted-foreground">Label</label>
                <Input
                  value={form.label}
                  onChange={(e) => setForm({ ...form, label: e.target.value, ...(dialog?.mode === 'add' && !form.key ? { key: slugKey(e.target.value) } : {}) })}
                  placeholder="e.g. Send SMS"
                  className="mt-1 h-8 text-sm"
                />
              </div>
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">Key (step type id used in JSON/YAML)</label>
              <Input
                value={form.key}
                onChange={(e) => setForm({ ...form, key: e.target.value })}
                disabled={dialog?.mode === 'edit'}
                placeholder="e.g. custom.send_sms"
                className="mt-1 h-8 font-mono text-xs"
              />
              {dialog?.mode === 'edit' && <div className="mt-1 text-[10px] text-muted-foreground">Keys are immutable — existing workflow definitions reference them.</div>}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs font-medium text-muted-foreground">Color</label>
                <Select value={form.color} onValueChange={(v) => setForm({ ...form, color: v })}>
                  <SelectTrigger className="mt-1 h-8 text-xs"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {Object.keys(COLOR_CLASSES).map((c) => <SelectItem key={c} value={c} className="text-xs capitalize">{c}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Icon</label>
                <Select value={form.icon} onValueChange={(v) => setForm({ ...form, icon: v })}>
                  <SelectTrigger className="mt-1 h-8 text-xs"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {Object.keys(ICONS).map((i) => <SelectItem key={i} value={i} className="text-xs">{i}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">Description (palette tooltip)</label>
              <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className="mt-1 h-8 text-sm" placeholder="What does this step do?" />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <Button variant="outline" size="sm" onClick={() => setDialog(null)}>Cancel</Button>
              <Button size="sm" onClick={saveDialog}>{dialog?.mode === 'edit' ? 'Save changes' : 'Add control'}</Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

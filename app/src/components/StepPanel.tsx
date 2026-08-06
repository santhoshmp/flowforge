import { AlertTriangle, Plus, Trash2, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import type { WorkflowStep } from '@/lib/types';
import { useStore } from '@/lib/store';
import { StepIcon, ConfidenceBadge } from '@/components/step';

// Docked step-properties panel — opens on single click of any canvas node.

interface StepPanelProps {
  step: WorkflowStep;
  onChange: (s: WorkflowStep) => void;
  onSave: () => void;
  onClose: () => void;
  onDelete: () => void;
}

export default function StepPanel({ step, onChange, onSave, onClose, onDelete }: StepPanelProps) {
  const { controlMap } = useStore();
  const meta = controlMap[step.type] ?? { label: step.type };
  const setParamKey = (i: number, key: string) =>
    onChange({ ...step, params: Object.fromEntries(Object.entries(step.params).map(([k, v], j) => (j === i ? [key, v] : [k, v]))) });
  const setParamVal = (i: number, val: string) =>
    onChange({ ...step, params: Object.fromEntries(Object.entries(step.params).map(([k, v], j) => (j === i ? [k, val] : [k, v]))) });
  const removeParam = (i: number) =>
    onChange({ ...step, params: Object.fromEntries(Object.entries(step.params).filter((_, j) => j !== i)) });

  return (
    <div className="absolute right-0 top-0 bottom-0 z-10 flex w-80 flex-col border-l bg-white shadow-xl">
      {/* Header */}
      <div className="flex items-center gap-2 border-b px-4 py-3">
        <StepIcon type={step.type} />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold">Step details</div>
          <div className="text-[10px] text-muted-foreground">{meta.label} · {step.id}</div>
        </div>
        <button onClick={onClose} className="rounded-md p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700">
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Body */}
      <div className="flex-1 space-y-4 overflow-y-auto p-4">
        <div className="flex items-center gap-2">
          <ConfidenceBadge value={step.confidence} />
          {step.edited && <Badge variant="outline" className="text-[10px] text-sky-700 border-sky-300">edited by you</Badge>}
        </div>

        <div>
          <label className="text-xs font-semibold text-muted-foreground">Name</label>
          <Input value={step.name} onChange={(e) => onChange({ ...step, name: e.target.value })} className="mt-1 h-8 text-sm" />
        </div>

        {step.assumptions.length > 0 && (
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-amber-700">AI assumptions to confirm</label>
            {step.assumptions.map((a, i) => (
              <div key={i} className="flex items-start gap-1.5 rounded-md bg-amber-50 border border-amber-200 px-2.5 py-1.5 text-[11px] text-amber-800">
                <AlertTriangle className="h-3.5 w-3.5 mt-px shrink-0" />
                <span>{a} <span className="font-medium">Saving marks this human-confirmed.</span></span>
              </div>
            ))}
          </div>
        )}

        <div>
          <label className="text-xs font-semibold text-muted-foreground">Parameters</label>
          <div className="mt-1.5 space-y-2">
            {Object.entries(step.params).map(([k, v], i) => (
              <div key={i} className="flex items-center gap-1.5">
                <Input value={k} onChange={(e) => setParamKey(i, e.target.value)} className="h-8 w-2/5 text-xs font-mono" placeholder="key" />
                <Input value={String(v)} onChange={(e) => setParamVal(i, e.target.value)} className="h-8 w-3/5 text-xs font-mono" placeholder="value" />
                <button onClick={() => removeParam(i)} className="rounded p-1 text-slate-300 hover:bg-rose-50 hover:text-rose-500">
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
            <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs" onClick={() => onChange({ ...step, params: { ...step.params, new_key: '' } })}>
              <Plus className="h-3 w-3" /> Add parameter
            </Button>
          </div>
        </div>
      </div>

      {/* Footer */}
      <div className="flex items-center gap-2 border-t px-4 py-3">
        {step.type !== 'trigger' && (
          <Button variant="outline" size="sm" className="gap-1.5 text-rose-600 border-rose-200 hover:bg-rose-50" onClick={onDelete}>
            <Trash2 className="h-3.5 w-3.5" /> Delete
          </Button>
        )}
        <Button size="sm" className="ml-auto gap-1.5 bg-violet-600 hover:bg-violet-700" onClick={onSave}>
          Save & confirm step
        </Button>
      </div>
    </div>
  );
}

import { useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';

// Structured, collapsible, color-coded JSON viewer — replaces free-text dumps.

function summarize(value: unknown): string {
  if (Array.isArray(value)) return `[ ${value.length} item${value.length === 1 ? '' : 's'} ]`;
  if (value !== null && typeof value === 'object') {
    const o = value as Record<string, unknown>;
    if (typeof o.type === 'string' && typeof o.name === 'string') return `${o.type} · “${o.name}”`;
    return `{ ${Object.keys(o).length} keys }`;
  }
  return '';
}

function Leaf({ value }: { value: unknown }) {
  if (typeof value === 'string') return <span className="text-emerald-700">"{value}"</span>;
  if (typeof value === 'number') return <span className="text-violet-700">{value}</span>;
  if (typeof value === 'boolean') return <span className="text-rose-600">{String(value)}</span>;
  if (value === null) return <span className="italic text-slate-400">null</span>;
  return <span className="text-slate-600">{String(value)}</span>;
}

function TreeNode({ name, value, depth }: { name: string; value: unknown; depth: number }) {
  const [open, setOpen] = useState(depth < 2);
  const isObj = value !== null && typeof value === 'object';
  if (!isObj) {
    return (
      <div className="flex gap-2 py-px" style={{ paddingLeft: depth * 18 }}>
        <span className="shrink-0 font-medium text-slate-600">{name}:</span>
        <Leaf value={value} />
      </div>
    );
  }
  const entries = Array.isArray(value)
    ? value.map((v, i) => [String(i), v] as const)
    : Object.entries(value as Record<string, unknown>);
  return (
    <div>
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-1 rounded py-px text-left hover:bg-slate-100"
        style={{ paddingLeft: depth * 18 }}
      >
        {open ? <ChevronDown className="h-3 w-3 shrink-0 text-slate-400" /> : <ChevronRight className="h-3 w-3 shrink-0 text-slate-400" />}
        <span className="font-medium text-slate-700">{name}</span>
        <span className="text-slate-400">{Array.isArray(value) ? '[]' : '{}'}</span>
        {!open && <span className="truncate text-slate-400">{summarize(value)}</span>}
      </button>
      {open && (
        <div className="border-l border-slate-200" style={{ marginLeft: depth * 18 + 5 }}>
          {entries.map(([k, v]) => (
            <TreeNode key={k} name={k} value={v} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  );
}

export default function JsonTree({ data, className }: { data: unknown; className?: string }) {
  return (
    <div className={cn('overflow-auto rounded-lg border bg-slate-50/60 p-4 font-mono text-xs leading-relaxed', className)}>
      <TreeNode name="workflow" value={data} depth={0} />
    </div>
  );
}

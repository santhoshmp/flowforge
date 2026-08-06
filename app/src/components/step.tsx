import {
  Zap, Sparkles, Database, ShieldCheck, GitBranch, UserCheck, Bell,
  ArrowRightLeft, Globe, Code, Timer, CheckCircle2, XCircle, Loader2,
  Clock, MinusCircle, AlertTriangle, type LucideIcon,
} from 'lucide-react';
import { STEP_META, type StepStatus, type StepType } from '@/lib/types';
import { useStore } from '@/lib/store';
import { cn } from '@/lib/utils';

export const ICONS: Record<string, LucideIcon> = {
  Zap, Sparkles, Database, ShieldCheck, GitBranch, UserCheck, Bell,
  ArrowRightLeft, Globe, Code, Timer,
};

export const COLOR_CLASSES: Record<string, string> = {
  emerald: 'bg-emerald-100 text-emerald-700 border-emerald-200',
  violet: 'bg-violet-100 text-violet-700 border-violet-200',
  amber: 'bg-amber-100 text-amber-700 border-amber-200',
  sky: 'bg-sky-100 text-sky-700 border-sky-200',
  rose: 'bg-rose-100 text-rose-700 border-rose-200',
  indigo: 'bg-indigo-100 text-indigo-700 border-indigo-200',
  cyan: 'bg-cyan-100 text-cyan-700 border-cyan-200',
  slate: 'bg-slate-100 text-slate-700 border-slate-200',
  orange: 'bg-orange-100 text-orange-700 border-orange-200',
};

export function StepIcon({ type, className }: { type: string; className?: string }) {
  const { controlMap } = useStore();
  const meta = controlMap[type] ?? STEP_META[type as StepType] ?? { label: type, color: 'slate', icon: 'Code' };
  const Icon = ICONS[meta.icon] ?? Code;
  return (
    <span className={cn('inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border', COLOR_CLASSES[meta.color] ?? COLOR_CLASSES.slate, className)}>
      <Icon className="h-4 w-4" />
    </span>
  );
}

export function ConfidenceBadge({ value }: { value: number }) {
  const tone = value >= 90 ? 'text-emerald-700 bg-emerald-50 border-emerald-200'
    : value >= 78 ? 'text-amber-700 bg-amber-50 border-amber-200'
    : 'text-rose-700 bg-rose-50 border-rose-200';
  return (
    <span className={cn('inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium', tone)}>
      <Sparkles className="h-3 w-3" /> {value}% AI confidence
    </span>
  );
}

const STATUS_STYLE: Record<StepStatus, { icon: LucideIcon; cls: string; spin?: boolean }> = {
  pending: { icon: Clock, cls: 'text-slate-400' },
  running: { icon: Loader2, cls: 'text-indigo-600', spin: true },
  waiting: { icon: AlertTriangle, cls: 'text-amber-500' },
  succeeded: { icon: CheckCircle2, cls: 'text-emerald-600' },
  failed: { icon: XCircle, cls: 'text-rose-600' },
  skipped: { icon: MinusCircle, cls: 'text-slate-400' },
};

export function StepStatusIcon({ status }: { status: StepStatus }) {
  const s = STATUS_STYLE[status];
  const Icon = s.icon;
  return <Icon className={cn('h-4 w-4 shrink-0', s.cls, s.spin && 'animate-spin')} />;
}

export function StatusPill({ status }: { status: string }) {
  const map: Record<string, string> = {
    running: 'bg-indigo-50 text-indigo-700 border-indigo-200',
    waiting: 'bg-amber-50 text-amber-700 border-amber-200',
    failed: 'bg-rose-50 text-rose-700 border-rose-200',
    completed: 'bg-emerald-50 text-emerald-700 border-emerald-200',
    cancelled: 'bg-slate-100 text-slate-500 border-slate-200',
    draft: 'bg-slate-100 text-slate-600 border-slate-200',
    approved: 'bg-sky-50 text-sky-700 border-sky-200',
    deployed: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  };
  return (
    <span className={cn('rounded-full border px-2.5 py-0.5 text-[11px] font-semibold capitalize', map[status] ?? 'bg-slate-100 text-slate-600 border-slate-200')}>
      {status}
    </span>
  );
}

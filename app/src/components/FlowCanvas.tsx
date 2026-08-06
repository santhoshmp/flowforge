import { useCallback, useMemo, useState } from 'react';
import {
  ReactFlow, ReactFlowProvider, Background, BackgroundVariant, Controls, MiniMap,
  Handle, Position, useReactFlow,
  type Node, type Edge, type NodeChange, type Connection, type NodeProps,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { AlertTriangle, Pencil, GripVertical } from 'lucide-react';
import type { WorkflowStep } from '@/lib/types';
import { StepIcon } from '@/components/step';
import { useStore } from '@/lib/store';
import { cn } from '@/lib/utils';

// ---------------------------------------------------------------------------
// Drag-and-drop workflow designer — node canvas, palette, reconnectable edges,
// zoom/pan controls, minimap. Steps stay the single source of truth; edges
// define execution order (linear in MVP, branching later).
// ---------------------------------------------------------------------------

export interface DraftEdge { id: string; source: string; target: string }

export const chainEdges = (steps: WorkflowStep[]): DraftEdge[] =>
  steps.slice(1).map((s, i) => ({ id: `e-${steps[i].id}-${s.id}`, source: steps[i].id, target: s.id }));

export const autoLayout = (steps: WorkflowStep[]): WorkflowStep[] =>
  steps.map((s, i) => (s.position ? s : { ...s, position: { x: 80, y: 30 + i * 118 } }));

// Recompute linear execution order by walking edges from the trigger.
export function orderFromEdges(steps: WorkflowStep[], edges: DraftEdge[]): WorkflowStep[] {
  const byId = new Map(steps.map((s) => [s.id, s]));
  const next = new Map(edges.map((e) => [e.source, e.target]));
  const start = steps.find((s) => s.type === 'trigger') ?? steps[0];
  const ordered: WorkflowStep[] = [];
  const seen = new Set<string>();
  let cur: WorkflowStep | undefined = start;
  while (cur && !seen.has(cur.id)) {
    ordered.push(cur);
    seen.add(cur.id);
    cur = byId.get(next.get(cur.id) ?? '');
  }
  for (const s of steps) if (!seen.has(s.id)) ordered.push(s);
  return ordered;
}

// ---- Custom node -------------------------------------------------------------

type StepNodeData = { step: WorkflowStep; onEdit: (s: WorkflowStep) => void; review: boolean };
type StepFlowNode = Node<StepNodeData, 'step'>;

function StepNode({ data, selected }: NodeProps<StepFlowNode>) {
  const { controlMap } = useStore();
  const { step, onEdit, review } = data;
  const metaLabel = controlMap[step.type]?.label ?? step.type;
  const hasAssumptions = step.assumptions.length > 0;
  const lowConf = step.confidence < 78;
  return (
    <div
      className={cn(
        'group w-[230px] cursor-pointer rounded-xl border-2 bg-white shadow-sm transition-shadow hover:shadow-md',
        hasAssumptions && review ? 'border-amber-400' : lowConf && review ? 'border-rose-300' : 'border-slate-200',
        selected && 'ring-2 ring-violet-400 border-violet-400',
      )}
      onDoubleClick={() => onEdit(step)}
    >
      {step.type !== 'trigger' && <Handle type="target" position={Position.Top} className="!h-2.5 !w-2.5 !bg-slate-400" />}
      <div className="flex items-start gap-2.5 p-3">
        <StepIcon type={step.type} className="h-8 w-8" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1">
            <span className="truncate text-[13px] font-semibold leading-tight">{step.name}</span>
          </div>
          <div className="mt-0.5 flex flex-wrap items-center gap-1">
            <span className="rounded bg-slate-100 px-1.5 py-px text-[9px] font-medium text-slate-600">{metaLabel}</span>
            {review && (
              <span className={cn('rounded px-1.5 py-px text-[9px] font-medium',
                step.confidence >= 90 ? 'bg-emerald-50 text-emerald-700' : step.confidence >= 78 ? 'bg-amber-50 text-amber-700' : 'bg-rose-50 text-rose-700')}>
                {step.confidence}%
              </span>
            )}
            {step.edited && <span className="rounded bg-sky-50 px-1.5 py-px text-[9px] font-medium text-sky-700">edited</span>}
          </div>
        </div>
        <button
          onClick={(e) => { e.stopPropagation(); onEdit(step); }}
          className="rounded p-1 text-slate-400 opacity-0 transition-opacity hover:bg-slate-100 hover:text-slate-700 group-hover:opacity-100"
          title="Edit step"
        >
          <Pencil className="h-3.5 w-3.5" />
        </button>
      </div>
      {hasAssumptions && review && (
        <div className="flex items-center gap-1.5 border-t border-amber-200 bg-amber-50 px-3 py-1.5 text-[10px] text-amber-800 rounded-b-[10px]">
          <AlertTriangle className="h-3 w-3 shrink-0" />
          <span className="truncate">{step.assumptions.length} AI assumption{step.assumptions.length > 1 ? 's' : ''} — double-click to confirm</span>
        </div>
      )}
      <Handle type="source" position={Position.Bottom} className="!h-2.5 !w-2.5 !bg-violet-500" />
    </div>
  );
}

const nodeTypes = { step: StepNode };

// ---- Palette -----------------------------------------------------------------

export function StepPalette({ onAdd }: { onAdd: (t: string) => void }) {
  const { controls } = useStore();
  const items = controls.filter((c) => c.key !== 'trigger');
  return (
    <div className="w-44 shrink-0 space-y-1 overflow-y-auto border-r bg-slate-50/70 p-3">
      <div className="mb-2 text-[10px] font-bold uppercase tracking-wider text-slate-500">Step palette</div>
      {items.map((c) => (
        <div
          key={c.key}
          draggable={c.enabled}
          onDragStart={(e) => {
            if (!c.enabled) return;
            e.dataTransfer.setData('application/flowforge-step', c.key);
            e.dataTransfer.effectAllowed = 'move';
          }}
          onClick={() => c.enabled && onAdd(c.key)}
          className={cn(
            'flex items-center gap-2 rounded-lg border bg-white px-2.5 py-2 text-xs transition-colors',
            c.enabled
              ? 'cursor-grab hover:border-violet-300 hover:bg-violet-50 active:cursor-grabbing'
              : 'cursor-not-allowed opacity-40',
          )}
          title={c.enabled ? (c.description ?? 'Drag onto canvas or click to add') : 'Disabled by admin — enable it in Admin → Step controls'}
        >
          <GripVertical className="h-3 w-3 shrink-0 text-slate-300" />
          <StepIcon type={c.key} className="h-6 w-6 rounded-md" />
          <span className="font-medium leading-tight">{c.label}</span>
          {!c.enabled && <span className="ml-auto rounded bg-slate-100 px-1 text-[9px] text-slate-500">off</span>}
        </div>
      ))}
      <div className="pt-2 text-[10px] leading-relaxed text-slate-400">
        Drag a step onto the canvas, or click to append. Drag between node handles to rewire the flow.
      </div>
    </div>
  );
}

// ---- Canvas ------------------------------------------------------------------

interface FlowCanvasProps {
  steps: WorkflowStep[];
  edges: DraftEdge[];
  review: boolean;
  onStepsChange: (steps: WorkflowStep[], edges: DraftEdge[]) => void;
  onEditStep: (s: WorkflowStep) => void;
}

function CanvasInner({ steps, edges, review, onStepsChange, onEditStep }: FlowCanvasProps) {
  const { screenToFlowPosition } = useReactFlow();
  const { controlMap } = useStore();
  // Selection is canvas-local UI state — it must NOT touch workflow data,
  // otherwise measurement/dimension events would loop and nodes stay hidden.
  const [selectedIds, setSelectedIds] = useState<ReadonlySet<string>>(new Set());

  const nodes: StepFlowNode[] = useMemo(
    () => steps.map((s, i) => ({
      id: s.id,
      type: 'step',
      position: s.position ?? { x: 80, y: 30 + i * 118 },
      selected: selectedIds.has(s.id),
      data: { step: s, onEdit: onEditStep, review },
    })),
    [steps, review, onEditStep, selectedIds],
  );

  const rfEdges: Edge[] = useMemo(
    () => edges.map((e) => ({
      ...e,
      animated: true,
      style: { stroke: '#a5b4fc', strokeWidth: 2 },
    })),
    [edges],
  );

  const onNodesChange = useCallback((changes: NodeChange<StepFlowNode>[]) => {
    // Ignore 'dimensions'/'position' transient events unless they carry a real
    // position — recreating nodes on measurement events leaves them hidden.
    let nextSteps: WorkflowStep[] | null = null;
    let nextEdges = edges;
    const removed: string[] = [];
    let sel: Set<string> | null = null;

    for (const c of changes) {
      if (c.type === 'select') {
        sel = sel ?? new Set(selectedIds);
        if (c.selected) sel.add(c.id); else sel.delete(c.id);
      } else if (c.type === 'position' && c.position) {
        nextSteps = nextSteps ?? steps.map((s) => ({ ...s }));
        nextSteps = nextSteps.map((s) => (s.id === c.id ? { ...s, position: c.position! } : s));
      } else if (c.type === 'remove') {
        removed.push(c.id);
      }
    }

    if (sel) setSelectedIds(sel);

    if (removed.length) {
      const base = nextSteps ?? steps;
      const kept = base.filter((s) => !removed.includes(s.id));
      nextEdges = chainEdges(orderFromEdges(kept, edges.filter((e) => !removed.includes(e.source) && !removed.includes(e.target))));
      onStepsChange(kept, nextEdges);
    } else if (nextSteps) {
      onStepsChange(nextSteps, nextEdges);
    }
  }, [steps, edges, onStepsChange, selectedIds]);

  const onConnect = useCallback((conn: Connection) => {
    if (!conn.source || !conn.target) return;
    const filtered = edges.filter((e) => e.source !== conn.source); // linear: one outgoing edge per node
    const nextEdges = [...filtered, { id: `e-${conn.source}-${conn.target}`, source: conn.source, target: conn.target }];
    const ordered = orderFromEdges(steps, nextEdges);
    onStepsChange(ordered, chainEdges(ordered));
  }, [steps, edges, onStepsChange]);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    const type = e.dataTransfer.getData('application/flowforge-step');
    if (!type) return;
    const pos = screenToFlowPosition({ x: e.clientX, y: e.clientY });
    const newStep: WorkflowStep = {
      id: `manual_${Math.random().toString(36).slice(2, 7)}`,
      type,
      name: `New ${controlMap[type]?.label ?? type}`,
      params: {},
      confidence: 100,
      assumptions: [],
      edited: true,
      position: { x: pos.x - 115, y: pos.y - 40 },
    };
    const nextSteps = [...steps, newStep];
    const last = steps[steps.length - 1];
    const nextEdges = last ? [...edges, { id: `e-${last.id}-${newStep.id}`, source: last.id, target: newStep.id }] : edges;
    onStepsChange(nextSteps, nextEdges);
  }, [steps, edges, onStepsChange, screenToFlowPosition, controlMap]);

  const onNodeClick = useCallback((_: unknown, node: StepFlowNode) => {
    onEditStep(node.data.step);
  }, [onEditStep]);

  return (
    <ReactFlow
      nodes={nodes}
      edges={rfEdges}
      nodeTypes={nodeTypes}
      onNodesChange={onNodesChange}
      onNodeClick={onNodeClick}
      onConnect={onConnect}
      onDrop={onDrop}
      onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; }}
      deleteKeyCode={['Backspace', 'Delete']}
      fitView
      fitViewOptions={{ padding: 0.25, maxZoom: 1 }}
      proOptions={{ hideAttribution: true }}
      className="bg-slate-50"
    >
      <Background variant={BackgroundVariant.Dots} gap={18} size={1.5} color="#cbd5e1" />
      <Controls showInteractive={false} className="!shadow-md" />
      <MiniMap pannable zoomable className="!h-24 !w-36" nodeColor="#c4b5fd" maskColor="rgba(248,250,252,0.7)" />
    </ReactFlow>
  );
}

export default function FlowCanvas(props: FlowCanvasProps) {
  return (
    <ReactFlowProvider>
      <CanvasInner {...props} />
    </ReactFlowProvider>
  );
}

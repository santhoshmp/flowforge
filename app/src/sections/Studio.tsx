import { useCallback, useEffect, useState } from 'react';
import {
  Sparkles, Wand2, AlertTriangle, CheckCircle2,
  Download, Package, Play, ShieldCheck, X, LayoutGrid, Braces, Pencil,
  Maximize2, Minimize2,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import StepPanel from '@/components/StepPanel';
import JsonTree from '@/components/JsonTree';
import { toast } from 'sonner';
import { SAMPLE_PROMPTS, type GeneratedDraft } from '@/lib/ai';
import type { Workflow, WorkflowStep } from '@/lib/types';
import { toYAML, download, runnerReadme } from '@/lib/dsl';
import { useStore } from '@/lib/store';
import { ConfidenceBadge } from '@/components/step';
import FlowCanvas, { StepPalette, chainEdges, autoLayout, type DraftEdge } from '@/components/FlowCanvas';
import { cn } from '@/lib/utils';

const AI_PHASES = ['Parsing intent…', 'Mapping to master data entities…', 'Selecting connectors…', 'Scoring confidence per step…'];

export default function Studio({ onGoMonitor, editWorkflow, onEditDone }: { onGoMonitor: () => void; editWorkflow?: Workflow | null; onEditDone?: () => void }) {
  const { createWorkflowFromDraft, updateWorkflow, approveAndDeploy, runWorkflow, logAudit, generateDraft } = useStore();
  const [editSource, setEditSource] = useState<Workflow | null>(null);
  const [prompt, setPrompt] = useState('');
  const [promptOpen, setPromptOpen] = useState(true);
  const [viewMode, setViewMode] = useState<'designer' | 'json'>('designer');
  const [jsonText, setJsonText] = useState('');
  const [jsonError, setJsonError] = useState<string | null>(null);
  const [jsonEditing, setJsonEditing] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const { controlMap } = useStore();

  // Load an existing workflow into the designer when "Edit" is clicked
  useEffect(() => {
    if (!editWorkflow) return;
    const steps = autoLayout(editWorkflow.steps.map((s) => ({ ...s, position: undefined })));
    setDraft({
      name: editWorkflow.name,
      description: editWorkflow.description,
      steps,
      model: editWorkflow.aiModel,
      overallConfidence: Math.round(editWorkflow.steps.reduce((a, s) => a + s.confidence, 0) / editWorkflow.steps.length),
    });
    setEdges(chainEdges(steps));
    setEditSource(editWorkflow);
    setApproved(null);
    setPrompt(editWorkflow.prompt);
    setPromptOpen(false);
    setViewMode('designer');
  }, [editWorkflow]);
  const [phase, setPhase] = useState<number>(-1);
  const [draft, setDraft] = useState<GeneratedDraft | null>(null);
  const [edges, setEdges] = useState<DraftEdge[]>([]);
  const [editing, setEditing] = useState<WorkflowStep | null>(null);
  const [approved, setApproved] = useState<Workflow | null>(null);

  const generate = async (text: string) => {
    if (!text.trim() || phase >= 0) return;
    setApproved(null);
    setDraft(null);
    setEditSource(null);
    onEditDone?.();
    setPhase(0);
    AI_PHASES.forEach((_, i) => setTimeout(() => setPhase(i + 1), 500 * (i + 1)));
    try {
      const { draft: d } = await generateDraft(text);
      const laidOut = { ...d, steps: autoLayout(d.steps) };
      setDraft(laidOut);
      setEdges(chainEdges(laidOut.steps));
      setPhase(-1);
      setPromptOpen(false);
      setViewMode('designer');
    } catch (e) {
      setPhase(-1);
      toast.error('Could not generate draft', { description: e instanceof Error ? e.message : 'please try again' });
    }
  };

  // ---- JSON view -------------------------------------------------------------
  const draftToJSON = (d: GeneratedDraft) =>
    JSON.stringify({
      name: d.name,
      description: d.description,
      steps: d.steps.map((s) => ({
        id: s.id, type: s.type, name: s.name, params: s.params,
        ...(s.assumptions.length ? { assumptions: s.assumptions } : {}),
      })),
    }, null, 2);

  const openJSON = () => {
    if (!draft) return;
    setJsonText(draftToJSON(draft));
    setJsonError(null);
    setJsonEditing(false);
    setEditing(null);
    setViewMode('json');
  };

  // Escape exits full-screen
  useEffect(() => {
    if (!expanded) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setExpanded(false);
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [expanded]);

  const applyJSON = () => {
    if (!draft) return;
    try {
      const parsed = JSON.parse(jsonText) as { name?: string; description?: string; steps?: unknown };
      if (!parsed || !Array.isArray(parsed.steps)) throw new Error('Expected an object with a "steps" array.');
      const seen = new Set<string>();
      const steps: WorkflowStep[] = (parsed.steps as Array<Partial<WorkflowStep>>).map((s, i) => {
        if (!s.type || !controlMap[s.type]) {
          throw new Error(`Step ${i + 1}: unknown or missing type "${s.type ?? ''}". Registered controls: ${Object.keys(controlMap).join(', ')}`);
        }
        const id = s.id || `step_${Math.random().toString(36).slice(2, 7)}`;
        if (seen.has(id)) throw new Error(`Duplicate step id "${id}".`);
        seen.add(id);
        return {
          id, type: s.type,
          name: s.name || controlMap[s.type].label,
          params: s.params ?? {},
          confidence: typeof s.confidence === 'number' ? s.confidence : 100,
          assumptions: Array.isArray(s.assumptions) ? s.assumptions : [],
          edited: true,
        };
      });
      const triggers = steps.filter((s) => s.type === 'trigger');
      if (triggers.length === 0) throw new Error('A workflow needs exactly one trigger step — none found.');
      if (triggers.length > 1) throw new Error('Only one trigger step is allowed.');
      const laidOut = autoLayout(steps);
      setDraft({ ...draft, name: parsed.name || draft.name, description: parsed.description ?? draft.description, steps: laidOut });
      setEdges(chainEdges(laidOut));
      setViewMode('designer');
      toast.success('JSON applied — canvas updated', { description: 'Steps execute top-to-bottom in array order.' });
    } catch (e) {
      setJsonError(e instanceof Error ? e.message : 'Invalid JSON');
    }
  };

  const onStepsChange = useCallback((steps: WorkflowStep[], nextEdges: DraftEdge[]) => {
    setDraft((d) => (d ? { ...d, steps } : d));
    setEdges(nextEdges);
  }, []);

  const onEditStep = useCallback((s: WorkflowStep) => setEditing(s), []);

  // Close the panel if the step being edited was removed from the canvas
  useEffect(() => {
    if (editing && draft && !draft.steps.some((s) => s.id === editing.id)) setEditing(null);
  }, [draft, editing]);

  const addFromPalette = useCallback((type: string) => {
    setDraft((d) => {
      if (!d) return d;
      const newStep: WorkflowStep = {
        id: `manual_${Math.random().toString(36).slice(2, 7)}`,
        type, name: `New ${controlMap[type]?.label ?? type}`, params: {},
        confidence: 100, assumptions: [], edited: true,
        position: { x: 80, y: 30 + d.steps.length * 118 },
      };
      const next = [...d.steps, newStep];
      setEdges(chainEdges(next));
      return { ...d, steps: next };
    });
    toast.success(`${controlMap[type]?.label ?? type} step added — click it to configure`);
  }, [controlMap]);

  const rearrange = () => {
    if (!draft) return;
    setDraft({ ...draft, steps: autoLayout(draft.steps.map((s) => ({ ...s, position: undefined }))) });
  };

  const patchStep = (id: string, patch: Partial<WorkflowStep>) => {
    if (!draft) return;
    setDraft({ ...draft, steps: draft.steps.map((s) => (s.id === id ? { ...s, ...patch, edited: true } : s)) });
  };

  const approve = async () => {
    if (!draft) return;
    try {
      const wf = await createWorkflowFromDraft(draft, prompt);
      await approveAndDeploy(wf.id);
      setApproved({ ...wf, status: 'deployed', approvedBy: 'You (reviewer)' });
      toast.success('Workflow approved & deployed', { description: 'Nothing executed until a human approved this AI draft — that approval is now on the audit trail.' });
    } catch (e) {
      toast.error('Could not approve & deploy', { description: e instanceof Error ? e.message : 'please try again' });
    }
  };

  const saveVersion = async () => {
    if (!draft || !editSource) return;
    const nextVersion = editSource.version + 1;
    try {
      await updateWorkflow(editSource.id, { steps: draft.steps, description: draft.description, version: nextVersion });
      setEditSource({ ...editSource, version: nextVersion });
      logAudit('You', `Saved v${nextVersion}`, `${editSource.name} — edited in the designer`, 'approval');
      toast.success(`Saved as version ${nextVersion}`, { description: 'The API now serves the new version; in-flight executions finish on the old one.' });
    } catch (e) {
      toast.error('Could not save version', { description: e instanceof Error ? e.message : 'please try again' });
    }
  };

  const discardEdit = () => {
    setEditSource(null);
    setDraft(null);
    setEdges([]);
    onEditDone?.();
  };

  const exportYAML = () => {
    if (!approved && !draft) return;
    const wf: Workflow = approved ?? {
      id: 'draft', name: draft!.name, description: draft!.description, prompt, status: 'draft', version: 1,
      steps: draft!.steps, createdBy: 'You', aiModel: draft!.model, createdAt: 'now', runs: 0,
    };
    const fname = `${wf.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}.flow.yaml`;
    download(fname, toYAML(wf));
    logAudit('You', 'Exported workflow', `${fname} — portable flowforge/v1 artifact`, 'export');
    toast.success('Workflow downloaded', { description: 'This file runs anywhere with the standalone runner — no control plane needed.' });
  };

  const exportRunner = () => {
    if (!approved) return;
    download(`${approved.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}-runner.md`, runnerReadme(approved), 'text/markdown');
    toast.success('Runner package notes downloaded');
  };

  const assumptions = draft?.steps.flatMap((s) => s.assumptions.map((a) => ({ step: s.name, text: a }))) ?? [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Workflow Studio</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Describe the process in plain language. AI drafts the workflow — <span className="font-medium text-foreground">you review, edit, and approve</span>. Nothing runs without your sign-off.
        </p>
      </div>

      {/* Prompt panel — collapses to a slim bar once a workflow is on the canvas */}
      {!promptOpen && (
        <div className="flex items-center gap-3 rounded-xl border bg-card px-5 py-3 shadow-sm">
          <Wand2 className="h-4 w-4 shrink-0 text-violet-600" />
          <span className="flex-1 truncate text-sm text-muted-foreground">{prompt || 'Describe your workflow in plain language'}</span>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setPromptOpen(true)}>
            <Pencil className="h-3.5 w-3.5" /> Edit prompt
          </Button>
        </div>
      )}
      <div className={cn('rounded-xl border bg-card p-5 shadow-sm', !promptOpen && 'hidden')}>
        <div className="flex items-center gap-2 mb-3">
          <Wand2 className="h-4 w-4 text-violet-600" />
          <span className="text-sm font-semibold">Describe your workflow</span>
          <Badge variant="outline" className="ml-auto text-[11px]">model-agnostic · runs on local LLMs too</Badge>
        </div>
        <Textarea
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="e.g. When a vendor invoice over $10K arrives, extract line items, validate against the vendor master, route to the cost-center manager for approval…"
          className="min-h-[96px] resize-y text-sm"
        />
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Button onClick={() => generate(prompt)} disabled={!prompt.trim() || phase >= 0} className="gap-2">
            <Sparkles className="h-4 w-4" />
            {phase >= 0 ? 'Generating…' : 'Generate workflow'}
          </Button>
          <span className="text-xs text-muted-foreground">or try:</span>
          {SAMPLE_PROMPTS.map((s, i) => (
            <button
              key={i}
              onClick={() => setPrompt(s)}
              className="rounded-full border bg-muted/50 px-3 py-1 text-[11px] text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
            >
              Sample {i + 1}
            </button>
          ))}
        </div>
        {phase >= 0 && (
          <div className="mt-4 flex items-center gap-3 rounded-lg bg-violet-50 border border-violet-200 px-4 py-3">
            <Sparkles className="h-4 w-4 text-violet-600 animate-pulse" />
            <div className="flex gap-4">
              {AI_PHASES.map((p, i) => (
                <span key={p} className={cn('text-xs transition-opacity', i < phase ? 'text-violet-700 font-medium' : 'text-violet-400 opacity-50')}>
                  {i < phase ? '✓ ' : ''}{p}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Review mode — drag & drop designer */}
      {draft && !approved && (
        <div className={cn(
          'rounded-xl border-2 border-violet-200 bg-card shadow-sm overflow-hidden flex flex-col',
          expanded && 'fixed inset-0 z-50 rounded-none border-0',
        )}>
          <div className="flex flex-wrap items-center gap-3 border-b bg-violet-50/60 px-5 py-3">
            <ShieldCheck className="h-5 w-5 text-violet-700" />
            <div>
              <div className="font-semibold text-sm">
                {editSource ? `Editing — ${draft.name} · v${editSource.version}` : `Review mode — ${draft.name}`}
              </div>
              <div className="text-[11px] text-muted-foreground">
                {editSource
                  ? `${draft.steps.length} steps · drag to rearrange, click a step to edit its details · saving creates a new version`
                  : `${draft.model} · ${draft.steps.length} steps · drag to rearrange, click a step to edit its details`}
              </div>
            </div>
            <div className="ml-auto flex items-center gap-2">
              <ConfidenceBadge value={draft.overallConfidence} />
              <div className="flex overflow-hidden rounded-lg border bg-white">
                <button
                  onClick={() => setViewMode('designer')}
                  className={cn('flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium transition-colors',
                    viewMode === 'designer' ? 'bg-violet-600 text-white' : 'text-slate-600 hover:bg-slate-50')}
                >
                  <LayoutGrid className="h-3.5 w-3.5" /> Designer
                </button>
                <button
                  onClick={openJSON}
                  className={cn('flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium transition-colors',
                    viewMode === 'json' ? 'bg-violet-600 text-white' : 'text-slate-600 hover:bg-slate-50')}
                >
                  <Braces className="h-3.5 w-3.5" /> JSON
                </button>
              </div>
              {viewMode === 'designer' && (
                <Button variant="outline" size="sm" className="gap-1.5" onClick={rearrange}>
                  <LayoutGrid className="h-3.5 w-3.5" /> Auto-arrange
                </Button>
              )}
              <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setExpanded(!expanded)} title={expanded ? 'Exit full screen (Esc)' : 'Expand editor full screen'}>
                {expanded ? <Minimize2 className="h-3.5 w-3.5" /> : <Maximize2 className="h-3.5 w-3.5" />}
                {expanded ? 'Exit' : 'Expand'}
              </Button>
              <Button variant="outline" size="sm" className="gap-1.5" onClick={exportYAML}>
                <Download className="h-3.5 w-3.5" /> Export YAML
              </Button>
              {editSource ? (
                <>
                  <Button variant="ghost" size="sm" onClick={discardEdit}>Discard</Button>
                  <Button size="sm" className="gap-1.5 bg-violet-600 hover:bg-violet-700" onClick={saveVersion}>
                    <CheckCircle2 className="h-3.5 w-3.5" /> Save as v{editSource.version + 1}
                  </Button>
                </>
              ) : (
                <Button size="sm" className="gap-1.5 bg-violet-600 hover:bg-violet-700" onClick={approve}>
                  <CheckCircle2 className="h-3.5 w-3.5" /> Approve & deploy
                </Button>
              )}
            </div>
          </div>

          {viewMode === 'json' ? (
            <div className={cn('flex flex-col gap-3 p-5', expanded && 'flex-1 min-h-0')} style={expanded ? undefined : { height: 560 }}>
              <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
                <Braces className="h-3.5 w-3.5" />
                {jsonEditing
                  ? 'Editing raw JSON — steps execute top-to-bottom in array order. Apply validates before touching the canvas.'
                  : 'Structured view of the workflow definition — expand nodes to inspect, or switch to raw editing.'}
                {!jsonEditing && (
                  <Button variant="outline" size="sm" className="ml-auto h-7 gap-1.5 text-xs" onClick={() => { setJsonText(draftToJSON(draft)); setJsonError(null); setJsonEditing(true); }}>
                    <Pencil className="h-3 w-3" /> Edit raw JSON
                  </Button>
                )}
              </div>
              {jsonEditing ? (
                <>
                  <Textarea
                    value={jsonText}
                    onChange={(e) => { setJsonText(e.target.value); setJsonError(null); }}
                    spellCheck={false}
                    className="flex-1 resize-none font-mono text-xs leading-relaxed"
                  />
                  {jsonError && (
                    <div className="flex items-start gap-2 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
                      <AlertTriangle className="h-3.5 w-3.5 mt-px shrink-0" />
                      <span className="font-mono">{jsonError}</span>
                    </div>
                  )}
                  <div className="flex items-center gap-2">
                    <Button size="sm" className="gap-1.5 bg-violet-600 hover:bg-violet-700" onClick={applyJSON}>
                      <CheckCircle2 className="h-3.5 w-3.5" /> Apply to canvas
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => { setJsonText(draftToJSON(draft)); setJsonError(null); }}>
                      Revert changes
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => setJsonEditing(false)}>
                      Back to structured view
                    </Button>
                  </div>
                </>
              ) : (
                <JsonTree data={JSON.parse(draftToJSON(draft))} className="flex-1 min-h-0" />
              )}
            </div>
          ) : (
          <div className={cn('relative flex', expanded && 'flex-1 min-h-0')} style={expanded ? undefined : { height: 620 }}>
            <StepPalette onAdd={addFromPalette} />
            <div className="flex-1">
              <FlowCanvas
                key={expanded ? 'full' : 'docked'}
                steps={draft.steps}
                edges={edges}
                review
                onStepsChange={onStepsChange}
                onEditStep={onEditStep}
              />
            </div>
            {editing && (
              <StepPanel
                step={editing}
                onChange={setEditing}
                onClose={() => setEditing(null)}
                onSave={() => {
                  patchStep(editing.id, { name: editing.name, params: editing.params, confidence: 100, assumptions: [] });
                  setEditing(null);
                  toast.success('Step updated — marked as human-confirmed');
                }}
                onDelete={() => {
                  if (!draft) return;
                  const kept = draft.steps.filter((s) => s.id !== editing.id);
                  setDraft({ ...draft, steps: kept });
                  setEdges(chainEdges(kept));
                  setEditing(null);
                  toast.success('Step deleted');
                }}
              />
            )}
          </div>
          )}

          {/* Assumptions checklist */}
          {assumptions.length > 0 && (
            <div className="border-t bg-amber-50/50 px-5 py-3">
              <div className="text-xs font-semibold text-amber-800 mb-1.5 flex items-center gap-1.5">
                <AlertTriangle className="h-3.5 w-3.5" /> {assumptions.length} AI assumption{assumptions.length > 1 ? 's' : ''} to confirm before approving
              </div>
              <div className="grid gap-1 md:grid-cols-2">
                {assumptions.map((a, i) => (
                  <div key={i} className="text-[11px] text-amber-800 flex gap-1.5">
                    <span className="text-amber-400">•</span>
                    <span><span className="font-medium">{a.step}:</span> {a.text}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Approved state */}
      {approved && (
        <div className="rounded-xl border-2 border-emerald-200 bg-emerald-50/40 p-6">
          <div className="flex flex-wrap items-center gap-3">
            <CheckCircle2 className="h-6 w-6 text-emerald-600" />
            <div>
              <div className="font-semibold">{approved.name} is live</div>
              <div className="text-xs text-muted-foreground">Approved by You (reviewer) · version {approved.version} · available via API: <code className="bg-white border rounded px-1">POST /api/v1/workflows/{approved.id}/executions</code></div>
            </div>
            <div className="ml-auto flex flex-wrap gap-2">
              <Button size="sm" className="gap-1.5" onClick={() => { runWorkflow(approved.id); onGoMonitor(); }}>
                <Play className="h-3.5 w-3.5" /> Run now
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={exportYAML}>
                <Download className="h-3.5 w-3.5" /> Download .flow.yaml
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={exportRunner}>
                <Package className="h-3.5 w-3.5" /> Standalone runner
              </Button>
              <Button size="sm" variant="ghost" className="gap-1.5" onClick={() => { setApproved(null); setDraft(null); setPrompt(''); }}>
                <X className="h-3.5 w-3.5" /> New workflow
              </Button>
            </div>
          </div>
          <div className="mt-4 rounded-lg bg-white border border-emerald-200 px-4 py-3 text-xs text-muted-foreground">
            <span className="font-semibold text-foreground">Run it anywhere:</span>
            <code className="block mt-1 font-mono text-[11px]">docker run -v $(pwd):/flows flowforge/runner:1.0 run /flows/{approved.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}.flow.yaml</code>
          </div>
        </div>
      )}

    </div>
  );
}

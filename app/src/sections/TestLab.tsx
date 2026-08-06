import { useEffect, useMemo, useRef, useState } from 'react';
import { FlaskConical, Play, CheckCircle2, XCircle, AlertTriangle, ArrowDown, History } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useStore, fmtDur } from '@/lib/store';
import { StepIcon, StepStatusIcon } from '@/components/step';
import { cn } from '@/lib/utils';
import type { ControlDef, MDMEntity, StepRun, Workflow } from '@/lib/types';

// ---------------------------------------------------------------------------
// Test Lab — pre-flight validation + sandboxed test executions.
// Test runs never touch external systems and never appear in Executions.
// ---------------------------------------------------------------------------

interface Check { level: 'pass' | 'warn' | 'fail'; label: string }
interface TestRecord { id: string; wfName: string; at: string; verdict: 'PASSED' | 'FAILED'; passed: number; skipped: number; failed: number; ms: number }

const SAMPLE = JSON.stringify({ total: 24310.0, currency: 'USD', vendor_id: 'V-10293', line_items: 14, priority: 'standard' }, null, 2);

function runChecks(wf: Workflow, payload: string, mdmEntities: MDMEntity[], controls: ControlDef[]): Check[] {
  const checks: Check[] = [];
  const byKey = new Map(controls.map((c) => [c.key, c]));
  const unknown = [...new Set(wf.steps.filter((s) => !byKey.has(s.type)).map((s) => s.type))];
  const disabledUsed = [...new Set(wf.steps.filter((s) => byKey.get(s.type)?.enabled === false).map((s) => byKey.get(s.type)!.label))];
  if (unknown.length) {
    checks.push({ level: 'fail', label: `Unregistered control: ${unknown.join(', ')} — register it in Admin → Step controls` });
  } else {
    checks.push(disabledUsed.length === 0
      ? { level: 'pass', label: 'All step controls enabled by admin' }
      : { level: 'fail', label: `Uses disabled control: ${disabledUsed.join(', ')}` });
  }
  checks.push(wf.steps.some((s) => s.type === 'trigger')
    ? { level: 'pass', label: 'Trigger defined' }
    : { level: 'fail', label: 'No trigger step — workflow cannot start' });

  const unconfigured = wf.steps.filter((s) =>
    s.name.startsWith('New ') || Object.entries(s.params).some(([k, v]) => k === 'new_key' || !String(v).trim()));
  checks.push(unconfigured.length === 0
    ? { level: 'pass', label: 'All steps configured' }
    : { level: 'fail', label: `Unconfigured: ${unconfigured.map((s) => s.name).join(', ')}` });

  try { JSON.parse(payload); checks.push({ level: 'pass', label: 'Test payload is valid JSON' }); }
  catch { checks.push({ level: 'fail', label: 'Test payload is not valid JSON' }); }

  const mdmRefs = wf.steps.filter((s) => s.type.startsWith('mdm.')).map((s) => s.params.entity).filter(Boolean);
  const missing = mdmRefs.filter((e) => !mdmEntities.some((m) => m.key === e));
  checks.push(missing.length === 0
    ? { level: 'pass', label: mdmRefs.length ? 'MDM entity references resolve' : 'No MDM references' }
    : { level: 'fail', label: `Unknown MDM entity: ${missing.join(', ')}` });

  const firstWrite = wf.steps.findIndex((s) => s.type.startsWith('integration.'));
  const firstGate = wf.steps.findIndex((s) => s.type === 'human.approval');
  if (firstWrite !== -1 && (firstGate === -1 || firstGate > firstWrite)) {
    checks.push({ level: 'warn', label: 'External write without a prior human approval gate' });
  } else {
    checks.push({ level: 'pass', label: 'Human gate precedes external writes' });
  }
  return checks;
}

export default function TestLab() {
  const { workflows, mdm, controls } = useStore();
  const runnable = workflows.filter((w) => w.status !== 'draft');
  const [wfId, setWfId] = useState(runnable[0]?.id ?? '');
  const wf = workflows.find((w) => w.id === wfId) ?? runnable[0];
  const [payload, setPayload] = useState(SAMPLE);
  const [recordRef, setRecordRef] = useState('vendors/V-10293');
  const [dryRun, setDryRun] = useState(true);
  const [testRuns, setTestRuns] = useState<StepRun[]>([]);
  const [status, setStatus] = useState<'idle' | 'running' | 'done'>('idle');
  const [history, setHistory] = useState<TestRecord[]>([]);
  const runsRef = useRef<StepRun[]>([]);
  const autoApproveRef = useRef(false);
  const payloadRef = useRef<Record<string, unknown>>({});
  const startedRef = useRef(0);

  const checks = useMemo(() => (wf ? runChecks(wf, payload, mdm, controls) : []), [wf, payload, mdm, controls]);
  const hasFail = checks.some((c) => c.level === 'fail');

  const allRecords = mdm.flatMap((e) => e.records.map((r) => ({ value: `${e.key}/${r.id}`, label: `${e.key}/${r.id} — ${r.name ?? r.id}` })));

  const start = () => {
    if (!wf || hasFail || status === 'running') return;
    try { payloadRef.current = JSON.parse(payload); } catch { return; }
    autoApproveRef.current = false;
    startedRef.current = Date.now();
    runsRef.current = wf.steps.map((s) => ({ stepId: s.id, name: s.name, type: s.type, status: 'pending' as const }));
    setTestRuns(runsRef.current);
    setStatus('running');
  };

  useEffect(() => {
    if (status !== 'running' || !wf) return;
    const t = setInterval(() => {
      const runs = runsRef.current.map((r) => ({ ...r }));
      const idx = runs.findIndex((r) => r.status === 'running' || r.status === 'pending');
      if (idx === -1) {
        clearInterval(t);
        setStatus('done');
        const passed = runs.filter((r) => r.status === 'succeeded').length;
        const skipped = runs.filter((r) => r.status === 'skipped').length;
        const failed = runs.filter((r) => r.status === 'failed').length;
        setHistory((h) => [{
          id: `test-${Math.random().toString(36).slice(2, 6)}`, wfName: wf.name, at: 'just now',
          verdict: failed ? 'FAILED' : 'PASSED', passed, skipped, failed, ms: Date.now() - startedRef.current,
        }, ...h]);
        setTestRuns(runs);
        return;
      }
      const cur = runs[idx];
      if (cur.status === 'pending') {
        cur.status = 'running';
      } else {
        const step = wf.steps.find((s) => s.id === cur.stepId);
        const p = payloadRef.current;
        cur.durationMs = cur.type === 'human.approval' ? 1500 : 80 + idx * 45;
        if (cur.type === 'condition') {
          const m = step?.params.expression?.match(/>\s*([\d.]+)/);
          const total = Number(p.total ?? 0);
          const threshold = Number(m?.[1] ?? 0);
          if (total > threshold) {
            cur.status = 'succeeded';
            cur.output = `${step?.params.expression} → true (total ${total.toLocaleString()})`;
          } else {
            cur.status = 'succeeded';
            cur.output = `${step?.params.expression} → false · auto-approve path`;
            autoApproveRef.current = true;
          }
        } else if (cur.type === 'human.approval' && autoApproveRef.current) {
          cur.status = 'skipped';
          cur.output = 'auto-approved — condition below threshold';
          cur.durationMs = 4;
        } else {
          cur.status = 'succeeded';
          cur.output =
            cur.type === 'trigger' ? 'test payload received (sandbox)' :
            cur.type === 'ai.extract' ? `extracted ${p.line_items ?? '—'} line items · total $${Number(p.total ?? 0).toLocaleString()}` :
            cur.type === 'ai.classify' ? `classified: ${String(p.priority ?? 'standard')}` :
            cur.type === 'mdm.validate' ? `validated against ${recordRef} (golden record, test snapshot)` :
            cur.type === 'mdm.lookup' ? `resolved ${recordRef} (test snapshot)` :
            cur.type === 'human.approval' ? `approved by ${step?.params.approver ?? 'approver'} (simulated test user)` :
            cur.type === 'notify' ? (dryRun ? `mocked — no ${step?.params.channel ?? 'email'} sent (dry run)` : `sent via ${step?.params.channel ?? 'email'} (test)`) :
            cur.type === 'integration.post' ? (dryRun ? `mocked — would POST ${step?.params.endpoint ?? step?.params.system ?? ''} (dry run)` : `posted to ${step?.params.system ?? 'target'} (test) · 200 OK`) :
            cur.type === 'integration.http' ? (dryRun ? 'mocked HTTP call (dry run)' : 'HTTP 200 (test)') :
            'done (test)';
        }
      }
      runsRef.current = runs;
      setTestRuns(runs);
    }, 750);
    return () => clearInterval(t);
  }, [status, wf, dryRun, recordRef]);

  const summary = useMemo(() => ({
    passed: testRuns.filter((r) => r.status === 'succeeded').length,
    skipped: testRuns.filter((r) => r.status === 'skipped').length,
    failed: testRuns.filter((r) => r.status === 'failed').length,
    totalMs: testRuns.reduce((a, r) => a + (r.durationMs ?? 0), 0),
  }), [testRuns]);

  const CheckIcon = ({ level }: { level: Check['level'] }) =>
    level === 'pass' ? <CheckCircle2 className="h-3.5 w-3.5 text-emerald-600" /> :
    level === 'warn' ? <AlertTriangle className="h-3.5 w-3.5 text-amber-500" /> :
    <XCircle className="h-3.5 w-3.5 text-rose-600" />;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
          Test Lab <FlaskConical className="h-5 w-5 text-violet-600" />
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          Pre-flight validation + sandboxed executions. Test runs never touch external systems and never appear in Executions or the audit trail.
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-5">
        {/* Config */}
        <div className="lg:col-span-2 space-y-4">
          <div className="rounded-xl border bg-card p-5 shadow-sm space-y-4">
            <div>
              <label className="text-xs font-semibold text-muted-foreground">Workflow</label>
              <Select value={wf?.id ?? ''} onValueChange={(v) => { setWfId(v); setTestRuns([]); setStatus('idle'); }}>
                <SelectTrigger className="mt-1 h-9 text-sm"><SelectValue placeholder="Select workflow" /></SelectTrigger>
                <SelectContent>
                  {runnable.map((w) => <SelectItem key={w.id} value={w.id} className="text-sm">{w.name} · v{w.version}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div>
              <label className="text-xs font-semibold text-muted-foreground">Test entity (MDM snapshot)</label>
              <Select value={recordRef} onValueChange={setRecordRef}>
                <SelectTrigger className="mt-1 h-9 text-sm"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {allRecords.map((r) => <SelectItem key={r.value} value={r.value} className="text-sm font-mono">{r.label}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div>
              <label className="text-xs font-semibold text-muted-foreground">Input payload (JSON)</label>
              <Textarea value={payload} onChange={(e) => setPayload(e.target.value)} className="mt-1 min-h-[130px] font-mono text-xs" />
              <div className="mt-1 text-[10px] text-muted-foreground">
                Tip: set <code className="bg-muted px-1 rounded">"total": 5000</code> to test the auto-approve path (below the $10K threshold).
              </div>
            </div>
            <div className="flex items-center justify-between rounded-lg border bg-muted/40 px-3 py-2.5">
              <div>
                <div className="text-xs font-semibold">Dry run</div>
                <div className="text-[10px] text-muted-foreground">Mock notifications & external writes</div>
              </div>
              <Switch checked={dryRun} onCheckedChange={setDryRun} />
            </div>
            <Button className="w-full gap-2" disabled={!wf || hasFail || status === 'running'} onClick={start}>
              <Play className="h-4 w-4" /> {status === 'running' ? 'Test running…' : 'Run test'}
            </Button>
          </div>

          {/* Pre-flight */}
          <div className="rounded-xl border bg-card p-5 shadow-sm">
            <div className="text-xs font-bold uppercase tracking-wide text-muted-foreground mb-2">Pre-flight checks</div>
            <div className="space-y-1.5">
              {checks.map((c, i) => (
                <div key={i} className="flex items-center gap-2 text-xs">
                  <CheckIcon level={c.level} />
                  <span className={cn(c.level === 'fail' && 'text-rose-700 font-medium', c.level === 'warn' && 'text-amber-700')}>{c.label}</span>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Results */}
        <div className="lg:col-span-3 space-y-4">
          <div className="rounded-xl border bg-card p-5 shadow-sm min-h-[420px]">
            <div className="flex items-center gap-2 border-b pb-3">
              <span className="font-semibold text-sm">Test execution</span>
              {status === 'running' && <Badge className="bg-indigo-100 text-indigo-700 border-indigo-200 text-[10px]">running</Badge>}
              {status === 'done' && (
                <Badge className={cn('text-[10px]', summary.failed ? 'bg-rose-100 text-rose-700 border-rose-200' : 'bg-emerald-100 text-emerald-700 border-emerald-200')}>
                  {summary.failed ? 'FAILED' : 'PASSED'} · {summary.passed} passed · {summary.skipped} skipped · {fmtDur(summary.totalMs)}
                </Badge>
              )}
              {dryRun && <Badge variant="outline" className="ml-auto text-[10px]">dry run — all external effects mocked</Badge>}
            </div>

            {testRuns.length === 0 ? (
              <div className="flex h-[340px] flex-col items-center justify-center text-center">
                <FlaskConical className="h-8 w-8 text-slate-300" />
                <div className="mt-3 text-sm font-medium text-slate-500">No test run yet</div>
                <p className="mt-1 max-w-xs text-xs text-muted-foreground">
                  Pick a workflow, tune the payload, and hit Run test. You'll see each step resolve against your test data.
                </p>
              </div>
            ) : (
              <div className="mt-4">
                {testRuns.map((sr, i) => (
                  <div key={sr.stepId}>
                    {i > 0 && <div className="ml-[26px] py-0.5"><ArrowDown className="h-3.5 w-3.5 text-slate-300" /></div>}
                    <div className={cn(
                      'flex items-start gap-3 rounded-lg border p-3',
                      sr.status === 'running' && 'border-indigo-300 bg-indigo-50/40',
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
                      </div>
                      <StepStatusIcon status={sr.status} />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* History */}
          {history.length > 0 && (
            <div className="rounded-xl border bg-card p-5 shadow-sm">
              <div className="flex items-center gap-2 mb-3">
                <History className="h-4 w-4 text-slate-500" />
                <span className="font-semibold text-sm">Test history</span>
              </div>
              <div className="space-y-1.5">
                {history.map((h) => (
                  <div key={h.id} className="flex items-center gap-3 rounded-lg border px-3 py-2 text-xs">
                    <span className="font-mono text-muted-foreground">{h.id}</span>
                    <span className="font-medium flex-1 truncate">{h.wfName}</span>
                    <span className="text-muted-foreground">{h.passed}✓ {h.skipped}⤼ {h.failed > 0 ? `${h.failed}✗` : ''} · {fmtDur(h.ms)}</span>
                    <span className={cn('rounded-full border px-2 py-0.5 text-[10px] font-bold',
                      h.verdict === 'PASSED' ? 'bg-emerald-50 text-emerald-700 border-emerald-200' : 'bg-rose-50 text-rose-700 border-rose-200')}>
                      {h.verdict}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

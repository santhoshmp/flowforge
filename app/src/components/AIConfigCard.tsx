import { useEffect, useState } from 'react';
import { Sparkles, KeyRound, PlugZap, CheckCircle2, XCircle, Eye, EyeOff, Save, Loader2, Server, Cpu } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { api, type AISettings, type AITestResult } from '@/lib/api';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';

// Admin → AI model configuration. Pick a provider (incl. local LLMs), set the
// API key / base URL / model, test the connection, and save. Changes take effect
// immediately for authoring (no restart).

interface Props {
  onSaved?: () => void;
}

interface Form {
  provider: string;
  baseURL: string;
  model: string;
  apiKey: string;
}

export default function AIConfigCard({ onSaved }: Props) {
  const [settings, setSettings] = useState<AISettings | null>(null);
  const [form, setForm] = useState<Form>({ provider: 'ollama', baseURL: '', model: '', apiKey: '' });
  const [showKey, setShowKey] = useState(false);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<AITestResult | null>(null);

  const load = async () => {
    const s = await api.getAISettings();
    setSettings(s);
    setForm({ provider: s.provider, baseURL: s.baseURL, model: s.model, apiKey: '' });
  };
  useEffect(() => { load().catch((e) => toast.error('Could not load AI settings', { description: String(e.message ?? e) })); }, []);

  if (!settings) {
    return (
      <div className="rounded-xl border bg-card p-5 shadow-sm">
        <div className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" /> Loading AI settings…</div>
      </div>
    );
  }

  const preset = settings.providers.find((p) => p.id === form.provider);
  const needsKey = preset?.needsKey ?? true;

  const onProvider = (id: string) => {
    const p = settings.providers.find((x) => x.id === id);
    setForm({ provider: id, baseURL: p?.baseURL ?? '', model: p?.defaultModel ?? '', apiKey: '' });
    setTestResult(null);
  };

  const payload = () => ({
    provider: form.provider,
    baseURL: form.baseURL,
    model: form.model,
    apiKey: needsKey && form.apiKey.trim() ? form.apiKey.trim() : undefined,
  });

  const save = async () => {
    setSaving(true);
    try {
      const s = await api.setAISettings(payload());
      setSettings(s);
      setForm((f) => ({ ...f, apiKey: '' }));
      setTestResult(null);
      onSaved?.();
      toast.success('AI model saved', { description: s.activeLabel });
    } catch (e) {
      toast.error('Could not save', { description: String((e as Error).message ?? e) });
    } finally {
      setSaving(false);
    }
  };

  const test = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await api.testAISettings(payload());
      setTestResult(res);
      if (res.ok) toast.success('Connection OK', { description: res.note ?? `${res.model ?? 'model'} reachable${res.latencyMs ? ` · ${res.latencyMs}ms` : ''}` });
      else toast.error('Connection failed', { description: res.error ?? 'unknown error' });
    } catch (e) {
      setTestResult({ ok: false, error: String((e as Error).message ?? e) });
      toast.error('Connection failed', { description: String((e as Error).message ?? e) });
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="rounded-xl border bg-card p-5 shadow-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="inline-flex h-8 w-8 items-center justify-center rounded-lg border bg-violet-50 border-violet-200 text-violet-700"><Sparkles className="h-4 w-4" /></span>
        <div className="min-w-0">
          <div className="text-sm font-semibold">AI authoring model</div>
          <div className="text-[11px] text-muted-foreground">Provider, API key & model for “describe → draft”. Model-agnostic · any OpenAI-compatible endpoint.</div>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Badge variant="outline" className={cn('gap-1 text-[11px]', settings.active ? 'border-emerald-200 text-emerald-700 bg-emerald-50' : 'border-amber-200 text-amber-700 bg-amber-50')}>
            <span className={cn('h-1.5 w-1.5 rounded-full', settings.active ? 'bg-emerald-500' : 'bg-amber-500')} />
            {settings.active ? 'Active' : 'Fallback'}
          </Badge>
          <span className="text-[11px] text-muted-foreground truncate max-w-[220px]">{settings.activeLabel}</span>
        </div>
      </div>

      <div className="mt-4 grid gap-4 md:grid-cols-2">
        {/* Provider */}
        <div>
          <label className="text-xs font-semibold text-muted-foreground">Provider</label>
          <Select value={form.provider} onValueChange={onProvider}>
            <SelectTrigger className="mt-1 h-9 text-sm"><SelectValue /></SelectTrigger>
            <SelectContent>
              {settings.providers.map((p) => (
                <SelectItem key={p.id} value={p.id} className="text-sm">
                  <span className="flex items-center gap-2">
                    {p.local ? <Server className="h-3.5 w-3.5 text-emerald-600" /> : <Cpu className="h-3.5 w-3.5 text-slate-500" />}
                    {p.label}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {preset && <div className="mt-1 text-[10px] text-muted-foreground">{preset.hint}</div>}
        </div>

        {/* Model */}
        <div>
          <label className="text-xs font-semibold text-muted-foreground">Model</label>
          <Input
            list="ai-models"
            value={form.model}
            onChange={(e) => { setForm({ ...form, model: e.target.value }); setTestResult(null); }}
            className="mt-1 h-9 text-sm font-mono"
            placeholder="e.g. gpt-4o-mini"
          />
          <datalist id="ai-models">
            {(testResult?.models ?? []).map((m) => <option key={m} value={m} />)}
          </datalist>
        </div>

        {/* Base URL */}
        <div className="md:col-span-2">
          <label className="text-xs font-semibold text-muted-foreground">Base URL</label>
          <Input
            value={form.baseURL}
            onChange={(e) => { setForm({ ...form, baseURL: e.target.value }); setTestResult(null); }}
            className="mt-1 h-9 text-sm font-mono"
            placeholder="https://api.openai.com/v1"
          />
        </div>

        {/* API key */}
        {needsKey && (
          <div className="md:col-span-2">
            <label className="text-xs font-semibold text-muted-foreground">API key</label>
            <div className="mt-1 flex items-center gap-2">
              <div className="relative flex-1">
                <KeyRound className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
                <Input
                  type={showKey ? 'text' : 'password'}
                  value={form.apiKey}
                  onChange={(e) => { setForm({ ...form, apiKey: e.target.value }); setTestResult(null); }}
                  className="h-9 pl-8 pr-9 text-sm font-mono"
                  placeholder={settings.hasKey ? `Leave blank to keep current (${settings.maskedKey})` : 'Paste your API key'}
                />
                <button type="button" onClick={() => setShowKey(!showKey)} className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600">
                  {showKey ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                </button>
              </div>
            </div>
            <div className="mt-1 text-[10px] text-muted-foreground">Stored locally on the server; never sent anywhere except the chosen endpoint.</div>
          </div>
        )}
        {!needsKey && (
          <div className="md:col-span-2 flex items-center gap-2 rounded-lg border border-emerald-200 bg-emerald-50/60 px-3 py-2 text-[11px] text-emerald-800">
            <Server className="h-3.5 w-3.5 shrink-0" /> Local model — no API key needed. Make sure the server (<code className="font-mono">{form.baseURL || 'local'}</code>) is running, then Test connection.
          </div>
        )}
      </div>

      {/* Test result */}
      {testResult && (
        <div className={cn('mt-4 rounded-lg border px-3 py-2.5 text-xs', testResult.ok ? 'border-emerald-200 bg-emerald-50 text-emerald-800' : 'border-rose-200 bg-rose-50 text-rose-800')}>
          <div className="flex items-center gap-1.5 font-medium">
            {testResult.ok ? <CheckCircle2 className="h-3.5 w-3.5" /> : <XCircle className="h-3.5 w-3.5" />}
            {testResult.ok ? `Connected${testResult.latencyMs ? ` · ${testResult.latencyMs}ms` : ''}` : 'Connection failed'}
          </div>
          {testResult.ok && testResult.models && testResult.models.length > 0 && (
            <div className="mt-1.5">
              <span className="text-muted-foreground">{testResult.models.length} models available.</span>
              {testResult.modelAvailable === false && <span className="ml-1 text-amber-700 font-medium">“{testResult.model}” not in the list — it may be mistyped.</span>}
              <div className="mt-1 flex flex-wrap gap-1">
                {testResult.models.slice(0, 12).map((m) => (
                  <button key={m} onClick={() => setForm({ ...form, model: m })} className="rounded border bg-white px-1.5 py-0.5 font-mono text-[10px] hover:border-violet-300 hover:text-violet-700">{m}</button>
                ))}
              </div>
            </div>
          )}
          {!testResult.ok && <div className="mt-1 font-mono text-[10px] break-words">{testResult.error}</div>}
          {testResult.note && <div className="mt-1 text-muted-foreground">{testResult.note}</div>}
        </div>
      )}

      {/* Actions */}
      <div className="mt-4 flex items-center gap-2">
        <Button size="sm" variant="outline" className="gap-1.5" onClick={test} disabled={testing}>
          {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <PlugZap className="h-3.5 w-3.5" />}
          {testing ? 'Testing…' : 'Test connection'}
        </Button>
        <Button size="sm" className="ml-auto gap-1.5 bg-violet-600 hover:bg-violet-700" onClick={save} disabled={saving}>
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />} Save
        </Button>
      </div>
    </div>
  );
}

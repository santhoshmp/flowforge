import type { GeneratedDraft } from './ai';
import type { AuditEntry, ControlDef, Instance, MDMEntity, MDMRecord, Workflow, WorkflowStep } from './types';

// Thin REST client for the FlowForge control plane. All UI state flows through
// here; the store wraps these calls and exposes the same method names the
// sections already used (now async, backend-backed, with live polling).

// Relative by default so the embedded SPA calls its own origin (works on any
// host). Set VITE_API_URL (e.g., in .env.development) for the Vite dev server.
const BASE = import.meta.env.VITE_API_URL ?? '';

// --- Auth token (local sessions for the Go control plane; opaque to Node) ---
const TOKEN_KEY = 'flowforge_token';
const USER_KEY = 'flowforge_user';
export const getToken = (): string | null => localStorage.getItem(TOKEN_KEY);
export const getUser = (): string => localStorage.getItem(USER_KEY) ?? '';
export function setSession(token: string, username: string) {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(USER_KEY, username);
}
export function clearSession() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

async function http<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json', ...(init?.headers as Record<string, string> | undefined ?? {}) };
  const tok = getToken();
  if (tok) headers.Authorization = `Bearer ${tok}`;
  const res = await fetch(`${BASE}${path}`, { ...init, headers });
  if (res.status === 401) {
    clearSession();
    window.location.reload();
    throw new Error('unauthorized');
  }
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const j = (await res.json()) as { error?: string };
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

const json = (body: unknown) => JSON.stringify(body ?? {});

export interface AuthStatus {
  setupRequired: boolean;
  authRequired: boolean;
}
export interface AuthUser {
  id: string;
  username: string;
  role: string;
}
interface AuthResponse {
  token: string;
  user: AuthUser;
}

export interface Bootstrap {
  workflows: Workflow[];
  instances: Instance[];
  audit: AuditEntry[];
  mdm: MDMEntity[];
  controls: ControlDef[];
}

export interface DraftResult {
  draft: GeneratedDraft;
  source: 'llm' | 'fallback';
  model: string;
}

export interface AIProvider {
  id: string;
  label: string;
  baseURL: string;
  defaultModel: string;
  needsKey: boolean;
  local: boolean;
  hint: string;
}

export interface AISettings {
  provider: string;
  baseURL: string;
  model: string;
  hasKey: boolean;
  maskedKey: string;
  active: boolean;
  activeLabel: string;
  needsKey: boolean;
  local: boolean;
  providers: AIProvider[];
}

export interface AITestResult {
  ok: boolean;
  latencyMs?: number;
  model?: string;
  models?: string[];
  modelAvailable?: boolean;
  note?: string;
  error?: string;
}

export interface AIPayload {
  provider: string;
  baseURL: string;
  model: string;
  apiKey?: string;
}

export interface DayBucket {
  date: string;
  completed: number;
  failed: number;
  running: number;
  waiting: number;
  cancelled: number;
  total: number;
}

export interface TemplateInfo {
  id: string;
  name: string;
  description: string;
  category: string;
  steps: number;
}

export interface ConnectorInfo {
  id: string;
  name: string;
  version: string;
  description: string;
  category: string;
  executor: string;
  authMode: string;
  paramsSchema?: Record<string, unknown>;
  builtin: boolean;
  dir?: string;
}

export interface WorkflowMetric {
  id: string;
  name: string;
  status: string;
  runs: number;
  completed: number;
  failed: number;
  running: number;
  waiting: number;
  cancelled: number;
  successRate: number | null;
  avgDurationMs: number | null;
  lastRunIso: string | null;
}

export interface Metrics {
  fleet: {
    workflows: number;
    deployed: number;
    totalRuns: number;
    running: number;
    waiting: number;
    failed: number;
    completed: number;
    cancelled: number;
    successRate: number | null;
    avgDurationMs: number | null;
    humanTasksPending: number;
  };
  byDay: DayBucket[];
  statusMix: { status: string; count: number }[];
  workflows: WorkflowMetric[];
}

export const api = {
  health: () => http<{ status: string; model: string }>('/api/v1/health'),
  bootstrap: () => http<Bootstrap>('/api/v1/bootstrap'),
  metrics: () => http<Metrics>('/api/v1/metrics'),
  workflowExecutions: (id: string) => http<Instance[]>(`/api/v1/workflows/${id}/executions`),

  authStatus: () => http<AuthStatus>('/api/v1/auth/status'),
  authMe: () => http<AuthUser>('/api/v1/auth/me'),
  authSetup: async (username: string, password: string) => {
    const r = await http<AuthResponse>('/api/v1/auth/setup', { method: 'POST', body: json({ username, password }) });
    setSession(r.token, r.user.username);
    return r.user;
  },
  authLogin: async (username: string, password: string) => {
    const r = await http<AuthResponse>('/api/v1/auth/login', { method: 'POST', body: json({ username, password }) });
    setSession(r.token, r.user.username);
    return r.user;
  },

  aiDraft: (prompt: string) =>
    http<DraftResult>('/api/v1/ai/draft', { method: 'POST', body: json({ prompt }) }),

  createWorkflow: (payload: { name: string; description: string; prompt: string; steps: WorkflowStep[]; aiModel?: string }) =>
    http<Workflow>('/api/v1/workflows', { method: 'POST', body: json(payload) }),
  updateWorkflow: (id: string, patch: Partial<Workflow>) =>
    http<Workflow>(`/api/v1/workflows/${id}`, { method: 'PATCH', body: json(patch) }),
  approveWorkflow: (id: string) =>
    http<Workflow>(`/api/v1/workflows/${id}/approve`, { method: 'POST', body: '{}' }),
  runWorkflow: (id: string, entity?: string, input?: Record<string, unknown>) =>
    http<Instance>(`/api/v1/workflows/${id}/executions`, { method: 'POST', body: json({ ...(entity ? { entity } : {}), ...(input ? { input } : {}) }) }),

  getExecutions: () => http<Instance[]>('/api/v1/executions'),
  getInstance: (id: string) => http<Instance>(`/api/v1/executions/${id}`),
  approveTask: (id: string) => http<Instance>(`/api/v1/executions/${id}/approve`, { method: 'POST', body: '{}' }),
  retryInstance: (id: string) => http<Instance>(`/api/v1/executions/${id}/retry`, { method: 'POST', body: '{}' }),
  cancelInstance: (id: string) => http<Instance>(`/api/v1/executions/${id}/cancel`, { method: 'POST', body: '{}' }),

  getMDM: () => http<MDMEntity[]>('/api/v1/mdm'),
  addMDMRecord: (entityKey: string, rec: MDMRecord) =>
    http<MDMEntity>(`/api/v1/mdm/${entityKey}`, { method: 'POST', body: json({ record: rec }) }),

  getControls: () => http<ControlDef[]>('/api/v1/controls'),
  addControl: (def: ControlDef) => http<ControlDef>('/api/v1/controls', { method: 'POST', body: json(def) }),
  updateControl: (key: string, patch: Partial<ControlDef>) =>
    http<ControlDef>(`/api/v1/controls/${key}`, { method: 'PATCH', body: json(patch) }),
  toggleControl: (key: string) => http<ControlDef>(`/api/v1/controls/${key}/toggle`, { method: 'POST', body: '{}' }),
  removeControl: (key: string) => http<{ ok: boolean }>(`/api/v1/controls/${key}`, { method: 'DELETE' }),

  getAudit: () => http<AuditEntry[]>('/api/v1/audit'),
  logAudit: (actor: string, action: string, detail: string, kind: AuditEntry['kind']) =>
    http<AuditEntry>('/api/v1/audit', { method: 'POST', body: json({ actor, action, detail, kind }) }),

  getAISettings: () => http<AISettings>('/api/v1/settings/ai'),
  setAISettings: (cfg: AIPayload) =>
    http<AISettings>('/api/v1/settings/ai', { method: 'PUT', body: json(cfg) }),
  testAISettings: (cfg: AIPayload) =>
    http<AITestResult>('/api/v1/settings/ai/test', { method: 'POST', body: json(cfg) }),

  getTemplates: () => http<TemplateInfo[]>('/api/v1/templates'),
  instantiateTemplate: (id: string) =>
    http<Workflow>(`/api/v1/templates/${id}/instantiate`, { method: 'POST', body: '{}' }),
  getConnectors: () => http<ConnectorInfo[]>('/api/v1/connectors'),
  getSecretNames: () => http<{ names: string[] }>('/api/v1/secrets'),
};

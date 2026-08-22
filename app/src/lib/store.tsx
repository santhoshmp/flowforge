import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import type { AuditEntry, ControlDef, Instance, MDMEntity, MDMRecord, Workflow } from './types';
import type { GeneratedDraft } from './ai';
import { api, type DraftResult } from './api';

export type ControlMap = Record<string, { label: string; color: string; icon: string }>;

// ---------------------------------------------------------------------------
// Central store — a thin, backend-backed facade over the control-plane API.
// Mirrors the method names the sections already use, now async + persisted.
// A light poll keeps Executions / Admin / the sidebar badge live.
// ---------------------------------------------------------------------------

const POLL_MS = 1200;

const fmtDur = (ms: number) => (ms >= 60000 ? `${Math.round(ms / 60000)}m` : ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`);

interface Store {
  loaded: boolean;
  workflows: Workflow[];
  instances: Instance[];
  audit: AuditEntry[];
  mdm: MDMEntity[];
  controls: ControlDef[];
  controlMap: ControlMap;
  refresh: () => Promise<void>;
  generateDraft: (prompt: string) => Promise<DraftResult>;
  createWorkflowFromDraft: (draft: GeneratedDraft, prompt: string) => Promise<Workflow>;
  updateWorkflow: (id: string, patch: Partial<Workflow>) => Promise<void>;
  approveAndDeploy: (id: string) => Promise<void>;
  runWorkflow: (id: string, entity?: string, input?: Record<string, unknown>) => Promise<string>;
  approveTask: (instanceId: string) => Promise<void>;
  retryInstance: (instanceId: string) => Promise<void>;
  cancelInstance: (instanceId: string) => Promise<void>;
  addMDMRecord: (entityKey: string, rec: MDMRecord) => Promise<void>;
  instantiateTemplate: (id: string) => Promise<Workflow>;
  toggleControl: (key: string) => Promise<void>;
  addControl: (def: ControlDef) => Promise<void>;
  updateControl: (key: string, patch: Partial<ControlDef>) => Promise<void>;
  removeControl: (key: string) => Promise<void>;
  logAudit: (actor: string, action: string, detail: string, kind: AuditEntry['kind']) => Promise<void>;
}

const StoreCtx = createContext<Store | null>(null);

export function StoreProvider({ children }: { children: ReactNode }) {
  const [loaded, setLoaded] = useState(false);
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [mdm, setMdm] = useState<MDMEntity[]>([]);
  const [controls, setControls] = useState<ControlDef[]>([]);
  const inFlight = useRef(false);

  const controlMap = useMemo<ControlMap>(
    () => Object.fromEntries(controls.map((c) => [c.key, { label: c.label, color: c.color, icon: c.icon }])),
    [controls],
  );

  const refresh = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    try {
      const b = await api.bootstrap();
      setWorkflows(b.workflows);
      setInstances(b.instances);
      setAudit(b.audit);
      setMdm(b.mdm);
      setControls(b.controls);
      setLoaded(true);
    } catch (e) {
      // Stay silent on poll errors so a transient blip doesn't blank the UI;
      // the initial load failure is surfaced via `loaded` staying false.
      console.warn('[store] bootstrap failed', e);
    } finally {
      inFlight.current = false;
    }
  }, []);

  // Initial load + live polling for executions / audit / the sidebar badge.
  useEffect(() => {
    refresh();
    const t = setInterval(refresh, POLL_MS);
    const onVis = () => { if (document.visibilityState === 'visible') refresh(); };
    document.addEventListener('visibilitychange', onVis);
    return () => { clearInterval(t); document.removeEventListener('visibilitychange', onVis); };
  }, [refresh]);

  const generateDraft = useCallback((prompt: string) => api.aiDraft(prompt), []);

  const createWorkflowFromDraft = useCallback(async (draft: GeneratedDraft, prompt: string) => {
    const wf = await api.createWorkflow({ name: draft.name, description: draft.description, prompt, steps: draft.steps, aiModel: draft.model });
    await refresh();
    return wf;
  }, [refresh]);

  const updateWorkflow = useCallback(async (id: string, patch: Partial<Workflow>) => { await api.updateWorkflow(id, patch); await refresh(); }, [refresh]);
  const approveAndDeploy = useCallback(async (id: string) => { await api.approveWorkflow(id); await refresh(); }, [refresh]);
  const runWorkflow = useCallback(async (id: string, entity?: string, input?: Record<string, unknown>) => { const inst = await api.runWorkflow(id, entity, input); await refresh(); return inst.id; }, [refresh]);
  const approveTask = useCallback(async (instanceId: string) => { await api.approveTask(instanceId); await refresh(); }, [refresh]);
  const retryInstance = useCallback(async (instanceId: string) => { await api.retryInstance(instanceId); await refresh(); }, [refresh]);
  const cancelInstance = useCallback(async (instanceId: string) => { await api.cancelInstance(instanceId); await refresh(); }, [refresh]);
  const addMDMRecord = useCallback(async (entityKey: string, rec: MDMRecord) => { await api.addMDMRecord(entityKey, rec); await refresh(); }, [refresh]);
  const instantiateTemplate = useCallback(async (id: string) => { const wf = await api.instantiateTemplate(id); await refresh(); return wf; }, [refresh]);
  const toggleControl = useCallback(async (key: string) => { await api.toggleControl(key); await refresh(); }, [refresh]);
  const addControl = useCallback(async (def: ControlDef) => { await api.addControl(def); await refresh(); }, [refresh]);
  const updateControl = useCallback(async (key: string, patch: Partial<ControlDef>) => { await api.updateControl(key, patch); await refresh(); }, [refresh]);
  const removeControl = useCallback(async (key: string) => { await api.removeControl(key); await refresh(); }, [refresh]);
  const logAudit = useCallback(async (actor: string, action: string, detail: string, kind: AuditEntry['kind']) => { await api.logAudit(actor, action, detail, kind); await refresh(); }, [refresh]);

  const value = useMemo<Store>(() => ({
    loaded, workflows, instances, audit, mdm, controls, controlMap, refresh,
    generateDraft, createWorkflowFromDraft, updateWorkflow, approveAndDeploy, runWorkflow,
    approveTask, retryInstance, cancelInstance, addMDMRecord, instantiateTemplate,
    toggleControl, addControl, updateControl, removeControl, logAudit,
  }), [loaded, workflows, instances, audit, mdm, controls, controlMap, refresh, generateDraft, createWorkflowFromDraft, updateWorkflow, approveAndDeploy, runWorkflow, approveTask, retryInstance, cancelInstance, addMDMRecord, instantiateTemplate, toggleControl, addControl, updateControl, removeControl, logAudit]);

  return <StoreCtx.Provider value={value}>{children}</StoreCtx.Provider>;
}

export function useStore(): Store {
  const ctx = useContext(StoreCtx);
  if (!ctx) throw new Error('useStore outside provider');
  return ctx;
}

export { fmtDur };

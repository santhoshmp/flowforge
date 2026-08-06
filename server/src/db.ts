import Database from 'better-sqlite3';
import type { AuditEntry, ControlDef, Instance, MDMEntity, Workflow } from './types.js';

// ---------------------------------------------------------------------------
// SQLite persistence. At demo scale we store nested arrays/objects as JSON
// columns; the engine rewrites the instance row on every step transition so
// execution survives restarts.
// ---------------------------------------------------------------------------

export interface DB {
  db: Database.Database;
  // workflows
  listWorkflows: () => Workflow[];
  getWorkflow: (id: string) => Workflow | undefined;
  upsertWorkflow: (wf: Workflow) => void;
  patchWorkflow: (id: string, patch: Partial<Workflow>) => void;
  // instances
  listInstances: () => Instance[];
  getInstance: (id: string) => Instance | undefined;
  upsertInstance: (inst: Instance) => void;
  // audit
  listAudit: () => AuditEntry[];
  addAudit: (a: AuditEntry) => void;
  // mdm
  listMDM: () => MDMEntity[];
  getMDM: (key: string) => MDMEntity | undefined;
  upsertMDM: (e: MDMEntity) => void;
  // controls
  listControls: () => ControlDef[];
  upsertControl: (c: ControlDef) => void;
  deleteControl: (key: string) => void;
  close: () => void;
}

const wfFromRow = (r: any): Workflow => ({
  id: r.id,
  name: r.name,
  description: r.description,
  prompt: r.prompt,
  status: r.status,
  version: r.version,
  steps: JSON.parse(r.steps),
  createdBy: r.created_by,
  approvedBy: r.approved_by ?? undefined,
  aiModel: r.ai_model,
  createdAt: r.created_at,
  runs: r.runs,
});

const instFromRow = (r: any): Instance => ({
  id: r.id,
  workflowId: r.workflow_id,
  workflowName: r.workflow_name,
  status: r.status,
  entity: r.entity ?? undefined,
  startedAt: r.started_at,
  endedAt: r.ended_at ?? undefined,
  input: r.input ? JSON.parse(r.input) : undefined,
  autoApprove: !!r.auto_approve,
  stepRuns: JSON.parse(r.step_runs),
  currentStep: r.current_step,
  waitingOn: r.waiting_on ?? undefined,
  error: r.error ?? undefined,
});

const auditFromRow = (r: any): AuditEntry => ({
  id: r.id,
  at: r.at,
  actor: r.actor,
  action: r.action,
  detail: r.detail,
  kind: r.kind,
});

const mdmFromRow = (r: any): MDMEntity => ({
  key: r.key,
  label: r.label,
  icon: r.icon,
  fields: JSON.parse(r.fields),
  records: JSON.parse(r.records),
});

const ctrlFromRow = (r: any): ControlDef => ({
  key: r.key,
  label: r.label,
  color: r.color,
  icon: r.icon,
  enabled: !!r.enabled,
  custom: !!r.custom,
  description: r.description ?? undefined,
});

export function openDB(path: string): DB {
  const db = new Database(path);
  db.pragma('journal_mode = WAL');

  db.exec(`
    CREATE TABLE IF NOT EXISTS workflows (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      description TEXT NOT NULL,
      prompt TEXT NOT NULL,
      status TEXT NOT NULL,
      version INTEGER NOT NULL,
      steps TEXT NOT NULL,
      created_by TEXT NOT NULL,
      approved_by TEXT,
      ai_model TEXT NOT NULL,
      created_at TEXT NOT NULL,
      runs INTEGER NOT NULL DEFAULT 0
    );
    CREATE TABLE IF NOT EXISTS instances (
      id TEXT PRIMARY KEY,
      workflow_id TEXT NOT NULL,
      workflow_name TEXT NOT NULL,
      status TEXT NOT NULL,
      entity TEXT,
      started_at TEXT NOT NULL,
      step_runs TEXT NOT NULL,
      current_step INTEGER NOT NULL,
      waiting_on TEXT,
      error TEXT,
      ended_at TEXT,
      input TEXT,
      auto_approve INTEGER NOT NULL DEFAULT 0
    );
    CREATE TABLE IF NOT EXISTS audit (
      id TEXT PRIMARY KEY,
      at TEXT NOT NULL,
      actor TEXT NOT NULL,
      action TEXT NOT NULL,
      detail TEXT NOT NULL,
      kind TEXT NOT NULL
    );
    CREATE TABLE IF NOT EXISTS mdm (
      key TEXT PRIMARY KEY,
      label TEXT NOT NULL,
      icon TEXT NOT NULL,
      fields TEXT NOT NULL,
      records TEXT NOT NULL
    );
    CREATE TABLE IF NOT EXISTS controls (
      key TEXT PRIMARY KEY,
      label TEXT NOT NULL,
      color TEXT NOT NULL,
      icon TEXT NOT NULL,
      enabled INTEGER NOT NULL DEFAULT 1,
      custom INTEGER NOT NULL DEFAULT 0,
      description TEXT
    );
    CREATE TABLE IF NOT EXISTS settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );
  `);

  const listWorkflows = db.prepare('SELECT * FROM workflows');
  const getWorkflow = db.prepare('SELECT * FROM workflows WHERE id = ?');
  const insWorkflow = db.prepare(
    `INSERT INTO workflows (id,name,description,prompt,status,version,steps,created_by,approved_by,ai_model,created_at,runs)
     VALUES (@id,@name,@description,@prompt,@status,@version,@steps,@created_by,@approved_by,@ai_model,@created_at,@runs)
     ON CONFLICT(id) DO UPDATE SET
       name=@name, description=@description, prompt=@prompt, status=@status, version=@version, steps=@steps,
       created_by=@created_by, approved_by=@approved_by, ai_model=@ai_model, created_at=@created_at, runs=@runs`,
  );
  const listInstances = db.prepare('SELECT * FROM instances ORDER BY rowid DESC');
  const getInstance = db.prepare('SELECT * FROM instances WHERE id = ?');
  const insInstance = db.prepare(
    `INSERT INTO instances (id,workflow_id,workflow_name,status,entity,started_at,step_runs,current_step,waiting_on,error,ended_at,input,auto_approve)
     VALUES (@id,@workflow_id,@workflow_name,@status,@entity,@started_at,@step_runs,@current_step,@waiting_on,@error,@ended_at,@input,@auto_approve)
     ON CONFLICT(id) DO UPDATE SET
       workflow_id=@workflow_id, workflow_name=@workflow_name, status=@status, entity=@entity, started_at=@started_at,
       step_runs=@step_runs, current_step=@current_step, waiting_on=@waiting_on, error=@error, ended_at=@ended_at, input=@input, auto_approve=@auto_approve`,
  );
  const listAudit = db.prepare('SELECT * FROM audit ORDER BY rowid DESC');
  const insAudit = db.prepare(
    'INSERT INTO audit (id,at,actor,action,detail,kind) VALUES (@id,@at,@actor,@action,@detail,@kind)',
  );
  const listMDM = db.prepare('SELECT * FROM mdm');
  const getMDM = db.prepare('SELECT * FROM mdm WHERE key = ?');
  const insMDM = db.prepare(
    `INSERT INTO mdm (key,label,icon,fields,records) VALUES (@key,@label,@icon,@fields,@records)
     ON CONFLICT(key) DO UPDATE SET label=@label, icon=@icon, fields=@fields, records=@records`,
  );
  const listControls = db.prepare('SELECT * FROM controls ORDER BY custom, key');
  const insControl = db.prepare(
    `INSERT INTO controls (key,label,color,icon,enabled,custom,description) VALUES (@key,@label,@color,@icon,@enabled,@custom,@description)
     ON CONFLICT(key) DO UPDATE SET label=@label, color=@color, icon=@icon, enabled=@enabled, custom=@custom, description=@description`,
  );
  const delControl = db.prepare('DELETE FROM controls WHERE key = ?');

  return {
    db,
    listWorkflows: () => listWorkflows.all().map(wfFromRow),
    getWorkflow: (id) => { const r = getWorkflow.get(id); return r ? wfFromRow(r) : undefined; },
    upsertWorkflow: (wf) => insWorkflow.run({
      id: wf.id, name: wf.name, description: wf.description, prompt: wf.prompt, status: wf.status,
      version: wf.version, steps: JSON.stringify(wf.steps), created_by: wf.createdBy,
      approved_by: wf.approvedBy ?? null, ai_model: wf.aiModel, created_at: wf.createdAt, runs: wf.runs,
    }),
    patchWorkflow: (id, patch) => {
      const cur = getWorkflow.get(id);
      if (!cur) return;
      const merged = { ...wfFromRow(cur), ...patch };
      insWorkflow.run({
        id: merged.id, name: merged.name, description: merged.description, prompt: merged.prompt,
        status: merged.status, version: merged.version, steps: JSON.stringify(merged.steps),
        created_by: merged.createdBy, approved_by: merged.approvedBy ?? null, ai_model: merged.aiModel,
        created_at: merged.createdAt, runs: merged.runs,
      });
    },
    listInstances: () => listInstances.all().map(instFromRow),
    getInstance: (id) => { const r = getInstance.get(id); return r ? instFromRow(r) : undefined; },
    upsertInstance: (inst) => insInstance.run({
      id: inst.id, workflow_id: inst.workflowId, workflow_name: inst.workflowName, status: inst.status,
      entity: inst.entity ?? null, started_at: inst.startedAt, step_runs: JSON.stringify(inst.stepRuns),
      current_step: inst.currentStep, waiting_on: inst.waitingOn ?? null, error: inst.error ?? null,
      ended_at: inst.endedAt ?? null, input: inst.input != null ? JSON.stringify(inst.input) : null,
      auto_approve: inst.autoApprove ? 1 : 0,
    }),
    listAudit: () => listAudit.all().map(auditFromRow),
    addAudit: (a) => insAudit.run({ id: a.id, at: a.at, actor: a.actor, action: a.action, detail: a.detail, kind: a.kind }),
    listMDM: () => listMDM.all().map(mdmFromRow),
    getMDM: (key) => { const r = getMDM.get(key); return r ? mdmFromRow(r) : undefined; },
    upsertMDM: (e) => insMDM.run({
      key: e.key, label: e.label, icon: e.icon, fields: JSON.stringify(e.fields), records: JSON.stringify(e.records),
    }),
    listControls: () => listControls.all().map(ctrlFromRow),
    upsertControl: (c) => insControl.run({
      key: c.key, label: c.label, color: c.color, icon: c.icon,
      enabled: c.enabled ? 1 : 0, custom: c.custom ? 1 : 0, description: c.description ?? null,
    }),
    deleteControl: (key) => delControl.run(key),
    close: () => db.close(),
  };
}

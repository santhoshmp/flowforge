// Package store is the SQLite-backed persistence layer for the Go control
// plane. It mirrors server/src/db.ts: the same schema and JSON columns for
// nested fields (steps, stepRuns, records, input). Single-connection to keep
// :memory: tests consistent and writes simple/synchronous.
package store

import (
	"database/sql"
	"encoding/json"

	_ "modernc.org/sqlite"

	"github.com/flowforge/flowforge/internal/models"
)

type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database and ensures the schema exists.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Single connection: simple/synchronous writes and shared :memory: in tests.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

const schema = `
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
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  created_at TEXT NOT NULL
);
`

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// ---- Workflows ---------------------------------------------------------------

const workflowCols = "id,name,description,prompt,status,version,steps,created_by,approved_by,ai_model,created_at,runs"

func scanWorkflow(row interface{ Scan(...any) error }) (*models.Workflow, error) {
	var w models.Workflow
	var steps, approvedBy sql.NullString
	if err := row.Scan(&w.ID, &w.Name, &w.Description, &w.Prompt, &w.Status, &w.Version,
		&steps, &w.CreatedBy, &approvedBy, &w.AIModel, &w.CreatedAt, &w.Runs); err != nil {
		return nil, err
	}
	w.ApprovedBy = approvedBy.String
	if err := json.Unmarshal([]byte(steps.String), &w.Steps); err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *Store) ListWorkflows() ([]models.Workflow, error) {
	rows, err := s.db.Query("SELECT " + workflowCols + " FROM workflows")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, nil
}

func (s *Store) GetWorkflow(id string) (*models.Workflow, error) {
	row := s.db.QueryRow("SELECT "+workflowCols+" FROM workflows WHERE id = ?", id)
	return scanWorkflow(row)
}

func (s *Store) UpsertWorkflow(w models.Workflow) error {
	approved := &sql.NullString{}
	if w.ApprovedBy != "" {
		approved = &sql.NullString{String: w.ApprovedBy, Valid: true}
	}
	_, err := s.db.Exec(`INSERT INTO workflows (id,name,description,prompt,status,version,steps,created_by,approved_by,ai_model,created_at,runs)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,prompt=excluded.prompt,
		status=excluded.status,version=excluded.version,steps=excluded.steps,created_by=excluded.created_by,
		approved_by=excluded.approved_by,ai_model=excluded.ai_model,created_at=excluded.created_at,runs=excluded.runs`,
		w.ID, w.Name, w.Description, w.Prompt, w.Status, w.Version, mustJSON(w.Steps),
		w.CreatedBy, approved, w.AIModel, w.CreatedAt, w.Runs)
	return err
}

// ---- Instances ---------------------------------------------------------------

const instanceCols = "id,workflow_id,workflow_name,status,entity,started_at,step_runs,current_step,waiting_on,error,ended_at,input,auto_approve"

func scanInstance(row interface{ Scan(...any) error }) (*models.Instance, error) {
	var inst models.Instance
	var entity, stepRuns, waitingOn, errMsg, endedAt, input sql.NullString
	var autoApprove int
	if err := row.Scan(&inst.ID, &inst.WorkflowID, &inst.WorkflowName, &inst.Status, &entity,
		&inst.StartedAt, &stepRuns, &inst.CurrentStep, &waitingOn, &errMsg, &endedAt, &input, &autoApprove); err != nil {
		return nil, err
	}
	inst.Entity = entity.String
	inst.WaitingOn = waitingOn.String
	inst.Error = errMsg.String
	inst.EndedAt = endedAt.String
	inst.AutoApprove = autoApprove != 0
	if input.String != "" {
		_ = json.Unmarshal([]byte(input.String), &inst.Input)
	}
	if err := json.Unmarshal([]byte(stepRuns.String), &inst.StepRuns); err != nil {
		return nil, err
	}
	return &inst, nil
}

func (s *Store) ListInstances() ([]models.Instance, error) {
	rows, err := s.db.Query("SELECT " + instanceCols + " FROM instances ORDER BY rowid DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Instance
	for rows.Next() {
		i, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, nil
}

func (s *Store) GetInstance(id string) (*models.Instance, error) {
	row := s.db.QueryRow("SELECT "+instanceCols+" FROM instances WHERE id = ?", id)
	return scanInstance(row)
}

func (s *Store) UpsertInstance(i models.Instance) error {
	var input any
	if i.Input != nil {
		input = mustJSON(i.Input)
	}
	_, err := s.db.Exec(`INSERT INTO instances (id,workflow_id,workflow_name,status,entity,started_at,step_runs,current_step,waiting_on,error,ended_at,input,auto_approve)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET workflow_id=excluded.workflow_id,workflow_name=excluded.workflow_name,
		status=excluded.status,entity=excluded.entity,started_at=excluded.started_at,step_runs=excluded.step_runs,
		current_step=excluded.current_step,waiting_on=excluded.waiting_on,error=excluded.error,ended_at=excluded.ended_at,
		input=excluded.input,auto_approve=excluded.auto_approve`,
		i.ID, i.WorkflowID, i.WorkflowName, i.Status, nullable(i.Entity), i.StartedAt, mustJSON(i.StepRuns),
		i.CurrentStep, nullable(i.WaitingOn), nullable(i.Error), nullable(i.EndedAt), input, boolToInt(i.AutoApprove))
	return err
}

// ---- Audit -------------------------------------------------------------------

func (s *Store) ListAudit() ([]models.AuditEntry, error) {
	rows, err := s.db.Query("SELECT id,at,actor,action,detail,kind FROM audit ORDER BY rowid DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AuditEntry
	for rows.Next() {
		var a models.AuditEntry
		if err := rows.Scan(&a.ID, &a.At, &a.Actor, &a.Action, &a.Detail, &a.Kind); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *Store) AddAudit(a models.AuditEntry) error {
	_, err := s.db.Exec("INSERT INTO audit (id,at,actor,action,detail,kind) VALUES (?,?,?,?,?,?)",
		a.ID, a.At, a.Actor, a.Action, a.Detail, a.Kind)
	return err
}

// ---- MDM ---------------------------------------------------------------------

func (s *Store) ListMDM() ([]models.MDMEntity, error) {
	rows, err := s.db.Query("SELECT key,label,icon,fields,records FROM mdm")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.MDMEntity
	for rows.Next() {
		var e models.MDMEntity
		var fields, records string
		if err := rows.Scan(&e.Key, &e.Label, &e.Icon, &fields, &records); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(fields), &e.Fields)
		_ = json.Unmarshal([]byte(records), &e.Records)
		out = append(out, e)
	}
	return out, nil
}

func (s *Store) UpsertMDM(e models.MDMEntity) error {
	_, err := s.db.Exec(`INSERT INTO mdm (key,label,icon,fields,records) VALUES (?,?,?,?,?)
		ON CONFLICT(key) DO UPDATE SET label=excluded.label,icon=excluded.icon,fields=excluded.fields,records=excluded.records`,
		e.Key, e.Label, e.Icon, mustJSON(e.Fields), mustJSON(e.Records))
	return err
}

// ---- Controls ----------------------------------------------------------------

func (s *Store) ListControls() ([]models.ControlDef, error) {
	rows, err := s.db.Query("SELECT key,label,color,icon,enabled,custom,description FROM controls ORDER BY custom, key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ControlDef
	for rows.Next() {
		var c models.ControlDef
		var enabled, custom int
		var desc sql.NullString
		if err := rows.Scan(&c.Key, &c.Label, &c.Color, &c.Icon, &enabled, &custom, &desc); err != nil {
			return nil, err
		}
		c.Enabled = enabled != 0
		c.Custom = custom != 0
		c.Description = desc.String
		out = append(out, c)
	}
	return out, nil
}

func (s *Store) UpsertControl(c models.ControlDef) error {
	var desc any
	if c.Description != "" {
		desc = c.Description
	}
	_, err := s.db.Exec(`INSERT INTO controls (key,label,color,icon,enabled,custom,description) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(key) DO UPDATE SET label=excluded.label,color=excluded.color,icon=excluded.icon,
		enabled=excluded.enabled,custom=excluded.custom,description=excluded.description`,
		c.Key, c.Label, c.Color, c.Icon, boolToInt(c.Enabled), boolToInt(c.Custom), desc)
	return err
}

func (s *Store) DeleteControl(key string) error {
	_, err := s.db.Exec("DELETE FROM controls WHERE key = ?", key)
	return err
}

// ---- Settings (AI config) ----------------------------------------------------

func (s *Store) GetSetting(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec("INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return err
}

// ---- helpers -----------------------------------------------------------------

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

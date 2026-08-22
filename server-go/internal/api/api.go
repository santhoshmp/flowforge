// Package api is the HTTP control plane for the Go server. It mirrors the
// Node server's /api/v1/* surface so the same React UI can talk to either.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/flowforge/flowforge/internal/ai"
	"github.com/flowforge/flowforge/internal/auth"
	"github.com/flowforge/flowforge/internal/engine"
	"github.com/flowforge/flowforge/internal/metrics"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/settings"
	"github.com/flowforge/flowforge/internal/store"
	"github.com/flowforge/flowforge/internal/util"
)

// Server is the HTTP control plane bound to a store.
type Server struct {
	store    *store.Store
	mux      *http.ServeMux
	authMode string
	api      http.Handler // auth-wrapped mux
	ui       http.Handler // embedded SPA (public)
}

// New builds a Server with all /api/v1 routes registered. authMode is "auto"
// (setup-mode gating then token-required) or "off" (dev / Node parity).
func New(s *store.Store, authMode string) *Server {
	if authMode == "" {
		authMode = "auto"
	}
	srv := &Server{store: s, mux: http.NewServeMux(), authMode: authMode}
	srv.routes()
	srv.registerExtRoutes()
	srv.api = auth.Wrap(s, authMode, srv.mux)
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rw := &statusRecorder{ResponseWriter: w, status: 200}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.api.ServeHTTP(rw, r)
		fmt.Printf("[api] %s %s -> %d (%dms)\n", r.Method, r.URL.Path, rw.status, time.Since(start).Milliseconds())
		return
	}
	if s.ui != nil {
		s.ui.ServeHTTP(rw, r)
		return
	}
	http.NotFound(w, r)
}

// statusRecorder captures the response status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// EnableUI mounts an embedded SPA (served at /, with index.html fallback).
func (s *Server) EnableUI(fsys fs.FS) {
	s.ui = spaHandler(fsys)
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	body, _ := io.ReadAll(r.Body)
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

func (s *Server) audit(actor, action, detail, kind string) {
	_ = s.store.AddAudit(models.AuditEntry{ID: util.UID(), At: nowISO(), Actor: actor, Action: action, Detail: detail, Kind: kind})
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/v1/health", s.health)
	m.HandleFunc("GET /api/v1/bootstrap", s.bootstrap)
	m.HandleFunc("GET /api/v1/metrics", s.metrics)

	m.HandleFunc("POST /api/v1/ai/draft", s.aiDraft)

	m.HandleFunc("GET /api/v1/workflows", s.listWorkflows)
	m.HandleFunc("POST /api/v1/workflows", s.createWorkflow)
	m.HandleFunc("GET /api/v1/workflows/{id}", s.getWorkflow)
	m.HandleFunc("PATCH /api/v1/workflows/{id}", s.patchWorkflow)
	m.HandleFunc("POST /api/v1/workflows/{id}/approve", s.approveWorkflow)
	m.HandleFunc("GET /api/v1/workflows/{id}/executions", s.workflowExecutions)
	m.HandleFunc("POST /api/v1/workflows/{id}/executions", s.runWorkflow)

	m.HandleFunc("GET /api/v1/executions", s.listExecutions)
	m.HandleFunc("GET /api/v1/executions/{id}", s.getInstance)
	m.HandleFunc("GET /api/v1/executions/{id}/steps", s.instanceSteps)
	m.HandleFunc("POST /api/v1/executions/{id}/approve", s.approveTask)
	m.HandleFunc("POST /api/v1/executions/{id}/retry", s.retryInstance)
	m.HandleFunc("POST /api/v1/executions/{id}/cancel", s.cancelInstance)

	m.HandleFunc("GET /api/v1/mdm", s.listMDM)
	m.HandleFunc("GET /api/v1/mdm/{entity}", s.getMDM)
	m.HandleFunc("POST /api/v1/mdm/{entity}", s.addMDMRecord)

	m.HandleFunc("GET /api/v1/controls", s.listControls)
	m.HandleFunc("POST /api/v1/controls", s.createControl)
	m.HandleFunc("PATCH /api/v1/controls/{key}", s.patchControl)
	m.HandleFunc("POST /api/v1/controls/{key}/toggle", s.toggleControl)
	m.HandleFunc("DELETE /api/v1/controls/{key}", s.deleteControl)

	m.HandleFunc("GET /api/v1/audit", s.listAudit)
	m.HandleFunc("POST /api/v1/audit", s.addAudit)

	m.HandleFunc("GET /api/v1/settings/ai", s.getAISettings)
	m.HandleFunc("PUT /api/v1/settings/ai", s.putAISettings)
	m.HandleFunc("POST /api/v1/settings/ai/test", s.testAISettings)

	m.HandleFunc("GET /api/v1/auth/status", s.authStatus)
	m.HandleFunc("POST /api/v1/auth/setup", s.authSetup)
	m.HandleFunc("POST /api/v1/auth/login", s.authLogin)
	m.HandleFunc("GET /api/v1/auth/me", s.authMe)
	m.HandleFunc("POST /api/v1/auth/logout", s.authLogout)
}

// ---- health / bootstrap / metrics ------------------------------------------

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok", "model": settings.AuthoringModel(settings.GetConfig(s.store))})
}

func (s *Server) bootstrap(w http.ResponseWriter, _ *http.Request) {
	wfs, _ := s.store.ListWorkflows()
	ins, _ := s.store.ListInstances()
	au, _ := s.store.ListAudit()
	mdm, _ := s.store.ListMDM()
	ctrl, _ := s.store.ListControls()
	writeJSON(w, 200, map[string]any{"workflows": wfs, "instances": ins, "audit": au, "mdm": mdm, "controls": ctrl})
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	m, err := metrics.Compute(s.store)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, m)
}

// ---- AI ---------------------------------------------------------------------

func (s *Server) aiDraft(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeErr(w, 400, "prompt is required")
		return
	}
	cfg := settings.GetConfig(s.store)
	res := ai.GenerateDraft(strings.TrimSpace(req.Prompt), cfg)
	s.audit("AI author", "Draft generated", res.Draft.Name+" · "+itoa(res.Draft.OverallConfidence)+"% avg confidence · model: "+res.Model, "ai")
	writeJSON(w, 200, res)
}

// ---- Workflows --------------------------------------------------------------

func (s *Server) listWorkflows(w http.ResponseWriter, _ *http.Request) {
	wfs, _ := s.store.ListWorkflows()
	writeJSON(w, 200, wfs)
}

func (s *Server) createWorkflow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name, Description, Prompt string
		Steps                     []models.WorkflowStep
		AIModel                   string
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	aiModel := req.AIModel
	if aiModel == "" {
		aiModel = settings.AuthoringModel(settings.GetConfig(s.store))
	}
	wf := models.Workflow{
		ID: "wf-" + util.UID(), Name: req.Name, Description: req.Description, Prompt: req.Prompt,
		Status: models.StatusDraft, Version: 1, Steps: req.Steps, CreatedBy: "You",
		AIModel: aiModel, CreatedAt: nowISO(),
	}
	if err := s.store.UpsertWorkflow(wf); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.audit("You", "Workflow created", wf.Name+" — draft saved ("+itoa(len(wf.Steps))+" steps)", "deploy")
	writeJSON(w, 200, wf)
}

func (s *Server) getWorkflow(w http.ResponseWriter, r *http.Request) {
	wf, err := s.store.GetWorkflow(r.PathValue("id"))
	if err != nil || wf == nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	writeJSON(w, 200, wf)
}

func (s *Server) patchWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wf, err := s.store.GetWorkflow(id)
	if err != nil || wf == nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	var req struct {
		Name, Description, Status *string
		Version                   *int
		Steps                     []models.WorkflowStep
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if req.Name != nil {
		wf.Name = *req.Name
	}
	if req.Description != nil {
		wf.Description = *req.Description
	}
	if req.Status != nil {
		wf.Status = *req.Status
	}
	if req.Version != nil {
		wf.Version = *req.Version
	}
	if len(req.Steps) > 0 {
		wf.Steps = req.Steps
	}
	_ = s.store.UpsertWorkflow(*wf)
	writeJSON(w, 200, wf)
}

func (s *Server) approveWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wf, err := s.store.GetWorkflow(id)
	if err != nil || wf == nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	// Approval gate (D1): connector steps must resolve against the registry.
	if err := s.validateConnectors(wf); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	wf.Status = models.StatusDeployed
	wf.ApprovedBy = "You (reviewer)"
	_ = s.store.UpsertWorkflow(*wf)
	s.audit("You (reviewer)", "Approved & deployed", wf.Name+" v"+itoa(wf.Version)+" — "+itoa(len(wf.Steps))+" steps", "approval")
	writeJSON(w, 200, wf)
}

func (s *Server) workflowExecutions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if wf, err := s.store.GetWorkflow(id); err != nil || wf == nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	ins, _ := s.store.ListInstances()
	out := []models.Instance{}
	for _, i := range ins {
		if i.WorkflowID == id {
			out = append(out, i)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) runWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wf, err := s.store.GetWorkflow(id)
	if err != nil || wf == nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	if wf.Status == models.StatusDraft {
		writeErr(w, 400, "workflow must be approved before execution")
		return
	}
	var req struct {
		Entity string
		Input  map[string]any
	}
	_ = decode(r, &req)
	entity := req.Entity
	if entity == "" {
		entity = "REC-" + itoa(1000+randomInt(9000)) + " · demo"
	}
	runs := make([]models.StepRun, len(wf.Steps))
	for i, st := range wf.Steps {
		runs[i] = models.StepRun{StepID: st.ID, Name: st.Name, Type: st.Type, Status: models.StepPending}
	}
	inst := models.Instance{
		ID: "run-" + util.UID(), WorkflowID: id, WorkflowName: wf.Name, Status: models.InstRunning,
		Entity: entity, StartedAt: nowISO(), Input: req.Input, CurrentStep: 0, StepRuns: runs,
	}
	_ = s.store.UpsertInstance(inst)
	wf.Runs++
	_ = s.store.UpsertWorkflow(*wf)
	s.audit("You", "Execution started", inst.ID+" · "+wf.Name, "execution")
	writeJSON(w, 200, inst)
}

// ---- Executions -------------------------------------------------------------

func (s *Server) listExecutions(w http.ResponseWriter, _ *http.Request) {
	ins, _ := s.store.ListInstances()
	writeJSON(w, 200, ins)
}

func (s *Server) getInstance(w http.ResponseWriter, r *http.Request) {
	inst, err := s.store.GetInstance(r.PathValue("id"))
	if err != nil || inst == nil {
		writeErr(w, 404, "execution not found")
		return
	}
	writeJSON(w, 200, inst)
}

func (s *Server) instanceSteps(w http.ResponseWriter, r *http.Request) {
	inst, err := s.store.GetInstance(r.PathValue("id"))
	if err != nil || inst == nil {
		writeErr(w, 404, "execution not found")
		return
	}
	writeJSON(w, 200, inst.StepRuns)
}

func (s *Server) approveTask(w http.ResponseWriter, r *http.Request) {
	inst, err := engine.ApproveWaiting(s.store, r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if inst == nil {
		writeErr(w, 404, "execution not found")
		return
	}
	writeJSON(w, 200, inst)
}

func (s *Server) retryInstance(w http.ResponseWriter, r *http.Request) {
	inst, err := engine.RetryFailed(s.store, r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if inst == nil {
		writeErr(w, 404, "execution not found")
		return
	}
	writeJSON(w, 200, inst)
}

func (s *Server) cancelInstance(w http.ResponseWriter, r *http.Request) {
	inst, err := engine.CancelInstance(s.store, r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if inst == nil {
		writeErr(w, 404, "execution not found")
		return
	}
	writeJSON(w, 200, inst)
}

// ---- MDM --------------------------------------------------------------------

func (s *Server) listMDM(w http.ResponseWriter, _ *http.Request) {
	mdm, _ := s.store.ListMDM()
	writeJSON(w, 200, mdm)
}

func (s *Server) findMDM(key string) *models.MDMEntity {
	mdm, _ := s.store.ListMDM()
	for _, e := range mdm {
		if e.Key == key {
			return &e
		}
	}
	return nil
}

func (s *Server) getMDM(w http.ResponseWriter, r *http.Request) {
	e := s.findMDM(r.PathValue("entity"))
	if e == nil {
		writeErr(w, 404, "entity not found")
		return
	}
	writeJSON(w, 200, e)
}

func (s *Server) addMDMRecord(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("entity")
	e := s.findMDM(key)
	if e == nil {
		writeErr(w, 404, "entity not found")
		return
	}
	var req struct {
		Record map[string]string
	}
	_ = decode(r, &req)
	rec := map[string]string{}
	for k, v := range req.Record {
		rec[k] = v
	}
	id := rec["id"]
	if id == "" && len(e.Fields) > 0 {
		id = rec[e.Fields[0]]
	}
	if id == "" {
		id = "X-" + itoa(1000+randomInt(9000))
	}
	rec["id"] = id
	rec["status"] = "pending stewardship"
	e.Records = append([]map[string]string{rec}, e.Records...)
	_ = s.store.UpsertMDM(*e)
	s.audit("You", "MDM record created", key+"/"+id+" — pending stewardship", "mdm")
	writeJSON(w, 200, e)
}

// ---- Controls ---------------------------------------------------------------

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_.]*$`)

func (s *Server) listControls(w http.ResponseWriter, _ *http.Request) {
	c, _ := s.store.ListControls()
	writeJSON(w, 200, c)
}

func (s *Server) createControl(w http.ResponseWriter, r *http.Request) {
	var c models.ControlDef
	if err := decode(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !keyRe.MatchString(c.Key) {
		writeErr(w, 400, "key must be lowercase letters, numbers, dots or underscores")
		return
	}
	if existing, _ := s.store.ListControls(); existing != nil {
		for _, x := range existing {
			if x.Key == c.Key {
				writeErr(w, 400, "control \""+c.Key+"\" already exists")
				return
			}
		}
	}
	c.Enabled = true
	c.Custom = true
	if c.Color == "" {
		c.Color = "violet"
	}
	if c.Icon == "" {
		c.Icon = "Code"
	}
	if c.Label == "" {
		c.Label = c.Key
	}
	_ = s.store.UpsertControl(c)
	s.audit("You", "Control created", c.Label+" ("+c.Key+") — available in the palette", "deploy")
	writeJSON(w, 200, c)
}

func (s *Server) patchControl(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	cur := s.findControl(key)
	if cur == nil {
		writeErr(w, 404, "control not found")
		return
	}
	var req struct {
		Label, Color, Icon, Description *string
		Enabled                         *bool
	}
	_ = decode(r, &req)
	if req.Label != nil {
		cur.Label = *req.Label
	}
	if req.Color != nil {
		cur.Color = *req.Color
	}
	if req.Icon != nil {
		cur.Icon = *req.Icon
	}
	if req.Description != nil {
		cur.Description = *req.Description
	}
	if req.Enabled != nil {
		cur.Enabled = *req.Enabled
	}
	_ = s.store.UpsertControl(*cur)
	writeJSON(w, 200, cur)
}

func (s *Server) toggleControl(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	cur := s.findControl(key)
	if cur == nil {
		writeErr(w, 404, "control not found")
		return
	}
	cur.Enabled = !cur.Enabled
	_ = s.store.UpsertControl(*cur)
	s.audit("You", ifElse(cur.Enabled, "Control enabled", "Control disabled"), cur.Label, "deploy")
	writeJSON(w, 200, cur)
}

func (s *Server) deleteControl(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	cur := s.findControl(key)
	if cur == nil {
		writeErr(w, 404, "control not found")
		return
	}
	if !cur.Custom {
		writeErr(w, 400, "built-in controls cannot be removed")
		return
	}
	used := 0
	wfs, _ := s.store.ListWorkflows()
	for _, w := range wfs {
		for _, st := range w.Steps {
			if st.Type == key {
				used++
			}
		}
	}
	if used > 0 {
		writeErr(w, 400, "control is used by "+itoa(used)+" step(s)")
		return
	}
	_ = s.store.DeleteControl(key)
	s.audit("You", "Control removed", key, "deploy")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) findControl(key string) *models.ControlDef {
	cs, _ := s.store.ListControls()
	for _, c := range cs {
		if c.Key == key {
			return &c
		}
	}
	return nil
}

// ---- Audit ------------------------------------------------------------------

func (s *Server) listAudit(w http.ResponseWriter, _ *http.Request) {
	au, _ := s.store.ListAudit()
	writeJSON(w, 200, au)
}

func (s *Server) addAudit(w http.ResponseWriter, r *http.Request) {
	var a models.AuditEntry
	_ = decode(r, &a)
	if a.Actor == "" {
		a.Actor = "You"
	}
	if a.Action == "" {
		a.Action = "Event"
	}
	if a.Kind == "" {
		a.Kind = "deploy"
	}
	a.ID = util.UID()
	a.At = nowISO()
	_ = s.store.AddAudit(a)
	writeJSON(w, 200, a)
}

// ---- AI settings ------------------------------------------------------------

func (s *Server) getAISettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, settings.Public(s.store))
}

func (s *Server) putAISettings(w http.ResponseWriter, r *http.Request) {
	var cfg models.AIConfig
	if err := decode(r, &cfg); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := settings.SetConfig(s.store, cfg); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	pub := settings.Public(s.store)
	s.audit("You", "AI model updated", pub.ActiveLabel, "deploy")
	writeJSON(w, 200, pub)
}

type testResult struct {
	OK             bool     `json:"ok"`
	LatencyMs      int      `json:"latencyMs,omitempty"`
	Model          string   `json:"model,omitempty"`
	Models         []string `json:"models,omitempty"`
	ModelAvailable *bool    `json:"modelAvailable,omitempty"`
	Note           string   `json:"note,omitempty"`
	Error          string   `json:"error,omitempty"`
}

func (s *Server) testAISettings(w http.ResponseWriter, r *http.Request) {
	var req models.AIConfig
	_ = decode(r, &req)
	cur := settings.GetConfig(s.store)
	cfg := models.AIConfig{
		Provider: orDefault(req.Provider, cur.Provider),
		BaseURL:  orDefault(req.BaseURL, cur.BaseURL),
		Model:    orDefault(req.Model, cur.Model),
		APIKey:   orDefault(req.APIKey, cur.APIKey),
	}
	writeJSON(w, 200, testConnection(cfg))
}

func testConnection(cfg models.AIConfig) testResult {
	client := &http.Client{Timeout: 20 * time.Second}
	key := cfg.APIKey
	if key == "" {
		key = "local-no-key"
	}
	start := time.Now()
	httpReq, _ := http.NewRequest("GET", cfg.BaseURL+"/models", nil)
	httpReq.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(httpReq)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode < 400 {
			var mr struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			body, _ := io.ReadAll(resp.Body)
			ids := []string{}
			if json.Unmarshal(body, &mr) == nil {
				for _, m := range mr.Data {
					ids = append(ids, m.ID)
				}
			}
			avail := true
			if len(ids) > 0 {
				avail = contains(ids, cfg.Model)
			}
			return testResult{OK: true, LatencyMs: int(time.Since(start).Milliseconds()), Model: cfg.Model, Models: ids, ModelAvailable: &avail}
		}
		return testResult{OK: false, LatencyMs: int(time.Since(start).Milliseconds()), Error: "HTTP " + itoa(resp.StatusCode)}
	}
	// fallback: minimal chat ping
	chatReq, _ := http.NewRequest("POST", cfg.BaseURL+"/chat/completions",
		strings.NewReader(`{"model":"`+cfg.Model+`","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+key)
	if cr, err := client.Do(chatReq); err == nil {
		defer cr.Body.Close()
		if cr.StatusCode < 400 {
			return testResult{OK: true, LatencyMs: int(time.Since(start).Milliseconds()), Model: cfg.Model, Note: "Chat endpoint reachable (model list not supported)."}
		}
		b, _ := io.ReadAll(cr.Body)
		return testResult{OK: false, LatencyMs: int(time.Since(start).Milliseconds()), Error: "HTTP " + itoa(cr.StatusCode) + ": " + string(b)}
	}
	return testResult{OK: false, LatencyMs: int(time.Since(start).Milliseconds()), Error: err.Error()}
}

// ---- Auth -------------------------------------------------------------------

func (s *Server) authStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, auth.StatusOf(s.store))
}

func (s *Server) authSetup(w http.ResponseWriter, r *http.Request) {
	if n, _ := s.store.CountUsers(); n > 0 {
		writeErr(w, 409, "already set up")
		return
	}
	var req struct {
		Username, Password string
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !auth.ValidCredentials(req.Username, req.Password) {
		writeErr(w, 400, "username must be 3-32 chars (a-z0-9_.-) and password >= 6 chars")
		return
	}
	u, err := auth.CreateUser(s.store, req.Username, req.Password, "admin")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	token, _ := auth.MakeToken(s.store, u)
	s.audit(req.Username, "Admin created (first-run setup)", u.Username+" ("+u.ID+")", "deploy")
	writeJSON(w, 200, map[string]any{"token": token, "user": userResp(u)})
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username, Password string
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u, _ := s.store.GetUserByUsername(req.Username)
	if u == nil || !auth.VerifyPassword(u.PasswordHash, req.Password) {
		writeErr(w, 401, "invalid credentials")
		return
	}
	token, _ := auth.MakeToken(s.store, *u)
	s.audit(u.Username, "Signed in", u.Username, "deploy")
	writeJSON(w, 200, map[string]any{"token": token, "user": userResp(*u)})
}

func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	c := auth.ClaimsFrom(r.Context())
	if c == nil {
		writeErr(w, 401, "unauthorized")
		return
	}
	writeJSON(w, 200, map[string]any{"id": c.Sub, "username": c.U, "role": c.R})
}

func (s *Server) authLogout(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func userResp(u store.UserRow) map[string]any {
	return map[string]any{"id": u.ID, "username": u.Username, "role": u.Role}
}

// ---- SPA --------------------------------------------------------------------

func spaHandler(fsys fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(fsys))
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		cleaned := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		// SPA fallback: serve index.html for unknown non-asset paths.
		if _, err := fs.Stat(fsys, cleaned); err != nil {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		// Never cache index.html so new builds are always served.
		if cleaned == "index.html" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		fileServer.ServeHTTP(w, r)
	}
}

// ---- tiny helpers -----------------------------------------------------------

func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = digits[n%10]
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func randomInt(maxExclusive int) int {
	return int(time.Now().UnixNano()) % maxExclusive
}

func ifElse(b bool, a, c string) string {
	if b {
		return a
	}
	return c
}

func orDefault(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

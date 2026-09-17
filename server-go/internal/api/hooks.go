// Artifact import + inbound webhooks: the two directions that connect the
// portable flowforge/v1 artifact world with a running control plane.
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/flowforge/flowforge/internal/auth"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/spec"
	"github.com/flowforge/flowforge/internal/util"
)

// ---- Artifact import ---------------------------------------------------------

// importArtifact: POST /api/v1/workflows/from-artifact {"yaml": "..."} —
// validates a flowforge/v1 artifact and creates a draft from it (the
// round-trip of the export button; the draft still needs human approval).
func (s *Server) importArtifact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		YAML string `json:"yaml"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.YAML) == "" {
		writeErr(w, 400, `body must be {"yaml": "<flowforge/v1 artifact>"}`)
		return
	}
	sp, err := spec.ParseYAML(req.YAML)
	if err != nil {
		writeErr(w, 400, "invalid artifact: "+err.Error())
		return
	}
	wf := spec.ToWorkflow(sp)
	wf.ID = "wf-" + util.UID()
	wf.Status = models.StatusDraft
	wf.CreatedBy = "You"
	wf.CreatedAt = nowISO()
	if wf.AIModel == "" {
		wf.AIModel = "import"
	}
	if err := s.store.UpsertWorkflow(wf); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.audit("You", "Workflow imported from artifact", wf.Name+" — review & approve to deploy (v"+itoa(wf.Version)+")", "ai")
	writeJSON(w, 200, wf)
}

// ---- Inbound webhooks --------------------------------------------------------

// workflowHookInfo: GET /api/v1/workflows/{id}/hook — the hook URL + token
// for a deployed workflow (share the URL, keep the token with the caller).
func (s *Server) workflowHookInfo(w http.ResponseWriter, r *http.Request) {
	wf, err := s.store.GetWorkflow(r.PathValue("id"))
	if err != nil || wf == nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	if wf.Status != models.StatusDeployed {
		writeErr(w, 400, "webhooks require a deployed workflow (approve first)")
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	writeJSON(w, 200, map[string]string{
		"workflowId": wf.ID,
		"method":     "POST",
		"url":        scheme + "://" + r.Host + "/api/v1/hooks/" + wf.ID,
		"token":      auth.HookToken(s.store, wf.ID),
		"usage":      `curl -X POST <url> -H "X-FlowForge-Token: <token>" -H "Content-Type: application/json" -d '{"input": {...}}'`,
	})
}

// triggerHook: POST /api/v1/hooks/{id} — public path (external systems);
// authenticates via the per-workflow HMAC token (query ?token= or
// X-FlowForge-Token). The JSON body becomes the run input.
func (s *Server) triggerHook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// In auth mode "auto", hooks bypass the session gate via their own
	// token — but not before first-run setup completed. Mode "off" is
	// explicit dev-no-auth: hooks work.
	if s.authMode != "off" && auth.StatusOf(s.store).SetupRequired {
		writeErr(w, 403, "setup required")
		return
	}
	token := r.Header.Get("X-FlowForge-Token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if !auth.VerifyHookToken(s.store, id, token) {
		writeErr(w, 401, "invalid hook token")
		return
	}
	wf, err := s.store.GetWorkflow(id)
	if err != nil || wf == nil {
		writeErr(w, 404, "workflow not found")
		return
	}
	if wf.Status != models.StatusDeployed {
		writeErr(w, 409, "workflow is not deployed")
		return
	}

	input := map[string]any{}
	if body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)); err == nil && len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			writeErr(w, 400, "body must be a JSON object (it becomes the run input): "+err.Error())
			return
		}
	}
	entity := r.URL.Query().Get("entity")
	if entity == "" {
		if e, ok := input["entity"].(string); ok && e != "" {
			entity = e
			delete(input, "entity")
		}
	}
	if entity == "" {
		entity = "webhook · " + wf.Name
	}

	runs := make([]models.StepRun, len(wf.Steps))
	for i, st := range wf.Steps {
		runs[i] = models.StepRun{StepID: st.ID, Name: st.Name, Type: st.Type, Status: models.StepPending}
	}
	inst := models.Instance{
		ID: "run-" + util.UID(), WorkflowID: wf.ID, WorkflowName: wf.Name,
		Status: models.InstRunning, Entity: entity, StartedAt: nowISO(),
		Input: input, CurrentStep: 0, StepRuns: runs,
	}
	_ = s.store.UpsertInstance(inst)
	wf.Runs++
	_ = s.store.UpsertWorkflow(*wf)
	s.audit("webhook", "Execution started (webhook)", inst.ID+" · "+wf.Name, "execution")
	writeJSON(w, 200, map[string]any{"instanceId": inst.ID, "status": inst.Status, "entity": entity})
}

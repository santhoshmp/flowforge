// P4 extensibility endpoints: connectors (SDK registry), templates (gallery),
// secrets (encrypted vault — names only, values are never returned), and
// approve-time connector validation (docs/decisions.md D1).
package api

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/flowforge/flowforge/internal/connectors"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/secrets"
	"github.com/flowforge/flowforge/internal/templates"
	"github.com/flowforge/flowforge/internal/util"
	"github.com/flowforge/flowforge/internal/wasm"
)

func (s *Server) registerExtRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/v1/connectors", s.listConnectors)
	m.HandleFunc("GET /api/v1/connectors/{id}", s.getConnector)
	m.HandleFunc("POST /api/v1/connectors/{id}/test", s.testConnector)

	m.HandleFunc("GET /api/v1/templates", s.listTemplates)
	m.HandleFunc("GET /api/v1/templates/{id}", s.getTemplate)
	m.HandleFunc("POST /api/v1/templates/{id}/instantiate", s.instantiateTemplate)

	m.HandleFunc("GET /api/v1/secrets", s.listSecrets)
	m.HandleFunc("PUT /api/v1/secrets", s.putSecret)
	m.HandleFunc("DELETE /api/v1/secrets/{name}", s.deleteSecret)
}

// ---- Connectors -------------------------------------------------------------

func (s *Server) listConnectors(w http.ResponseWriter, _ *http.Request) {
	reg, err := connectors.Default()
	if err != nil {
		writeErr(w, 500, "connectors: "+err.Error())
		return
	}
	out := []map[string]any{}
	for _, e := range reg.List() {
		item := e.Manifest.Summary()
		item["builtin"] = e.Builtin
		if e.Dir != "" {
			item["dir"] = e.Dir
		}
		out = append(out, item)
	}
	writeJSON(w, 200, out)
}

func (s *Server) getConnector(w http.ResponseWriter, r *http.Request) {
	reg, err := connectors.Default()
	if err != nil {
		writeErr(w, 500, "connectors: "+err.Error())
		return
	}
	e := reg.Get(r.PathValue("id"))
	if e == nil {
		writeErr(w, 404, "connector not found")
		return
	}
	item := e.Manifest.Summary()
	item["builtin"] = e.Builtin
	if e.Dir != "" {
		item["dir"] = e.Dir
	}
	writeJSON(w, 200, item)
}

func (s *Server) testConnector(w http.ResponseWriter, r *http.Request) {
	reg, err := connectors.Default()
	if err != nil {
		writeErr(w, 500, "connectors: "+err.Error())
		return
	}
	e := reg.Get(r.PathValue("id"))
	if e == nil {
		writeErr(w, 404, "connector not found")
		return
	}
	var req struct {
		Params map[string]string `json:"params"`
		Input  map[string]any    `json:"input"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := e.Manifest.ValidateParams(req.Params); err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	preview, warnings, err := connectors.Preview(e, req.Params, req.Input)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error(), "warnings": warnings})
		return
	}
	resp := map[string]any{"ok": true, "preview": preview}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	if e.Manifest.Executor == connectors.KindWASM {
		module := connectors.ModuleBytes(e)
		if len(module) == 0 {
			resp["ok"] = false
			resp["error"] = "wasm module not found: " + e.Manifest.WASM.Module
			writeJSON(w, 200, resp)
			return
		}
		inputJSON, _ := json.Marshal(map[string]any{"params": req.Params, "input": req.Input})
		result, logs, runErr := wasm.Run(module, inputJSON, policy.FromEnv(os.Getenv))
		if runErr != nil {
			resp["ok"] = false
			resp["error"] = runErr.Error()
		} else {
			resp["result"] = result
		}
		resp["logs"] = logs
	}
	writeJSON(w, 200, resp)
}

// validateConnectors enforces the D1 contract at the approval gate: connector
// steps must name an installed connector and satisfy its params schema.
func (s *Server) validateConnectors(wf *models.Workflow) error {
	reg, err := connectors.Default()
	if err != nil {
		return err
	}
	for _, st := range wf.Steps {
		if st.Type != "connector" {
			continue
		}
		id := strings.TrimSpace(st.Params["connector"])
		if id == "" {
			return errText(`step "` + st.Name + `" is missing the "connector" param`)
		}
		e := reg.Get(id)
		if e == nil {
			return errText("unknown connector \"" + id + "\" (installed: " + strings.Join(reg.IDs(), ", ") + ")")
		}
		if err := e.Manifest.ValidateParams(st.Params); err != nil {
			return errText("connector \"" + id + "\": " + err.Error())
		}
	}
	return nil
}

type errText string

func (e errText) Error() string { return string(e) }

// ---- Templates --------------------------------------------------------------

func (s *Server) listTemplates(w http.ResponseWriter, _ *http.Request) {
	list, err := templates.List()
	if err != nil {
		writeErr(w, 500, "templates: "+err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	t, err := templates.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "template not found")
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) instantiateTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wf, err := templates.Instantiate(id)
	if err != nil {
		writeErr(w, 404, "template not found")
		return
	}
	wf.ID = "wf-" + util.UID()
	wf.CreatedAt = nowISO()
	if err := s.store.UpsertWorkflow(wf); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.audit("You", "Workflow created from template", wf.Name+" — "+itoa(len(wf.Steps))+" steps (template: "+id+")", "deploy")
	writeJSON(w, 200, wf)
}

// ---- Secrets ----------------------------------------------------------------

var secretNameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func (s *Server) listSecrets(w http.ResponseWriter, _ *http.Request) {
	v, err := secrets.Default()
	if err != nil {
		writeErr(w, 500, "secrets: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"names": v.Names()})
}

func (s *Server) putSecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name, Value string
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !secretNameRe.MatchString(req.Name) {
		writeErr(w, 400, "secret name must be UPPER_SNAKE_CASE")
		return
	}
	v, err := secrets.Default()
	if err != nil {
		writeErr(w, 500, "secrets: "+err.Error())
		return
	}
	if err := v.Set(req.Name, req.Value); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.audit("You", "Secret stored", req.Name+" (value encrypted, never displayed)", "deploy")
	writeJSON(w, 200, map[string]any{"ok": true, "name": req.Name})
}

func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request) {
	v, err := secrets.Default()
	if err != nil {
		writeErr(w, 500, "secrets: "+err.Error())
		return
	}
	if err := v.Delete(r.PathValue("name")); err != nil {
		writeErr(w, 404, "secret not found")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

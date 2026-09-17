// Artifact → workflow conversion shared by the standalone runner and
// artifact import (both consume portable flowforge/v1 files).
package spec

import (
	"github.com/flowforge/flowforge/internal/models"
)

// ToWorkflow converts a validated artifact into a workflow the engine can
// run. The caller assigns identity and lifecycle fields (ID, status,
// CreatedBy, ApprovedBy, CreatedAt) per its context.
func ToWorkflow(sp *WorkflowSpec) models.Workflow {
	trigParams := map[string]string{"event": sp.Spec.Trigger.Event}
	if sp.Spec.Trigger.Schedule != "" {
		trigParams["schedule"] = sp.Spec.Trigger.Schedule
	}
	wf := models.Workflow{
		Name:        DisplayName(sp.Metadata.Name),
		Description: sp.Spec.Description,
		Prompt:      "imported from artifact " + sp.Metadata.Name + ".flow.yaml",
		Version:     sp.Metadata.Version,
		AIModel:     sp.Metadata.AuthoredWith,
		Steps: []models.WorkflowStep{
			{ID: "trigger", Type: "trigger", Name: "Trigger", Params: trigParams, Confidence: 90, Assumptions: []string{}},
		},
	}
	for _, st := range sp.Spec.Steps {
		params := map[string]string{}
		for k, v := range st.Params {
			params[k] = v
		}
		wf.Steps = append(wf.Steps, models.WorkflowStep{
			ID: st.ID, Type: st.Type, Name: st.Name, Params: params,
			Confidence: 90, Assumptions: []string{},
		})
	}
	return wf
}

// DisplayName turns a kebab-case metadata name into a human title.
func DisplayName(kebab string) string {
	out := ""
	upper := true
	for _, c := range kebab {
		if c == '-' || c == '_' {
			out += " "
			upper = true
			continue
		}
		if upper && c >= 'a' && c <= 'z' {
			out += string(rune(c - 32))
		} else {
			out += string(c)
		}
		upper = false
	}
	return out
}

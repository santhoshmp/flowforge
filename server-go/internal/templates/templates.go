// Package templates is the P4.4 template gallery: flowforge/v1 artifacts
// (embedded from gallery/) exposed over the API so a new workflow can start
// from a proven pattern instead of a blank canvas. Every gallery file must
// parse + validate against the frozen DSL (checked in tests, TPL-01).
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/spec"
)

//go:embed gallery
var galleryFS embed.FS

// Info describes one gallery template.
type Info struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Steps       int    `json:"steps"`
}

type index struct {
	Templates []struct {
		File     string `yaml:"file"`
		Category string `yaml:"category"`
	} `yaml:"templates"`
}

func loadIndex() (index, error) {
	var ix index
	raw, err := fs.ReadFile(galleryFS, "gallery/index.yaml")
	if err != nil {
		return ix, err
	}
	if err := yaml.Unmarshal(raw, &ix); err != nil {
		return ix, fmt.Errorf("gallery index: %v", err)
	}
	return ix, nil
}

// List returns the gallery (validated documents only).
func List() ([]Info, error) {
	ix, err := loadIndex()
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(ix.Templates))
	for _, t := range ix.Templates {
		s, err := parseFile(t.File)
		if err != nil {
			return nil, err
		}
		out = append(out, Info{
			ID:          fileID(t.File),
			Name:        s.Metadata.Name,
			Description: s.Spec.Description,
			Category:    t.Category,
			Steps:       len(s.Spec.Steps),
		})
	}
	return out, nil
}

// Get returns one validated template document by id.
func Get(id string) (*spec.WorkflowSpec, error) {
	file := id + ".flow.yaml"
	if !validFile(file) {
		return nil, fs.ErrNotExist
	}
	return parseFile(file)
}

// validFile guards against path traversal (ids must match a gallery file).
func validFile(file string) bool {
	ix, err := loadIndex()
	if err != nil {
		return false
	}
	for _, t := range ix.Templates {
		if t.File == file {
			return true
		}
	}
	return false
}

func parseFile(file string) (*spec.WorkflowSpec, error) {
	raw, err := fs.ReadFile(galleryFS, "gallery/"+file)
	if err != nil {
		return nil, err
	}
	s, err := spec.ParseYAML(string(raw))
	if err != nil {
		return nil, fmt.Errorf("template %s: %v", file, err)
	}
	return s, nil
}

func fileID(file string) string {
	return strings.TrimSuffix(file, ".flow.yaml")
}

// TemplateConfidence is the confidence assigned to template steps (a human
// curated them; they carry no AI assumptions).
const TemplateConfidence = 90

// Instantiate converts a template into a draft workflow (the caller assigns
// IDs, timestamps, and persistence).
func Instantiate(id string) (models.Workflow, error) {
	s, err := Get(id)
	if err != nil {
		return models.Workflow{}, err
	}
	wf := models.Workflow{
		Name:        displayName(s.Metadata.Name),
		Description: s.Spec.Description,
		Prompt:      "started from template: " + s.Metadata.Name,
		Status:      models.StatusDraft,
		Version:     1,
		CreatedBy:   "You",
		AIModel:     "template",
	}
	// The trigger event becomes the first step (engine contract).
	wf.Steps = append(wf.Steps, models.WorkflowStep{
		ID: "trigger", Type: "trigger", Name: "Trigger",
		Params:     map[string]string{"event": s.Spec.Trigger.Event},
		Confidence: TemplateConfidence, Assumptions: []string{},
	})
	seen := map[string]int{}
	for _, st := range s.Spec.Steps {
		params := map[string]string{}
		for k, v := range st.Params {
			params[k] = v
		}
		id := st.ID
		if n, dup := seen[id]; dup {
			seen[id] = n + 1
			id = fmt.Sprintf("%s_%d", id, seen[id])
		} else {
			seen[id] = 0
		}
		wf.Steps = append(wf.Steps, models.WorkflowStep{
			ID: id, Type: st.Type, Name: st.Name, Params: params,
			Confidence: TemplateConfidence, Assumptions: []string{},
		})
	}
	return wf, nil
}

// displayName turns a kebab id into a human title.
func displayName(kebab string) string {
	words := strings.Split(kebab, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

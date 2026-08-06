// Package spec is the Go implementation of the flowforge/v1 contract.
// It mirrors @flowforge/dsl (TypeScript): the same JSON-Schema-validated
// shape, parse, validate, and serialize, so the central API and the portable
// runner consume one identical artifact. The conformance scenarios in
// spec_test.go are the Go side of DSL-02 / DSL-03 (see docs/test-strategy.md).
package spec

import (
	"bytes"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// WorkflowSpec is the top-level flowforge/v1 document.
type WorkflowSpec struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Body     `yaml:"spec"`
}

// Metadata is authoring/provenance metadata (not executed).
type Metadata struct {
	Name         string `yaml:"name"`
	Version      int    `yaml:"version"`
	CreatedBy    string `yaml:"createdBy"`
	ApprovedBy   string `yaml:"approvedBy,omitempty"`
	AuthoredWith string `yaml:"authoredWith,omitempty"`
}

// Body is the executable spec.
type Body struct {
	Description string  `yaml:"description,omitempty"`
	Trigger     Trigger `yaml:"trigger"`
	Steps       []Step  `yaml:"steps"`
}

// Trigger holds the start event and common params. (General arbitrary trigger
// params are supported by the TS schema; the Go runner models the common
// subset here and will generalize to a typed map in P1 alongside the engine.)
type Trigger struct {
	Event  string `yaml:"event"`
	Source string `yaml:"source,omitempty"`
}

// Step is one workflow step. Params values are strings by contract.
type Step struct {
	ID          string            `yaml:"id"`
	Type        string            `yaml:"type"`
	Name        string            `yaml:"name"`
	Params      map[string]string `yaml:"params,omitempty"`
	OnSLABreach string            `yaml:"on_sla_breach,omitempty"`
}

const APIVersion = "flowforge/v1"

var (
	kebabRe    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	validTypes = map[string]bool{
		"trigger": true, "ai.extract": true, "ai.classify": true,
		"mdm.lookup": true, "mdm.validate": true, "condition": true,
		"human.approval": true, "notify": true, "integration.post": true,
		"integration.http": true, "script": true, "wait": true,
	}
)

// Validate checks the document against the flowforge/v1 rules (the Go mirror
// of the JSON Schema in dsl/src/schema.json).
func Validate(s *WorkflowSpec) error {
	if s.APIVersion != APIVersion {
		return fmt.Errorf("apiVersion must be %q (got %q)", APIVersion, s.APIVersion)
	}
	if s.Kind != "Workflow" {
		return fmt.Errorf("kind must be %q (got %q)", "Workflow", s.Kind)
	}
	m := s.Metadata
	if !kebabRe.MatchString(m.Name) {
		return fmt.Errorf("metadata.name must be kebab-case (got %q)", m.Name)
	}
	if m.Version < 1 {
		return fmt.Errorf("metadata.version must be >= 1 (got %d)", m.Version)
	}
	if m.CreatedBy == "" {
		return fmt.Errorf("metadata.createdBy is required")
	}
	if s.Spec.Trigger.Event == "" {
		return fmt.Errorf("spec.trigger.event is required")
	}
	seen := make(map[string]bool, len(s.Spec.Steps))
	for i, st := range s.Spec.Steps {
		if st.ID == "" {
			return fmt.Errorf("steps[%d].id is required", i)
		}
		if !validTypes[st.Type] {
			return fmt.Errorf("steps[%d] (%s): unknown type %q", i, st.ID, st.Type)
		}
		if seen[st.ID] {
			return fmt.Errorf("steps[%d]: duplicate id %q", i, st.ID)
		}
		seen[st.ID] = true
	}
	return nil
}

// ParseYAML parses and validates a flowforge/v1 document.
func ParseYAML(text string) (*WorkflowSpec, error) {
	var s WorkflowSpec
	if err := yaml.Unmarshal([]byte(text), &s); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if err := Validate(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ToYAML serializes a spec canonically. yaml.v3 quotes string values that would
// otherwise be coerced (e.g. "48"), so params round-trip as strings.
func ToYAML(s *WorkflowSpec) (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(s); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Summary is a one-line human description used by the CLI.
func (s *WorkflowSpec) Summary() string {
	return fmt.Sprintf("%s v%d (%d steps, trigger %q)", s.Metadata.Name, s.Metadata.Version, len(s.Spec.Steps), s.Spec.Trigger.Event)
}

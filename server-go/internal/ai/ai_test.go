package ai

import (
	"testing"

	"github.com/flowforge/flowforge/internal/models"
)

// Go mirror of AI-01/AI-02 (deterministic path only). Feature F-AI.
func TestDeterministicFallback(t *testing.T) {
	cfg := models.AIConfig{Provider: "openai", APIKey: "", BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"}
	res := GenerateDraft("When a vendor invoice over $10K arrives, extract line items, validate against the vendor master, route to the manager for approval, then post to the ERP.", cfg)

	if res.Source != "fallback" {
		t.Errorf("source = %q, want fallback", res.Source)
	}
	d := res.Draft
	if len(d.Steps) < 2 {
		t.Fatalf("too few steps: %d", len(d.Steps))
	}
	if d.Steps[0].Type != "trigger" {
		t.Errorf("first step must be trigger, got %q", d.Steps[0].Type)
	}
	triggers := 0
	for _, s := range d.Steps {
		if s.Type == "trigger" {
			triggers++
		}
	}
	if triggers != 1 {
		t.Errorf("want exactly 1 trigger, got %d", triggers)
	}
	hasCond, hasAppr, hasPost := false, false, false
	for _, s := range d.Steps {
		switch s.Type {
		case "condition":
			hasCond = true
		case "human.approval":
			hasAppr = true
		case "integration.post":
			hasPost = true
		}
	}
	if !hasCond || !hasAppr || !hasPost {
		t.Errorf("expected condition + approval + post: %v %v %v", hasCond, hasAppr, hasPost)
	}
	for _, s := range d.Steps {
		if s.Confidence < 50 || s.Confidence > 99 {
			t.Errorf("confidence out of range: %d", s.Confidence)
		}
		if s.Assumptions == nil {
			t.Errorf("step %q assumptions must not be nil (JSON contract: [])", s.ID)
		}
		if s.Params == nil {
			t.Errorf("step %q params must not be nil (JSON contract: {})", s.ID)
		}
	}
	if d.OverallConfidence <= 0 {
		t.Errorf("overall confidence should be positive")
	}
}

package templates

// TPL-03: every gallery entry instantiates into a valid draft (loop, not
// spot-check). Feature F-EXT (P4.4).

import (
	"testing"

	"github.com/flowforge/flowforge/internal/models"
)

func TestTPL03_EveryTemplateInstantiates(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 6 {
		t.Fatalf("gallery = %d, want >= 6", len(list))
	}
	categories := map[string]bool{}
	for _, info := range list {
		wf, err := Instantiate(info.ID)
		if err != nil {
			t.Errorf("instantiate %s: %v", info.ID, err)
			continue
		}
		if wf.Status != models.StatusDraft || wf.Version != 1 {
			t.Errorf("%s: status=%s version=%d, want draft/1", info.ID, wf.Status, wf.Version)
		}
		// Every non-trigger template step is present, in order, with string params.
		if len(wf.Steps) != info.Steps+1 {
			t.Errorf("%s: steps = %d, want %d (trigger + template)", info.ID, len(wf.Steps), info.Steps+1)
		}
		if wf.Steps[0].Type != "trigger" || wf.Steps[0].Params["event"] == "" {
			t.Errorf("%s: first step must be the trigger with an event: %+v", info.ID, wf.Steps[0])
		}
		ids := map[string]bool{}
		for _, s := range wf.Steps {
			if ids[s.ID] {
				t.Errorf("%s: duplicate step id %s", info.ID, s.ID)
			}
			ids[s.ID] = true
			if s.Name == "" || s.Confidence <= 0 {
				t.Errorf("%s: step %s missing name/confidence", info.ID, s.ID)
			}
			for _, v := range s.Params {
				if v == "" {
					t.Errorf("%s: step %s has an empty param value", info.ID, s.ID)
				}
			}
		}
		if wf.Description == "" || wf.Prompt == "" || wf.CreatedBy == "" || wf.AIModel != "template" {
			t.Errorf("%s: provenance fields missing: %+v", info.ID, wf)
		}
		categories[info.Category] = true
	}
	// The gallery spans all three advertised categories.
	for _, cat := range []string{"finance", "hr", "operations"} {
		if !categories[cat] {
			t.Errorf("category %s not represented", cat)
		}
	}
}

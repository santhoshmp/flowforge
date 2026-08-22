package templates

// TPL-01/02: the template gallery (P4.4). Feature F-EXT.

import (
	"strings"
	"testing"
)

func TestTPL01_GalleryValidates(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 6 {
		t.Fatalf("gallery size = %d, want 6", len(list))
	}
	for _, info := range list {
		s, err := Get(info.ID)
		if err != nil {
			t.Errorf("template %s: %v", info.ID, err)
			continue
		}
		if s.Metadata.Name != info.Name || len(s.Spec.Steps) != info.Steps {
			t.Errorf("template %s: info/doc mismatch (%+v)", info.ID, info)
		}
		if info.Category == "" {
			t.Errorf("template %s: missing category", info.ID)
		}
	}
	// Path traversal is not a template id.
	if _, err := Get("../../etc/passwd"); err == nil {
		t.Error("traversal id should be rejected")
	}
}

func TestTPL02_InstantiateCreatesDraft(t *testing.T) {
	wf, err := Instantiate("vendor-invoice-approval")
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	if wf.Status != "draft" {
		t.Errorf("status = %s, want draft", wf.Status)
	}
	// trigger step + 6 template steps
	if len(wf.Steps) != 7 {
		t.Fatalf("steps = %d, want 7", len(wf.Steps))
	}
	if wf.Steps[0].Type != "trigger" || wf.Steps[0].Params["event"] != "vendor_invoice.created" {
		t.Errorf("trigger step = %+v", wf.Steps[0])
	}
	for _, s := range wf.Steps {
		if s.Confidence != TemplateConfidence {
			t.Errorf("step %s confidence = %d", s.ID, s.Confidence)
		}
		for _, v := range s.Params {
			if v != "" && !isStringable(v) {
				t.Errorf("step %s has non-string param %v", s.ID, v)
			}
		}
	}
	// Duplicate step ids across a template are uniqued.
	wf2, err := Instantiate("employee-onboarding")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range wf2.Steps {
		if seen[s.ID] {
			t.Errorf("duplicate step id %s", s.ID)
		}
		seen[s.ID] = true
	}
	if !strings.HasPrefix(wf2.Name, "Employee Onboarding") {
		t.Errorf("name = %q", wf2.Name)
	}
}

func isStringable(v any) bool {
	_, ok := v.(string)
	return ok
}

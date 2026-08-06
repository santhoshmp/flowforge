package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Go mirror of DSL-02 (round-trip) and DSL-03 (rejection). These are the
// CONF-* conformance scenarios the Go distributable must pass to match the
// Node reference (@flowforge/dsl).

func mustRead(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		b, err = os.ReadFile(rel)
	}
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

func sampleSpec() *WorkflowSpec {
	return &WorkflowSpec{
		APIVersion: APIVersion,
		Kind:       "Workflow",
		Metadata: Metadata{
			Name: "vendor-invoice-approval", Version: 3, CreatedBy: "Priya",
			ApprovedBy: "Ravi", AuthoredWith: "flowforge-author",
		},
		Spec: Body{
			Description: "Extract, validate, approve and post vendor invoices over $10K.",
			Trigger:     Trigger{Event: "vendor_invoice.created", Source: "any"},
			Steps: []Step{
				{ID: "extract", Type: "ai.extract", Name: "Extract line items", Params: map[string]string{"fields": "x, y", "model": "auto"}},
				{ID: "validate", Type: "mdm.validate", Name: "Validate vendor", Params: map[string]string{"entity": "vendors"}},
				{ID: "amount", Type: "condition", Name: "Amount over 10000", Params: map[string]string{"expression": "total > 10000"}},
				{ID: "approve", Type: "human.approval", Name: "Manager approval", Params: map[string]string{"approver": "Manager", "sla_hours": "48"}, OnSLABreach: "escalate"},
				{ID: "post", Type: "integration.post", Name: "Post to ERP", Params: map[string]string{"system": "ERP"}},
			},
		},
	}
}

func TestRoundTrip(t *testing.T) {
	cases := []*WorkflowSpec{sampleSpec(), {
		APIVersion: APIVersion, Kind: "Workflow",
		Metadata: Metadata{Name: "ticket-routing", Version: 1, CreatedBy: "Carlos"},
		Spec: Body{Trigger: Trigger{Event: "support_ticket.created"},
			Steps: []Step{
				{ID: "classify", Type: "ai.classify", Name: "Classify priority"},
				{ID: "lead", Type: "human.approval", Name: "Support Lead approval", Params: map[string]string{"sla_hours": "4"}},
			}},
	}}
	for i, s := range cases {
		out, err := ToYAML(s)
		if err != nil {
			t.Fatalf("case %d ToYAML: %v", i, err)
		}
		parsed, err := ParseYAML(out)
		if err != nil {
			t.Fatalf("case %d ParseYAML: %v", i, err)
		}
		if !reflect.DeepEqual(parsed, s) {
			t.Fatalf("case %d round-trip mismatch:\nwant %+v\ngot  %+v", i, s, parsed)
		}
	}
}

func TestParseFixture(t *testing.T) {
	text := mustRead(t, filepath.Join("fixtures", "vendor-invoice.flow.yaml"))
	s, err := ParseYAML(text)
	if err != nil {
		t.Fatalf("fixture parse: %v", err)
	}
	if s.Metadata.Name != "vendor-invoice-approval" || s.Metadata.Version != 3 {
		t.Fatalf("unexpected metadata: %+v", s.Metadata)
	}
	if len(s.Spec.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(s.Spec.Steps))
	}
	// params must come back as strings (sla_hours quoted in the fixture)
	if v := s.Spec.Steps[3].Params["sla_hours"]; v != "48" {
		t.Fatalf("sla_hours not string '48': %q", v)
	}
}

func TestRejectInvalid(t *testing.T) {
	good := sampleSpec()

	clone := func() *WorkflowSpec { c := *good; return &c }

	badVersion := clone()
	badVersion.APIVersion = "flowforge/v0"

	badType := clone()
	badType.Spec.Steps[0].Type = "rocket.launch"

	badName := clone()
	badName.Metadata.Name = "Bad Name"

	missingEvent := clone()
	missingEvent.Spec.Trigger.Event = ""

	dupID := clone()
	dupID.Spec.Steps = append(dupID.Spec.Steps, Step{ID: "post", Type: "script", Name: "Dup"})

	cases := []struct {
		name string
		spec *WorkflowSpec
	}{
		{"wrong apiVersion", badVersion},
		{"unknown step type", badType},
		{"non-slug name", badName},
		{"missing trigger event", missingEvent},
		{"duplicate step id", dupID},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Validate(c.spec); err == nil {
				t.Fatalf("expected validation error for %s", c.name)
			}
		})
	}
}

func TestValidateAcceptsValid(t *testing.T) {
	if err := Validate(sampleSpec()); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
}

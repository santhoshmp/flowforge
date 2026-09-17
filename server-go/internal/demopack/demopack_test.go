package demopack

// DEM-01..05: the Meridian Components demo pack (feature F-DEMO).

import (
	"testing"
	"time"

	"github.com/flowforge/flowforge/internal/metrics"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/spec"
	"github.com/flowforge/flowforge/internal/store"
)

// parseYAML is a thin alias to the frozen-DSL parser.
func parseYAML(s string) (*spec.WorkflowSpec, error) { return spec.ParseYAML(s) }

// anchor is the clock for history dates. It tracks the real now (truncated)
// because metrics.Compute builds its 14-day window from time.Now() — a hard
// fixed date would drift out of the window.
var anchor = time.Now().UTC().Truncate(time.Hour)

// DEM-01: every embedded artifact parses + validates against the frozen DSL.
func TestDEM01_ArtifactsValidate(t *testing.T) {
	entries, err := workflowsFS.ReadDir("workflows")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 6 {
		t.Fatalf("demo pack = %d workflows, want 6", len(entries))
	}
	for _, e := range entries {
		raw, err := workflowsFS.ReadFile("workflows/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		sp, err := parseYAML(string(raw))
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		if sp.Metadata.Name == "" || len(sp.Spec.Steps) < 3 {
			t.Errorf("%s: not a meaningful demo workflow", e.Name())
		}
	}
}

// DEM-02: Load populates deployed workflows, run history, MDM, audit.
func TestDEM02_Load(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SeedIfEmpty(); err != nil {
		t.Fatal(err)
	}

	sum, err := Load(s, anchor)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if sum.Org != "Meridian Components" || sum.Workflows != 6 {
		t.Fatalf("summary: %+v", sum)
	}

	wfs, _ := s.ListWorkflows()
	demo := 0
	for _, w := range wfs {
		if w.CreatedBy == "Meridian demo pack" {
			demo++
			if w.Status != models.StatusDeployed || w.ApprovedBy == "" {
				t.Errorf("%s: status=%s approvedBy=%q", w.Name, w.Status, w.ApprovedBy)
			}
		}
	}
	if demo != 6 {
		t.Fatalf("demo workflows = %d, want 6", demo)
	}

	// Every demo workflow carries Meridian approvers.
	wf, _ := s.GetWorkflow("wf-demo-vendor-invoice-approval")
	if wf == nil || wf.ApprovedBy != "Priya Raman (CFO)" {
		t.Fatalf("invoice workflow: %+v", wf)
	}
	foundApprover := false
	for _, st := range wf.Steps {
		if st.Params["approver"] == "Aisha Khan (Controller)" {
			foundApprover = true
		}
	}
	if !foundApprover {
		t.Fatal("Controller approval step missing")
	}
}

// DEM-03: the dashboards light up — 14-day series, statuses, pending tasks.
func TestDEM03_DashboardsAlive(t *testing.T) {
	s, _ := store.Open(":memory:")
	defer s.Close()
	_ = s.SeedIfEmpty()
	if _, err := Load(s, anchor); err != nil {
		t.Fatal(err)
	}
	m, err := metrics.Compute(s)
	if err != nil {
		t.Fatal(err)
	}
	if m.Fleet.Completed < 12 {
		t.Errorf("completed runs = %d, want a healthy history", m.Fleet.Completed)
	}
	// Waiting tasks created BY THE PACK (one per department story).
	insts, _ := s.ListInstances()
	demoWaiting := 0
	var waiting *models.Instance
	for i := range insts {
		in := insts[i]
		if in.Status == models.InstWaiting && stringsHasPrefix(in.WorkflowID, "wf-demo-") {
			demoWaiting++
			if in.WorkflowID == "wf-demo-vendor-invoice-approval" {
				w := in
				waiting = &w
			}
		}
	}
	if demoWaiting != 6 {
		t.Errorf("demo waiting human tasks = %d, want 6 (one live task per workflow)", demoWaiting)
	}
	if m.Fleet.Failed < 1 {
		t.Error("expected the failed-then-retried invoice story")
	}
	total := 0
	for _, b := range m.ByDay {
		total += b.Total
	}
	if total < m.Fleet.TotalRuns {
		t.Errorf("byDay total = %d but fleet = %d (history outside the 14-day window?)", total, m.Fleet.TotalRuns)
	}
	// The waiting instance is a resolvable human task with the org's name.
	if waiting == nil || waiting.WaitingOn != "Aisha Khan (Controller)" {
		t.Fatalf("waiting invoice task: %+v", waiting)
	}
}

func stringsHasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}

// DEM-04: the MDM governance stories are present (duplicate + mismatch).
func TestDEM04_MasterDataStories(t *testing.T) {
	s, _ := store.Open(":memory:")
	defer s.Close()
	_ = s.SeedIfEmpty()
	if _, err := Load(s, anchor); err != nil {
		t.Fatal(err)
	}
	mdm, _ := s.ListMDM()
	pending, employees := 0, ""
	for _, e := range mdm {
		if e.Key != "vendors" && e.Key != "employees" {
			continue
		}
		for _, r := range e.Records {
			if r["status"] == "pending stewardship" {
				pending++
				if e.Key == "employees" && r["name"] == "Jonas de Vries" {
					employees = r["id"]
				}
			}
		}
	}
	if pending < 3 {
		t.Errorf("pending-stewardship records = %d, want the duplicate + mismatch + new hire", pending)
	}
	if employees != "E-4417" {
		t.Errorf("new hire pending record missing: %q", employees)
	}
}

// DEM-05: Load is idempotent — reruns neither duplicate nor corrupt.
func TestDEM05_Idempotent(t *testing.T) {
	s, _ := store.Open(":memory:")
	defer s.Close()
	_ = s.SeedIfEmpty()
	if _, err := Load(s, anchor); err != nil {
		t.Fatal(err)
	}
	insts1, _ := s.ListInstances()
	audit1, _ := s.ListAudit()
	if _, err := Load(s, anchor); err != nil {
		t.Fatal(err)
	}
	insts2, _ := s.ListInstances()
	audit2, _ := s.ListAudit()
	if len(insts2) != len(insts1) {
		t.Errorf("instances after re-load = %d, want %d", len(insts2), len(insts1))
	}
	if len(audit2) < len(audit1) {
		t.Errorf("audit after re-load = %d, want >= %d", len(audit2), len(audit1))
	}
	wfs, _ := s.ListWorkflows()
	demo := 0
	for _, w := range wfs {
		if w.CreatedBy == "Meridian demo pack" {
			demo++
		}
	}
	if demo != 6 {
		t.Errorf("demo workflows after re-load = %d, want 6", demo)
	}
}

package store

// STORE-01..04: persistence roundtrips for every collection. Features
// F-PERSIST-01, F-CTRL, F-MDM, F-API (settings backing).

import (
	"testing"

	"github.com/flowforge/flowforge/internal/models"
)

// STORE-01: workflow upsert → get → update → get roundtrip (nested steps JSON).
func TestSTORE01_WorkflowRoundtrip(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	wf := models.Workflow{
		ID: "wf-1", Name: "Roundtrip", Description: "d", Prompt: "p",
		Status: models.StatusDraft, Version: 2,
		Steps: []models.WorkflowStep{
			{ID: "a", Type: "trigger", Name: "T", Params: map[string]string{"event": "e"}, Confidence: 90, Assumptions: []string{"x"}},
			{ID: "b", Type: "connector", Name: "C", Params: map[string]string{"connector": "http-json", "url": "https://x"}, Confidence: 80, Assumptions: []string{}},
		},
		CreatedBy: "tester", AIModel: "m", CreatedAt: "2026-01-01T00:00:00Z", Runs: 3,
	}
	if err := s.UpsertWorkflow(wf); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetWorkflow("wf-1")
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Name != "Roundtrip" || got.Version != 2 || got.Runs != 3 || len(got.Steps) != 2 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.Steps[1].Type != "connector" || got.Steps[1].Params["connector"] != "http-json" {
		t.Fatalf("connector step params lost: %+v", got.Steps[1])
	}
	if got.Steps[0].Assumptions[0] != "x" {
		t.Fatalf("assumptions lost: %+v", got.Steps[0])
	}

	// Update in place (upsert on same id).
	wf.Status = models.StatusDeployed
	wf.ApprovedBy = "rev"
	wf.Runs = 4
	if err := s.UpsertWorkflow(wf); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetWorkflow("wf-1")
	if got.Status != models.StatusDeployed || got.ApprovedBy != "rev" || got.Runs != 4 {
		t.Fatalf("update mismatch: %+v", got)
	}
	if list, _ := s.ListWorkflows(); len(list) != 1 {
		t.Fatalf("list = %d, want 1 (update must not duplicate)", len(list))
	}

	// Missing id resolves to nil.
	if miss, _ := s.GetWorkflow("nope"); miss != nil {
		t.Fatalf("missing workflow = %+v", miss)
	}
}

// STORE-02: instance upsert preserves step runs, input, and terminal fields.
func TestSTORE02_InstanceRoundtrip(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()

	inst := models.Instance{
		ID: "run-1", WorkflowID: "wf-1", WorkflowName: "W", Status: models.InstWaiting,
		Entity: "INV-1", StartedAt: "2026-01-01T00:00:00Z", EndedAt: "",
		Input:       map[string]any{"total": float64(42), "nested": map[string]any{"k": "v"}},
		CurrentStep: 2, WaitingOn: "Manager",
		StepRuns: []models.StepRun{
			{StepID: "a", Name: "T", Type: "trigger", Status: "succeeded", DurationMs: 12, Output: "record received"},
			{StepID: "b", Name: "M", Type: "human.approval", Status: "waiting", Note: "Waiting on Manager · SLA 24h"},
		},
	}
	if err := s.UpsertInstance(inst); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetInstance("run-1")
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Status != models.InstWaiting || got.WaitingOn != "Manager" || got.CurrentStep != 2 {
		t.Fatalf("instance mismatch: %+v", got)
	}
	if len(got.StepRuns) != 2 || got.StepRuns[1].Note != "Waiting on Manager · SLA 24h" {
		t.Fatalf("step runs lost: %+v", got.StepRuns)
	}
	if v, ok := got.Input["total"].(float64); !ok || v != 42 {
		t.Fatalf("input lost: %+v", got.Input)
	}
	if n, ok := got.Input["nested"].(map[string]any); !ok || n["k"] != "v" {
		t.Fatalf("nested input lost: %+v", got.Input)
	}

	// Terminal update: error + endedAt set on conflict path.
	got.Status = models.InstFailed
	got.Error = "egress blocked"
	got.EndedAt = "2026-01-01T01:00:00Z"
	if err := s.UpsertInstance(*got); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetInstance("run-1")
	if after.Error != "egress blocked" || after.EndedAt == "" {
		t.Fatalf("terminal fields lost: %+v", after)
	}
	if list, _ := s.ListInstances(); len(list) != 1 {
		t.Fatalf("list = %d, want 1", len(list))
	}
}

// STORE-03: controls CRUD + MDM upsert + settings kv + audit append-only growth.
func TestSTORE03_ControlsMDMSettingsAudit(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()

	// Controls: insert custom, toggle by upsert, delete.
	c := models.ControlDef{Key: "custom.sms", Label: "SMS", Color: "violet", Icon: "Bell", Enabled: true, Custom: true, Description: "d"}
	if err := s.UpsertControl(c); err != nil {
		t.Fatal(err)
	}
	c.Enabled = false
	_ = s.UpsertControl(c)
	got, _ := s.ListControls()
	found := false
	for _, x := range got {
		if x.Key == "custom.sms" {
			found = true
			if x.Enabled || !x.Custom || x.Description != "d" {
				t.Fatalf("control mismatch: %+v", x)
			}
		}
	}
	if !found {
		t.Fatal("custom control missing")
	}
	if err := s.DeleteControl("custom.sms"); err != nil {
		t.Fatal(err)
	}
	for _, x := range mustList(s) {
		if x.Key == "custom.sms" {
			t.Fatal("delete failed")
		}
	}

	// MDM upsert preserves records.
	e := models.MDMEntity{Key: "vendors", Label: "Vendors", Icon: "Building2", Fields: []string{"id", "name"}, Records: []map[string]string{{"id": "V-1", "name": "Acme", "status": "golden"}}}
	_ = s.UpsertMDM(e)
	e.Records = append(e.Records, map[string]string{"id": "V-2", "name": "Globex", "status": "pending stewardship"})
	_ = s.UpsertMDM(e)
	mdm, _ := s.ListMDM()
	if len(mdm) != 1 || len(mdm[0].Records) != 2 || mdm[0].Records[1]["status"] != "pending stewardship" {
		t.Fatalf("mdm mismatch: %+v", mdm)
	}

	// Settings kv: absent → present → overwrite.
	if _, ok, _ := s.GetSetting("ai"); ok {
		t.Fatal("unset setting should be absent")
	}
	_ = s.SetSetting("ai", `{"provider":"openai"}`)
	if v, ok, _ := s.GetSetting("ai"); !ok || v != `{"provider":"openai"}` {
		t.Fatalf("setting mismatch: %q %v", v, ok)
	}
	_ = s.SetSetting("ai", `{"provider":"ollama"}`)
	if v, _, _ := s.GetSetting("ai"); v != `{"provider":"ollama"}` {
		t.Fatalf("setting overwrite failed: %q", v)
	}

	// Audit grows append-only.
	for i := 0; i < 3; i++ {
		_ = s.AddAudit(models.AuditEntry{ID: string(rune('a' + i)), At: "t", Actor: "You", Action: "E", Detail: "d", Kind: "deploy"})
	}
	au, _ := s.ListAudit()
	if len(au) != 3 {
		t.Fatalf("audit = %d, want 3", len(au))
	}
}

// STORE-04: users (auth backing) — add, count, lookup by username, duplicate rejected.
func TestSTORE04_Users(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()

	if n, _ := s.CountUsers(); n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}
	u := UserRow{ID: "u-1", Username: "admin", PasswordHash: "x", Role: "admin", CreatedAt: "t"}
	if err := s.AddUser(u); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountUsers(); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
	got, _ := s.GetUserByUsername("admin")
	if got == nil || got.ID != "u-1" || got.Role != "admin" {
		t.Fatalf("lookup mismatch: %+v", got)
	}
	if miss, _ := s.GetUserByUsername("nope"); miss != nil {
		t.Fatalf("missing user = %+v", miss)
	}
	// Duplicate username violates the UNIQUE constraint.
	if err := s.AddUser(UserRow{ID: "u-2", Username: "admin", PasswordHash: "y", Role: "admin", CreatedAt: "t"}); err == nil {
		t.Fatal("duplicate username should fail")
	}
}

func mustList(s *Store) []models.ControlDef {
	c, _ := s.ListControls()
	return c
}

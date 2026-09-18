package engine

// SCHED-04..08: scheduled triggers — fire, no-refire, catch-up window,
// draft-skip, and artifact validation.

import (
	"testing"
	"time"

	"github.com/santhoshmp/flowforge/internal/models"
	"github.com/santhoshmp/flowforge/internal/spec"
	"github.com/santhoshmp/flowforge/internal/store"
)

func schedStore(t *testing.T, schedule string, status string) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	params := map[string]string{"event": "tick"}
	if schedule != "" {
		params["schedule"] = schedule
	}
	wf := models.Workflow{
		ID: "wf-sched", Name: "Sched", Status: status, Version: 1,
		CreatedBy: "t", AIModel: "t", CreatedAt: "2026-01-01T00:00:00Z",
		Steps: []models.WorkflowStep{
			{ID: "trigger", Type: "trigger", Name: "T", Params: params, Confidence: 90, Assumptions: []string{}},
			{ID: "n", Type: "notify", Name: "N", Params: map[string]string{"channel": "email"}, Confidence: 80, Assumptions: []string{}},
		},
	}
	if err := s.UpsertWorkflow(wf); err != nil {
		t.Fatal(err)
	}
	return s
}

func countInstances(t *testing.T, s *store.Store) int {
	t.Helper()
	insts, err := s.ListInstances()
	if err != nil {
		t.Fatal(err)
	}
	return len(insts)
}

// SCHED-04: a due slot starts an execution; the same slot never refires.
func TestSCHED04_FiresOncePerSlot(t *testing.T) {
	s := schedStore(t, "*/1 * * * *", models.StatusDeployed)
	now := time.Date(2026, 9, 16, 10, 0, 30, 0, time.Local) // slot 10:00, 30s ago

	started, err := TickScheduler(s, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("started = %d, want 1", len(started))
	}
	if n := countInstances(t, s); n != 1 {
		t.Fatalf("instances = %d", n)
	}
	// Same minute again: no refire.
	started, _ = TickScheduler(s, now.Add(20*time.Second))
	if len(started) != 0 || countInstances(t, s) != 1 {
		t.Fatalf("refired: started=%d instances=%d", len(started), countInstances(t, s))
	}
	// Next minute: fires again.
	started, _ = TickScheduler(s, now.Add(61*time.Second))
	if len(started) != 1 || countInstances(t, s) != 2 {
		t.Fatalf("next slot did not fire: started=%d instances=%d", len(started), countInstances(t, s))
	}
	// The scheduled instance runs on the engine like any other.
	insts, _ := s.ListInstances()
	var final *models.Instance
	for i := 0; i < 30; i++ {
		TickAll(s, nil)
		got, _ := s.GetInstance(insts[0].ID)
		if got != nil && (got.Status == models.InstCompleted || got.Status == models.InstFailed) {
			final = got
			break
		}
	}
	if final == nil || final.Status != models.InstCompleted {
		t.Fatalf("scheduled instance did not complete: %+v", final)
	}
}

// SCHED-05: drafts and unscheduled workflows are never fired.
func TestSCHED05_SkipsDraftsAndUnscheduled(t *testing.T) {
	for _, tc := range []struct {
		schedule, status string
	}{
		{"*/1 * * * *", models.StatusDraft},
		{"", models.StatusDeployed},
	} {
		s := schedStore(t, tc.schedule, tc.status)
		started, err := TickScheduler(s, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if len(started) != 0 || countInstances(t, s) != 0 {
			t.Fatalf("schedule=%q status=%s fired unexpectedly", tc.schedule, tc.status)
		}
	}
}

// SCHED-06: a slot missed by more than the catch-up window is skipped, and
// a restart-recent slot still fires.
func TestSCHED06_CatchUpWindow(t *testing.T) {
	s := schedStore(t, "0 3 * * *", models.StatusDeployed)
	fireTime := time.Date(2026, 9, 16, 3, 0, 0, 0, time.Local)

	// Fresh store, slot 4 minutes old (restart catch-up) -> fires.
	started, err := TickScheduler(s, fireTime.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("recent missed slot should fire, started=%d", len(started))
	}

	// Slot a day later, checked 7 minutes late (> 5min catch-up) -> skipped
	// but lastFired advances so it never replays.
	nextDay := fireTime.AddDate(0, 0, 1).Add(7 * time.Minute)
	started, _ = TickScheduler(s, nextDay)
	if len(started) != 0 {
		t.Fatal("stale slot (> catch-up) must not fire")
	}
	// And a later pass does not fire the skipped slot either.
	started, _ = TickScheduler(s, nextDay.Add(time.Minute))
	if len(started) != 0 || countInstances(t, s) != 1 {
		t.Fatalf("skipped slot replayed: started=%d instances=%d", len(started), countInstances(t, s))
	}
}

// SCHED-07: invalid stored schedules are ignored (validation happens at
// parse/approve; robustness at runtime).
func TestSCHED07_InvalidStoredScheduleIgnored(t *testing.T) {
	s := schedStore(t, "not a cron", models.StatusDeployed)
	started, err := TickScheduler(s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 0 {
		t.Fatal("invalid schedule fired")
	}
}

// SCHED-08: artifacts with a schedule trigger validate; bad crons are
// rejected at parse time.
func TestSCHED08_ArtifactValidation(t *testing.T) {
	if _, err := spec.ParseYAML(`apiVersion: flowforge/v1
kind: Workflow
metadata:
  name: sched-flow
  version: 1
  createdBy: t
spec:
  trigger:
    event: tick
  steps:
    - id: n
      type: notify
      name: N
      params:
        channel: email
`); err != nil { // no schedule
		t.Fatalf("artifact without schedule must parse: %v", err)
	}
	if _, err := spec.ParseYAML(`apiVersion: flowforge/v1
kind: Workflow
metadata:
  name: sched-flow2
  version: 1
  createdBy: t
spec:
  trigger:
    event: tick
    schedule: nonsense
  steps:
    - id: n
      type: notify
      name: N
      params:
        channel: email
`); err == nil {
		t.Fatal("invalid schedule accepted at parse time")
	}
	// The schedule survives the artifact -> workflow conversion.
	sp, err := spec.ParseYAML(`apiVersion: flowforge/v1
kind: Workflow
metadata:
  name: sched-flow3
  version: 1
  createdBy: t
spec:
  trigger:
    event: tick
    schedule: "0 6 * * 1-5"
  steps:
    - id: n
      type: notify
      name: N
      params:
        channel: email
`)
	if err != nil {
		t.Fatal(err)
	}
	wf := spec.ToWorkflow(sp)
	if wf.Steps[0].Params["schedule"] != "0 6 * * 1-5" {
		t.Fatalf("schedule param lost in conversion: %+v", wf.Steps[0].Params)
	}
}

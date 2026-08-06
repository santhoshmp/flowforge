package store

import (
	"testing"

	"github.com/flowforge/flowforge/internal/models"
)

// Validates that the seeded data maintains correct relationships:
// instance steps ↔ workflow steps, entity refs ↔ MDM, waiting/failed states,
// and per-workflow status coverage. (DATA-01..06)
func TestSeedConsistency(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.SeedIfEmpty(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	wfs, _ := s.ListWorkflows()
	insts, _ := s.ListInstances()
	mdm, _ := s.ListMDM()
	audit, _ := s.ListAudit()

	wfByID := map[string]models.Workflow{}
	for _, w := range wfs {
		wfByID[w.ID] = w
	}
	mdmKeys := map[string]bool{}
	for _, e := range mdm {
		mdmKeys[e.Key] = true
	}

	// DATA-01: every workflow's mdm.validate/mdm.lookup step references an existing MDM entity
	for _, w := range wfs {
		for _, st := range w.Steps {
			if st.Type == "mdm.validate" || st.Type == "mdm.lookup" {
				ref := st.Params["entity"]
				if ref == "" {
					t.Errorf("workflow %q step %q: mdm step without entity param", w.ID, st.ID)
				}
				if !mdmKeys[ref] {
					t.Errorf("workflow %q step %q: references MDM entity %q which does not exist", w.ID, st.ID, ref)
				}
			}
		}
	}

	// DATA-02: every instance references a valid workflow
	for _, inst := range insts {
		if _, ok := wfByID[inst.WorkflowID]; !ok {
			t.Errorf("instance %q references unknown workflow %q", inst.ID, inst.WorkflowID)
		}
	}

	// DATA-03: every instance's step runs match the workflow's steps (count, IDs, types)
	for _, inst := range insts {
		wf := wfByID[inst.WorkflowID]
		if len(inst.StepRuns) != len(wf.Steps) {
			t.Errorf("instance %q: %d step runs vs workflow %d steps", inst.ID, len(inst.StepRuns), len(wf.Steps))
			continue
		}
		for i, sr := range inst.StepRuns {
			ws := wf.Steps[i]
			if sr.StepID != ws.ID || sr.Type != ws.Type || sr.Name != ws.Name {
				t.Errorf("instance %q step[%d]: run {%s,%s} != wf {%s,%s}", inst.ID, i, sr.StepID, sr.Type, ws.ID, ws.Type)
			}
		}
	}

	// DATA-04: waiting instances wait on a human.approval step at currentStep
	for _, inst := range insts {
		if inst.Status != models.InstWaiting {
			continue
		}
		if inst.WaitingOn == "" {
			t.Errorf("waiting instance %q has no WaitingOn", inst.ID)
		}
		if inst.CurrentStep >= len(inst.StepRuns) {
			t.Errorf("waiting instance %q currentStep out of range", inst.ID)
			continue
		}
		cur := inst.StepRuns[inst.CurrentStep]
		if cur.Status != models.StepWaiting {
			t.Errorf("waiting instance %q: step at currentStep (%s) is %s, expected waiting", inst.ID, cur.Name, cur.Status)
		}
		if cur.Type != "human.approval" {
			t.Errorf("waiting instance %q: waiting on non-approval step %s (%s)", inst.ID, cur.Name, cur.Type)
		}
	}

	// DATA-05: failed instances have error set and a failed step at currentStep
	for _, inst := range insts {
		if inst.Status != models.InstFailed {
			continue
		}
		if inst.Error == "" {
			t.Errorf("failed instance %q has no error", inst.ID)
		}
		if inst.CurrentStep >= len(inst.StepRuns) {
			t.Errorf("failed instance %q currentStep out of range", inst.ID)
			continue
		}
		cur := inst.StepRuns[inst.CurrentStep]
		if cur.Status != models.StepFailed {
			t.Errorf("failed instance %q: step at currentStep (%s) is %s, expected failed", inst.ID, cur.Name, cur.Status)
		}
	}

	// DATA-06: completed instances have no pending/running/failed steps
	for _, inst := range insts {
		if inst.Status != models.InstCompleted {
			continue
		}
		for _, sr := range inst.StepRuns {
			if sr.Status == models.StepPending || sr.Status == models.StepRunning || sr.Status == models.StepFailed {
				t.Errorf("completed instance %q has non-terminal step %s (%s)", inst.ID, sr.Name, sr.Status)
			}
		}
		if inst.EndedAt == "" {
			t.Errorf("completed instance %q has no endedAt", inst.ID)
		}
	}

	// DATA-07: each workflow has at least 1 completed and 1 non-completed instance
	for _, w := range wfs {
		hasCompleted, hasOther := false, false
		for _, inst := range insts {
			if inst.WorkflowID != w.ID {
				continue
			}
			if inst.Status == models.InstCompleted {
				hasCompleted = true
			} else {
				hasOther = true
			}
		}
		if !hasCompleted {
			t.Errorf("workflow %q has no completed instances", w.ID)
		}
		if !hasOther {
			t.Errorf("workflow %q has no non-completed instances", w.ID)
		}
	}

	// DATA-08: audit entries have valid kinds and non-empty fields
	validKinds := map[string]bool{"ai": true, "approval": true, "deploy": true, "execution": true, "mdm": true, "export": true}
	for _, a := range audit {
		if a.ID == "" || a.Actor == "" || a.Action == "" || a.Detail == "" {
			t.Errorf("audit entry %q has empty required field", a.ID)
		}
		if !validKinds[a.Kind] {
			t.Errorf("audit entry %q has invalid kind %q", a.ID, a.Kind)
		}
	}

	// Summary
	t.Logf("seed: %d workflows, %d instances, %d audit, %d mdm entities", len(wfs), len(insts), len(audit), len(mdm))
}

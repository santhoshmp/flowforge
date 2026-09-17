// Scheduled triggers: deployed workflows whose trigger step carries a
// `schedule` cron param are evaluated on every scheduler pass. Each workflow
// fires at most once per matching minute slot; last-fired slots persist in
// settings so restarts neither double-fire nor replay long outages (a slot
// missed by more than catchUp is skipped).
package engine

import (
	"time"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/schedule"
	"github.com/flowforge/flowforge/internal/store"
	"github.com/flowforge/flowforge/internal/util"
)

// catchUp bounds how late a missed slot may still fire after a restart.
const catchUp = 5 * time.Minute

// TickScheduler starts executions for due scheduled workflows and returns
// the instance IDs it started.
func TickScheduler(s *store.Store, now time.Time) ([]string, error) {
	wfs, err := s.ListWorkflows()
	if err != nil {
		return nil, err
	}
	var started []string
	for _, wf := range wfs {
		if wf.Status != models.StatusDeployed || len(wf.Steps) == 0 {
			continue
		}
		trig := wf.Steps[0]
		if trig.Type != "trigger" {
			continue
		}
		expr := trig.Params["schedule"]
		if expr == "" {
			continue
		}
		cron, err := schedule.Parse(expr)
		if err != nil {
			continue // invalid schedules are rejected at parse/approve time
		}

		key := "sched:" + wf.ID
		lastFired := ""
		if v, ok, _ := s.GetSetting(key); ok {
			lastFired = v
		}
		due := mostRecentSlot(cron, now)
		if due.IsZero() {
			continue
		}
		dueKey := dueKeyOf(due)
		if lastFired >= dueKey {
			continue // already fired this slot
		}
		missed := now.Sub(due)
		_ = s.SetSetting(key, dueKey)
		if missed > catchUp {
			continue // server was down too long: skip the stale slot
		}

		runs := make([]models.StepRun, len(wf.Steps))
		for i, st := range wf.Steps {
			runs[i] = models.StepRun{StepID: st.ID, Name: st.Name, Type: st.Type, Status: models.StepPending}
		}
		entity := "schedule · " + wf.Name + " · " + dueKey
		inst := models.Instance{
			ID: "run-" + util.UID(), WorkflowID: wf.ID, WorkflowName: wf.Name,
			Status: models.InstRunning, Entity: entity,
			StartedAt: now.UTC().Format(time.RFC3339), CurrentStep: 0, StepRuns: runs,
		}
		if err := s.UpsertInstance(inst); err != nil {
			return started, err
		}
		wf.Runs++
		_ = s.UpsertWorkflow(wf)
		_ = s.AddAudit(auditSched(wf.Name, inst.ID))
		started = append(started, inst.ID)
	}
	return started, nil
}

// mostRecentSlot returns the latest minute <= now matching the cron, or the
// zero time when no slot occurred within the lookback window. Only slots
// inside the catch-up window can still be due, so ~2x catch-up is complete.
func mostRecentSlot(cron *schedule.Cron, now time.Time) time.Time {
	t := now.Truncate(time.Minute)
	for i := 0; i <= int(catchUp.Minutes())*2; i++ {
		if cron.Match(t) {
			return t
		}
		t = t.Add(-time.Minute)
	}
	return time.Time{}
}

func dueKeyOf(t time.Time) string { return t.Format(time.RFC3339) }

func auditSched(wfName, instID string) models.AuditEntry {
	return models.AuditEntry{
		ID: "aud-" + util.UID(), At: nowUTC(), Actor: "scheduler",
		Action: "Execution started (schedule)", Detail: wfName + " · " + instID, Kind: "execution",
	}
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

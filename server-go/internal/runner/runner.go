// Package runner executes a portable flowforge/v1 artifact headlessly —
// no control plane, no UI: parse → validate → run on the durable engine
// (in-memory store), the same executor registry and policy gates as serve.
// Human tasks auto-approve (--auto-approve), prompt via the Interactive
// resolver, or stop the run in `waiting` state.
package runner

import (
	"errors"
	"fmt"
	"time"

	"github.com/santhoshmp/flowforge/internal/engine"
	"github.com/santhoshmp/flowforge/internal/models"
	"github.com/santhoshmp/flowforge/internal/policy"
	"github.com/santhoshmp/flowforge/internal/spec"
	"github.com/santhoshmp/flowforge/internal/store"
	"github.com/santhoshmp/flowforge/internal/util"
)

// Options shape a headless run.
type Options struct {
	Policy      *policy.Policy // nil = engine default (simulate-only real steps)
	Input       map[string]any
	Entity      string
	AutoApprove bool
	// Interactive resolves human tasks when AutoApprove is false; return
	// true to approve. nil (or returning false) leaves the run waiting.
	Interactive func(approver string) bool
	// Progress receives human-readable step events (nil = silent).
	Progress func(line string)
	MaxTicks int // safety bound; default 500
}

// ErrWaiting reports the run stopped at a human task (nothing failed).
var ErrWaiting = errors.New("run stopped at a human task")

// Run executes the artifact to a terminal state (completed/failed) or stops
// at a human task (returns the waiting instance + ErrWaiting).
func Run(s *store.Store, artifact string, opts Options) (*models.Instance, error) {
	sp, err := spec.ParseYAML(artifact)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact: %v", err)
	}
	if opts.MaxTicks <= 0 {
		opts.MaxTicks = 500
	}
	say := func(format string, a ...any) {
		if opts.Progress != nil {
			opts.Progress(fmt.Sprintf(format, a...))
		}
	}

	wf := spec.ToWorkflow(sp)
	wf.ID = "wf-run-local"
	wf.Status = models.StatusDeployed
	wf.CreatedBy = "runner"
	wf.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.UpsertWorkflow(wf); err != nil {
		return nil, err
	}

	entity := opts.Entity
	if entity == "" {
		entity = "runner · " + sp.Metadata.Name
	}
	runs := make([]models.StepRun, len(wf.Steps))
	for i, st := range wf.Steps {
		runs[i] = models.StepRun{StepID: st.ID, Name: st.Name, Type: st.Type, Status: models.StepPending}
	}
	inst := models.Instance{
		ID: "run-" + util.UID(), WorkflowID: wf.ID, WorkflowName: wf.Name,
		Status: models.InstRunning, Entity: entity,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Input:     opts.Input, CurrentStep: 0, StepRuns: runs,
	}
	if err := s.UpsertInstance(inst); err != nil {
		return nil, err
	}
	say("running %q (v%d) — %d steps", wf.Name, wf.Version, len(wf.Steps)-1)

	prev := map[string]string{}
	reportDiff := func(cur *models.Instance) {
		for _, r := range cur.StepRuns {
			if prev[r.StepID] != r.Status {
				prev[r.StepID] = r.Status
				switch r.Status {
				case models.StepSucceeded:
					out := ""
					if r.Output != "" {
						out = " — " + r.Output
					}
					say("  ok    %s%s", r.Name, out)
				case models.StepSkipped:
					say("  skip  %s", r.Name)
				case models.StepFailed:
					say("  FAIL  %s — %s", r.Name, r.Output)
				case models.StepWaiting:
					say("  wait  %s — waiting on %s", r.Name, cur.WaitingOn)
				}
			}
		}
	}

	for tick := 0; tick < opts.MaxTicks; tick++ {
		engine.TickAll(s, opts.Policy)
		cur, err := s.GetInstance(inst.ID)
		if err != nil || cur == nil {
			return nil, fmt.Errorf("instance vanished mid-run: %v", err)
		}
		reportDiff(cur)

		if cur.Status == models.InstWaiting {
			if opts.AutoApprove {
				say("  auto-approved %s", cur.WaitingOn)
				if _, err := engine.ApproveWaiting(s, cur.ID); err != nil {
					return nil, err
				}
				continue
			}
			if opts.Interactive != nil && opts.Interactive(cur.WaitingOn) {
				say("  approved %s", cur.WaitingOn)
				if _, err := engine.ApproveWaiting(s, cur.ID); err != nil {
					return nil, err
				}
				continue
			}
			return cur, ErrWaiting
		}
		if cur.Status == models.InstCompleted || cur.Status == models.InstFailed || cur.Status == models.InstCancelled {
			return cur, nil
		}
	}
	cur, _ := s.GetInstance(inst.ID)
	return cur, fmt.Errorf("run did not reach a terminal state within %d ticks", opts.MaxTicks)
}

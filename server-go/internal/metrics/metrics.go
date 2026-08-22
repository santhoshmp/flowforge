// Package metrics computes the tracking-dashboard aggregates from live
// instance data. Mirrors server/src/metrics.ts so the React UI is unchanged.
package metrics

import (
	"time"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/store"
)

const dayMs = 24 * 60 * 60 * 1000

type Fleet struct {
	Workflows         int      `json:"workflows"`
	Deployed          int      `json:"deployed"`
	TotalRuns         int      `json:"totalRuns"`
	Running           int      `json:"running"`
	Waiting           int      `json:"waiting"`
	Failed            int      `json:"failed"`
	Completed         int      `json:"completed"`
	Cancelled         int      `json:"cancelled"`
	SuccessRate       *float64 `json:"successRate"`
	AvgDurationMs     *int     `json:"avgDurationMs"`
	HumanTasksPending int      `json:"humanTasksPending"`
}

type DayBucket struct {
	Date      string `json:"date"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	Running   int    `json:"running"`
	Waiting   int    `json:"waiting"`
	Cancelled int    `json:"cancelled"`
	Total     int    `json:"total"`
}

type WorkflowMetric struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Runs          int      `json:"runs"`
	Completed     int      `json:"completed"`
	Failed        int      `json:"failed"`
	Running       int      `json:"running"`
	Waiting       int      `json:"waiting"`
	Cancelled     int      `json:"cancelled"`
	SuccessRate   *float64 `json:"successRate"`
	AvgDurationMs *int     `json:"avgDurationMs"`
	LastRunISO    *string  `json:"lastRunIso"`
}

type Metrics struct {
	Fleet     Fleet            `json:"fleet"`
	ByDay     []DayBucket      `json:"byDay"`
	StatusMix []map[string]any `json:"statusMix"`
	Workflows []WorkflowMetric `json:"workflows"`
}

func instanceDurationMs(i models.Instance) (int, bool) {
	if i.EndedAt != "" {
		s, e1 := time.Parse(time.RFC3339, i.StartedAt)
		en, e2 := time.Parse(time.RFC3339, i.EndedAt)
		if e1 == nil && e2 == nil {
			d := int(en.Sub(s).Milliseconds())
			if d >= 0 {
				return d, true
			}
		}
	}
	sum := 0
	for _, r := range i.StepRuns {
		sum += r.DurationMs
	}
	if sum > 0 {
		return sum, true
	}
	return 0, false
}

func rate(completed, failed int) *float64 {
	if completed+failed == 0 {
		return nil
	}
	v := float64(completed) / float64(completed+failed) * 100
	v = float64(int(v*10)) / 10
	return &v
}

// Compute builds the dashboard metrics from the store.
func Compute(s *store.Store) (Metrics, error) {
	insts, err := s.ListInstances()
	if err != nil {
		return Metrics{}, err
	}
	wfs, err := s.ListWorkflows()
	if err != nil {
		return Metrics{}, err
	}

	count := func(st string) int {
		n := 0
		for _, i := range insts {
			if i.Status == st {
				n++
			}
		}
		return n
	}
	completed := count(models.InstCompleted)
	failed := count(models.InstFailed)
	running := count(models.InstRunning)
	waiting := count(models.InstWaiting)
	cancelled := count(models.InstCancelled)

	var durations []int
	for _, i := range insts {
		if i.Status == models.InstCompleted {
			if d, ok := instanceDurationMs(i); ok {
				durations = append(durations, d)
			}
		}
	}
	avgDuration := func() *int {
		if len(durations) == 0 {
			return nil
		}
		sum := 0
		for _, d := range durations {
			sum += d
		}
		v := sum / len(durations)
		return &v
	}()

	// 14-day series
	byDay := make([]DayBucket, 14)
	today := time.Now().UTC()
	for i := 0; i < 14; i++ {
		dt := today.AddDate(0, 0, -(13 - i))
		byDay[i] = DayBucket{Date: dt.Format("2006-01-02")}
	}
	idx := map[string]int{}
	for i, b := range byDay {
		idx[b.Date] = i
	}
	for _, inst := range insts {
		if len(inst.StartedAt) < 10 {
			continue
		}
		i, ok := idx[inst.StartedAt[:10]]
		if !ok {
			continue
		}
		switch inst.Status {
		case models.InstCompleted:
			byDay[i].Completed++
		case models.InstFailed:
			byDay[i].Failed++
		case models.InstRunning:
			byDay[i].Running++
		case models.InstWaiting:
			byDay[i].Waiting++
		case models.InstCancelled:
			byDay[i].Cancelled++
		}
		byDay[i].Total++
	}

	statusMix := []map[string]any{}
	for _, st := range []string{"completed", "failed", "running", "waiting", "cancelled"} {
		statusMix = append(statusMix, map[string]any{"status": st, "count": count(st)})
	}

	var wfMetrics []WorkflowMetric
	for _, w := range wfs {
		var runs []models.Instance
		var lastRun string
		for _, i := range insts {
			if i.WorkflowID == w.ID {
				runs = append(runs, i)
				if i.StartedAt > lastRun {
					lastRun = i.StartedAt
				}
			}
		}
		wc, wf2, wr, ww, wcanc := 0, 0, 0, 0, 0
		for _, i := range runs {
			switch i.Status {
			case models.InstCompleted:
				wc++
			case models.InstFailed:
				wf2++
			case models.InstRunning:
				wr++
			case models.InstWaiting:
				ww++
			case models.InstCancelled:
				wcanc++
			}
		}
		var wdurations []int
		for _, i := range runs {
			if i.Status == models.InstCompleted {
				if d, ok := instanceDurationMs(i); ok {
					wdurations = append(wdurations, d)
				}
			}
		}
		var avg *int
		if len(wdurations) > 0 {
			sum := 0
			for _, d := range wdurations {
				sum += d
			}
			v := sum / len(wdurations)
			avg = &v
		}
		var lastISO *string
		if lastRun != "" {
			lastISO = &lastRun
		}
		wfMetrics = append(wfMetrics, WorkflowMetric{
			ID: w.ID, Name: w.Name, Status: w.Status, Runs: len(runs),
			Completed: wc, Failed: wf2, Running: wr, Waiting: ww, Cancelled: wcanc,
			SuccessRate: rate(wc, wf2), AvgDurationMs: avg, LastRunISO: lastISO,
		})
	}

	deployed := 0
	for _, w := range wfs {
		if w.Status == models.StatusDeployed {
			deployed++
		}
	}

	return Metrics{
		Fleet: Fleet{
			Workflows: len(wfs), Deployed: deployed, TotalRuns: len(insts),
			Running: running, Waiting: waiting, Failed: failed, Completed: completed, Cancelled: cancelled,
			SuccessRate: rate(completed, failed), AvgDurationMs: avgDuration, HumanTasksPending: waiting,
		},
		ByDay: byDay, StatusMix: statusMix, Workflows: wfMetrics,
	}, nil
}

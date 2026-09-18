// Dead-letter queue + Prometheus metrics: operational surfaces for the
// control plane. Failed instances are the DLQ (oldest first, with age);
// requeue re-drives them from the failed step via the engine's retry.
package api

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/santhoshmp/flowforge/internal/engine"
	"github.com/santhoshmp/flowforge/internal/metrics"
	"github.com/santhoshmp/flowforge/internal/models"
)

// Version is stamped by main at startup for the build_info metric.
var Version = "dev"

func (s *Server) registerOpsRoutes() {
	s.mux.HandleFunc("GET /api/v1/dlq", s.listDLQ)
	s.mux.HandleFunc("POST /api/v1/dlq/{id}/requeue", s.requeueDLQ)
	s.mux.HandleFunc("GET /metrics", s.prometheusMetrics)
}

// ---- DLQ ----------------------------------------------------------------------

type dlqItem struct {
	models.Instance
	AgeSeconds int `json:"ageSeconds"`
}

// listDLQ: GET /api/v1/dlq — failed instances, oldest first.
func (s *Server) listDLQ(w http.ResponseWriter, _ *http.Request) {
	insts, err := s.store.ListInstances()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	now := time.Now().UTC()
	out := []dlqItem{}
	for _, i := range insts {
		if i.Status != models.InstFailed {
			continue
		}
		age := 0
		if t, err := time.Parse(time.RFC3339, i.StartedAt); err == nil {
			age = int(now.Sub(t).Seconds())
			if age < 0 {
				age = 0
			}
		}
		out = append(out, dlqItem{Instance: i, AgeSeconds: age})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].AgeSeconds > out[b].AgeSeconds })
	writeJSON(w, 200, out)
}

// requeueDLQ: POST /api/v1/dlq/{id}/requeue — re-drive from the failed step.
func (s *Server) requeueDLQ(w http.ResponseWriter, r *http.Request) {
	inst, err := engine.RetryFailed(s.store, r.PathValue("id"))
	if err != nil || inst == nil {
		writeErr(w, 404, "failed execution not found")
		return
	}
	s.audit("You", "Requeued from DLQ", inst.ID+" — "+inst.WorkflowName, "execution")
	writeJSON(w, 200, inst)
}

// ---- Prometheus ----------------------------------------------------------------

// prometheusMetrics: GET /metrics — text exposition format (behind the
// normal auth gate; scrapers pass the bearer token).
func (s *Server) prometheusMetrics(w http.ResponseWriter, _ *http.Request) {
	m, err := metrics.Compute(s.store)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	writeProm(w, "# HELP flowforge_build_info Build information.", "# TYPE flowforge_build_info gauge",
		fmt.Sprintf("flowforge_build_info{version=%q} 1", Version))
	writeProm(w, "# HELP flowforge_workflows Workflows by status.", "# TYPE flowforge_workflows gauge",
		fmt.Sprintf("flowforge_workflows{status=%q} %d", "deployed", m.Fleet.Deployed),
		fmt.Sprintf("flowforge_workflows{status=%q} %d", "total", m.Fleet.Workflows))
	writeProm(w, "# HELP flowforge_instances_total Instances by status.", "# TYPE flowforge_instances_total counter",
		fmt.Sprintf("flowforge_instances_total{status=%q} %d", "completed", m.Fleet.Completed),
		fmt.Sprintf("flowforge_instances_total{status=%q} %d", "failed", m.Fleet.Failed),
		fmt.Sprintf("flowforge_instances_total{status=%q} %d", "running", m.Fleet.Running),
		fmt.Sprintf("flowforge_instances_total{status=%q} %d", "waiting", m.Fleet.Waiting),
		fmt.Sprintf("flowforge_instances_total{status=%q} %d", "cancelled", m.Fleet.Cancelled))
	writeProm(w, "# HELP flowforge_human_tasks_pending Human tasks awaiting approval.", "# TYPE flowforge_human_tasks_pending gauge",
		fmt.Sprintf("flowforge_human_tasks_pending %d", m.Fleet.HumanTasksPending))
	if m.Fleet.SuccessRate != nil {
		writeProm(w, "# HELP flowforge_success_rate_percent Fleet success rate (completed vs failed).", "# TYPE flowforge_success_rate_percent gauge",
			fmt.Sprintf("flowforge_success_rate_percent %v", *m.Fleet.SuccessRate))
	}
}

func writeProm(w http.ResponseWriter, samples ...string) {
	for _, s := range samples {
		_, _ = w.Write([]byte(s + "\n"))
	}
}

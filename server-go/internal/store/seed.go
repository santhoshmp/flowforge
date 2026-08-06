package store

import "github.com/flowforge/flowforge/internal/seed"

// SeedIfEmpty populates the database with the demo dataset on first run.
func (s *Store) SeedIfEmpty() error {
	have, err := s.ListWorkflows()
	if err != nil {
		return err
	}
	if len(have) > 0 {
		return nil // already seeded
	}
	workflows := seed.Workflows()
	instances := seed.Instances(workflows)

	counts := map[string]int{}
	for _, i := range instances {
		counts[i.WorkflowID]++
	}
	for _, w := range workflows {
		w.Runs = counts[w.ID]
		if err := s.UpsertWorkflow(w); err != nil {
			return err
		}
	}
	for _, i := range instances {
		if err := s.UpsertInstance(i); err != nil {
			return err
		}
	}
	for _, a := range seed.Audit(workflows, instances) {
		if err := s.AddAudit(a); err != nil {
			return err
		}
	}
	for _, e := range seed.MDM() {
		if err := s.UpsertMDM(e); err != nil {
			return err
		}
	}
	for _, c := range seed.Controls() {
		if err := s.UpsertControl(c); err != nil {
			return err
		}
	}
	return nil
}

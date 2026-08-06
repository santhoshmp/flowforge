package store

import "testing"

func TestSeed(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.SeedIfEmpty(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	wfs, _ := s.ListWorkflows()
	if len(wfs) != 6 {
		t.Fatalf("want 6 workflows, got %d", len(wfs))
	}
	ins, _ := s.ListInstances()
	if len(ins) < 30 {
		t.Fatalf("want >=30 instances, got %d", len(ins))
	}
	mdm, _ := s.ListMDM()
	if len(mdm) != 4 {
		t.Fatalf("want 4 mdm entities, got %d", len(mdm))
	}
	ctrl, _ := s.ListControls()
	if len(ctrl) != 12 {
		t.Fatalf("want 12 controls, got %d", len(ctrl))
	}

	// idempotent
	if err := s.SeedIfEmpty(); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if wfs2, _ := s.ListWorkflows(); len(wfs2) != 6 {
		t.Fatalf("seed not idempotent: %d", len(wfs2))
	}

	// instance + nested JSON round-trips
	got, err := s.GetInstance(ins[0].ID)
	if err != nil || got == nil || len(got.StepRuns) == 0 {
		t.Fatalf("instance not read back: %v %v", got, err)
	}

	// settings round-trip
	if err := s.SetSetting("ai", `{"provider":"openai"}`); err != nil {
		t.Fatal(err)
	}
	v, ok, _ := s.GetSetting("ai")
	if !ok || v != `{"provider":"openai"}` {
		t.Fatalf("setting not read back: %q %v", v, ok)
	}
}

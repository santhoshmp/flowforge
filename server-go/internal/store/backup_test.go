package store

// BAK-01..03: DB backup/restore (F-BACKUP).

import (
	"path/filepath"
	"testing"

	"github.com/santhoshmp/flowforge/internal/models"
)

func TestBAK01_BackupRoundtrip(t *testing.T) {
	dir := t.TempDir()
	src, err := Open(filepath.Join(dir, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if err := src.SeedIfEmpty(); err != nil {
		t.Fatal(err)
	}
	wfs, _ := src.ListWorkflows()
	insts, _ := src.ListInstances()

	backup := filepath.Join(dir, "backup.db")
	if err := src.Backup(backup); err != nil {
		t.Fatalf("backup: %v", err)
	}

	// The snapshot is a fully readable database with identical counts.
	snap, err := Open(backup)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer snap.Close()
	snapWfs, _ := snap.ListWorkflows()
	snapInsts, _ := snap.ListInstances()
	if len(snapWfs) != len(wfs) || len(snapInsts) != len(insts) {
		t.Fatalf("snapshot counts: workflows %d/%d instances %d/%d", len(snapWfs), len(wfs), len(snapInsts), len(insts))
	}

	// The live DB keeps working after the backup.
	if err := src.UpsertWorkflow(models.Workflow{ID: "wf-post-backup", Name: "After", Description: "d", Prompt: "p", Status: "draft", Version: 1, Steps: []models.WorkflowStep{}, CreatedBy: "t", AIModel: "t", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("live db after backup: %v", err)
	}
}

// BAK-02: backups overwrite cleanly and pass the integrity check.
func TestBAK02_BackupOverwrite(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(filepath.Join(dir, "live.db"))
	defer s.Close()
	_ = s.SeedIfEmpty()
	backup := filepath.Join(dir, "backup.db")
	if err := s.Backup(backup); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(backup); err != nil { // second run replaces the file
		t.Fatalf("second backup: %v", err)
	}
	if chk, err := QuickCheck(backup); err != nil || chk != "ok" {
		t.Fatalf("quick_check = %q err=%v", chk, err)
	}
}

// BAK-03: the snapshot is isolated — writes to it don't touch the live DB.
func TestBAK03_SnapshotIsolated(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(filepath.Join(dir, "live.db"))
	defer s.Close()
	_ = s.SeedIfEmpty()
	backup := filepath.Join(dir, "backup.db")
	if err := s.Backup(backup); err != nil {
		t.Fatal(err)
	}
	snap, _ := Open(backup)
	defer snap.Close()
	if err := snap.UpsertWorkflow(models.Workflow{ID: "wf-snap-only", Name: "Snap", Description: "d", Prompt: "p", Status: "draft", Version: 1, Steps: []models.WorkflowStep{}, CreatedBy: "t", AIModel: "t", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWorkflow("wf-snap-only"); got != nil {
		t.Fatal("snapshot write leaked into the live DB")
	}
}

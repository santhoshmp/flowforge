package e2e

// E2E-10: backup CLI on the built binary (F-BACKUP).

import (
	"strings"
	"testing"
)

func TestE2E10_BackupCLI(t *testing.T) {
	dir := t.TempDir()
	// Load demo data so the backup has real content, then snapshot it.
	if out, err := runIn(t, dir, "demo"); err != nil {
		t.Fatalf("demo: %v\n%s", err, out)
	}
	out, err := runIn(t, dir, "backup", "snap.db")
	if err != nil || !strings.Contains(out, "backup complete") || !strings.Contains(out, "integrity verified") {
		t.Fatalf("backup: %v\n%s", err, out)
	}

	// Restore requires --force over an existing DB (and refuses without it).
	_, err = runIn(t, dir, "restore", "snap.db")
	if err == nil {
		t.Fatal("restore over existing DB must refuse without --force")
	}
	out, err = runIn(t, dir, "restore", "snap.db", "--force")
	if err != nil || !strings.Contains(out, "restored") {
		t.Fatalf("restore --force: %v\n%s", err, out)
	}
}

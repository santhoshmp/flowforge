package secrets

// SEC-04: encrypted local secrets store. Feature F-EXT (P4.2).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSEC04_RoundtripEncryptedAtRest(t *testing.T) {
	dir := t.TempDir()
	v, err := Open(filepath.Join(dir, "v.secrets"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := v.Set("SLACK_WEBHOOK_URL", "https://hooks.slack.com/services/T000/B000/XXX"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Re-open from disk (fresh instance) and read back.
	v2, err := Open(filepath.Join(dir, "v.secrets"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, ok := v2.Get("SLACK_WEBHOOK_URL"); !ok || got != "https://hooks.slack.com/services/T000/B000/XXX" {
		t.Fatalf("get = %q ok=%v", got, ok)
	}
	// The plaintext value must not appear in the vault file.
	raw, _ := os.ReadFile(filepath.Join(dir, "v.secrets"))
	if strings.Contains(string(raw), "hooks.slack.com") {
		t.Error("secret value stored in plaintext")
	}
	// Names only — the API never exposes values.
	if names := v2.Names(); len(names) != 1 || names[0] != "SLACK_WEBHOOK_URL" {
		t.Errorf("names = %v", names)
	}
	if err := v2.Delete("SLACK_WEBHOOK_URL"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := v2.Get("SLACK_WEBHOOK_URL"); ok {
		t.Error("deleted secret still present")
	}
	if err := v2.Delete("SLACK_WEBHOOK_URL"); err == nil {
		t.Error("deleting a missing secret should error")
	}
}

func TestSEC04_ExplicitKeyFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v.secrets")
	t.Setenv("FLOWFORGE_SECRETS_KEY", "MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=") // 32 bytes
	os.Unsetenv("FLOWFORGE_SECRETS_FILE")
	v, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := v.Set("SMTP_PASS", "hunter2"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// A different (but valid 32-byte) key must fail to decrypt (AES-GCM auth).
	t.Setenv("FLOWFORGE_SECRETS_KEY", "MTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTE=")
	if _, err := Open(path); err == nil {
		t.Error("decrypting with the wrong key should fail")
	}
}

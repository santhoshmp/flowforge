package signing

// SIGN-01..03: flowforge/v1 artifact signing (F-DSL-03).

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// SIGN-01: keygen → sign → verify roundtrip via files.
func TestSIGN01_KeygenSignVerifyRoundtrip(t *testing.T) {
	dir := t.TempDir()
	privPath, pubPath, err := GenerateKeyFiles(dir)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	artifact := writeTemp(t, dir, "flow.flow.yaml", "apiVersion: flowforge/v1\nkind: Workflow\n")

	sigPath, err := SignFile(artifact, mustPriv(t, privPath))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if sigPath != artifact+".sig" {
		t.Errorf("sigPath = %s", sigPath)
	}
	pub := mustPub(t, pubPath)
	if err := VerifyFile(artifact, pub); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// SIGN-02: any modification of the artifact breaks verification.
func TestSIGN02_TamperDetection(t *testing.T) {
	dir := t.TempDir()
	privPath, pubPath, _ := GenerateKeyFiles(dir)
	artifact := writeTemp(t, dir, "flow.flow.yaml", "apiVersion: flowforge/v1\n")
	if _, err := SignFile(artifact, mustPriv(t, privPath)); err != nil {
		t.Fatal(err)
	}
	// Tamper: append a byte.
	if err := os.WriteFile(artifact, []byte("apiVersion: flowforge/v1\n "), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(artifact, mustPub(t, pubPath)); err == nil {
		t.Fatal("tampered artifact verified — must fail")
	}
	// Missing signature is an explicit error.
	if err := VerifyFile(writeTemp(t, dir, "other.flow.yaml", "x"), mustPub(t, pubPath)); err == nil {
		t.Fatal("missing signature must error")
	}
}

// SIGN-03: a signature from a different key does not verify.
func TestSIGN03_WrongKeyRejected(t *testing.T) {
	dir := t.TempDir()
	_, pubPathA, _ := GenerateKeyFiles(dir)
	privB, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	artifact := writeTemp(t, dir, "flow.flow.yaml", "apiVersion: flowforge/v1\n")
	if _, err := SignFile(artifact, privB); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(artifact, mustPub(t, pubPathA)); err == nil {
		t.Fatal("signature from another key verified — must fail")
	}
}

func mustPriv(t *testing.T, path string) ed25519.PrivateKey {
	t.Helper()
	k, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func mustPub(t *testing.T, path string) ed25519.PublicKey {
	t.Helper()
	k, err := LoadPublicKey(path)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

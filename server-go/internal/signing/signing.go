// Package signing implements flowforge/v1 artifact signing (F-DSL-03):
// detached Ed25519 signatures over the exact artifact bytes, distributed as a
// sibling `<file>.sig` (single-line base64). Keypairs are Ed25519; key files
// are base64 with `#` comment lines. Release binaries are signed separately
// with cosign (see docs/release.md); this package covers portable .flow.yaml
// artifacts so a runner can verify provenance offline (docs/decisions.md D4).
package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	privHeader = "# flowforge ed25519 private key — keep secret"
	pubHeader  = "# flowforge ed25519 public key — safe to share"
)

// GenerateKeyPair creates a new Ed25519 keypair.
func GenerateKeyPair() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}

// SavePrivateKey writes a private key (base64 + header comment), 0600.
func SavePrivateKey(path string, priv ed25519.PrivateKey) error {
	body := base64.StdEncoding.EncodeToString(priv)
	return os.WriteFile(path, []byte(privHeader+"\n"+body+"\n"), 0o600)
}

// SavePublicKey writes a public key (base64 + header comment).
func SavePublicKey(path string, pub ed25519.PublicKey) error {
	body := base64.StdEncoding.EncodeToString(pub)
	return os.WriteFile(path, []byte(pubHeader+"\n"+body+"\n"), 0o644)
}

// LoadPrivateKey reads a private key file (comment lines ignored).
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	raw, err := readKeyFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%s: want %d-byte private key, got %d", path, ed25519.PrivateKeySize, len(raw))
	}
	return ed25519.PrivateKey(raw), nil
}

// LoadPublicKey reads a public key file (comment lines ignored).
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := readKeyFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%s: want %d-byte public key, got %d", path, ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

func readKeyFile(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		payload = line
		break
	}
	if payload == "" {
		return nil, fmt.Errorf("%s: no key material found", path)
	}
	return base64.StdEncoding.DecodeString(payload)
}

// Sign returns a detached signature over data.
func Sign(priv ed25519.PrivateKey, data []byte) []byte {
	return ed25519.Sign(priv, data)
}

// Verify checks a detached signature.
func Verify(pub ed25519.PublicKey, data, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, data, sig)
}

// SigPath is the sibling signature path for an artifact.
func SigPath(file string) string { return file + ".sig" }

// SignFile signs the exact bytes of file and writes SigPath(file) (base64).
// Returns the signature path.
func SignFile(file string, priv ed25519.PrivateKey) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	sig := Sign(priv, data)
	if err := os.WriteFile(SigPath(file), []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		return "", err
	}
	return SigPath(file), nil
}

// VerifyFile checks file against SigPath(file) using the public key.
// A missing signature is an error naming the expected path (verification is
// explicit, never silent).
func VerifyFile(file string, pub ed25519.PublicKey) error {
	sigPath := SigPath(file)
	sigB64, err := os.ReadFile(sigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no signature found at %s", sigPath)
		}
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil {
		return fmt.Errorf("%s: invalid signature encoding: %v", sigPath, err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if !Verify(pub, data, sig) {
		return errors.New("signature verification FAILED — artifact modified or signed by another key")
	}
	return nil
}

// GenerateKeyFiles writes a keypair (flowforge.key + flowforge.key.pub) into
// dir and returns the two paths. Refuses to overwrite existing keys.
func GenerateKeyFiles(dir string) (privPath, pubPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	privPath = filepath.Join(dir, "flowforge.key")
	pubPath = filepath.Join(dir, "flowforge.key.pub")
	if _, err := os.Stat(privPath); err == nil {
		return "", "", fmt.Errorf("%s already exists — move it or pick another dir", privPath)
	}
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		return "", "", err
	}
	if err := SavePrivateKey(privPath, priv); err != nil {
		return "", "", err
	}
	if err := SavePublicKey(pubPath, pub); err != nil {
		return "", "", err
	}
	return privPath, pubPath, nil
}

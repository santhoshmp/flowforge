// Package secrets is the encrypted local secret store (P4.2). Values are
// persisted with AES-256-GCM under the vault path; the key comes from
// FLOWFORGE_SECRETS_KEY (base64, 32 bytes) or is generated once and kept in a
// key file next to the vault. Secret values are never logged and never
// returned by the API — only names.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Vault is an encrypted name→value store.
type Vault struct {
	mu      sync.Mutex
	path    string
	keyPath string
	key     []byte
	data    map[string]string
}

// DefaultPath resolves the vault location: FLOWFORGE_SECRETS_FILE, else
// "flowforge.secrets" in the working directory.
func DefaultPath() string {
	if p := os.Getenv("FLOWFORGE_SECRETS_FILE"); p != "" {
		return p
	}
	return "flowforge.secrets"
}

var (
	defaultOnce  sync.Once
	defaultVault *Vault
	defaultErr   error
)

// Default opens the process-wide vault (lazily, on first use).
func Default() (*Vault, error) {
	defaultOnce.Do(func() {
		defaultVault, defaultErr = Open(DefaultPath())
	})
	return defaultVault, defaultErr
}

// Reset re-opens the process-wide vault. Intended for tests that change
// FLOWFORGE_SECRETS_FILE between runs; not safe for concurrent use.
func Reset() {
	defaultOnce = sync.Once{}
	defaultVault, defaultErr = nil, nil
}

// Open loads the vault at path, creating it (and a key) on first use.
func Open(path string) (*Vault, error) {
	v := &Vault{
		path:    path,
		keyPath: path + ".key",
		data:    map[string]string{},
	}
	if err := v.loadKey(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return v, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return v, nil
	}
	plain, err := decrypt(v.key, raw)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(plain, &v.data); err != nil {
		return nil, err
	}
	return v, nil
}

func (v *Vault) loadKey() error {
	if k := os.Getenv("FLOWFORGE_SECRETS_KEY"); k != "" {
		key, err := base64.StdEncoding.DecodeString(k)
		if err != nil {
			return errors.New("FLOWFORGE_SECRETS_KEY must be base64")
		}
		if len(key) != 32 {
			return errors.New("FLOWFORGE_SECRETS_KEY must decode to 32 bytes")
		}
		v.key = key
		return nil
	}
	if raw, err := os.ReadFile(v.keyPath); err == nil {
		key, err := base64.StdEncoding.DecodeString(string(raw))
		if err != nil || len(key) != 32 {
			return errors.New("secrets key file is corrupt; set FLOWFORGE_SECRETS_KEY or remove " + v.keyPath)
		}
		v.key = key
		return nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	v.key = key
	_ = os.WriteFile(v.keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0o600)
	return nil
}

// persist encrypts and writes the vault atomically.
func (v *Vault) persist() error {
	plain, err := json.Marshal(v.data)
	if err != nil {
		return err
	}
	sealed, err := encrypt(v.key, plain)
	if err != nil {
		return err
	}
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}

// Set stores a secret (encrypted at rest).
func (v *Vault) Set(name, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data[name] = value
	return v.persist()
}

// Get resolves a secret reference value (used only by executors; never logged).
func (v *Vault) Get(name string) (string, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	s, ok := v.data[name]
	return s, ok
}

// Delete removes a secret.
func (v *Vault) Delete(name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.data[name]; !ok {
		return os.ErrNotExist
	}
	delete(v.data, name)
	return v.persist()
}

// Names lists secret names (values are never exposed).
func (v *Vault) Names() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]string, 0, len(v.data))
	for k := range v.data {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- AES-256-GCM ------------------------------------------------------------

func encrypt(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func decrypt(key, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("secrets vault is corrupt")
	}
	nonce, body := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	return gcm.Open(nil, nonce, body, nil)
}

// KeyFileFor reports the sibling key path for a vault path (helper for tests).
func KeyFileFor(path string) string { return filepath.Join(path + ".key") }

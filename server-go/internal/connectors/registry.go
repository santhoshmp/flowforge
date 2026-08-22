// Registry: loads connector manifests from the embedded built-ins and user
// drop-in directories (FLOWFORGE_CONNECTOR_DIR). Later sources override the
// same id (see docs/decisions.md D3).
package connectors

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed builtin
var builtinFS embed.FS

// Entry is a loaded connector: its manifest plus where it came from.
type Entry struct {
	Manifest *Manifest
	Dir      string // user dir path, or "" for embedded built-ins
	Builtin  bool
	fsys     fs.FS // embedded FS for built-ins
	fsDir    string
}

// Registry resolves connector ids to loaded entries.
type Registry struct {
	mu    sync.RWMutex
	byID  map[string]*Entry
	order []string
}

// NewRegistry loads the embedded built-ins and, when set, overlays the user
// connector dir (each subdirectory must contain connector.yaml).
func NewRegistry(userDir string) (*Registry, error) {
	r := &Registry{byID: map[string]*Entry{}}
	if err := r.loadEmbed(); err != nil {
		return nil, err
	}
	if userDir != "" {
		if err := r.loadDir(userDir); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// DefaultUserDir resolves the drop-in location: FLOWFORGE_CONNECTOR_DIR.
func DefaultUserDir() string { return os.Getenv("FLOWFORGE_CONNECTOR_DIR") }

var (
	defOnce   sync.Once
	defReg    *Registry
	defRegErr error
)

// Default opens the process-wide registry (embedded + FLOWFORGE_CONNECTOR_DIR).
func Default() (*Registry, error) {
	defOnce.Do(func() {
		defReg, defRegErr = NewRegistry(DefaultUserDir())
	})
	return defReg, defRegErr
}

func (r *Registry) loadEmbed() error {
	return loadFS(r, builtinFS, "builtin", true)
}

func loadFS(r *Registry, fsys fs.FS, root string, builtin bool) error {
	dirs, err := fs.ReadDir(fsys, root)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		path := root + "/" + d.Name() + "/connector.yaml"
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("%s: %v", path, err)
		}
		e, err := parseEntry(raw, fsys, root+"/"+d.Name(), "", builtin)
		if err != nil {
			return fmt.Errorf("connector %s: %v", d.Name(), err)
		}
		r.put(e)
	}
	return nil
}

func (r *Registry) loadDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("connector dir: %v", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("connector dir %s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(filepath.Join(sub, "connector.yaml"))
		if err != nil {
			return fmt.Errorf("%s: %v", sub, err)
		}
		entry, err := parseEntryOS(raw, sub)
		if err != nil {
			return fmt.Errorf("connector %s: %v", e.Name(), err)
		}
		r.put(entry)
	}
	return nil
}

func parseEntry(raw []byte, fsys fs.FS, fsDir, osDir string, builtin bool) (*Entry, error) {
	m := &Manifest{}
	if err := yaml.Unmarshal(raw, m); err != nil {
		return nil, fmt.Errorf("invalid connector.yaml: %v", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if m.ParamsSchema != "" {
		var raw []byte
		var err error
		if fsys != nil {
			raw, err = fs.ReadFile(fsys, fsDir+"/"+m.ParamsSchema)
		} else {
			raw, err = os.ReadFile(filepath.Join(osDir, m.ParamsSchema))
		}
		if err != nil {
			return nil, fmt.Errorf("paramsSchema %s: %v", m.ParamsSchema, err)
		}
		var probe map[string]any
		if err := json.Unmarshal(raw, &probe); err != nil {
			return nil, fmt.Errorf("paramsSchema %s is not valid JSON: %v", m.ParamsSchema, err)
		}
		if probe["type"] != "object" {
			return nil, fmt.Errorf("paramsSchema %s must have type object", m.ParamsSchema)
		}
		m.Params = json.RawMessage(raw)
	}
	return &Entry{Manifest: m, Dir: osDir, Builtin: builtin, fsys: fsys, fsDir: fsDir}, nil
}

func parseEntryOS(raw []byte, dir string) (*Entry, error) {
	return parseEntry(raw, nil, "", dir, false)
}

// ParseDir loads a single connector directory (CLI validate/test + tests).
func ParseDir(dir string) (*Entry, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "connector.yaml"))
	if err != nil {
		return nil, err
	}
	return parseEntryOS(raw, dir)
}

func (r *Registry) put(e *Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = map[string]*Entry{}
	}
	if _, exists := r.byID[e.Manifest.ID]; !exists {
		r.order = append(r.order, e.Manifest.ID)
	}
	r.byID[e.Manifest.ID] = e
}

// Get resolves a connector id.
func (r *Registry) Get(id string) *Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byID[id]
}

// IDs lists connector ids in load order.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.order...)
}

// List returns all entries sorted by id.
func (r *Registry) List() []*Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Entry, 0, len(r.byID))
	for _, id := range r.order {
		out = append(out, r.byID[id])
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Manifest.Name) < strings.ToLower(out[j].Manifest.Name)
	})
	return out
}

// Package executor runs the untrusted execution surfaces in a controlled way.
// It is a registry of StepExecutor implementations: built-ins (script sandbox,
// egress-gated HTTP) self-register at startup, and the connectors package
// registers the `connector` step executor (HTTP/SMTP/WASM). Execution is
// opt-in: a step runs for real only when its executor reports it Configured
// (e.g. `code` for script, `url` for integration); otherwise the engine
// simulates it (so demo workflows are unaffected).
package executor

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.starlark.net/starlark"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
)

// ErrNotConfigured signals the step is not configured for real execution and
// the engine should fall back to simulation.
var ErrNotConfigured = errors.New("step not configured for real execution")

// ErrNoExecutor signals that no registered executor handles this step type
// (the engine simulates those steps).
var ErrNoExecutor = errors.New("no executor registered for step type")

const httpTimeout = 5 * time.Second

// StepExecutor is a pluggable executor for a class of step types.
type StepExecutor interface {
	// Name identifies the executor (for logs and the registry listing).
	Name() string
	// Matches reports whether this executor handles the given step type.
	Matches(stepType string) bool
	// Configured reports whether the step carries real-execution config.
	Configured(step *models.WorkflowStep) bool
	// Run executes the step for real. A returned error fails the instance.
	Run(step *models.WorkflowStep, input map[string]any, pol *policy.Policy) (string, error)
}

var (
	mu        sync.RWMutex
	executors []StepExecutor
)

// Register adds an executor to the registry. Later registrations win: an
// executor registered again for the same types overrides earlier ones
// (this is how `connector` extends execution without touching core).
func Register(e StepExecutor) {
	mu.Lock()
	defer mu.Unlock()
	executors = append([]StepExecutor{e}, executors...)
}

// Registered lists registered executors (most recent first).
func Registered() []StepExecutor {
	mu.RLock()
	defer mu.RUnlock()
	return append([]StepExecutor(nil), executors...)
}

// ForType returns the executor handling the step type, or nil.
func ForType(stepType string) StepExecutor {
	mu.RLock()
	defer mu.RUnlock()
	for _, e := range executors {
		if e.Matches(stepType) {
			return e
		}
	}
	return nil
}

// Run executes a step for real when an executor handles it and the step is
// configured for real execution. Returns (output, err, real):
// real=false with ErrNoExecutor or ErrNotConfigured means "simulate";
// real=true with err!=nil means the step attempted real execution and failed
// (e.g., blocked by policy).
func Run(step *models.WorkflowStep, input map[string]any, pol *policy.Policy) (string, error, bool) {
	e := ForType(step.Type)
	if e == nil {
		return "", ErrNoExecutor, false
	}
	if !e.Configured(step) {
		return "", ErrNotConfigured, false
	}
	out, err := e.Run(step, input, pol)
	return out, err, true
}

// ---- Script (Starlark sandbox) ---------------------------------------------

type scriptExecutor struct{}

func init() {
	Register(scriptExecutor{})
	Register(httpExecutor{})
}

func (scriptExecutor) Name() string          { return "script" }
func (scriptExecutor) Matches(t string) bool { return t == "script" }
func (scriptExecutor) Configured(step *models.WorkflowStep) bool {
	return strings.TrimSpace(step.Params["code"]) != ""
}

func (scriptExecutor) Run(step *models.WorkflowStep, input map[string]any, pol *policy.Policy) (string, error) {
	if pol != nil && pol.SafeMode {
		return "", errors.New("blocked: safe-mode disables script steps")
	}
	return runScript(step.Params["code"], input)
}

func runScript(code string, input map[string]any) (string, error) {
	thread := &starlark.Thread{Load: noLoad} // no `load()`; no built-in I/O exists
	predeclared := starlark.StringDict{"input": toStarlark(input)}
	globals, err := starlark.ExecFile(thread, "step.star", code, predeclared)
	if err != nil {
		return "", fmt.Errorf("script error: %v", err)
	}
	result, ok := globals["result"]
	if !ok {
		return "", errors.New("script must define a top-level `result` value")
	}
	return result.String(), nil
}

// noLoad forbids `load(...)` so scripts cannot pull in other modules.
func noLoad(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	return nil, fmt.Errorf("load(%q) disabled by sandbox", module)
}

func toStarlark(v any) starlark.Value {
	switch x := v.(type) {
	case nil:
		return starlark.None
	case bool:
		return starlark.Bool(x)
	case string:
		return starlark.String(x)
	case float64:
		return starlark.Float(x)
	case int:
		return starlark.MakeInt(x)
	case int64:
		return starlark.MakeInt64(x)
	case []any:
		elems := make([]starlark.Value, len(x))
		for i, e := range x {
			elems[i] = toStarlark(e)
		}
		return starlark.NewList(elems)
	case map[string]any:
		d := starlark.NewDict(len(x))
		for k, vv := range x {
			_ = d.SetKey(starlark.String(k), toStarlark(vv))
		}
		return d
	}
	return starlark.String(fmt.Sprintf("%v", v))
}

// ---- Integration (real HTTP, egress-gated) ---------------------------------

type httpExecutor struct{}

func (httpExecutor) Name() string { return "integration.http" }
func (httpExecutor) Matches(t string) bool {
	return strings.HasPrefix(t, "integration.")
}
func (httpExecutor) Configured(step *models.WorkflowStep) bool {
	return strings.TrimSpace(step.Params["url"]) != ""
}

func (httpExecutor) Run(step *models.WorkflowStep, input map[string]any, pol *policy.Policy) (string, error) {
	if pol != nil && pol.SafeMode {
		return "", errors.New("blocked: safe-mode disables integration steps")
	}
	url := strings.TrimSpace(step.Params["url"])
	if pol != nil && !pol.EgressAllowed(url) {
		return "", fmt.Errorf("blocked by egress policy: %s", url)
	}
	method := strings.ToUpper(step.Params["method"])
	if method == "" {
		if step.Type == "integration.post" {
			method = "POST"
		} else {
			method = "GET"
		}
	}
	var body io.Reader
	if b := step.Params["body"]; b != "" {
		body = strings.NewReader(b)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return "", err
	}
	if step.Params["body"] != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Sprintf("%s %s -> %d (%d bytes)", method, url, resp.StatusCode, len(respBody)), nil
}

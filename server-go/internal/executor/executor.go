// Package executor runs the two untrusted execution surfaces in a controlled
// way: `script` steps run in a Starlark sandbox (pure-Go, no filesystem or
// network by design — the host provides only the step input), and `integration`
// steps perform real HTTP calls gated by the request policy (safe-mode +
// egress allow-list). Execution is opt-in: a step runs for real only when it
// carries `code` (script) or `url` (integration); otherwise the engine
// simulates it (so demo workflows are unaffected).
package executor

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.starlark.net/starlark"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
)

// ErrNotConfigured signals the step is not configured for real execution and
// the engine should fall back to simulation.
var ErrNotConfigured = errors.New("step not configured for real execution")

const httpTimeout = 5 * time.Second

// Run executes a step for real when configured. Returns (output, err, real):
// real=false means "not configured" (simulate); real=true with err!=nil means
// the step attempted real execution and failed (e.g., blocked by policy).
func Run(step *models.WorkflowStep, input map[string]any, pol *policy.Policy) (string, error, bool) {
	switch {
	case step.Type == "script" && strings.TrimSpace(step.Params["code"]) != "":
		return runScript(step.Params["code"], input, pol)
	case strings.HasPrefix(step.Type, "integration.") && strings.TrimSpace(step.Params["url"]) != "":
		return runHTTP(step, input, pol)
	}
	return "", ErrNotConfigured, false
}

// ---- Script (Starlark sandbox) ---------------------------------------------

func runScript(code string, input map[string]any, pol *policy.Policy) (string, error, bool) {
	if pol != nil && pol.SafeMode {
		return "", errors.New("blocked: safe-mode disables script steps"), true
	}
	thread := &starlark.Thread{Load: noLoad} // no `load()`; no built-in I/O exists
	predeclared := starlark.StringDict{"input": toStarlark(input)}
	globals, err := starlark.ExecFile(thread, "step.star", code, predeclared)
	if err != nil {
		return "", fmt.Errorf("script error: %v", err), true
	}
	result, ok := globals["result"]
	if !ok {
		return "", errors.New("script must define a top-level `result` value"), true
	}
	return result.String(), nil, true
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

func runHTTP(step *models.WorkflowStep, input map[string]any, pol *policy.Policy) (string, error, bool) {
	if pol != nil && pol.SafeMode {
		return "", errors.New("blocked: safe-mode disables integration steps"), true
	}
	url := strings.TrimSpace(step.Params["url"])
	if pol != nil && !pol.EgressAllowed(url) {
		return "", fmt.Errorf("blocked by egress policy: %s", url), true
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
		return "", err, true
	}
	if step.Params["body"] != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err, true
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Sprintf("%s %s -> %d (%d bytes)", method, url, resp.StatusCode, len(respBody)), nil, true
}

// The `connector` step executor: resolves ${params.*}/${input.*}/${secret.*}/${env.*}
// templates, validates params against the connector manifest, and dispatches
// to the http / smtp / wasm executors. Registered with the executor registry
// (EXT) so `type: connector` steps run for real, gated by the policy module.
package connectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flowforge/flowforge/internal/executor"
	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/secrets"
	"github.com/flowforge/flowforge/internal/wasm"
)

func init() { executor.Register(connectorStepExecutor{}) }

type connectorStepExecutor struct{}

func (connectorStepExecutor) Name() string          { return "connector" }
func (connectorStepExecutor) Matches(t string) bool { return t == "connector" }

// Configured: a connector step is always real — it names a registry entry.
// Unknown connectors fail loudly at run time (approval gates catch it earlier).
func (connectorStepExecutor) Configured(step *models.WorkflowStep) bool { return true }

func (connectorStepExecutor) Run(step *models.WorkflowStep, input map[string]any, pol *policy.Policy) (string, error) {
	if pol != nil && pol.SafeMode {
		return "", errors.New("blocked: safe-mode disables connector steps")
	}
	reg, err := Default()
	if err != nil {
		return "", err
	}
	return RunStep(reg, step, input, pol)
}

// RunStep resolves and executes a connector step against a registry.
func RunStep(reg *Registry, step *models.WorkflowStep, input map[string]any, pol *policy.Policy) (string, error) {
	id := strings.TrimSpace(step.Params["connector"])
	if id == "" {
		return "", errors.New(`connector step is missing the "connector" param`)
	}
	entry := reg.Get(id)
	if entry == nil {
		known := strings.Join(reg.IDs(), ", ")
		return "", fmt.Errorf("unknown connector %q (installed: %s)", id, known)
	}
	m := entry.Manifest
	if err := m.ValidateParams(step.Params); err != nil {
		return "", fmt.Errorf("connector %s: %v", id, err)
	}
	ctx := newTemplateContext(m, step.Params, input)

	switch m.Executor {
	case KindHTTP:
		return runHTTPConnector(entry, ctx, pol)
	case KindSMTP:
		return runSMTPConnector(entry, ctx)
	case KindWASM:
		return runWASMConnector(entry, ctx, pol)
	default:
		return "", fmt.Errorf("connector %s: unsupported executor %q", id, m.Executor)
	}
}

// ---- templating -------------------------------------------------------------

var refRe = regexp.MustCompile(`\$\{(params|input|secret|env)(?:\.([A-Za-z0-9_.-]+))?\}`)

type templateContext struct {
	params map[string]string
	input  map[string]any
	// missing collects unresolved refs (secrets/env not set).
	missing []string
}

func newTemplateContext(m *Manifest, params map[string]string, input map[string]any) *templateContext {
	return &templateContext{params: params, input: input}
}

func (c *templateContext) noteMissing(ref string) {
	for _, m := range c.missing {
		if m == ref {
			return
		}
	}
	c.missing = append(c.missing, ref)
}

// resolve returns the template with all refs substituted. Secret values are
// resolved from the vault; env from the process. Unresolved refs become an
// error listing every missing name.
func (c *templateContext) resolve(tpl string) (string, error) {
	var firstErr error
	out := refRe.ReplaceAllStringFunc(tpl, func(ref string) string {
		m := refRe.FindStringSubmatch(ref)
		section, path := m[1], m[2]
		switch section {
		case "params":
			if v, ok := c.params[path]; ok {
				return v
			}
		case "input":
			if v, ok := lookupPath(c.input, path); ok {
				return fmt.Sprintf("%v", v)
			}
		case "secret":
			vault, err := secrets.Default()
			if err == nil {
				if v, ok := vault.Get(path); ok {
					return v
				}
			}
		case "env":
			if v, ok := lookupEnv(path); ok {
				return v
			}
		}
		c.noteMissing(ref)
		if firstErr == nil {
			firstErr = fmt.Errorf("unresolved reference %s", ref)
		}
		return ref
	})
	if firstErr != nil {
		return "", fmt.Errorf("%w — missing: %s", firstErr, strings.Join(c.missing, ", "))
	}
	return out, nil
}

func lookupEnv(name string) (string, bool) {
	v := os.Getenv(name)
	return v, v != ""
}

// lookupPath resolves a dot path ("a.b.c") against nested input maps.
func lookupPath(input map[string]any, path string) (any, bool) {
	if path == "" || input == nil {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var cur any = input
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// resolveOrDefault resolves a template, falling back to a default when refs
// are unresolved (used for optional fields like the HTTP method).
func (c *templateContext) resolveOrDefault(tpl, def string) string {
	if strings.TrimSpace(tpl) == "" {
		return def
	}
	out, err := c.resolve(tpl)
	if err != nil || strings.TrimSpace(out) == "" {
		return def
	}
	return out
}

// resolveOptional resolves a template where unresolved refs mean "absent"
// (e.g. an HTTP body whose ${params.body} was not supplied): they become an
// empty string instead of an error. Used only for inherently-optional fields.
func (c *templateContext) resolveOptional(tpl string) string {
	if strings.TrimSpace(tpl) == "" {
		return ""
	}
	return refRe.ReplaceAllStringFunc(tpl, func(ref string) string {
		m := refRe.FindStringSubmatch(ref)
		section, path := m[1], m[2]
		switch section {
		case "params":
			if v, ok := c.params[path]; ok {
				return v
			}
		case "input":
			if v, ok := lookupPath(c.input, path); ok {
				return fmt.Sprintf("%v", v)
			}
		case "secret":
			if vault, err := secrets.Default(); err == nil {
				if v, ok := vault.Get(path); ok {
					return v
				}
			}
		case "env":
			if v, ok := lookupEnv(path); ok {
				return v
			}
		}
		return ""
	})
}

// ---- http executor ----------------------------------------------------------

const connectorHTTPTimeout = 10 * time.Second

func runHTTPConnector(entry *Entry, c *templateContext, pol *policy.Policy) (string, error) {
	o := entry.Manifest.HTTP
	method := strings.ToUpper(c.resolveOrDefault(o.Method, "GET"))
	url, err := c.resolve(o.URL)
	if err != nil {
		return "", err
	}
	if pol != nil && !pol.EgressAllowed(url) {
		return "", fmt.Errorf("blocked by egress policy: %s", url)
	}
	// Body is optional: an unresolved/empty body template sends no body.
	body := c.resolveOptional(o.Body)
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	for k, tpl := range o.Headers {
		v, err := c.resolve(tpl)
		if err != nil {
			return "", err
		}
		req.Header.Set(k, v)
	}
	if err := applyAuth(entry.Manifest, req, c); err != nil {
		return "", err
	}
	client := &http.Client{Timeout: connectorHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("connector %s: %s %s -> %d (upstream error)", entry.Manifest.ID, method, redactURL(url), resp.StatusCode)
	}
	n := countBody(resp)
	return fmt.Sprintf("connector %s: %s %s -> %d (%d bytes)", entry.Manifest.ID, method, redactURL(url), resp.StatusCode, n), nil
}

func countBody(resp *http.Response) int {
	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return int(n)
}

// redactURL strips the query string (may carry tokens) for logs/outputs.
func redactURL(url string) string {
	if i := strings.IndexByte(url, '?'); i >= 0 {
		return url[:i] + "?…"
	}
	return url
}

func applyAuth(m *Manifest, req *http.Request, c *templateContext) error {
	switch m.Auth.Mode {
	case AuthNone, "":
		return nil
	case AuthBearer, AuthHeader, AuthBasic:
		token, err := c.resolve("${secret." + m.Auth.SecretKey + "}")
		if err != nil {
			return err
		}
		switch m.Auth.Mode {
		case AuthBearer:
			req.Header.Set("Authorization", "Bearer "+token)
		case AuthHeader:
			req.Header.Set(m.Auth.HeaderName, token)
		default:
			user, pass, _ := strings.Cut(token, ":")
			req.SetBasicAuth(user, pass)
		}
		return nil
	default:
		return fmt.Errorf("unknown auth mode %q", m.Auth.Mode)
	}
}

// ---- smtp executor ----------------------------------------------------------

func runSMTPConnector(entry *Entry, c *templateContext) (string, error) {
	o := entry.Manifest.SMTP
	host, err := c.resolve(o.Host)
	if err != nil {
		return "", err
	}
	from, err := c.resolve(o.From)
	if err != nil {
		return "", err
	}
	to, err := c.resolve(o.To)
	if err != nil {
		return "", err
	}
	subject, err := c.resolve(o.Subject)
	if err != nil {
		return "", err
	}
	body, err := c.resolve(o.Body)
	if err != nil {
		return "", err
	}
	addr := host + ":" + strconv.Itoa(o.Port)
	msg := []byte("To: " + to + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"\r\n" +
		body + "\r\n")
	var auth smtp.Auth
	if o.UserSecret != "" {
		user, err := c.resolve("${secret." + o.UserSecret + "}")
		if err != nil {
			return "", err
		}
		pass, err := c.resolve("${secret." + o.PassSecret + "}")
		if err != nil {
			return "", err
		}
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if err := smtp.SendMail(addr, auth, from, []string{to}, msg); err != nil {
		return "", err
	}
	return fmt.Sprintf("connector %s: mail to %s sent", entry.Manifest.ID, to), nil
}

// ---- wasm executor ----------------------------------------------------------

func runWASMConnector(entry *Entry, c *templateContext, pol *policy.Policy) (string, error) {
	module := readAsset(entry, entry.Manifest.WASM.Module)
	if len(module) == 0 {
		return "", fmt.Errorf("connector %s: wasm module %s not found", entry.Manifest.ID, entry.Manifest.WASM.Module)
	}
	inputJSON, err := marshalTemplateInput(c)
	if err != nil {
		return "", err
	}
	res, logs, err := wasm.Run(module, inputJSON, pol)
	if err != nil {
		return "", fmt.Errorf("connector %s (wasm): %v", entry.Manifest.ID, err)
	}
	out := "connector " + entry.Manifest.ID + " (wasm): " + res
	if len(logs) > 0 {
		out += " · logs: " + strings.Join(logs, " | ")
	}
	return out, nil
}

// readAsset reads a connector file (module, schema) from the embedded built-in
// FS or the user directory.
func readAsset(entry *Entry, name string) []byte {
	if entry == nil || name == "" {
		return nil
	}
	if entry.fsys != nil {
		raw, err := fs.ReadFile(entry.fsys, entry.fsDir+"/"+name)
		if err != nil {
			return nil
		}
		return raw
	}
	raw, err := os.ReadFile(filepath.Join(entry.Dir, name))
	if err != nil {
		return nil
	}
	return raw
}

// marshalTemplateInput builds the JSON document a wasm plugin receives:
// params + input only. Secrets are deliberately NOT passed into the sandbox —
// plugins reach the outside world exclusively via the egress-gated
// ff.http_request host function (see docs/decisions.md D2).
func marshalTemplateInput(c *templateContext) ([]byte, error) {
	return json.Marshal(map[string]any{"params": c.params, "input": c.input})
}

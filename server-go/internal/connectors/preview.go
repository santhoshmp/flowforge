// Preview resolves connector templates for the test endpoint WITHOUT
// executing anything: secret values are masked, unresolved refs are reported
// as warnings. No network is touched.
package connectors

import (
	"fmt"
	"strings"

	"github.com/flowforge/flowforge/internal/secrets"
)

// Preview returns a redacted request preview plus warnings for unresolved refs.
func Preview(entry *Entry, params map[string]string, input map[string]any) (map[string]any, []string, error) {
	m := entry.Manifest
	c := newTemplateContext(m, params, input)
	var warnings []string

	masked := func(tpl string) string {
		return refRe.ReplaceAllStringFunc(tpl, func(ref string) string {
			sub := refRe.FindStringSubmatch(ref)
			section, path := sub[1], sub[2]
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
					if _, ok := vault.Get(path); ok {
						return "***"
					}
				}
			case "env":
				if v, ok := lookupEnv(path); ok && v != "" {
					return v
				}
			}
			warnings = append(warnings, "unresolved "+ref)
			return ref
		})
	}

	out := map[string]any{"executor": m.Executor}
	switch m.Executor {
	case KindHTTP:
		headers := map[string]string{}
		for k, v := range m.HTTP.Headers {
			headers[k] = masked(v)
		}
		method := masked(m.HTTP.Method)
		if method == "" || strings.Contains(method, "${") {
			method = "GET (default)"
		} else {
			method = strings.ToUpper(method)
		}
		out["method"] = method
		out["url"] = masked(m.HTTP.URL)
		out["headers"] = headers
		if m.HTTP.Body != "" {
			out["body"] = masked(m.HTTP.Body)
		}
	case KindSMTP:
		out["host"] = masked(m.SMTP.Host)
		out["port"] = m.SMTP.Port
		out["from"] = masked(m.SMTP.From)
		out["to"] = masked(m.SMTP.To)
		out["subject"] = masked(m.SMTP.Subject)
	case KindWASM:
		out["module"] = m.WASM.Module
		out["note"] = "module executes during test (sandboxed, limits apply)"
	}
	return out, warnings, nil
}

// ModuleBytes reads the connector's wasm module (empty when absent).
func ModuleBytes(entry *Entry) []byte {
	if entry == nil || entry.Manifest.WASM == nil {
		return nil
	}
	return readAsset(entry, entry.Manifest.WASM.Module)
}

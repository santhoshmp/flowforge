// Package policy centralizes request-safety policy for the control plane: a
// safe-mode toggle (disables script + arbitrary-HTTP steps) and an egress
// allow-list that connector execution must consult before any outbound call.
//
// Real connector/step execution is simulated in the engine today; this package
// is the enforcement hook that real execution (P2/P3) will call. Configured via
// environment: FLOWFORGE_SAFE_MODE and FLOWFORGE_EGRESS_ALLOW.
package policy

import (
	"net/url"
	"strings"
)

// Policy is the resolved safety policy.
type Policy struct {
	SafeMode      bool     // disable script + arbitrary-HTTP steps
	Allow         []string // host suffixes; when non-empty, egress defaults to deny
	DenyByDefault bool
}

// FromEnv builds the policy from environment variables.
func FromEnv(getenv func(string) string) *Policy {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	p := &Policy{SafeMode: strings.EqualFold(getenv("FLOWFORGE_SAFE_MODE"), "on")}
	if raw := strings.TrimSpace(getenv("FLOWFORGE_EGRESS_ALLOW")); raw != "" {
		for _, h := range strings.Split(raw, ",") {
			if h = strings.TrimSpace(strings.ToLower(h)); h != "" {
				p.Allow = append(p.Allow, h)
			}
		}
		p.DenyByDefault = true
	}
	return p
}

// ScriptAllowed reports whether script steps may execute.
func (p *Policy) ScriptAllowed() bool { return !p.SafeMode }

// EgressAllowed reports whether an outbound call to rawURL is permitted.
// With no allow-list, everything is allowed; with a list, the host must match.
func (p *Policy) EgressAllowed(rawURL string) bool {
	if p == nil || len(p.Allow) == 0 {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range p.Allow {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// Summary is a one-line description for startup logging.
func (p *Policy) Summary() string {
	if p.SafeMode {
		return "safe-mode (script + arbitrary-HTTP disabled)"
	}
	if len(p.Allow) > 0 {
		return "egress allow-list: " + strings.Join(p.Allow, ", ") + " (default deny)"
	}
	return "egress: unrestricted"
}

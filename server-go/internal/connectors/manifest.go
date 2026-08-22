// Package connectors implements the P4.2 Connector SDK: a directory-based
// manifest format (connector.yaml), a registry (embedded built-ins plus user
// drop-in dirs), parameter validation, secret/env/input templating, and the
// `connector` step executor (HTTP, SMTP, WASM kinds) registered with the
// executor registry. See docs/decisions.md D1/D3.
package connectors

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Executors supported by connector manifests.
const (
	KindHTTP = "http"
	KindSMTP = "smtp"
	KindWASM = "wasm"
)

// Auth modes. oauth2-client-credentials is reserved for P5.
const (
	AuthNone   = "none"
	AuthHeader = "header"
	AuthBearer = "bearer"
	AuthBasic  = "basic"
)

// Manifest is connector.yaml — the typed connector contract.
type Manifest struct {
	ID           string    `yaml:"id" json:"id"`
	Name         string    `yaml:"name" json:"name"`
	Version      string    `yaml:"version" json:"version"`
	Description  string    `yaml:"description" json:"description"`
	Category     string    `yaml:"category" json:"category"`
	Executor     string    `yaml:"executor" json:"executor"`
	Auth         Auth      `yaml:"auth" json:"auth"`
	ParamsSchema string    `yaml:"paramsSchema" json:"paramsSchema"` // file name within the connector dir
	HTTP         *HTTPOpts `yaml:"http,omitempty" json:"http,omitempty"`
	SMTP         *SMTPOpts `yaml:"smtp,omitempty" json:"smtp,omitempty"`
	WASM         *WASMOpts `yaml:"wasm,omitempty" json:"wasm,omitempty"`

	// Params is the parsed params schema (from ParamsSchema file), set by load.
	Params json.RawMessage `yaml:"-" json:"params,omitempty"`
}

// Auth declares how the connector authenticates.
type Auth struct {
	Mode       string `yaml:"mode" json:"mode"`
	HeaderName string `yaml:"headerName,omitempty" json:"headerName,omitempty"`
	// SecretKey names the secret used for bearer/basic/header auth.
	SecretKey string `yaml:"secretKey,omitempty" json:"secretKey,omitempty"`
}

// HTTPOpts is the http executor config (all values templated).
type HTTPOpts struct {
	Method  string            `yaml:"method" json:"method"`
	URL     string            `yaml:"url" json:"url"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Body    string            `yaml:"body,omitempty" json:"body,omitempty"`
}

// SMTPOpts is the smtp executor config (all values templated).
type SMTPOpts struct {
	Host    string `yaml:"host" json:"host"`
	Port    int    `yaml:"port" json:"port"`
	From    string `yaml:"from" json:"from"`
	To      string `yaml:"to" json:"to"`
	Subject string `yaml:"subject" json:"subject"`
	Body    string `yaml:"body" json:"body"`
	// UserSecret/PassSecret name secrets used for SMTP AUTH.
	UserSecret string `yaml:"userSecret,omitempty" json:"userSecret,omitempty"`
	PassSecret string `yaml:"passSecret,omitempty" json:"passSecret,omitempty"`
}

// WASMOpts is the wasm executor config (P4.3 plugin runtime).
type WASMOpts struct {
	Module string `yaml:"module" json:"module"` // file name within the connector dir
}

// Summary is the API-facing manifest view (no templates that embed secrets).
func (m *Manifest) Summary() map[string]any {
	return map[string]any{
		"id": m.ID, "name": m.Name, "version": m.Version, "description": m.Description,
		"category": m.Category, "executor": m.Executor, "authMode": m.Auth.Mode,
		"paramsSchema": m.Params,
	}
}

// Validate checks the manifest against the SDK rules.
func (m *Manifest) Validate() error {
	if m.ID == "" || !idRe(m.ID) {
		return fmt.Errorf("id must be kebab-case (got %q)", m.ID)
	}
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	switch m.Executor {
	case KindHTTP:
		if m.HTTP == nil {
			return fmt.Errorf("executor %q requires an `http:` block", KindHTTP)
		}
		if m.HTTP.URL == "" {
			return fmt.Errorf("http.url is required")
		}
		if m.HTTP.Method == "" {
			m.HTTP.Method = "GET"
		}
	case KindSMTP:
		if m.SMTP == nil {
			return fmt.Errorf("executor %q requires an `smtp:` block", KindSMTP)
		}
		if m.SMTP.Host == "" || m.SMTP.From == "" || m.SMTP.To == "" {
			return fmt.Errorf("smtp.host, smtp.from and smtp.to are required")
		}
		if m.SMTP.Port == 0 {
			m.SMTP.Port = 587
		}
	case KindWASM:
		if m.WASM == nil || m.WASM.Module == "" {
			return fmt.Errorf("executor %q requires wasm.module", KindWASM)
		}
	default:
		return fmt.Errorf("unknown executor %q (want %s|%s|%s)", m.Executor, KindHTTP, KindSMTP, KindWASM)
	}
	switch m.Auth.Mode {
	case "", AuthNone:
		m.Auth.Mode = AuthNone
	case AuthHeader:
		if m.Auth.HeaderName == "" || m.Auth.SecretKey == "" {
			return fmt.Errorf("auth mode %q requires headerName and secretKey", AuthHeader)
		}
	case AuthBearer, AuthBasic:
		if m.Auth.SecretKey == "" {
			return fmt.Errorf("auth mode %q requires secretKey", m.Auth.Mode)
		}
	default:
		return fmt.Errorf("unknown auth mode %q", m.Auth.Mode)
	}
	return nil
}

func idRe(s string) bool {
	if s == "" || len(s) > 60 {
		return false
	}
	for i, c := range s {
		ok := c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-'
		if !ok {
			return false
		}
		if c == '-' && (i == 0 || i == len(s)-1) {
			return false
		}
	}
	return true
}

// ValidateParams checks step params against the connector's params schema.
// The minimal P4 validator enforces `required` presence and string types
// (params are map[string]string by DSL contract); richer JSON-Schema rules
// arrive with the P4.5 contract tooling.
func (m *Manifest) ValidateParams(params map[string]string) error {
	if len(m.Params) == 0 {
		return nil
	}
	var schema struct {
		Type     string              `json:"type"`
		Required []string            `json:"required"`
		Props    map[string]struct { //nolint:revive
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(m.Params, &schema); err != nil {
		return fmt.Errorf("invalid params schema in connector %s: %v", m.ID, err)
	}
	if schema.Type != "" && schema.Type != "object" {
		return fmt.Errorf("connector %s params schema must be type object", m.ID)
	}
	var missing []string
	for _, r := range schema.Required {
		if strings.TrimSpace(params[r]) == "" {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required params: %s", strings.Join(missing, ", "))
	}
	return nil
}

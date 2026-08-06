// Package settings manages the runtime AI authoring configuration (provider,
// API key, base URL, model), persisted in SQLite and editable from the Admin
// console. Mirrors server/src/settings.ts.
package settings

import (
	"encoding/json"
	"os"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/store"
)

const settingsKey = "ai"

type ProviderPreset struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	BaseURL      string `json:"baseURL"`
	DefaultModel string `json:"defaultModel"`
	NeedsKey     bool   `json:"needsKey"`
	Local        bool   `json:"local"`
	Hint         string `json:"hint"`
}

var Providers = []ProviderPreset{
	{ID: "openai", Label: "OpenAI", BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-4o-mini", NeedsKey: true, Hint: "Get a key at platform.openai.com"},
	{ID: "openrouter", Label: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", DefaultModel: "anthropic/claude-3.5-sonnet", NeedsKey: true, Hint: "One key for Anthropic, Google, Meta and more"},
	{ID: "groq", Label: "Groq", BaseURL: "https://api.groq.com/openai/v1", DefaultModel: "llama-3.3-70b-versatile", NeedsKey: true, Hint: "Fast inference for open models"},
	{ID: "together", Label: "Together AI", BaseURL: "https://api.together.xyz/v1", DefaultModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo", NeedsKey: true, Hint: "Hosted open models"},
	{ID: "ollama", Label: "Ollama (Local)", BaseURL: "http://localhost:11434/v1", DefaultModel: "llama3.1", NeedsKey: false, Local: true, Hint: "Runs on your machine — no key, works offline"},
	{ID: "lmstudio", Label: "LM Studio (Local)", BaseURL: "http://localhost:1234/v1", DefaultModel: "local-model", NeedsKey: false, Local: true, Hint: "Local server with an OpenAI-compatible endpoint"},
	{ID: "custom", Label: "Custom (OpenAI-compatible)", BaseURL: "", DefaultModel: "gpt-3.5-turbo", NeedsKey: true, Hint: "Any endpoint speaking the OpenAI Chat Completions API"},
}

func presetOf(provider string) ProviderPreset {
	for _, p := range Providers {
		if p.ID == provider {
			return p
		}
	}
	return Providers[0]
}

// IsLLMActive reports whether a real model will be attempted (local providers
// are always considered active; cloud providers need a key).
func IsLLMActive(cfg models.AIConfig) bool {
	if presetOf(cfg.Provider).Local {
		return true
	}
	return cfg.APIKey != ""
}

// GetConfig returns the live AI config, falling back to env then preset defaults.
func GetConfig(s *store.Store) models.AIConfig {
	var stored models.AIConfig
	if raw, ok, _ := s.GetSetting(settingsKey); ok {
		_ = json.Unmarshal([]byte(raw), &stored)
	}
	provider := stored.Provider
	if provider == "" {
		if os.Getenv("OPENAI_API_KEY") != "" {
			provider = "openai"
		} else {
			provider = "ollama"
		}
	}
	preset := presetOf(provider)
	cfg := models.AIConfig{
		Provider: provider,
		APIKey:   firstNonEmpty(stored.APIKey, os.Getenv("OPENAI_API_KEY")),
		BaseURL:  firstNonEmpty(stored.BaseURL, preset.BaseURL, os.Getenv("OPENAI_BASE_URL"), "https://api.openai.com/v1"),
		Model:    firstNonEmpty(stored.Model, preset.DefaultModel, os.Getenv("OPENAI_MODEL"), "gpt-4o-mini"),
	}
	return cfg
}

// SetConfig persists the config; an empty key preserves the existing one.
func SetConfig(s *store.Store, incoming models.AIConfig) (models.AIConfig, error) {
	cur := GetConfig(s)
	provider := incoming.Provider
	if provider == "" {
		provider = cur.Provider
	}
	preset := presetOf(provider)
	next := models.AIConfig{
		Provider: provider,
		BaseURL:  firstNonEmpty(incoming.BaseURL, preset.BaseURL, cur.BaseURL),
		Model:    firstNonEmpty(incoming.Model, preset.DefaultModel, cur.Model),
	}
	if incoming.APIKey != "" {
		next.APIKey = incoming.APIKey
	} else {
		next.APIKey = cur.APIKey
	}
	b, _ := json.Marshal(next)
	if err := s.SetSetting(settingsKey, string(b)); err != nil {
		return next, err
	}
	return next, nil
}

// AuthoringModel returns the display label for the active model (or fallback note).
func AuthoringModel(cfg models.AIConfig) string {
	if IsLLMActive(cfg) {
		return cfg.Model
	}
	return "flowforge-author (deterministic · no model set)"
}

// PublicSettings is the GET /settings/ai response (key masked, never raw).
type PublicSettings struct {
	Provider    string           `json:"provider"`
	BaseURL     string           `json:"baseURL"`
	Model       string           `json:"model"`
	HasKey      bool             `json:"hasKey"`
	MaskedKey   string           `json:"maskedKey"`
	Active      bool             `json:"active"`
	ActiveLabel string           `json:"activeLabel"`
	NeedsKey    bool             `json:"needsKey"`
	Local       bool             `json:"local"`
	Providers   []ProviderPreset `json:"providers"`
}

func Public(s *store.Store) PublicSettings {
	cfg := GetConfig(s)
	preset := presetOf(cfg.Provider)
	label := AuthoringModel(cfg)
	if IsLLMActive(cfg) {
		label = preset.Label + " · " + cfg.Model
	} else {
		label = "Deterministic fallback (no model configured)"
	}
	return PublicSettings{
		Provider: cfg.Provider, BaseURL: cfg.BaseURL, Model: cfg.Model,
		HasKey: cfg.APIKey != "", MaskedKey: maskKey(cfg.APIKey),
		Active: IsLLMActive(cfg), ActiveLabel: label,
		NeedsKey: preset.NeedsKey, Local: preset.Local, Providers: Providers,
	}
}

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "••••"
	}
	return k[:3] + "••••" + k[len(k)-4:]
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

package ai

// AI-03: opt-in live LLM authoring. Skips unless FLOWFORGE_LIVE_AI_KEY /
// OPENAI_API_KEY is set — run with:
//
//	FLOWFORGE_LIVE_AI_KEY=sk-... go test ./internal/ai/ -run TestAI03_LiveLLM -v
import (
	"os"
	"testing"

	"github.com/santhoshmp/flowforge/internal/models"
)

func TestAI03_LiveLLM(t *testing.T) {
	key := envOr("FLOWFORGE_LIVE_AI_KEY", envOr("OPENAI_API_KEY", ""))
	if key == "" {
		t.Skip("AI-03 live test: no API key set (FLOWFORGE_LIVE_AI_KEY / OPENAI_API_KEY)")
	}
	baseURL := envOr("FLOWFORGE_LIVE_AI_BASE_URL", envOr("OPENAI_BASE_URL", "https://api.openai.com/v1"))
	model := envOr("OPENAI_MODEL", "gpt-4o-mini")

	res := GenerateDraft("When an invoice over 10000 arrives, validate the vendor, route to the manager for approval within 48 hours, then post to ERP.", models.AIConfig{
		Provider: "openai", APIKey: key, BaseURL: baseURL, Model: model,
	})
	if res.Source != "llm" {
		t.Fatalf("expected live LLM authoring (source=llm), got %q", res.Source)
	}
	if len(res.Draft.Steps) < 3 {
		t.Fatalf("live draft too thin: %d steps", len(res.Draft.Steps))
	}
	t.Logf("live model %s drafted %d steps", res.Model, len(res.Draft.Steps))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

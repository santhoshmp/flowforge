package executor

// DLQ-01..03: failure-alert digests (ALERT_WEBHOOK_URL).

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/flowforge/flowforge/internal/policy"
)

// DLQ-01: a configured ops webhook receives the failure digest.
func TestDLQ01_AlertWebhookFires(t *testing.T) {
	var hits int32
	var body atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		body.Store(string(buf[:n]))
		w.WriteHeader(200)
	}))
	defer srv.Close()
	_ = vault(t).Set("ALERT_WEBHOOK_URL", srv.URL)

	SendFailureAlert("Vendor Invoice Approval", "run-1", "Post to ERP", "egress blocked", &policy.Policy{})
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("alert hits = %d", hits)
	}
	b, _ := body.Load().(string)
	if !contains(b, "Vendor Invoice Approval") || !contains(b, "Post to ERP") || !contains(b, "run-1") {
		t.Fatalf("alert body = %q", b)
	}
	_ = vault(t).Delete("ALERT_WEBHOOK_URL")
}

// DLQ-02: no secret configured -> silently no alert (demos never break).
func TestDLQ02_NoAlertWithoutSecret(t *testing.T) {
	_ = vault(t).Delete("ALERT_WEBHOOK_URL")
	SendFailureAlert("w", "run-2", "s", "err", &policy.Policy{}) // must not panic or block
}

// DLQ-03: safe-mode and egress denials skip the alert silently.
func TestDLQ03_PolicySkipsAlert(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()
	_ = vault(t).Set("ALERT_WEBHOOK_URL", srv.URL)
	defer vault(t).Delete("ALERT_WEBHOOK_URL")

	SendFailureAlert("w", "run-3", "s", "err", &policy.Policy{SafeMode: true})
	SendFailureAlert("w", "run-4", "s", "err", &policy.Policy{Allow: []string{"api.openai.com"}, DenyByDefault: true})
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("policy-gated alerts fired: %d", hits)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

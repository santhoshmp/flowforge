package executor

// NOTIF-01..05: real notify steps (F-NOTIF). Slack rides httptest + the
// egress gate; email rides a minimal in-test SMTP server.

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/secrets"
)

func TestMain(m *testing.M) {
	// Isolate the secrets vault for the whole executor package.
	dir, _ := os.MkdirTemp("", "ff-exec-secrets-*")
	_ = os.Setenv("FLOWFORGE_SECRETS_FILE", filepath.Join(dir, "test.secrets"))
	secrets.Reset()
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func vault(t *testing.T) *secrets.Vault {
	t.Helper()
	v, err := secrets.Default()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// smtpStub serves a minimal SMTP dialogue and captures the message.
func smtpStub(t *testing.T) (host string, port int, mail *capturedMail) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	mail = &capturedMail{}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		defer ln.Close()
		r := bufio.NewReader(conn)
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 stub")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			raw := strings.TrimSpace(line)
			up := strings.ToUpper(raw)
			switch {
			case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
				write("250-stub")
				write("250 8BITMIME")
			case strings.HasPrefix(up, "AUTH"):
				write("235 ok")
			case strings.HasPrefix(up, "MAIL FROM:"):
				mail.from = strings.TrimPrefix(raw, "MAIL FROM:")
				write("250 ok")
			case strings.HasPrefix(up, "RCPT TO:"):
				mail.to = append(mail.to, strings.TrimPrefix(raw, "RCPT TO:"))
				write("250 ok")
			case strings.HasPrefix(up, "DATA"):
				write("354 end with .")
				var body strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimRight(l, "\r\n") == "." {
						break
					}
					body.WriteString(l)
				}
				mail.data = body.String()
				write("250 ok")
			case strings.HasPrefix(up, "QUIT"):
				write("221 bye")
				return
			default:
				write("250 ok")
			}
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return "127.0.0.1", addr.Port, mail
}

type capturedMail struct {
	from string
	to   []string
	data string
}

// NOTIF-01: slack notify posts to the webhook through the egress gate.
func TestNOTIF01_SlackSends(t *testing.T) {
	var hits int32
	var body atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		b := &bytes.Buffer{}
		_, _ = b.ReadFrom(r.Body)
		body.Store(b.String())
		w.WriteHeader(200)
	}))
	defer srv.Close()
	host := hostOf(srv.URL)
	_ = vault(t).Set("SLACK_WEBHOOK_URL", srv.URL+"/services/T1/B1/x")

	step := &models.WorkflowStep{Type: "notify", Name: "Page on-call", Params: map[string]string{
		"channel": "slack", "text": "line down on Nordwind 3",
	}}
	if !ForType("notify").Configured(step) {
		t.Fatal("slack notify should be configured when the webhook secret exists")
	}
	out, err, real := Run(step, nil, &policy.Policy{Allow: []string{host}, DenyByDefault: true})
	if !real || err != nil {
		t.Fatalf("run: real=%v err=%v", real, err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("webhook hits = %d", hits)
	}
	if !strings.Contains(body.Load().(string), "line down on Nordwind 3") {
		t.Fatalf("webhook body = %v", body.Load())
	}
	_ = out

	// Egress deny fails the step loudly.
	_, err, real = Run(step, nil, &policy.Policy{Allow: []string{"api.openai.com"}, DenyByDefault: true})
	if !real || err == nil || !strings.Contains(err.Error(), "egress") {
		t.Fatalf("egress deny should fail: real=%v err=%v", real, err)
	}
}

// NOTIF-02: email notify sends via SMTP to all recipients.
func TestNOTIF02_EmailSends(t *testing.T) {
	host, port, mail := smtpStub(t)
	v := vault(t)
	_ = v.Set("SMTP_HOST", host)
	_ = v.Set("SMTP_PORT", itoa(port))
	_ = v.Set("SMTP_FROM", "flowforge@meridian.example")

	step := &models.WorkflowStep{Type: "notify", Name: "Notify requester", Params: map[string]string{
		"channel": "email", "recipients": "ops@meridian.example, qa@meridian.example", "subject": "PO approved", "text": "PO-4515 approved",
	}}
	if !ForType("notify").Configured(step) {
		t.Fatal("email notify should be configured when SMTP_HOST + recipients exist")
	}
	out, err, real := Run(step, nil, &policy.Policy{})
	if !real || err != nil {
		t.Fatalf("run: real=%v err=%v", real, err)
	}
	if !strings.Contains(out, "ops@meridian.example") {
		t.Fatalf("output = %q", out)
	}
	if len(mail.to) != 2 || !strings.Contains(mail.data, "PO approved") || !strings.Contains(mail.data, "PO-4515 approved") {
		t.Fatalf("captured mail: to=%v data=%.200s", mail.to, mail.data)
	}
	if !strings.Contains(mail.from, "flowforge@meridian.example") {
		t.Fatalf("from = %q", mail.from)
	}
}

// NOTIF-03: safe-mode fails configured notifications (consistent with every
// other real-execution step).
func TestNOTIF03_SafeModeBlocks(t *testing.T) {
	_ = vault(t).Set("SLACK_WEBHOOK_URL", "https://hooks.example/x")
	step := &models.WorkflowStep{Type: "notify", Name: "N", Params: map[string]string{"channel": "slack"}}
	_, err, real := Run(step, nil, &policy.Policy{SafeMode: true})
	if !real || err == nil || !strings.Contains(err.Error(), "safe-mode") {
		t.Fatalf("safe-mode should fail the step: real=%v err=%v", real, err)
	}
}

// NOTIF-04: unconfigured backends stay simulated (real=false) — demos never
// break because SMTP/Slack is absent.
func TestNOTIF04_UnconfiguredSimulated(t *testing.T) {
	v := vault(t)
	// Reset delivery secrets so earlier tests' configuration can't leak.
	_ = v.Delete("SMTP_HOST")
	_ = v.Delete("SLACK_WEBHOOK_URL")
	_ = v.Delete("SMTP_FROM")
	cases := []map[string]string{
		{"channel": "email", "recipients": ""},      // host set below, but no recipients
		{"channel": "email", "recipients": "a@b.c"}, // recipients, but no host
		{"channel": "slack"},                        // no webhook
		{"channel": "pagerduty"},                    // unsupported channel
	}
	_ = v.Set("SMTP_HOST", "smtp.example") // only for the first case
	defer v.Delete("SMTP_HOST")
	for _, p := range cases {
		if p["channel"] == "email" && p["recipients"] == "a@b.c" {
			_ = v.Delete("SMTP_HOST")
		}
		step := &models.WorkflowStep{Type: "notify", Name: "N", Params: p}
		if e := ForType("notify"); e.Configured(step) {
			t.Fatalf("channel=%s recipients=%q should be unconfigured/simulated", p["channel"], p["recipients"])
		}
		_, _, real := Run(step, nil, &policy.Policy{})
		if real {
			t.Fatalf("channel=%s must simulate when unconfigured", p["channel"])
		}
	}
}

// NOTIF-05: engine integration — a notify step with a configured backend
// produces a real-send output; unconfigured produces the simulated default.
func TestNOTIF05_EngineIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	_ = vault(t).Set("SLACK_WEBHOOK_URL", srv.URL)

	out, err, _ := Run(&models.WorkflowStep{Type: "notify", Name: "N", Params: map[string]string{
		"channel": "slack", "text": "integration",
	}}, nil, &policy.Policy{})
	if err != nil || !strings.Contains(out, "slack message sent") {
		t.Fatalf("engine notify: %v %q", err, out)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

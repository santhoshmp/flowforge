// Real notify steps: `type: notify` sends for real when the delivery
// backend is configured (secrets present), else the engine simulates —
// notifications are best-effort, never a reason a demo flow breaks.
//
//	channel: email → secrets SMTP_HOST (+ optional SMTP_PORT/SMTP_FROM/
//	          SMTP_USER/SMTP_PASS) and params recipients (comma-separated)
//	channel: slack → secret SLACK_WEBHOOK_URL
//
// Like every configured execution step, real sends are disabled and FAILED
// under safe-mode; slack posts ride the egress allow-list.
package executor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/secrets"
)

const notifyHTTPTimeout = 10 * time.Second

func init() { Register(notifyExecutor{}) }

type notifyExecutor struct{}

func (notifyExecutor) Name() string          { return "notify" }
func (notifyExecutor) Matches(t string) bool { return t == "notify" }

// Configured: real only when the channel's backend secrets exist. Missing
// configuration means simulate (the engine's default output covers it).
func (notifyExecutor) Configured(step *models.WorkflowStep) bool {
	switch strings.ToLower(step.Params["channel"]) {
	case "email":
		host, ok := notifySecret("SMTP_HOST")
		return ok && host != "" && strings.TrimSpace(step.Params["recipients"]) != ""
	case "slack":
		url, ok := notifySecret("SLACK_WEBHOOK_URL")
		return ok && url != ""
	default:
		return false
	}
}

func (notifyExecutor) Run(step *models.WorkflowStep, _ map[string]any, pol *policy.Policy) (string, error) {
	if pol != nil && pol.SafeMode {
		return "", errors.New("blocked: safe-mode disables notification steps")
	}
	text := step.Params["text"]
	if text == "" {
		text = step.Params["template"]
	}
	if text == "" {
		text = "FlowForge: " + step.Name
	}
	switch strings.ToLower(step.Params["channel"]) {
	case "email":
		return sendNotifyEmail(step, text)
	case "slack":
		return sendNotifySlack(text, pol)
	default:
		return "", fmt.Errorf("notify: unsupported channel %q (real sends support email, slack)", step.Params["channel"])
	}
}

func sendNotifyEmail(step *models.WorkflowStep, text string) (string, error) {
	vault, err := secrets.Default()
	if err != nil {
		return "", err
	}
	host, _ := vault.Get("SMTP_HOST")
	port := 587
	if p, ok := vault.Get("SMTP_PORT"); ok {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			port = v
		}
	}
	from := "flowforge@localhost"
	if f, ok := vault.Get("SMTP_FROM"); ok && f != "" {
		from = f
	}
	var to []string
	for _, r := range strings.Split(step.Params["recipients"], ",") {
		if r = strings.TrimSpace(r); r != "" {
			to = append(to, r)
		}
	}
	if len(to) == 0 {
		return "", errors.New("notify email: no recipients")
	}
	subject := step.Params["subject"]
	if subject == "" {
		subject = "FlowForge: " + step.Name
	}
	msg := &bytes.Buffer{}
	fmt.Fprintf(msg, "From: %s\r\n", from)
	fmt.Fprintf(msg, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(msg, "Subject: %s\r\n\r\n", subject)
	fmt.Fprint(msg, text)

	var auth smtp.Auth
	if u, ok := vault.Get("SMTP_USER"); ok && u != "" {
		p, _ := vault.Get("SMTP_PASS")
		auth = smtp.PlainAuth("", u, p, host)
	}
	addr := host + ":" + strconv.Itoa(port)
	if err := smtp.SendMail(addr, auth, from, to, msg.Bytes()); err != nil {
		return "", fmt.Errorf("notify email: %v", err)
	}
	return fmt.Sprintf("email sent to %s", strings.Join(to, ", ")), nil
}

func sendNotifySlack(text string, pol *policy.Policy) (string, error) {
	vault, err := secrets.Default()
	if err != nil {
		return "", err
	}
	url, _ := vault.Get("SLACK_WEBHOOK_URL")
	if pol != nil && !pol.EgressAllowed(url) {
		return "", errors.New("notify slack: blocked by egress policy")
	}
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: notifyHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("notify slack: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("notify slack: webhook returned %d", resp.StatusCode)
	}
	return "slack message sent", nil
}

func notifySecret(name string) (string, bool) {
	vault, err := secrets.Default()
	if err != nil {
		return "", false
	}
	v, ok := vault.Get(name)
	return v, ok && v != ""
}

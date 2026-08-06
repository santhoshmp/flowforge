// Package ai turns a natural-language prompt into a flowforge draft. It calls
// an OpenAI-compatible chat endpoint when a model is configured and falls back
// to a deterministic local generator otherwise. Mirrors server/src/ai.ts.
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/flowforge/flowforge/internal/models"
	"github.com/flowforge/flowforge/internal/settings"
	"github.com/flowforge/flowforge/internal/util"
)

type GeneratedDraft struct {
	Name               string                `json:"name"`
	Description        string                `json:"description"`
	Steps              []models.WorkflowStep `json:"steps"`
	Model              string                `json:"model"`
	OverallConfidence  int                   `json:"overallConfidence"`
}

type DraftResult struct {
	Draft         GeneratedDraft `json:"draft"`
	Source        string         `json:"source"` // "llm" | "fallback"
	Model         string         `json:"model"`
	SamplePrompts []string       `json:"samplePrompts,omitempty"`
}

var SamplePrompts = []string{
	"When a vendor invoice over $10K arrives, extract line items, validate against the vendor master, route to the cost-center manager for approval, escalate to Finance VP after 48 hours, then post to the ERP.",
	"When an employee onboarding request is submitted, validate the employee against the HR master, route to the hiring manager for approval, notify IT on Slack, then post to Workday.",
	"When a support ticket is created, classify it with AI, if priority is critical notify the on-call team on Slack, route to the support lead for approval, escalate to the support director after 4 hours.",
}

// GenerateDraft produces a draft with the configured model, falling back to the
// deterministic generator on any failure so the demo always works.
func GenerateDraft(prompt string, cfg models.AIConfig) DraftResult {
	if settings.IsLLMActive(cfg) {
		if d, err := authorWithLLM(prompt, cfg); err == nil {
			d.Model = cfg.Model
			return DraftResult{Draft: d, Source: "llm", Model: cfg.Model, SamplePrompts: SamplePrompts}
		}
	}
	d := authorDeterministic(prompt)
	d.Model = "flowforge-author (deterministic fallback)"
	return DraftResult{Draft: d, Source: "fallback", Model: d.Model, SamplePrompts: SamplePrompts}
}

// ---- LLM path ---------------------------------------------------------------

const systemPrompt = `You are FlowForge Author, an expert that turns a natural-language description of a business process into a structured workflow draft.

Output STRICT JSON only (no prose, no markdown) with this exact shape:
{
  "name": "short workflow name",
  "description": "one sentence",
  "steps": [
    { "type": "<stepType>", "name": "human readable", "params": { "key": "value" }, "confidence": <0-100>, "assumptions": ["any inferred decision a human must confirm"] }
  ]
}

Rules:
- The FIRST step MUST have type "trigger".
- Choose stepType from exactly: trigger, ai.extract, ai.classify, mdm.lookup, mdm.validate, condition, human.approval, notify, integration.post, integration.http, script, wait.
- "route to <role> for approval" -> human.approval with params { "approver": "<role>", "sla_hours": "<n>" }; "escalate to <role> after Xh" -> human.approval with params { "approver": "<role>", "after_hours": "<n>", "condition": "previous_step.sla_breached" }; "notify/email/slack" -> notify with params { "channel": "email|slack|teams" }; "post to <system>" -> integration.post with params { "system": "<System>", "endpoint": "<system>.inbound" }; amount/threshold checks -> condition with params { "expression": "total > <number>", "on_false": "auto_approve" }.
- Set confidence (0-100) per step: high for explicit, lower for inferred. Add an assumption string for anything inferred. Keep params values as strings. 4-8 steps is ideal.`

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatReq struct {
	Model          string    `json:"model"`
	Temperature    float64   `json:"temperature"`
	Messages       []chatMsg `json:"messages"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}
type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func authorWithLLM(prompt string, cfg models.AIConfig) (GeneratedDraft, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	messages := []chatMsg{{Role: "system", Content: systemPrompt}, {Role: "user", Content: prompt}}

	content, err := chat(client, cfg, messages, true)
	if err != nil || content == "" {
		c2, err2 := chat(client, cfg, messages, false) // retry without JSON mode (broader compat)
		if err2 != nil {
			return GeneratedDraft{}, err2
		}
		content = c2
	}
	if content == "" {
		return GeneratedDraft{}, fmt.Errorf("empty completion")
	}
	var raw rawDraft
	if err := unmarshalJSON(content, &raw); err != nil {
		return GeneratedDraft{}, err
	}
	return normalizeDraft(raw, prompt), nil
}

func chat(client *http.Client, cfg models.AIConfig, messages []chatMsg, jsonMode bool) (string, error) {
	req := chatReq{Model: cfg.Model, Temperature: 0.3, Messages: messages}
	if jsonMode {
		req.ResponseFormat = &struct {
			Type string `json:"type"`
		}{Type: "json_object"}
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	key := cfg.APIKey
	if key == "" {
		key = "local-no-key"
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	var cr chatResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("no choices")
	}
	return cr.Choices[0].Message.Content, nil
}

func unmarshalJSON(content string, v any) error {
	if err := json.Unmarshal([]byte(content), v); err == nil {
		return nil
	}
	start := strings.IndexAny(content, "{[")
	if start < 0 {
		return fmt.Errorf("no JSON found")
	}
	closeC := byte('}')
	if content[start] == '[' {
		closeC = ']'
	}
	end := strings.LastIndexByte(content, closeC)
	if end <= start {
		return fmt.Errorf("could not parse JSON")
	}
	return json.Unmarshal([]byte(content[start:end+1]), v)
}

// ---- Normalization (shared by LLM path) -------------------------------------

type rawStep struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Params      map[string]any `json:"params"`
	Confidence  any            `json:"confidence"`
	Assumptions []string       `json:"assumptions"`
}
type rawDraft struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Steps       []rawStep `json:"steps"`
}

var allowedTypes = map[string]bool{
	"trigger": true, "ai.extract": true, "ai.classify": true, "mdm.lookup": true, "mdm.validate": true,
	"condition": true, "human.approval": true, "notify": true, "integration.post": true,
	"integration.http": true, "script": true, "wait": true,
}

var typeAliases = map[string]string{
	"trigger": "trigger", "start": "trigger", "when": "trigger", "event": "trigger",
	"ai.extract": "ai.extract", "extract": "ai.extract", "ocr": "ai.extract", "parse": "ai.extract",
	"ai.classify": "ai.classify", "classify": "ai.classify", "categorize": "ai.classify",
	"mdm.lookup": "mdm.lookup", "lookup": "mdm.lookup", "resolve": "mdm.lookup",
	"mdm.validate": "mdm.validate", "validate": "mdm.validate", "verify": "mdm.validate", "match": "mdm.validate",
	"condition": "condition", "branch": "condition", "if": "condition", "decision": "condition", "gate": "condition",
	"human.approval": "human.approval", "approval": "human.approval", "approve": "human.approval", "human": "human.approval", "review": "human.approval", "signoff": "human.approval",
	"notify": "notify", "notification": "notify", "email": "notify", "slack": "notify", "teams": "notify", "send": "notify", "alert": "notify",
	"integration.post": "integration.post", "post": "integration.post", "create": "integration.post",
	"integration.http": "integration.http", "http": "integration.http", "api": "integration.http", "call": "integration.http", "request": "integration.http",
	"script": "script", "code": "script", "transform": "script", "compute": "script",
	"wait": "wait", "delay": "wait", "sla": "wait", "timer": "wait", "pause": "wait",
}

func normalizeType(t string) string {
	k := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(t, " ", "")))
	if allowedTypes[k] {
		return k
	}
	if a, ok := typeAliases[k]; ok {
		return a
	}
	return "script"
}

func toStringParams(raw map[string]any) map[string]string {
	out := map[string]string{}
	for k, v := range raw {
		switch vv := v.(type) {
		case nil:
		case string:
			out[k] = vv
		default:
			b, _ := json.Marshal(v)
			out[k] = string(b)
		}
	}
	return out
}

func clampConf(v any) int {
	switch n := v.(type) {
	case float64:
		return clamp(int(n))
	case int:
		return clamp(n)
	}
	return 80
}
func clamp(n int) int {
	if n < 50 {
		return 50
	}
	if n > 99 {
		return 99
	}
	return n
}

func normalizeDraft(raw rawDraft, prompt string) GeneratedDraft {
	var steps []models.WorkflowStep
	for _, s := range raw.Steps {
		typ := normalizeType(s.Type)
		name := s.Name
		if name == "" {
			name = typ
		}
		if len(name) > 80 {
			name = name[:80]
		}
		assumptions := s.Assumptions
		if assumptions == nil {
			assumptions = []string{}
		}
		steps = append(steps, models.WorkflowStep{
			ID: util.Slug(name) + "_" + util.UID(), Type: typ, Name: name,
			Params: toStringParams(s.Params), Confidence: clampConf(s.Confidence), Assumptions: assumptions,
		})
	}

	// ensure exactly one trigger, first
	triggerIdx := -1
	for i, s := range steps {
		if s.Type == "trigger" {
			triggerIdx = i
			break
		}
	}
	if triggerIdx == -1 {
		m := regexp.MustCompile(`(?i)when (?:an? |the )?([a-z ]+?)(?: arrives| is |,|\.|$)`).FindStringSubmatch(prompt)
		entity := "Record"
		if len(m) > 1 {
			entity = strings.TrimSpace(util.TitleCase(m[1]))
		}
		steps = append([]models.WorkflowStep{{
			ID: "trigger_" + util.UID(), Type: "trigger", Name: entity + " received",
			Params: map[string]string{"event": util.Slug(entity) + ".created", "source": "any"}, Confidence: 95,
			Assumptions: []string{"No explicit trigger found — assumed \"" + entity + " received\"."},
		}}, steps...)
	} else {
		// drop extra triggers, move first to front
		keep := steps[:0]
		seenTrigger := false
		for _, s := range steps {
			if s.Type == "trigger" {
				if seenTrigger {
					continue
				}
				seenTrigger = true
			}
			keep = append(keep, s)
		}
		steps = keep
		if steps[0].Type != "trigger" {
			for i, s := range steps {
				if s.Type == "trigger" {
					steps = append(append([]models.WorkflowStep{s}, steps[:i]...), steps[i+1:]...)
					break
				}
			}
		}
	}

	if len(steps) <= 1 {
		steps = append(steps, models.WorkflowStep{
			ID: "script_" + util.UID(), Type: "script", Name: "Process record",
			Params: map[string]string{"runtime": "javascript"}, Confidence: 60,
			Assumptions: []string{"Could not infer detailed steps from the prompt — placeholder."},
		})
	}

	name := raw.Name
	if name == "" {
		name = "Workflow"
	}
	desc := raw.Description
	if desc == "" {
		desc = "Auto-generated from prompt."
	}
	overall := 0
	for _, s := range steps {
		overall += s.Confidence
	}
	overall /= len(steps)

	return GeneratedDraft{Name: name, Description: desc, Steps: steps, OverallConfidence: overall}
}

// ---- Deterministic fallback -------------------------------------------------

var (
	amountRe = regexp.MustCompile(`(?i)\$\s?([\d,.]+)\s?(k|m|thousand|million)?`)
	hoursRe  = regexp.MustCompile(`(?i)(\d+)\s?(hours?|hrs?|days?)`)
	roleRes  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)route(?:d)? to (?:the )?([a-z -]+?)(?: for approval| to approve| after| then| ,|\.|$)`),
		regexp.MustCompile(`(?i)(?:the )?([a-z -]*?(?:manager|vp|director|head|lead|officer|controller|steward|admin|owner))(?: approves| for approval| to approve|,| after| then|\.|$)`),
		regexp.MustCompile(`(?i)escalate(?:s|d)? to (?:the )?([a-z -]+?)(?: after| if| ,|\.|$)`),
	}
	triggerRe = regexp.MustCompile(`(?i)when (?:an? |the )?([a-z ]+?)(?: arrives| is (?:created|submitted|received)| is over|,|\.|$)`)
)

func extractAmount(text string) (string, bool) {
	m := amountRe.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	n := 0.0
	for _, r := range m[1] {
		if r >= '0' && r <= '9' {
			n = n*10 + float64(r-'0')
		}
	}
	switch strings.ToLower(m[2]) {
	case "k", "thousand":
		n *= 1000
	case "m", "million":
		n *= 1000000
	}
	return fmt.Sprintf("%d", int(n)), true
}

func extractHours(text string) (string, bool) {
	m := hoursRe.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	n := 0
	for _, r := range m[1] {
		n = n*10 + int(r-'0')
	}
	if strings.Contains(strings.ToLower(m[2]), "day") {
		n *= 24
	}
	return fmt.Sprintf("%d", n), true
}

func extractRole(text string) string {
	for _, re := range roleRes {
		m := re.FindStringSubmatch(text)
		if len(m) > 1 && len(strings.TrimSpace(m[1])) > 2 {
			return util.TitleCase(strings.TrimSpace(m[1]))
		}
	}
	return "Process Owner"
}

func authorDeterministic(prompt string) GeneratedDraft {
	lower := strings.ToLower(prompt)
	var steps []models.WorkflowStep
	add := func(typ, name string, params map[string]string, conf int, assumptions ...string) {
		if params == nil {
			params = map[string]string{}
		}
		if assumptions == nil {
			assumptions = []string{}
		}
		steps = append(steps, models.WorkflowStep{
			ID: util.Slug(name) + "_" + util.UID(), Type: typ, Name: name, Params: params,
			Confidence: conf, Assumptions: assumptions,
		})
	}

	m := triggerRe.FindStringSubmatch(prompt)
	entity := "Record"
	if len(m) > 1 {
		entity = util.TitleCase(strings.TrimSpace(m[1]))
	}
	source := "any"
	if strings.Contains(lower, "email") {
		source = "email"
	} else if strings.Contains(lower, "api") {
		source = "api"
	}
	assumptions := []string{}
	if len(m) <= 1 {
		assumptions = []string{"No explicit trigger found — assumed \"" + entity + " received\"."}
	}
	add("trigger", entity+" received", map[string]string{"event": util.Slug(entity) + ".created", "source": source}, 95, assumptions...)

	amount, hasAmount := extractAmount(prompt)
	if regexp.MustCompile(`extract|parse|read|ocr|line.?item|capture`).MatchString(lower) {
		fields := "key fields"
		if regexp.MustCompile(`line.?item`).MatchString(lower) {
			fields = "line_items, vendor, total, currency, due_date"
		}
		add("ai.extract", "Extract data with AI", map[string]string{"fields": fields, "model": "auto"}, 88, "Field list inferred — confirm exact fields.")
	}
	if regexp.MustCompile(`valid|match|master|vendor|customer|mdm|verify`).MatchString(lower) {
		entityRef := "vendors"
		if strings.Contains(lower, "customer") {
			entityRef = "customers"
		} else if strings.Contains(lower, "product") {
			entityRef = "products"
		}
		matchOn := "id, email"
		if entityRef == "vendors" {
			matchOn = "vendor_id, tax_id"
		}
		add("mdm.validate", "Validate against "+util.TitleCase(entityRef)+" master", map[string]string{"entity": entityRef, "match_on": matchOn, "on_mismatch": "route_to_steward"}, 91, "Assumed master data entity "+entityRef+".")
	}
	if hasAmount || regexp.MustCompile(`if |over |exceed|greater|above`).MatchString(lower) {
		expr := "total > threshold"
		conf := 70
		if hasAmount {
			expr = "total > " + amount
			conf = 93
		}
		add("condition", "Amount check > $"+amount, map[string]string{"expression": expr, "on_false": "auto_approve"}, conf)
	}
	if regexp.MustCompile(`approv|review|sign.?off|route`).MatchString(lower) {
		role := extractRole(prompt)
		params := map[string]string{"approver": role, "resolve_via": "hr_hierarchy"}
		if h, ok := extractHours(prompt); ok {
			params["sla_hours"] = h
		}
		add("human.approval", "Approval by "+role, params, 82, "Approver "+role+" resolves via the HR hierarchy.")
	}
	if regexp.MustCompile(`escalat`).MatchString(lower) {
		em := regexp.MustCompile(`(?i)escalate(?:s|d)? to (?:the )?([a-z -]+?)(?: after| if| ,|\.|$)`).FindStringSubmatch(prompt)
		escRole := "Finance VP"
		if len(em) > 1 {
			escRole = util.TitleCase(strings.TrimSpace(em[1]))
		}
		params := map[string]string{"approver": escRole, "condition": "previous_step.sla_breached"}
		if h, ok := extractHours(prompt); ok {
			params["after_hours"] = h
		}
		add("human.approval", "Escalation to "+escRole, params, 78, "Escalation triggers on SLA breach of the previous step.")
	}
	if regexp.MustCompile(`notif|inform|email|slack|teams|alert`).MatchString(lower) {
		ch := "email"
		if strings.Contains(lower, "slack") {
			ch = "slack"
		} else if strings.Contains(lower, "teams") {
			ch = "teams"
		}
		add("notify", "Notify stakeholders", map[string]string{"channel": ch, "recipients": "requester, procurement"}, 86)
	}
	if regexp.MustCompile(`post|sync|push|send to|update|erp|sap|salesforce|servicenow|workday`).MatchString(lower) {
		sm := regexp.MustCompile(`(?i)post (?:it |the \w+ )?to (?:the )?([a-z0-9 ]+?)(?:\.|,| then| and|$)`).FindStringSubmatch(lower)
		system := "Target system"
		if len(sm) > 1 {
			system = util.TitleCase(strings.TrimSpace(sm[1]))
		} else if strings.Contains(lower, "erp") {
			system = "ERP"
		}
		add("integration.post", "Post to "+system, map[string]string{"system": system, "endpoint": util.Slug(system) + ".inbound", "mapping": "auto"}, 84, "Assumed "+system+" exposes a standard inbound API.")
	}

	if len(steps) <= 1 {
		add("script", "Process record", map[string]string{"runtime": "javascript"}, 60, "Could not infer detailed steps — placeholder.")
		add("notify", "Notify requester", map[string]string{"channel": "email", "recipients": "requester"}, 65)
	}

	name := entity + " Workflow"
	for _, k := range []struct{ kw, n string }{{"invoice", "Vendor Invoice Approval"}, {"onboard", "Employee Onboarding"}, {"purchase", "Purchase Request"}, {"expense", "Expense Approval"}, {"ticket", "Support Ticket Routing"}} {
		if strings.Contains(lower, k.kw) {
			name = k.n
			break
		}
	}
	overall := 0
	for _, s := range steps {
		overall += s.Confidence
	}
	overall /= len(steps)

	return GeneratedDraft{
		Name: name, Description: "Auto-generated from prompt.",
		Steps: steps, OverallConfidence: overall,
	}
}

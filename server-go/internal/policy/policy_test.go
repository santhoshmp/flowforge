package policy

import "testing"

func TestEgress(t *testing.T) {
	// unrestricted by default
	p := &Policy{}
	if !p.EgressAllowed("https://evil.example.com/x") {
		t.Error("expected unrestricted egress")
	}

	// allow-list => default deny, suffix match
	p = &Policy{Allow: []string{"api.openai.com", "internal.corp"}, DenyByDefault: true}
	cases := []struct {
		url  string
		want bool
	}{
		{"https://api.openai.com/v1/chat", true},
		{"https://sub.api.openai.com/v1", true},
		{"https://internal.corp/svc", true},
		{"https://evil.example.com/exfil", false},
		{"https://openai.com.evil.example.com/", false}, // suffix must not be spoofable
		{"not-a-url", false},
	}
	for _, c := range cases {
		if got := p.EgressAllowed(c.url); got != c.want {
			t.Errorf("EgressAllowed(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestSafeMode(t *testing.T) {
	if !(&Policy{}).ScriptAllowed() {
		t.Error("default should allow scripts")
	}
	if (&Policy{SafeMode: true}).ScriptAllowed() {
		t.Error("safe-mode should block scripts")
	}
}

func TestFromEnv(t *testing.T) {
	env := map[string]string{"FLOWFORGE_SAFE_MODE": "on", "FLOWFORGE_EGRESS_ALLOW": "api.openai.com, internal.corp"}
	p := FromEnv(func(k string) string { return env[k] })
	if !p.SafeMode || !p.DenyByDefault || len(p.Allow) != 2 {
		t.Errorf("unexpected policy: %+v", p)
	}
	if p.EgressAllowed("https://api.openai.com") == false {
		t.Error("openai should be allowed")
	}
}

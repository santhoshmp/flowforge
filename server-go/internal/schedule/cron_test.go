package schedule

// SCHED-01..03: the cron matcher (5-field, lists/ranges/steps/names).

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// SCHED-01: expressions parse and match the expected times.
func TestSCHED01_Match(t *testing.T) {
	cases := []struct {
		expr  string
		match []string
		not   []string
	}{
		{"0 6 * * 1-5", []string{"2026-09-14 06:00", "2026-09-16 06:00"}, []string{"2026-09-19 06:00", "2026-09-16 06:01", "2026-09-16 07:00"}}, // Mon/Wed, Sat
		{"*/15 * * * *", []string{"2026-09-16 06:00", "2026-09-16 06:45"}, []string{"2026-09-16 06:07", "2026-09-16 06:31"}},
		{"30 8 1 * *", []string{"2026-10-01 08:30"}, []string{"2026-10-02 08:30", "2026-10-01 08:31"}},
		{"0 0 1 1 *", []string{"2027-01-01 00:00"}, []string{"2027-01-02 00:00"}},
		{"0 9 * * mon,fri", []string{"2026-09-14 09:00", "2026-09-18 09:00"}, []string{"2026-09-16 09:00"}}, // Mon/Thu -> mon ok, thu no; wed no, fri ok
		{"5,35 * * * *", []string{"2026-09-16 07:05", "2026-09-16 07:35"}, []string{"2026-09-16 07:06"}},
		{"1-5 0 * * *", []string{"2026-09-16 00:03"}, []string{"2026-09-16 00:06"}},
	}
	for _, tc := range cases {
		c, err := Parse(tc.expr)
		if err != nil {
			t.Fatalf("%s: %v", tc.expr, err)
		}
		for _, m := range tc.match {
			if !c.Match(at(m)) {
				t.Errorf("%s: should match %s", tc.expr, m)
			}
		}
		for _, n := range tc.not {
			if c.Match(at(n)) {
				t.Errorf("%s: should NOT match %s", tc.expr, n)
			}
		}
	}
}

// SCHED-02: invalid expressions are rejected with useful errors.
func TestSCHED02_Invalid(t *testing.T) {
	for _, bad := range []string{"", "* * * *", "60 * * * *", "* 25 * * *", "*/0 * * * *", "5-1 * * * *", "a * * * *", "* * * * * *", "1/ * * * *"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("accepted invalid cron %q", bad)
		}
	}
}

// SCHED-03: Next returns the strictly-following match.
func TestSCHED03_Next(t *testing.T) {
	c, err := Parse("0 6 * * 1-5")
	if err != nil {
		t.Fatal(err)
	}
	next := c.Next(at("2026-09-16 06:00")) // Wed 06:00 already fired
	if next != at("2026-09-17 06:00") {
		t.Fatalf("next = %s, want Thu 06:00", next)
	}
	weekend := c.Next(at("2026-09-18 06:30")) // Fri after fire -> Mon
	if weekend != at("2026-09-21 06:00") {
		t.Fatalf("weekend next = %s, want Mon 06:00", weekend)
	}
}

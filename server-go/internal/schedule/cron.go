// Package schedule implements the minimal 5-field cron matcher used by
// scheduled triggers (minute hour dom month dow, server-local time).
// Supported syntax: `*`, `*/n`, `a-b`, `a-b/n`, comma-separated lists.
package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron is a parsed cron expression.
type Cron struct {
	expr    string
	minutes map[int]bool
	hours   map[int]bool
	doms    map[int]bool
	months  map[int]bool
	dows    map[int]bool
	domStar bool
	dowStar bool
}

type fieldSpec struct {
	min, max int
	names    map[string]int
}

var fields = []fieldSpec{
	{0, 59, nil}, // minute
	{0, 23, nil}, // hour
	{1, 31, nil}, // day of month
	{1, 12, map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}}, // month
	{0, 6, map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}},                                                       // day of week
}

// Parse validates and compiles a 5-field cron expression.
func Parse(expr string) (*Cron, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron must have 5 fields (minute hour dom month dow), got %d", len(parts))
	}
	c := &Cron{expr: expr}
	sets := []*map[int]bool{&c.minutes, &c.hours, &c.doms, &c.months, &c.dows}
	stars := []*bool{nil, nil, &c.domStar, nil, &c.dowStar}
	for i, p := range parts {
		set, err := parseField(p, fields[i])
		if err != nil {
			return nil, fmt.Errorf("field %d (%q): %v", i+1, p, err)
		}
		*sets[i] = set
		if stars[i] != nil && set == nil {
			*stars[i] = true
		}
	}
	return c, nil
}

// parseField returns the matched value set, or nil for a bare `*`.
func parseField(f string, spec fieldSpec) (map[int]bool, error) {
	if f == "*" {
		return nil, nil // nil = unrestricted
	}
	out := map[int]bool{}
	for _, part := range strings.Split(f, ",") {
		rangePart, stepStr := part, ""
		if i := strings.IndexByte(part, '/'); i >= 0 {
			rangePart, stepStr = part[:i], part[i+1:]
			if stepStr == "" {
				return nil, fmt.Errorf("missing step after /")
			}
		}
		step := 1
		if stepStr != "" {
			v, err := strconv.Atoi(stepStr)
			if err != nil || v < 1 {
				return nil, fmt.Errorf("invalid step %q", stepStr)
			}
			step = v
		}
		lo, hi := 0, 0
		if rangePart == "*" {
			lo, hi = spec.min, spec.max
		} else if i := strings.IndexByte(rangePart, '-'); i >= 0 {
			var err error
			if lo, err = parseValue(rangePart[:i], spec); err != nil {
				return nil, err
			}
			if hi, err = parseValue(rangePart[i+1:], spec); err != nil {
				return nil, err
			}
		} else {
			v, err := parseValue(rangePart, spec)
			if err != nil {
				return nil, err
			}
			lo, hi = v, v
		}
		if lo < spec.min || hi > spec.max || lo > hi {
			return nil, fmt.Errorf("value out of range %d-%d", spec.min, spec.max)
		}
		if step < 1 {
			return nil, fmt.Errorf("step must be >= 1")
		}
		for v := lo; v <= hi; v += step {
			out[v] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty field")
	}
	return out, nil
}

func parseValue(s string, spec fieldSpec) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}
	if spec.names != nil {
		if v, ok := spec.names[strings.ToLower(s)]; ok {
			return v, nil
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		if spec.names != nil {
			return 0, fmt.Errorf("not a number or name")
		}
		return 0, fmt.Errorf("not a number")
	}
	return v, nil
}

// Match reports whether t (truncated to the minute) satisfies the cron.
// Standard cron quirk: if both dom and dow are restricted, either may match.
func (c *Cron) Match(t time.Time) bool {
	if c.minutes != nil && !c.minutes[t.Minute()] {
		return false
	}
	if c.hours != nil && !c.hours[t.Hour()] {
		return false
	}
	if c.months != nil && !c.months[int(t.Month())] {
		return false
	}
	domOK := c.doms == nil || c.doms[t.Day()]
	dowOK := c.dows == nil || c.dows[int(t.Weekday())]
	if c.doms != nil && c.dows != nil {
		if !domOK && !dowOK {
			return false
		}
	} else if !domOK || !dowOK {
		return false
	}
	return true
}

// Next returns the first matching time strictly after t (may scan up to
// ~4 years for leap-day crons; fine for scheduler windows).
func (c *Cron) Next(after time.Time) time.Time {
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0)
	for t.Before(limit) {
		if c.Match(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

// String returns the original expression.
func (c *Cron) String() string { return c.expr }

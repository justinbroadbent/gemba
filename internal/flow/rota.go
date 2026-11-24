package flow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/justinbroadbent/gemba/internal/store"
)

// The triage lead is one person each week. Who it is comes from three things,
// most specific first:
//
//  1. a stand-in: whoever was handed the role during the current week;
//  2. a named week: a week the manager gave to a particular person;
//  3. the rotation: an ordered list of people, taking a week each.
//
// The rotation and the named weeks are the schedule, a small piece of text the
// store keeps (for GitHub, the body of the triage issue). The stand-in is the
// store's record of who holds the role and since when (for GitHub, the issue's
// assignee). Nothing rolls over on a timer: the lead is worked out each time.

// Schedule is the plan for who leads triage.
type Schedule struct {
	Rotation []string          `yaml:"rotation"`
	Starts   string            `yaml:"starts,omitempty"` // the Monday the first person in the rotation leads
	Weeks    map[string]string `yaml:"weeks,omitempty"`  // Monday (2006-01-02) to login
}

const day = "2006-01-02"

// WeekOf is the Monday that starts the week containing t, in UTC.
func WeekOf(t time.Time) time.Time {
	t = t.UTC()
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

var fenced = regexp.MustCompile("(?s)```(?:ya?ml)?\\s*\\n(.*?)```")

// ParseSchedule reads a schedule from text. The text may be the schedule alone
// or prose with the schedule in a fenced block, so the issue that holds it can
// also explain itself. Text with no schedule in it gives an empty one.
func ParseSchedule(text string) (Schedule, error) {
	var s Schedule
	if m := fenced.FindStringSubmatch(text); m != nil {
		text = m[1]
	} else if !strings.Contains(text, "rotation:") && !strings.Contains(text, "weeks:") {
		return s, nil
	}
	if err := yaml.Unmarshal([]byte(text), &s); err != nil {
		return Schedule{}, fmt.Errorf("the triage schedule could not be read: %w", err)
	}
	for week := range s.Weeks {
		if _, err := time.Parse(day, week); err != nil {
			return Schedule{}, fmt.Errorf("the triage schedule has a week that is not a date: %q", week)
		}
	}
	return s, nil
}

// Text writes the schedule the way ParseSchedule reads it, with a line for
// whoever opens the issue it is kept in.
func (s Schedule) Text() string {
	out, _ := yaml.Marshal(s)
	return "This issue holds the triage rota. Whoever it is assigned to is standing in this week.\n" +
		"Gemba keeps the schedule below; it can also be edited here.\n\n```yaml\n" + string(out) + "```\n"
}

// For is who the schedule gives a week to, and whether that week was named
// outright. Empty when the schedule says nothing.
func (s Schedule) For(week time.Time) (login string, named bool) {
	week = WeekOf(week)
	if l := s.Weeks[week.Format(day)]; l != "" {
		return l, true
	}
	if len(s.Rotation) == 0 {
		return "", false
	}
	start, err := time.Parse(day, s.Starts)
	if err != nil {
		start = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // a Monday; any fixed one gives a steady rotation
	}
	n := int(week.Sub(WeekOf(start)).Hours()/(7*24)) % len(s.Rotation)
	if n < 0 {
		n += len(s.Rotation)
	}
	return s.Rotation[n], false
}

// Prune drops named weeks that are over, so the schedule does not grow for ever.
func (s *Schedule) Prune(now time.Time) {
	this := WeekOf(now).Format(day)
	for week := range s.Weeks {
		if week < this {
			delete(s.Weeks, week)
		}
	}
}

// Lead is who leads triage now, and why.
type Lead struct {
	Login  string
	Since  time.Time
	Source string // "stand-in", "named" or "rotation"; "assigned" when there is no schedule
}

// LeadNow works out the triage lead. held is who the store says holds the role.
func LeadNow(s Schedule, held *store.Lead, now time.Time) *Lead {
	week := WeekOf(now)
	planned, named := s.For(now)
	switch {
	case held != nil && !held.Since.Before(week) && held.Login != planned:
		return &Lead{held.Login, held.Since, "stand-in"}
	case planned != "" && named:
		return &Lead{planned, week, "named"}
	case planned != "":
		return &Lead{planned, week, "rotation"}
	case held != nil:
		return &Lead{held.Login, held.Since, "assigned"}
	}
	return nil
}

// Upcoming lists who leads each of the next n weeks, starting with this one.
type Week struct {
	Start time.Time
	Login string
	Named bool
}

func (s Schedule) Upcoming(now time.Time, n int) []Week {
	out := make([]Week, 0, n)
	for i := 0; i < n; i++ {
		w := WeekOf(now).AddDate(0, 0, 7*i)
		login, named := s.For(w)
		out = append(out, Week{w, login, named})
	}
	return out
}

// sorted is here so tests and callers can rely on a steady order of named weeks.
func (s Schedule) NamedWeeks() []string {
	out := make([]string, 0, len(s.Weeks))
	for w := range s.Weeks {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

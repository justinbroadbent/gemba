package flow

import (
	"testing"
	"time"

	"github.com/justinbroadbent/gemba/internal/store"
)

// Wednesday 18 November 2026; its week starts Monday the 16th.
var wed = time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)

func TestTheRotationTakesAWeekEach(t *testing.T) {
	s := Schedule{Rotation: []string{"rin", "alex", "sam"}, Starts: "2026-11-16"}
	var got []string
	for _, w := range s.Upcoming(wed, 5) {
		got = append(got, w.Login)
	}
	if want := "rin alex sam rin alex"; join(got) != want {
		t.Errorf("rotation = %q, want %q", join(got), want)
	}
	if w := WeekOf(wed); w.Weekday() != time.Monday || w.Day() != 16 {
		t.Errorf("the week starts on Monday the 16th, got %v", w)
	}
	if before, _ := s.For(wed.AddDate(0, 0, -7)); before != "sam" {
		t.Errorf("the rotation also runs backwards from its start: %q", before)
	}
}

func TestANamedWeekAndAStandInOverrideTheRotation(t *testing.T) {
	s := Schedule{Rotation: []string{"rin", "alex"}, Starts: "2026-11-16", Weeks: map[string]string{"2026-11-23": "jo"}}
	if l := LeadNow(s, nil, wed); l.Login != "rin" || l.Source != "rotation" {
		t.Errorf("this week is the rotation's: %+v", l)
	}
	if l := LeadNow(s, nil, wed.AddDate(0, 0, 7)); l.Login != "jo" || l.Source != "named" {
		t.Errorf("next week was given to jo: %+v", l)
	}
	// Someone was handed the role last week: that is over, and the rotation resumes.
	old := &store.Lead{Login: "sam", Since: wed.AddDate(0, 0, -5)}
	if l := LeadNow(s, old, wed); l.Login != "rin" {
		t.Errorf("last week's stand-in does not carry over: %+v", l)
	}
	// Handed over on Tuesday of this week: sam stands in until the week ends.
	sub := &store.Lead{Login: "sam", Since: wed.AddDate(0, 0, -1)}
	if l := LeadNow(s, sub, wed); l.Login != "sam" || l.Source != "stand-in" {
		t.Errorf("this week's stand-in leads: %+v", l)
	}
	if l := LeadNow(s, sub, wed.AddDate(0, 0, 7)); l.Login != "jo" {
		t.Errorf("and the plan resumes the week after: %+v", l)
	}
	// With no schedule at all, whoever holds the role keeps it.
	if l := LeadNow(Schedule{}, old, wed); l.Login != "sam" || l.Source != "assigned" {
		t.Errorf("no schedule: %+v", l)
	}
	if LeadNow(Schedule{}, nil, wed) != nil {
		t.Error("nobody holds it and nothing is planned")
	}
}

func TestTheScheduleSurvivesBeingWrittenAndRead(t *testing.T) {
	s := Schedule{Rotation: []string{"rin", "alex"}, Starts: "2026-11-16", Weeks: map[string]string{"2026-11-23": "jo", "2026-11-02": "old"}}
	s.Prune(wed)
	if len(s.Weeks) != 1 {
		t.Errorf("weeks that are over should be dropped: %v", s.Weeks)
	}
	back, err := ParseSchedule(s.Text())
	if err != nil {
		t.Fatal(err)
	}
	if join(back.Rotation) != "rin alex" || back.Starts != "2026-11-16" || back.Weeks["2026-11-23"] != "jo" {
		t.Errorf("read back wrong: %+v", back)
	}
	if empty, err := ParseSchedule("Whoever this is assigned to leads triage."); err != nil || len(empty.Rotation) != 0 {
		t.Errorf("prose with no schedule is an empty schedule: %+v %v", empty, err)
	}
	if _, err := ParseSchedule("```yaml\nweeks:\n  next-tuesday: jo\n```"); err == nil {
		t.Error("a week that is not a date should be refused")
	}
}

func join(s []string) string {
	out := ""
	for n, x := range s {
		if n > 0 {
			out += " "
		}
		out += x
	}
	return out
}

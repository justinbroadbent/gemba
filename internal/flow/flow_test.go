package flow

import (
	"testing"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/store"
)

// A Wednesday afternoon.
var now = time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)

func ago(h float64) time.Time   { return now.Add(-time.Duration(h * float64(time.Hour))) }
func agoP(h float64) *time.Time { t := ago(h); return &t }
func rk(r float64) *float64     { return &r }

func workspace() *config.Workspace {
	return &config.Workspace{
		Slug:  "t",
		Cells: []config.Cell{{ID: "net", Name: "Network"}, {ID: "data", Name: "Data"}},
		People: []config.Person{
			{Login: "rin", Name: "Rin", Cell: "net", WIPLimit: 2},
			{Login: "ava", Name: "Ava", Cell: "data", WIPLimit: 2},
			{Login: "bo", Name: "Bo", Cell: "data", WIPLimit: 1},
		},
		Policies: config.Policies{
			DefaultWIPLimit: 2, TriageAfterHours: 8, UrgentPriorities: []string{"high", "urgent"},
			AgingDays: 5, ThroughputWeeks: 4, HorizonWeeks: 2,
		},
	}
}

func TestStage(t *testing.T) {
	cases := []struct {
		item store.Item
		want string
	}{
		{store.Item{Status: store.StatusOpen}, Intake},
		{store.Item{Status: store.StatusOpen, Cell: "net"}, Intake}, // pre-sorted but not triaged
		{store.Item{Status: store.StatusOpen, Cell: "net", COS: "standard"}, Queued},
		{store.Item{Status: store.StatusInProgress}, Doing},
		{store.Item{Status: store.StatusDone, COS: "standard"}, Done},
	}
	for _, c := range cases {
		if got := Stage(c.item); got != c.want {
			t.Errorf("Stage(%+v) = %s, want %s", c.item, got, c.want)
		}
	}
}

func TestLastStandupSkipsWeekend(t *testing.T) {
	monday := time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
	want := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC) // Friday
	if got := LastStandup(monday); !got.Equal(want) {
		t.Errorf("LastStandup(Monday) = %s, want %s", got, want)
	}
	if got := LastStandup(now); !got.Equal(now.AddDate(0, 0, -1)) {
		t.Errorf("LastStandup(Wednesday) = %s, want Tuesday", got)
	}
}

func TestInbox(t *testing.T) {
	ws := workspace()
	items := []store.Item{
		{ID: 1, Created: ago(2), Mirrored: true, ExtPriority: "low"},    // fresh: fast lane
		{ID: 2, Created: ago(20), Mirrored: true, ExtPriority: "low"},   // lingering
		{ID: 3, Created: ago(1), Mirrored: true, ExtPriority: "Urgent"}, // urgent, any age
		{ID: 4, Created: ago(1)}, // planned, not queued
		{ID: 5, Created: ago(50), Mirrored: true, Cell: "net", COS: "standard"}, // already queued
		{ID: 6, Created: ago(50), Mirrored: true, Status: store.StatusDone},     // resolved in the fast lane
	}
	got := Inbox(items, ws, now)
	var ids []int
	for _, r := range got {
		ids = append(ids, r.Item.ID)
	}
	want := []int{3, 2, 4} // urgent first, then oldest
	if len(ids) != len(want) {
		t.Fatalf("inbox = %v, want %v", ids, want)
	}
	for n := range want {
		if ids[n] != want[n] {
			t.Fatalf("inbox = %v, want %v", ids, want)
		}
	}
	if !got[0].Urgent || got[1].Urgent {
		t.Errorf("urgent flags wrong: %+v", got)
	}
	if n := FastLane(items, ws, now); n != 1 {
		t.Errorf("FastLane = %d, want 1", n)
	}
}

func TestQueueOrder(t *testing.T) {
	items := []store.Item{
		{ID: 1, Cell: "net", COS: "standard", Rank: rk(2)},
		{ID: 2, Cell: "net", COS: "standard"}, // no rank: last
		{ID: 3, Cell: "net", COS: "expedite", Rank: rk(9)},
		{ID: 4, Cell: "net", COS: "background", Rank: rk(1)},
		{ID: 5, Cell: "data", COS: "standard", Rank: rk(0)},
		{ID: 6, Cell: "net", COS: "standard", Rank: rk(1), Status: store.StatusInProgress},
	}
	q := Queue(items, "net")
	want := []int{3, 4, 1, 2}
	if len(q) != len(want) {
		t.Fatalf("queue has %d items, want %d", len(q), len(want))
	}
	for n, id := range want {
		if q[n].ID != id {
			t.Errorf("queue[%d] = #%d, want #%d", n, q[n].ID, id)
		}
	}
}

func TestLaneCapacityLine(t *testing.T) {
	ws := workspace()
	var items []store.Item
	id := 0
	add := func(i store.Item) { id++; i.ID = id; items = append(items, i) }
	for n := 0; n < 8; n++ { // 8 triaged items finished in 4 weeks: 2 a week
		add(store.Item{Cell: "data", COS: "standard", Status: store.StatusDone, Closed: agoP(float64(30 + n*72))})
	}
	for n := 0; n < 4; n++ { // quick tickets: 1 a week
		add(store.Item{Cell: "data", Mirrored: true, Status: store.StatusDone, Closed: agoP(float64(40 + n*100))})
	}
	add(store.Item{Cell: "data", COS: "standard", Status: store.StatusDone, Closed: agoP(24 * 40)}) // outside the window
	add(store.Item{Cell: "data", COS: "standard", Status: store.StatusDone, Closed: agoP(3)})       // since last standup
	add(store.Item{Cell: "data", Mirrored: true, Status: store.StatusDone, Closed: agoP(2)})        // quick, since last standup
	add(store.Item{Cell: "data", COS: "standard", Status: store.StatusInProgress, Assignee: "bo", Started: agoP(24 * 7)})
	add(store.Item{Cell: "data", COS: "standard", Status: store.StatusInProgress, Assignee: "bo", Started: agoP(5),
		BlockedReason: "Waiting on vendor"})
	for n := 0; n < 5; n++ {
		add(store.Item{Cell: "data", COS: "standard", Rank: rk(float64(n))})
	}

	l := BuildLane(items, ws, *ws.Cell("data"), now)
	if l.Throughput != 2.25 { // 8 + the one finished today, over 4 weeks
		t.Errorf("throughput = %v, want 2.25", l.Throughput)
	}
	if l.QuickRate != 1.25 {
		t.Errorf("quick rate = %v, want 1.25", l.QuickRate)
	}
	// round(2.25 * 2) = 5 (round half away from zero on 4.5), minus 2 in progress.
	if l.Line != 3 {
		t.Errorf("line = %d, want 3", l.Line)
	}
	if len(l.Above()) != 3 || len(l.Queue) != 5 {
		t.Errorf("above = %d of %d, want 3 of 5", len(l.Above()), len(l.Queue))
	}
	if len(l.Done) != 1 || l.QuickDone != 1 {
		t.Errorf("done = %d, quick done = %d, want 1 and 1", len(l.Done), l.QuickDone)
	}
	if l.WIPLimit != 3 || l.Blocked() != 1 {
		t.Errorf("wip limit = %d, blocked = %d, want 3 and 1", l.WIPLimit, l.Blocked())
	}
	var bo Load
	for _, p := range l.People {
		if p.Person.Login == "bo" {
			bo = p
		}
	}
	if !bo.Over() || len(bo.Items) != 2 {
		t.Errorf("bo should be over the limit with 2 items, got %d", len(bo.Items))
	}
	if !IsAging(l.Doing[0], ws, now) || IsAging(l.Doing[1], ws, now) {
		t.Errorf("aging flags wrong")
	}
}

func TestLaneWithNoHistory(t *testing.T) {
	ws := workspace()
	items := []store.Item{{ID: 1, Cell: "net", COS: "standard", Rank: rk(1)}}
	l := BuildLane(items, ws, *ws.Cell("net"), now)
	if l.Measured || l.Line != 0 || len(l.Above()) != 0 {
		t.Errorf("a cell with no history should be unmeasured with nothing above the line: %+v", l)
	}
	if len(l.People) != 1 || l.WIPLimit != 2 {
		t.Errorf("a one-person cell should work: %+v", l.People)
	}
}

func TestLineLongerThanQueue(t *testing.T) {
	ws := workspace()
	var items []store.Item
	for n := 0; n < 20; n++ {
		items = append(items, store.Item{ID: n + 1, Cell: "net", COS: "standard", Status: store.StatusDone, Closed: agoP(float64(n * 24))})
	}
	items = append(items, store.Item{ID: 99, Cell: "net", COS: "standard"})
	l := BuildLane(items, ws, *ws.Cell("net"), now)
	if l.Line != 10 || len(l.Above()) != 1 {
		t.Errorf("line = %d, above = %d; want 10 and 1", l.Line, len(l.Above()))
	}
}

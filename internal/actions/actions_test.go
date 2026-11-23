package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/flow"
	"github.com/justinbroadbent/gemba/internal/store"
)

var (
	now = time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)
	ctx = context.Background()
)

func setup(t *testing.T) (*store.DemoStore, *config.Workspace) {
	t.Helper()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	ws := cfg.Workspaces[0]
	var cells []store.DemoCell
	for _, c := range ws.Cells {
		dc := store.DemoCell{ID: c.ID}
		for _, p := range ws.People {
			if p.Cell == c.ID {
				dc.People = append(dc.People, p.Login)
			}
		}
		cells = append(cells, dc)
	}
	return store.NewDemo(cells, ws.Policies.ThroughputWeeks, now), ws
}

func items(t *testing.T, st store.Store) []store.Item {
	t.Helper()
	all, err := st.Items(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func get(t *testing.T, st store.Store, id int) store.Item {
	t.Helper()
	for _, i := range items(t, st) {
		if i.ID == id {
			return i
		}
	}
	t.Fatalf("no item #%d", id)
	return store.Item{}
}

func ids(q []store.Item) []int {
	out := make([]int, len(q))
	for n, i := range q {
		out[n] = i.ID
	}
	return out
}

func isConflict(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Conflict
}

func TestTriageQueuesAtFrontOrBack(t *testing.T) {
	st, ws := setup(t)
	inbox := flow.Inbox(items(t, st), ws, now)
	if len(inbox) < 2 {
		t.Fatalf("demo data should have an inbox, got %d", len(inbox))
	}
	a, b := inbox[0].Item.ID, inbox[1].Item.ID
	if err := Triage(ctx, st, ws, a, "data", "standard", false); err != nil {
		t.Fatal(err)
	}
	if err := Triage(ctx, st, ws, b, "data", "fixed-date", true); err != nil {
		t.Fatal(err)
	}
	q := ids(flow.Queue(items(t, st), "data"))
	if q[0] != b || q[len(q)-1] != a {
		t.Errorf("queue = %v; want #%d first and #%d last", q, b, a)
	}
	if err := Triage(ctx, st, ws, a, "data", "standard", false); !isConflict(err) {
		t.Errorf("triaging twice should conflict, got %v", err)
	}
	if err := Triage(ctx, st, ws, inbox[2].Item.ID, "nowhere", "standard", false); err == nil {
		t.Error("unknown cell should be refused")
	}
}

func TestMoveWritesOneRank(t *testing.T) {
	st, ws := setup(t)
	before := flow.Queue(items(t, st), "platform")
	last, second := before[len(before)-1], before[1]
	if err := Move(ctx, st, ws, "platform", last.ID, &second.ID); err != nil {
		t.Fatal(err)
	}
	after := flow.Queue(items(t, st), "platform")
	if after[1].ID != last.ID || after[2].ID != second.ID {
		t.Errorf("queue = %v; want #%d in second place", ids(after), last.ID)
	}
	changed := 0
	for _, i := range after {
		for _, old := range before {
			if old.ID == i.ID && *old.Rank != *i.Rank {
				changed++
			}
		}
	}
	if changed != 1 {
		t.Errorf("%d ranks changed, want 1", changed)
	}

	// To the end.
	if err := Move(ctx, st, ws, "platform", after[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	end := flow.Queue(items(t, st), "platform")
	if end[len(end)-1].ID != after[0].ID {
		t.Errorf("queue = %v; want #%d last", ids(end), after[0].ID)
	}
}

func TestMoveRefusesStaleQueue(t *testing.T) {
	st, ws := setup(t)
	q := flow.Queue(items(t, st), "network")
	gone := 99999
	if err := Move(ctx, st, ws, "network", q[0].ID, &gone); !isConflict(err) {
		t.Errorf("moving before a missing item should conflict, got %v", err)
	}
	if err := Move(ctx, st, ws, "network", gone, nil); !isConflict(err) {
		t.Errorf("moving a missing item should conflict, got %v", err)
	}
}

func TestMoveRenumbersWhenThereIsNoGap(t *testing.T) {
	st, ws := setup(t)
	q := flow.Queue(items(t, st), "network")
	same := 5.0
	for _, i := range q {
		if err := st.Update(ctx, i.ID, store.Change{Rank: &same}); err != nil {
			t.Fatal(err)
		}
	}
	q = flow.Queue(items(t, st), "network")
	if err := Move(ctx, st, ws, "network", q[3].ID, &q[1].ID); err != nil {
		t.Fatal(err)
	}
	after := flow.Queue(items(t, st), "network")
	if after[1].ID != q[3].ID {
		t.Errorf("queue = %v; want #%d second", ids(after), q[3].ID)
	}
	for n, i := range after {
		if *i.Rank != float64(n+1) {
			t.Errorf("rank of position %d = %v, want %d", n, *i.Rank, n+1)
		}
	}
}

func TestDecideWithoutPausePushesOneItemPastTheLine(t *testing.T) {
	st, ws := setup(t)
	lane := flow.BuildLane(items(t, st), ws, *ws.Cell("platform"), now)
	if lane.Line < 1 || lane.Line >= len(lane.Queue) {
		t.Fatalf("demo platform lane should have a line inside the queue, got %d of %d", lane.Line, len(lane.Queue))
	}
	newcomer := lane.Queue[len(lane.Queue)-1]
	falls := lane.Queue[lane.Line-1]

	notes, err := Decide(ctx, st, ws, newcomer.ID, "platform", nil, "Dana (director)", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].ID != falls.ID || notes[1].ID != newcomer.ID {
		t.Fatalf("notes = %+v; want one on #%d then one on #%d", notes, falls.ID, newcomer.ID)
	}
	if !strings.Contains(notes[1].Text, "Dana (director)") {
		t.Errorf("the note should name who authorized it: %q", notes[1].Text)
	}
	after := flow.BuildLane(items(t, st), ws, *ws.Cell("platform"), now)
	if after.Queue[0].ID != newcomer.ID {
		t.Errorf("#%d should be first, queue = %v", newcomer.ID, ids(after.Queue))
	}
	if after.Queue[after.Line].ID != falls.ID {
		t.Errorf("#%d should now be the first item past the line", falls.ID)
	}
	if got := get(t, st, newcomer.ID).COS; got != newcomer.COS {
		t.Errorf("class of service changed to %s without a pause", got)
	}
}

func TestDecideWithPause(t *testing.T) {
	st, ws := setup(t)
	lane := flow.BuildLane(items(t, st), ws, *ws.Cell("data"), now)
	var planned, mirrored *store.Item
	for n := range lane.Doing {
		if lane.Doing[n].Mirrored && mirrored == nil {
			mirrored = &lane.Doing[n]
		}
		if !lane.Doing[n].Mirrored && planned == nil {
			planned = &lane.Doing[n]
		}
	}
	if planned == nil || mirrored == nil {
		t.Fatal("demo data should have both planned and mirrored work in progress")
	}
	a, b := lane.Queue[3], lane.Queue[4]

	notes, err := Decide(ctx, st, ws, a.ID, "data", &planned.ID, "Morgan", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("want a note on each item, got %+v", notes)
	}
	if got := get(t, st, a.ID); got.COS != "expedite" {
		t.Errorf("the new item should be an expedite, got %s", got.COS)
	}
	if got := get(t, st, planned.ID); got.Status != store.StatusOpen {
		t.Errorf("a paused planned item goes back to open, got %s", got.Status)
	}
	q := ids(flow.Queue(items(t, st), "data"))
	if q[0] != a.ID || q[1] != planned.ID {
		t.Errorf("queue = %v; want the expedite first and the paused item next", q)
	}

	// A mirrored item's status belongs to the ticketing system: Gemba only asks.
	notes, err = Decide(ctx, st, ws, b.ID, "data", &mirrored.ID, "Morgan", now)
	if err != nil {
		t.Fatal(err)
	}
	if got := get(t, st, mirrored.ID); got.Status != store.StatusInProgress {
		t.Errorf("Gemba must not change a mirrored item's status, got %s", got.Status)
	}
	if !strings.Contains(notes[1].Text, "ticketing system") {
		t.Errorf("the note should ask for the ticket to be set back: %q", notes[1].Text)
	}
}

func TestDecideGuards(t *testing.T) {
	st, ws := setup(t)
	lane := flow.BuildLane(items(t, st), ws, *ws.Cell("data"), now)
	if _, err := Decide(ctx, st, ws, lane.Queue[0].ID, "data", nil, "  ", now); err == nil {
		t.Error("a decision with nobody authorizing it should be refused")
	}
	if _, err := Decide(ctx, st, ws, lane.Doing[0].ID, "data", nil, "Morgan", now); err == nil {
		t.Error("an item already in progress cannot be traded in")
	}
	other := flow.BuildLane(items(t, st), ws, *ws.Cell("network"), now).Doing[0].ID
	if _, err := Decide(ctx, st, ws, lane.Queue[0].ID, "data", &other, "Morgan", now); !isConflict(err) {
		t.Errorf("pausing work from another cell should conflict, got %v", err)
	}
	if len(st.Comments) != 0 {
		t.Errorf("refused decisions must not write comments: %v", st.Comments)
	}
}

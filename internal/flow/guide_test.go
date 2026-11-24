package flow

import (
	"strings"
	"testing"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/store"
)

func guideWorkspace() *config.Workspace {
	ws := workspace()
	ws.Kinds = []config.Kind{{ID: "enterprise", Name: "Enterprise project", Project: true}, {ID: "maintenance", Name: "Maintenance"}}
	ws.Policies.QuietDays = 3
	return ws
}

func in(h float64) *time.Time { t := now.Add(time.Duration(h * float64(time.Hour))); return &t }

func TestTypeOf(t *testing.T) {
	ws := guideWorkspace()
	cases := []struct {
		item store.Item
		want string
	}{
		{store.Item{Mirrored: true}, Unplanned},                      // from the ticketing system
		{store.Item{Mirrored: true, Kind: "maintenance"}, Unplanned}, // and it stays unplanned
		{store.Item{Kind: "enterprise", COS: "expedite"}, Unplanned}, // planned, then expedited
		{store.Item{Kind: "enterprise", COS: "standard"}, "enterprise"},
		{store.Item{Kind: "nonsense"}, Other},
		{store.Item{}, Other},
	}
	for _, c := range cases {
		if got := TypeOf(c.item, ws); got != c.want {
			t.Errorf("TypeOf(%+v) = %s, want %s", c.item, got, c.want)
		}
	}
}

func TestMixCountsQuickTicketsAsUnplanned(t *testing.T) {
	ws := guideWorkspace()
	items := []store.Item{
		{ID: 1, Cell: "data", Mirrored: true, Status: store.StatusDone, Closed: agoP(10)},                  // quick ticket
		{ID: 2, Cell: "data", Mirrored: true, COS: "standard", Status: store.StatusDone, Closed: agoP(20)}, // triaged ticket
		{ID: 3, Cell: "data", Kind: "maintenance", COS: "standard", Status: store.StatusDone, Closed: agoP(30)},
		{ID: 4, Cell: "data", Kind: "enterprise", COS: "standard", Status: store.StatusDone, Closed: agoP(40)},
		{ID: 5, Cell: "data", Kind: "enterprise", COS: "standard", Status: store.StatusDone, Closed: agoP(24 * 60)}, // too old
		{ID: 6, Cell: "net", Mirrored: true, Status: store.StatusDone, Closed: agoP(10)},                            // another cell
	}
	mix := Mix(items, ws, "data", now)
	want := map[string]int{Unplanned: 2, "enterprise": 1, "maintenance": 1}
	if len(mix) != 3 {
		t.Fatalf("mix = %+v, want unplanned and the two kinds", mix)
	}
	for _, s := range mix {
		if s.Count != want[s.ID] {
			t.Errorf("%s = %d, want %d", s.ID, s.Count, want[s.ID])
		}
	}
	if mix[0].ID != Unplanned || mix[0].Pct != 50 {
		t.Errorf("unplanned should come first at 50%%, got %+v", mix[0])
	}
}

// A lane finishing 2 a week, with one of two hands free.
func guideLane(t *testing.T, queue ...store.Item) ([]store.Item, Lane, Norms) {
	t.Helper()
	ws := guideWorkspace()
	var items []store.Item
	for n := 0; n < 8; n++ {
		items = append(items, store.Item{ID: 100 + n, Cell: "net", Kind: "maintenance", COS: "standard", Status: store.StatusDone,
			Started: agoP(float64(40 + n*72)), Closed: agoP(float64(16 + n*72))}) // each took a day
	}
	items = append(items, store.Item{ID: 50, Cell: "net", Kind: "maintenance", COS: "standard", Status: store.StatusInProgress,
		Assignee: "rin", Started: agoP(5), Updated: agoP(2)})
	items = append(items, queue...)
	return items, BuildLane(items, ws, *ws.Cell("net"), now), BuildNorms(items, ws, now)
}

func TestOutlooksTurnRankIntoTime(t *testing.T) {
	_, lane, norms := guideLane(t,
		store.Item{ID: 1, Cell: "net", Kind: "maintenance", COS: "standard", Rank: rk(1)},
		store.Item{ID: 2, Cell: "net", Kind: "maintenance", COS: "standard", Rank: rk(2)},
		store.Item{ID: 3, Cell: "net", Kind: "maintenance", COS: "fixed-date", Rank: rk(3), Due: in(24 * 5)},
		store.Item{ID: 4, Cell: "net", Kind: "maintenance", COS: "fixed-date", Rank: rk(4), Due: in(24 * 30)},
	)
	o := Outlooks(lane, guideWorkspace(), norms, now)
	if o[0].Wait != 0 || !o[0].Known {
		t.Errorf("the free hand takes the top of the queue now, got %+v", o[0])
	}
	if week := 7 * 24 * time.Hour; o[1].Wait != week/2 || o[2].Wait != week {
		t.Errorf("at 2 a week the next two start after half a week and a week, got %v and %v", o[1].Wait, o[2].Wait)
	}
	// Item 3 is due in 5 days and takes about a day, so it must start within 4. It starts in 7.
	if !o[2].AtRisk || o[3].AtRisk {
		t.Errorf("only the item due in 5 days is at risk: %+v %+v", o[2], o[3])
	}
}

func TestOutlookUnknownWithoutAPace(t *testing.T) {
	ws := guideWorkspace()
	items := []store.Item{
		{ID: 1, Cell: "net", COS: "standard", Rank: rk(1)}, {ID: 2, Cell: "net", COS: "standard", Rank: rk(2)},
		{ID: 3, Cell: "net", COS: "standard", Rank: rk(3)},
	}
	lane := BuildLane(items, ws, *ws.Cell("net"), now)
	o := Outlooks(lane, ws, BuildNorms(items, ws, now), now)
	if !o[0].Known || !o[1].Known || o[2].Known {
		t.Errorf("two free hands take two items; the third cannot be forecast: %+v", o)
	}
}

func TestNormsCompareLikeWithLike(t *testing.T) {
	ws := guideWorkspace()
	items, _, norms := guideLane(t)
	maint := store.Item{Kind: "maintenance", COS: "standard", Status: store.StatusInProgress, Started: agoP(48)}
	if aging, usual, measured := norms.Aging(maint, ws, now); !aging || !measured || usual != 24*time.Hour {
		t.Errorf("maintenance usually takes a day here, so two days is aging: %v %v %v", aging, usual, measured)
	}
	// No enterprise work has finished, so the fixed policy of 5 days applies.
	ent := store.Item{Kind: "enterprise", COS: "standard", Status: store.StatusInProgress, Started: agoP(48)}
	if aging, usual, measured := norms.Aging(ent, ws, now); aging || measured || usual != 5*24*time.Hour {
		t.Errorf("with no history the policy applies: %v %v %v", aging, usual, measured)
	}
	_ = items
}

func TestPullsGiveEachFreeHandADifferentItem(t *testing.T) {
	ws := guideWorkspace()
	items := []store.Item{
		{ID: 1, Cell: "data", COS: "standard", Rank: rk(1), Assignee: "bo"}, // already Bo's
		{ID: 2, Cell: "data", COS: "standard", Rank: rk(2)},
		{ID: 3, Cell: "data", COS: "standard", Rank: rk(3)},
	}
	pulls := Pulls(BuildLane(items, ws, *ws.Cell("data"), now))
	if pulls["ava"].ID != 2 || pulls["bo"].ID != 1 {
		t.Errorf("Ava takes the top unclaimed item and Bo his own: %+v", pulls)
	}
	items = append(items, store.Item{ID: 4, Status: store.StatusInProgress, Assignee: "bo"})
	if _, ok := Pulls(BuildLane(items, ws, *ws.Cell("data"), now))["bo"]; ok {
		t.Error("Bo's one hand is full, so nothing is suggested for him")
	}
}

func TestAttentionListsOnlyExceptions(t *testing.T) {
	ws := guideWorkspace()
	items, _, norms := guideLane(t,
		store.Item{ID: 1, Title: "Calm", Cell: "net", Kind: "maintenance", COS: "standard", Rank: rk(1)},
		store.Item{ID: 2, Title: "Rush", Cell: "net", COS: "expedite", Rank: rk(9)},
		store.Item{ID: 3, Title: "Dated", Cell: "net", Kind: "maintenance", COS: "fixed-date", Rank: rk(5), Due: in(24)},
	)
	items = append(items,
		store.Item{ID: 60, Title: "Stuck", Cell: "data", COS: "standard", Status: store.StatusInProgress, Assignee: "ava",
			Started: agoP(30), Updated: agoP(1), BlockedReason: "Waiting on vendor", BlockedSince: agoP(26)},
		store.Item{ID: 61, Title: "Silent", Cell: "data", COS: "standard", Status: store.StatusInProgress, Assignee: "ava",
			Started: agoP(100), Updated: agoP(90)},
	)
	alerts := Attention(items, ws, norms, now)
	var got []string
	for _, a := range alerts {
		got = append(got, a.Text)
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{"Expedited and not started: Rush", "Likely to miss", "Blocked 26 h: Stuck. Waiting on vendor (Ava)", "No activity for 4 d: Silent"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if strings.Contains(all, "Calm") {
		t.Errorf("work that is fine must not be listed:\n%s", all)
	}
	if !strings.HasPrefix(got[0], "Expedited") {
		t.Errorf("the most serious comes first, got %q", got[0])
	}
}

func TestQuickWorkStaysOffTheRankedQueue(t *testing.T) {
	ws := guideWorkspace()
	ws.Policies.QuickDays = 2
	items := []store.Item{
		{ID: 1, Title: "Ranked", Cell: "net", COS: "standard", Rank: rk(1)},
		{ID: 2, Title: "Fresh quick", Cell: "net", COS: "quick", Created: now.Add(-3 * time.Hour)},
		{ID: 3, Title: "Old quick", Cell: "net", COS: "quick", Created: now.Add(-72 * time.Hour)},
		{ID: 4, Cell: "net", COS: "quick", Status: store.StatusDone, Closed: agoP(5)},
	}
	lane := BuildLane(items, ws, *ws.Cell("net"), now)
	if len(lane.Queue) != 1 || lane.Queue[0].ID != 1 {
		t.Errorf("only ranked work is in the queue: %+v", lane.Queue)
	}
	if len(lane.Quick) != 2 || lane.Quick[0].ID != 3 {
		t.Errorf("the quick list holds both, oldest first: %+v", lane.Quick)
	}
	if lane.Throughput != 0 || lane.QuickRate == 0 {
		t.Errorf("finished quick work does not set the pace of the ranked queue: %v %v", lane.Throughput, lane.QuickRate)
	}
	var texts []string
	for _, a := range Attention(items, ws, BuildNorms(items, ws, now), now) {
		texts = append(texts, a.Text)
	}
	all := strings.Join(texts, "\n")
	if !strings.Contains(all, "Quick, but waiting 3 d: Old quick") || strings.Contains(all, "Fresh quick") {
		t.Errorf("only the quick item nobody took is flagged:\n%s", all)
	}
}

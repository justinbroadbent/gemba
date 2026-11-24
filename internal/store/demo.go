package store

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// DemoCell describes a cell to seed: its id and the logins of its people.
type DemoCell struct {
	ID     string
	People []string
}

var demoVerbs = map[string][]string{
	"network": {"Replace failing uplink on", "Add VLAN for", "Review firewall rules for", "Renew certificate on",
		"Tune routing timers on", "Open port request for", "Audit access lists on", "Upgrade firmware on"},
	"platform": {"Patch", "Rebuild", "Automate provisioning of", "Expand disk on", "Migrate", "Fix failing pipeline for",
		"Add monitoring to", "Rotate secrets for", "Decommission", "Harden"},
	"data": {"Tune slow query on", "Restore test copy of", "Add index to", "Grant read access to", "Upgrade",
		"Review backup job for", "Rotate credentials for", "Resize storage for", "Set up replica for"},
}

var demoNouns = map[string][]string{
	"network": {"core switch B", "the branch VPN", "the DMZ segment", "the guest Wi-Fi", "edge router 2",
		"the warehouse link", "the load balancer pair"},
	"platform": {"the build agents", "the intranet web farm", "the file services cluster", "the identity sync host",
		"the patching pipeline", "the container registry", "the jump hosts", "the print servers"},
	"data": {"the orders database", "the reporting replica", "the HR database", "the warehouse cluster",
		"the staging SQL instance", "the audit database", "the billing database"},
}

var demoBlocks = []string{"Waiting on requester", "Waiting on vendor", "Waiting on another team", "Waiting for change window"}

// DemoStore is an in-memory store with believable, fictional data. It resets on restart.
type DemoStore struct {
	mu       sync.Mutex
	items    map[int]*Item
	order    []int
	Comments []string
	lead     *Lead
	schedule string
}

func (s *DemoStore) TriageSchedule(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.schedule, nil
}

func (s *DemoStore) SetTriageSchedule(_ context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.schedule = text
	return nil
}

func (s *DemoStore) TriageLead(context.Context) (*Lead, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lead == nil {
		return nil, nil
	}
	l := *s.lead
	return &l, nil
}

func (s *DemoStore) SetTriageLead(_ context.Context, login string) error {
	return s.SeedTriageLead(login, time.Now().UTC())
}

// SeedTriageLead sets the lead as of a given time.
func (s *DemoStore) SeedTriageLead(login string, since time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lead = &Lead{Login: login, Since: since}
	return nil
}

// NewDemo seeds a floor for the given cells, with timestamps relative to now.
func NewDemo(cells []DemoCell, throughputWeeks int, now time.Time) *DemoStore {
	rnd := rand.New(rand.NewSource(7))
	s := &DemoStore{items: map[int]*Item{}}
	days := func(lo, hi float64) time.Duration {
		return time.Duration((lo + rnd.Float64()*(hi-lo)) * 24 * float64(time.Hour))
	}
	flavour := func(cell string) string {
		if _, ok := demoVerbs[cell]; ok {
			return cell
		}
		return "platform"
	}
	add := func(cell, flavourOf string, created time.Time, set func(*Item)) *Item {
		f := flavour(flavourOf)
		i := &Item{
			ID:      101 + len(s.order),
			Title:   demoVerbs[f][rnd.Intn(len(demoVerbs[f]))] + " " + demoNouns[f][rnd.Intn(len(demoNouns[f]))],
			Created: created, Status: StatusOpen, Cell: cell,
		}
		if set != nil {
			set(i)
		}
		s.items[i.ID] = i
		s.order = append(s.order, i.ID)
		return i
	}
	at := func(t time.Time) *time.Time { return &t }

	history := map[string][2]int{"network": {2, 3}, "platform": {7, 10}, "data": {6, 8}}
	queued := map[string]int{"network": 7, "platform": 10, "data": 9}
	wip := []int{2, 1, 3, 2, 1}

	for _, c := range cells {
		pick := func() string {
			if len(c.People) == 0 {
				return ""
			}
			return c.People[rnd.Intn(len(c.People))]
		}
		h, ok := history[c.ID]
		if !ok {
			h = [2]int{2 * len(c.People), 3 * len(c.People)}
		}
		for week := 0; week < throughputWeeks; week++ {
			base := time.Duration(week) * 7 * 24 * time.Hour
			for n := 0; n < h[0]; n++ {
				closed := now.Add(-base - days(0.1, 6.9))
				started := closed.Add(-days(0.5, 6))
				add(c.ID, c.ID, started.Add(-days(1, 8)), func(i *Item) {
					i.Status, i.COS, i.Assignee = StatusDone, "standard", pick()
					i.Mirrored = rnd.Float64() < 0.6
					i.Started, i.Closed = at(started), at(closed)
				})
			}
			for n := 0; n < h[1]; n++ {
				created := now.Add(-base - days(0.2, 6.9))
				add(c.ID, c.ID, created, func(i *Item) {
					i.Status, i.Assignee, i.Mirrored, i.ExtPriority = StatusDone, pick(), true, "low"
					i.Closed = at(created.Add(days(0.01, 0.12)))
				})
			}
		}

		n := 0
		for k, login := range c.People {
			count := wip[k%len(wip)]
			if len(c.People) == 1 {
				count = 3
			}
			for j := 0; j < count; j++ {
				started := now.Add(-days(0.2, 6.5))
				item := add(c.ID, c.ID, started.Add(-days(1, 6)), func(i *Item) {
					i.Status, i.COS, i.Assignee = StatusInProgress, "standard", login
					i.Mirrored = rnd.Float64() < 0.5
					i.Started = at(started)
				})
				if n++; n%4 == 0 {
					item.BlockedReason = demoBlocks[rnd.Intn(len(demoBlocks))]
					since := now.Add(-days(0.2, 4))
					if since.Before(started) {
						since = started
					}
					item.BlockedSince = at(since)
				}
			}
		}

		size, ok := queued[c.ID]
		if !ok {
			size = 5
		}
		for r := 1; r <= size; r++ {
			rank := float64(r)
			add(c.ID, c.ID, now.Add(-days(1, 30)), func(i *Item) {
				i.Rank = &rank
				switch x := rnd.Intn(10); {
				case x < 6:
					i.COS, i.Mirrored = "standard", rnd.Float64() < 0.5
				case x < 8:
					i.COS, i.Mirrored = "fixed-date", rnd.Float64() < 0.5
					i.Due = at(now.Add(time.Duration(3+rnd.Intn(18)) * 24 * time.Hour))
				default:
					i.COS = "background"
				}
			})
		}
	}

	// Older history, so a look back over months has something to show. It has
	// its own random source, which leaves the floor above unchanged.
	old := rand.New(rand.NewSource(11))
	for _, c := range cells {
		h, ok := history[c.ID]
		if !ok {
			h = [2]int{2 * len(c.People), 3 * len(c.People)}
		}
		f := flavour(c.ID)
		for week := throughputWeeks; week < 30; week++ {
			count := h[0] + h[1] - 2 + old.Intn(5)
			for n := 0; n < count; n++ {
				closed := now.Add(-time.Duration((float64(week*7) + old.Float64()*6.9) * 24 * float64(time.Hour)))
				i := &Item{
					ID:     101 + len(s.order),
					Title:  demoVerbs[f][old.Intn(len(demoVerbs[f]))] + " " + demoNouns[f][old.Intn(len(demoNouns[f]))],
					Status: StatusDone, Cell: c.ID, Mirrored: true, ExtPriority: "low", Closed: at(closed),
					Created: closed.Add(-2 * time.Hour),
				}
				if len(c.People) > 0 {
					i.Assignee = c.People[old.Intn(len(c.People))]
				}
				if n < h[0] { // worked through a queue, not a quick ticket
					started := closed.Add(-time.Duration((0.5 + old.Float64()*old.Float64()*14) * 24 * float64(time.Hour)))
					i.COS, i.Mirrored, i.Started = "standard", old.Float64() < 0.5, at(started)
					i.Created = started.Add(-72 * time.Hour)
					if old.Intn(6) == 0 {
						i.COS, i.Due = "fixed-date", at(closed.Add(time.Duration(old.Intn(9)-2)*24*time.Hour))
					}
				}
				s.items[i.ID] = i
				s.order = append(s.order, i.ID)
			}
		}
	}

	// Planned work gets a kind, and project work a project. A separate random
	// source keeps the rest of the floor identical from run to run.
	kinds := rand.New(rand.NewSource(3))
	projects := map[string][]string{
		"enterprise": {"ERP migration", "Zero-trust rollout"},
		"department": {"Backup modernisation", "Patch automation"},
	}
	for _, id := range s.order {
		i := s.items[id]
		if i.Status == StatusInProgress { // most work was touched recently; some has gone quiet
			touched := now.Add(-time.Duration(kinds.Float64()*kinds.Float64()*6*24) * time.Hour)
			if touched.Before(*i.Started) {
				touched = *i.Started
			}
			i.Updated = &touched
		}
		if i.Mirrored || i.COS == "" {
			continue
		}
		switch x := kinds.Intn(10); {
		case x < 3:
			i.Kind = "enterprise"
		case x < 6:
			i.Kind = "department"
		default:
			i.Kind = "maintenance"
		}
		if names, ok := projects[i.Kind]; ok {
			i.Project = names[kinds.Intn(len(names))]
			if i.Kind == "enterprise" {
				i.ProjectURL = "https://example.com/epics/" + strings.ReplaceAll(strings.ToLower(i.Project), " ", "-")
			}
		}
	}

	if len(cells) == 0 {
		return s
	}
	any := func() string { return cells[rnd.Intn(len(cells))].ID }
	low := []string{"low", "medium"}
	for n := 0; n < 5; n++ { // fresh tickets, still in the fast lane
		c := any()
		add(c, c, now.Add(-days(0.02, 0.2)), func(i *Item) { i.Mirrored, i.ExtPriority = true, low[rnd.Intn(2)] })
	}
	for n := 0; n < 4; n++ { // lingering tickets that need a decision
		c := any()
		add(c, c, now.Add(-days(0.45, 1.7)), func(i *Item) { i.Mirrored, i.ExtPriority = true, low[rnd.Intn(2)] })
	}
	add(cells[0].ID, cells[0].ID, now.Add(-time.Hour), func(i *Item) { i.Mirrored, i.ExtPriority = true, "urgent" })
	for n := 0; n < 2; n++ { // planned work nobody has queued yet
		add("", any(), now.Add(-48*time.Hour), nil)
	}
	waits := []string{"the requester to confirm a time", "the vendor's support desk", "the security team's review", "the change window on Thursday"}
	for n, c := range cells { // started, then parked until someone else does their part
		for k, login := range c.People {
			if (n+k)%2 == 1 {
				continue
			}
			since := now.Add(-time.Duration(8+((n+k)%4)*40) * time.Hour)
			add(c.ID, c.ID, since.Add(-96*time.Hour), func(i *Item) {
				i.Status, i.COS, i.Assignee, i.Mirrored = StatusWaiting, "standard", login, k%3 == 0
				i.Started, i.WaitingSince, i.WaitingOn = at(since.Add(-30*time.Hour)), at(since), waits[(n+k)%len(waits)]
			})
		}
	}
	for n, c := range cells { // small jobs on each cell's quick list
		for k := 0; k <= n%2+1; k++ {
			age := time.Duration(5+k*30+n*7) * time.Hour
			add(c.ID, c.ID, now.Add(-age), func(i *Item) { i.Mirrored, i.ExtPriority, i.COS = true, "low", "quick" })
		}
	}
	return s
}

func (s *DemoStore) Items(context.Context) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Item, 0, len(s.order))
	for _, id := range s.order {
		i := *s.items[id]
		// Made-up addresses, so the demo shows where each item would open.
		i.URL = fmt.Sprintf("https://example.com/issues/%d", i.ID)
		if i.Mirrored {
			i.SourceURL = fmt.Sprintf("https://example.com/tickets/%d", i.ID)
		}
		out = append(out, i)
	}
	return out, nil
}

func (s *DemoStore) Update(_ context.Context, id int, c Change) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.items[id]
	if !ok {
		return fmt.Errorf("demo store: no item #%d", id)
	}
	if c.Cell != nil {
		i.Cell = *c.Cell
	}
	if c.Kind != nil {
		i.Kind = *c.Kind
	}
	if c.COS != nil {
		i.COS = *c.COS
	}
	if c.Rank != nil {
		r := *c.Rank
		i.Rank = &r
	}
	if c.Status != nil {
		i.Status, i.WaitingSince = *c.Status, nil
		if i.Status == StatusWaiting {
			now := time.Now().UTC()
			i.WaitingSince = &now
		}
	}
	if c.WaitingOn != nil {
		i.WaitingOn = *c.WaitingOn
	}
	if c.Assignee != nil {
		i.Assignee = *c.Assignee
	}
	if c.Status != nil && *c.Status == StatusInProgress && i.Started == nil {
		now := time.Now().UTC()
		i.Started, i.Updated = &now, &now
	}
	if c.Due != nil {
		d := *c.Due
		i.Due = &d
	}
	return nil
}

func (s *DemoStore) Comment(_ context.Context, id int, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return fmt.Errorf("demo store: no item #%d", id)
	}
	s.Comments = append(s.Comments, fmt.Sprintf("#%d: %s", id, text))
	return nil
}

// DemoTitle makes up a believable work item title for a cell.
func DemoTitle(rnd *rand.Rand, cell string) string {
	if _, ok := demoVerbs[cell]; !ok {
		cell = "platform"
	}
	return demoVerbs[cell][rnd.Intn(len(demoVerbs[cell]))] + " " + demoNouns[cell][rnd.Intn(len(demoNouns[cell]))]
}

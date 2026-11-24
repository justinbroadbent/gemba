package flow

import (
	"math"
	"sort"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/store"
)

// Work is either unplanned or one of the workspace's kinds of planned work.
const (
	Unplanned = "unplanned"
	Other     = "other" // planned, but nobody has said what kind
)

// IsUnplanned: work that arrived through the ticketing system starts unplanned
// and stays that way; planned work becomes unplanned the moment it is expedited.
func IsUnplanned(i store.Item) bool { return i.Mirrored || i.COS == "expedite" }

func TypeOf(i store.Item, ws *config.Workspace) string {
	if IsUnplanned(i) {
		return Unplanned
	}
	if ws.Kind(i.Kind) != nil {
		return i.Kind
	}
	return Other
}

func TypeName(t string, ws *config.Workspace) string {
	switch t {
	case Unplanned:
		return "Unplanned"
	case Other:
		return "Other planned"
	}
	return ws.Kind(t).Name
}

// TypeIcon names the small mark drawn beside work of a type.
func TypeIcon(t string, ws *config.Workspace) string {
	switch t {
	case Unplanned:
		return "bolt"
	case Other:
		return "dot"
	}
	if icon := ws.Kind(t).Icon; icon != "" {
		return icon
	}
	return "dot"
}

// Share is one type's part of the work a cell finished.
type Share struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Icon  string  `json:"icon"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
}

// Mix is where a cell's finished work went over the throughput window, by type.
// Quick tickets count: they are unplanned work too.
func Mix(items []store.Item, ws *config.Workspace, cell string, now time.Time) []Share {
	window := now.Add(-hours(float64(ws.Policies.ThroughputWeeks) * 7 * 24))
	counts, total := map[string]int{}, 0
	for _, i := range items {
		if CellOf(i, ws) == cell && i.Closed != nil && !i.Closed.Before(window) {
			counts[TypeOf(i, ws)]++
			total++
		}
	}
	order := []string{Unplanned}
	for _, k := range ws.Kinds {
		order = append(order, k.ID)
	}
	if counts[Other] > 0 {
		order = append(order, Other)
	}
	out := make([]Share, 0, len(order))
	for _, t := range order {
		s := Share{ID: t, Name: TypeName(t, ws), Icon: TypeIcon(t, ws), Count: counts[t]}
		if total > 0 {
			s.Pct = float64(counts[t]) / float64(total) * 100
		}
		out = append(out, s)
	}
	return out
}

// Norms holds how long each type of work usually takes here, so that an item
// is called aging only against work like it. A migration is not a password reset.
type Norms struct {
	usual    map[string]time.Duration
	fallback time.Duration
}

const (
	normWeeks   = 13
	normSamples = 8 // fewer finished items than this and the fixed policy applies
)

// BuildNorms takes the 85th percentile of time in progress for each type of
// work finished recently: "85% of items like this finish within this long".
func BuildNorms(items []store.Item, ws *config.Workspace, now time.Time) Norms {
	window := now.Add(-normWeeks * 7 * 24 * time.Hour)
	by := map[string][]time.Duration{}
	for _, i := range items {
		if i.Closed == nil || i.Started == nil || i.Closed.Before(window) || !i.Closed.After(*i.Started) {
			continue
		}
		t := TypeOf(i, ws)
		by[t] = append(by[t], i.Closed.Sub(*i.Started))
	}
	n := Norms{usual: map[string]time.Duration{}, fallback: hours(ws.Policies.AgingDays * 24)}
	for t, spans := range by {
		if len(spans) < normSamples {
			continue
		}
		sort.Slice(spans, func(a, b int) bool { return spans[a] < spans[b] })
		n.usual[t] = spans[int(math.Ceil(0.85*float64(len(spans))))-1]
	}
	return n
}

// Aging reports whether an in-progress item has outlasted the norm for its
// type, and what that norm is. Measured is false when the fixed policy was used.
func (n Norms) Aging(i store.Item, ws *config.Workspace, now time.Time) (aging bool, usual time.Duration, measured bool) {
	usual, measured = n.usual[TypeOf(i, ws)]
	if !measured {
		usual = n.fallback
	}
	return Stage(i) == Doing && i.Started != nil && now.Sub(*i.Started) > usual, usual, measured
}

// Project rolls up the items that belong to one project, across cells.
type Project struct {
	Name      string
	URL       string
	Kind      string
	Total     int
	Done      int
	Doing     int
	Blocked   int
	Queued    int
	Cells     []string
	LastMoved time.Time
	Pace      float64 // items finished per week over the throughput window
}

func (p Project) Remaining() int { return p.Total - p.Done }

// WeeksLeft is a rough forecast from the recent pace, or -1 when there has
// been no recent progress to forecast from.
func (p Project) WeeksLeft() float64 {
	if p.Pace <= 0 {
		return -1
	}
	return float64(p.Remaining()) / p.Pace
}

func Projects(items []store.Item, ws *config.Workspace, now time.Time) []Project {
	window := now.Add(-hours(float64(ws.Policies.ThroughputWeeks) * 7 * 24))
	by := map[string]*Project{}
	var names []string
	for _, i := range items {
		if i.Project == "" {
			continue
		}
		p := by[i.Project]
		if p == nil {
			p = &Project{Name: i.Project}
			by[i.Project] = p
			names = append(names, i.Project)
		}
		if p.URL == "" {
			p.URL = i.ProjectURL
		}
		if p.Kind == "" && ws.Kind(i.Kind) != nil {
			p.Kind = i.Kind
		}
		p.Total++
		moved := i.Created
		switch Stage(i) {
		case Done:
			p.Done++
			if i.Closed != nil {
				moved = *i.Closed
				if !i.Closed.Before(window) {
					p.Pace++
				}
			}
		case Doing:
			p.Doing++
			if i.BlockedReason != "" {
				p.Blocked++
			}
			if i.Started != nil {
				moved = *i.Started
			}
		case Queued:
			p.Queued++
		}
		if moved.After(p.LastMoved) {
			p.LastMoved = moved
		}
		if c := CellOf(i, ws); c != "" && Stage(i) != Done {
			found := false
			for _, have := range p.Cells {
				found = found || have == c
			}
			if !found {
				p.Cells = append(p.Cells, c)
			}
		}
	}
	kindOrder := func(id string) int {
		for n, k := range ws.Kinds {
			if k.ID == id {
				return n
			}
		}
		return len(ws.Kinds)
	}
	sort.Slice(names, func(a, b int) bool {
		ka, kb := kindOrder(by[names[a]].Kind), kindOrder(by[names[b]].Kind)
		if ka != kb {
			return ka < kb
		}
		return names[a] < names[b]
	})
	out := make([]Project, 0, len(names))
	for _, name := range names {
		p := *by[name]
		p.Pace /= float64(ws.Policies.ThroughputWeeks)
		out = append(out, p)
	}
	return out
}

// Next is a queued item a person could pull, with where it stands.
type Next struct {
	Item     store.Item
	Position int  // 1 is the top of the cell's queue
	Above    bool // within the capacity line
}

// Mine is one person's view: what they have in progress and what to pull next.
// It holds nothing historical.
type Mine struct {
	Person  config.Person
	Doing   []store.Item
	Waiting []store.Item
	Next    []Next
}

func (m Mine) Full() bool { return len(m.Doing) >= m.Person.WIPLimit }

// BuildMine lists a person's work in progress and the top of their cell's
// queue: items already theirs, or not yet anyone's.
func BuildMine(items []store.Item, ws *config.Workspace, person config.Person, now time.Time) Mine {
	m := Mine{Person: person}
	for _, i := range items {
		if i.Assignee != person.Login {
			continue
		}
		switch Stage(i) {
		case Doing:
			m.Doing = append(m.Doing, i)
		case Waiting:
			m.Waiting = append(m.Waiting, i)
		}
	}
	sort.SliceStable(m.Doing, func(a, b int) bool { return begun(m.Doing[a]).Before(begun(m.Doing[b])) })
	cell := ws.Cell(person.Cell)
	if cell == nil {
		return m
	}
	lane := BuildLane(items, ws, *cell, now)
	for n, i := range lane.Queue {
		if i.Assignee != "" && i.Assignee != person.Login {
			continue
		}
		if len(m.Next) == 5 {
			break
		}
		m.Next = append(m.Next, Next{Item: i, Position: n + 1, Above: n < lane.Line})
	}
	return m
}

package api

import (
	"sort"
	"time"

	"github.com/justinbroadbent/gemba/internal/actions"
	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/flow"
	"github.com/justinbroadbent/gemba/internal/store"
)

// The API returns view models that are ready to render. The web app applies no
// flow rules of its own.

type card struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	URL        string `json:"url,omitempty"`
	Owner      string `json:"owner,omitempty"`
	Cell       string `json:"cell,omitempty"`
	COS        string `json:"cos,omitempty"`
	Due        string `json:"due,omitempty"`
	Mirrored   bool   `json:"mirrored"`
	Blocked    string `json:"blocked,omitempty"`    // the reason
	BlockedFor string `json:"blockedFor,omitempty"` // how long
	AgingFor   string `json:"agingFor,omitempty"`   // set only when past the aging threshold
}

func cardOf(i store.Item, ws *config.Workspace, now time.Time) card {
	c := card{ID: i.ID, Title: i.Title, URL: i.URL, Cell: i.Cell, COS: i.COS, Mirrored: i.Mirrored, Owner: i.Assignee}
	if i.SourceURL != "" {
		c.URL = i.SourceURL // open the item where it is worked
	}
	if p := ws.Person(i.Assignee); p != nil {
		c.Owner = p.Name
	}
	if i.Due != nil {
		c.Due = i.Due.Format("Jan 2")
	}
	if i.BlockedReason != "" && flow.Stage(i) != flow.Done {
		c.Blocked = i.BlockedReason
		if i.BlockedSince != nil {
			c.BlockedFor = flow.Span(now.Sub(*i.BlockedSince))
		}
	} else if flow.IsAging(i, ws, now) {
		c.AgingFor = flow.Span(now.Sub(*i.Started))
	}
	return c
}

func cardsOf(items []store.Item, ws *config.Workspace, now time.Time) []card {
	out := make([]card, 0, len(items))
	for _, i := range items {
		out = append(out, cardOf(i, ws, now))
	}
	return out
}

type personView struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Limit int    `json:"limit"`
}

type laneView struct {
	Cell       config.Cell  `json:"cell"`
	Throughput float64      `json:"throughput"`
	QuickRate  float64      `json:"quickRate"`
	Measured   bool         `json:"measured"`
	Line       int          `json:"line"`
	WIPLimit   int          `json:"wipLimit"`
	People     []personView `json:"people"`
	Queue      []card       `json:"queue"`
	Doing      []card       `json:"doing"`
	Done       []card       `json:"done"`
	QuickDone  int          `json:"quickDone"`
}

func laneOf(l flow.Lane, ws *config.Workspace, now time.Time) laneView {
	v := laneView{
		Cell: l.Cell, Throughput: l.Throughput, QuickRate: l.QuickRate, Measured: l.Measured,
		Line: l.Line, WIPLimit: l.WIPLimit, QuickDone: l.QuickDone, People: []personView{},
		Queue: cardsOf(l.Queue, ws, now), Doing: cardsOf(l.Doing, ws, now), Done: cardsOf(l.Done, ws, now),
	}
	for _, p := range l.People {
		v.People = append(v.People, personView{p.Person.Name, len(p.Items), p.Person.WIPLimit})
	}
	return v
}

type floorView struct {
	Signals struct {
		Triage  int `json:"triage"`
		Blocked int `json:"blocked"`
		Aging   int `json:"aging"`
		Over    int `json:"over"`
	} `json:"signals"`
	Lanes []laneView `json:"lanes"`
}

func floorOf(items []store.Item, ws *config.Workspace, now time.Time) floorView {
	var v floorView
	v.Signals.Triage = len(flow.Inbox(items, ws, now))
	for _, c := range ws.Cells {
		l := flow.BuildLane(items, ws, c, now)
		v.Signals.Blocked += l.Blocked()
		for _, i := range l.Doing {
			if i.BlockedReason == "" && flow.IsAging(i, ws, now) {
				v.Signals.Aging++
			}
		}
		for _, p := range l.People {
			if p.Over() {
				v.Signals.Over++
			}
		}
		v.Lanes = append(v.Lanes, laneOf(l, ws, now))
	}
	return v
}

type triageRow struct {
	Card     card   `json:"card"`
	Why      string `json:"why"`
	Urgent   bool   `json:"urgent"`
	Priority string `json:"priority,omitempty"`
}

type inboxView struct {
	FastLane int         `json:"fastLane"`
	Rows     []triageRow `json:"rows"`
}

func inboxOf(items []store.Item, ws *config.Workspace, now time.Time) inboxView {
	v := inboxView{FastLane: flow.FastLane(items, ws, now), Rows: []triageRow{}}
	for _, t := range flow.Inbox(items, ws, now) {
		v.Rows = append(v.Rows, triageRow{cardOf(t.Item, ws, now), t.Why, t.Urgent, t.Item.ExtPriority})
	}
	return v
}

type candidate struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Cell  string `json:"cell,omitempty"`
	Stage string `json:"stage"`
}

type previewView struct {
	Item         card     `json:"item"`
	Lane         laneView `json:"lane"`
	Falls        *card    `json:"falls"`
	AlreadyAbove bool     `json:"alreadyAbove"`
}

type tradeoffView struct {
	Candidates []candidate  `json:"candidates"`
	Preview    *previewView `json:"preview"`
}

func tradeoffOf(items []store.Item, ws *config.Workspace) tradeoffView {
	v := tradeoffView{Candidates: []candidate{}}
	sort.SliceStable(items, func(a, b int) bool { return items[a].Created.After(items[b].Created) })
	for _, i := range items {
		if s := flow.Stage(i); s == flow.Intake || s == flow.Queued {
			v.Candidates = append(v.Candidates, candidate{i.ID, i.Title, i.Cell, s})
		}
	}
	return v
}

func previewOf(p *actions.Preview, ws *config.Workspace, now time.Time) *previewView {
	v := &previewView{Item: cardOf(p.Item, ws, now), Lane: laneOf(p.Lane, ws, now), AlreadyAbove: p.AlreadyAbove}
	if p.Falls != nil {
		c := cardOf(*p.Falls, ws, now)
		v.Falls = &c
	}
	return v
}

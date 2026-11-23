package flow

import (
	"fmt"
	"sort"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/store"
)

// This file is the part that makes Gemba a guide rather than a board: it turns
// rank into time, spots conflicts before they happen, and says what needs help.

// Outlook is what a queued item's position means in practice.
type Outlook struct {
	Wait    time.Duration // until a hand is likely free for it; meaningless if !Known
	Known   bool          // false when the cell has no measured pace
	StartBy *time.Time    // for dated work: the latest start that still makes the date
	AtRisk  bool          // dated work whose likely start is after StartBy
}

// Outlooks forecasts, for each item in a lane's queue, when it is likely to
// start. Free hands take the top of the queue now; after that, each finish at
// the cell's measured pace frees one more.
func Outlooks(l Lane, ws *config.Workspace, norms Norms, now time.Time) []Outlook {
	free := max(0, l.WIPLimit-len(l.Doing))
	out := make([]Outlook, len(l.Queue))
	for n, i := range l.Queue {
		o := Outlook{Known: true}
		switch {
		case n < free:
		case l.Throughput <= 0:
			o.Known = false
		default:
			o.Wait = hours(float64(n-free+1) / l.Throughput * 7 * 24)
		}
		if i.Due != nil {
			_, usual, _ := norms.Aging(i, ws, now)
			by := i.Due.Add(-usual)
			o.StartBy = &by
			o.AtRisk = (o.Known && now.Add(o.Wait).After(by)) || (!o.Known && now.After(by))
		}
		out[n] = o
	}
	return out
}

// Reason says in a few words why a queued item sits where it does. A priority
// people can see the reason for is one they will follow.
func Reason(i store.Item, position int, cell config.Cell, ws *config.Workspace, now time.Time) string {
	switch {
	case i.COS == "expedite":
		return "Expedited: goes before everything else"
	case i.COS == "fixed-date" && i.Due != nil:
		return "Has a date: due " + i.Due.Format("Jan 2")
	case position == 0:
		return "Top of the " + cell.Name + " queue"
	case i.Mirrored:
		return "A requester has been waiting " + Span(now.Sub(i.Created))
	case i.Project != "":
		return "Part of " + i.Project
	}
	return fmt.Sprintf("Number %d in the %s queue", position+1, cell.Name)
}

// IsQuiet: in progress, not blocked, and nothing has happened to it for a
// while. Work nobody is touching is not really in progress.
func IsQuiet(i store.Item, ws *config.Workspace, now time.Time) bool {
	return Stage(i) == Doing && i.BlockedReason == "" && i.Updated != nil &&
		now.Sub(*i.Updated) > hours(ws.Policies.QuietDays*24)
}

// Arrivals compares this week's unplanned arrivals in a cell with the usual.
func Arrivals(items []store.Item, ws *config.Workspace, cell string, now time.Time) (week int, usual float64) {
	weeks := ws.Policies.ThroughputWeeks
	window := now.Add(-hours(float64(weeks) * 7 * 24))
	total := 0
	for _, i := range items {
		if !i.Mirrored || CellOf(i, ws) != cell || i.Created.Before(window) {
			continue
		}
		total++
		if now.Sub(i.Created) <= 7*24*time.Hour {
			week++
		}
	}
	return week, float64(total) / float64(weeks)
}

// Pulls suggests, for each person with a free hand, the next item to pull:
// the highest item in their cell's queue that is theirs or nobody's. Nobody is
// assigned anything; this is the queue meeting free capacity.
func Pulls(l Lane) map[string]store.Item {
	out, taken := map[string]store.Item{}, map[int]bool{}
	for _, p := range l.People {
		if len(p.Items) >= p.Person.WIPLimit {
			continue
		}
		for _, i := range l.Queue {
			if taken[i.ID] || (i.Assignee != "" && i.Assignee != p.Person.Login) {
				continue
			}
			out[p.Person.Login], taken[i.ID] = i, true
			break
		}
	}
	return out
}

// Alert is one thing that needs someone's attention, as a sentence.
type Alert struct {
	Level string `json:"level"` // "stop" or "warn"
	Text  string `json:"text"`
	Cell  string `json:"cell,omitempty"`
	ID    int    `json:"id,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Attention lists what needs help, most serious first. It is the opposite of a
// board: it shows only the exceptions, so everything not listed can be left alone.
func Attention(items []store.Item, ws *config.Workspace, norms Norms, now time.Time) []Alert {
	var out []Alert
	link := func(i store.Item) string {
		if i.SourceURL != "" {
			return i.SourceURL
		}
		return i.URL
	}
	add := func(level, text string, cell config.Cell, i *store.Item) {
		a := Alert{Level: level, Text: text, Cell: cell.Name}
		if i != nil {
			a.ID, a.URL = i.ID, link(*i)
		}
		out = append(out, a)
	}
	var lanes []Lane
	for _, c := range ws.Cells {
		lanes = append(lanes, BuildLane(items, ws, c, now))
	}

	for _, l := range lanes { // expedites that nobody has picked up
		for n := range l.Queue {
			if i := l.Queue[n]; i.COS == "expedite" {
				add("stop", fmt.Sprintf("Expedited and not started: %s", i.Title), l.Cell, &i)
			}
		}
	}
	for _, l := range lanes { // dates that the current order will miss
		for n, o := range Outlooks(l, ws, norms, now) {
			if i := l.Queue[n]; o.AtRisk {
				add("stop", fmt.Sprintf("Likely to miss %s: %s is number %d in the queue and needs to start by %s",
					i.Due.Format("Jan 2"), i.Title, n+1, o.StartBy.Format("Jan 2")), l.Cell, &i)
			}
		}
	}
	var blocked []store.Item
	for _, l := range lanes {
		for _, i := range l.Doing {
			if i.BlockedReason != "" {
				blocked = append(blocked, i)
			}
		}
	}
	sort.SliceStable(blocked, func(a, b int) bool {
		return blocked[a].BlockedSince != nil && (blocked[b].BlockedSince == nil || blocked[a].BlockedSince.Before(*blocked[b].BlockedSince))
	})
	for n := range blocked {
		i := blocked[n]
		text := "Blocked"
		if i.BlockedSince != nil {
			text += " " + Span(now.Sub(*i.BlockedSince))
		}
		text += fmt.Sprintf(": %s. %s", i.Title, i.BlockedReason)
		if p := ws.Person(i.Assignee); p != nil {
			text += " (" + p.Name + ")"
		}
		c := ws.Cell(CellOf(i, ws))
		add("stop", text, *c, &i)
	}
	for _, l := range lanes {
		if l.WIPLimit > 0 && len(l.Doing) > l.WIPLimit {
			add("warn", fmt.Sprintf("%s has %d in progress against a limit of %d: finish something before starting more",
				l.Cell.Name, len(l.Doing), l.WIPLimit), l.Cell, nil)
		}
	}
	for _, l := range lanes {
		for n := range l.Doing {
			i := l.Doing[n]
			if i.BlockedReason != "" {
				continue
			}
			if aging, usual, measured := norms.Aging(i, ws, now); aging {
				text := fmt.Sprintf("In progress %s: %s", Span(now.Sub(*i.Started)), i.Title)
				if measured {
					text += fmt.Sprintf(". Work like it usually takes under %s", Span(usual))
				}
				add("warn", text, l.Cell, &i)
			} else if IsQuiet(i, ws, now) {
				add("warn", fmt.Sprintf("No activity for %s: %s", Span(now.Sub(*i.Updated)), i.Title), l.Cell, &i)
			}
		}
	}
	for _, l := range lanes {
		if week, usual := Arrivals(items, ws, l.Cell.ID, now); week >= 3 && float64(week) > usual*1.5 {
			add("warn", fmt.Sprintf("%s took %d unplanned arrivals this week, against a usual %.0f", l.Cell.Name, week, usual), l.Cell, nil)
		}
	}
	return out
}

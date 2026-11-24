// Package flow holds the lean flow rules. It does no I/O: items and policy in,
// numbers and lists out.
package flow

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/store"
)

const (
	Intake = "intake"
	Queued = "queued"
	Doing  = "doing"
	// Waiting: started, then parked until someone else does their part. It
	// does not hold a hand, which is why each person may have only a few.
	Waiting = "waiting"
	Done    = "done"
)

// Stage derives where an item is. Triage (setting a class of service) is what
// moves an item from intake into a queue.
func Stage(i store.Item) string {
	switch i.Status {
	case store.StatusDone:
		return Done
	case store.StatusInProgress:
		return Doing
	case store.StatusWaiting:
		return Waiting
	}
	if i.Cell != "" && i.COS != "" {
		return Queued
	}
	return Intake
}

// CellOf is the item's cell, falling back to its assignee's home cell.
func CellOf(i store.Item, ws *config.Workspace) string {
	if i.Cell != "" {
		return i.Cell
	}
	if p := ws.Person(i.Assignee); p != nil {
		return p.Cell
	}
	return ""
}

// LastStandup is the same time on the previous working day.
func LastStandup(now time.Time) time.Time {
	d := now.AddDate(0, 0, -1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

func IsAging(i store.Item, ws *config.Workspace, now time.Time) bool {
	return Stage(i) == Doing && i.Started != nil &&
		now.Sub(*i.Started) > hours(ws.Policies.AgingDays*24)
}

type Triage struct {
	Item   store.Item
	Why    string
	Urgent bool
}

// Inbox lists intake items that need a decision. Fresh mirrored tickets are
// left out so quick ones can be resolved without anyone triaging them.
func Inbox(items []store.Item, ws *config.Workspace, now time.Time) []Triage {
	var out []Triage
	for _, i := range items {
		if Stage(i) != Intake {
			continue
		}
		age := now.Sub(i.Created)
		switch {
		case !i.Mirrored:
			out = append(out, Triage{i, "Planned work, not yet queued", false})
		case slices.Contains(ws.Policies.UrgentPriorities, strings.ToLower(i.ExtPriority)):
			out = append(out, Triage{i, fmt.Sprintf("Marked %s at the source", strings.ToLower(i.ExtPriority)), true})
		case age >= hours(ws.Policies.TriageAfterHours):
			out = append(out, Triage{i, fmt.Sprintf("Open %s without being resolved", Span(age)), false})
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Urgent != out[b].Urgent {
			return out[a].Urgent
		}
		return out[a].Item.Created.Before(out[b].Item.Created)
	})
	return out
}

// FastLane counts intake items that have not yet surfaced for triage.
func FastLane(items []store.Item, ws *config.Workspace, now time.Time) int {
	n := 0
	for _, i := range items {
		if Stage(i) == Intake {
			n++
		}
	}
	return n - len(Inbox(items, ws, now))
}

// Queue is a cell's queued items in pull order: expedite first, then by rank.
func Queue(items []store.Item, cell string) []store.Item {
	var q []store.Item
	for _, i := range items {
		if Stage(i) == Queued && i.Cell == cell && i.COS != "quick" {
			q = append(q, i)
		}
	}
	sort.SliceStable(q, func(a, b int) bool {
		ea, eb := q[a].COS == "expedite", q[b].COS == "expedite"
		if ea != eb {
			return ea
		}
		ra, rb := rank(q[a]), rank(q[b])
		if ra != rb {
			return ra < rb
		}
		return q[a].Created.Before(q[b].Created)
	})
	return q
}

// Quick is a cell's quick list: small jobs that are not worth a place in the
// ranked queue. Anyone with a spare half hour takes the oldest.
func Quick(items []store.Item, cell string) []store.Item {
	var q []store.Item
	for _, i := range items {
		if Stage(i) == Queued && i.Cell == cell && i.COS == "quick" {
			q = append(q, i)
		}
	}
	sort.SliceStable(q, func(a, b int) bool { return q[a].Created.Before(q[b].Created) })
	return q
}

func rank(i store.Item) float64 {
	if i.Rank == nil {
		return math.Inf(1)
	}
	return *i.Rank
}

// Load is one person's current work against their limit. It deliberately
// carries nothing historical.
type Load struct {
	Person  config.Person
	Items   []store.Item
	Waiting []store.Item
}

func (l Load) Over() bool { return len(l.Items) > l.Person.WIPLimit }

type Lane struct {
	Cell       config.Cell
	Queue      []store.Item
	Quick      []store.Item // the quick list, oldest first
	Doing      []store.Item
	Done       []store.Item // triaged work finished since the last standup
	QuickDone  int          // fast-lane tickets finished since the last standup
	People     []Load
	Throughput float64 // triaged items finished per week
	QuickRate  float64 // fast-lane tickets finished per week
	Measured   bool    // false when there is no history to base throughput on
	Line       int     // how many queued items fit within the horizon
	WIPLimit   int
}

// Above is the part of the queue that fits within the horizon.
func (l Lane) Above() []store.Item { return l.Queue[:min(l.Line, len(l.Queue))] }

func (l Lane) Blocked() int {
	n := 0
	for _, i := range l.Doing {
		if i.BlockedReason != "" {
			n++
		}
	}
	return n
}

func BuildLane(items []store.Item, ws *config.Workspace, cell config.Cell, now time.Time) Lane {
	p := ws.Policies
	l := Lane{Cell: cell, Queue: Queue(items, cell.ID), Quick: Quick(items, cell.ID)}
	window := now.Add(-hours(float64(p.ThroughputWeeks) * 7 * 24))
	since := LastStandup(now)
	triaged, quick := 0, 0
	for _, i := range items {
		if CellOf(i, ws) != cell.ID {
			continue
		}
		if Stage(i) == Doing {
			l.Doing = append(l.Doing, i)
		}
		if i.Closed == nil || i.Closed.Before(window) {
			continue
		}
		// Quick work is left out of the pace that forecasts the ranked queue.
		queued := i.COS != "" && i.COS != "quick"
		if queued {
			triaged++
		} else {
			quick++
		}
		if !i.Closed.Before(since) {
			if queued {
				l.Done = append(l.Done, i)
			} else {
				l.QuickDone++
			}
		}
	}
	sort.SliceStable(l.Doing, func(a, b int) bool { return begun(l.Doing[a]).Before(begun(l.Doing[b])) })
	sort.SliceStable(l.Done, func(a, b int) bool { return l.Done[a].Closed.After(*l.Done[b].Closed) })

	for _, person := range ws.People {
		if person.Cell != cell.ID {
			continue
		}
		load := Load{Person: person}
		for _, i := range items {
			if i.Assignee != person.Login {
				continue
			}
			switch Stage(i) {
			case Doing:
				load.Items = append(load.Items, i)
			case Waiting:
				load.Waiting = append(load.Waiting, i)
			}
		}
		l.People = append(l.People, load)
		l.WIPLimit += person.WIPLimit
	}

	weeks := float64(p.ThroughputWeeks)
	l.Throughput = float64(triaged) / weeks
	l.QuickRate = float64(quick) / weeks
	l.Measured = triaged > 0
	// Work already in progress finishes first; the queue gets what is left.
	l.Line = max(0, int(math.Round(l.Throughput*float64(p.HorizonWeeks)))-len(l.Doing))
	return l
}

func begun(i store.Item) time.Time {
	if i.Started != nil {
		return *i.Started
	}
	return i.Created
}

func hours(h float64) time.Duration { return time.Duration(h * float64(time.Hour)) }

// Span renders a duration the way people say it: hours up to two days, then days.
func Span(d time.Duration) string {
	h := d.Hours()
	if h < 1 {
		return "under 1 h"
	}
	if h < 48 {
		return fmt.Sprintf("%.0f h", h)
	}
	return fmt.Sprintf("%.0f d", h/24)
}

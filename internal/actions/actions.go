// Package actions holds the few things Gemba changes: triage, queue order,
// and recorded trade-offs.
package actions

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/flow"
	"github.com/justinbroadbent/gemba/internal/store"
)

// Error is a problem the user can act on. Conflict means the data changed
// underneath them and they should reload.
type Error struct {
	Msg      string
	Conflict bool
}

func (e *Error) Error() string { return e.Msg }

func bad(format string, a ...any) error { return &Error{Msg: fmt.Sprintf(format, a...)} }

func stale() error {
	return &Error{Msg: "The queue changed while you were working. It has been reloaded.", Conflict: true}
}

func find(items []store.Item, id int) (store.Item, error) {
	for _, i := range items {
		if i.ID == id {
			return i, nil
		}
	}
	return store.Item{}, bad("There is no item #%d", id)
}

func top(q []store.Item) float64 {
	for _, i := range q {
		if i.Rank != nil {
			lo := *i.Rank
			for _, j := range q {
				if j.Rank != nil && *j.Rank < lo {
					lo = *j.Rank
				}
			}
			return lo - 1
		}
	}
	return 1
}

func bottom(q []store.Item) float64 {
	hi, found := 0.0, false
	for _, i := range q {
		if i.Rank != nil && (!found || *i.Rank > hi) {
			hi, found = *i.Rank, true
		}
	}
	return hi + 1
}

func ptr[T any](v T) *T { return &v }

// Triage gives an intake item a cell and a class of service, which queues it.
func Triage(ctx context.Context, st store.Store, ws *config.Workspace, id int, cell, cos string, toTop bool) error {
	if ws.Cell(cell) == nil {
		return bad("Unknown cell %q", cell)
	}
	if !slices.Contains(config.COS, cos) {
		return bad("Unknown class of service %q", cos)
	}
	items, err := st.Items(ctx)
	if err != nil {
		return err
	}
	item, err := find(items, id)
	if err != nil {
		return err
	}
	if flow.Stage(item) != flow.Intake {
		return &Error{Msg: fmt.Sprintf("#%d has already been triaged", id), Conflict: true}
	}
	q := flow.Queue(items, cell)
	r := bottom(q)
	if toTop {
		r = top(q)
	}
	return st.Update(ctx, id, store.Change{Cell: &cell, COS: &cos, Rank: &r})
}

// Move places a queued item directly before another one, or at the end when
// before is nil. It normally writes a single rank.
func Move(ctx context.Context, st store.Store, ws *config.Workspace, cell string, id int, before *int) error {
	if ws.Cell(cell) == nil {
		return bad("Unknown cell %q", cell)
	}
	items, err := st.Items(ctx)
	if err != nil {
		return err
	}
	q := flow.Queue(items, cell)
	from := slices.IndexFunc(q, func(i store.Item) bool { return i.ID == id })
	if from < 0 {
		return stale()
	}
	moved := q[from]
	q = slices.Delete(q, from, from+1)
	at := len(q)
	if before != nil {
		if at = slices.IndexFunc(q, func(i store.Item) bool { return i.ID == *before }); at < 0 {
			return stale()
		}
	}
	var prev, next *float64
	if at > 0 {
		prev = q[at-1].Rank
	}
	if at < len(q) {
		next = q[at].Rank
	}
	switch {
	case at > 0 && prev == nil, at < len(q) && next == nil, prev != nil && next != nil && *prev >= *next:
		// No usable gap: renumber the whole queue in its new order.
		q = slices.Insert(q, at, moved)
		for n, i := range q {
			if err := st.Update(ctx, i.ID, store.Change{Rank: ptr(float64(n + 1))}); err != nil {
				return err
			}
		}
		return nil
	case prev != nil && next != nil:
		return st.Update(ctx, id, store.Change{Rank: ptr((*prev + *next) / 2)})
	case next != nil:
		return st.Update(ctx, id, store.Change{Rank: ptr(*next - 1)})
	case prev != nil:
		return st.Update(ctx, id, store.Change{Rank: ptr(*prev + 1)})
	}
	return st.Update(ctx, id, store.Change{Rank: ptr(1.0)})
}

// Preview shows what putting an item at the front of a cell would cost.
type Preview struct {
	Item         store.Item
	Lane         flow.Lane
	Falls        *store.Item // the queued item this pushes past the capacity line
	AlreadyAbove bool
}

func preview(items []store.Item, ws *config.Workspace, id int, cell string, now time.Time) (*Preview, error) {
	c := ws.Cell(cell)
	if c == nil {
		return nil, bad("Unknown cell %q", cell)
	}
	item, err := find(items, id)
	if err != nil {
		return nil, err
	}
	if s := flow.Stage(item); s != flow.Intake && s != flow.Queued {
		return nil, bad("#%d is already in progress or done", id)
	}
	p := &Preview{Item: item, Lane: flow.BuildLane(items, ws, *c, now)}
	var others []store.Item
	for n, i := range p.Lane.Queue {
		if i.ID == id {
			p.AlreadyAbove = n < p.Lane.Line
			continue
		}
		others = append(others, i)
	}
	if !p.AlreadyAbove && p.Lane.Line > 0 && len(others) >= p.Lane.Line {
		p.Falls = &others[p.Lane.Line-1]
	}
	return p, nil
}

func PreviewTradeoff(ctx context.Context, st store.Store, ws *config.Workspace, id int, cell string, now time.Time) (*Preview, error) {
	items, err := st.Items(ctx)
	if err != nil {
		return nil, err
	}
	return preview(items, ws, id, cell, now)
}

// Note is a comment Gemba wrote on an item.
type Note struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// Decide puts an item at the front of a cell and records what it cost, as
// comments on the affected items. If pause is set, that in-progress item is
// displaced and the new item becomes an expedite.
func Decide(ctx context.Context, st store.Store, ws *config.Workspace, id int, cell string, pause *int, authorizedBy string, now time.Time) ([]Note, error) {
	authorizedBy = strings.TrimSpace(authorizedBy)
	if authorizedBy == "" {
		return nil, bad("Record who authorized this")
	}
	items, err := st.Items(ctx)
	if err != nil {
		return nil, err
	}
	p, err := preview(items, ws, id, cell, now)
	if err != nil {
		return nil, err
	}
	item, name := p.Item, p.Lane.Cell.Name
	var rest []store.Item
	for _, i := range p.Lane.Queue {
		if i.ID != id {
			rest = append(rest, i)
		}
	}
	front := top(rest)
	var notes []Note
	note := func(target store.Item, text string) error {
		notes = append(notes, Note{target.ID, target.Title, text})
		return st.Comment(ctx, target.ID, text)
	}

	if pause != nil {
		at := slices.IndexFunc(p.Lane.Doing, func(i store.Item) bool { return i.ID == *pause })
		if at < 0 {
			return nil, &Error{Msg: fmt.Sprintf("#%d is no longer in progress in %s", *pause, name), Conflict: true}
		}
		paused := p.Lane.Doing[at]
		if err := st.Update(ctx, id, store.Change{Cell: &cell, COS: ptr("expedite"), Rank: &front}); err != nil {
			return nil, err
		}
		if err := note(item, fmt.Sprintf("Expedited in %s. Authorized by %s. Displaces #%d (%s).",
			name, authorizedBy, paused.ID, paused.Title)); err != nil {
			return nil, err
		}
		text := fmt.Sprintf("Paused to make room for expedited #%d (%s). Authorized by %s.", item.ID, item.Title, authorizedBy)
		if paused.Mirrored {
			text += " Please set this back to open in the ticketing system."
		} else if err := st.Update(ctx, paused.ID, store.Change{Status: ptr(store.StatusOpen), Rank: ptr(front + 0.5)}); err != nil {
			return nil, err
		}
		return notes, note(paused, text)
	}

	cos := item.COS
	if cos == "" {
		cos = "standard"
	}
	if err := st.Update(ctx, id, store.Change{Cell: &cell, COS: &cos, Rank: &front}); err != nil {
		return nil, err
	}
	text := fmt.Sprintf("Moved to the front of the %s queue. Authorized by %s.", name, authorizedBy)
	if p.Falls != nil {
		weeks := ws.Policies.HorizonWeeks
		text += fmt.Sprintf(" Pushes #%d (%s) past the %d-week capacity line.", p.Falls.ID, p.Falls.Title, weeks)
		if err := note(*p.Falls, fmt.Sprintf("Pushed past the %d-week capacity line by #%d (%s). Authorized by %s.",
			weeks, item.ID, item.Title, authorizedBy)); err != nil {
			return nil, err
		}
	}
	return notes, note(item, text)
}

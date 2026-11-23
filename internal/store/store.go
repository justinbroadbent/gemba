// Package store defines the contract between Gemba and wherever work items live.
//
// Gemba holds no work state of its own. A Store supplies items and accepts a
// deliberately small set of writes.
//
// Field ownership:
//   - Gemba writes only Cell, Kind, COS (class of service), Rank, and comments. The
//     one exception is setting Status back to open on a non-mirrored item that
//     a trade-off decision paused.
//   - Everything else is owned by the engineers or by whatever sync mirrors
//     tickets in from an external system. Gemba never writes it.
package store

import (
	"context"
	"time"
)

const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusDone       = "done"
)

// Item is one unit of work. ID is unique within a workspace.
type Item struct {
	ID    int
	Title string
	URL   string
	// SourceURL is where a mirrored item is actually worked: the ticket in the
	// external system. Empty for items that live in the store itself.
	SourceURL string
	Created   time.Time
	Status    string

	Kind       string // kind of planned work, by id from the workspace config; may be empty
	Project    string // name of the project this belongs to, if any
	ProjectURL string // where that project is described (an epic, a parent issue)

	Cell string   // discipline queue this belongs to; empty until sorted
	COS  string   // class of service; empty means not yet triaged
	Rank *float64 // position in the cell queue, lowest first

	Assignee    string
	Mirrored    bool   // copied in from an external ticketing system
	ExtPriority string // the requester-facing priority at the source
	Due         *time.Time

	BlockedReason string
	BlockedSince  *time.Time
	Started       *time.Time
	Updated       *time.Time // when anything last happened to it
	Closed        *time.Time
}

// Change lists the fields a write may touch. Nil means leave alone.
type Change struct {
	Cell   *string
	Kind   *string
	COS    *string
	Rank   *float64
	Status *string
}

// Prioritizer is implemented by stores that can supply the team's current
// priorities as a short piece of text.
type Prioritizer interface {
	Priorities(ctx context.Context) (string, error)
}

type Store interface {
	Items(ctx context.Context) ([]Item, error)
	Update(ctx context.Context, id int, c Change) error
	// Comment records a decision on the item, where the team will see it.
	Comment(ctx context.Context, id int, text string) error
}

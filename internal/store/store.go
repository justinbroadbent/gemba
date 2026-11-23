// Package store defines the contract between Gemba and wherever work items live.
//
// Gemba holds no work state of its own. A Store supplies items and accepts a
// deliberately small set of writes.
//
// Field ownership:
//   - Gemba writes only Cell, COS (class of service), Rank, and comments. The
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
	ID      int
	Title   string
	URL     string
	Created time.Time
	Status  string

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
	Closed        *time.Time
}

// Change lists the fields a write may touch. Nil means leave alone.
type Change struct {
	Cell   *string
	COS    *string
	Rank   *float64
	Status *string
}

type Store interface {
	Items(ctx context.Context) ([]Item, error)
	Update(ctx context.Context, id int, c Change) error
	// Comment records a decision on the item, where the team will see it.
	Comment(ctx context.Context, id int, text string) error
}

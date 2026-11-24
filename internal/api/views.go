package api

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/justinbroadbent/gemba/internal/actions"
	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/flow"
	"github.com/justinbroadbent/gemba/internal/store"
)

// The API returns view models that are ready to render. The web app applies no
// flow rules of its own.

// view carries what every view model needs to describe an item.
type view struct {
	ws    *config.Workspace
	now   time.Time
	norms flow.Norms
}

func newView(items []store.Item, ws *config.Workspace, now time.Time) view {
	return view{ws, now, flow.BuildNorms(items, ws, now)}
}

type card struct {
	ID         int      `json:"id"`
	Title      string   `json:"title"`
	URL        string   `json:"url,omitempty"`
	Owner      string   `json:"owner,omitempty"`
	Cell       string   `json:"cell,omitempty"`
	Type       string   `json:"type"`     // "unplanned", a kind id, or "other"
	TypeName   string   `json:"typeName"` // how to say it
	Icon       string   `json:"icon"`     // the mark drawn for it
	Kind       string   `json:"kind,omitempty"`
	Project    string   `json:"project,omitempty"`
	ProjectURL string   `json:"projectUrl,omitempty"`
	COS        string   `json:"cos,omitempty"`
	Due        string   `json:"due,omitempty"`
	Mirrored   bool     `json:"mirrored"`
	Blocked    string   `json:"blocked,omitempty"`    // the reason
	BlockedFor string   `json:"blockedFor,omitempty"` // how long
	AgingFor   string   `json:"agingFor,omitempty"`   // set only when past the norm for its type
	Usual      string   `json:"usual,omitempty"`      // that norm, when it was measured
	Day        int      `json:"day,omitempty"`        // in progress: which day this is
	UsualDays  float64  `json:"usualDays,omitempty"`  // in progress: how many days work like it usually takes
	WaitDays   *float64 `json:"waitDays,omitempty"`   // queued: days until it is likely to start, when known
	QuietFor   string   `json:"quietFor,omitempty"`   // in progress with no recent activity
	Reason     string   `json:"reason,omitempty"`     // queued: why it sits where it does
	Starts     string   `json:"starts,omitempty"`     // queued: when it is likely to start
	Risk       string   `json:"risk,omitempty"`       // queued and dated: likely to miss the date
}

func (v view) card(i store.Item) card {
	t := flow.TypeOf(i, v.ws)
	c := card{ID: i.ID, Title: i.Title, URL: i.URL, Cell: i.Cell, COS: i.COS, Mirrored: i.Mirrored, Owner: i.Assignee,
		Type: t, TypeName: flow.TypeName(t, v.ws), Icon: flow.TypeIcon(t, v.ws), Kind: i.Kind, Project: i.Project, ProjectURL: i.ProjectURL}
	if i.SourceURL != "" {
		c.URL = i.SourceURL // open the item where it is worked
	}
	if p := v.ws.Person(i.Assignee); p != nil {
		c.Owner = p.Name
	}
	if i.Due != nil {
		c.Due = i.Due.Format("Jan 2")
	}
	if i.BlockedReason != "" && flow.Stage(i) != flow.Done {
		c.Blocked = i.BlockedReason
		if i.BlockedSince != nil {
			c.BlockedFor = flow.Span(v.now.Sub(*i.BlockedSince))
		}
	}
	if flow.Stage(i) == flow.Doing && i.Started != nil {
		c.Day = int(v.now.Sub(*i.Started).Hours()/24) + 1
		_, usual, _ := v.norms.Aging(i, v.ws, v.now)
		c.UsualDays = usual.Hours() / 24
	}
	if flow.IsQuiet(i, v.ws, v.now) {
		c.QuietFor = flow.Span(v.now.Sub(*i.Updated))
	}
	if aging, usual, measured := v.norms.Aging(i, v.ws, v.now); aging && c.Blocked == "" {
		c.AgingFor = flow.Span(v.now.Sub(*i.Started))
		if measured {
			c.Usual = flow.Span(usual)
		}
	}
	return c
}

func (v view) cards(items []store.Item) []card {
	out := make([]card, 0, len(items))
	for _, i := range items {
		out = append(out, v.card(i))
	}
	return out
}

type personView struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Count int    `json:"count"`
	Limit int    `json:"limit"`
}

// handView is one person on the floor: what they hold, whether they have a
// free hand, and what the queue suggests they pull.
type handView struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Limit int    `json:"limit"`
	Items []card `json:"items"`
	Pull  *card  `json:"pull"`
}

// queue describes a lane's queue with what each position means: the reason
// for it, when the item is likely to start, and whether a date is at risk.
func (v view) queue(l flow.Lane) []card {
	cards := v.cards(l.Queue)
	for n, o := range flow.Outlooks(l, v.ws, v.norms, v.now) {
		c := &cards[n]
		c.Reason = flow.Reason(l.Queue[n], n, l.Cell, v.ws, v.now)
		if o.Known {
			days := o.Wait.Hours() / 24
			c.WaitDays = &days
		}
		c.Starts = starts(o.Wait, o.Known)
		if o.AtRisk {
			c.Risk = "Likely to miss " + l.Queue[n].Due.Format("Jan 2") + ": needs to start by " + o.StartBy.Format("Jan 2")
		}
	}
	return cards
}

// starts says when a wait is likely to end; empty when it cannot be forecast.
func starts(wait time.Duration, known bool) string {
	switch days := wait.Hours() / 24; {
	case !known:
		return ""
	case wait == 0:
		return "Can start now"
	case days < 1.5:
		return "Likely to start in about a day"
	case days < 10:
		return "Likely to start in about " + strconv.Itoa(int(days+0.5)) + " days"
	case days < 10.5:
		return "Likely to start in about a week"
	default:
		return "Likely to start in about " + strconv.Itoa(int(days/7+0.5)) + " weeks"
	}
}

// quick describes a lane's quick list: how long each has waited, and whether
// that is longer than a quick job should.
func (v view) quick(items []store.Item) []card {
	cards := v.cards(items)
	for n, i := range items {
		wait := v.now.Sub(i.Created)
		cards[n].Reason = "Quick: for anyone with a spare half hour. Waiting " + flow.Span(wait)
		if wait > time.Duration(v.ws.Policies.QuickDays*24*float64(time.Hour)) {
			cards[n].Risk = "Marked quick, but it has waited " + flow.Span(wait)
		}
	}
	return cards
}

type laneView struct {
	Cell          config.Cell  `json:"cell"`
	Throughput    float64      `json:"throughput"`
	QuickRate     float64      `json:"quickRate"`
	Measured      bool         `json:"measured"`
	Line          int          `json:"line"`
	WIPLimit      int          `json:"wipLimit"`
	Mix           []flow.Share `json:"mix"`
	Hands         []handView   `json:"hands"`
	FreeHands     int          `json:"freeHands"`
	Arrivals      int          `json:"arrivals"`      // unplanned arrivals this week
	ArrivalsUsual float64      `json:"arrivalsUsual"` // and in a usual week
	Queue         []card       `json:"queue"`
	Quick         []card       `json:"quick"`
	Doing         []card       `json:"doing"`
	Done          []card       `json:"done"`
	QuickDone     int          `json:"quickDone"`
}

func (v view) lane(l flow.Lane, items []store.Item) laneView {
	out := laneView{
		Cell: l.Cell, Throughput: l.Throughput, QuickRate: l.QuickRate, Measured: l.Measured,
		Line: l.Line, WIPLimit: l.WIPLimit, QuickDone: l.QuickDone, Hands: []handView{},
		Mix:   flow.Mix(items, v.ws, l.Cell.ID, v.now),
		Queue: v.queue(l), Quick: v.quick(l.Quick), Doing: v.cards(l.Doing), Done: v.cards(l.Done),
	}
	out.Arrivals, out.ArrivalsUsual = flow.Arrivals(items, v.ws, l.Cell.ID, v.now)
	pulls := flow.Pulls(l)
	for _, p := range l.People {
		h := handView{Login: p.Person.Login, Name: p.Person.Name, Limit: p.Person.WIPLimit, Items: v.cards(p.Items)}
		if next, ok := pulls[p.Person.Login]; ok {
			for n := range out.Queue {
				if out.Queue[n].ID == next.ID {
					h.Pull = &out.Queue[n]
				}
			}
		}
		out.FreeHands += max(0, p.Person.WIPLimit-len(p.Items))
		out.Hands = append(out.Hands, h)
	}
	return out
}

type floorView struct {
	Signals struct {
		Lead    string `json:"lead,omitempty"` // this week's triage lead
		Triage  int    `json:"triage"`
		Blocked int    `json:"blocked"`
		Aging   int    `json:"aging"`
		Over    int    `json:"over"`
	} `json:"signals"`
	Attention []flow.Alert `json:"attention"`
	Lanes     []laneView   `json:"lanes"`
}

func floorOf(items []store.Item, ws *config.Workspace, now time.Time) floorView {
	v := newView(items, ws, now)
	var out floorView
	out.Signals.Triage = len(flow.Inbox(items, ws, now))
	out.Attention = flow.Attention(items, ws, v.norms, now)
	if out.Attention == nil {
		out.Attention = []flow.Alert{}
	}
	for _, c := range ws.Cells {
		l := flow.BuildLane(items, ws, c, now)
		out.Signals.Blocked += l.Blocked()
		for _, i := range l.Doing {
			if aging, _, _ := v.norms.Aging(i, ws, now); aging && i.BlockedReason == "" {
				out.Signals.Aging++
			}
		}
		for _, p := range l.People {
			if p.Over() {
				out.Signals.Over++
			}
		}
		out.Lanes = append(out.Lanes, v.lane(l, items))
	}
	return out
}

type triageRow struct {
	Card     card   `json:"card"`
	Why      string `json:"why"`
	Urgent   bool   `json:"urgent"`
	Priority string `json:"priority,omitempty"`
	Waiting  string `json:"waiting"` // how long it has existed without a decision
	Late     bool   `json:"late"`    // longer than the triage target
}

// leadView is the person who answers for triage this week.
type leadView struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Since string `json:"since"`
	Days  int    `json:"days"`
}

func leadOf(l *store.Lead, ws *config.Workspace, now time.Time) *leadView {
	if l == nil {
		return nil
	}
	out := &leadView{Login: l.Login, Name: l.Login, Since: l.Since.Format("Mon, Jan 2"), Days: int(now.Sub(l.Since).Hours() / 24)}
	if p := ws.Person(l.Login); p != nil {
		out.Name = p.Name
	}
	return out
}

// cellOutlook says what queuing something in a cell would mean right now.
type cellOutlook struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Queued int    `json:"queued"`
	Quick  int    `json:"quick"` // on the quick list
	Free   int    `json:"free"`  // free hands
	Back   string `json:"back"`  // when the back of the queue is likely to start
	Front  string `json:"front"` // and the front
}

type inboxView struct {
	FastLane    int           `json:"fastLane"`
	TargetHours float64       `json:"targetHours"`
	QuickDays   float64       `json:"quickDays"`
	Late        int           `json:"late"`
	Rows        []triageRow   `json:"rows"`
	Cells       []cellOutlook `json:"cells"`
	Lead        *leadView     `json:"lead"`
	People      []personView  `json:"people"`
	CanTriage   bool          `json:"canTriage"` // may sort items into a queue
	CanDecide   bool          `json:"canDecide"` // may also expedite or put at the front
	CanAssign   bool          `json:"canAssign"` // may name the triage lead
}

func inboxOf(items []store.Item, ws *config.Workspace, now time.Time) inboxView {
	v := newView(items, ws, now)
	out := inboxView{FastLane: flow.FastLane(items, ws, now), TargetHours: ws.Policies.TriageAfterHours, QuickDays: ws.Policies.QuickDays,
		Rows: []triageRow{}, People: []personView{}}
	for _, t := range flow.Inbox(items, ws, now) {
		age := now.Sub(t.Item.Created)
		late := age.Hours() > ws.Policies.TriageAfterHours
		if late {
			out.Late++
		}
		c := v.card(t.Item)
		c.Cell = flow.CellOf(t.Item, ws)
		out.Rows = append(out.Rows, triageRow{c, t.Why, t.Urgent, t.Item.ExtPriority, flow.Span(age), late})
	}
	for _, c := range ws.Cells {
		l := flow.BuildLane(items, ws, c, now)
		o := cellOutlook{ID: c.ID, Name: c.Name, Queued: len(l.Queue), Quick: len(l.Quick), Free: max(0, l.WIPLimit-len(l.Doing))}
		o.Back = starts(flow.Wait(l, len(l.Queue)))
		o.Front = starts(flow.Wait(l, 0))
		out.Cells = append(out.Cells, o)
	}
	for _, p := range ws.People {
		out.People = append(out.People, personView{Login: p.Login, Name: p.Name})
	}
	return out
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

func tradeoffOf(items []store.Item) tradeoffView {
	out := tradeoffView{Candidates: []candidate{}}
	sort.SliceStable(items, func(a, b int) bool { return items[a].Created.After(items[b].Created) })
	for _, i := range items {
		if s := flow.Stage(i); s == flow.Intake || s == flow.Queued {
			out.Candidates = append(out.Candidates, candidate{i.ID, i.Title, i.Cell, s})
		}
	}
	return out
}

func previewOf(p *actions.Preview, items []store.Item, ws *config.Workspace, now time.Time) *previewView {
	v := newView(items, ws, now)
	out := &previewView{Item: v.card(p.Item), Lane: v.lane(p.Lane, items), AlreadyAbove: p.AlreadyAbove}
	if p.Falls != nil {
		c := v.card(*p.Falls)
		out.Falls = &c
	}
	return out
}

type projectView struct {
	Name      string   `json:"name"`
	URL       string   `json:"url,omitempty"`
	Kind      string   `json:"kind"` // the kind's name
	Total     int      `json:"total"`
	Done      int      `json:"done"`
	Doing     int      `json:"doing"`
	Blocked   int      `json:"blocked"`
	Queued    int      `json:"queued"`
	Cells     []string `json:"cells"`    // names of the cells with work still to do
	Quiet     string   `json:"quiet"`    // how long since anything moved
	Stalled   bool     `json:"stalled"`  // unfinished, nothing in progress, and quiet for over a week
	Forecast  string   `json:"forecast"` // plain words; empty when finished
	Remaining int      `json:"remaining"`
}

func projectsOf(items []store.Item, ws *config.Workspace, now time.Time) []projectView {
	out := []projectView{}
	for _, p := range flow.Projects(items, ws, now) {
		v := projectView{Name: p.Name, URL: p.URL, Total: p.Total, Done: p.Done, Doing: p.Doing, Blocked: p.Blocked,
			Queued: p.Queued, Remaining: p.Remaining(), Cells: []string{}, Quiet: flow.Span(now.Sub(p.LastMoved))}
		if k := ws.Kind(p.Kind); k != nil {
			v.Kind = k.Name
		}
		for _, id := range p.Cells {
			if c := ws.Cell(id); c != nil {
				v.Cells = append(v.Cells, c.Name)
			}
		}
		v.Stalled = v.Remaining > 0 && p.Doing == 0 && now.Sub(p.LastMoved) > 7*24*time.Hour
		switch weeks := p.WeeksLeft(); {
		case v.Remaining == 0:
		case weeks < 0:
			v.Forecast = "Nothing finished recently, so no forecast"
		case weeks < 1.5:
			v.Forecast = "About a week left at the recent pace"
		default:
			v.Forecast = "About " + strconv.Itoa(int(weeks+0.5)) + " weeks left at the recent pace"
		}
		out = append(out, v)
	}
	return out
}

type nextView struct {
	Card     card `json:"card"`
	Position int  `json:"position"`
	Above    bool `json:"above"`
}

type mineView struct {
	Person     *personView  `json:"person"` // nil when nobody is selected or recognised
	People     []personView `json:"people"` // everyone, for the picker
	Full       bool         `json:"full"`
	Doing      []card       `json:"doing"`
	Next       []nextView   `json:"next"`
	Quick      []card       `json:"quick"` // the cell's quick list
	Cell       string       `json:"cell,omitempty"`
	Priorities []string     `json:"priorities"`
	EditURL    string       `json:"editUrl,omitempty"` // where a manager changes the priorities
	Lead       bool         `json:"lead"`              // this person is the triage lead this week
	Triage     int          `json:"triage"`            // how many items are waiting for triage
}

func mineOf(items []store.Item, ws *config.Workspace, login, priorities string, now time.Time) mineView {
	v := newView(items, ws, now)
	out := mineView{People: []personView{}, Doing: []card{}, Next: []nextView{}, Quick: []card{}, Priorities: lines(priorities)}
	for _, p := range ws.People {
		out.People = append(out.People, personView{Login: p.Login, Name: p.Name, Limit: p.WIPLimit})
	}
	person := ws.Person(login)
	if person == nil {
		return out
	}
	m := flow.BuildMine(items, ws, *person, now)
	out.Person = &personView{person.Login, person.Name, len(m.Doing), person.WIPLimit}
	described := map[int]card{}
	if c := ws.Cell(person.Cell); c != nil {
		for _, qc := range v.quick(flow.Quick(items, c.ID)) {
			if (qc.Owner == "" || qc.Owner == person.Name) && len(out.Quick) < 5 {
				out.Quick = append(out.Quick, qc)
			}
		}
		for _, qc := range v.queue(flow.BuildLane(items, ws, *c, now)) {
			described[qc.ID] = qc
		}
	}
	out.Full = m.Full()
	out.Doing = v.cards(m.Doing)
	if c := ws.Cell(person.Cell); c != nil {
		out.Cell = c.Name
	}
	for _, n := range m.Next {
		out.Next = append(out.Next, nextView{described[n.Item.ID], n.Position, n.Above})
	}
	return out
}

// lines turns a short free-text statement into plain lines, dropping list and
// heading marks so a Markdown issue body reads cleanly.
func lines(text string) []string {
	out := []string{}
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-*#>"))
		if l != "" && len(out) < 8 {
			out = append(out, l)
		}
	}
	return out
}

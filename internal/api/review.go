package api

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/flow"
	"github.com/justinbroadbent/gemba/internal/store"
)

// The review looks back over a period: what was finished, where the effort
// went and how long things took. It is the record for a planning session, a
// report upward or a conversation with one person about their half-year.
//
// It states facts and links to them. It does not score or rank anyone, and it
// never sets one person beside another.

type weekView struct {
	Start     string `json:"start"` // "Jun 1"
	Planned   int    `json:"planned"`
	Unplanned int    `json:"unplanned"`
}

type paceView struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	Count   int    `json:"count"`   // finished items that were measured
	Typical string `json:"typical"` // half took no longer than this
	Most    string `json:"most"`    // 85 in 100 took no longer than this
}

type reviewProject struct {
	Name      string `json:"name"`
	URL       string `json:"url,omitempty"`
	Icon      string `json:"icon"`
	Finished  int    `json:"finished"`  // in the period, within scope
	Remaining int    `json:"remaining"` // across the whole team, today
	Total     int    `json:"total"`
}

type doneView struct {
	Card   card   `json:"card"`
	Closed string `json:"closed"`
	Took   string `json:"took,omitempty"`
	Note   string `json:"note,omitempty"` // why it is worth a mention
}

type reviewView struct {
	Days     int          `json:"days"`
	From     string       `json:"from"`
	To       string       `json:"to"`
	Person   *personView  `json:"person"` // nil for the whole team
	People   []personView `json:"people"`
	Finished int          `json:"finished"`
	PerWeek  float64      `json:"perWeek"`
	Mix      []flow.Share `json:"mix"`
	Weeks    []weekView   `json:"weeks"`
	Pace     []paceView   `json:"pace"`
	Dates    struct {
		Met    int `json:"met"`
		Missed int `json:"missed"`
	} `json:"dates"`
	Cells    []flow.Share    `json:"cells"` // whole team only
	Projects []reviewProject `json:"projects"`
	Notable  []doneView      `json:"notable"`
	Items    []doneView      `json:"items"` // one person only
	Holding  []card          `json:"holding"`
}

func quantile(sorted []time.Duration, q float64) time.Duration {
	return sorted[int(float64(len(sorted)-1)*q+0.5)]
}

func reviewOf(items []store.Item, ws *config.Workspace, login string, days int, now time.Time) reviewView {
	v := newView(items, ws, now)
	from := now.AddDate(0, 0, -days)
	out := reviewView{Days: days, From: from.Format("Jan 2, 2006"), To: now.Format("Jan 2, 2006"),
		People: []personView{}, Mix: []flow.Share{}, Pace: []paceView{}, Cells: []flow.Share{}, Projects: []reviewProject{}, Notable: []doneView{}, Items: []doneView{}, Holding: []card{}}
	for _, p := range ws.People {
		out.People = append(out.People, personView{Login: p.Login, Name: p.Name})
	}
	person := ws.Person(login)
	if person != nil {
		out.Person = &personView{Login: person.Login, Name: person.Name, Limit: person.WIPLimit}
	}
	mine := func(i store.Item) bool { return person == nil || i.Assignee == person.Login }

	var done []store.Item
	for _, i := range items {
		switch {
		case !mine(i):
		case i.Closed != nil && !i.Closed.Before(from) && !i.Closed.After(now):
			done = append(done, i)
		case flow.Stage(i) == flow.Doing && person != nil:
			out.Holding = append(out.Holding, v.card(i))
		}
	}
	sort.SliceStable(done, func(a, b int) bool { return done[a].Closed.After(*done[b].Closed) })
	out.Finished = len(done)
	out.PerWeek = float64(len(done)) / (float64(days) / 7)

	// Weeks run forward from the start of the period.
	weeks := (days + 6) / 7
	out.Weeks = make([]weekView, weeks)
	for n := range out.Weeks {
		out.Weeks[n].Start = from.AddDate(0, 0, n*7).Format("Jan 2")
	}

	order := []string{flow.Unplanned}
	for _, k := range ws.Kinds {
		order = append(order, k.ID)
	}
	order = append(order, flow.Other)
	counts, took := map[string]int{}, map[string][]time.Duration{}
	cells, projects := map[string]int{}, map[string]int{}
	for _, i := range done {
		t := flow.TypeOf(i, ws)
		counts[t]++
		cells[flow.CellOf(i, ws)]++
		if i.Project != "" {
			projects[i.Project]++
		}
		w := min(int(i.Closed.Sub(from).Hours()/(7*24)), weeks-1)
		if t == flow.Unplanned {
			out.Weeks[w].Unplanned++
		} else {
			out.Weeks[w].Planned++
		}
		if i.Started != nil {
			took[t] = append(took[t], i.Closed.Sub(*i.Started))
		}
		if i.Due != nil {
			if i.Closed.After(i.Due.Add(24 * time.Hour)) {
				out.Dates.Missed++
			} else {
				out.Dates.Met++
			}
		}
	}
	share := func(id, name, icon string, n int) flow.Share {
		s := flow.Share{ID: id, Name: name, Icon: icon, Count: n}
		if len(done) > 0 {
			s.Pct = float64(n) / float64(len(done)) * 100
		}
		return s
	}
	for _, t := range order {
		if t == flow.Other && counts[t] == 0 {
			continue
		}
		out.Mix = append(out.Mix, share(t, flow.TypeName(t, ws), flow.TypeIcon(t, ws), counts[t]))
		if d := took[t]; len(d) >= 3 {
			sort.Slice(d, func(a, b int) bool { return d[a] < d[b] })
			out.Pace = append(out.Pace, paceView{t, flow.TypeName(t, ws), flow.TypeIcon(t, ws), len(d),
				flow.Span(quantile(d, 0.5)), flow.Span(quantile(d, 0.85))})
		}
	}
	if person == nil {
		for _, c := range ws.Cells {
			out.Cells = append(out.Cells, share(c.ID, c.Name, "", cells[c.ID]))
		}
	}

	for _, p := range flow.Projects(items, ws, now) {
		if n := projects[p.Name]; n > 0 {
			icon := "dot"
			if k := ws.Kind(p.Kind); k != nil && k.Icon != "" {
				icon = k.Icon
			}
			out.Projects = append(out.Projects, reviewProject{p.Name, p.URL, icon, n, p.Remaining(), p.Total})
		}
	}
	sort.SliceStable(out.Projects, func(a, b int) bool { return out.Projects[a].Finished > out.Projects[b].Finished })

	row := func(i store.Item, note string) doneView {
		d := doneView{Card: v.card(i), Closed: i.Closed.Format("Jan 2"), Note: note}
		if i.Started != nil {
			d.Took = flow.Span(i.Closed.Sub(*i.Started))
		}
		return d
	}
	// Worth a mention: expedited work, and the longest pieces of planned work.
	var long []store.Item
	for _, i := range done {
		switch {
		case i.COS == "expedite":
			if len(out.Notable) < 6 {
				out.Notable = append(out.Notable, row(i, "Expedited"))
			}
		case !i.Mirrored && i.Started != nil:
			long = append(long, i)
		}
	}
	sort.SliceStable(long, func(a, b int) bool {
		return long[a].Closed.Sub(*long[a].Started) > long[b].Closed.Sub(*long[b].Started)
	})
	for n := 0; n < len(long) && n < 5; n++ {
		out.Notable = append(out.Notable, row(long[n], "Among the longest pieces of planned work"))
	}
	if person != nil {
		for n := 0; n < len(done) && n < 500; n++ {
			out.Items = append(out.Items, row(done[n], ""))
		}
	}
	return out
}

// review is open to everyone in the workspace, like the rest of what Gemba
// shows: it contains nothing the issues themselves do not.
func (s *Server) review(w http.ResponseWriter, r *http.Request, q *request) {
	items, err := q.store.Items(r.Context())
	if err != nil {
		s.storeError(w, q, err)
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 7 || days > 731 {
		days = 30
	}
	writeJSON(w, 200, reviewOf(items, q.ws, r.URL.Query().Get("as"), days, q.now))
}

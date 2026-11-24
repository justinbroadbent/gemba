// Package api serves the JSON API and the built web app.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/justinbroadbent/gemba/internal/actions"
	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/notify"
	"github.com/justinbroadbent/gemba/internal/store"
)

type Server struct {
	Config *config.Config
	Stores map[string]store.Store // by workspace slug
	// Notify announces decisions to the team's chat, by workspace slug. Optional.
	Notify map[string]notify.Notifier
	Now    func() time.Time
	// RequireAuth refuses requests that carry no signed-in identity. Turn it on
	// in every deployed environment.
	RequireAuth bool
	Static      fs.FS // the built web app; may be nil
	// Logo replaces the built-in mark when set.
	Logo     []byte
	LogoType string // its content type
	Log      *slog.Logger
}

// identity is who is asking. Sign-in is delegated to the platform: Azure
// Container Apps authentication puts the Entra sign-in name in this header and
// strips any copy sent by the client.
const principalHeader = "X-MS-CLIENT-PRINCIPAL-NAME"

type identity struct {
	Name  string
	Local bool // no platform sign-in; only possible when RequireAuth is off
}

func (s *Server) identify(r *http.Request) (identity, bool) {
	if name := strings.TrimSpace(r.Header.Get(principalHeader)); name != "" {
		return identity{Name: name}, true
	}
	if s.RequireAuth {
		return identity{}, false
	}
	return identity{Name: "local", Local: true}, true
}

// rights are what an identity may do in a workspace. Everyone with access can
// read. People on the team can triage. Managers can also reorder queues, record
// trade-offs and name the triage lead.
type rights struct {
	read, team, manage bool
	login              string // the person they are on this team, if recognised
}

const (
	toRead = iota
	toTriage
	toManage
)

func access(ws *config.Workspace, id identity) rights {
	if id.Local {
		return rights{read: true, team: true, manage: true}
	}
	name := strings.ToLower(id.Name)
	r := rights{manage: slices.Contains(ws.Managers, name)}
	r.read = r.manage || slices.Contains(ws.Members, name)
	if p := ws.PersonBySignIn(id.Name); p != nil && r.read {
		r.login = p.Login
	}
	r.team = r.manage || r.login != ""
	return r
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("GET /api/logo", s.logo)
	mux.HandleFunc("GET /api/w/{ws}/floor", s.in(toRead, s.floor))
	mux.HandleFunc("GET /api/w/{ws}/inbox", s.in(toRead, s.inbox))
	mux.HandleFunc("POST /api/w/{ws}/inbox/{id}", s.in(toTriage, s.triage))
	mux.HandleFunc("POST /api/w/{ws}/triage-lead", s.in(toManage, s.setLead))
	mux.HandleFunc("POST /api/w/{ws}/queues/{cell}/move", s.in(toManage, s.move))
	mux.HandleFunc("GET /api/w/{ws}/projects", s.in(toRead, s.projects))
	mux.HandleFunc("GET /api/w/{ws}/mine", s.in(toRead, s.mine))
	mux.HandleFunc("POST /api/w/{ws}/items/{id}/wait", s.in(toTriage, s.wait))
	mux.HandleFunc("POST /api/w/{ws}/items/{id}/resume", s.in(toTriage, s.resume))
	mux.HandleFunc("GET /api/w/{ws}/pulse", s.in(toRead, s.pulse))
	mux.HandleFunc("POST /api/w/{ws}/refresh", s.in(toTriage, s.refresh))
	mux.HandleFunc("GET /api/w/{ws}/review", s.in(toRead, s.review))
	mux.HandleFunc("GET /api/w/{ws}/tradeoff", s.in(toRead, s.tradeoff))
	mux.HandleFunc("POST /api/w/{ws}/tradeoff", s.in(toManage, s.decide))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { fail(w, 404, "Not found") })
	mux.HandleFunc("/", s.static)
	return mux
}

type request struct {
	ws    *config.Workspace
	store store.Store
	who   identity
	can   rights
	now   time.Time
}

// in resolves the workspace and checks access before calling fn. A workspace
// the caller cannot read is reported as missing, so its existence is not leaked.
func (s *Server) in(need int, fn func(http.ResponseWriter, *http.Request, *request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		who, ok := s.identify(r)
		if !ok {
			fail(w, 401, "Sign-in required")
			return
		}
		ws := s.Config.Workspace(r.PathValue("ws"))
		if ws == nil {
			fail(w, 404, "No such workspace")
			return
		}
		can := access(ws, who)
		if !can.read {
			fail(w, 404, "No such workspace")
			return
		}
		if need > toRead {
			if (need == toTriage && !can.team) || (need == toManage && !can.manage) {
				fail(w, 403, "You do not have access to change this")
				return
			}
			// A custom header cannot be sent by a cross-site form, which blocks request forgery.
			if r.Header.Get("X-Gemba") == "" {
				fail(w, 403, "Missing X-Gemba header")
				return
			}
		}
		fn(w, r, &request{ws: ws, store: s.Stores[ws.Slug], who: who, can: can, now: s.Now()})
	}
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	who, ok := s.identify(r)
	if !ok {
		fail(w, 401, "Sign-in required")
		return
	}
	type workspace struct {
		Slug         string        `json:"slug"`
		Name         string        `json:"name"`
		CanWrite     bool          `json:"canWrite"`
		CanTriage    bool          `json:"canTriage"`
		Cells        []config.Cell `json:"cells"`
		Kinds        []config.Kind `json:"kinds"`
		HorizonWeeks int           `json:"horizonWeeks"`
		Me           string        `json:"me,omitempty"` // the signed-in person's login here, if recognised
	}
	out := struct {
		User  string `json:"user"`
		Brand struct {
			Name string `json:"name"`
			Logo string `json:"logo,omitempty"`
		} `json:"brand"`
		Theme      config.Theme `json:"theme"`
		COS        []string     `json:"cos"`
		Workspaces []workspace  `json:"workspaces"`
	}{User: who.Name, Theme: s.Config.Theme, COS: config.COS, Workspaces: []workspace{}}
	out.Brand.Name = s.Config.Brand.Name
	if s.Logo != nil {
		out.Brand.Logo = "/api/logo"
	}
	for _, ws := range s.Config.Workspaces {
		if can := access(ws, who); can.read {
			out.Workspaces = append(out.Workspaces, workspace{ws.Slug, ws.Name, can.manage, can.team, ws.Cells, ws.Kinds, ws.Policies.HorizonWeeks, can.login})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) logo(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identify(r); !ok {
		fail(w, 401, "Sign-in required")
		return
	}
	if s.Logo == nil {
		fail(w, 404, "No custom logo is configured")
		return
	}
	w.Header().Set("Content-Type", s.LogoType)
	// An SVG can carry script; this stops it running if the file is opened directly.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(s.Logo)
}

func (s *Server) floor(w http.ResponseWriter, r *http.Request, q *request) {
	items, err := q.store.Items(r.Context())
	if err != nil {
		s.storeError(w, q, err)
		return
	}
	out := floorOf(items, q.ws, q.now)
	if l := leadOf(s.lead(r, q), q.ws, q.now); l != nil {
		out.Signals.Lead = l.Name
	}
	writeJSON(w, 200, out)
}

// lead is this week's triage lead, or nil. A failure to read it is logged and
// treated as nobody, so the pages that mention it still load.
func (s *Server) lead(r *http.Request, q *request) *store.Lead {
	rota, ok := q.store.(store.Rota)
	if !ok {
		return nil
	}
	l, err := rota.TriageLead(r.Context())
	if err != nil {
		s.Log.Error("triage lead", "workspace", q.ws.Slug, "err", err)
		return nil
	}
	return l
}

// decides reports whether the caller holds the authority to expedite or to put
// work at the front: a manager, or the triage lead acting for them this week.
func (s *Server) decides(r *http.Request, q *request) bool {
	if q.can.manage {
		return true
	}
	l := s.lead(r, q)
	return l != nil && q.can.login != "" && l.Login == q.can.login
}

func (s *Server) setLead(w http.ResponseWriter, r *http.Request, q *request) {
	var in struct {
		Login string `json:"login"`
	}
	if _, ok := body(w, r, "", &in); !ok {
		return
	}
	rota, ok := q.store.(store.Rota)
	if !ok || (q.ws.Store == "github" && q.ws.GitHub.TriageIssue == 0) {
		fail(w, 400, "This workspace has nowhere to record the triage lead. Set triage_issue in its configuration.")
		return
	}
	if q.ws.Person(in.Login) == nil {
		fail(w, 400, "Choose someone on the team")
		return
	}
	if err := rota.SetTriageLead(r.Context(), in.Login); err != nil {
		s.storeError(w, q, err)
		return
	}
	s.audit(q, "triage-lead", "login", in.Login)
	s.announce(q, "Triage lead", q.ws.Person(in.Login).Name+" is the triage lead from today.")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request, q *request) {
	items, err := q.store.Items(r.Context())
	if err != nil {
		s.storeError(w, q, err)
		return
	}
	out := inboxOf(items, q.ws, q.now)
	out.Lead = leadOf(s.lead(r, q), q.ws, q.now)
	out.CanTriage, out.CanDecide = q.can.team, q.can.team && s.decides(r, q)
	_, rota := q.store.(store.Rota)
	out.CanAssign = q.can.manage && rota && (q.ws.Store != "github" || q.ws.GitHub.TriageIssue > 0)
	writeJSON(w, 200, out)
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request, q *request) {
	items, err := q.store.Items(r.Context())
	if err != nil {
		s.storeError(w, q, err)
		return
	}
	writeJSON(w, 200, map[string]any{"projects": projectsOf(items, q.ws, q.now)})
}

// mine is one person's work. Anyone in the workspace may look at anyone's:
// it shows only what the board already shows, and nothing historical.
func (s *Server) mine(w http.ResponseWriter, r *http.Request, q *request) {
	items, err := q.store.Items(r.Context())
	if err != nil {
		s.storeError(w, q, err)
		return
	}
	login := r.URL.Query().Get("as")
	if login == "" {
		if p := q.ws.PersonBySignIn(q.who.Name); p != nil {
			login = p.Login
		}
	}
	priorities := q.ws.Priorities
	if p, ok := q.store.(store.Prioritizer); ok {
		if text, err := p.Priorities(r.Context()); err != nil {
			s.Log.Error("priorities", "workspace", q.ws.Slug, "err", err)
		} else if text != "" {
			priorities = text
		}
	}
	out := mineOf(items, q.ws, login, priorities, q.now)
	if g := q.ws.GitHub; q.can.manage && q.ws.Store == "github" && g.PrioritiesIssue > 0 {
		out.EditURL = "https://github.com/" + g.Owner + "/" + g.Repo + "/issues/" + strconv.Itoa(g.PrioritiesIssue)
	}
	out.CanAct = out.Person != nil && (q.can.manage || q.can.login == out.Person.Login)
	if l := s.lead(r, q); l != nil && out.Person != nil && l.Login == out.Person.Login {
		out.Lead, out.Triage = true, len(inboxOf(items, q.ws, q.now).Rows)
	}
	writeJSON(w, 200, out)
}

func (s *Server) triage(w http.ResponseWriter, r *http.Request, q *request) {
	var in struct {
		Cell string `json:"cell"`
		COS  string `json:"cos"`
		Kind string `json:"kind"`
		Top  bool   `json:"top"`
		Due  string `json:"due"`
	}
	id, ok := body(w, r, "id", &in)
	if !ok {
		return
	}
	if (in.Top || in.COS == "expedite") && !s.decides(r, q) {
		fail(w, 403, "Only the triage lead or a manager can expedite work or put it at the front. Queue it at the back and tell them.")
		return
	}
	if err := actions.Triage(r.Context(), q.store, q.ws, id, in.Cell, in.COS, in.Kind, in.Top, in.Due); err != nil {
		s.actionError(w, q, err)
		return
	}
	s.audit(q, "triage", "item", id, "cell", in.Cell, "cos", in.COS, "kind", in.Kind, "top", in.Top)
	if in.COS == "expedite" {
		s.announce(q, "Expedited", "#"+strconv.Itoa(id)+" was expedited in "+q.ws.Cell(in.Cell).Name+". It goes ahead of everything else there.")
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// pulse is the little a browser needs to notice change: what is waiting for
// triage, and what one person holds, has parked and would pull next. The
// browser compares one pulse with the last and announces the difference.
func (s *Server) pulse(w http.ResponseWriter, r *http.Request, q *request) {
	items, err := q.store.Items(r.Context())
	if err != nil {
		s.storeError(w, q, err)
		return
	}
	type ref struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	}
	out := struct {
		As      string `json:"as"`
		Triage  []ref  `json:"triage"`
		Doing   []ref  `json:"doing"`
		Waiting []ref  `json:"waiting"`
		Next    *ref   `json:"next"`
		Blocked []ref  `json:"blocked"` // in this person's hands and blocked
		Risk    []ref  `json:"risk"`    // coming to this person and likely to miss a date
		Chase   []ref  `json:"chase"`   // this person's waiting items that are due a chase
		Alerts  []ref  `json:"alerts"`  // for managers: what on the floor needs someone to step in
	}{Triage: []ref{}, Doing: []ref{}, Waiting: []ref{}, Blocked: []ref{}, Risk: []ref{}, Chase: []ref{}, Alerts: []ref{}}
	for _, row := range inboxOf(items, q.ws, q.now).Rows {
		out.Triage = append(out.Triage, ref{row.Card.ID, row.Card.Title})
	}
	login := r.URL.Query().Get("as")
	if login == "" {
		login = q.can.login
	}
	if mine := mineOf(items, q.ws, login, "", q.now); mine.Person != nil {
		out.As = mine.Person.Login
		for _, c := range mine.Doing {
			out.Doing = append(out.Doing, ref{c.ID, c.Title})
			if c.Blocked != "" {
				out.Blocked = append(out.Blocked, ref{c.ID, c.Title})
			}
		}
		for _, c := range mine.Waiting {
			out.Waiting = append(out.Waiting, ref{c.ID, c.Title})
			if c.Chase {
				out.Chase = append(out.Chase, ref{c.ID, c.Title})
			}
		}
		for _, n := range mine.Next {
			if n.Card.Risk != "" {
				out.Risk = append(out.Risk, ref{n.Card.ID, n.Card.Risk + ": " + n.Card.Title})
			}
		}
		if len(mine.Next) > 0 {
			out.Next = &ref{mine.Next[0].Card.ID, mine.Next[0].Card.Title}
		}
	}
	if q.can.manage { // only the serious ones, and only for someone who can act on them
		for _, a := range floorOf(items, q.ws, q.now).Attention {
			if a.Level == "stop" && a.ID != 0 {
				out.Alerts = append(out.Alerts, ref{a.ID, a.Text})
			}
		}
	}
	writeJSON(w, 200, out)
}

// refresh drops what Gemba remembers of the work items, so the next read goes
// to the source. For when someone has just created or changed something there.
func (s *Server) refresh(w http.ResponseWriter, r *http.Request, q *request) {
	if c, ok := q.store.(interface{ Invalidate() }); ok {
		c.Invalidate()
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) wait(w http.ResponseWriter, r *http.Request, q *request) {
	var in struct {
		On string `json:"on"`
	}
	id, ok := body(w, r, "id", &in)
	if !ok {
		return
	}
	if err := actions.Wait(r.Context(), q.store, id, in.On, q.can.login, q.can.manage); err != nil {
		s.actionError(w, q, err)
		return
	}
	s.audit(q, "wait", "item", id, "on", in.On)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request, q *request) {
	var in struct{}
	id, ok := body(w, r, "id", &in)
	if !ok {
		return
	}
	if err := actions.Resume(r.Context(), q.store, id, q.can.login, q.can.manage); err != nil {
		s.actionError(w, q, err)
		return
	}
	s.audit(q, "resume", "item", id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) move(w http.ResponseWriter, r *http.Request, q *request) {
	var in struct {
		ID     int  `json:"id"`
		Before *int `json:"before"`
	}
	if _, ok := body(w, r, "", &in); !ok {
		return
	}
	cell := r.PathValue("cell")
	if err := actions.Move(r.Context(), q.store, q.ws, cell, in.ID, in.Before); err != nil {
		s.actionError(w, q, err)
		return
	}
	s.audit(q, "move", "item", in.ID, "cell", cell, "before", in.Before)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) tradeoff(w http.ResponseWriter, r *http.Request, q *request) {
	items, err := q.store.Items(r.Context())
	if err != nil {
		s.storeError(w, q, err)
		return
	}
	out := tradeoffOf(items, q.ws)
	if raw := r.URL.Query().Get("item"); raw != "" {
		id, _ := strconv.Atoi(raw)
		p, err := actions.PreviewTradeoff(r.Context(), q.store, q.ws, id, r.URL.Query().Get("cell"), q.now)
		if err != nil {
			s.actionError(w, q, err)
			return
		}
		out.Preview = previewOf(p, items, q.ws, q.now)
	}
	writeJSON(w, 200, out)
}

func (s *Server) decide(w http.ResponseWriter, r *http.Request, q *request) {
	var in struct {
		Item         int    `json:"item"`
		Cell         string `json:"cell"`
		Pause        *int   `json:"pause"`
		AuthorizedBy string `json:"authorizedBy"`
	}
	if _, ok := body(w, r, "", &in); !ok {
		return
	}
	notes, err := actions.Decide(r.Context(), q.store, q.ws, in.Item, in.Cell, in.Pause, in.AuthorizedBy, q.now)
	if err != nil {
		s.actionError(w, q, err)
		return
	}
	s.audit(q, "tradeoff", "item", in.Item, "cell", in.Cell, "pause", in.Pause, "authorized_by", in.AuthorizedBy)
	var said []string
	for _, n := range notes {
		said = append(said, "#"+strconv.Itoa(n.ID)+" "+n.Title+": "+n.Text)
	}
	s.announce(q, "Trade-off recorded", strings.Join(said, "\n\n"))
	writeJSON(w, 200, map[string]any{"written": notes})
}

// static serves the built web app, falling back to index.html so that
// client-side routes survive a reload.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.Static == nil {
		fail(w, 404, "The web app has not been built. Run: npm --prefix web run build")
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if info, err := fs.Stat(s.Static, name); name == "" || err != nil || info.IsDir() {
		name = "index.html"
	}
	b, err := fs.ReadFile(s.Static, name)
	if err != nil {
		fail(w, 404, "The web app has not been built. Run: npm --prefix web run build")
		return
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(w, r, name, time.Time{}, strings.NewReader(string(b)))
}

// announce tells the team's chat about a decision. It never holds up or fails
// the request: the decision is already recorded where it matters.
func (s *Server) announce(q *request, title, text string) {
	n, ok := s.Notify[q.ws.Slug]
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := n.Notify(ctx, notify.Event{Workspace: q.ws.Name, Title: title, Text: text}); err != nil {
			s.Log.Error("notify", "workspace", q.ws.Slug, "err", err)
		}
	}()
}

func (s *Server) audit(q *request, action string, attrs ...any) {
	s.Log.Info("audit", append([]any{"action", action, "workspace", q.ws.Slug, "actor", q.who.Name}, attrs...)...)
}

func (s *Server) actionError(w http.ResponseWriter, q *request, err error) {
	var ue *actions.Error
	if errors.As(err, &ue) {
		code := 400
		if ue.Conflict {
			code = 409
		}
		fail(w, code, ue.Msg)
		return
	}
	s.storeError(w, q, err)
}

func (s *Server) storeError(w http.ResponseWriter, q *request, err error) {
	s.Log.Error("store", "workspace", q.ws.Slug, "err", err)
	fail(w, 502, "Could not reach the work items for this workspace. Try again shortly.")
}

// body decodes a JSON request body and, when idParam is set, a numeric path value.
func body(w http.ResponseWriter, r *http.Request, idParam string, into any) (int, bool) {
	id := 0
	if idParam != "" {
		var err error
		if id, err = strconv.Atoi(r.PathValue(idParam)); err != nil {
			fail(w, 400, "Bad item number")
			return 0, false
		}
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(into); err != nil {
		fail(w, 400, "Bad request body")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

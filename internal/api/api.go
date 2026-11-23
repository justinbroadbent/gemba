// Package api serves the JSON API and the built web app.
package api

import (
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
	"github.com/justinbroadbent/gemba/internal/store"
)

type Server struct {
	Config *config.Config
	Stores map[string]store.Store // by workspace slug
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

// access reports what an identity may do in a workspace.
func access(ws *config.Workspace, id identity) (read, write bool) {
	if id.Local {
		return true, true
	}
	name := strings.ToLower(id.Name)
	write = slices.Contains(ws.Managers, name)
	return write || slices.Contains(ws.Members, name), write
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("GET /api/logo", s.logo)
	mux.HandleFunc("GET /api/w/{ws}/floor", s.in(false, s.floor))
	mux.HandleFunc("GET /api/w/{ws}/inbox", s.in(false, s.inbox))
	mux.HandleFunc("POST /api/w/{ws}/inbox/{id}", s.in(true, s.triage))
	mux.HandleFunc("POST /api/w/{ws}/queues/{cell}/move", s.in(true, s.move))
	mux.HandleFunc("GET /api/w/{ws}/tradeoff", s.in(false, s.tradeoff))
	mux.HandleFunc("POST /api/w/{ws}/tradeoff", s.in(true, s.decide))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { fail(w, 404, "Not found") })
	mux.HandleFunc("/", s.static)
	return mux
}

type request struct {
	ws    *config.Workspace
	store store.Store
	who   identity
	now   time.Time
}

// in resolves the workspace and checks access before calling fn. A workspace
// the caller cannot read is reported as missing, so its existence is not leaked.
func (s *Server) in(write bool, fn func(http.ResponseWriter, *http.Request, *request)) http.HandlerFunc {
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
		canRead, canWrite := access(ws, who)
		if !canRead {
			fail(w, 404, "No such workspace")
			return
		}
		if write {
			if !canWrite {
				fail(w, 403, "You have read-only access to this workspace")
				return
			}
			// A custom header cannot be sent by a cross-site form, which blocks request forgery.
			if r.Header.Get("X-Gemba") == "" {
				fail(w, 403, "Missing X-Gemba header")
				return
			}
		}
		fn(w, r, &request{ws: ws, store: s.Stores[ws.Slug], who: who, now: s.Now()})
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
		Cells        []config.Cell `json:"cells"`
		HorizonWeeks int           `json:"horizonWeeks"`
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
		if canRead, canWrite := access(ws, who); canRead {
			out.Workspaces = append(out.Workspaces, workspace{ws.Slug, ws.Name, canWrite, ws.Cells, ws.Policies.HorizonWeeks})
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
	writeJSON(w, 200, floorOf(items, q.ws, q.now))
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request, q *request) {
	items, err := q.store.Items(r.Context())
	if err != nil {
		s.storeError(w, q, err)
		return
	}
	writeJSON(w, 200, inboxOf(items, q.ws, q.now))
}

func (s *Server) triage(w http.ResponseWriter, r *http.Request, q *request) {
	var in struct {
		Cell string `json:"cell"`
		COS  string `json:"cos"`
		Top  bool   `json:"top"`
	}
	id, ok := body(w, r, "id", &in)
	if !ok {
		return
	}
	if err := actions.Triage(r.Context(), q.store, q.ws, id, in.Cell, in.COS, in.Top); err != nil {
		s.actionError(w, q, err)
		return
	}
	s.audit(q, "triage", "item", id, "cell", in.Cell, "cos", in.COS, "top", in.Top)
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
		out.Preview = previewOf(p, q.ws, q.now)
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

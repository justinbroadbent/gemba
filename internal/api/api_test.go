package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/store"
)

var now = time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)

// server builds the two built-in demo workspaces. morgan manages infra and can
// read nothing else; priya is a read-only member of infra; lee manages apps.
func server(t *testing.T, requireAuth bool) http.Handler {
	t.Helper()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Workspace("infra").Managers = []string{"morgan@example.com"}
	cfg.Workspace("infra").Members = []string{"priya@example.com"}
	cfg.Workspace("apps").Managers = []string{"lee@example.com"}
	stores := map[string]store.Store{}
	for _, ws := range cfg.Workspaces {
		var cells []store.DemoCell
		for _, c := range ws.Cells {
			cells = append(cells, store.DemoCell{ID: c.ID})
		}
		stores[ws.Slug] = store.NewDemo(cells, 4, now)
	}
	s := &Server{Config: cfg, Stores: stores, Now: func() time.Time { return now }, RequireAuth: requireAuth,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return s.Handler()
}

type call struct {
	method, path, user, body string
	noHeader                 bool
}

func do(h http.Handler, c call) *httptest.ResponseRecorder {
	req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	if c.user != "" {
		req.Header.Set(principalHeader, c.user)
	}
	if c.method == "POST" && !c.noHeader {
		req.Header.Set("X-Gemba", "1")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Every workspace route, with a body that would be valid for a manager.
func routes(ws string) []call {
	return []call{
		{method: "GET", path: "/api/w/" + ws + "/floor"},
		{method: "GET", path: "/api/w/" + ws + "/inbox"},
		{method: "GET", path: "/api/w/" + ws + "/tradeoff"},
		{method: "POST", path: "/api/w/" + ws + "/inbox/101", body: `{"cell":"x","cos":"standard"}`},
		{method: "POST", path: "/api/w/" + ws + "/queues/x/move", body: `{"id":101}`},
		{method: "POST", path: "/api/w/" + ws + "/tradeoff", body: `{"item":101,"cell":"x","authorizedBy":"a"}`},
	}
}

func TestLocalModeCanReadEverything(t *testing.T) {
	h := server(t, false)
	for _, ws := range []string{"infra", "apps"} {
		for _, c := range routes(ws) {
			if c.method != "GET" {
				continue
			}
			if rec := do(h, c); rec.Code != 200 {
				t.Errorf("%s %s = %d: %s", c.method, c.path, rec.Code, rec.Body)
			}
		}
	}
}

func TestRequireAuthRefusesAnonymous(t *testing.T) {
	h := server(t, true)
	for _, c := range append(routes("infra"), call{method: "GET", path: "/api/session"}) {
		if rec := do(h, c); rec.Code != 401 {
			t.Errorf("%s %s = %d, want 401", c.method, c.path, rec.Code)
		}
	}
}

func TestWorkspacesAreIsolated(t *testing.T) {
	h := server(t, true)
	// Each user against the workspace they do not belong to: every route must look missing.
	for user, foreign := range map[string]string{"morgan@example.com": "apps", "lee@example.com": "infra", "priya@example.com": "apps", "stranger@example.com": "infra"} {
		for _, c := range routes(foreign) {
			c.user = user
			if rec := do(h, c); rec.Code != 404 {
				t.Errorf("%s: %s %s = %d, want 404", user, c.method, c.path, rec.Code)
			}
		}
	}

	rec := do(h, call{method: "GET", path: "/api/session", user: "Morgan@Example.com"})
	var session struct {
		Workspaces []struct {
			Slug     string
			CanWrite bool
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if len(session.Workspaces) != 1 || session.Workspaces[0].Slug != "infra" || !session.Workspaces[0].CanWrite {
		t.Errorf("morgan should see only infra, as a manager: %+v", session.Workspaces)
	}
}

func TestMembersAreReadOnly(t *testing.T) {
	h := server(t, true)
	for _, c := range routes("infra") {
		c.user = "priya@example.com"
		want := 200
		if c.method == "POST" {
			want = 403
		}
		if rec := do(h, c); rec.Code != want {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, rec.Code, want)
		}
	}
}

func TestWritesNeedTheCustomHeader(t *testing.T) {
	h := server(t, true)
	for _, c := range routes("infra") {
		if c.method != "POST" {
			continue
		}
		c.user, c.noHeader = "morgan@example.com", true
		if rec := do(h, c); rec.Code != 403 {
			t.Errorf("%s %s without X-Gemba = %d, want 403", c.method, c.path, rec.Code)
		}
	}
}

func TestManagerCanTriageAndSeesConflicts(t *testing.T) {
	h := server(t, true)
	rec := do(h, call{method: "GET", path: "/api/w/infra/inbox", user: "morgan@example.com"})
	var inbox struct {
		Rows []struct{ Card struct{ ID int } }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &inbox); err != nil || len(inbox.Rows) == 0 {
		t.Fatalf("inbox: %v %s", err, rec.Body)
	}
	path := "/api/w/infra/inbox/" + itoa(inbox.Rows[0].Card.ID)
	body := `{"cell":"network","cos":"expedite","top":true}`
	if rec := do(h, call{method: "POST", path: path, user: "morgan@example.com", body: body}); rec.Code != 200 {
		t.Fatalf("triage = %d: %s", rec.Code, rec.Body)
	}
	if rec := do(h, call{method: "POST", path: path, user: "morgan@example.com", body: body}); rec.Code != 409 {
		t.Errorf("triaging twice = %d, want 409", rec.Code)
	}
	if rec := do(h, call{method: "POST", path: "/api/w/infra/queues/network/move", user: "morgan@example.com", body: `{"id":1}`}); rec.Code != 409 {
		t.Errorf("moving a missing item = %d, want 409", rec.Code)
	}
}

// rin is on the team: she can sort work into a queue, but only the triage lead
// or a manager can expedite it or put it at the front.
func TestTheTeamCanTriageAndTheLeadDecides(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	ws := cfg.Workspace("infra")
	ws.Managers, ws.Members = []string{"morgan@example.com"}, []string{"rin@example.com", "priya@example.com"}
	ws.People[0].SignIn = "rin@example.com"
	demo := store.NewDemo([]store.DemoCell{{ID: ws.Cells[0].ID}}, 4, now)
	s := &Server{Config: cfg, Stores: map[string]store.Store{"infra": demo}, Now: func() time.Time { return now },
		RequireAuth: true, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h := s.Handler()

	var inbox struct {
		Rows      []struct{ Card struct{ ID int } }
		CanTriage bool
		CanDecide bool
		Lead      *struct{ Login string }
	}
	read := func(user string) {
		inbox.Lead = nil
		rec := do(h, call{method: "GET", path: "/api/w/infra/inbox", user: user})
		if err := json.Unmarshal(rec.Body.Bytes(), &inbox); err != nil || len(inbox.Rows) < 3 {
			t.Fatalf("inbox: %v %s", err, rec.Body)
		}
	}
	read("rin@example.com")
	if !inbox.CanTriage || inbox.CanDecide || inbox.Lead != nil {
		t.Fatalf("rin can triage but not decide, and nobody leads yet: %+v", inbox)
	}
	post := func(user string, n int, body string) int {
		return do(h, call{method: "POST", path: "/api/w/infra/inbox/" + itoa(inbox.Rows[n].Card.ID), user: user, body: body}).Code
	}
	cell := ws.Cells[0].ID
	if code := post("rin@example.com", 0, `{"cell":"`+cell+`","cos":"standard"}`); code != 200 {
		t.Errorf("a team member queues at the back = %d, want 200", code)
	}
	if code := post("rin@example.com", 1, `{"cell":"`+cell+`","cos":"standard","top":true}`); code != 403 {
		t.Errorf("a team member puts at the front = %d, want 403", code)
	}
	if code := post("rin@example.com", 1, `{"cell":"`+cell+`","cos":"expedite"}`); code != 403 {
		t.Errorf("a team member expedites = %d, want 403", code)
	}
	if code := post("priya@example.com", 1, `{"cell":"`+cell+`","cos":"standard"}`); code != 403 {
		t.Errorf("a reader who is not on the team triages = %d, want 403", code)
	}

	lead := func(user, login string) int {
		return do(h, call{method: "POST", path: "/api/w/infra/triage-lead", user: user, body: `{"login":"` + login + `"}`}).Code
	}
	if code := lead("rin@example.com", ws.People[0].Login); code != 403 {
		t.Errorf("a team member names the lead = %d, want 403", code)
	}
	if code := lead("morgan@example.com", "nobody"); code != 400 {
		t.Errorf("naming someone who is not on the team = %d, want 400", code)
	}
	if code := lead("morgan@example.com", ws.People[0].Login); code != 200 {
		t.Fatalf("the manager names the lead = %d, want 200", code)
	}
	read("priya@example.com")
	if inbox.Lead == nil || inbox.Lead.Login != ws.People[0].Login || inbox.CanTriage {
		t.Errorf("everyone can see who leads: %+v", inbox)
	}
	if code := post("rin@example.com", 1, `{"cell":"`+cell+`","cos":"expedite","top":true}`); code != 200 {
		t.Errorf("the lead expedites = %d, want 200", code)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

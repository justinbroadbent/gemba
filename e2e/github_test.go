// Package e2e proves Gemba against a real GitHub project.
//
// It drives every write through Gemba's own HTTP API, then reads the result
// back from GitHub with a plain query that shares none of Gemba's mapping
// code. It creates its own issues and removes them afterwards.
//
// It runs only when asked, against a project you can afford to change:
//
//	GEMBA_E2E=1 GEMBA_CONFIG=gemba.local.yml GITHUB_TOKEN=... go test ./e2e -v
//
// GEMBA_E2E_WS names the workspace (default "testfloor"). That workspace needs
// at least one person whose login is a real GitHub user, and a triage_issue.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/justinbroadbent/gemba/internal/api"
	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/store"
)

var ctx = context.Background()

type rig struct {
	t       *testing.T
	ws      *config.Workspace
	gh      *store.GitHub // the store Gemba uses
	raw     *store.GitHub // a second client, used only for plain queries
	h       http.Handler
	project string
	repo    string
}

func setup(t *testing.T) *rig {
	if os.Getenv("GEMBA_E2E") != "1" {
		t.Skip("set GEMBA_E2E=1 to run against a real GitHub project")
	}
	cfg, err := config.Load(os.Getenv("GEMBA_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	slug := os.Getenv("GEMBA_E2E_WS")
	if slug == "" {
		slug = "testfloor"
	}
	ws := cfg.Workspace(slug)
	if ws == nil || ws.Store != "github" {
		t.Fatalf("workspace %q must exist and use the github store", slug)
	}
	token := os.Getenv("GITHUB_TOKEN")
	r := &rig{t: t, ws: ws, gh: store.NewGitHub(ws, token), raw: store.NewGitHub(ws, token)}
	r.gh.TTL = 0                                     // always read GitHub, never Gemba's memory of what it wrote
	if r.ws, err = r.gh.Workspace(ctx); err != nil { // cells, kinds and people as GitHub describes them
		t.Fatal(err)
	}
	for _, p := range r.ws.Problems {
		t.Errorf("setup problem: %s", p)
	}
	srv := &api.Server{Config: cfg, Stores: map[string]store.Store{slug: r.gh},
		Now: func() time.Time { return time.Now().UTC() }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	r.h = srv.Handler()
	if r.project, _, err = r.gh.Project(ctx); err != nil {
		t.Fatal(err)
	}
	var out struct{ Repository struct{ ID string } }
	r.query(`query($o:String!,$r:String!){repository(owner:$o,name:$r){id}}`, map[string]any{"o": ws.GitHub.Owner, "r": ws.GitHub.Repo}, &out)
	r.repo = out.Repository.ID
	return r
}

func (r *rig) query(q string, vars map[string]any, out any) {
	r.t.Helper()
	if err := r.raw.GraphQL(ctx, q, vars, out); err != nil {
		r.t.Fatal(err)
	}
}

// issue creates an issue on the project and removes both when the test ends.
func (r *rig) issue(title string) (number int, node, item string) {
	r.t.Helper()
	var made struct {
		CreateIssue struct {
			Issue struct {
				ID     string
				Number int
			}
		}
	}
	r.query(`mutation($r:ID!,$t:String!){createIssue(input:{repositoryId:$r,title:$t,body:"Created by Gemba's end-to-end test. Safe to delete."}){issue{id number}}}`,
		map[string]any{"r": r.repo, "t": title}, &made)
	node, number = made.CreateIssue.Issue.ID, made.CreateIssue.Issue.Number
	var added struct {
		AddProjectV2ItemById struct{ Item struct{ ID string } }
	}
	r.query(`mutation($p:ID!,$c:ID!){addProjectV2ItemById(input:{projectId:$p,contentId:$c}){item{id}}}`,
		map[string]any{"p": r.project, "c": node}, &added)
	item = added.AddProjectV2ItemById.Item.ID
	r.t.Cleanup(func() {
		_ = r.raw.GraphQL(ctx, `mutation($p:ID!,$i:ID!){deleteProjectV2Item(input:{projectId:$p,itemId:$i}){deletedItemId}}`, map[string]any{"p": r.project, "i": item}, nil)
		_ = r.raw.GraphQL(ctx, `mutation($i:ID!){closeIssue(input:{issueId:$i,stateReason:NOT_PLANNED}){clientMutationId}}`, map[string]any{"i": node}, nil)
	})
	return number, node, item
}

// seen is what GitHub itself says about an issue: its project fields by name,
// its assignee and its latest comment.
type seen struct {
	Fields   map[string]string
	Assignee string
	Comment  string
}

func (r *rig) look(number int) seen {
	r.t.Helper()
	var out struct {
		Repository struct {
			Issue struct {
				Assignees    struct{ Nodes []struct{ Login string } }
				Comments     struct{ Nodes []struct{ Body string } }
				ProjectItems struct {
					Nodes []struct {
						FieldValues struct {
							Nodes []struct {
								Name, Text, Date string
								Number           *float64
								Field            struct{ Name string }
							}
						}
					}
				}
			}
		}
	}
	r.query(`query($o:String!,$r:String!,$n:Int!){repository(owner:$o,name:$r){issue(number:$n){
	  assignees(first:1){nodes{login}} comments(last:1){nodes{body}}
	  projectItems(first:5){nodes{fieldValues(first:40){nodes{
	    ... on ProjectV2ItemFieldSingleSelectValue{name field{... on ProjectV2FieldCommon{name}}}
	    ... on ProjectV2ItemFieldTextValue{text field{... on ProjectV2FieldCommon{name}}}
	    ... on ProjectV2ItemFieldNumberValue{number field{... on ProjectV2FieldCommon{name}}}
	    ... on ProjectV2ItemFieldDateValue{date field{... on ProjectV2FieldCommon{name}}}
	  }}}}}}}`, map[string]any{"o": r.ws.GitHub.Owner, "r": r.ws.GitHub.Repo, "n": number}, &out)
	s := seen{Fields: map[string]string{}}
	is := out.Repository.Issue
	if len(is.Assignees.Nodes) > 0 {
		s.Assignee = is.Assignees.Nodes[0].Login
	}
	if len(is.Comments.Nodes) > 0 {
		s.Comment = is.Comments.Nodes[0].Body
	}
	for _, p := range is.ProjectItems.Nodes {
		for _, v := range p.FieldValues.Nodes {
			val := v.Name + v.Text + v.Date
			if v.Number != nil {
				val = fmt.Sprint(*v.Number)
			}
			if v.Field.Name != "" {
				s.Fields[v.Field.Name] = val
			}
		}
	}
	return s
}

func (r *rig) call(method, path, body string, into any) int {
	r.t.Helper()
	req := httptest.NewRequest(method, "/api/w/"+r.ws.Slug+path, strings.NewReader(body))
	req.Header.Set("X-Gemba", "1")
	rec := httptest.NewRecorder()
	r.h.ServeHTTP(rec, req)
	if into != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			r.t.Fatalf("%s %s: %v: %s", method, path, err, rec.Body)
		}
	}
	if rec.Code != 200 {
		r.t.Logf("%s %s = %d: %s", method, path, rec.Code, rec.Body)
	}
	return rec.Code
}

func (r *rig) post(path, body string) {
	r.t.Helper()
	if code := r.call("POST", path, body, nil); code != 200 {
		r.t.Fatalf("POST %s = %d", path, code)
	}
}

// want checks fields as GitHub reports them, and says so in the test output.
func (r *rig) want(what string, number int, fields map[string]string) seen {
	r.t.Helper()
	s := r.look(number)
	for name, value := range fields {
		if s.Fields[name] != value {
			r.t.Errorf("%s: GitHub has %s = %q on #%d, want %q", what, name, s.Fields[name], number, value)
		}
	}
	if !r.t.Failed() {
		r.t.Logf("ok  %s (#%d: %v)", what, number, fields)
	}
	return s
}

// setStatus stands in for the engineer, who changes status in GitHub.
func (r *rig) setStatus(item, option string) {
	r.t.Helper()
	_, fields, err := r.gh.Project(ctx)
	if err != nil {
		r.t.Fatal(err)
	}
	f := fields[strings.ToLower(r.ws.GitHub.StatusField)]
	r.query(`mutation($p:ID!,$i:ID!,$f:ID!,$o:String!){updateProjectV2ItemFieldValue(input:{projectId:$p,itemId:$i,fieldId:$f,value:{singleSelectOptionId:$o}}){clientMutationId}}`,
		map[string]any{"p": r.project, "i": item, "f": f.ID, "o": f.Options[strings.ToLower(option)]}, nil)
}

type card struct {
	ID    int
	Title string
}

func has(cards []card, id int) bool {
	for _, c := range cards {
		if c.ID == id {
			return true
		}
	}
	return false
}

func TestEveryWriteReachesGitHub(t *testing.T) {
	r := setup(t)
	g := r.ws.GitHub
	cell := r.ws.Cells[0]
	if len(r.ws.People) == 0 {
		t.Fatal("the workspace needs a person")
	}
	person := r.ws.People[0]
	cell = *r.ws.Cell(person.Cell)
	kind := r.ws.Kinds[0]
	stamp := time.Now().UTC().Format("15:04:05")

	a, aNode, aItem := r.issue("E2E dated work " + stamp)
	b, _, _ := r.issue("E2E quick job " + stamp)
	c, _, _ := r.issue("E2E new priority " + stamp)

	// Reading: new issues on the project show up as planned work to triage.
	var inbox struct{ Rows []struct{ Card card } }
	// GitHub's project listing can trail a new item by a few seconds.
	for deadline := time.Now().Add(45 * time.Second); ; time.Sleep(3 * time.Second) {
		r.call("GET", "/inbox", "", &inbox)
		missing := 0
		for _, id := range []int{a, b, c} {
			found := false
			for _, row := range inbox.Rows {
				found = found || row.Card.ID == id
			}
			if !found {
				missing = id
			}
		}
		if missing == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("#%d should be waiting for triage", missing)
		}
	}
	t.Logf("ok  new issues appear in triage (#%d #%d #%d)", a, b, c)

	// Triage with a type, a fixed date and its date.
	r.post(fmt.Sprintf("/inbox/%d", a), fmt.Sprintf(`{"cell":%q,"cos":"fixed-date","kind":%q,"due":"2027-01-15"}`, cell.ID, kind.ID))
	s := r.want("triage writes cell, type, handling and date", a, map[string]string{
		g.CellField: cell.Name, g.COSField: "Fixed date", g.KindField: kind.Name, g.DueField: "2027-01-15"})
	if s.Fields[g.RankField] == "" {
		t.Errorf("triage should give #%d a rank", a)
	}

	// Quick work goes to the quick list, not the ranked queue.
	r.post(fmt.Sprintf("/inbox/%d", b), fmt.Sprintf(`{"cell":%q,"cos":"quick"}`, cell.ID))
	r.want("triage as quick", b, map[string]string{g.COSField: "Quick"})
	var floor struct {
		Lanes []struct {
			Cell         struct{ ID string }
			Queue, Quick []card
		}
	}
	r.call("GET", "/floor", "", &floor)
	var queue []card
	for _, l := range floor.Lanes {
		if l.Cell.ID == cell.ID {
			queue = l.Queue
			if !has(l.Quick, b) || has(l.Queue, b) || !has(l.Queue, a) {
				t.Errorf("#%d belongs on the quick list and #%d in the queue", b, a)
			}
		}
	}
	t.Logf("ok  the floor shows #%d queued and #%d on the quick list", a, b)

	// Reordering writes one rank.
	if len(queue) > 1 && queue[0].ID != a {
		first := queue[0].ID
		r.post("/queues/"+cell.ID+"/move", fmt.Sprintf(`{"id":%d,"before":%d}`, a, first))
		var ra, rf float64
		fmt.Sscan(r.look(a).Fields[g.RankField], &ra)
		fmt.Sscan(r.look(first).Fields[g.RankField], &rf)
		if ra >= rf {
			t.Errorf("after moving #%d before #%d its rank %v should be lower than %v", a, first, ra, rf)
		} else {
			t.Logf("ok  reorder moved #%d (rank %v) ahead of #%d (rank %v)", a, ra, first, rf)
		}
	}

	// The engineer starts it in GitHub; Gemba shows it in their hands.
	var user struct{ User struct{ ID string } }
	r.query(`query($l:String!){user(login:$l){id}}`, map[string]any{"l": person.Login}, &user)
	r.query(`mutation($a:ID!,$u:[ID!]!){addAssigneesToAssignable(input:{assignableId:$a,assigneeIds:$u}){clientMutationId}}`,
		map[string]any{"a": aNode, "u": []string{user.User.ID}}, nil)
	r.setStatus(aItem, g.InProgressStatus[0])
	var mine struct{ Doing, Waiting []card }
	r.call("GET", "/mine?as="+person.Login, "", &mine)
	if !has(mine.Doing, a) {
		t.Fatalf("#%d was started in GitHub and should be in %s's hands", a, person.Name)
	}
	t.Logf("ok  work started in GitHub appears in %s's hands", person.Name)

	// Waiting on someone, and taking it back.
	r.post(fmt.Sprintf("/items/%d/wait", a), `{"on":"the vendor"}`)
	r.want("waiting writes status and reason", a, map[string]string{g.StatusField: g.WaitingStatus[0], g.WaitingField: "the vendor"})
	r.call("GET", "/mine?as="+person.Login, "", &mine)
	if has(mine.Doing, a) || !has(mine.Waiting, a) {
		t.Errorf("#%d should be waiting and out of hand", a)
	}
	r.post(fmt.Sprintf("/items/%d/resume", a), `{}`)
	r.want("taking it back restores the status and clears the reason", a, map[string]string{g.StatusField: g.InProgressStatus[0], g.WaitingField: ""})

	// A trade-off: the new priority is expedited and the work in hand is paused.
	r.post("/tradeoff", fmt.Sprintf(`{"item":%d,"cell":%q,"pause":%d,"authorizedBy":"the e2e test"}`, c, cell.ID, a))
	r.want("trade-off expedites the new priority", c, map[string]string{g.CellField: cell.Name, g.COSField: "Expedite"})
	paused := r.want("trade-off sends the paused work back to the queue", a, map[string]string{g.StatusField: g.OpenStatus})
	if !strings.Contains(paused.Comment, "Authorized by the e2e test") {
		t.Errorf("the paused issue should carry a comment recording the decision, got %q", paused.Comment)
	} else {
		t.Logf("ok  the decision is recorded as a comment on #%d", a)
	}

	// The triage lead is the assignee of the triage issue.
	if g.TriageIssue > 0 {
		r.post("/triage-lead", fmt.Sprintf(`{"login":%q}`, person.Login))
		if got := r.look(g.TriageIssue).Assignee; !strings.EqualFold(got, person.Login) {
			t.Errorf("triage issue #%d is assigned to %q, want %q", g.TriageIssue, got, person.Login)
		} else {
			t.Logf("ok  the triage lead is the assignee of #%d", g.TriageIssue)
		}
	}

	// Every page still loads from the changed project.
	for _, page := range []string{"/floor", "/inbox", "/projects", "/tradeoff", "/review?days=30", "/mine?as=" + person.Login} {
		if code := r.call("GET", page, "", nil); code != 200 {
			t.Errorf("GET %s = %d", page, code)
		}
	}
}

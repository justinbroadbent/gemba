package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinbroadbent/gemba/internal/config"
)

// A stand-in for GitHub that answers the queries Gemba makes when it reads a
// project owned by an organisation with teams. It is the only proof of the
// team path short of a real organisation.
func fakeGitHub(t *testing.T, settings string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string
			Variables map[string]any
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var data string
		switch {
		case strings.Contains(req.Query, "projectV2"):
			data = `{"repositoryOwner":{"projectV2":{"id":"P1",
			  "fields":{"nodes":[
			    {"id":"F1","name":"Status","options":[{"id":"s1","name":"Todo"},{"id":"s2","name":"In Progress"},{"id":"s3","name":"Done"}]},
			    {"id":"F2","name":"Cell","options":[{"id":"c1","name":"Network"},{"id":"c2","name":"Data Services"}]},
			    {"id":"F3","name":"Work type","options":[{"id":"k1","name":"Enterprise project"},{"id":"k2","name":"Maintenance"}]},
			    {"id":"F4","name":"Rank"}]},
			  "items":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
			    {"id":"I1","content":{"id":"N1","number":7,"title":"Patch the core switches","url":"https://example.com/7","state":"OPEN",
			      "createdAt":"2026-11-01T00:00:00Z","assignees":{"nodes":[{"login":"rin"}]},"labels":{"nodes":[]},
			      "repository":{"nameWithOwner":"acme/work"}},
			     "fieldValues":{"nodes":[
			       {"name":"Data Services","field":{"name":"Cell"}},
			       {"name":"Maintenance","field":{"name":"Work type"}},
			       {"name":"In Progress","updatedAt":"2026-11-02T00:00:00Z","field":{"name":"Status"}}]}}]}}}}`
		case strings.Contains(req.Query, "object(expression"):
			if req.Variables["e"] != "HEAD:.gemba.yml" {
				t.Errorf("settings should be read from the default branch, got %v", req.Variables["e"])
			}
			if settings == "" {
				data = `{"repository":{"object":null}}`
			} else {
				b, _ := json.Marshal(settings)
				data = `{"repository":{"object":{"text":` + string(b) + `}}}`
			}
		case strings.Contains(req.Query, "organization("):
			switch req.Variables["s"] {
			case "infra-network":
				data = `{"organization":{"team":{"members":{"nodes":[{"login":"rin","name":"Rin Nakamura"},{"login":"kit","name":""}]}}}}`
			default:
				data = `{"organization":{"team":null}}`
			}
		default:
			t.Errorf("unexpected query: %.80s", req.Query)
			data = `{}`
		}
		_, _ = w.Write([]byte(`{"data":` + data + `}`))
	}))
}

func describe(t *testing.T, settings string) (*config.Workspace, []Item) {
	t.Helper()
	srv := fakeGitHub(t, settings)
	t.Cleanup(srv.Close)
	// Only where the work lives: no cells, kinds or people in the deployment config.
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(`workspaces:
  - slug: infra
    store: github
    github: { owner: acme, repo: work, project: 1 }
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	base := cfg.Workspaces[0]
	g := NewGitHub(base, "token")
	g.Endpoint = srv.URL
	ws, err := g.Workspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	items, err := g.Items(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ws, items
}

func TestTheTeamIsReadFromGitHub(t *testing.T) {
	ws, items := describe(t, "teams:\n  Network: infra-network\n  Data Services: no-such-team\npeople:\n  - { login: rin, wip_limit: 1 }\n")
	if len(ws.Cells) != 2 || ws.Cells[1].ID != "data-services" {
		t.Fatalf("cells should come from the Cell field: %+v", ws.Cells)
	}
	if len(ws.Kinds) != 2 || ws.Kinds[1].ID != "maintenance" {
		t.Fatalf("kinds should come from the Work type field: %+v", ws.Kinds)
	}
	if len(ws.People) != 2 || ws.People[0].Login != "rin" || ws.People[0].Name != "Rin Nakamura" || ws.People[0].Cell != "network" ||
		ws.People[0].WIPLimit != 1 || ws.People[1].Name != "kit" || ws.People[1].WIPLimit != ws.Policies.DefaultWIPLimit {
		t.Errorf("people should come from the team, adjusted by the settings file: %+v", ws.People)
	}
	if len(ws.Problems) != 1 || !strings.Contains(ws.Problems[0], "no-such-team") {
		t.Errorf("a team that cannot be found is reported and the rest still applies: %v", ws.Problems)
	}
	// Items are read against the cells and kinds found, not ones from the deployment config.
	if len(items) != 1 || items[0].Cell != "data-services" || items[0].Kind != "maintenance" || items[0].Status != StatusInProgress {
		t.Errorf("the item should map onto the cells and kinds found: %+v", items)
	}
}

func TestNoSettingsFileMeansDefaults(t *testing.T) {
	ws, _ := describe(t, "")
	if len(ws.Cells) != 2 || len(ws.People) != 0 || len(ws.Problems) != 0 {
		t.Errorf("with no settings file the project still gives cells, and nothing is wrong: %+v %v", ws.People, ws.Problems)
	}
}

func TestABrokenSettingsFileDoesNotTakeTheFloorDown(t *testing.T) {
	ws, items := describe(t, "poeple: [oops\n")
	if len(ws.Cells) != 2 || len(items) != 1 {
		t.Errorf("the floor should still load: %+v", ws.Cells)
	}
	if len(ws.Problems) != 1 || !strings.Contains(ws.Problems[0], "has a mistake") {
		t.Errorf("the mistake should be reported: %v", ws.Problems)
	}
}

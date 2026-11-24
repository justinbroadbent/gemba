// Command seedgithub fills a GitHub-backed workspace with fictional test issues,
// so the app can be tried against a real project.
//
//	GEMBA_CONFIG=gemba.local.yml GITHUB_TOKEN=... go run ./cmd/seedgithub <workspace-slug>
//
// It creates about forty issues in the workspace's repository and adds them to
// its project. Only run it against a repository and project made for testing.
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/store"
)

type seed struct {
	cell, cos, status, blocked, priority string
	rank                                 float64
	due                                  string
	mirrored, assign, closed             bool
}

func main() {
	if len(os.Args) != 2 {
		fail("usage: seedgithub <workspace-slug>")
	}
	cfg, err := config.Load(os.Getenv("GEMBA_CONFIG"))
	check(err)
	ws := cfg.Workspace(os.Args[1])
	if ws == nil || ws.Store != "github" {
		fail("workspace %q is not a github workspace in this config", os.Args[1])
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fail("GITHUB_TOKEN is not set")
	}
	ctx := context.Background()
	gh := store.NewGitHub(ws, token)
	if eff, err := gh.Workspace(context.Background()); err == nil {
		ws = eff // cells come from the project when the config does not list them
	}
	g := ws.GitHub

	existing, err := gh.Items(ctx)
	check(err)
	if len(existing) > 0 && os.Getenv("SEED_FORCE") != "1" {
		fail("the project already has %d issues from %s/%s; refusing to seed twice", len(existing), g.Owner, g.Repo)
	}
	projectID, fields, err := gh.Project(ctx)
	check(err)

	var meta struct {
		Viewer     struct{ ID string }
		Repository struct {
			ID    string
			Label *struct{ ID string }
		}
	}
	check(gh.GraphQL(ctx, `query($o:String!,$r:String!,$l:String!){viewer{id} repository(owner:$o,name:$r){id label(name:$l){id}}}`,
		map[string]any{"o": g.Owner, "r": g.Repo, "l": g.MirroredLabel}, &meta))
	if meta.Repository.Label == nil {
		fail("repository %s/%s has no %q label", g.Owner, g.Repo, g.MirroredLabel)
	}

	var plan []seed
	closed := []int{2, 6, 4}
	doing := []int{1, 2, 1}
	classes := []string{"standard", "fixed date", "standard", "background"}
	for n, c := range ws.Cells {
		k := n % 3
		for i := 0; i < closed[k]; i++ {
			plan = append(plan, seed{cell: c.Name, cos: "standard", status: "Done", closed: true, mirrored: i%2 == 0})
		}
		for i := 0; i < 2; i++ { // quick tickets: resolved without ever being triaged
			plan = append(plan, seed{cell: c.Name, priority: "low", closed: true, mirrored: true})
		}
		for i := 0; i < doing[k]; i++ {
			s := seed{cell: c.Name, cos: "standard", status: g.InProgressStatus[0], assign: ws.Person(g.Owner) != nil && ws.Person(g.Owner).Cell == c.ID}
			if i == 0 && k != 1 {
				s.blocked = "Waiting on vendor"
			}
			plan = append(plan, s)
		}
		for i, cos := range classes {
			s := seed{cell: c.Name, cos: cos, rank: float64(i + 1), mirrored: i == 0}
			if cos == "fixed date" {
				s.due = time.Now().AddDate(0, 0, 9+n).Format("2006-01-02")
			}
			plan = append(plan, s)
		}
	}
	first := ws.Cells[0].Name
	plan = append(plan,
		seed{cell: first, priority: "urgent", mirrored: true}, // surfaces for triage at once
		seed{cell: first, priority: "low", mirrored: true},    // fresh: in the fast lane for now
		seed{cell: ws.Cells[len(ws.Cells)-1].Name, priority: "medium", mirrored: true},
		seed{}, seed{}, // planned work nobody has queued yet
	)

	rnd := rand.New(rand.NewSource(11))
	set := func(itemID, field string, value map[string]any) {
		f, ok := fields[strings.ToLower(field)]
		if !ok {
			fail("the project has no field named %q", field)
		}
		if option, ok := value["option"].(string); ok {
			id, found := f.Options[strings.ToLower(option)]
			if !found {
				fail("field %q has no option %q", field, option)
			}
			value = map[string]any{"singleSelectOptionId": id}
		}
		check(gh.GraphQL(ctx, `mutation($p:ID!,$i:ID!,$f:ID!,$v:ProjectV2FieldValue!){updateProjectV2ItemFieldValue(input:{projectId:$p,itemId:$i,fieldId:$f,value:$v}){clientMutationId}}`,
			map[string]any{"p": projectID, "i": itemID, "f": f.ID, "v": value}, nil))
	}

	for n, s := range plan {
		flavour := "platform"
		for _, c := range ws.Cells {
			if c.Name == s.cell {
				flavour = c.ID
			}
		}
		input := map[string]any{"repositoryId": meta.Repository.ID, "title": store.DemoTitle(rnd, flavour),
			"body": "Fictional test item created by `seedgithub` for the Gemba prototype."}
		if s.mirrored {
			input["labelIds"] = []string{meta.Repository.Label.ID}
		}
		if s.assign {
			input["assigneeIds"] = []string{meta.Viewer.ID}
		}
		var created struct {
			CreateIssue struct {
				Issue struct {
					ID     string
					Number int
				}
			}
		}
		check(gh.GraphQL(ctx, `mutation($in:CreateIssueInput!){createIssue(input:$in){issue{id number}}}`, map[string]any{"in": input}, &created))
		issue := created.CreateIssue.Issue

		var added struct {
			AddProjectV2ItemById struct{ Item struct{ ID string } }
		}
		check(gh.GraphQL(ctx, `mutation($p:ID!,$c:ID!){addProjectV2ItemById(input:{projectId:$p,contentId:$c}){item{id}}}`,
			map[string]any{"p": projectID, "c": issue.ID}, &added))
		item := added.AddProjectV2ItemById.Item.ID

		if s.cell != "" {
			set(item, g.CellField, map[string]any{"option": s.cell})
		}
		if s.cos != "" {
			set(item, g.COSField, map[string]any{"option": s.cos})
		}
		if s.rank != 0 {
			set(item, g.RankField, map[string]any{"number": s.rank})
		}
		if s.status != "" {
			set(item, g.StatusField, map[string]any{"option": s.status})
		}
		if s.blocked != "" {
			set(item, g.BlockedField, map[string]any{"text": s.blocked})
		}
		if s.priority != "" {
			set(item, g.PriorityField, map[string]any{"option": s.priority})
		}
		if s.due != "" {
			set(item, g.DueField, map[string]any{"date": s.due})
		}
		if s.closed {
			// A project workflow may already have closed it when Status became Done; that is fine.
			_ = gh.GraphQL(ctx, `mutation($i:ID!){closeIssue(input:{issueId:$i}){clientMutationId}}`, map[string]any{"i": issue.ID}, nil)
		}
		fmt.Printf("%2d/%d  #%d\n", n+1, len(plan), issue.Number)
	}
}

func check(err error) {
	if err != nil {
		fail("%v", err)
	}
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/justinbroadbent/gemba/internal/config"
)

// GitHub is a Store backed by a GitHub Projects (v2) project. Issues in the
// configured repository that are on the project are the items; Gemba's fields
// are project fields, mapped by name in the workspace configuration.
type GitHub struct {
	cfg   *config.GitHub
	cells []config.Cell
	token string

	Endpoint string        // GraphQL endpoint; override for GitHub Enterprise Server or tests
	TTL      time.Duration // how long a read is reused before asking GitHub again
	Client   *http.Client

	mu        sync.Mutex
	projectID string
	fields    map[string]Field // by lower-case name
	refs      map[int]ref
	cache     []Item
	fetched   time.Time
}

// Field is a project field and, for single selects, its options by lower-case name.
type Field struct {
	ID      string
	Name    string
	Options map[string]string
}

type ref struct{ item, issue string }

func NewGitHub(ws *config.Workspace, token string) *GitHub {
	return &GitHub{
		cfg: ws.GitHub, cells: ws.Cells, token: token,
		Endpoint: "https://api.github.com/graphql",
		TTL:      20 * time.Second,
		Client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// GraphQL runs one query or mutation and decodes its data into out.
func (g *GitHub) GraphQL(ctx context.Context, query string, vars map[string]any, out any) error {
	body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	req, err := http.NewRequestWithContext(ctx, "POST", g.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := g.Client.Do(req)
	if err != nil {
		return fmt.Errorf("github: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode != 200 {
		return fmt.Errorf("github: HTTP %d: %.200s", res.StatusCode, raw)
	}
	var env struct {
		Data   json.RawMessage
		Errors []struct{ Message string }
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("github: %w", err)
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("github: %s", env.Errors[0].Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

const projectQuery = `query($owner:String!,$number:Int!,$after:String){
  repositoryOwner(login:$owner){ ... on ProjectV2Owner { projectV2(number:$number){
    id
    fields(first:50){nodes{
      ... on ProjectV2Field{id name}
      ... on ProjectV2SingleSelectField{id name options{id name}}
    }}
    items(first:100, after:$after){
      pageInfo{hasNextPage endCursor}
      nodes{
        id
        content{ ... on Issue{
          id number title url state createdAt closedAt
          assignees(first:1){nodes{login}}
          labels(first:20){nodes{name}}
          repository{nameWithOwner}
        }}
        fieldValues(first:30){nodes{
          ... on ProjectV2ItemFieldSingleSelectValue{name updatedAt field{... on ProjectV2FieldCommon{name}}}
          ... on ProjectV2ItemFieldNumberValue{number field{... on ProjectV2FieldCommon{name}}}
          ... on ProjectV2ItemFieldTextValue{text updatedAt field{... on ProjectV2FieldCommon{name}}}
          ... on ProjectV2ItemFieldDateValue{date field{... on ProjectV2FieldCommon{name}}}
        }}
      }
    }
  }}}
}`

type ghValue struct {
	Name      string
	Text      string
	Number    *float64
	Date      string
	UpdatedAt *time.Time
	Field     struct{ Name string }
}

type ghNode struct {
	ID      string
	Content struct {
		ID         string
		Number     int
		Title      string
		URL        string
		State      string
		CreatedAt  time.Time
		ClosedAt   *time.Time
		Assignees  struct{ Nodes []struct{ Login string } }
		Labels     struct{ Nodes []struct{ Name string } }
		Repository struct{ NameWithOwner string }
	}
	FieldValues struct{ Nodes []ghValue }
}

type ghProject struct {
	ID     string
	Fields struct {
		Nodes []struct {
			ID      string
			Name    string
			Options []struct{ ID, Name string }
		}
	}
	Items struct {
		PageInfo struct {
			HasNextPage bool
			EndCursor   string
		}
		Nodes []ghNode
	}
}

// load reads the whole project. The caller holds g.mu.
func (g *GitHub) load(ctx context.Context) error {
	var (
		items  []Item
		refs   = map[int]ref{}
		fields = map[string]Field{}
		id     string
		after  any
	)
	repo := strings.ToLower(g.cfg.Owner + "/" + g.cfg.Repo)
	for {
		var out struct {
			RepositoryOwner *struct {
				ProjectV2 *ghProject `json:"projectV2"`
			} `json:"repositoryOwner"`
		}
		vars := map[string]any{"owner": g.cfg.Owner, "number": g.cfg.Project, "after": after}
		if err := g.GraphQL(ctx, projectQuery, vars, &out); err != nil {
			return err
		}
		if out.RepositoryOwner == nil || out.RepositoryOwner.ProjectV2 == nil {
			return fmt.Errorf("github: project %d not found for %s", g.cfg.Project, g.cfg.Owner)
		}
		p := out.RepositoryOwner.ProjectV2
		id = p.ID
		for _, f := range p.Fields.Nodes {
			if f.ID == "" {
				continue
			}
			field := Field{ID: f.ID, Name: f.Name, Options: map[string]string{}}
			for _, o := range f.Options {
				field.Options[strings.ToLower(o.Name)] = o.ID
			}
			fields[strings.ToLower(f.Name)] = field
		}
		for _, n := range p.Items.Nodes {
			if n.Content.Number == 0 || strings.ToLower(n.Content.Repository.NameWithOwner) != repo {
				continue // drafts, pull requests, and issues from other repositories
			}
			items = append(items, g.item(n))
			refs[n.Content.Number] = ref{item: n.ID, issue: n.Content.ID}
		}
		if !p.Items.PageInfo.HasNextPage {
			break
		}
		after = p.Items.PageInfo.EndCursor
	}
	g.projectID, g.fields, g.refs, g.cache, g.fetched = id, fields, refs, items, time.Now()
	return nil
}

func (g *GitHub) item(n ghNode) Item {
	c := n.Content
	i := Item{ID: c.Number, Title: c.Title, URL: c.URL, Created: c.CreatedAt, Status: StatusOpen}
	if len(c.Assignees.Nodes) > 0 {
		i.Assignee = c.Assignees.Nodes[0].Login
	}
	for _, l := range c.Labels.Nodes {
		if strings.EqualFold(l.Name, g.cfg.MirroredLabel) {
			i.Mirrored = true
		}
	}
	is := func(v ghValue, field string) bool { return strings.EqualFold(v.Field.Name, field) }
	oneOf := func(name string, set []string) bool {
		return slices.ContainsFunc(set, func(s string) bool { return strings.EqualFold(s, name) })
	}
	for _, v := range n.FieldValues.Nodes {
		switch {
		case v.Field.Name == "":
		case is(v, g.cfg.StatusField):
			if oneOf(v.Name, g.cfg.InProgressStatus) {
				i.Status, i.Started = StatusInProgress, v.UpdatedAt
			} else if oneOf(v.Name, g.cfg.DoneStatus) {
				i.Status, i.Closed = StatusDone, v.UpdatedAt
			}
		case is(v, g.cfg.CellField):
			for _, cell := range g.cells {
				if strings.EqualFold(cell.Name, v.Name) || strings.EqualFold(cell.ID, v.Name) {
					i.Cell = cell.ID
				}
			}
		case is(v, g.cfg.COSField):
			cos := strings.ReplaceAll(strings.ToLower(v.Name), " ", "-")
			if slices.Contains(config.COS, cos) {
				i.COS = cos
			}
		case is(v, g.cfg.RankField):
			i.Rank = v.Number
		case is(v, g.cfg.BlockedField):
			if reason := strings.TrimSpace(v.Text + v.Name); reason != "" {
				i.BlockedReason, i.BlockedSince = reason, v.UpdatedAt
			}
		case is(v, g.cfg.PriorityField):
			i.ExtPriority = strings.ToLower(v.Text + v.Name)
		case is(v, g.cfg.SourceURLField):
			if u := strings.TrimSpace(v.Text); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
				i.SourceURL = u
			}
		case is(v, g.cfg.DueField):
			if t, err := time.Parse("2006-01-02", v.Date); err == nil {
				i.Due = &t
			}
		}
	}
	if c.State == "CLOSED" { // a closed issue is done whatever the board says
		i.Status = StatusDone
		i.Closed = c.ClosedAt
	}
	return i
}

func (g *GitHub) fresh(ctx context.Context) error {
	if g.cache != nil && time.Since(g.fetched) < g.TTL {
		return nil
	}
	return g.load(ctx)
}

func (g *GitHub) Items(ctx context.Context) ([]Item, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return nil, err
	}
	return slices.Clone(g.cache), nil
}

// Project returns the project's node ID and its fields, for tools that set
// things up (such as the test-floor seeder).
func (g *GitHub) Project(ctx context.Context) (string, map[string]Field, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return "", nil, err
	}
	return g.projectID, g.fields, nil
}

// Invalidate forces the next read to go to GitHub.
func (g *GitHub) Invalidate() {
	g.mu.Lock()
	g.cache = nil
	g.mu.Unlock()
}

const setField = `mutation($p:ID!,$i:ID!,$f:ID!,$v:ProjectV2FieldValue!){
  updateProjectV2ItemFieldValue(input:{projectId:$p,itemId:$i,fieldId:$f,value:$v}){clientMutationId}
}`

func (g *GitHub) set(ctx context.Context, r ref, field string, value map[string]any) error {
	f, ok := g.fields[strings.ToLower(field)]
	if !ok {
		return fmt.Errorf("github: the project has no field named %q", field)
	}
	if option, isSelect := value["option"]; isSelect {
		id, ok := f.Options[strings.ToLower(option.(string))]
		if !ok {
			return fmt.Errorf("github: field %q has no option %q", field, option)
		}
		value = map[string]any{"singleSelectOptionId": id}
	}
	return g.GraphQL(ctx, setField, map[string]any{"p": g.projectID, "i": r.item, "f": f.ID, "v": value}, nil)
}

func (g *GitHub) Update(ctx context.Context, id int, c Change) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return err
	}
	r, ok := g.refs[id]
	at := slices.IndexFunc(g.cache, func(i Item) bool { return i.ID == id })
	if !ok || at < 0 {
		return fmt.Errorf("github: issue #%d is not on the project", id)
	}
	cached := &g.cache[at]
	if c.Cell != nil {
		name := *c.Cell
		for _, cell := range g.cells {
			if cell.ID == *c.Cell {
				name = cell.Name
			}
		}
		if err := g.set(ctx, r, g.cfg.CellField, map[string]any{"option": name}); err != nil {
			return err
		}
		cached.Cell = *c.Cell
	}
	if c.COS != nil {
		if err := g.set(ctx, r, g.cfg.COSField, map[string]any{"option": strings.ReplaceAll(*c.COS, "-", " ")}); err != nil {
			return err
		}
		cached.COS = *c.COS
	}
	if c.Rank != nil {
		if err := g.set(ctx, r, g.cfg.RankField, map[string]any{"number": *c.Rank}); err != nil {
			return err
		}
		rank := *c.Rank
		cached.Rank = &rank
	}
	if c.Status != nil {
		if *c.Status != StatusOpen {
			return fmt.Errorf("github: Gemba only ever sets status back to open, not %q", *c.Status)
		}
		if err := g.set(ctx, r, g.cfg.StatusField, map[string]any{"option": g.cfg.OpenStatus}); err != nil {
			return err
		}
		cached.Status, cached.Started = StatusOpen, nil
	}
	return nil
}

func (g *GitHub) Comment(ctx context.Context, id int, text string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return err
	}
	r, ok := g.refs[id]
	if !ok {
		return fmt.Errorf("github: issue #%d is not on the project", id)
	}
	return g.GraphQL(ctx, `mutation($s:ID!,$b:String!){addComment(input:{subjectId:$s,body:$b}){clientMutationId}}`,
		map[string]any{"s": r.issue, "b": text}, nil)
}

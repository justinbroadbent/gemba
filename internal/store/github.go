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
	base  *config.Workspace // as the deployment config describes it
	token string

	Endpoint string        // GraphQL endpoint; override for GitHub Enterprise Server or tests
	TTL      time.Duration // how long a read is reused before asking GitHub again
	Client   *http.Client

	mu         sync.Mutex
	eff        *config.Workspace // base, plus what the project, its teams and its settings file say
	projectID  string
	fields     map[string]Field // by lower-case name
	refs       map[int]ref
	cache      []Item
	priorities string
	lead       *Lead
	leadIssue  string // node ID of the triage issue
	schedule   string // its body, which holds the triage schedule
	fetched    time.Time
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
		cfg: ws.GitHub, base: ws, eff: ws, token: token,
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
          id number title url state createdAt closedAt updatedAt
          assignees(first:1){nodes{login}}
          labels(first:20){nodes{name}}
          repository{nameWithOwner}
          parent{title url}
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
		UpdatedAt  *time.Time
		Assignees  struct{ Nodes []struct{ Login string } }
		Labels     struct{ Nodes []struct{ Name string } }
		Repository struct{ NameWithOwner string }
		Parent     *struct{ Title, URL string }
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
		nodes  []ghNode
		found  config.Found
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
			names := []string{}
			for _, o := range f.Options {
				field.Options[strings.ToLower(o.Name)] = o.ID
				names = append(names, o.Name)
			}
			fields[strings.ToLower(f.Name)] = field
			switch {
			case f.Options == nil:
			case strings.EqualFold(f.Name, g.cfg.CellField):
				found.Cells = names
			case strings.EqualFold(f.Name, g.cfg.KindField):
				found.Kinds = names
			}
		}
		for _, n := range p.Items.Nodes {
			if n.Content.Number == 0 || strings.ToLower(n.Content.Repository.NameWithOwner) != repo {
				continue // drafts, pull requests, and issues from other repositories
			}
			nodes = append(nodes, n)
			refs[n.Content.Number] = ref{item: n.ID, issue: n.Content.ID}
		}
		if !p.Items.PageInfo.HasNextPage {
			break
		}
		after = p.Items.PageInfo.EndCursor
	}
	// Who the team is and how it works, before the items are read against it.
	g.describe(ctx, &found)
	g.eff = g.base.Resolve(found)
	for _, n := range nodes {
		items = append(items, g.item(n))
	}
	priorities := ""
	if g.cfg.PrioritiesIssue > 0 {
		var out struct {
			Repository struct {
				Issue *struct{ Body string }
			}
		}
		vars := map[string]any{"o": g.cfg.Owner, "r": g.cfg.Repo, "n": g.cfg.PrioritiesIssue}
		if err := g.GraphQL(ctx, `query($o:String!,$r:String!,$n:Int!){repository(owner:$o,name:$r){issue(number:$n){body}}}`, vars, &out); err != nil {
			return err
		}
		if out.Repository.Issue != nil {
			priorities = out.Repository.Issue.Body
		}
	}
	var lead *Lead
	leadIssue, schedule := "", ""
	if g.cfg.TriageIssue > 0 {
		var out struct {
			Repository struct {
				Issue *struct {
					ID            string
					Body          string
					UpdatedAt     time.Time
					Assignees     struct{ Nodes []struct{ Login string } }
					TimelineItems struct {
						Nodes []struct{ CreatedAt *time.Time }
					}
				}
			}
		}
		vars := map[string]any{"o": g.cfg.Owner, "r": g.cfg.Repo, "n": g.cfg.TriageIssue}
		if err := g.GraphQL(ctx, `query($o:String!,$r:String!,$n:Int!){repository(owner:$o,name:$r){issue(number:$n){
		  id body updatedAt assignees(first:1){nodes{login}}
		  timelineItems(last:1,itemTypes:[ASSIGNED_EVENT]){nodes{... on AssignedEvent{createdAt}}}}}}`, vars, &out); err != nil {
			return err
		}
		if is := out.Repository.Issue; is != nil {
			leadIssue, schedule = is.ID, is.Body
			if len(is.Assignees.Nodes) > 0 {
				// When the role was handed over: the last time the issue was assigned.
				lead = &Lead{Login: is.Assignees.Nodes[0].Login, Since: is.UpdatedAt}
				if ev := is.TimelineItems.Nodes; len(ev) > 0 && ev[0].CreatedAt != nil {
					lead.Since = *ev[0].CreatedAt
				}
			}
		}
	}
	g.projectID, g.fields, g.refs, g.cache, g.fetched, g.priorities = id, fields, refs, items, time.Now(), priorities
	g.lead, g.leadIssue, g.schedule = lead, leadIssue, schedule
	return nil
}

// describe reads the team's settings file and the teams it names. Nothing here
// stops the floor loading: what cannot be read is reported as a problem.
func (g *GitHub) describe(ctx context.Context, found *config.Found) {
	problem := func(format string, a ...any) { found.Problems = append(found.Problems, fmt.Sprintf(format, a...)) }
	var file struct {
		Repository struct {
			Object *struct{ Text *string }
		}
	}
	vars := map[string]any{"o": g.cfg.Owner, "r": g.cfg.Repo, "e": "HEAD:" + g.cfg.SettingsPath}
	if err := g.GraphQL(ctx, `query($o:String!,$r:String!,$e:String!){repository(owner:$o,name:$r){object(expression:$e){... on Blob{text}}}}`, vars, &file); err != nil {
		problem("The settings file %s could not be read: %v", g.cfg.SettingsPath, err)
		return
	}
	if file.Repository.Object == nil || file.Repository.Object.Text == nil {
		return // no settings file: the defaults apply
	}
	settings, err := config.ParseSettings(*file.Repository.Object.Text)
	if err != nil {
		problem("The settings file %s has a mistake and is being ignored: %v", g.cfg.SettingsPath, err)
		return
	}
	found.Settings = settings
	found.Teams = map[string][]config.Member{}
	for _, slug := range settings.Teams {
		if _, done := found.Teams[slug]; done {
			continue
		}
		var out struct {
			Organization *struct {
				Team *struct {
					Members struct {
						Nodes []struct{ Login, Name string }
					}
				}
			}
		}
		err := g.GraphQL(ctx, `query($o:String!,$s:String!){organization(login:$o){team(slug:$s){members(first:100){nodes{login name}}}}}`,
			map[string]any{"o": g.cfg.Owner, "s": slug}, &out)
		switch {
		case err != nil:
			problem("The team %s could not be read: %v", slug, err)
		case out.Organization == nil || out.Organization.Team == nil:
			problem("There is no team called %s in %s, or Gemba may not see it.", slug, g.cfg.Owner)
		default:
			members := []config.Member{}
			for _, m := range out.Organization.Team.Members.Nodes {
				members = append(members, config.Member{Login: m.Login, Name: m.Name})
			}
			found.Teams[slug] = members
		}
	}
}

// Workspace is the workspace as it stands: the deployment config, plus what
// the project, its teams and its settings file say.
func (g *GitHub) Workspace(ctx context.Context) (*config.Workspace, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return nil, err
	}
	return g.eff, nil
}

func (g *GitHub) item(n ghNode) Item {
	c := n.Content
	i := Item{ID: c.Number, Title: c.Title, URL: c.URL, Created: c.CreatedAt, Updated: c.UpdatedAt, Status: StatusOpen}
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
			} else if oneOf(v.Name, g.cfg.WaitingStatus) {
				i.Status, i.WaitingSince = StatusWaiting, v.UpdatedAt
			} else if oneOf(v.Name, g.cfg.DoneStatus) {
				i.Status, i.Closed = StatusDone, v.UpdatedAt
			}
		case is(v, g.cfg.CellField):
			for _, cell := range g.eff.Cells {
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
		case is(v, g.cfg.WaitingField):
			i.WaitingOn = strings.TrimSpace(v.Text + v.Name)
		case is(v, g.cfg.PriorityField):
			i.ExtPriority = strings.ToLower(v.Text + v.Name)
		case is(v, g.cfg.SourceURLField):
			if u := strings.TrimSpace(v.Text); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
				i.SourceURL = u
			}
		case is(v, g.cfg.KindField):
			for _, k := range g.eff.Kinds {
				if strings.EqualFold(k.Name, v.Name) || strings.EqualFold(k.ID, v.Name) {
					i.Kind = k.ID
				}
			}
		case is(v, g.cfg.ProjectField):
			i.Project = strings.TrimSpace(v.Text + v.Name)
		case is(v, g.cfg.ProjectURLField):
			if u := strings.TrimSpace(v.Text); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
				i.ProjectURL = u
			}
		case is(v, g.cfg.DueField):
			if t, err := time.Parse("2006-01-02", v.Date); err == nil {
				i.Due = &t
			}
		}
	}
	if i.Kind == "" { // no field value: fall back to labels
		for _, l := range c.Labels.Nodes {
			for _, k := range g.eff.Kinds {
				if k.Label != "" && strings.EqualFold(k.Label, l.Name) {
					i.Kind = k.ID
				}
			}
		}
	}
	if i.Project == "" && c.Parent != nil { // a sub-issue belongs to its parent's project
		i.Project, i.ProjectURL = c.Parent.Title, c.Parent.URL
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
		for _, cell := range g.eff.Cells {
			if cell.ID == *c.Cell {
				name = cell.Name
			}
		}
		if err := g.set(ctx, r, g.cfg.CellField, map[string]any{"option": name}); err != nil {
			return err
		}
		cached.Cell = *c.Cell
	}
	if c.Kind != nil {
		name := *c.Kind
		for _, k := range g.eff.Kinds {
			if k.ID == *c.Kind {
				name = k.Name
			}
		}
		if err := g.set(ctx, r, g.cfg.KindField, map[string]any{"option": name}); err != nil {
			return err
		}
		cached.Kind = *c.Kind
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
		option := g.cfg.OpenStatus
		switch {
		case *c.Status == StatusWaiting && len(g.cfg.WaitingStatus) > 0:
			option = g.cfg.WaitingStatus[0]
		case *c.Status == StatusInProgress && len(g.cfg.InProgressStatus) > 0:
			option = g.cfg.InProgressStatus[0]
		case *c.Status != StatusOpen:
			return fmt.Errorf("github: Gemba does not set status to %q", *c.Status)
		}
		if err := g.set(ctx, r, g.cfg.StatusField, map[string]any{"option": option}); err != nil {
			return err
		}
		now := time.Now()
		cached.Status, cached.Started, cached.WaitingSince = *c.Status, nil, nil
		switch *c.Status {
		case StatusWaiting:
			cached.WaitingSince = &now
		case StatusInProgress:
			cached.Started = &now
		}
	}
	if c.Due != nil {
		if err := g.set(ctx, r, g.cfg.DueField, map[string]any{"date": c.Due.Format("2006-01-02")}); err != nil {
			return err
		}
		d := *c.Due
		cached.Due = &d
	}
	if c.WaitingOn != nil {
		// The reason is a convenience: a project without the field still gets the status.
		if _, ok := g.fields[strings.ToLower(g.cfg.WaitingField)]; ok {
			if err := g.set(ctx, r, g.cfg.WaitingField, map[string]any{"text": *c.WaitingOn}); err != nil {
				return err
			}
		}
		cached.WaitingOn = *c.WaitingOn
	}
	return nil
}

// Priorities returns the body of the configured priorities issue, if any.
func (g *GitHub) Priorities(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return "", err
	}
	return g.priorities, nil
}

// TriageLead is whoever the triage issue is assigned to.
func (g *GitHub) TriageLead(ctx context.Context) (*Lead, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return nil, err
	}
	return g.lead, nil
}

// TriageSchedule is the body of the triage issue, which holds the schedule.
func (g *GitHub) TriageSchedule(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return "", err
	}
	return g.schedule, nil
}

// SetTriageSchedule rewrites the body of the triage issue.
func (g *GitHub) SetTriageSchedule(ctx context.Context, text string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return err
	}
	if g.leadIssue == "" {
		return fmt.Errorf("github: no triage issue is configured")
	}
	err := g.GraphQL(ctx, `mutation($i:ID!,$b:String!){updateIssue(input:{id:$i,body:$b}){clientMutationId}}`,
		map[string]any{"i": g.leadIssue, "b": text}, nil)
	g.cache = nil
	return err
}

// SetTriageLead reassigns the triage issue, so the hand-over is visible in
// GitHub and the new lead is notified there.
func (g *GitHub) SetTriageLead(ctx context.Context, login string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.fresh(ctx); err != nil {
		return err
	}
	if g.leadIssue == "" {
		return fmt.Errorf("github: no triage issue is configured")
	}
	var out struct{ User *struct{ ID string } }
	if err := g.GraphQL(ctx, `query($l:String!){user(login:$l){id}}`, map[string]any{"l": login}, &out); err != nil {
		return err
	}
	if out.User == nil {
		return fmt.Errorf("github: no user %q", login)
	}
	err := g.GraphQL(ctx, `mutation($i:ID!,$u:[ID!]){updateIssue(input:{id:$i,assigneeIds:$u}){clientMutationId}}`,
		map[string]any{"i": g.leadIssue, "u": []string{out.User.ID}}, nil)
	g.cache = nil
	return err
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

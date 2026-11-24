// Package config loads the deployment configuration: brand, theme and workspaces.
package config

import (
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed default.yml
var defaultYAML []byte

// COS lists the classes of service, in the order they are offered.
// "quick" work skips the ranked queue: it sits on a short list for anyone with
// a spare half hour.
var COS = []string{"quick", "standard", "fixed-date", "background", "expedite"}

var themeKeys = []string{"paper", "surface", "ink", "muted", "rule", "wood", "wood-ink", "accent", "accent-ink", "ok", "warn", "stop"}

var (
	hexRe  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

type Config struct {
	Brand      Brand        `yaml:"brand"`
	Theme      Theme        `yaml:"theme"`
	Workspaces []*Workspace `yaml:"workspaces"`
}

type Brand struct {
	Name string `yaml:"name"`
	Logo string `yaml:"logo"` // optional path to an SVG or PNG that replaces the built-in mark
}

type Theme struct {
	Light map[string]string `yaml:"light" json:"light"`
	Dark  map[string]string `yaml:"dark" json:"dark"`
}

type Workspace struct {
	Slug   string  `yaml:"slug"`
	Name   string  `yaml:"name"`
	Store  string  `yaml:"store"`
	GitHub *GitHub `yaml:"github"`
	Cells  []Cell  `yaml:"cells"`
	Kinds  []Kind  `yaml:"kinds"`
	// Priorities is a short statement of what matters most right now, one line
	// each, shown to everyone. A GitHub workspace can read it from an issue instead.
	Priorities string   `yaml:"priorities"`
	People     []Person `yaml:"people"`
	Policies   Policies `yaml:"policies"`
	Notify     Notify   `yaml:"notify"`
	Managers   []string `yaml:"managers"`
	Members    []string `yaml:"members"`

	// Problems lists what could not be honoured when the workspace was put
	// together from its sources. Managers are shown it.
	Problems    []string `yaml:"-"`
	kindsPinned bool     // the deployment config lists the kinds, so the project does not decide them
}

// Notify says where decisions are announced. Optional.
type Notify struct {
	// TeamsWebhook is a Microsoft Teams incoming webhook address, or "env:NAME"
	// to read it from an environment variable.
	TeamsWebhook string `yaml:"teams_webhook"`
}

type Cell struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
}

// Kind is a kind of planned work. Unplanned work is not a kind: it is anything
// that arrived through the ticketing system or was expedited.
type Kind struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Label   string `yaml:"label" json:"-"`         // issue label that marks this kind, if labels are used
	Icon    string `yaml:"icon" json:"icon"`       // the small mark drawn beside items of this kind
	Project bool   `yaml:"project" json:"project"` // items of this kind belong to a project
}

type Person struct {
	SignIn   string `yaml:"signin"` // sign-in name, to recognise this person; optional
	Login    string `yaml:"login"`
	Name     string `yaml:"name"`
	Cell     string `yaml:"cell"`
	WIPLimit int    `yaml:"wip_limit"`
}

type Policies struct {
	DefaultWIPLimit  int      `yaml:"default_wip_limit"`
	TriageAfterHours float64  `yaml:"triage_after_hours"`
	UrgentPriorities []string `yaml:"urgent_priorities"`
	AgingDays        float64  `yaml:"aging_days"`
	QuietDays        float64  `yaml:"quiet_days"`
	QuickDays        float64  `yaml:"quick_days"`    // a quick item waiting longer than this needs attention
	WaitingDays      float64  `yaml:"waiting_days"`  // waiting longer than this: time to chase
	WaitingLimit     int      `yaml:"waiting_limit"` // how many items one person may have waiting
	ThroughputWeeks  int      `yaml:"throughput_weeks"`
	HorizonWeeks     int      `yaml:"horizon_weeks"`
}

// GitHub says where a workspace's items live and how Gemba's concepts map onto
// the project's fields. Field and option names are matched case-insensitively.
type GitHub struct {
	Owner   string `yaml:"owner"`   // user or organisation that owns the project
	Repo    string `yaml:"repo"`    // repository holding the issues
	Project int    `yaml:"project"` // project number

	StatusField      string   `yaml:"status_field"`
	OpenStatus       string   `yaml:"open_status"` // option set when Gemba pauses an item
	InProgressStatus []string `yaml:"in_progress_statuses"`
	// WaitingStatus are the statuses that mean started but parked on someone
	// else. WaitingField is a text field saying who or what.
	WaitingStatus []string `yaml:"waiting_statuses"`
	WaitingField  string   `yaml:"waiting_field"`
	DoneStatus    []string `yaml:"done_statuses"`

	CellField     string `yaml:"cell_field"` // single select; option names match cell names
	COSField      string `yaml:"cos_field"`  // single select; options match the classes of service
	RankField     string `yaml:"rank_field"` // number
	BlockedField  string `yaml:"blocked_field"`
	PriorityField string `yaml:"priority_field"`
	DueField      string `yaml:"due_field"`
	MirroredLabel string `yaml:"mirrored_label"` // issue label marking a mirrored ticket
	// SourceURLField is a text field holding the link to the ticket a mirrored
	// issue was copied from. Cards open that link, since that is where the work is done.
	SourceURLField string `yaml:"source_url_field"`

	KindField       string `yaml:"kind_field"`        // single select; options match the kind names. Labels are the fallback
	ProjectField    string `yaml:"project_field"`     // text or single select: the project's name
	ProjectURLField string `yaml:"project_url_field"` // text: a link to the project, such as its epic
	// PrioritiesIssue is the number of an issue whose body states the current priorities.
	// SettingsPath is the team's settings file in the repository. Default .gemba.yml.
	SettingsPath    string `yaml:"settings_path"`
	PrioritiesIssue int    `yaml:"priorities_issue"`
	// TriageIssue is the number of an issue whose assignee is this week's triage
	// lead. Gemba changes that assignee when a manager hands the role over.
	TriageIssue int `yaml:"triage_issue"`
}

func (w *Workspace) Person(login string) *Person {
	for i := range w.People {
		if w.People[i].Login == login {
			return &w.People[i]
		}
	}
	return nil
}

func (w *Workspace) PersonBySignIn(name string) *Person {
	for i := range w.People {
		if w.People[i].SignIn != "" && strings.EqualFold(w.People[i].SignIn, name) {
			return &w.People[i]
		}
	}
	return nil
}

func (w *Workspace) Kind(id string) *Kind {
	for i := range w.Kinds {
		if w.Kinds[i].ID == id {
			return &w.Kinds[i]
		}
	}
	return nil
}

func (w *Workspace) Cell(id string) *Cell {
	for i := range w.Cells {
		if w.Cells[i].ID == id {
			return &w.Cells[i]
		}
	}
	return nil
}

func (c *Config) Workspace(slug string) *Workspace {
	for _, w := range c.Workspaces {
		if w.Slug == slug {
			return w
		}
	}
	return nil
}

// Load reads the file at path, or the built-in demo configuration if path is empty.
func Load(path string) (*Config, error) {
	var defaults Config
	if err := yaml.Unmarshal(defaultYAML, &defaults); err != nil {
		return nil, fmt.Errorf("built-in config: %w", err)
	}
	raw := defaultYAML
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if c.Brand.Name == "" {
		c.Brand.Name = defaults.Brand.Name
	}
	var err error
	if c.Theme.Light, err = mergeTheme(defaults.Theme.Light, c.Theme.Light); err != nil {
		return nil, err
	}
	if c.Theme.Dark, err = mergeTheme(defaults.Theme.Dark, c.Theme.Dark); err != nil {
		return nil, err
	}
	if len(c.Workspaces) == 0 {
		return nil, fmt.Errorf("config: at least one workspace is required")
	}
	seen := map[string]bool{}
	for _, w := range c.Workspaces {
		if !slugRe.MatchString(w.Slug) {
			return nil, fmt.Errorf("config: workspace slug %q must be lowercase letters, digits and hyphens", w.Slug)
		}
		if seen[w.Slug] {
			return nil, fmt.Errorf("config: duplicate workspace %q", w.Slug)
		}
		seen[w.Slug] = true
		if err := w.normalise(defaults.Workspaces[0].Policies); err != nil {
			return nil, fmt.Errorf("config: workspace %s: %w", w.Slug, err)
		}
	}
	return &c, nil
}

func mergeTheme(base, over map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for _, k := range themeKeys {
		out[k] = base[k]
	}
	for k, v := range over {
		if _, ok := out[k]; !ok {
			return nil, fmt.Errorf("config: theme has unknown colour %q", k)
		}
		if !hexRe.MatchString(v) {
			return nil, fmt.Errorf("config: theme colour %q must be six-digit hex, got %q", k, v)
		}
		out[k] = v
	}
	return out, nil
}

func (w *Workspace) normalise(d Policies) error {
	if w.Name == "" {
		w.Name = w.Slug
	}
	if w.Store == "" {
		w.Store = "demo"
	}
	p := &w.Policies
	if p.DefaultWIPLimit <= 0 {
		p.DefaultWIPLimit = d.DefaultWIPLimit
	}
	if p.TriageAfterHours <= 0 {
		p.TriageAfterHours = d.TriageAfterHours
	}
	if p.UrgentPriorities == nil {
		p.UrgentPriorities = d.UrgentPriorities
	}
	if p.AgingDays <= 0 {
		p.AgingDays = d.AgingDays
	}
	if p.WaitingDays <= 0 {
		p.WaitingDays = 3
	}
	if p.WaitingLimit <= 0 {
		p.WaitingLimit = 2
	}
	if p.QuickDays <= 0 {
		p.QuickDays = 2
	}
	if p.QuietDays <= 0 {
		p.QuietDays = d.QuietDays
	}
	if p.ThroughputWeeks <= 0 {
		p.ThroughputWeeks = d.ThroughputWeeks
	}
	if p.HorizonWeeks <= 0 {
		p.HorizonWeeks = d.HorizonWeeks
	}
	// A demo floor is described here in full. A GitHub workspace may leave
	// cells, kinds and people out and let the project and its settings file say.
	if len(w.Cells) == 0 && w.Store != "github" {
		return fmt.Errorf("at least one cell is required")
	}
	w.kindsPinned = w.Kinds != nil
	for i := range w.People {
		if person := &w.People[i]; w.Cell(person.Cell) == nil && len(w.Cells) > 0 {
			return fmt.Errorf("%s is in unknown cell %q", person.Login, person.Cell)
		}
	}
	if w.Store != "github" {
		w.finish()
	}
	for i, m := range w.Managers {
		w.Managers[i] = strings.ToLower(m)
	}
	for i, m := range w.Members {
		w.Members[i] = strings.ToLower(m)
	}
	switch w.Store {
	case "demo":
	case "github":
		g := w.GitHub
		if g == nil || g.Owner == "" || g.Repo == "" || g.Project == 0 {
			return fmt.Errorf("store github needs github.owner, github.repo and github.project")
		}
		def := func(s *string, v string) {
			if *s == "" {
				*s = v
			}
		}
		def(&g.StatusField, "Status")
		def(&g.OpenStatus, "Todo")
		def(&g.CellField, "Cell")
		def(&g.COSField, "Class of service")
		def(&g.RankField, "Rank")
		def(&g.BlockedField, "Blocked")
		def(&g.WaitingField, "Waiting on")
		if g.WaitingStatus == nil {
			g.WaitingStatus = []string{"Waiting"}
		}
		def(&g.PriorityField, "Source priority")
		def(&g.DueField, "Due")
		def(&g.MirroredLabel, "mirrored")
		def(&g.SourceURLField, "Source URL")
		def(&g.KindField, "Work type")
		def(&g.ProjectField, "Project")
		def(&g.ProjectURLField, "Project URL")
		def(&g.SettingsPath, ".gemba.yml")
		if g.InProgressStatus == nil {
			g.InProgressStatus = []string{"In Progress"}
		}
		if g.DoneStatus == nil {
			g.DoneStatus = []string{"Done"}
		}
	default:
		return fmt.Errorf("unknown store %q", w.Store)
	}
	return nil
}

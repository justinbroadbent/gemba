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
var COS = []string{"expedite", "fixed-date", "standard", "background"}

var themeKeys = []string{"paper", "surface", "ink", "muted", "rule", "accent", "accent-ink", "ok", "warn", "stop"}

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
	Name string `yaml:"name" json:"name"`
	Mark string `yaml:"mark" json:"mark"`
}

type Theme struct {
	Light map[string]string `yaml:"light" json:"light"`
	Dark  map[string]string `yaml:"dark" json:"dark"`
}

type Workspace struct {
	Slug     string   `yaml:"slug"`
	Name     string   `yaml:"name"`
	Store    string   `yaml:"store"`
	GitHub   *GitHub  `yaml:"github"`
	Cells    []Cell   `yaml:"cells"`
	People   []Person `yaml:"people"`
	Policies Policies `yaml:"policies"`
	Managers []string `yaml:"managers"`
	Members  []string `yaml:"members"`
}

type Cell struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
}

type Person struct {
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
	DoneStatus       []string `yaml:"done_statuses"`

	CellField     string `yaml:"cell_field"` // single select; option names match cell names
	COSField      string `yaml:"cos_field"`  // single select; options match the classes of service
	RankField     string `yaml:"rank_field"` // number
	BlockedField  string `yaml:"blocked_field"`
	PriorityField string `yaml:"priority_field"`
	DueField      string `yaml:"due_field"`
	MirroredLabel string `yaml:"mirrored_label"` // issue label marking a mirrored ticket
}

func (w *Workspace) Person(login string) *Person {
	for i := range w.People {
		if w.People[i].Login == login {
			return &w.People[i]
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
	if c.Brand.Mark == "" {
		c.Brand.Mark = defaults.Brand.Mark
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
	if p.ThroughputWeeks <= 0 {
		p.ThroughputWeeks = d.ThroughputWeeks
	}
	if p.HorizonWeeks <= 0 {
		p.HorizonWeeks = d.HorizonWeeks
	}
	if len(w.Cells) == 0 {
		return fmt.Errorf("at least one cell is required")
	}
	for i := range w.Cells {
		if w.Cells[i].Name == "" {
			w.Cells[i].Name = w.Cells[i].ID
		}
	}
	for i := range w.People {
		person := &w.People[i]
		if w.Cell(person.Cell) == nil {
			return fmt.Errorf("%s is in unknown cell %q", person.Login, person.Cell)
		}
		if person.Name == "" {
			person.Name = person.Login
		}
		if person.WIPLimit <= 0 {
			person.WIPLimit = p.DefaultWIPLimit
		}
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
		def(&g.PriorityField, "Source priority")
		def(&g.DueField, "Due")
		def(&g.MirroredLabel, "mirrored")
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

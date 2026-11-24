package config

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// A workspace is described in three places, each owned by someone different:
//
//  1. The deployment config (this package's Config) says where the work lives,
//     who may get in, and how the app looks. Whoever deploys Gemba owns it.
//  2. The place the work lives says what the team is: its cells are the
//     options of the project's cell field, its kinds of work the options of
//     the work-type field, and its people the members of its teams.
//  3. A small settings file in the work-items repository says how the team
//     chooses to work: limits, thresholds, and anything the first two cannot
//     know. The team's manager owns it and edits it there.
//
// Resolve puts the three together. The settings file can never grant access:
// managers and members come only from the deployment config.

// Settings is the team's own file, by default .gemba.yml at the root of the
// work-items repository. Every part is optional.
type Settings struct {
	// Teams maps a cell (by name or id) to the team whose members work in it.
	Teams map[string]string `yaml:"teams"`
	// People adds people, or overrides what a team says about them, by login.
	People []Person `yaml:"people"`
	// Kinds adjusts kinds of work found on the project, by name.
	Kinds []KindSetting `yaml:"kinds"`
	// Policies overrides the deployment's thresholds and limits.
	Policies Policies `yaml:"policies"`
}

type KindSetting struct {
	Name    string `yaml:"name"`
	Icon    string `yaml:"icon"`
	Label   string `yaml:"label"`
	Project *bool  `yaml:"project"`
}

// ParseSettings reads a settings file. Unknown keys are an error, so a typo is
// reported instead of silently ignored.
func ParseSettings(text string) (*Settings, error) {
	var s Settings
	dec := yaml.NewDecoder(bytes.NewReader([]byte(text)))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil && err.Error() != "EOF" {
		return nil, err
	}
	return &s, nil
}

// Member is one person on a team, as the source reports them.
type Member struct{ Login, Name string }

// Found is what the place the work lives says about the team.
type Found struct {
	Cells    []string            // options of the cell field, in order; nil when there is no such field
	Kinds    []string            // options of the work-type field; nil when there is no such field
	Settings *Settings           // nil when there is no settings file, or it could not be read
	Teams    map[string][]Member // members by team slug, for the teams the settings name
	Problems []string            // anything that went wrong finding these, in plain words
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(name string) string {
	return strings.Trim(notSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// cellRef finds a cell by id or by name, ignoring case.
func (w *Workspace) cellRef(ref string) *Cell {
	for i := range w.Cells {
		if strings.EqualFold(w.Cells[i].ID, ref) || strings.EqualFold(w.Cells[i].Name, ref) {
			return &w.Cells[i]
		}
	}
	return nil
}

func guessIcon(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "enterprise"), strings.Contains(n, "company"):
		return "tower"
	case strings.Contains(n, "department"), strings.Contains(n, "team"):
		return "house"
	case strings.Contains(n, "maint"), strings.Contains(n, "ktlo"), strings.Contains(n, "run"):
		return "wrench"
	}
	return ""
}

// Resolve returns the workspace as it stands once what was found at the source
// is taken into account. The receiver is not changed. Anything that could not
// be honoured is listed in the result's Problems; the rest still applies, so a
// mistake in the settings file never takes the floor down.
func (w *Workspace) Resolve(f Found) *Workspace {
	out := *w
	out.Cells = append([]Cell(nil), w.Cells...)
	out.Kinds = append([]Kind(nil), w.Kinds...)
	out.People = append([]Person(nil), w.People...)
	out.Problems = append([]string(nil), f.Problems...)
	problem := func(format string, a ...any) { out.Problems = append(out.Problems, fmt.Sprintf(format, a...)) }

	// Cells and kinds listed in the deployment config are pinned; otherwise the project decides.
	if len(out.Cells) == 0 {
		for _, name := range f.Cells {
			out.Cells = append(out.Cells, Cell{ID: slug(name), Name: name})
		}
	}
	if len(out.Cells) == 0 {
		problem("No cells were found. Give the project's cell field at least one option.")
	}
	if !w.kindsPinned && f.Kinds != nil {
		out.Kinds = nil
		for _, name := range f.Kinds {
			out.Kinds = append(out.Kinds, Kind{ID: slug(name), Name: name, Label: slug(name),
				Icon: guessIcon(name), Project: strings.Contains(strings.ToLower(name), "project")})
		}
	}

	s := f.Settings
	if s == nil {
		s = &Settings{}
	}
	for _, ks := range s.Kinds {
		at := -1
		for i := range out.Kinds {
			if strings.EqualFold(out.Kinds[i].Name, ks.Name) || strings.EqualFold(out.Kinds[i].ID, ks.Name) {
				at = i
			}
		}
		if at < 0 {
			problem("The settings file adjusts a kind of work called %q, but the project has none by that name.", ks.Name)
			continue
		}
		k := &out.Kinds[at]
		if ks.Icon != "" {
			k.Icon = ks.Icon
		}
		if ks.Label != "" {
			k.Label = ks.Label
		}
		if ks.Project != nil {
			k.Project = *ks.Project
		}
	}
	out.Policies = w.Policies.over(s.Policies)

	// People: the deployment config first, then teams, then the settings file's own list.
	at := func(login string) int {
		for i := range out.People {
			if strings.EqualFold(out.People[i].Login, login) {
				return i
			}
		}
		return -1
	}
	teamCells := make([]string, 0, len(s.Teams))
	for ref := range s.Teams {
		teamCells = append(teamCells, ref)
	}
	sort.Strings(teamCells) // a steady order, whatever order the file lists them in
	for _, ref := range teamCells {
		cell := out.cellRef(ref)
		if cell == nil {
			problem("The settings file gives a team for %q, which is not a cell.", ref)
			continue
		}
		for _, m := range f.Teams[s.Teams[ref]] {
			p := Person{Login: m.Login, Name: m.Name, Cell: cell.ID}
			if i := at(m.Login); i >= 0 {
				p.SignIn, p.WIPLimit = out.People[i].SignIn, out.People[i].WIPLimit
				out.People[i] = p
			} else {
				out.People = append(out.People, p)
			}
		}
	}
	for _, o := range s.People {
		if o.Login == "" {
			problem("The settings file lists a person without a login.")
			continue
		}
		i := at(o.Login)
		if i < 0 {
			out.People = append(out.People, Person{Login: o.Login})
			i = len(out.People) - 1
		}
		p := &out.People[i]
		if o.Name != "" {
			p.Name = o.Name
		}
		if o.SignIn != "" {
			p.SignIn = o.SignIn
		}
		if o.WIPLimit > 0 {
			p.WIPLimit = o.WIPLimit
		}
		if o.Cell != "" {
			p.Cell = o.Cell
		}
	}
	kept := out.People[:0]
	for _, p := range out.People {
		cell := out.cellRef(p.Cell)
		switch {
		case p.Cell == "":
			problem("%s is not in a team and has no cell, so is left off the floor.", p.Login)
		case cell == nil:
			problem("%s is in %q, which is not a cell, so is left off the floor.", p.Login, p.Cell)
		default:
			p.Cell = cell.ID
			kept = append(kept, p)
		}
	}
	out.People = kept
	order := map[string]int{}
	for n, c := range out.Cells {
		order[c.ID] = n
	}
	sort.SliceStable(out.People, func(a, b int) bool { return order[out.People[a].Cell] < order[out.People[b].Cell] })

	out.finish()
	return &out
}

// over returns p with every value that o sets taking its place.
func (p Policies) over(o Policies) Policies {
	num := func(dst *float64, v float64) {
		if v > 0 {
			*dst = v
		}
	}
	whole := func(dst *int, v int) {
		if v > 0 {
			*dst = v
		}
	}
	whole(&p.DefaultWIPLimit, o.DefaultWIPLimit)
	num(&p.TriageAfterHours, o.TriageAfterHours)
	if o.UrgentPriorities != nil {
		p.UrgentPriorities = o.UrgentPriorities
	}
	num(&p.AgingDays, o.AgingDays)
	num(&p.QuietDays, o.QuietDays)
	num(&p.QuickDays, o.QuickDays)
	num(&p.WaitingDays, o.WaitingDays)
	whole(&p.WaitingLimit, o.WaitingLimit)
	whole(&p.ThroughputWeeks, o.ThroughputWeeks)
	whole(&p.HorizonWeeks, o.HorizonWeeks)
	return p
}

// finish fills in what can be defaulted once cells, kinds and people are known.
func (w *Workspace) finish() {
	if w.Kinds == nil {
		w.Kinds = []Kind{
			{ID: "enterprise", Name: "Enterprise project", Label: "enterprise-project", Project: true},
			{ID: "department", Name: "Department project", Label: "department-project", Project: true},
			{ID: "maintenance", Name: "Maintenance", Label: "maintenance"},
		}
	}
	for i := range w.Kinds {
		k := &w.Kinds[i]
		if k.Name == "" {
			k.Name = k.ID
		}
		if k.Icon == "" {
			k.Icon = guessIcon(k.Name) // unknown kinds get a plain mark
		}
	}
	for i := range w.Cells {
		if w.Cells[i].Name == "" {
			w.Cells[i].Name = w.Cells[i].ID
		}
	}
	for i := range w.People {
		p := &w.People[i]
		if p.Name == "" {
			p.Name = p.Login
		}
		if p.WIPLimit <= 0 {
			p.WIPLimit = w.Policies.DefaultWIPLimit
		}
	}
}

package config

import (
	"strings"
	"testing"
)

// base is a GitHub workspace whose deployment config says only where the work lives.
func base(t *testing.T) *Workspace {
	t.Helper()
	w := &Workspace{Slug: "infra", Store: "github", GitHub: &GitHub{Owner: "o", Repo: "r", Project: 1},
		Managers: []string{"Boss@Example.com"}}
	if err := w.normalise(Policies{DefaultWIPLimit: 2, TriageAfterHours: 8, AgingDays: 5, QuietDays: 3, ThroughputWeeks: 4, HorizonWeeks: 2}); err != nil {
		t.Fatal(err)
	}
	return w
}

func settings(t *testing.T, text string) *Settings {
	t.Helper()
	s, err := ParseSettings(text)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTheProjectDecidesCellsAndKinds(t *testing.T) {
	w := base(t).Resolve(Found{Cells: []string{"Network", "Data Services"}, Kinds: []string{"Enterprise project", "Maintenance", "Research"}})
	if len(w.Cells) != 2 || w.Cells[1].ID != "data-services" || w.Cells[1].Name != "Data Services" {
		t.Errorf("cells should be the cell field's options: %+v", w.Cells)
	}
	if len(w.Kinds) != 3 || w.Kinds[0].ID != "enterprise-project" || !w.Kinds[0].Project || w.Kinds[0].Icon != "tower" ||
		w.Kinds[1].Icon != "wrench" || w.Kinds[1].Project || w.Kinds[2].Icon != "" {
		t.Errorf("kinds should be the work-type options, with icons guessed from their names: %+v", w.Kinds)
	}
	if len(w.Problems) != 0 {
		t.Errorf("nothing is wrong: %v", w.Problems)
	}
	if w.Managers[0] != "boss@example.com" {
		t.Errorf("who gets in still comes from the deployment config: %v", w.Managers)
	}
}

func TestWithoutAWorkTypeFieldTheUsualKindsApply(t *testing.T) {
	w := base(t).Resolve(Found{Cells: []string{"Network"}})
	if len(w.Kinds) != 3 || w.Kinds[2].ID != "maintenance" {
		t.Errorf("the default kinds should apply: %+v", w.Kinds)
	}
}

func TestPeopleComeFromTeamsThenTheSettingsFile(t *testing.T) {
	found := Found{
		Cells: []string{"Network", "Platform"},
		Settings: settings(t, `
teams:
  platform: infra-platform
  Network: infra-network
people:
  - { login: ALEX, wip_limit: 1, signin: alex@example.com }
  - { login: jo, name: Jo Park, cell: Network }
  - { login: lost }
policies: { default_wip_limit: 3, waiting_days: 5 }
kinds:
  - { name: Nonesuch, icon: flag }
`),
		Teams: map[string][]Member{
			"infra-network":  {{Login: "rin", Name: "Rin N"}},
			"infra-platform": {{Login: "alex", Name: "Alex P"}, {Login: "sam"}},
		},
	}
	w := base(t).Resolve(found)
	got := map[string]Person{}
	var order []string
	for _, p := range w.People {
		got[p.Login] = p
		order = append(order, p.Login)
	}
	if strings.Join(order, ",") != "rin,jo,alex,sam" {
		t.Errorf("people should be listed cell by cell: %v", order)
	}
	if p := got["alex"]; p.Cell != "platform" || p.Name != "Alex P" || p.WIPLimit != 1 || p.SignIn != "alex@example.com" {
		t.Errorf("the settings file overrides one person, matching login without regard to case: %+v", p)
	}
	if p := got["sam"]; p.Name != "sam" || p.WIPLimit != 3 {
		t.Errorf("a team member with nothing else said gets the defaults: %+v", p)
	}
	if p := got["jo"]; p.Cell != "network" || p.Name != "Jo Park" {
		t.Errorf("the settings file can add a person outside any team: %+v", p)
	}
	if w.Policies.DefaultWIPLimit != 3 || w.Policies.WaitingDays != 5 || w.Policies.TriageAfterHours != 8 {
		t.Errorf("the settings file overrides only the policies it names: %+v", w.Policies)
	}
	all := strings.Join(w.Problems, "\n")
	for _, want := range []string{"lost is not in a team and has no cell", `"Nonesuch"`} {
		if !strings.Contains(all, want) {
			t.Errorf("missing problem %q in:\n%s", want, all)
		}
	}
	if len(base(t).People) != 0 {
		t.Error("resolving must not change the deployment config")
	}
}

func TestAMistakeInTheSettingsFileIsReportedNotIgnored(t *testing.T) {
	if _, err := ParseSettings("poeple: []\n"); err == nil {
		t.Error("an unknown key should be an error, so a typo is noticed")
	}
	if s, err := ParseSettings(""); err != nil || s == nil {
		t.Errorf("an empty file is fine: %v", err)
	}
	if _, err := ParseSettings("managers: [me@example.com]\n"); err == nil {
		t.Error("the settings file must not be able to name managers")
	}
}

func TestTheDeploymentConfigCanPinCellsAndKinds(t *testing.T) {
	w := base(t)
	w.Cells = []Cell{{ID: "net", Name: "Network"}}
	w.Kinds = []Kind{{ID: "only", Name: "Only"}}
	w.kindsPinned = true
	got := w.Resolve(Found{Cells: []string{"A", "B"}, Kinds: []string{"X"}})
	if len(got.Cells) != 1 || got.Cells[0].ID != "net" || len(got.Kinds) != 1 || got.Kinds[0].ID != "only" {
		t.Errorf("what the deployment config lists is kept: %+v %+v", got.Cells, got.Kinds)
	}
}

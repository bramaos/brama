package anonymize_test

import (
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
)

// resolved is Resolve's config, failing when the preset could not be read. Most of
// these tests are about what the merge produced, not about it having happened.
func resolved(t *testing.T, cfg *config.Config) *config.Anonymize {
	t.Helper()
	out, problems := anonymize.Resolve(cfg)
	if len(problems) > 0 {
		t.Fatalf("Resolve() = %v problems, want the preset read in", problems)
	}
	return out.Anonymize
}

// withPreset is a project naming a preset, plus whatever it says for itself.
func withPreset(name string, tables map[string]config.Table) *config.Config {
	cfg := project(tables, nil)
	cfg.Anonymize.Preset = name
	return cfg
}

// The whole of the feature: a name in the file stands for a classification brama ships,
// and the file is not where that classification lives.
func TestResolveReadsInTheClassificationThePresetShips(t *testing.T) {
	a := resolved(t, withPreset("wordpress", nil))

	users, known := a.Tables["wp_users"]
	if !known {
		t.Fatalf("tables = %v, want the preset's tables read in", a.Tables)
	}
	if got := users.Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("wp_users.user_email = %q, want the preset's classification", got)
	}
	if a.Preset != "wordpress" {
		t.Errorf("preset = %q, want the reference kept", a.Preset)
	}
}

// The file is a reference and not a copy. Resolution happens in memory, every run.
func TestResolveDoesNotWriteThePresetBackIntoTheFile(t *testing.T) {
	cfg := withPreset("wordpress", nil)

	if _, problems := anonymize.Resolve(cfg); len(problems) > 0 {
		t.Fatalf("Resolve() = %v", problems)
	}

	if len(cfg.Anonymize.Tables) != 0 {
		t.Errorf("tables = %v, want the file left holding only its own half", cfg.Anonymize.Tables)
	}
}

// The project's own half still stands after the merge. What the override does column by
// column is Apply's, and tested where it lives.
func TestResolveKeepsTheProjectsOwnTables(t *testing.T) {
	a := resolved(t, withPreset("wordpress", map[string]config.Table{
		"plugin_leads": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}))

	if got := a.Tables["plugin_leads"].Columns["email"].Action; got != "fake.email" {
		t.Errorf("plugin_leads.email = %q, want the project's own table kept", got)
	}
	if got := a.Tables["wp_users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("wp_users.user_email = %q, want the preset's tables beside them", got)
	}
}

// A typo in a preset name resolves to nothing, and nothing is every column the preset
// was carrying. It is refused by name, with the ones brama does ship listed.
func TestResolveRefusesAPresetBramaDoesNotShip(t *testing.T) {
	_, problems := anonymize.Resolve(withPreset("wordpres", nil))

	problem := only(t, problems)
	if problem.At != "anonymize.preset" {
		t.Errorf("At = %q, want the preset key", problem.At)
	}
	if !strings.Contains(problem.Detail, "wordpres") || !strings.Contains(problem.Detail, "wordpress") {
		t.Errorf("Detail = %q, want the unknown name and the ones brama ships", problem.Detail)
	}
}

// A file naming no preset is left exactly as it is.
func TestResolveLeavesAFileWithNoPresetAlone(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, nil)

	out, problems := anonymize.Resolve(cfg)

	if len(problems) != 0 || out != cfg {
		t.Errorf("Resolve() = %v, %v, want the config through untouched", out, problems)
	}
}

// ADR 0010, and the reason a preset is knowledge and not authorization: a preset's
// `keep` is not pre-approved by virtue of not being written into brama.yaml. Resolution
// moves classification and nothing else, and every environment still approves nothing.
func TestResolveGrantsNoApprovalForAPresetsKeep(t *testing.T) {
	cfg := withPreset("wordpress", nil)
	cfg.Environments = map[string]config.Environment{
		"staging": {},
		"local":   {Anonymize: &config.EnvironmentAnonymize{}},
	}

	out, problems := anonymize.Resolve(cfg)
	if len(problems) > 0 {
		t.Fatalf("Resolve() = %v", problems)
	}

	// A column the preset classifies `keep` — real production data at any destination
	// that is allowed to receive it.
	if got := out.Anonymize.Tables["wp_options"].Columns["option_value"].Action; got != config.Keep {
		t.Fatalf("wp_options.option_value = %q, want the preset to keep it", got)
	}
	for name, env := range out.Environments {
		if env.Approves("wp_options", "option_value") {
			t.Errorf("%s approves a column only the preset kept — a preset grants no approval", name)
		}
	}
}

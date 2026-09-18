package anonymize_test

import (
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
)

// project builds a config carrying one classified table, for a test to bend.
func project(tables map[string]config.Table, environments map[string]config.Environment) *config.Config {
	return &config.Config{
		Version:      config.SchemaVersion,
		App:          config.App{Adapter: "wordpress"},
		Environments: environments,
		Anonymize:    &config.Anonymize{Tables: tables},
	}
}

func columns(cols map[string]config.Column) config.Table {
	return config.Table{Columns: cols}
}

// names lists the environments of a config, the way the command does.
func names(cfg *config.Config) []string {
	var out []string
	for name := range cfg.Environments {
		out = append(out, name)
	}
	return out
}

// problemsOf is Check's second return, for the tests that only have something to say
// about what it refused.
func problemsOf(cfg *config.Config, environments []string) []anonymize.Problem {
	_, problems := anonymize.Check(cfg, environments, nil, nil)
	return problems
}

// only returns the single problem, failing when there is not exactly one. Most checks
// have one thing to say, and the message is the whole of what they are worth testing.
func only(t *testing.T, problems []anonymize.Problem) anonymize.Problem {
	t.Helper()
	if len(problems) != 1 {
		t.Fatalf("got %d problems, want exactly 1: %v", len(problems), problems)
	}
	return problems[0]
}

func TestCheckPassesAConsistentClassification(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"email":        {Action: "fake.email", Correlate: "customer"},
			"display_name": {Action: config.Keep},
			"activation":   {Action: config.Drop},
		}),
		"orders": columns(map[string]config.Column{
			"billing_email": {Action: "fake.email", Correlate: "customer"},
		}),
	}, map[string]config.Environment{
		"staging": {Anonymize: &config.EnvironmentAnonymize{
			Approved: []config.ColumnRef{{Table: "users", Column: "display_name"}},
		}},
	})

	summary, problems := anonymize.Check(cfg, names(cfg), nil, nil)

	if len(problems) != 0 {
		t.Fatalf("Check() = %v, want no problems", problems)
	}
	if summary.Tables != 2 || summary.Columns != 4 || summary.Groups != 1 {
		t.Errorf("Summary = %+v, want 2 tables, 4 columns, 1 group", summary)
	}
}

// brama owns the Generator vocabulary, so a name it does not know is caught in the
// editor rather than partway through a dump. See ADR 0013.
func TestCheckRefusesAGeneratorThatDoesNotExist(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.e_mail"}}),
	}, nil)

	problem := only(t, problemsOf(cfg, nil))

	if problem.At != "anonymize.tables.users.columns.email.action" {
		t.Errorf("At = %q, want the column's action", problem.At)
	}
	if !strings.Contains(problem.Detail, "e_mail") {
		t.Errorf("Detail = %q, want it to quote the name nobody knows", problem.Detail)
	}
	if !strings.Contains(problem.Detail, "email") {
		t.Errorf("Detail = %q, want it to list the names brama does know", problem.Detail)
	}
}

func TestCheckAcceptsEveryGeneratorBramaShips(t *testing.T) {
	// `password` included: its implementation is the Adapter's, but the name is
	// brama's and a file naming it is not wrong. Whether this project can fabricate
	// with it is a question for the Pull, which is where the Adapter is.
	for _, action := range []config.Classification{"fake.email", "fake.full_name", "fake.password"} {
		t.Run(string(action), func(t *testing.T) {
			cfg := project(map[string]config.Table{
				"users": columns(map[string]config.Column{"col": {Action: action}}),
			}, nil)

			if _, problems := anonymize.Check(cfg, nil, nil, nil); len(problems) != 0 {
				t.Errorf("Check() = %v, want %s accepted", problems, action)
			}
		})
	}
}

// correlate is meaningless on both of the Classifications that fabricate nothing, and
// it is refused on both rather than ignored: it reads like a decision in the diff.
func TestCheckRefusesCorrelateOnKeepAndDrop(t *testing.T) {
	for _, action := range []config.Classification{config.Keep, config.Drop} {
		t.Run(string(action), func(t *testing.T) {
			cfg := project(map[string]config.Table{
				"users": columns(map[string]config.Column{
					"email":       {Action: "fake.email", Correlate: "customer"},
					"customer_id": {Action: action, Correlate: "customer"},
				}),
			}, nil)

			problem := only(t, problemsOf(cfg, nil))

			if problem.At != "anonymize.tables.users.columns.customer_id.correlate" {
				t.Errorf("At = %q, want the offending correlate", problem.At)
			}
			if !strings.Contains(problem.Detail, string(action)) {
				t.Errorf("Detail = %q, want it to name the action it sits beside", problem.Detail)
			}
		})
	}
}

// One wrong line is one problem. A correlate beside a keep is usually also the only
// member of its group, and saying both reads as two edits to make.
func TestCheckReportsAMisplacedCorrelateOnce(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"display_name": {Action: config.Keep, Correlate: "customer"},
		}),
	}, nil)

	problem := only(t, problemsOf(cfg, nil))

	if !strings.Contains(problem.Detail, "keep") {
		t.Errorf("Detail = %q, want the classification it sits beside, not the group size", problem.Detail)
	}
}

// A group of one is the typo guard: `correlate: custmer` otherwise fabricates exactly
// what it would have without the line.
func TestCheckRefusesACorrelationGroupOfOne(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"email": {Action: "fake.email", Correlate: "customer"},
		}),
	}, nil)

	problem := only(t, problemsOf(cfg, nil))

	if problem.At != "anonymize.tables.users.columns.email.correlate" {
		t.Errorf("At = %q, want the lone member", problem.At)
	}
	if !strings.Contains(problem.Detail, "customer") {
		t.Errorf("Detail = %q, want it to name the group", problem.Detail)
	}
}

func TestCheckSuggestsTheNearMissGroupName(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"email":         {Action: "fake.email", Correlate: "customer"},
			"billing_email": {Action: "fake.email", Correlate: "customer"},
			"contact_email": {Action: "fake.email", Correlate: "custmer"},
		}),
	}, nil)

	problem := only(t, problemsOf(cfg, nil))

	if !strings.Contains(problem.Detail, `did you mean "customer"`) {
		t.Errorf("Detail = %q, want the near-miss spelling suggested", problem.Detail)
	}
}

// Two lone groups that are nothing like each other get no suggestion. A wrong
// suggestion is worse than none: it sends the reader to rename the wrong line.
func TestCheckSuggestsNothingWhenNoGroupIsClose(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"email": {Action: "fake.email", Correlate: "customer"},
			"phone": {Action: "fake.phone", Correlate: "warehouse"},
		}),
	}, nil)

	_, problems := anonymize.Check(cfg, nil, nil, nil)

	if len(problems) != 2 {
		t.Fatalf("got %d problems, want one per lone group: %v", len(problems), problems)
	}
	for _, p := range problems {
		if strings.Contains(p.Detail, "did you mean") {
			t.Errorf("Detail = %q, want no suggestion between unrelated names", p.Detail)
		}
	}
}

// A group spanning a discriminated key and an ordinary column is a group of two. Both
// halves of a key/value table classify values, so both can share an identity.
func TestCheckCountsDiscriminatedKeysAsGroupMembers(t *testing.T) {
	cfg := project(map[string]config.Table{
		"usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys: map[string]config.Column{
				"billing_email": {Action: "fake.email", Correlate: "customer"},
			},
			Columns: map[string]config.Column{
				"user_email": {Action: "fake.email", Correlate: "customer"},
			},
		},
	}, nil)

	if _, problems := anonymize.Check(cfg, nil, nil, nil); len(problems) != 0 {
		t.Errorf("Check() = %v, want a key and a column to correlate with each other", problems)
	}
}

func TestCheckRefusesAnApprovalOfAColumnThatIsNotKept(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, map[string]config.Environment{
		"staging": {Anonymize: &config.EnvironmentAnonymize{
			Approved: []config.ColumnRef{{Table: "users", Column: "email"}},
		}},
	})

	problem := only(t, problemsOf(cfg, names(cfg)))

	if problem.At != "environments.staging.anonymize.approved[0]" {
		t.Errorf("At = %q, want the approval", problem.At)
	}
	if !strings.Contains(problem.Detail, "users.email") {
		t.Errorf("Detail = %q, want it to name the column", problem.Detail)
	}
}

func TestCheckRefusesAnApprovalOfAnUnclassifiedColumn(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, map[string]config.Environment{
		"staging": {Anonymize: &config.EnvironmentAnonymize{
			Approved: []config.ColumnRef{{Table: "users", Column: "display_name"}},
		}},
	})

	problem := only(t, problemsOf(cfg, names(cfg)))

	if !strings.Contains(problem.Detail, "users.display_name") {
		t.Errorf("Detail = %q, want it to name the column nothing classifies", problem.Detail)
	}
}

// An Approval may name a column only the Preset classifies. The Preset is read in
// first, so `check` knows the column is kept and the approval means what it says.
func TestCheckAcceptsAnApprovalOfAColumnOnlyThePresetClassifies(t *testing.T) {
	cfg := project(map[string]config.Table{
		"plugin_leads": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, map[string]config.Environment{
		"staging": {Anonymize: &config.EnvironmentAnonymize{
			Approved: []config.ColumnRef{{Table: "wp_posts", Column: "post_content"}},
		}},
	})
	cfg.Anonymize.Preset = "wordpress"

	resolved, _, problems := anonymize.Resolve(cfg, "wp_")
	if len(problems) != 0 {
		t.Fatalf("Resolve() = %v, want the shipped preset read in", problems)
	}
	if _, problems := anonymize.Check(resolved, names(resolved), nil, nil); len(problems) != 0 {
		t.Errorf("Check() = %v, want an approval of a preset-kept column accepted", problems)
	}
}

// Naming a Preset is not a blanket excuse for a column nobody classifies. Once the
// Preset is read in, a column still missing from the answer is missing from it.
func TestCheckStillRefusesAnApprovalNoPresetClassifies(t *testing.T) {
	cfg := project(map[string]config.Table{
		"plugin_leads": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, map[string]config.Environment{
		"staging": {Anonymize: &config.EnvironmentAnonymize{
			Approved: []config.ColumnRef{{Table: "plugin_leads", Column: "internal_note"}},
		}},
	})
	cfg.Anonymize.Preset = "wordpress"

	resolved, _, _ := anonymize.Resolve(cfg, "wp_")
	problem := only(t, problemsOf(resolved, names(resolved)))

	if !strings.Contains(problem.Detail, "plugin_leads.internal_note") {
		t.Errorf("Detail = %q, want it to name the column nothing classifies", problem.Detail)
	}
}

func TestCheckSaysApprovingADiscriminatedValueColumnCoversNoKeys(t *testing.T) {
	cfg := project(map[string]config.Table{
		"usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys:          map[string]config.Column{"billing_phone": {Action: config.Keep}},
		},
	}, map[string]config.Environment{
		"staging": {Anonymize: &config.EnvironmentAnonymize{
			Approved: []config.ColumnRef{{Table: "usermeta", Column: "meta_value"}},
		}},
	})

	problem := only(t, problemsOf(cfg, names(cfg)))

	if !strings.Contains(problem.Detail, "per key") {
		t.Errorf("Detail = %q, want it to say the column is classified per key", problem.Detail)
	}
}

// Approval is the only part of the model that differs by destination, so it is the
// only part `--env` can narrow.
func TestCheckNarrowsToTheEnvironmentsItIsGiven(t *testing.T) {
	approves := func(table, column string) config.Environment {
		return config.Environment{Anonymize: &config.EnvironmentAnonymize{
			Approved: []config.ColumnRef{{Table: table, Column: column}},
		}}
	}
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"email":        {Action: "fake.email"},
			"display_name": {Action: config.Keep},
		}),
	}, map[string]config.Environment{
		"staging": approves("users", "display_name"),
		"local":   approves("users", "email"),
	})

	if _, problems := anonymize.Check(cfg, []string{"staging"}, nil, nil); len(problems) != 0 {
		t.Errorf("Check(staging) = %v, want only staging's approvals read", problems)
	}
	if _, problems := anonymize.Check(cfg, []string{"staging", "local"}, nil, nil); len(problems) != 1 {
		t.Errorf("Check(all) = %v, want local's approval reported", problems)
	}
}

package anonymize_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/schema"
)

// varchar is one string column of a declared width, the shape most of these tests bend.
func varchar(name string, length int64) schema.Column {
	return schema.Column{
		Name:     name,
		Type:     "varchar",
		Declared: "varchar(" + strconv.FormatInt(length, 10) + ")",
		Length:   length,
	}
}

func bigint(name string) schema.Column {
	return schema.Column{Name: name, Type: "bigint", Declared: "bigint(20) unsigned"}
}

// schemaOf builds a Schema out of tables named in declaration order.
func schemaOf(tables ...schema.Table) schema.Schema {
	return schema.Schema{Database: "acme", Tables: tables}
}

func table(name string, cols ...schema.Column) schema.Table {
	return schema.Table{Name: name, Columns: cols}
}

// uncovered names the unclassified columns, for a test that cares which they are
// rather than what they hold.
func uncovered(c anonymize.Coverage) []string {
	out := make([]string, 0, len(c.Unclassified))
	for _, u := range c.Unclassified {
		out = append(out, u.String())
	}
	return out
}

func TestCoverAcceptsAClassificationThatAnswersForEveryColumn(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"id":    {Action: config.Keep},
			"email": {Action: "fake.email"},
		}),
	}, nil)

	coverage, problems := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("users", bigint("id"), varchar("email", 100)),
	))

	if len(problems) != 0 {
		t.Fatalf("Cover() = %v, want no problems", problems)
	}
	if !coverage.Complete() {
		t.Errorf("Unclassified = %v, want none", uncovered(coverage))
	}
	if coverage.Columns != 2 {
		t.Errorf("Columns = %d, want 2", coverage.Columns)
	}
}

// The gap between a Schema and a file is the whole reason this pass needs a database:
// a column nobody classified is a column nobody decided about, and ADR 0003 refuses it.
func TestCoverReportsAColumnTheSchemaHasAndTheFileDoesNot(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, nil)

	coverage, _ := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("users", varchar("email", 100), varchar("internal_note", 255)),
	))

	if got := uncovered(coverage); len(got) != 1 || got[0] != "users.internal_note" {
		t.Errorf("Unclassified = %v, want [users.internal_note]", got)
	}
	if coverage.Complete() {
		t.Error("Complete() = true, want false with a column unanswered for")
	}
}

// A table the file never mentions is not a table that needs no decision.
func TestCoverReportsEveryColumnOfATableTheFileOmits(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, nil)

	coverage, _ := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("users", varchar("email", 100)),
		table("orders", bigint("id"), varchar("billing_email", 100)),
	))

	want := []string{"orders.id", "orders.billing_email"}
	if got := uncovered(coverage); !equal(got, want) {
		t.Errorf("Unclassified = %v, want %v", got, want)
	}
}

// The column carries its Schema entry, not just its name. `anonymize init` asks a
// Generator to claim a column, and a claim is the name and the type together.
func TestCoverCarriesTheSchemaColumnForInitToClassify(t *testing.T) {
	cfg := project(nil, nil)

	coverage, _ := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("users", varchar("email", 100)),
	))

	if len(coverage.Unclassified) != 1 {
		t.Fatalf("Unclassified = %v, want one column", uncovered(coverage))
	}
	if got := coverage.Unclassified[0].Column; got.Type != "varchar" || got.Length != 100 {
		t.Errorf("Column = %+v, want the schema's own type and length", got)
	}
}

// The refusal ADR 0013 promises: in the editor, not mid-dump on a production Server.
func TestCoverRefusesAGeneratorThatCannotFitTheColumnsLength(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, nil)

	_, problems := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("users", varchar("email", 20)),
	))

	p := only(t, problems)
	if p.At != "anonymize.tables.users.columns.email.action" {
		t.Errorf("At = %q, want the action that cannot be carried out", p.At)
	}
	if !strings.Contains(p.Detail, "varchar(20)") {
		t.Errorf("Detail = %q, want the declared type a reader has to change", p.Detail)
	}
}

func TestCoverRefusesAGeneratorThatCannotFitTheColumnsType(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, nil)

	_, problems := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("users", bigint("email")),
	))

	if !strings.Contains(only(t, problems).Detail, "bigint") {
		t.Errorf("problems = %v, want the type refused", problems)
	}
}

// keep and drop write no fabricated value, so no Generator has to fit them.
func TestCoverLeavesKeepAndDropAloneWhateverTheColumnHolds(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"flags": {Action: config.Keep},
			"blob":  {Action: config.Drop},
		}),
	}, nil)

	_, problems := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("users", bigint("flags"), varchar("blob", 4)),
	))

	if len(problems) != 0 {
		t.Errorf("Cover() = %v, want no problems", problems)
	}
}

// An unknown Generator is Check's to report, and reporting it twice reads as two
// edits to make.
func TestCoverLeavesAnUnknownGeneratorToCheck(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.e_mail"}}),
	}, nil)

	_, problems := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("users", bigint("email")),
	))

	if len(problems) != 0 {
		t.Errorf("Cover() = %v, want the unknown name reported once, by Check", problems)
	}
}

// A generated column cannot be written to at all, so its absence from the file is not
// a decision anybody failed to make.
func TestCoverDoesNotAskForAClassificationOfAGeneratedColumn(t *testing.T) {
	generated := varchar("search_text", 255)
	generated.Generated = true
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, nil)

	coverage, _ := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("users", varchar("email", 100), generated),
	))

	if !coverage.Complete() {
		t.Errorf("Unclassified = %v, want a generated column left out", uncovered(coverage))
	}
	if coverage.Columns != 1 {
		t.Errorf("Columns = %d, want only the column a classification could act on", coverage.Columns)
	}
}

// A key/value table classifies its value column per key, so the discriminator and the
// column it selects for are answered for by the keys and not by a column entry.
func TestCoverTreatsADiscriminatorAndItsValueColumnAsAnsweredFor(t *testing.T) {
	cfg := project(map[string]config.Table{
		"usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys:          map[string]config.Column{"billing_phone": {Action: "fake.phone"}},
			Columns:       map[string]config.Column{"umeta_id": {Action: config.Keep}},
		},
	}, nil)

	coverage, problems := anonymize.Cover(cfg.Anonymize, schemaOf(
		table("usermeta", bigint("umeta_id"), varchar("meta_key", 255), varchar("meta_value", 20)),
	))

	if len(problems) != 0 {
		t.Fatalf("Cover() = %v, want no problems", problems)
	}
	if !coverage.Complete() {
		t.Errorf("Unclassified = %v, want the discriminator and its value column left out", uncovered(coverage))
	}
}

// A Preset is read in before coverage is compared, so a column it classifies is a
// classified column here — answered for without being written into the file. What the
// preset does not know is still reported, which is the only part left to decide.
func TestCoverCountsAPresetColumnAsClassified(t *testing.T) {
	cfg := project(map[string]config.Table{
		"plugin_leads": columns(map[string]config.Column{"email": {Action: "fake.email"}}),
	}, nil)
	cfg.Anonymize.Preset = "wordpress"

	resolved, _, problems := anonymize.Resolve(cfg)
	if len(problems) != 0 {
		t.Fatalf("Resolve() = %v, want the shipped preset read in", problems)
	}

	coverage, _ := anonymize.Cover(resolved.Anonymize, schemaOf(
		table("wp_users", varchar("user_email", 100), varchar("display_name", 250)),
		table("plugin_leads", varchar("email", 100), varchar("internal_note", 255)),
	))

	if !equal(uncovered(coverage), []string{"plugin_leads.internal_note"}) {
		t.Errorf("Unclassified = %v, want only the column neither the preset nor the file answers for",
			uncovered(coverage))
	}
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

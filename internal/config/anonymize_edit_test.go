package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/bramaos/brama/internal/config"
)

// AddAnonymize writes the block `brama anonymize init` decided on. Like AddServer it
// edits lines rather than re-marshalling, because brama.yaml is a review surface a
// human also writes in — so these tests are about what survives the edit as much as
// what the edit adds.

// classification is what init hands the writer: two tables, three columns, every one
// of them claimed by a generator.
func classification() []config.TableClassification {
	return []config.TableClassification{
		{Name: "users", Columns: []config.ColumnClassification{
			{Name: "user_login", Action: "fake.username", Declared: "varchar(60)"},
			{Name: "user_email", Action: "fake.email", Declared: "varchar(100)"},
		}},
		{Name: "orders", Columns: []config.ColumnClassification{
			{Name: "billing_email", Action: "fake.email", Declared: "varchar(100)"},
		}},
	}
}

// parseAnonymize reads back the block that was written, proving the bytes are YAML
// brama itself accepts and not just text that looks right.
func parseAnonymize(t *testing.T, doc []byte) config.Anonymize {
	t.Helper()
	var probe struct {
		Anonymize config.Anonymize `yaml:"anonymize"`
	}
	if err := yaml.Unmarshal(doc, &probe); err != nil {
		t.Fatalf("the written block does not parse: %v\n%s", err, doc)
	}
	return probe.Anonymize
}

const skeleton = `version: 1

app:
  adapter: wordpress
  paths:
    config: wp-config.php            # holds the database credentials

environments:
  local:
    url: https://acme.local.test

servers:
  hetzner:
    host: staging.acme.test

# No anonymize block yet, which means every column is unclassified and the first
# pull will refuse. Classify them with: brama anonymize init
`

// The note `brama init` leaves says there is no classification. The moment there is
// one, it is a sentence that contradicts the file it sits in.
func TestAddAnonymizeReplacesTheNoteInitLeft(t *testing.T) {
	out, err := config.AddAnonymize([]byte(skeleton), "", classification())
	if err != nil {
		t.Fatalf("AddAnonymize: %v", err)
	}

	if strings.Contains(string(out), "No anonymize block yet") {
		t.Errorf("the note survived, and it is no longer true:\n%s", out)
	}
	block := parseAnonymize(t, out)
	if got := block.Tables["users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("users.user_email.action = %q, want fake.email", got)
	}
	if got := block.Tables["orders"].Columns["billing_email"].Action; got != "fake.email" {
		t.Errorf("orders.billing_email.action = %q, want fake.email", got)
	}
}

// Everything outside the inserted lines comes through byte for byte, including the
// column-aligned comments `brama init` wrote.
func TestAddAnonymizePreservesCommentsAndKeyOrder(t *testing.T) {
	out, err := config.AddAnonymize([]byte(skeleton), "", classification())
	if err != nil {
		t.Fatalf("AddAnonymize: %v", err)
	}

	kept := "    config: wp-config.php            # holds the database credentials"
	if !strings.Contains(string(out), kept) {
		t.Errorf("the aligned comment was reflowed:\n%s", out)
	}
	before, _, found := strings.Cut(string(out), "anonymize:")
	if !found {
		t.Fatalf("no anonymize block was written:\n%s", out)
	}
	if !strings.Contains(before, "version: 1") || strings.Index(before, "app:") > strings.Index(before, "environments:") {
		t.Errorf("the keys were reordered:\n%s", out)
	}
}

// The order is init's, which is the schema's. A file whose tables shuffled between
// runs is churn no reviewer can read past.
func TestAddAnonymizeWritesTablesInTheOrderItWasGiven(t *testing.T) {
	out, err := config.AddAnonymize([]byte(skeleton), "", classification())
	if err != nil {
		t.Fatalf("AddAnonymize: %v", err)
	}

	users, orders := strings.Index(string(out), "    users:"), strings.Index(string(out), "    orders:")
	if users < 0 || orders < 0 || users > orders {
		t.Errorf("tables are not in the order they were given:\n%s", out)
	}
	login, email := strings.Index(string(out), "user_login:"), strings.Index(string(out), "user_email:")
	if login > email {
		t.Errorf("columns are not in the order they were given:\n%s", out)
	}
}

// The declared type is written beside the action, so "why fake.email here?" is
// answered in the diff rather than by reopening the database.
func TestAddAnonymizeNamesTheTypeItDecidedAgainst(t *testing.T) {
	out, err := config.AddAnonymize([]byte(skeleton), "", classification())
	if err != nil {
		t.Fatalf("AddAnonymize: %v", err)
	}

	if !strings.Contains(string(out), "# varchar(100)") {
		t.Errorf("the declared type is missing:\n%s", out)
	}
}

// A file with a block has decisions in it that were reviewed and committed. Replacing
// them wholesale is not what `init` is for, and merging into them silently reopens
// them.
func TestAddAnonymizeRefusesAFileThatAlreadyClassifies(t *testing.T) {
	doc := skeleton + `
anonymize:
  tables:
    users:
      columns:
        user_email:
          action: keep
`

	_, err := config.AddAnonymize([]byte(doc), "", classification())

	if !errors.Is(err, config.ErrAnonymizeExists) {
		t.Errorf("AddAnonymize() = %v, want ErrAnonymizeExists", err)
	}
}

// An `anonymize:` with nothing under it decodes to no block at all, and appending a
// second one produces a document neither brama nor YAML can read.
func TestAddAnonymizeRefusesAnEmptyBlockAlreadyInTheFile(t *testing.T) {
	_, err := config.AddAnonymize([]byte(skeleton+"\nanonymize:\n"), "", classification())

	if !errors.Is(err, config.ErrAnonymizeExists) {
		t.Errorf("AddAnonymize() = %v, want ErrAnonymizeExists", err)
	}
}

// Writing `anonymize:` with nothing under it would turn "no generator recognised your
// columns" into a file that looks decided and is not — and `anonymize check` refuses
// a block that classifies nothing by name.
func TestAddAnonymizeRefusesToWriteAnEmptyBlock(t *testing.T) {
	_, err := config.AddAnonymize([]byte(skeleton), "", nil)

	if err == nil {
		t.Fatal("AddAnonymize() = nil, want a refusal to write a block that decides nothing")
	}
	if errors.Is(err, config.ErrAnonymizeExists) {
		t.Errorf("AddAnonymize() = %v, want it reported as nothing to write", err)
	}
}

// A Preset is written as a name and never as the classification behind it. That is what
// keeps the file short enough to review, and what lets a preset brama tightens reach
// this project without the file being edited again.
func TestAddAnonymizeReferencesThePresetByName(t *testing.T) {
	out, err := config.AddAnonymize([]byte(skeleton), "wordpress", classification())
	if err != nil {
		t.Fatalf("AddAnonymize: %v", err)
	}

	got := parseAnonymize(t, out)
	if got.Preset != "wordpress" {
		t.Errorf("preset = %q, want the preset referenced by name", got.Preset)
	}
	if _, expanded := got.Tables["wp_users"]; expanded {
		t.Error("the preset was expanded into the file — it is referenced, not copied")
	}
}

// A preset covering everything the schema has leaves no tables to write, and that is a
// complete block rather than an empty one. `tables:` with nothing under it would read,
// in the diff, as a list somebody forgot to fill.
func TestAddAnonymizeWritesAPresetWithNoTablesOfItsOwn(t *testing.T) {
	out, err := config.AddAnonymize([]byte(skeleton), "wordpress", nil)
	if err != nil {
		t.Fatalf("AddAnonymize: %v", err)
	}

	if strings.Contains(string(out), "tables:") {
		t.Errorf("an empty tables key was written:\n%s", out)
	}
	if got := parseAnonymize(t, out); got.Preset != "wordpress" {
		t.Errorf("preset = %q, want the preset alone to be a whole block", got.Preset)
	}
}

// With no note to replace — a file a human wrote themselves — the block goes at the
// end, one blank line down.
func TestAddAnonymizeAppendsWhenThereIsNoNote(t *testing.T) {
	doc := `version: 1

app:
  adapter: wordpress
`

	out, err := config.AddAnonymize([]byte(doc), "", classification())
	if err != nil {
		t.Fatalf("AddAnonymize: %v", err)
	}

	if !strings.HasPrefix(string(out), doc) {
		t.Errorf("the original file did not survive unchanged:\n%s", out)
	}
	if got := parseAnonymize(t, out); len(got.Tables) != 2 {
		t.Errorf("tables = %v, want both written", got.Tables)
	}
}

// A key/value table is written with its Discriminator and value beside the keys a
// Generator claimed, because `keys` without them classifies nothing.
func TestAddAnonymizeWritesAKeyedTable(t *testing.T) {
	out, err := config.AddAnonymize([]byte(skeleton), "wordpress", []config.TableClassification{
		{Name: "wp_usermeta", Discriminator: "meta_key", Value: "meta_value", Keys: []config.ColumnClassification{
			{Name: "billing_email", Action: "fake.email"},
		}},
	})
	if err != nil {
		t.Fatalf("AddAnonymize() = %v", err)
	}

	usermeta := parseAnonymize(t, out).Tables["wp_usermeta"]
	if usermeta.Discriminator != "meta_key" || usermeta.Value != "meta_value" {
		t.Errorf("wp_usermeta discriminator, value = %q, %q, want meta_key, meta_value", usermeta.Discriminator, usermeta.Value)
	}
	if got := usermeta.Keys["billing_email"].Action; got != "fake.email" {
		t.Errorf("wp_usermeta keys billing_email = %q, want fake.email\n%s", got, out)
	}
	if strings.Contains(string(out), "columns:") {
		t.Errorf("a table with no column claimed has a columns mapping:\n%s", out)
	}
}

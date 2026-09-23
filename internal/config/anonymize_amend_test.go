package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/config"
)

// classified is a file `brama anonymize review` amends: a preset, a table with a
// comment somebody wrote, a column carrying a second axis, and a discriminated table.
const classified = `version: 1
app:
  adapter: wordpress
  paths:
    config: wp-config.php
    uploads: wp-content/uploads
environments:
  local:
    url: https://acme.local.test

# The classification, reviewed and committed like any other decision.
anonymize:
  preset: wordpress
  tables:
    users:
      columns:
        # Real names are fine on staging; the display name is public anyway.
        display_name:
          action: keep
        email:
          action: fake.email            # varchar(100)
          correlate: customer
    usermeta:
      discriminator: meta_key
      value: meta_value
      keys:
        billing_phone:
          action: keep
`

func amend(t *testing.T, doc string, amendments ...config.Amendment) string {
	t.Helper()
	out, err := config.AmendAnonymize([]byte(doc), amendments)
	if err != nil {
		t.Fatalf("AmendAnonymize() = %v, want the amendment written", err)
	}
	return string(out)
}

// reread decodes the amended file, which is the only proof that what was written is
// what brama reads back.
func reread(t *testing.T, doc string) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("the amended file does not parse: %v\n%s", err, doc)
	}
	return cfg
}

// The ordinary amendment: a column the file already answers for gets a new answer.
func TestAmendReplacesTheActionOfAnExistingColumn(t *testing.T) {
	out := amend(t, classified, config.Amendment{Table: "users", Column: "display_name", Action: "fake.full_name"})

	if got := reread(t, out).Anonymize.Tables["users"].Columns["display_name"].Action; got != "fake.full_name" {
		t.Errorf("users.display_name.action = %q, want fake.full_name", got)
	}
}

// Everything the amendment is not about survives byte for byte: the comments a person
// wrote, the key order, the second axis beside the action. brama.yaml is a review
// surface, and a write that reflows it is a diff nobody can read.
func TestAmendPreservesCommentsAndKeyOrder(t *testing.T) {
	out := amend(t, classified, config.Amendment{Table: "users", Column: "display_name", Action: config.Drop})

	for _, kept := range []string{
		"# The classification, reviewed and committed like any other decision.",
		"# Real names are fine on staging; the display name is public anyway.",
		"          correlate: customer",
		"  preset: wordpress",
	} {
		if !strings.Contains(out, kept) {
			t.Errorf("the amended file lost %q:\n%s", kept, out)
		}
	}
	if before, after := index(classified, "display_name"), index(out, "display_name"); before != after {
		t.Errorf("display_name moved from line %d to %d, want the key order left alone:\n%s", before, after, out)
	}
}

// The type comment beside an action says why that generator was chosen. A new action is
// a new answer to the same question, so the column it is about still has to be named.
func TestAmendKeepsTheTypeComment(t *testing.T) {
	out := amend(t, classified, config.Amendment{Table: "users", Column: "email", Action: config.Drop})

	if !strings.Contains(out, "# varchar(100)") {
		t.Errorf("the type comment was dropped:\n%s", out)
	}
	if !strings.Contains(out, "action: drop") {
		t.Errorf("the action was not replaced:\n%s", out)
	}
}

// A correlation group is part of an answer, not a decoration on it. A column written
// out of its group still fabricates a value, and the joins the group exists to keep stop
// surviving the pull.
func TestAmendWritesTheCorrelationGroupBesideTheAction(t *testing.T) {
	out := amend(t, classified, config.Amendment{
		Table: "users", Column: "display_name", Action: "fake.full_name", Correlate: "customer",
	})

	if got := reread(t, out).Anonymize.Tables["users"].Columns["display_name"].Correlate; got != "customer" {
		t.Errorf("users.display_name.correlate = %q, want customer", got)
	}
}

// An amendment about the action alone leaves the group the file already records. The
// only answers that carry no group are `keep` and `drop`, and a `correlate` beside
// either is a file `anonymize check` refuses — so there is no group this could be
// clearing.
func TestAmendLeavesAGroupItSaysNothingAbout(t *testing.T) {
	out := amend(t, classified, config.Amendment{Table: "users", Column: "email", Action: "fake.username"})

	if got := reread(t, out).Anonymize.Tables["users"].Columns["email"].Correlate; got != "customer" {
		t.Errorf("users.email.correlate = %q, want the group left as it was", got)
	}
}

// A migration added a column to a table the file already classifies.
func TestAmendAddsAColumnToAnExistingTable(t *testing.T) {
	out := amend(t, classified, config.Amendment{
		Table: "users", Column: "user_url", Action: "fake.url", Declared: "varchar(100)",
	})

	if got := reread(t, out).Anonymize.Tables["users"].Columns["user_url"].Action; got != "fake.url" {
		t.Errorf("users.user_url.action = %q, want fake.url", got)
	}
	if !strings.Contains(out, "# varchar(100)") {
		t.Errorf("a new entry does not say what the column is:\n%s", out)
	}
}

// A migration added a table nobody has classified.
func TestAmendAddsATableTheFileDoesNotHave(t *testing.T) {
	out := amend(t, classified, config.Amendment{
		Table: "orders", Column: "billing_email", Action: "fake.email", Declared: "varchar(200)",
	})

	if got := reread(t, out).Anonymize.Tables["orders"].Columns["billing_email"].Action; got != "fake.email" {
		t.Errorf("orders.billing_email.action = %q, want the table written", got)
	}
}

// A Discriminator value is classified under `keys`, not under `columns`. Writing it in
// the wrong half would classify a column called `billing_phone` that does not exist.
func TestAmendWritesADiscriminatorKeyUnderKeys(t *testing.T) {
	out := amend(t, classified, config.Amendment{Table: "usermeta", Key: "billing_phone", Action: "fake.phone"})

	usermeta := reread(t, out).Anonymize.Tables["usermeta"]
	if got := usermeta.Keys["billing_phone"].Action; got != "fake.phone" {
		t.Errorf("usermeta keys billing_phone = %q, want fake.phone", got)
	}
	if _, wrong := usermeta.Columns["billing_phone"]; wrong {
		t.Errorf("the key was written as a column:\n%s", out)
	}
}

// The `columns` mapping is created where a table has only keys, because a discriminated
// table has ordinary columns too and the file may not mention them yet.
func TestAmendCreatesTheColumnsMapping(t *testing.T) {
	out := amend(t, classified, config.Amendment{Table: "usermeta", Column: "umeta_id", Action: config.Keep})

	usermeta := reread(t, out).Anonymize.Tables["usermeta"]
	if got := usermeta.Columns["umeta_id"].Action; got != config.Keep {
		t.Errorf("usermeta.umeta_id.action = %q, want keep", got)
	}
	if usermeta.Discriminator != "meta_key" || len(usermeta.Keys) != 1 {
		t.Errorf("the discriminated half of the table changed: %+v", usermeta)
	}
}

// A preset that covered everything leaves a file with no `tables` at all, until a
// migration adds a column the preset does not know.
func TestAmendCreatesTheTablesBlock(t *testing.T) {
	const presetOnly = `version: 1
app:
  adapter: wordpress
  paths:
    config: wp-config.php
    uploads: wp-content/uploads
environments:
  local:
    url: https://acme.local.test
anonymize:
  preset: wordpress
`
	out := amend(t, presetOnly, config.Amendment{Table: "orders", Column: "billing_email", Action: "fake.email"})

	cfg := reread(t, out)
	if cfg.Anonymize.Preset != "wordpress" {
		t.Errorf("preset = %q, want it untouched", cfg.Anonymize.Preset)
	}
	if got := cfg.Anonymize.Tables["orders"].Columns["billing_email"].Action; got != "fake.email" {
		t.Errorf("orders.billing_email.action = %q, want fake.email", got)
	}
}

// Several amendments in one write, which is what a review of a real migration is.
func TestAmendWritesEveryAmendmentInOnePass(t *testing.T) {
	out := amend(t, classified,
		config.Amendment{Table: "users", Column: "display_name", Action: "fake.full_name"},
		config.Amendment{Table: "users", Column: "user_url", Action: "fake.url"},
		config.Amendment{Table: "orders", Column: "billing_email", Action: "fake.email"},
	)

	tables := reread(t, out).Anonymize.Tables
	if got := tables["users"].Columns["display_name"].Action; got != "fake.full_name" {
		t.Errorf("users.display_name.action = %q", got)
	}
	if got := tables["users"].Columns["user_url"].Action; got != "fake.url" {
		t.Errorf("users.user_url.action = %q", got)
	}
	if got := tables["orders"].Columns["billing_email"].Action; got != "fake.email" {
		t.Errorf("orders.billing_email.action = %q", got)
	}
}

// Nothing to amend is the ordinary case on a project nobody has changed, and it must
// not rewrite a single byte.
func TestAmendWithNothingToWriteLeavesTheFileAlone(t *testing.T) {
	out, err := config.AmendAnonymize([]byte(classified), nil)
	if err != nil {
		t.Fatalf("AmendAnonymize() = %v, want the file returned as it was", err)
	}
	if string(out) != classified {
		t.Errorf("the file changed:\n%s", out)
	}
}

// Amending is editing decisions that exist. A file with no block has none, and that is
// `brama anonymize init`'s job rather than a block this should invent.
func TestAmendRefusesAFileWithNoBlock(t *testing.T) {
	const unclassified = `version: 1
app:
  adapter: wordpress
  paths:
    config: wp-config.php
    uploads: wp-content/uploads
environments:
  local:
    url: https://acme.local.test
`
	_, err := config.AmendAnonymize([]byte(unclassified), []config.Amendment{
		{Table: "users", Column: "email", Action: "fake.email"},
	})
	if !errors.Is(err, config.ErrNoAnonymizeBlock) {
		t.Errorf("AmendAnonymize() = %v, want ErrNoAnonymizeBlock", err)
	}
}

// An amendment that names neither a column nor a key, or names both, is a caller bug
// and not something to write a guess for.
func TestAmendRefusesAnAmendmentThatNamesNoColumn(t *testing.T) {
	for _, a := range []config.Amendment{
		{Table: "users", Action: config.Keep},
		{Table: "users", Column: "email", Key: "email", Action: config.Keep},
		{Column: "email", Action: config.Keep},
		{Table: "users", Column: "email", Action: "mask"},
	} {
		if _, err := config.AmendAnonymize([]byte(classified), []config.Amendment{a}); err == nil {
			t.Errorf("AmendAnonymize(%+v) = nil, want an error", a)
		}
	}
}

// A block written inline is one brama cannot edit a line at a time. It says so and
// leaves the file alone, rather than guessing at where the lines would have been.
func TestAmendRefusesAnInlineBlock(t *testing.T) {
	const inline = `version: 1
app:
  adapter: wordpress
  paths:
    config: wp-config.php
    uploads: wp-content/uploads
environments:
  local:
    url: https://acme.local.test
anonymize:
  preset: wordpress
  tables: {users: {columns: {email: {action: keep}}}}
`
	_, err := config.AmendAnonymize([]byte(inline), []config.Amendment{
		{Table: "users", Column: "email", Action: "fake.email"},
	})
	if err == nil {
		t.Fatal("AmendAnonymize() = nil, want a refusal to guess at an inline block")
	}
	if !strings.Contains(err.Error(), "inline") {
		t.Errorf("error = %v, want it to say why the file cannot be edited", err)
	}
}

// index is the 1-based line a string first appears on, for asserting that key order
// did not move.
func index(doc, want string) int {
	for i, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, want) {
			return i + 1
		}
	}
	return -1
}

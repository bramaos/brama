package config_test

import (
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/config"
)

// The classification model has three axes and three homes: `action` says what a
// column means, `correlate` says which identity mapping it shares, and `approved`
// — on the Environment, not the column — says which destination may receive real
// values. These tests are as much about what the file cannot say as what it can.
// See docs/adr/0010-classification-and-approval-are-separate-axes.md.

// full exercises every part of the model at once, because the shapes have to
// coexist: a discriminated table describing its ordinary columns is the case the
// old Keys-XOR-Columns type could not express at all.
const full = `
anonymize:
  preset: wordpress
  tables:
    users:
      columns:
        email:
          action: fake.email
          correlate: customer
        display_name:
          action: keep
        internal_note:
          action: drop
    usermeta:
      discriminator: meta_key
      value: meta_value
      keys:
        billing_phone:
          action: fake.phone
        billing_email:
          action: fake.email
          correlate: customer
      columns:
        umeta_id:
          action: keep
        user_id:
          action: keep
`

func TestAnonymizeColumnCarriesActionAndCorrelate(t *testing.T) {
	cfg := mustParse(t, valid+full)

	users := cfg.Anonymize.Tables["users"]
	email := users.Columns["email"]
	if email.Action != "fake.email" {
		t.Errorf("users.email.action = %q, want fake.email", email.Action)
	}
	if email.Correlate != "customer" {
		t.Errorf("users.email.correlate = %q, want customer", email.Correlate)
	}
	if got := users.Columns["display_name"].Action; got != config.Keep {
		t.Errorf("users.display_name.action = %q, want keep", got)
	}
	if got := users.Columns["internal_note"].Action; got != config.Drop {
		t.Errorf("users.internal_note.action = %q, want drop", got)
	}
	// Correlation is a property of the column, not of every column beside it.
	if got := users.Columns["display_name"].Correlate; got != "" {
		t.Errorf("users.display_name.correlate = %q, want empty", got)
	}
	if cfg.Anonymize.Preset != "wordpress" {
		t.Errorf("preset = %q, want wordpress", cfg.Anonymize.Preset)
	}
}

// The case the old type could not express: a discriminated table has ordinary
// columns too, and saying nothing about `umeta_id` leaves it Unclassified.
func TestAnonymizeDiscriminatedTableCarriesKeysAndColumnsTogether(t *testing.T) {
	cfg := mustParse(t, valid+full)

	meta := cfg.Anonymize.Tables["usermeta"]
	if meta.Discriminator != "meta_key" {
		t.Errorf("usermeta.discriminator = %q, want meta_key", meta.Discriminator)
	}
	if meta.Value != "meta_value" {
		t.Errorf("usermeta.value = %q, want meta_value", meta.Value)
	}
	if got := meta.Keys["billing_phone"].Action; got != "fake.phone" {
		t.Errorf("usermeta.keys.billing_phone.action = %q, want fake.phone", got)
	}
	if got := meta.Keys["billing_email"].Correlate; got != "customer" {
		t.Errorf("usermeta.keys.billing_email.correlate = %q, want customer", got)
	}
	if got := meta.Columns["umeta_id"].Action; got != config.Keep {
		t.Errorf("usermeta.columns.umeta_id.action = %q, want keep", got)
	}
	if len(meta.Columns) != 2 {
		t.Errorf("usermeta.columns = %v, want exactly umeta_id and user_id", meta.Columns)
	}
}

// Shorthand is what turns into the next migration problem. There is no scalar form
// and no conversion from one — the error teaches the object form instead.
func TestAnonymizeRejectsScalarShorthand(t *testing.T) {
	for _, body := range []string{
		"anonymize:\n  tables:\n    users:\n      columns:\n        email: fake.email\n",
		"anonymize:\n  tables:\n    usermeta:\n      discriminator: meta_key\n      value: meta_value\n      keys:\n        billing_phone: fake.phone\n",
	} {
		err := parseErr(t, valid+body)
		if !strings.Contains(err.Error(), "action:") {
			t.Errorf("error = %q, want it to show the object form", err)
		}
	}
}

// There is no fourth action. `mask` in particular is refused by name: CONTEXT.md
// bans the word, and partial preservation derives output from the real value, which
// is the pseudonymization ADR 0002 exists to prevent.
func TestAnonymizeRejectsAnythingThatIsNotAnAction(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		wantErr string
	}{
		{"invented", "maybe", "fake.<generator>, keep, or drop"},
		{"masking", "mask", "fake.<generator>, keep, or drop"},
		{"fake without a generator", "fake", "name the generator"},
		{"fake with an empty generator", "fake.", "name the generator"},
		{"wrong case", "Keep", "fake.<generator>, keep, or drop"},
		{"generator in the wrong case", "fake.Email", "not a generator name"},
		{"generator with punctuation", "fake.e-mail", "not a generator name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "anonymize:\n  tables:\n    users:\n      columns:\n        email:\n          action: " + tt.action + "\n"

			err := parseErr(t, valid+body)
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), "users.columns.email") {
				t.Errorf("error = %q, want it to name the offending column", err)
			}
		})
	}
}

// A column with no action is Unclassified, and Unclassified is not something the
// file gets to say quietly.
func TestAnonymizeRequiresAnAction(t *testing.T) {
	body := "anonymize:\n  tables:\n    users:\n      columns:\n        email:\n          correlate: customer\n"

	err := parseErr(t, valid+body)
	if !strings.Contains(err.Error(), "anonymize.tables.users.columns.email.action is required") {
		t.Errorf("error = %q, want it to name the column missing an action", err)
	}
}

// A Discriminator without the column holding the value it selects for classifies
// nothing, and `keys` without a Discriminator has no values to key on.
func TestAnonymizeRequiresTheDiscriminatorPairAndItsKeys(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "keys without a discriminator",
			body:    "anonymize:\n  tables:\n    usermeta:\n      keys:\n        billing_phone:\n          action: fake.phone\n",
			wantErr: "anonymize.tables.usermeta.discriminator is required",
		},
		{
			name:    "a discriminator without a value column",
			body:    "anonymize:\n  tables:\n    usermeta:\n      discriminator: meta_key\n      keys:\n        billing_phone:\n          action: fake.phone\n",
			wantErr: "anonymize.tables.usermeta.value is required",
		},
		{
			name:    "a discriminator with no keys",
			body:    "anonymize:\n  tables:\n    usermeta:\n      discriminator: meta_key\n      value: meta_value\n",
			wantErr: "anonymize.tables.usermeta.keys is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseErr(t, valid+tt.body)
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// The strict loader reaches all the way down. A typo under a column must never
// silently mean "unclassified, pass it through".
func TestAnonymizeRejectsUnknownKeys(t *testing.T) {
	for _, body := range []string{
		"anonymize:\n  tables:\n    users:\n      colums:\n        email:\n          action: keep\n",
		"anonymize:\n  tables:\n    users:\n      columns:\n        email:\n          actoin: keep\n",
		"anonymize:\n  presets: wordpress\n",
	} {
		err := parseErr(t, valid+body)
		if !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("error = %q, want an unknown-field error for:\n%s", err, body)
		}
	}
}

// Of every key a column could not have, this is the one it would plausibly be given:
// the model is three axes, and writing the third one in the wrong home is the collapse
// ADR 0010 exists to prevent. It gets a sentence rather than "unknown field".
func TestApprovedIsNotSpellableOnAColumn(t *testing.T) {
	body := "anonymize:\n  tables:\n    users:\n      columns:\n        display_name:\n          action: keep\n          approved: true\n"

	err := parseErr(t, valid+body)
	if !strings.Contains(err.Error(), "environments.<name>.anonymize.approved") {
		t.Errorf("error = %q, want it to point at where approval lives", err)
	}
	if !strings.Contains(err.Error(), "users.columns.display_name") {
		t.Errorf("error = %q, want it to name the offending column", err)
	}
}

// Authorization is not a property of a column. `approved` beside an action would make
// one project-wide answer mean different things at different destinations, which is
// the collapse ADR 0010 exists to prevent — so it is only spellable on an Environment,
// and only as a reference to a column somewhere else.
func TestApprovedParsesAsTableDotColumn(t *testing.T) {
	body := strings.Replace(valid,
		"    url: https://example.com\n",
		"    url: https://example.com\n    anonymize:\n      approved:\n        - users.display_name\n        - usermeta.umeta_id\n", 1)
	cfg := mustParse(t, body)

	approved := cfg.Environments["production"].Anonymize.Approved
	if len(approved) != 2 {
		t.Fatalf("production approved = %v, want two entries", approved)
	}
	if approved[0].Table != "users" || approved[0].Column != "display_name" {
		t.Errorf("approved[0] = %+v, want users.display_name", approved[0])
	}
	if got := approved[0].String(); got != "users.display_name" {
		t.Errorf("String() = %q, want users.display_name", got)
	}
}

func TestApproves(t *testing.T) {
	body := strings.Replace(valid,
		"    url: https://example.local.test\n",
		"    url: https://example.local.test\n    anonymize:\n      approved:\n        - users.display_name\n", 1)
	cfg := mustParse(t, body)

	local := cfg.Environments["local"]
	if !local.Approves("users", "display_name") {
		t.Error("local should approve users.display_name")
	}
	if local.Approves("users", "email") {
		t.Error("local should not approve a column it does not list")
	}
	if local.Approves("other", "display_name") {
		t.Error("approval is per table.column, not per column name")
	}
	// An Environment with no anonymize block approves nothing, rather than panicking
	// on a nil block — absence is the safe answer, not a missing one.
	if cfg.Environments["production"].Approves("users", "display_name") {
		t.Error("an environment with no anonymize block should approve nothing")
	}
}

// An explicit empty list is how an Environment says "real values, never". It has to
// parse, and it has to approve nothing.
func TestApprovedAcceptsAnEmptyList(t *testing.T) {
	body := strings.Replace(valid,
		"    url: https://example.local.test\n",
		"    url: https://example.local.test\n    anonymize:\n      approved: []\n", 1)
	cfg := mustParse(t, body)

	if cfg.Environments["local"].Approves("users", "display_name") {
		t.Error("an empty approved list should approve nothing")
	}
}

// An Approval names one column of one table. A bare column name would approve every
// table that happens to have one.
func TestApprovedRejectsAnythingThatIsNotTableDotColumn(t *testing.T) {
	for _, ref := range []string{"display_name", "db.users.display_name", ".display_name", "users.", ""} {
		body := strings.Replace(valid,
			"    url: https://example.local.test\n",
			"    url: https://example.local.test\n    anonymize:\n      approved:\n        - "+ref+"\n", 1)

		err := parseErr(t, body)
		if !strings.Contains(err.Error(), "table.column") {
			t.Errorf("error for %q = %q, want it to spell out the table.column form", ref, err)
		}
	}
}

// Approval is not cross-checked against `tables` here: a Preset supplies columns the
// file never names, so a reference to one is legitimate. What it means is settled by
// `anonymize check`, which has the Preset and the Schema; this is only its shape.
func TestApprovedNeedsNoMatchingTableEntry(t *testing.T) {
	body := strings.Replace(valid,
		"    url: https://example.local.test\n",
		"    url: https://example.local.test\n    anonymize:\n      approved:\n        - wp_posts.post_author\n", 1)

	mustParse(t, body)
}

func TestGeneratorIsTheNameAfterFake(t *testing.T) {
	tests := []struct {
		action config.Classification
		want   string
		ok     bool
	}{
		{"fake.email", "email", true},
		{"fake.full_name", "full_name", true},
		{config.Keep, "", false},
		{config.Drop, "", false},
	}

	for _, tt := range tests {
		got, ok := tt.action.Generator()
		if got != tt.want || ok != tt.ok {
			t.Errorf("Classification(%q).Generator() = %q, %v; want %q, %v", tt.action, got, ok, tt.want, tt.ok)
		}
	}
}

// A file with no anonymize block is valid. Its absence means every column is
// Unclassified, which is what makes the first Pull refuse.
func TestAnonymizeBlockIsOptional(t *testing.T) {
	cfg := mustParse(t, valid)

	if cfg.Anonymize != nil {
		t.Errorf("Anonymize = %v, want nil when the block is absent", cfg.Anonymize)
	}
}

func mustParse(t *testing.T, body string) *config.Config {
	t.Helper()

	cfg, err := config.Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse() = %v, want no error", err)
	}
	return cfg
}

func parseErr(t *testing.T, body string) error {
	t.Helper()

	_, err := config.Parse([]byte(body))
	if err == nil {
		t.Fatalf("Parse() = nil, want an error for:\n%s", body)
	}
	return err
}

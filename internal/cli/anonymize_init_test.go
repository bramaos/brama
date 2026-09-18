package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/refusal"
	"github.com/bramaos/brama/internal/renderer"
	"github.com/bramaos/brama/internal/schema"
)

// unclassifiedProject is a project brama has never classified — the state `anonymize
// init` runs in, and the state `brama init` leaves behind.
func unclassifiedProject(t *testing.T) string {
	t.Helper()
	return classifiedProject(t, "# No anonymize block yet, which means every column is unclassified and the first\n"+
		"# pull will refuse. Classify them with: brama anonymize init\n")
}

// presetlessProject is the same project under an adapter brama ships no preset for, so
// that what a generator claims is the whole of what init has to go on.
func presetlessProject(t *testing.T) string {
	t.Helper()
	root := unclassifiedProject(t)
	path := filepath.Join(root, config.Filename)
	body, err := os.ReadFile(path) //nolint:gosec // G703: path is this test's own temp dir.
	if err != nil {
		t.Fatal(err)
	}
	swapped := strings.Replace(string(body), "adapter: wordpress", "adapter: laravel", 1)
	if err := os.WriteFile(path, []byte(swapped), config.FileMode); err != nil {
		t.Fatal(err)
	}
	return root
}

// wordpressish is a schema with columns generators claim, columns they do not, and one
// column whose name a generator claims and whose type it cannot fill.
func wordpressish() schema.Schema {
	users := schema.Table{Name: "users", Columns: []schema.Column{
		{Name: "id", Type: "bigint", Declared: "bigint(20) unsigned"},
		{Name: "user_login", Type: "varchar", Declared: "varchar(60)", Length: 60},
		{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100},
		{Name: "internal_note", Type: "text", Declared: "text"},
	}}
	orders := schema.Table{Name: "orders", Columns: []schema.Column{
		{Name: "total_amount", Type: "decimal", Declared: "decimal(10,2)"},
		{Name: "billing_email", Type: "varchar", Declared: "varchar(100)", Length: 100},
	}}
	return schema.Schema{Database: "acme", Tables: []schema.Table{users, orders}}
}

// written reads the classification back out of the file the command wrote.
func written(t *testing.T, root string) *config.Config {
	t.Helper()
	cfg, _, err := config.Load(root)
	if err != nil {
		t.Fatalf("the written file does not load: %v", err)
	}
	return cfg
}

// The whole job: read a schema, write what a generator claims.
func TestAnonymizeInitWritesTheClaimedColumns(t *testing.T) {
	root := unclassifiedProject(t)
	env, out, _ := testEnv()

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v, want it to classify the schema", err)
	}

	tables := written(t, root).Anonymize.Tables
	if got := tables["users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("users.user_email.action = %q, want fake.email", got)
	}
	if got := tables["users"].Columns["user_login"].Action; got != "fake.username" {
		t.Errorf("users.user_login.action = %q, want fake.username", got)
	}
	if got := tables["orders"].Columns["billing_email"].Action; got != "fake.email" {
		t.Errorf("orders.billing_email.action = %q, want fake.email", got)
	}
	if !strings.Contains(out.String(), "staging") {
		t.Errorf("output does not name the schema it read:\n%s", out.String())
	}
}

// ADR 0012: two outcomes and no third. What nothing claims is left out of the file,
// which leaves it Unclassified — a state that already has a consequence.
func TestAnonymizeInitOmitsWhatNothingClaims(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v", err)
	}

	users := written(t, root).Anonymize.Tables["users"].Columns
	for _, name := range []string{"internal_note", "id"} {
		if col, ok := users[name]; ok {
			t.Errorf("users.%s = %v, want it left out — nothing claims it", name, col)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, config.Filename))
	if err != nil {
		t.Fatal(err)
	}
	// Not `drop`, and not a fourth token meaning "pending" either.
	for _, forbidden := range []string{"action: drop", "pending", "unclassified:"} {
		if strings.Contains(string(body), forbidden) {
			t.Errorf("the file contains %q, want the column simply absent:\n%s", forbidden, body)
		}
	}
}

// `keep` sends real production data to a destination. It enters the file only where a
// human put it.
func TestAnonymizeInitNeverWritesKeep(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v", err)
	}

	body, err := os.ReadFile(filepath.Join(root, config.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "action: keep") {
		t.Errorf("init wrote a keep, want that decision left to a human:\n%s", body)
	}
}

// The commented note `brama init` leaves is replaced, and everything a human wrote
// around it survives byte for byte.
func TestAnonymizeInitPreservesTheRestOfTheFile(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()
	path := filepath.Join(root, config.Filename)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	head, _, found := strings.Cut(string(before), "# No anonymize block yet")
	if !found {
		t.Fatal("the fixture no longer carries the note init leaves")
	}
	if !strings.HasPrefix(string(after), head) {
		t.Errorf("the file above the block changed:\n%s", after)
	}
	if strings.Contains(string(after), "No anonymize block yet") {
		t.Errorf("the note survived, and it is no longer true:\n%s", after)
	}
}

// A file that already classifies holds reviewed decisions. Replacing them is not
// init's job, and the command says whose it is.
func TestAnonymizeInitRefusesAFileThatAlreadyClassifies(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, _, _ := testEnv()
	path := filepath.Join(root, config.Filename)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish()))

	if err == nil {
		t.Fatal("runAnonymizeInit() = nil, want the existing classification left alone")
	}
	if !strings.Contains(err.Error(), "brama anonymize review") {
		t.Errorf("error = %q, want it to point at the command that amends a classification", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("init rewrote a file that already classifies, want it untouched")
	}
}

// init classifies the columns a database has, so with no database it has nothing to
// classify. That is an inability, not a guardrail: exit 1, not 42.
func TestAnonymizeInitFailsWhenNoEnvironmentIsReachable(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()

	err := runAnonymizeInit(t.Context(), env, root, "", false, unreachable)

	if err == nil {
		t.Fatal("runAnonymizeInit() = nil, want it to report that it read no schema")
	}
	if _, isRefusal := refusal.As(err); isRefusal {
		t.Error("an unreachable environment refused, want an error — nothing was declined")
	}
	if written := written(t, root).Anonymize; written != nil {
		t.Errorf("anonymize = %v, want nothing written when nothing was read", written)
	}
}

// The columns left behind are the review list, and they are named rather than counted:
// whoever runs this next has the file open in front of them.
func TestAnonymizeInitNamesWhatItLeftUnclassified(t *testing.T) {
	root := unclassifiedProject(t)
	env, out, _ := testEnv()

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v", err)
	}

	for _, want := range []string{"users.internal_note", "orders.total_amount", "brama anonymize review"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not mention %q:\n%s", want, out.String())
		}
	}
}

// Partial, not success: init does not produce a config that can pull, and a caller
// reading only the status must not take it for one that does.
func TestAnonymizeInitReportsPartialWhileAnythingIsUnclassified(t *testing.T) {
	root := unclassifiedProject(t)
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if payload["action"] != "anonymize_init" || payload["status"] != "partial" {
		t.Errorf("payload = %v, want the anonymize_init contract, partial", payload)
	}
	if payload["unclassified_columns"] != float64(3) {
		t.Errorf("unclassified_columns = %v, want the three columns nothing claimed", payload["unclassified_columns"])
	}
	if payload["columns"] != float64(3) {
		t.Errorf("columns = %v, want the three columns a generator claimed", payload["columns"])
	}
}

// A schema every column of which is claimed leaves nothing for review, and that is the
// one run init can report as finished.
func TestAnonymizeInitReportsSuccessWhenNothingIsLeft(t *testing.T) {
	root := unclassifiedProject(t)
	env, out, _ := testEnv()
	whole := schema.Schema{Tables: []schema.Table{{Name: "users", Columns: []schema.Column{
		{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100},
	}}}}

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", whole)); err != nil {
		t.Fatalf("runAnonymizeInit() = %v", err)
	}

	if !strings.Contains(out.String(), "brama anonymize check") {
		t.Errorf("output does not point at what comes next:\n%s", out.String())
	}
	result := &AnonymizeInitResult{}
	if result.Status() != renderer.StatusSuccess {
		t.Errorf("Status() = %q, want success when nothing is left unclassified", result.Status())
	}
}

// --dry-run is what makes a write reviewable before it happens.
func TestAnonymizeInitDryRunWritesNothingAndShowsWhatItWould(t *testing.T) {
	root := unclassifiedProject(t)
	env, out, _ := testEnv()
	path := filepath.Join(root, config.Filename)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := runAnonymizeInit(t.Context(), env, root, "", true, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit(--dry-run) = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(before) != string(after) {
		t.Error("--dry-run wrote to brama.yaml, want it untouched")
	}
	if !strings.Contains(out.String(), "action: fake.email") {
		t.Errorf("--dry-run does not show what it would write:\n%s", out.String())
	}
}

// A schema brama recognises nothing in, under an adapter it ships no preset for, is not
// a file to write. Saying so is more use than an empty block, which `anonymize check`
// would refuse by name anyway.
func TestAnonymizeInitFailsWhenNoGeneratorClaimsAnything(t *testing.T) {
	root := presetlessProject(t)
	env, _, _ := testEnv()
	opaque := schema.Schema{Tables: []schema.Table{{Name: "settings", Columns: []schema.Column{
		{Name: "retry_count", Type: "int", Declared: "int(11)"},
	}}}}

	err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", opaque))

	if err == nil {
		t.Fatal("runAnonymizeInit() = nil, want it to say it recognised nothing")
	}
	if !strings.Contains(err.Error(), "brama anonymize review") {
		t.Errorf("error = %q, want it to point at where the decisions get made", err)
	}
	if written := written(t, root).Anonymize; written != nil {
		t.Errorf("anonymize = %v, want no block that decides nothing", written)
	}
}

// wordpressCore is the part of a real WordPress schema the shipped preset knows, plus
// one table it does not — the case every WordPress project is actually in.
func wordpressCore() schema.Schema {
	users := schema.Table{Name: "wp_users", Columns: []schema.Column{
		{Name: "ID", Type: "bigint", Declared: "bigint(20) unsigned"},
		{Name: "user_login", Type: "varchar", Declared: "varchar(60)", Length: 60},
		{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100},
		{Name: "display_name", Type: "varchar", Declared: "varchar(250)", Length: 250},
	}}
	posts := schema.Table{Name: "wp_posts", Columns: []schema.Column{
		{Name: "ID", Type: "bigint", Declared: "bigint(20) unsigned"},
		{Name: "post_password", Type: "varchar", Declared: "varchar(255)", Length: 255},
	}}
	plugin := schema.Table{Name: "acme_leads", Columns: []schema.Column{
		{Name: "lead_email", Type: "varchar", Declared: "varchar(100)", Length: 100},
		{Name: "internal_note", Type: "text", Declared: "text"},
	}}
	return schema.Schema{Database: "acme", Tables: []schema.Table{users, posts, plugin}}
}

// The whole of the feature at the command: the block references the preset by name, and
// the tables it already knows are not written out again.
func TestAnonymizeInitReferencesThePresetRatherThanExpandingIt(t *testing.T) {
	root := unclassifiedProject(t)
	env, out, _ := testEnv()

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressCore())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v", err)
	}

	a := written(t, root).Anonymize
	if a.Preset != "wordpress" {
		t.Errorf("preset = %q, want the preset referenced by name", a.Preset)
	}
	if _, expanded := a.Tables["wp_users"]; expanded {
		t.Errorf("tables = %v, want the preset's tables left out of the file", a.Tables)
	}
	if !strings.Contains(out.String(), "wordpress") {
		t.Errorf("output does not name the preset it referenced:\n%s", out.String())
	}
}

// A column the preset already answers for is not init's to write. Writing it would
// expand the preset one column at a time, and pin what the preset says at today's answer.
func TestAnonymizeInitDoesNotWriteWhatThePresetCovers(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressCore())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v", err)
	}

	tables := written(t, root).Anonymize.Tables
	if _, written := tables["wp_posts"]; written {
		t.Errorf("wp_posts was written — a generator claims post_password, and the preset answers for it")
	}
	if got := tables["acme_leads"].Columns["lead_email"].Action; got != "fake.email" {
		t.Errorf("acme_leads.lead_email = %q, want the plugin table still classified", got)
	}
}

// A preset covering the whole schema leaves no tables to write, and a reference to it is
// still a decision worth recording. The old "nothing was claimed" refusal is about a
// project with nothing to go on, and this project has a preset.
func TestAnonymizeInitWritesAPresetOnlyBlock(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()
	core := schema.Schema{Tables: []schema.Table{{Name: "wp_options", Columns: []schema.Column{
		{Name: "option_id", Type: "bigint", Declared: "bigint(20) unsigned"},
		{Name: "option_value", Type: "longtext", Declared: "longtext"},
	}}}}

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", core)); err != nil {
		t.Fatalf("runAnonymizeInit() = %v, want a preset reference written", err)
	}

	a := written(t, root).Anonymize
	if a.Preset != "wordpress" || len(a.Tables) != 0 {
		t.Errorf("anonymize = %+v, want the preset alone", a)
	}
}

// --env narrows which database is read, and an unknown name is a typo worth stopping
// for rather than an empty run that exits 0.
func TestAnonymizeInitRejectsAnEnvironmentThatDoesNotExist(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()

	err := runAnonymizeInit(t.Context(), env, root, "prod", false, reachable("staging", wordpressish()))

	if err == nil {
		t.Fatal("runAnonymizeInit() = nil, want an unknown environment reported")
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("error = %q, want it to list the environments there are", err)
	}

	flags := newAnonymizeInitCmd(env).Flags()
	if flags.Lookup("env") == nil || flags.Lookup("dry-run") == nil {
		t.Error("init is missing --env or --dry-run")
	}
}

// The written file is what `anonymize check` reads, and a block init wrote must not be
// one check refuses.
func TestAnonymizeInitWritesAFileCheckAccepts(t *testing.T) {
	root := unclassifiedProject(t)
	env, _, _ := testEnv()

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeInit() = %v", err)
	}

	if err := runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", wordpressish())); err != nil {
		t.Fatalf("runAnonymizeCheck() after init = %v, want the written file to hold together", err)
	}
}

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/preset"
	"github.com/bramaos/brama/internal/refusal"
	"github.com/bramaos/brama/internal/renderer"
	"github.com/bramaos/brama/internal/schema"
)

// classifiedProject writes a valid brama.yaml carrying the given anonymize block. It
// is the whole input to `check`: the command reads a file and nothing else.
func classifiedProject(t *testing.T, anonymizeBlock string) string {
	t.Helper()
	return projectFile(t, anonymizeBlock, nil)
}

// projectFile writes a two-environment brama.yaml, optionally approving a column at one of
// them. approvals maps an environment name to the `table.column` it approves.
func projectFile(t *testing.T, anonymizeBlock string, approvals map[string]string) string {
	t.Helper()
	root := t.TempDir()

	approves := func(env string) string {
		ref, ok := approvals[env]
		if !ok {
			return ""
		}
		return "    anonymize:\n      approved:\n        - " + ref + "\n"
	}

	body := `version: 1
app:
  adapter: wordpress
  paths:
    config: wp-config.php
    uploads: wp-content/uploads
environments:
  local:
    url: https://acme.local.test
` + approves("local") + `  staging:
    server: hetzner
    path: /var/www/staging
    url: https://staging.acme.test
` + approves("staging") + `servers:
  hetzner:
    host: staging.acme.test
` + anonymizeBlock

	if err := os.WriteFile(filepath.Join(root, config.Filename), []byte(body), config.FileMode); err != nil {
		t.Fatal(err)
	}
	// The project's own config, because the preset is named after the prefix this
	// declares. `wp_` is what the installer writes, and it is what the tables below are
	// spelled with; a project on another prefix is prefixedProject.
	wpConfig(t, root, "wp_")
	return root
}

// wpConfig writes the one line of wp-config.php brama reads: what this install's tables
// are prefixed with. A project without it is a project whose preset cannot be named, and
// `check` refuses rather than assuming wp_.
func wpConfig(t *testing.T, root, prefix string) {
	t.Helper()
	body := "<?php\n$table_prefix = '" + prefix + "';\n"
	if err := os.WriteFile(filepath.Join(root, "wp-config.php"), []byte(body), config.FileMode); err != nil {
		t.Fatal(err)
	}
}

// consistent is a classification with nothing wrong with it.
const consistent = `anonymize:
  tables:
    users:
      columns:
        email:
          action: fake.email
          correlate: customer
        display_name:
          action: keep
    orders:
      columns:
        billing_email:
          action: fake.email
          correlate: customer
`

// refused reads the Refusal out of an error, failing when it is not one. The
// distinction is the command's contract: a Refusal exits 42, anything else exits 1.
func refused(t *testing.T, err error) *refusal.Refusal {
	t.Helper()
	if err == nil {
		t.Fatal("check succeeded, want a refusal")
	}
	r, ok := refusal.As(err)
	if !ok {
		t.Fatalf("error = %v, want a refusal — anything else exits 1, not 42", err)
	}
	return r
}

func TestCheckPassesAConsistentFile(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want success", err)
	}
	if !strings.Contains(out.String(), "holds together") {
		t.Errorf("output does not report the outcome:\n%s", out.String())
	}
}

// It reaches nothing and writes nothing: that is what lets it run on a CI runner with
// no route to production.
func TestCheckWritesNothing(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, _, _ := testEnv()
	path := filepath.Join(root, config.Filename)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("check modified brama.yaml, want it untouched")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	// brama.yaml and the wp-config.php the prefix was read out of, and nothing check
	// left behind beside them.
	if len(entries) != 2 {
		t.Errorf("the project holds %d files, want brama.yaml and wp-config.php alone", len(entries))
	}
}

func TestCheckRefusesAnUnknownGenerator(t *testing.T) {
	root := classifiedProject(t, `anonymize:
  tables:
    users:
      columns:
        email:
          action: fake.e_mail
`)
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", unreachable))

	if r.Reason != refusal.Invalid {
		t.Errorf("Reason = %q, want %q", r.Reason, refusal.Invalid)
	}
	if !strings.Contains(r.Detail, "e_mail") {
		t.Errorf("Detail = %q, want it to quote the name nobody knows", r.Detail)
	}
}

// A file with no anonymize block is not a file that passes. Every column is
// unclassified, and the first pull refuses — ADR 0003.
func TestCheckRefusesAFileThatClassifiesNothing(t *testing.T) {
	root := classifiedProject(t, "")
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", unreachable))

	if r.Reason != refusal.Unclassified {
		t.Errorf("Reason = %q, want %q", r.Reason, refusal.Unclassified)
	}
	if r.Fix == "" {
		t.Error("Fix is empty, want the command that writes the block")
	}
}

// Every problem in one run. Someone fixing a classification has the block open in
// front of them; one problem per run is ten CI runs for a ten-minute edit.
func TestCheckReportsEveryProblemAtOnce(t *testing.T) {
	root := classifiedProject(t, `anonymize:
  tables:
    users:
      columns:
        email:
          action: fake.e_mail
        display_name:
          action: keep
          correlate: customer
`)
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", unreachable))

	if !strings.Contains(r.Detail, "2 problems") {
		t.Errorf("Detail = %q, want both problems reported", r.Detail)
	}
}

// Approval is the only part of the model that differs by destination, so it is the
// only part --env can narrow.
func TestCheckValidatesEveryEnvironmentByDefault(t *testing.T) {
	root := projectFile(t, consistent, map[string]string{"local": "users.email"})
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", unreachable))
	if !strings.Contains(r.Detail, "users.email") {
		t.Errorf("Detail = %q, want local's approval reported without being asked for it", r.Detail)
	}

	if err := runAnonymizeCheck(t.Context(), env, root, "staging", unreachable); err != nil {
		t.Errorf("runAnonymizeCheck(--env staging) = %v, want local's problem left out", err)
	}
}

func TestCheckRejectsAnEnvironmentThatDoesNotExist(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, _, _ := testEnv()

	err := runAnonymizeCheck(t.Context(), env, root, "prod", unreachable)

	if err == nil {
		t.Fatal("runAnonymizeCheck() = nil, want an unknown environment reported")
	}
	if _, isRefusal := refusal.As(err); isRefusal {
		t.Error("an unknown environment refused, want an error — nothing was declined")
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("error = %q, want it to list the environments there are", err)
	}
}

// CI gets a different renderer, not a different command. --json is already the
// documented contract, and a second one is a second thing to keep in step.
func TestCheckRendersTheResultAsJSONAndHasNoCIFlag(t *testing.T) {
	root := classifiedProject(t, consistent)
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	// Partial, not success: nothing read a schema, so column coverage went unverified.
	if payload["action"] != "anonymize_check" || payload["status"] != "partial" {
		t.Errorf("payload = %v, want the anonymize_check contract", payload)
	}
	if _, ok := payload["columns"]; !ok {
		t.Errorf("payload = %v, want the fields of the result", payload)
	}

	flags := newAnonymizeCheckCmd(env).Flags()
	if flags.Lookup("ci") != nil {
		t.Error("--ci exists, want --json to be the only machine contract")
	}
	if flags.Lookup("env") == nil {
		t.Error("--env is missing, want a way to narrow to one environment")
	}
}

// The result names the environments it checked, so --json says what was covered
// rather than leaving the caller to infer it from the flags it passed.
func TestCheckResultNamesWhatItChecked(t *testing.T) {
	result := &AnonymizeCheckResult{
		Path:         "/x/brama.yaml",
		Environments: []string{"local", "staging"},
	}

	fields := map[string]any{}
	for _, f := range result.Fields() {
		fields[f.Key] = f.Value
	}

	names, ok := fields["environments"].([]string)
	if !ok || len(names) != 2 {
		t.Errorf("environments = %v, want both names", fields["environments"])
	}
	if result.Action() != "anonymize_check" {
		t.Errorf("Action() = %q, want anonymize_check", result.Action())
	}
	// A clean offline run is not a clean bill of health.
	if len(result.Notes()) == 0 {
		t.Error("Notes() is empty, want it to say column coverage was not verified")
	}
}

// reachable is a schemaSource that answers with a fixed Schema, standing in for an
// environment brama has a route to.
func reachable(from string, s schema.Schema) schemaSource {
	return func(_ context.Context, _ *config.Config, environment string, _ map[string]string) (schema.Schema, error) {
		if environment != from {
			return schema.Schema{}, errUnreachable
		}
		return s, nil
	}
}

// usersAndOrders is the schema the `consistent` block classifies, plus one column it
// says nothing about.
func usersAndOrders(extra ...schema.Column) schema.Schema {
	users := schema.Table{Name: "users", Columns: append([]schema.Column{
		{Name: "email", Type: "varchar", Declared: "varchar(100)", Length: 100},
		{Name: "display_name", Type: "varchar", Declared: "varchar(250)", Length: 250},
	}, extra...)}
	orders := schema.Table{Name: "orders", Columns: []schema.Column{
		{Name: "billing_email", Type: "varchar", Declared: "varchar(200)", Length: 200},
	}}
	return schema.Schema{Database: "acme", Tables: []schema.Table{users, orders}}
}

// With a schema to compare against, a clean check is a clean bill of health and says so.
func TestCheckReportsSuccessOnlyWhenItVerifiedColumnCoverage(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", usersAndOrders())); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want success", err)
	}
	if !strings.Contains(out.String(), "covering every column staging has") {
		t.Errorf("output does not name what it verified:\n%s", out.String())
	}
}

// The whole point of needing a database: only a schema can say the column is there.
func TestCheckReportsAColumnTheSchemaHasAndTheFileDoesNot(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, out, _ := testEnv()
	note := schema.Column{Name: "internal_note", Type: "text", Declared: "text"}

	if err := runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", usersAndOrders(note))); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want the column reported and not refused", err)
	}
	if !strings.Contains(out.String(), "users.internal_note") {
		t.Errorf("output does not name the unclassified column:\n%s", out.String())
	}
}

// fake.email on a varchar(20) is a value truncated on insert, or an error raised
// halfway through a dump on a production server. ADR 0013 stops it in the editor.
func TestCheckRefusesAGeneratorThatCannotFitTheColumn(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, _, _ := testEnv()
	narrow := usersAndOrders()
	narrow.Tables[0].Columns[0] = schema.Column{
		Name: "email", Type: "varchar", Declared: "varchar(20)", Length: 20,
	}

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", narrow)))

	if r.Reason != refusal.Invalid {
		t.Errorf("Reason = %q, want %q", r.Reason, refusal.Invalid)
	}
	if !strings.Contains(r.Detail, "varchar(20)") {
		t.Errorf("Detail = %q, want the column that cannot hold the value", r.Detail)
	}
}

// Unverified is not success. A caller reading only the status must not take a run that
// saw no columns for one that found nothing wrong with them.
func TestCheckSaysColumnCoverageWentUnverified(t *testing.T) {
	root := classifiedProject(t, consistent)
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, Renderer: renderer.NewHuman(&out, &out)}

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want it to run to completion", err)
	}
	if !strings.Contains(out.String(), "column coverage was not verified") {
		t.Errorf("output does not say coverage went unverified:\n%s", out.String())
	}

	result := &AnonymizeCheckResult{Path: "/x/brama.yaml"}
	if result.Status() != renderer.StatusPartial {
		t.Errorf("Status() = %q, want partial — nothing read a schema", result.Status())
	}
}

// A database that answered and then could not be read is not the same as no database,
// and carrying on would report coverage as unverified for a reason someone can fix.
func TestCheckFailsWhenAReachableEnvironmentCannotBeRead(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, _, _ := testEnv()
	broken := func(context.Context, *config.Config, string, map[string]string) (schema.Schema, error) {
		return schema.Schema{}, errors.New("information_schema is not readable by this user")
	}

	err := runAnonymizeCheck(t.Context(), env, root, "", broken)

	if err == nil {
		t.Fatal("runAnonymizeCheck() = nil, want the failure reported")
	}
	if _, isRefusal := refusal.As(err); isRefusal {
		t.Error("an unreadable schema refused, want an error — nothing was declined")
	}
}

// One schema is enough, and it is the first that answers: the classification is
// project-wide, and drift between two environments is not a contradiction in the file.
func TestCheckComparesAgainstOneSchemaAndNamesWhichOne(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, _, _ := testEnv()
	var asked []string
	counting := func(ctx context.Context, cfg *config.Config, name string, discriminators map[string]string) (schema.Schema, error) {
		asked = append(asked, name)
		return reachable("staging", usersAndOrders())(ctx, cfg, name, discriminators)
	}

	if err := runAnonymizeCheck(t.Context(), env, root, "", counting); err != nil {
		t.Fatal(err)
	}

	// local sorts before staging, is unreachable, and is skipped past.
	if len(asked) != 2 || asked[0] != "local" || asked[1] != "staging" {
		t.Errorf("asked = %v, want it to stop at the first environment that answered", asked)
	}
}

// The machine contract carries the same keys either way, and `schema` is what says
// which run this was. Zero unclassified columns off a run that saw none is not a fact.
func TestCheckResultReportsCoverageInTheContract(t *testing.T) {
	coverage := anonymize.Coverage{
		Columns:      4,
		Unclassified: []anonymize.Uncovered{{Table: "users", Column: schema.Column{Name: "internal_note"}}},
	}
	verified := &AnonymizeCheckResult{Path: "/x/brama.yaml", SchemaFrom: "staging", Coverage: &coverage}
	unverified := &AnonymizeCheckResult{Path: "/x/brama.yaml"}

	for _, result := range []*AnonymizeCheckResult{verified, unverified} {
		keys := map[string]any{}
		for _, f := range result.Fields() {
			keys[f.Key] = f.Value
		}
		for _, key := range []string{"schema", "schema_columns", "unclassified_columns"} {
			if _, ok := keys[key]; !ok {
				t.Errorf("fields = %v, want key %q on every run", keys, key)
			}
		}
	}

	fields := map[string]any{}
	for _, f := range verified.Fields() {
		fields[f.Key] = f.Value
	}
	if fields["schema"] != "staging" || fields["schema_columns"] != 4 || fields["unclassified_columns"] != 1 {
		t.Errorf("fields = %v, want the comparison it made", fields)
	}
	if verified.Status() != renderer.StatusPartial {
		t.Errorf("Status() = %q, want partial with a column unclassified", verified.Status())
	}
}

// A preset is expanded in memory and never into the file, so a column it classifies is
// covered without ever appearing in brama.yaml. This is the run every WordPress project
// gets: a reference one line long, answering for a schema the file does not list.
func TestCheckCoversTheColumnsAPresetClassifies(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	env, out, _ := testEnv()
	core := schema.Schema{Database: "acme", Tables: []schema.Table{{
		Name: "wp_users",
		Columns: []schema.Column{
			{Name: "ID", Type: "bigint", Declared: "bigint(20) unsigned"},
			{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100},
		},
	}}}

	if err := runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", core)); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want the preset to answer for the schema", err)
	}
	if !strings.Contains(out.String(), "covering every column staging has") {
		t.Errorf("output does not report the preset's columns as covered:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "wordpress") {
		t.Errorf("output does not name the preset the counts came from:\n%s", out.String())
	}
}

// A preset the file references and brama does not ship resolves to nothing, and nothing
// is every column that preset was carrying. Refused by name, with the ones brama has.
//
// It is the only thing reported. Carrying on would call every column the preset was
// holding unclassified and refuse every approval of one, which is a page of consequence
// stacked on top of the one typo that caused it.
func TestCheckRefusesAPresetBramaDoesNotShipAndSaysNothingElse(t *testing.T) {
	root := projectFile(t, "anonymize:\n  preset: wordpres\n",
		map[string]string{"local": "wp_posts.post_content"})
	env, _, _ := testEnv()
	core := schema.Schema{Database: "acme", Tables: []schema.Table{{
		Name:    "wp_users",
		Columns: []schema.Column{{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100}},
	}}}

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", core)))

	if !strings.Contains(r.Detail, "wordpres") || !strings.Contains(r.Detail, "wordpress") {
		t.Errorf("Detail = %q, want the unknown preset named and the ones brama ships listed", r.Detail)
	}
	if strings.Contains(r.Detail, "problems") {
		t.Errorf("Detail = %q, want the typo alone and not what followed from it", r.Detail)
	}
}

// A preset is knowledge, not authorization. It classifies `wp_posts.post_content` as
// keep, and that grants no environment anything: the approval is a separate line a human
// writes, and check accepts it only because they did.
func TestCheckTreatsAPresetsKeepAsUnapprovedUntilAHumanApprovesIt(t *testing.T) {
	env, _, _ := testEnv()
	block := "anonymize:\n  preset: wordpress\n"

	bare := classifiedProject(t, block)
	if err := runAnonymizeCheck(t.Context(), env, bare, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want a preset that approves nothing to pass", err)
	}
	cfg, _, err := config.Load(bare)
	if err != nil {
		t.Fatal(err)
	}
	for name, environment := range cfg.Environments {
		if environment.Approves("wp_posts", "post_content") {
			t.Errorf("%s approves a column only the preset kept — a preset grants no approval", name)
		}
	}

	// And the approval a human does write is accepted, because the preset said what the
	// column holds.
	approved := projectFile(t, block, map[string]string{"local": "wp_posts.post_content"})
	if err := runAnonymizeCheck(t.Context(), env, approved, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want an approval of a preset-kept column accepted", err)
	}
}

// Preset drift, both directions, in one run.
//
// `wp_users.user_email` records `keep` where the preset ships `fake.email`: the preset is
// stricter, so brama anonymizes it already and the file is out of step until review.
// `wp_posts.post_content` records `drop` where the preset ships `keep`: the preset is
// looser, so it is held and the file's answer stands. The two are reported apart, because
// "brama is already doing this" and "brama is refusing to do this" are opposite
// instructions to whoever is reading.
func TestCheckReportsPresetDriftApartFromHeld(t *testing.T) {
	root := classifiedProject(t, `anonymize:
  preset: wordpress
  tables:
    wp_users:
      columns:
        user_email:
          action: keep
    wp_posts:
      columns:
        post_content:
          action: drop
`)
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want drift reported and not refused", err)
	}

	printed := out.String()
	for _, want := range []string{
		"is stricter than", "wp_users.user_email: keep → fake.email",
		"is looser than", "wp_posts.post_content: drop → keep",
		"brama anonymize review",
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("output does not say %q:\n%s", want, printed)
		}
	}
	if strings.Index(printed, "wp_users.user_email") > strings.Index(printed, "is looser than") {
		t.Errorf("the applied drift is printed under the held heading:\n%s", printed)
	}
}

// The machine contract lists the two separately for the same reason the prose separates
// them. One key for "the preset and the file differ" would put a tightening brama has
// already carried out and a loosening it has refused to in the same bucket.
//
// It names the columns rather than counting them: a caller that can only count has to
// send a person to the repo to find out which column it was.
func TestCheckNamesAppliedAndHeldDriftSeparatelyInTheContract(t *testing.T) {
	root := classifiedProject(t, `anonymize:
  preset: wordpress
  tables:
    wp_users:
      columns:
        user_email:
          action: keep
        user_pass:
          action: keep
    wp_posts:
      columns:
        post_content:
          action: drop
`)
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	applied, held := payload["preset_drift_applied"], payload["preset_drift_held"]
	want := []any{"wp_users.user_email: keep → fake.email", "wp_users.user_pass: keep → fake.password"}
	if !reflect.DeepEqual(applied, want) {
		t.Errorf("preset_drift_applied = %v, want the two tightened columns named", applied)
	}
	if !reflect.DeepEqual(held, []any{"wp_posts.post_content: drop → keep"}) {
		t.Errorf("preset_drift_held = %v, want the held column named", held)
	}
}

// No drift is an answer, not an absent one. A caller reading null would have to tell "the
// preset agrees with the file" from "this run did not look", and those are not the same.
func TestTheDriftKeysAreEmptyListsAndNeverNull(t *testing.T) {
	root := classifiedProject(t, consistent)
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	for _, key := range []string{"preset_drift_applied", "preset_drift_held"} {
		list, ok := payload[key].([]any)
		if !ok || len(list) != 0 {
			t.Errorf("%s = %v, want an empty list", key, payload[key])
		}
	}
}

// The columns are the caller's to act on and the person's to read, and each audience gets
// them once. A list of columns is a wrapped line in the human table and a paragraph in
// the notes, so the contract carries the list and the terminal carries the prose.
func TestTheDriftListsStayOutOfTheHumanOutput(t *testing.T) {
	root := classifiedProject(t, `anonymize:
  preset: wordpress
  tables:
    wp_users:
      columns:
        user_email:
          action: keep
`)
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}
	printed := out.String()
	if strings.Contains(printed, "preset_drift_applied") {
		t.Errorf("the contract key reached the terminal:\n%s", printed)
	}
	if strings.Count(printed, "wp_users.user_email: keep → fake.email") != 1 {
		t.Errorf("the drifted column is not named exactly once:\n%s", printed)
	}
}

// Drift leaves something for a person to do either way round, so a run that found any is
// partial. Reporting success would tell a caller there was nothing left.
func TestCheckIsPartialWhileAnythingHasDrifted(t *testing.T) {
	drifted := &AnonymizeCheckResult{
		Path:     "brama.yaml",
		Preset:   "wordpress",
		Coverage: &anonymize.Coverage{Columns: 1},
		Drift: []preset.Drift{{
			Name: "wp_users.user_email", Recorded: config.Keep, Shipped: "fake.email", Applied: true,
		}},
	}
	if drifted.Status() != renderer.StatusPartial {
		t.Errorf("Status() = %q, want partial while the file and the preset disagree", drifted.Status())
	}

	agreed := *drifted
	agreed.Drift = nil
	if agreed.Status() != renderer.StatusSuccess {
		t.Errorf("Status() = %q, want success with full coverage and no drift", agreed.Status())
	}
}

// Drift is decided on every read and written back on none. The tightening brama applies
// is applied in memory: `brama anonymize review` is the only command that edits the file,
// so a check in CI cannot dirty the checkout or turn a read into a source-control event.
func TestCheckWritesNothingWhenAPresetHasDrifted(t *testing.T) {
	root := classifiedProject(t, `anonymize:
  preset: wordpress
  tables:
    wp_users:
      columns:
        user_email:
          action: keep
`)
	env, _, _ := testEnv()
	path := filepath.Join(root, config.Filename)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("check wrote the applied tightening back into brama.yaml, want the file untouched")
	}

	// And the config a read path hands back still says what the file says.
	cfg, _, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Anonymize.Tables["wp_users"].Columns["user_email"].Action; got != config.Keep {
		t.Errorf("loaded wp_users.user_email = %q, want the file's own answer", got)
	}
}

// A project that records no classification has no baseline, so there is nothing for the
// preset to have drifted from and every preset answer applies as-is.
func TestCheckReportsNoDriftWithNothingRecorded(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}
	if strings.Contains(out.String(), "is stricter than") || strings.Contains(out.String(), "is looser than") {
		t.Errorf("output reports drift against no baseline:\n%s", out.String())
	}
}

// A tightening resolves on its own, including over an approval.
//
// The file keeps `wp_users.user_email` and local approves it — two lines that agreed with
// each other when they were written. A preset brama tightened since overrides the
// classification, which leaves the approval inert. Refusing there would blame the project
// for brama's own decision, and would stop the pull the tightening exists to make safe.
func TestCheckDoesNotRefuseAnApprovalAPresetTighteningMadeInert(t *testing.T) {
	root := projectFile(t, `anonymize:
  preset: wordpress
  tables:
    wp_users:
      columns:
        user_email:
          action: keep
`, map[string]string{"local": "wp_users.user_email"})
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want the tightening reported and not refused", err)
	}
	printed := out.String()
	if !strings.Contains(printed, "wp_users.user_email: keep → fake.email") {
		t.Errorf("output does not report the tightening:\n%s", printed)
	}
	if !strings.Contains(printed, "an approval of one of these sends nothing") {
		t.Errorf("output does not say the approval is now inert:\n%s", printed)
	}
}

// An approval of a column nothing tightened is still refused. The suppression above is
// for the columns brama overrode, and for no others.
func TestCheckStillRefusesAnApprovalOfAFakedColumn(t *testing.T) {
	root := projectFile(t, `anonymize:
  preset: wordpress
  tables:
    wp_users:
      columns:
        user_login:
          action: drop
`, map[string]string{"local": "wp_users.user_login"})
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", unreachable))

	if !strings.Contains(r.Detail, "wp_users.user_login") {
		t.Errorf("Detail = %q, want the approval of a dropped column refused", r.Detail)
	}
}

// stranded is a classification whose one kept column no generator claims, so nothing can
// stand in for it at a destination that has not approved it.
const stranded = `anonymize:
  tables:
    orders:
      columns:
        internal_blob:
          action: keep
`

// The whole of what `check` can say about a pull that does not exist yet: given this
// file and these approvals, here is what each destination would actually receive.
//
// staging approves the column and local does not, so the same line of the file resolves
// two ways — which is the model's point, and why the report names the environment.
func TestCheckReportsWhatEachEnvironmentWouldReceive(t *testing.T) {
	root := projectFile(t, consistent, map[string]string{"staging": "users.display_name"})
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want the resolution reported and not refused", err)
	}
	printed := out.String()
	if !strings.Contains(printed, "local: users.display_name keep → fake.full_name") {
		t.Errorf("output does not report the substitution at the unapproved destination:\n%s", printed)
	}
	if strings.Contains(printed, "staging: users.display_name") {
		t.Errorf("output reports a substitution at the destination that approved the column:\n%s", printed)
	}
}

// Approval is the only part of the model that differs between destinations, so it is the
// only part `--env` narrows — and narrowing it means the report is about that one.
func TestCheckNarrowsTheResolutionToTheNamedEnvironment(t *testing.T) {
	root := projectFile(t, consistent, map[string]string{"staging": "users.display_name"})

	env, out, _ := testEnv()
	if err := runAnonymizeCheck(t.Context(), env, root, "local", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}
	if !strings.Contains(out.String(), "local: users.display_name") {
		t.Errorf("--env local does not report local's substitution:\n%s", out.String())
	}

	env, out, _ = testEnv()
	if err := runAnonymizeCheck(t.Context(), env, root, "staging", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}
	if strings.Contains(out.String(), "users.display_name") {
		t.Errorf("--env staging reports a column staging approves:\n%s", out.String())
	}
}

// A `keep` with no approval and no generator is the one case brama cannot derive its way
// out of. It is reported rather than refused — the file does not contradict itself, and
// what is being named is a pull that will refuse — and the run is partial, because there
// is something left for a person to do.
func TestCheckReportsAKeptColumnWithNothingToFallBackOn(t *testing.T) {
	root := classifiedProject(t, stranded)
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want it reported and not refused", err)
	}
	printed := out.String()
	if !strings.Contains(printed, "local: orders.internal_blob") {
		t.Errorf("output does not name the column with no fallback:\n%s", printed)
	}
	if !strings.Contains(printed, "refuses") {
		t.Errorf("output does not say a pull to it refuses:\n%s", printed)
	}
}

// The three exits, on the Refusal a pull raises. It is built here rather than reached
// through the command, because the operation that raises it does not exist yet.
func TestAKeepWithNoFallbackRefusesWithTheWaysOut(t *testing.T) {
	root := classifiedProject(t, stranded)
	cfg, _, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, _, _ := anonymize.Resolve(cfg, "wp_")

	stranded := anonymize.Effective(resolved, []string{"local"}).NoFallback()
	r := anonymize.Refuse("local", stranded)

	if r == nil {
		t.Fatal("Refuse() = nil, want a refusal for a keep with no fallback")
	}
	if r.Reason != refusal.NoFallback {
		t.Errorf("Reason = %q, want %q — exit 42 with a cause a caller can branch on", r.Reason, refusal.NoFallback)
	}
	if !strings.Contains(r.Detail, "orders.internal_blob") {
		t.Errorf("Detail = %q, want the column named", r.Detail)
	}
}

// The resolution is derived on every run. Nothing about a destination is written into a
// file that is supposed to hold one answer per column.
func TestCheckWritesNoResolutionIntoTheFile(t *testing.T) {
	root := projectFile(t, consistent, map[string]string{"staging": "users.display_name"})
	env, _, _ := testEnv()
	path := filepath.Join(root, config.Filename)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("check wrote the resolution back into brama.yaml, want it derived and never stored")
	}
}

// Two keys, never one. A pull that substitutes runs and says so; a pull that has nothing
// to substitute refuses. A caller gating a release on one key for both would act on the
// wrong half of the answer.
func TestCheckNamesSubstitutedAndStrandedColumnsSeparatelyInTheContract(t *testing.T) {
	root := classifiedProject(t, `anonymize:
  tables:
    users:
      columns:
        display_name:
          action: keep
    orders:
      columns:
        internal_blob:
          action: keep
`)
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}

	if err := runAnonymizeCheck(t.Context(), env, root, "local", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	substituted := []any{"local: users.display_name keep → fake.full_name"}
	if !reflect.DeepEqual(payload["keep_substituted"], substituted) {
		t.Errorf("keep_substituted = %v, want %v", payload["keep_substituted"], substituted)
	}
	if !reflect.DeepEqual(payload["keep_no_fallback"], []any{"local: orders.internal_blob"}) {
		t.Errorf("keep_no_fallback = %v, want the stranded column named", payload["keep_no_fallback"])
	}
}

// Both keys are always present. An empty list means every kept column resolves to what
// the file already says; a missing key would mean the run did not look.
func TestTheResolutionKeysAreEmptyListsAndNeverNull(t *testing.T) {
	root := projectFile(t, consistent, map[string]string{
		"local": "users.display_name", "staging": "users.display_name",
	})
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	for _, key := range []string{"keep_substituted", "keep_no_fallback"} {
		list, ok := payload[key].([]any)
		if !ok || len(list) != 0 {
			t.Errorf("%s = %v, want an empty list", key, payload[key])
		}
	}
}

// prefixed classifies a key/value table by exact names, a prefix and the empty value,
// which `check` settles from the file alone.
const prefixed = `anonymize:
  tables:
    plugin_settings:
      discriminator: setting
      value: payload
      keys:
        _cache_*:
          action: keep
        _cache_owner_email:
          action: fake.email
        "":
          action: drop
      columns:
        id:
          action: keep
`

func TestCheckPassesPrefixEntriesAndTheirApprovalOffline(t *testing.T) {
	root := projectFile(t, prefixed, map[string]string{"staging": "plugin_settings.setting=_cache_*"})
	env, _, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want success", err)
	}
}

func TestCheckRefusesAStarInsideAKeyOffline(t *testing.T) {
	root := classifiedProject(t, strings.Replace(prefixed, "_cache_*:", "_cache_*_tmp:", 1))
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", unreachable))

	if r.Reason != refusal.Invalid {
		t.Errorf("Reason = %q, want %q", r.Reason, refusal.Invalid)
	}
	if !strings.Contains(r.Detail, "write _cache_*") {
		t.Errorf("Detail = %q, want it to name the prefix to write", r.Detail)
	}
}

// keyed is the consistent classification plus a key/value table classified per key,
// exactly and by prefix. The table has no column but the two its keys answer for, so
// nothing here is a `keep` waiting on an approval.
const keyed = consistent + `    usermeta:
      discriminator: meta_key
      value: meta_value
      keys:
        billing_email:
          action: fake.email
        _transient_*:
          action: drop
`

// usermetaSchema is the schema keyed classifies, the usermeta table included.
func usermetaSchema() schema.Schema {
	s := usersAndOrders()
	s.Tables = append(s.Tables, schema.Table{Name: "usermeta", Columns: []schema.Column{
		{Name: "meta_key", Type: "varchar", Declared: "varchar(255)", Length: 255},
		{Name: "meta_value", Type: "longtext", Declared: "longtext"},
	}})
	return s
}

// withKeys is a schemaSource that answers from one environment the way schema.Read
// does: with s, and the values of each Discriminator it was asked for, out of values by
// table. A Discriminator nobody asked for stays unread, so a test passes only if the
// command asks.
func withKeys(from string, s schema.Schema, values map[string][]string) schemaSource {
	return func(_ context.Context, _ *config.Config, environment string, discriminators map[string]string) (schema.Schema, error) {
		if environment != from {
			return schema.Schema{}, errUnreachable
		}
		out := schema.Schema{Database: s.Database}
		for _, t := range s.Tables {
			if column, ok := discriminators[t.Name]; ok {
				t.Discriminator = schema.Discriminator{Column: column, Values: values[t.Name]}
			}
			out.Tables = append(out.Tables, t)
		}
		return out, nil
	}
}

// A key/value table's schema is four columns and says nothing about its keys, so the
// keys read from its Discriminator are what check covers there. Every one nothing
// classifies refuses, and every one is named — not the first, and not a count.
func TestCheckRefusesEveryUnclassifiedDiscriminatorValue(t *testing.T) {
	root := classifiedProject(t, keyed)
	env, _, _ := testEnv()
	source := withKeys("staging", usermetaSchema(), map[string][]string{"usermeta": {
		"", "Billing_Email", "_transient_doing_cron", "billing_email", "stripe_customer_id",
	}})

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", source))

	if r.Reason != refusal.Unclassified {
		t.Errorf("Reason = %q, want %q", r.Reason, refusal.Unclassified)
	}
	for _, want := range []string{
		`usermeta.meta_key="" has no classification`,
		"usermeta.meta_key='Billing_Email' has no classification",
		"usermeta.meta_key='stripe_customer_id' has no classification",
	} {
		if !strings.Contains(r.Detail, want) {
			t.Errorf("Detail = %q, want it to say %q", r.Detail, want)
		}
	}
	for _, classified := range []string{"'billing_email'", "'_transient_doing_cron'"} {
		if strings.Contains(r.Detail, classified) {
			t.Errorf("Detail = %q, want %s left out — a keys entry matches it", r.Detail, classified)
		}
	}

	// The whole list reaches the machine contract, in the refusal Main renders.
	var out bytes.Buffer
	if err := renderer.NewJSON(&out).Refused("anonymize_check", r); err != nil {
		t.Fatalf("Refused() = %v, want the refusal rendered", err)
	}
	var payload struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if payload.Status != "refused" || payload.Reason != string(refusal.Unclassified) {
		t.Errorf("status, reason = %q, %q, want refused, unclassified", payload.Status, payload.Reason)
	}
	if got := strings.Count(payload.Detail, "has no classification"); got != 3 {
		t.Errorf("detail names %d keys, want all 3:\n%s", got, payload.Detail)
	}
}

// Every key a keys entry matches is a covered key, and a run that read them all says so.
func TestCheckPassesWhenEveryDiscriminatorValueIsClassified(t *testing.T) {
	root := classifiedProject(t, keyed)
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}
	source := withKeys("staging", usermetaSchema(), map[string][]string{"usermeta": {
		"_transient_doing_cron", "_transient_timeout_abc", "billing_email",
	}})

	if err := runAnonymizeCheck(t.Context(), env, root, "", source); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want success", err)
	}

	var payload struct {
		Status     string `json:"status"`
		SchemaKeys int    `json:"schema_keys"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if payload.Status != string(renderer.StatusSuccess) {
		t.Errorf("status = %q, want success — every column and key is answered for", payload.Status)
	}
	if payload.SchemaKeys != 3 {
		t.Errorf("schema_keys = %d, want the 3 keys read", payload.SchemaKeys)
	}
}

// A multisite network writes each site's copy of a prefixed usermeta key with the site's id
// after the prefix. The preset's numbered entries answer for every site's copy, and a key
// nobody has examined in the same table still refuses.
func TestCheckCoversEverySiteOfAMultisiteUsermeta(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	env, _, _ := testEnv()
	multisite := map[string][]string{"wp_usermeta": {
		"wp_capabilities", "wp_2_capabilities", "wp_403_user_level", "wp_2_user-settings-time",
		"wp_12_dashboard_quick_press_last_post_id",
	}}

	if err := runAnonymizeCheck(t.Context(), env, root, "", withKeys("staging", wordpressUsermeta(), multisite)); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want every site's keys covered", err)
	}

	multisite["wp_usermeta"] = append(multisite["wp_usermeta"], "wp_admin_capabilities")
	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", withKeys("staging", wordpressUsermeta(), multisite)))
	if r.Reason != refusal.Unclassified {
		t.Errorf("Reason = %q, want %q", r.Reason, refusal.Unclassified)
	}
	if want := "wp_usermeta.meta_key='wp_admin_capabilities' has no classification"; !strings.Contains(r.Detail, want) {
		t.Errorf("Detail = %q, want it to say %q", r.Detail, want)
	}
	if strings.Contains(r.Detail, "wp_2_capabilities") {
		t.Errorf("Detail = %q, want wp_2_capabilities left out — wp_{n}_capabilities matches it", r.Detail)
	}
}

// A multisite network is the twelve tables every install has, two more columns on
// users, and six site-global tables with core's own keys in the two keyed ones. The
// preset answers for all of it: nothing is Unclassified, and every generator it names
// fits the column core declares, or the run would have refused.
func TestCheckCoversAWholeMultisiteNetwork(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}
	s, keys := wordpressInstall(true)

	if err := runAnonymizeCheck(t.Context(), env, root, "", withKeys("staging", s, keys)); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want the whole network covered\n%s", err, out.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if payload["tables"] != float64(18) {
		t.Errorf("tables = %v, want the network's eighteen", payload["tables"])
	}
	if payload["unclassified_columns"] != float64(0) {
		t.Errorf("unclassified_columns = %v, want none", payload["unclassified_columns"])
	}
	if want := float64(countKeys(keys)); payload["schema_keys"] != want {
		t.Errorf("schema_keys = %v, want %v — every key of the network read and covered", payload["schema_keys"], want)
	}
}

// A single-site install has none of the network's tables, and a preset table the
// database lacks is not counted: the six the preset names for a network change nothing
// about the twelve.
func TestCheckCountsNoNetworkTableOnASingleSite(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}
	s, keys := wordpressInstall(false)

	if err := runAnonymizeCheck(t.Context(), env, root, "", withKeys("staging", s, keys)); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want a single site covered\n%s", err, out.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if payload["tables"] != float64(12) {
		t.Errorf("tables = %v, want the twelve a single site has", payload["tables"])
	}
	// What the preset classifies in those twelve: the count before the network's tables
	// were named, and two more for the `spam` and `deleted` a network adds to users.
	if payload["columns"] != float64(293) || payload["correlation_groups"] != float64(2) {
		t.Errorf("columns, correlation_groups = %v, %v, want 293, 2", payload["columns"], payload["correlation_groups"])
	}
	if payload["unclassified_columns"] != float64(0) {
		t.Errorf("unclassified_columns = %v, want none", payload["unclassified_columns"])
	}
	if want := float64(countColumns(s)); payload["schema_columns"] != want {
		t.Errorf("schema_columns = %v, want %v", payload["schema_columns"], want)
	}
}

func countKeys(keys map[string][]string) int {
	n := 0
	for _, values := range keys {
		n += len(values)
	}
	return n
}

func countColumns(s schema.Schema) int {
	n := 0
	for _, t := range s.Tables {
		n += len(t.Columns)
	}
	return n
}

// wordpressInstall is the Schema WordPress core creates under `wp_`, as
// wp_get_db_schema() declares it, and the keys core itself writes into its keyed
// tables. A network adds `spam` and `deleted` to users, the six site-global tables, and
// a second site's copy of the prefixed usermeta keys.
func wordpressInstall(multisite bool) (schema.Schema, map[string][]string) {
	const prefix = "wp_"
	id := func(name string) schema.Column {
		return schema.Column{Name: name, Type: "bigint", Declared: "bigint(20) unsigned"}
	}
	varchar := func(name string, n int64) schema.Column {
		return schema.Column{Name: name, Type: "varchar", Declared: fmt.Sprintf("varchar(%d)", n), Length: n}
	}
	typed := func(name, declared string) schema.Column {
		typ, _, _ := strings.Cut(declared, "(")
		return schema.Column{Name: name, Type: typ, Declared: declared}
	}
	meta := func(owner string) []schema.Column {
		return []schema.Column{id("meta_id"), id(owner), varchar("meta_key", 255), typed("meta_value", "longtext")}
	}
	table := func(name string, cols ...schema.Column) schema.Table {
		return schema.Table{Name: prefix + name, Columns: cols}
	}

	users := []schema.Column{
		id("ID"), varchar("user_login", 60), varchar("user_pass", 255), varchar("user_nicename", 50),
		varchar("user_email", 100), varchar("user_url", 100), typed("user_registered", "datetime"),
		varchar("user_activation_key", 255), typed("user_status", "int(11)"), varchar("display_name", 250),
	}
	if multisite {
		users = append(users, typed("spam", "tinyint(2)"), typed("deleted", "tinyint(2)"))
	}

	s := schema.Schema{Database: "acme", Tables: []schema.Table{
		table("users", users...),
		table("usermeta", id("umeta_id"), id("user_id"), varchar("meta_key", 255), typed("meta_value", "longtext")),
		table("termmeta", meta("term_id")...),
		table("terms", id("term_id"), varchar("name", 200), varchar("slug", 200), typed("term_group", "bigint(10)")),
		table("term_taxonomy", id("term_taxonomy_id"), id("term_id"), varchar("taxonomy", 32),
			typed("description", "longtext"), id("parent"), typed("count", "bigint(20)")),
		table("term_relationships", id("object_id"), id("term_taxonomy_id"), typed("term_order", "int(11)")),
		table("commentmeta", meta("comment_id")...),
		table("comments", id("comment_ID"), id("comment_post_ID"), typed("comment_author", "tinytext"),
			varchar("comment_author_email", 100), varchar("comment_author_url", 200),
			varchar("comment_author_IP", 100), typed("comment_date", "datetime"),
			typed("comment_date_gmt", "datetime"), typed("comment_content", "text"),
			typed("comment_karma", "int(11)"), varchar("comment_approved", 20), varchar("comment_agent", 255),
			varchar("comment_type", 20), id("comment_parent"), id("user_id")),
		table("links", id("link_id"), varchar("link_url", 255), varchar("link_name", 255),
			varchar("link_image", 255), varchar("link_target", 25), varchar("link_description", 255),
			varchar("link_visible", 20), id("link_owner"), typed("link_rating", "int(11)"),
			typed("link_updated", "datetime"), varchar("link_rel", 255), typed("link_notes", "mediumtext"),
			varchar("link_rss", 255)),
		table("options", id("option_id"), varchar("option_name", 191), typed("option_value", "longtext"),
			varchar("autoload", 20)),
		table("postmeta", meta("post_id")...),
		table("posts", id("ID"), id("post_author"), typed("post_date", "datetime"),
			typed("post_date_gmt", "datetime"), typed("post_content", "longtext"), typed("post_title", "text"),
			typed("post_excerpt", "text"), varchar("post_status", 20), varchar("comment_status", 20),
			varchar("ping_status", 20), varchar("post_password", 255), varchar("post_name", 200),
			typed("to_ping", "text"), typed("pinged", "text"), typed("post_modified", "datetime"),
			typed("post_modified_gmt", "datetime"), typed("post_content_filtered", "longtext"), id("post_parent"),
			varchar("guid", 255), typed("menu_order", "int(11)"), varchar("post_type", 20),
			varchar("post_mime_type", 100), typed("comment_count", "bigint(20)")),
	}}
	keys := map[string][]string{
		prefix + "usermeta": {
			"nickname", "first_name", "last_name", "description", "rich_editing", "syntax_highlighting",
			"comment_shortcuts", "admin_color", "use_ssl", "show_admin_bar_front", "locale",
			prefix + "capabilities", prefix + "user_level", "dismissed_wp_pointers", "session_tokens",
		},
		prefix + "options":  {"siteurl", "home", "blogname", "admin_email", "cron", prefix + "user_roles", "_transient_doing_cron"},
		prefix + "postmeta": {"_edit_lock", "_edit_last", "_thumbnail_id", "_wp_attached_file"},
	}
	if !multisite {
		return s, keys
	}

	s.Tables = append(s.Tables,
		table("blogs", id("blog_id"), id("site_id"), varchar("domain", 200), varchar("path", 100),
			typed("registered", "datetime"), typed("last_updated", "datetime"), typed("public", "tinyint(2)"),
			typed("archived", "tinyint(2)"), typed("mature", "tinyint(2)"), typed("spam", "tinyint(2)"),
			typed("deleted", "tinyint(2)"), typed("lang_id", "int(11)")),
		table("blogmeta", meta("blog_id")...),
		table("registration_log", id("ID"), varchar("email", 255), varchar("IP", 30), id("blog_id"),
			typed("date_registered", "datetime")),
		table("site", id("id"), varchar("domain", 200), varchar("path", 100)),
		table("sitemeta", meta("site_id")...),
		table("signups", id("signup_id"), varchar("domain", 200), varchar("path", 100), typed("title", "longtext"),
			varchar("user_login", 60), varchar("user_email", 100), typed("registered", "datetime"),
			typed("activated", "datetime"), typed("active", "tinyint(1)"), varchar("activation_key", 50),
			typed("meta", "longtext")),
	)
	keys[prefix+"usermeta"] = append(keys[prefix+"usermeta"],
		"primary_blog", "source_domain", prefix+"2_capabilities", prefix+"2_user_level",
		prefix+"2_dashboard_quick_press_last_post_id",
	)
	keys[prefix+"blogmeta"] = []string{"db_version", "db_last_updated"}
	keys[prefix+"sitemeta"] = []string{
		// What populate_network_meta() writes for a new network.
		"site_name", "admin_email", "admin_user_id", "registration", "upload_filetypes",
		"blog_upload_space", "fileupload_maxk", "site_admins", "allowedthemes", "illegal_names",
		"wpmu_upgrade_site", "welcome_email", "first_post", "siteurl", "add_new_users",
		"upload_space_check_disabled", "subdomain_install", "ms_files_rewriting", "user_count",
		"initial_db_version", "active_sitewide_plugins", "WPLANG",
		// What the Network Settings screen saves.
		"registrationnotification", "menu_items", "first_page", "first_comment",
		"first_comment_url", "first_comment_author", "first_comment_email",
		"welcome_user_email", "limited_email_domains", "banned_email_domains", "new_admin_email",
		// What core writes later, as the network runs.
		"blog_count", "main_site", "recently_activated", "can_compress_scripts",
		"auto_update_plugins", "auto_update_themes", "auto_update_core_major",
		"dismissed_update_core", "auto_core_update_failed", "auto_core_update_notified",
		"global_terms_enabled", "site_meta_supported", "using_application_passwords",
		"wp_force_deactivated_plugins", "network_admin_hash", "secret_key",
		"auth_key", "auth_salt", "secure_auth_key", "secure_auth_salt", "logged_in_key",
		"logged_in_salt", "nonce_key", "nonce_salt", "recovery_mode_auth_key",
		"recovery_mode_auth_salt", "_site_transient_update_core", "_site_transient_timeout_theme_roots",
	}
	return s, keys
}

// With no environment in reach no key was read, and a key nothing classifies is still
// unclassified. That is said, beside column coverage, and the status is partial.
func TestCheckSaysKeyCoverageWentUnverified(t *testing.T) {
	root := classifiedProject(t, keyed)

	var human bytes.Buffer
	env := &console{Out: &human, Err: &human, Renderer: renderer.NewHuman(&human, &human)}
	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want it to run to completion", err)
	}
	for _, want := range []string{"column coverage was not verified", "key coverage was not verified"} {
		if !strings.Contains(human.String(), want) {
			t.Errorf("output does not say %q:\n%s", want, human.String())
		}
	}

	var out bytes.Buffer
	env = &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}
	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want it to run to completion", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if payload["status"] != string(renderer.StatusPartial) {
		t.Errorf("status = %v, want partial — no key was read", payload["status"])
	}
	if payload["schema"] != nil {
		t.Errorf("schema = %v, want null — no environment answered", payload["schema"])
	}
	if payload["schema_keys"] != float64(0) {
		t.Errorf("schema_keys = %v, want 0 — nothing was counted", payload["schema_keys"])
	}
}

// A file with no key/value table has no keys to leave unverified, and says nothing about
// them.
func TestCheckSaysNothingAboutKeysWhereNothingIsKeyed(t *testing.T) {
	root := classifiedProject(t, consistent)
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want it to run to completion", err)
	}
	if strings.Contains(out.String(), "key coverage") {
		t.Errorf("output mentions key coverage on a file that keys nothing:\n%s", out.String())
	}
}

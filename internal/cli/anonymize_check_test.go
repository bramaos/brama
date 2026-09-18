package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
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
	return root
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
	if len(entries) != 1 {
		t.Errorf("the project holds %d files, want only brama.yaml", len(entries))
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
	return func(_ context.Context, _ *config.Config, environment string) (schema.Schema, error) {
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
	broken := func(context.Context, *config.Config, string) (schema.Schema, error) {
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
	counting := func(ctx context.Context, cfg *config.Config, name string) (schema.Schema, error) {
		asked = append(asked, name)
		return reachable("staging", usersAndOrders())(ctx, cfg, name)
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

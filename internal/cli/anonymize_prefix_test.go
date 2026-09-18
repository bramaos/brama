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

// coreTables is what a WordPress install's own tables are called under a given prefix:
// the accounts table, and the two the preset's counting tests are about.
func coreTables(prefix string) schema.Schema {
	return schema.Schema{Database: "acme", Tables: []schema.Table{{
		Name: prefix + "users",
		Columns: []schema.Column{
			{Name: "ID", Type: "bigint", Declared: "bigint(20) unsigned"},
			{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100},
		},
	}}}
}

// The point of the whole thing: `acme_` is an ordinary prefix — hardening guides
// recommend moving off `wp_` — and the preset has to answer for the tables this install
// actually has.
func TestCheckMatchesThePresetAgainstTheProjectsOwnPrefix(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	wpConfig(t, root, "acme_")
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", coreTables("acme_"))); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want the preset to answer for acme_users", err)
	}
	if !strings.Contains(out.String(), "covering every column staging has") {
		t.Errorf("output does not report the preset's columns as covered:\n%s", out.String())
	}
}

// The other half of the same sentence: the preset must not answer for `wp_users` on a
// project whose tables are `acme_`. Classifying by resemblance would apply the accounts
// table's classification to whatever table sorted into place — the inference ADR 0013
// keeps out of the generators.
func TestCheckDoesNotMatchThePresetAgainstTheDefaultPrefix(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	wpConfig(t, root, "acme_")
	env, out, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", coreTables("wp_"))); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}
	if !strings.Contains(out.String(), "unclassified") {
		t.Errorf("wp_users was classified on a project whose prefix is acme_:\n%s", out.String())
	}
}

// A prefix brama cannot determine is a refusal naming what it read, never a fallback to
// the framework's default.
func TestCheckRefusesAPrefixItCannotDetermine(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	if err := os.WriteFile(filepath.Join(root, "wp-config.php"),
		[]byte("<?php\n$table_prefix = getenv('WP_PREFIX');\n"), config.FileMode); err != nil {
		t.Fatal(err)
	}
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", coreTables("wp_"))))

	if r.Reason != refusal.UnknownPrefix {
		t.Errorf("Reason = %q, want %q", r.Reason, refusal.UnknownPrefix)
	}
	if !strings.Contains(r.Detail, "wp-config.php") {
		t.Errorf("Detail = %q, want it to name what brama looked in", r.Detail)
	}
	if !strings.Contains(r.Detail, "wordpress") {
		t.Errorf("Detail = %q, want it to say which preset is waiting on the prefix", r.Detail)
	}
}

// A project that names no preset has no prefix to resolve, and nothing should go looking
// for one. Checking a classification written out column by column has never needed the
// framework's config, and it must keep working where there is none — on a CI runner with
// a checkout and nothing else.
func TestCheckAsksForNoPrefixWhereNoPresetWantsOne(t *testing.T) {
	root := classifiedProject(t, consistent)
	if err := os.Remove(filepath.Join(root, "wp-config.php")); err != nil {
		t.Fatal(err)
	}
	env, _, _ := testEnv()

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want a presetless file checked without a prefix", err)
	}
}

// The headline is a count of what this project classifies, and a preset table the
// database does not have is not one of them. Counting the preset's twelve tables on an
// install that has one is the headline overstating itself — which is what it did on
// every project whose prefix did not match.
func TestCheckCountsOnlyThePresetTablesTheSchemaHas(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}

	if err := runAnonymizeCheck(t.Context(), env, root, "", reachable("staging", coreTables("wp_"))); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if payload["tables"] != float64(1) {
		t.Errorf("tables = %v, want the one preset table staging has", payload["tables"])
	}
}

// With no schema in reach there is nothing to say which tables exist, so the count is
// the whole preset rather than none of it. A run that reported zero would read as a file
// that classifies nothing, which is the opposite of what it says.
func TestCheckCountsTheWholePresetWithNoSchemaToNarrowIt(t *testing.T) {
	root := classifiedProject(t, "anonymize:\n  preset: wordpress\n")
	var out bytes.Buffer
	env := &console{Out: &out, Err: &out, JSON: true, Renderer: renderer.NewJSON(&out)}

	if err := runAnonymizeCheck(t.Context(), env, root, "", unreachable); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if payload["tables"] != float64(12) {
		t.Errorf("tables = %v, want the preset's twelve where no schema could narrow them", payload["tables"])
	}
}

// init and check read the same prefix from the same file, so what init decides the
// preset covers is what check finds covered. The two disagreeing is the bug this is
// about: init writing `preset: wordpress` off app.adapter alone, and check then counting
// tables the database does not have.
func TestInitAndCheckAgreeOnWhatThePresetCovers(t *testing.T) {
	root := unclassifiedProject(t)
	wpConfig(t, root, "acme_")
	env, _, _ := testEnv()
	core := coreTables("acme_")

	if err := runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", core)); err != nil {
		t.Fatalf("runAnonymizeInit() = %v, want the preset to cover acme_users", err)
	}
	written := written(t, root)
	if written.Anonymize.Preset != "wordpress" {
		t.Fatalf("preset = %q, want the file to reference it", written.Anonymize.Preset)
	}
	if _, expanded := written.Anonymize.Tables["acme_users"]; expanded {
		t.Error("acme_users was written into the file, want the preset referenced rather than expanded")
	}

	checked, checkOut, _ := testEnv()
	if err := runAnonymizeCheck(t.Context(), checked, root, "", reachable("staging", core)); err != nil {
		t.Fatalf("runAnonymizeCheck() = %v, want check to agree with what init wrote", err)
	}
	if !strings.Contains(checkOut.String(), "covering every column staging has") {
		t.Errorf("check disagrees with init about what the preset covers:\n%s", checkOut.String())
	}
}

// init refuses for the same reason check does, and before it writes anything: a file
// carrying `preset: wordpress` on a project whose prefix nobody can read is a file whose
// preset line covers nothing.
func TestInitRefusesAPrefixItCannotDetermine(t *testing.T) {
	root := unclassifiedProject(t)
	if err := os.Remove(filepath.Join(root, "wp-config.php")); err != nil {
		t.Fatal(err)
	}
	env, _, _ := testEnv()

	r := refused(t, runAnonymizeInit(t.Context(), env, root, "", false, reachable("staging", coreTables("wp_"))))

	if r.Reason != refusal.UnknownPrefix {
		t.Errorf("Reason = %q, want %q", r.Reason, refusal.UnknownPrefix)
	}
	if written(t, root).Anonymize != nil {
		t.Error("init wrote a classification on a project whose tables it cannot name")
	}
}

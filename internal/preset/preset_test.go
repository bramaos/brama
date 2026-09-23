package preset_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/generator"
	"github.com/bramaos/brama/internal/preset"
)

// prefix is a project's own table prefix, which every lookup needs before a preset can
// name anything. `wp_` is what the WordPress installer writes, so the tests below read
// as the tables everybody knows — the ones that say prefixing works pass another.
const prefix = "wp_"

// every is the set brama ships, by name, so a preset added later is checked by
// everything below without anyone remembering to add it.
func every(t *testing.T) []preset.Preset {
	t.Helper()
	out := make([]preset.Preset, 0, len(preset.Names()))
	for _, name := range preset.Names() {
		p, err := preset.Lookup(name, prefix)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", name, err)
		}
		out = append(out, p)
	}
	return out
}

// A shipped Preset has to survive the same check a hand-written classification does.
// It reaches projects without anyone reviewing it, so an inconsistency in one is an
// inconsistency in every project that names it — caught here, before release.
func TestEveryShippedPresetHoldsTogether(t *testing.T) {
	for _, p := range every(t) {
		cfg := &config.Config{
			Version:   config.SchemaVersion,
			App:       config.App{Adapter: p.Name},
			Anonymize: mustApply(t, p.Name),
		}
		if _, problems := anonymize.Check(cfg, nil, nil, nil); len(problems) > 0 {
			t.Errorf("the %s preset does not hold together: %v", p.Name, problems)
		}
	}
}

// Every classification in a shipped Preset is one brama.yaml could have carried. The
// preset is data written in Go, so nothing parsed it on the way in.
func TestEveryShippedPresetWritesARealClassification(t *testing.T) {
	for _, p := range every(t) {
		for name, tbl := range p.Tables {
			for column, col := range tbl.Columns {
				assertAction(t, p.Name, name+"."+column, col)
			}
			for key, col := range tbl.Keys {
				assertAction(t, p.Name, name+"."+tbl.Discriminator+"="+key, col)
			}
		}
	}
}

func assertAction(t *testing.T, preset, at string, col config.Column) {
	t.Helper()
	if err := col.Action.Validate(); err != nil {
		t.Errorf("%s preset, %s: %v", preset, at, err)
		return
	}
	name, fakes := col.Action.Generator()
	if !fakes {
		return
	}
	if _, err := generator.Lookup(name); err != nil {
		t.Errorf("%s preset, %s: %v", preset, at, err)
	}
}

// A Preset is named after the Adapter whose tables it knows, so that `app.adapter` is
// the only thing `anonymize init` has to read to find one.
func TestEveryShippedPresetIsFoundByItsAdapterName(t *testing.T) {
	for _, p := range every(t) {
		found, ships, err := preset.For(p.Name, prefix)
		if !ships || err != nil || found.Name != p.Name {
			t.Errorf("For(%q) = %v, %v, %v, want the preset of that adapter", p.Name, found.Name, ships, err)
		}
	}
	if _, ships, _ := preset.For("symfony", prefix); ships {
		t.Error("For(symfony) found a preset brama does not ship")
	}
}

// An unknown name is an error and never an empty Preset: resolving to nothing would
// leave every column the preset was carrying unclassified, which reads as a project
// that decided nothing rather than as a typo.
func TestLookupRefusesANameBramaDoesNotShip(t *testing.T) {
	_, err := preset.Lookup("wordpres", prefix)

	if err == nil {
		t.Fatal("Lookup(wordpres) = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "wordpress") {
		t.Errorf("error = %q, want the presets brama does ship listed", err)
	}
}

// The prefix is half of every table's name, and it is the project's half. A preset that
// only matched `wp_` would match nothing at all on a hardened install — silently, and in
// a way that reads as "brama recognised nothing" rather than as a prefix that differs.
func TestLookupNamesTheTablesForThisProjectsPrefix(t *testing.T) {
	p, err := preset.Lookup("wordpress", "acme_")
	if err != nil {
		t.Fatalf("Lookup(wordpress, acme_): %v", err)
	}

	if _, known := p.Tables["acme_users"]; !known {
		t.Errorf("tables = %v, want the accounts table named acme_users", slices.Sorted(maps.Keys(p.Tables)))
	}
	if _, stale := p.Tables["wp_users"]; stale {
		t.Error("wp_users survived a project whose prefix is acme_ — a table this database does not have")
	}
	if got := p.Tables["acme_users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("acme_users.user_email = %q, want the classification carried over with the name", got)
	}
}

// The placeholder is never a table name. A preset resolved without a prefix would
// classify `{prefix}users`, which matches nothing and says nothing about why.
func TestLookupRefusesAPresetItCannotName(t *testing.T) {
	_, err := preset.Lookup("wordpress", "")

	if err == nil {
		t.Fatal("Lookup(wordpress, \"\") = nil, want a refusal rather than a preset named after nothing")
	}
	if !preset.NeedsPrefix("wordpress") {
		t.Error("NeedsPrefix(wordpress) = false, want the preset to say it is waiting on a prefix")
	}
	if preset.NeedsPrefix("symfony") {
		t.Error("NeedsPrefix(symfony) = true for a preset brama does not ship")
	}
}

// The vocabulary is closed, and handing a caller a writable alias of it would be that
// decision written down and then left unenforced: one project's override would become
// every project's preset for the life of the process.
func TestLookupHandsBackACopy(t *testing.T) {
	first := mustLookup(t, "wordpress")

	first.Tables["wp_users"].Columns["user_email"] = config.Column{Action: config.Keep}
	delete(first.Tables, "wp_posts")

	second := mustLookup(t, "wordpress")
	if got := second.Tables["wp_users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("wp_users.user_email = %q, want the shipped classification unchanged", got)
	}
	if _, ok := second.Tables["wp_posts"]; !ok {
		t.Error("wp_posts was deleted from the shipped preset by a caller")
	}
}

// A preset is a deliberate statement about a table an adapter actually knows; a
// generator pattern is an inference about a column name. The specific beats the general.
//
// `wp_posts.post_password` is the case in the shipped classification: the name matches
// `fake.password`'s declared pattern, and WordPress stores that column in plain text
// and compares it in plain text, so a hash fabricated into it is a post nobody can open.
func TestThePresetAnswerBeatsAGeneratorPatternClaim(t *testing.T) {
	p := mustLookup(t, "wordpress")

	claimed, byPattern := generator.ClaimName("post_password")
	if !byPattern {
		t.Fatal("no generator claims post_password by name — this test no longer tests anything")
	}

	got := p.Tables["wp_posts"].Columns["post_password"].Action
	if got != config.Drop {
		t.Errorf("wp_posts.post_password = %q, want the preset's own answer", got)
	}
	if name, _ := got.Generator(); name == claimed.Name {
		t.Errorf("wp_posts.post_password fell back to fake.%s, the pattern's inference", claimed.Name)
	}
}

// A preset supplies knowledge, never authorization. It classifies columns `keep` where
// that is what they hold, and it approves nothing anywhere: Approval lives on the
// Environment and only a human grants one.
func TestAShippedPresetKeepsColumnsAndApprovesNone(t *testing.T) {
	a := mustApply(t, "wordpress")

	if got := a.Tables["wp_posts"].Columns["post_content"].Action; got != config.Keep {
		t.Fatalf("wp_posts.post_content = %q, want a preset that does say keep", got)
	}

	cfg := &config.Config{Anonymize: a, Environments: map[string]config.Environment{"local": {}}}
	if cfg.Environments["local"].Approves("wp_posts", "post_content") {
		t.Error("a preset's keep approved an environment — knowledge is not authorization")
	}
}

// The tables a default single-site WordPress creates, so `anonymize init` on a vanilla
// install has nothing left to leave unclassified.
func TestTheWordPressPresetCoversTheCoreTables(t *testing.T) {
	a := mustApply(t, "wordpress")

	for _, table := range []string{
		"wp_commentmeta", "wp_comments", "wp_links", "wp_options", "wp_postmeta",
		"wp_posts", "wp_term_relationships", "wp_term_taxonomy", "wp_termmeta",
		"wp_terms", "wp_usermeta", "wp_users",
	} {
		if _, known := a.Tables[table]; !known {
			t.Errorf("the wordpress preset says nothing about %s", table)
		}
	}
}

// mustLookup is the shipped preset, failing when it is not there. Most of these tests
// are about what it says, not about it being found.
func mustLookup(t *testing.T, name string) preset.Preset {
	t.Helper()
	p, err := preset.Lookup(name, prefix)
	if err != nil {
		t.Fatalf("Lookup(%q): %v", name, err)
	}
	return p
}

func mustApply(t *testing.T, name string) *config.Anonymize {
	t.Helper()
	a, _ := mustLookup(t, name).Apply(nil)
	return a
}

// columns is one table classified per column, the shape most of the merge tests bend.
func columns(cols map[string]config.Column) config.Table {
	return config.Table{Columns: cols}
}

// A project's own answer is the deliberate override, and it is per column. Writing one
// column of `wp_users` must not take responsibility for the other nine — which is the
// whole reason the preset is referenced rather than expanded.
//
// The override here exposes no more than the preset does, which is the only kind the file
// settles on its own: one that loosened the preset would be held for review instead.
func TestApplyOverridesOneColumnAndLeavesTheRestToThePreset(t *testing.T) {
	p := mustLookup(t, "wordpress")

	a, _ := p.Apply(&config.Anonymize{Preset: "wordpress", Tables: map[string]config.Table{
		"wp_users": columns(map[string]config.Column{"display_name": {Action: config.Drop}}),
	}})

	users := a.Tables["wp_users"]
	if got := users.Columns["display_name"].Action; got != config.Drop {
		t.Errorf("wp_users.display_name = %q, want the file's answer", got)
	}
	if got := users.Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("wp_users.user_email = %q, want the preset's answer left standing", got)
	}
}

// A table the preset says nothing about — a plugin's, or the application's own — is the
// project's alone and carries over as written.
func TestApplyKeepsATableThePresetDoesNotKnow(t *testing.T) {
	p := mustLookup(t, "wordpress")

	a, _ := p.Apply(&config.Anonymize{Tables: map[string]config.Table{
		"acme_leads": columns(map[string]config.Column{"lead_email": {Action: "fake.email"}}),
	}})

	if got := a.Tables["acme_leads"].Columns["lead_email"].Action; got != "fake.email" {
		t.Errorf("acme_leads.lead_email = %q, want the project's own table kept", got)
	}
}

// Writing one ordinary column of a key/value table must not unset the discriminator the
// preset named. An empty field in the file is not an answer, and a table with keys and
// no discriminator classifies nothing at all.
func TestApplyKeepsTheDiscriminatorWhenTheFileOverridesAColumn(t *testing.T) {
	p := mustLookup(t, "wordpress")

	a, _ := p.Apply(&config.Anonymize{Tables: map[string]config.Table{
		"wp_usermeta": columns(map[string]config.Column{"user_id": {Action: config.Drop}}),
	}})

	meta := a.Tables["wp_usermeta"]
	if meta.Discriminator != "meta_key" || meta.Value != "meta_value" {
		t.Errorf("discriminator/value = %q/%q, want the preset's kept", meta.Discriminator, meta.Value)
	}
	if got := meta.Keys["first_name"].Action; got != "fake.first_name" {
		t.Errorf("wp_usermeta first_name key = %q, want the preset's keys kept", got)
	}
	if got := meta.Columns["user_id"].Action; got != config.Drop {
		t.Errorf("wp_usermeta.user_id = %q, want the file's override", got)
	}
}

// A key the preset does not classify is added to the ones it does, not swapped for them.
// A plugin writing `acme_vat_number` into usermeta is one more decision, not a new set.
func TestApplyAddsAKeyToTheOnesThePresetClassifies(t *testing.T) {
	p := mustLookup(t, "wordpress")

	a, _ := p.Apply(&config.Anonymize{Tables: map[string]config.Table{
		"wp_usermeta": {Keys: map[string]config.Column{"acme_vat_number": {Action: config.Drop}}},
	}})

	meta := a.Tables["wp_usermeta"]
	if got := meta.Keys["acme_vat_number"].Action; got != config.Drop {
		t.Errorf("acme_vat_number = %q, want the project's key added", got)
	}
	if got := meta.Keys["last_name"].Action; got != "fake.last_name" {
		t.Errorf("last_name = %q, want the preset's keys kept beside it", got)
	}
}

// Applying to a file must not write through to what brama ships, or one project's
// override would become every project's preset for the life of the process.
func TestApplyDoesNotWriteBackIntoTheShippedPreset(t *testing.T) {
	p := mustLookup(t, "wordpress")

	_, _ = p.Apply(&config.Anonymize{Tables: map[string]config.Table{
		"wp_users": columns(map[string]config.Column{"user_email": {Action: config.Keep}}),
	}})

	again := mustApply(t, "wordpress")
	if got := again.Tables["wp_users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("wp_users.user_email = %q, want the shipped classification unchanged", got)
	}
}

package preset_test

import (
	"testing"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/preset"
)

// only is the one Drift a case produced, and a failure when it produced any other number.
func only(t *testing.T, drift []preset.Drift) preset.Drift {
	t.Helper()
	if len(drift) != 1 {
		t.Fatalf("Apply() drift = %v, want exactly one", drift)
	}
	return drift[0]
}

// A preset moving a column off `keep` narrows what leaves production. It applies on its
// own: a security fix brama ships has to reach a project nobody is editing, and holding
// it for review is how it sits inert in every project until someone thinks to look.
func TestAPresetTighteningAppliesWithNoReview(t *testing.T) {
	p := mustLookup(t, "wordpress")

	a, drift := p.Apply(&config.Anonymize{Preset: "wordpress", Tables: map[string]config.Table{
		"wp_users": columns(map[string]config.Column{"user_email": {Action: config.Keep}}),
	}})

	if got := a.Tables["wp_users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("wp_users.user_email = %q, want the preset's stricter answer applied", got)
	}
	d := only(t, drift)
	if !d.Applied {
		t.Errorf("drift = %+v, want it applied", d)
	}
	if d.Name != "wp_users.user_email" || d.Recorded != config.Keep || d.Shipped != "fake.email" {
		t.Errorf("drift = %+v, want the column and both answers named", d)
	}
}

// A preset moving a column onto `keep` widens what leaves production, and widening is a
// decision only a human makes. The recorded answer stands until one does.
func TestAPresetLooseningIsHeldAndNeverApplies(t *testing.T) {
	p := mustLookup(t, "wordpress")

	a, drift := p.Apply(&config.Anonymize{Preset: "wordpress", Tables: map[string]config.Table{
		// The preset keeps `post_content`; a project that decided to fabricate it keeps
		// fabricating it.
		"wp_posts": columns(map[string]config.Column{"post_content": {Action: config.Drop}}),
	}})

	if got := a.Tables["wp_posts"].Columns["post_content"].Action; got != config.Drop {
		t.Errorf("wp_posts.post_content = %q, want the file's stricter answer standing", got)
	}
	d := only(t, drift)
	if d.Applied {
		t.Errorf("drift = %+v, want it held", d)
	}
	if d.Recorded != config.Drop || d.Shipped != config.Keep {
		t.Errorf("drift = %+v, want both answers named", d)
	}
}

// A key of a discriminated table drifts the same way an ordinary column does, and is
// named the way the rest of brama names one.
func TestADiscriminatorKeyDrifts(t *testing.T) {
	p := mustLookup(t, "wordpress")

	a, drift := p.Apply(&config.Anonymize{Tables: map[string]config.Table{
		"wp_usermeta": {Keys: map[string]config.Column{"first_name": {Action: config.Keep}}},
	}})

	if got := a.Tables["wp_usermeta"].Keys["first_name"].Action; got != "fake.first_name" {
		t.Errorf("first_name key = %q, want the preset's stricter answer applied", got)
	}
	d := only(t, drift)
	if d.Name != "wp_usermeta.meta_key=first_name" {
		t.Errorf("Name = %q, want the key named by its discriminator", d.Name)
	}
	if d.Table != "wp_usermeta" || d.Key != "first_name" || d.Column != "" {
		t.Errorf("drift = %+v, want the key located under keys, not columns", d)
	}
}

// A Drift says where the entry lives as well as what to call it, because
// `brama anonymize review` writes the applied half back into the file and has to find
// the entry to write. Taking the printed name apart again would be a parser for a format
// nobody defined.
func TestDriftLocatesTheEntryItIsAbout(t *testing.T) {
	p := mustLookup(t, "wordpress")

	_, drift := p.Apply(&config.Anonymize{Tables: map[string]config.Table{
		"wp_users": columns(map[string]config.Column{"user_email": {Action: config.Keep}}),
	}})

	d := only(t, drift)
	if d.Table != "wp_users" || d.Column != "user_email" || d.Key != "" {
		t.Errorf("drift = %+v, want the column located under columns, not keys", d)
	}
}

// Two answers that expose the same thing are not drift. `fake.first_name` where the
// preset says `fake.full_name`, or `drop` where it says `fake.url`, widens nothing and
// narrows nothing — it is the deliberate per-column override, and reporting it every run
// would bury the two cases that matter.
func TestAnOverrideThatExposesTheSameThingIsNotDrift(t *testing.T) {
	p := mustLookup(t, "wordpress")

	a, drift := p.Apply(&config.Anonymize{Tables: map[string]config.Table{
		"wp_users": columns(map[string]config.Column{
			"display_name": {Action: "fake.username"},
			"user_url":     {Action: config.Drop},
		}),
	}})

	if len(drift) != 0 {
		t.Errorf("Apply() drift = %v, want none", drift)
	}
	users := a.Tables["wp_users"]
	if got := users.Columns["display_name"].Action; got != "fake.username" {
		t.Errorf("wp_users.display_name = %q, want the file's answer", got)
	}
	if got := users.Columns["user_url"].Action; got != config.Drop {
		t.Errorf("wp_users.user_url = %q, want the file's answer", got)
	}
}

// A project with no recorded classification has no baseline to compare against, so every
// preset answer applies as-is and nothing has drifted. Nothing has been decided yet, so
// nothing is being overridden.
func TestNoRecordIsNoBaselineAndNoDrift(t *testing.T) {
	p := mustLookup(t, "wordpress")

	a, drift := p.Apply(nil)
	if len(drift) != 0 {
		t.Errorf("Apply(nil) drift = %v, want none", drift)
	}
	if got := a.Tables["wp_users"].Columns["user_email"].Action; got != "fake.email" {
		t.Errorf("wp_users.user_email = %q, want the preset's answer as-is", got)
	}

	// A file that names the preset and records nothing else is the same case.
	_, drift = p.Apply(&config.Anonymize{Preset: "wordpress"})
	if len(drift) != 0 {
		t.Errorf("drift on an empty record = %v, want none", drift)
	}
}

// A column the preset says nothing about cannot have drifted from it, whether it is in a
// table the preset knows or in one of the project's own.
func TestAColumnThePresetDoesNotKnowDoesNotDrift(t *testing.T) {
	p := mustLookup(t, "wordpress")

	_, drift := p.Apply(&config.Anonymize{Tables: map[string]config.Table{
		"wp_users":   columns(map[string]config.Column{"acme_billing_id": {Action: config.Keep}}),
		"acme_leads": columns(map[string]config.Column{"lead_email": {Action: config.Keep}}),
	}})

	if len(drift) != 0 {
		t.Errorf("Apply() drift = %v, want none", drift)
	}
}

// Drift is reported in one order, so two runs over one file say the same thing in the
// same sequence and a diff of two check outputs is a diff of what changed.
func TestDriftIsReportedInAStableOrder(t *testing.T) {
	p := mustLookup(t, "wordpress")

	record := &config.Anonymize{Tables: map[string]config.Table{
		"wp_users": columns(map[string]config.Column{"user_email": {Action: config.Keep}, "user_pass": {Action: config.Keep}}),
		"wp_posts": columns(map[string]config.Column{"post_content": {Action: config.Drop}}),
		"wp_comments": columns(map[string]config.Column{
			"comment_author": {Action: config.Keep}, "comment_agent": {Action: config.Keep},
		}),
	}}

	want := []string{
		"wp_comments.comment_agent", "wp_comments.comment_author",
		"wp_posts.post_content", "wp_users.user_email", "wp_users.user_pass",
	}
	for range 5 {
		_, drift := p.Apply(record)
		got := make([]string, 0, len(drift))
		for _, d := range drift {
			got = append(got, d.Name)
		}
		if len(got) != len(want) {
			t.Fatalf("drift = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("drift = %v, want %v", got, want)
			}
		}
	}
}

// Applied and held are the two ways drift is reported, and they are never one list: one
// says brama is already doing something the file does not say, the other says brama is
// refusing to do something the preset does say.
func TestAppliedAndHeldSplitTheDrift(t *testing.T) {
	p := mustLookup(t, "wordpress")

	_, drift := p.Apply(&config.Anonymize{Tables: map[string]config.Table{
		"wp_users": columns(map[string]config.Column{"user_email": {Action: config.Keep}}),
		"wp_posts": columns(map[string]config.Column{"post_content": {Action: config.Drop}}),
	}})

	applied, held := drift.Applied(), drift.Held()
	if len(applied) != 1 || applied[0].Name != "wp_users.user_email" {
		t.Errorf("Applied() = %v, want the tightening", applied)
	}
	if len(held) != 1 || held[0].Name != "wp_posts.post_content" {
		t.Errorf("Held() = %v, want the loosening", held)
	}
	if got := applied[0].String(); got != "wp_users.user_email: keep → fake.email" {
		t.Errorf("String() = %q, want the column and both answers in one line", got)
	}
}

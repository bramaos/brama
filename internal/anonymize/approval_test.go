package anonymize_test

import (
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/refusal"
)

// approving builds an Environment that approves the given references, in either the
// `table.column` or the `table.column=key` form.
func approving(refs ...string) config.Environment {
	approved := make([]config.ColumnRef, 0, len(refs))
	for _, ref := range refs {
		head, key, _ := strings.Cut(ref, "=")
		table, column, _ := strings.Cut(head, ".")
		approved = append(approved, config.ColumnRef{Table: table, Column: column, Key: key})
	}
	return config.Environment{Anonymize: &config.EnvironmentAnonymize{Approved: approved}}
}

// strings renders Fallbacks the way a reader sees them, which is the whole of what
// these tests have to say about one.
func rendered(f anonymize.Fallbacks) []string {
	out := make([]string, 0, len(f))
	for _, fallback := range f {
		out = append(out, fallback.String())
	}
	return out
}

// The substitution, in one line: the file keeps a column, the destination approves
// nothing, and a Generator claims the name — so what arrives is fabricated, and brama
// says which Generator fabricated it.
func TestEffectiveSubstitutesTheClaimingGeneratorForAnUnapprovedKeep(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"display_name": {Action: config.Keep},
		}),
	}, map[string]config.Environment{"local": {}})

	got := anonymize.Effective(cfg, []string{"local"})

	want := []string{"local: users.display_name keep → fake.full_name"}
	if !equal(rendered(got), want) {
		t.Errorf("Effective() = %v, want %v", rendered(got), want)
	}
	if len(got.NoFallback()) != 0 {
		t.Errorf("NoFallback() = %v, want none — a generator claims the column", got.NoFallback())
	}
}

// Approval is per destination and nothing else in the model is. The same column, the
// same file, two Environments, two answers.
func TestEffectiveApprovesOneEnvironmentAndNotTheOther(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"display_name": {Action: config.Keep},
		}),
	}, map[string]config.Environment{
		"staging": approving("users.display_name"),
		"local":   {},
	})

	got := anonymize.Effective(cfg, []string{"local", "staging"})

	want := []string{"local: users.display_name keep → fake.full_name"}
	if !equal(rendered(got), want) {
		t.Errorf("Effective() = %v, want the unapproved destination alone", rendered(got))
	}
}

// A `keep` nothing claims is the one case with no derived answer. Falling back to
// `drop` would be brama deciding to empty a column nobody asked it to empty.
func TestEffectiveLeavesAKeepNoGeneratorClaimsWithNoFallback(t *testing.T) {
	cfg := project(map[string]config.Table{
		"orders": columns(map[string]config.Column{
			"internal_blob": {Action: config.Keep},
		}),
	}, map[string]config.Environment{"local": {}})

	got := anonymize.Effective(cfg, []string{"local"})

	if !equal(rendered(got.NoFallback()), []string{"local: orders.internal_blob"}) {
		t.Fatalf("NoFallback() = %v, want the column named", rendered(got.NoFallback()))
	}
	if len(got.Substituted()) != 0 {
		t.Errorf("Substituted() = %v, want none — nothing claims the column", got.Substituted())
	}
}

// Only `keep` resolves against Approval. A fabricated column is fabricated everywhere
// and a dropped one sends nothing anywhere, so neither has a second answer to derive.
func TestEffectiveSaysNothingAboutFakeOrDropOrAnApprovedKeep(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"email":      {Action: "fake.email"},
			"activation": {Action: config.Drop},
			"last_name":  {Action: config.Keep},
		}),
	}, map[string]config.Environment{"local": approving("users.last_name")})

	if got := anonymize.Effective(cfg, []string{"local"}); len(got) != 0 {
		t.Errorf("Effective() = %v, want nothing — every column resolves to what the file says", rendered(got))
	}
}

// A Discriminator value cannot be approved: an Approval names a `table.column`, and the
// column these share holds every key's value at once. So a kept key resolves the same
// way an unapproved column does, by the key's own name.
func TestEffectiveResolvesAKeptDiscriminatorValueByItsKey(t *testing.T) {
	cfg := project(map[string]config.Table{
		"usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys: map[string]config.Column{
				"first_name":  {Action: config.Keep},
				"admin_color": {Action: config.Keep},
			},
		},
	}, map[string]config.Environment{"local": approving("usermeta.meta_value")})

	got := anonymize.Effective(cfg, []string{"local"})

	want := []string{
		"local: usermeta.meta_key=admin_color",
		"local: usermeta.meta_key=first_name keep → fake.first_name",
	}
	if !equal(rendered(got), want) {
		t.Errorf("Effective() = %v, want %v", rendered(got), want)
	}
}

// A keyed Approval answers the question a column Approval could not reach. Approving
// `meta_key=admin_color` resolves that key and leaves every other one exactly where it
// was, which is the narrowness the keyed form exists for.
func TestEffectiveResolvesAKeyItsOwnApprovalNames(t *testing.T) {
	cfg := project(map[string]config.Table{
		"usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys: map[string]config.Column{
				"first_name":  {Action: config.Keep},
				"admin_color": {Action: config.Keep},
			},
		},
	}, map[string]config.Environment{"local": approving("usermeta.meta_key=admin_color")})

	got := anonymize.Effective(cfg, []string{"local"})

	want := []string{"local: usermeta.meta_key=first_name keep → fake.first_name"}
	if !equal(rendered(got), want) {
		t.Errorf("Effective() = %v, want %v", rendered(got), want)
	}
}

// Approval is per destination for a key exactly as it is for a column. One destination's
// answer is not the other's.
func TestEffectiveApprovesAKeyAtOneEnvironmentAndNotTheOther(t *testing.T) {
	cfg := project(map[string]config.Table{
		"usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys:          map[string]config.Column{"admin_color": {Action: config.Keep}},
		},
	}, map[string]config.Environment{
		"staging": approving("usermeta.meta_key=admin_color"),
		"local":   {},
	})

	got := anonymize.Effective(cfg, []string{"local", "staging"})

	want := []string{"local: usermeta.meta_key=admin_color"}
	if !equal(rendered(got), want) {
		t.Errorf("Effective() = %v, want %v", rendered(got), want)
	}
}

// A table has one Discriminator, so an Approval naming a different column selects for
// nothing. Resolving it as though it had named the right one would send real values on
// the strength of a line `check` calls malformed — an exposure granted by a typo.
func TestEffectiveIgnoresAnApprovalThatNamesTheWrongDiscriminator(t *testing.T) {
	cfg := project(map[string]config.Table{
		"usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys:          map[string]config.Column{"admin_color": {Action: config.Keep}},
		},
	}, map[string]config.Environment{"local": approving("usermeta.meta_value=admin_color")})

	got := anonymize.Effective(cfg, []string{"local"})

	want := []string{"local: usermeta.meta_key=admin_color"}
	if !equal(rendered(got), want) {
		t.Errorf("Effective() = %v, want the key still unapproved", rendered(got))
	}
}

// A keyed Fallback carries the Discriminator column beside the key, because recording an
// Approval needs both and taking the printed name apart again is a parser for a format
// nobody defined.
func TestEffectiveCarriesTheDiscriminatorBesideTheKey(t *testing.T) {
	cfg := project(map[string]config.Table{
		"usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys:          map[string]config.Column{"admin_color": {Action: config.Keep}},
		},
	}, map[string]config.Environment{"local": {}})

	got := anonymize.Effective(cfg, []string{"local"})

	if len(got) != 1 {
		t.Fatalf("Effective() = %v, want one entry", rendered(got))
	}
	if !got[0].Keyed() || got[0].Discriminator != "meta_key" || got[0].Field != "admin_color" {
		t.Errorf("fallback = %+v, want the discriminator meta_key beside the key admin_color", got[0])
	}
}

// The resolution is derived on every run and never written down, so a column carries
// one `action` and no per-destination shadow of it.
func TestEffectiveWritesNothingIntoTheClassification(t *testing.T) {
	cfg := project(map[string]config.Table{
		"users": columns(map[string]config.Column{
			"display_name": {Action: config.Keep},
		}),
	}, map[string]config.Environment{"local": {}})

	anonymize.Effective(cfg, []string{"local"})

	if action := cfg.Anonymize.Tables["users"].Columns["display_name"].Action; action != config.Keep {
		t.Errorf("action = %q, want keep — the fallback is computed, never stored", action)
	}
}

func TestRefuseNamesTheColumnTheDestinationAndTheThreeWaysOut(t *testing.T) {
	stranded := anonymize.Fallbacks{{Environment: "local", Column: "orders.internal_blob"}}

	r := anonymize.Refuse("local", stranded)

	if r.Reason != refusal.NoFallback {
		t.Errorf("Reason = %q, want %q", r.Reason, refusal.NoFallback)
	}
	for _, want := range []string{
		"orders.internal_blob", "local",
		"environments.local.anonymize.approved",
		"fake.<generator>", "drop",
	} {
		if !strings.Contains(r.Detail, want) {
			t.Errorf("Detail = %q, want it to mention %q", r.Detail, want)
		}
	}
}

// Nothing stranded is not a Refusal. The caller raises it unconditionally and gets nil
// where there was nothing to refuse over.
func TestRefuseIsNilWhenEveryKeepResolved(t *testing.T) {
	if r := anonymize.Refuse("local", nil); r != nil {
		t.Errorf("Refuse() = %v, want nil", r)
	}
}

func TestRefuseListsEveryStrandedColumnInOneMessage(t *testing.T) {
	r := anonymize.Refuse("local", anonymize.Fallbacks{
		{Environment: "local", Column: "orders.internal_blob"},
		{Environment: "local", Column: "orders.ledger_ref"},
	})

	if !strings.Contains(r.Detail, "2 columns") {
		t.Errorf("Detail = %q, want both columns counted", r.Detail)
	}
	if !strings.Contains(r.Detail, "internal_blob") || !strings.Contains(r.Detail, "ledger_ref") {
		t.Errorf("Detail = %q, want both columns named", r.Detail)
	}
}

// For is how a Pull asks: one destination, and the answers for the others are not its
// business.
func TestForNarrowsToOneDestination(t *testing.T) {
	all := anonymize.Fallbacks{
		{Environment: "local", Column: "orders.internal_blob"},
		{Environment: "staging", Column: "orders.internal_blob"},
	}

	if got := all.For("staging"); len(got) != 1 || got[0].Environment != "staging" {
		t.Errorf("For(staging) = %v, want the staging answer alone", got)
	}
}

// An approved prefix `keep` travels real, for every key the prefix answers for.
func TestEffectiveResolvesAnApprovedPrefix(t *testing.T) {
	cfg := project(map[string]config.Table{
		"wp_options": options(map[string]config.Column{"_transient_*": {Action: config.Keep}}),
	}, map[string]config.Environment{"local": approving("wp_options.option_name=_transient_*")})

	if got := anonymize.Effective(cfg, []string{"local"}); len(got) != 0 {
		t.Errorf("Effective() = %v, want nothing — the prefix is approved", rendered(got))
	}
}

// A Generator claims a whole name, and a prefix is not one: nothing claims `email*` the
// way something claims `billing_email`. So an unapproved prefix `keep` refuses.
func TestEffectiveLeavesAnUnapprovedPrefixWithNoFallback(t *testing.T) {
	cfg := project(map[string]config.Table{
		"wp_options": options(map[string]config.Column{
			"_transient_*": {Action: config.Keep},
			"email*":       {Action: config.Keep},
		}),
	}, map[string]config.Environment{"local": {}})

	got := anonymize.Effective(cfg, []string{"local"})

	want := []string{"local: wp_options.option_name=_transient_*", "local: wp_options.option_name=email*"}
	if !equal(rendered(got.NoFallback()), want) {
		t.Errorf("NoFallback() = %v, want %v", rendered(got.NoFallback()), want)
	}
}

// The empty Discriminator value is an entry like any other, spelled `""`, and approved
// by that spelling.
func TestEffectiveResolvesTheEmptyValueByItsSpelling(t *testing.T) {
	cfg := project(map[string]config.Table{
		"wp_options": options(map[string]config.Column{"": {Action: config.Keep}}),
	}, map[string]config.Environment{
		"local":   {},
		"staging": approving(`wp_options.option_name=""`),
	})

	got := anonymize.Effective(cfg, []string{"local", "staging"})

	want := []string{`local: wp_options.option_name=""`}
	if !equal(rendered(got), want) {
		t.Fatalf("Effective() = %v, want %v", rendered(got), want)
	}
	if ref := got[0].Ref(); !cfg.Environments["staging"].ApprovesRef(ref) {
		t.Errorf("Ref() = %v, want the reference staging approves", ref)
	}
}

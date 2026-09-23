package anonymize

import (
	"fmt"
	"strings"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/generator"
	"github.com/bramaos/brama/internal/refusal"
)

// Fallback is what happens to one `keep` column at one Environment that has not
// approved it.
//
// Classification says what a column means; Approval says where its real values may go.
// Where the file keeps a column and the destination approves nothing for it, the two
// answers have to be reconciled into the one thing a Pull can act on — and the
// reconciliation is always downward. See
// docs/adr/0010-classification-and-approval-are-separate-axes.md.
type Fallback struct {
	// Environment is the destination this answer is for. Approval is per destination,
	// so one column resolves two ways at two Environments, and neither is the answer
	// for the other.
	Environment string
	// Column is what to call it in a sentence — `wp_users.display_name`, or
	// `wp_usermeta.meta_key=first_name` for a Discriminator value.
	Column string
	// Table is the table the column belongs to, unqualified, and Field is the column's
	// own name — or the Discriminator value, for a keyed entry.
	//
	// Column reads as a sentence and these two write into a file, which is why both
	// exist — the same split Drift makes, for the same reason. An interactive review
	// records an Approval of `table.column` or amends
	// `anonymize.tables.<table>.columns.<column>`, and taking a printable name apart
	// again to get there is a parser for a format nobody defined.
	Table string
	Field string
	// Discriminator is the column whose value Field is, and empty for an ordinary
	// column. An Approval of a Discriminator value names it — `usermeta.meta_key=admin_color`
	// — so recording one needs it, and reading it back out of Column would be a parser
	// for a format nobody defined.
	//
	// It is also what makes this entry a keyed one, which is why Keyed is derived from
	// it rather than stored beside it: two fields for one fact can disagree, and the one
	// that would go stale here decides whether real values leave production.
	Discriminator string
	// Action is the Classification brama acts on in place of `keep` — always a
	// `fake.<generator>`. Empty means no Generator claims the column and there is
	// nothing to fall back to, which is the Refusal, not a quiet `drop`.
	Action config.Classification
}

// Substituted reports whether a Generator stood in for the `keep`.
func (f Fallback) Substituted() bool { return f.Action != "" }

// Keyed reports whether this is one Discriminator value rather than a column.
func (f Fallback) Keyed() bool { return f.Discriminator != "" }

// Ref is the Approval that would resolve this fallback: `table.column` for a column, and
// `table.discriminator=key` for a Discriminator value.
//
// One place builds it, so one place decides what approving this entry means. Spelling the
// fork out at each call site instead is how a destination ends up approved by a reference
// that names the wrong Discriminator — which resolves nothing, and would send real values
// on the strength of a line `check` calls malformed.
func (f Fallback) Ref() config.ColumnRef {
	if f.Keyed() {
		return config.ColumnRef{Table: f.Table, Column: f.Discriminator, Key: config.SpellKey(f.Field)}
	}
	return config.ColumnRef{Table: f.Table, Column: f.Field}
}

func (f Fallback) String() string {
	if f.Substituted() {
		return f.Environment + ": " + f.Column + " keep → " + string(f.Action)
	}
	return f.Environment + ": " + f.Column
}

// Fallbacks is every `keep` an Environment has not approved, across the Environments
// that were asked about.
type Fallbacks []Fallback

// Substituted is the half a Generator answered for. A Pull carries these out and says
// so; there is nothing here for a person to do.
func (f Fallbacks) Substituted() Fallbacks {
	var out Fallbacks
	for _, fallback := range f {
		if fallback.Substituted() {
			out = append(out, fallback)
		}
	}
	return out
}

// NoFallback is the half nothing answered for. A Pull to any Environment named here
// refuses until a human resolves it, which is what Refuse says.
func (f Fallbacks) NoFallback() Fallbacks {
	var out Fallbacks
	for _, fallback := range f {
		if !fallback.Substituted() {
			out = append(out, fallback)
		}
	}
	return out
}

// For narrows to one Environment, which is what a Pull has: one destination.
func (f Fallbacks) For(environment string) Fallbacks {
	var out Fallbacks
	for _, fallback := range f {
		if fallback.Environment == environment {
			out = append(out, fallback)
		}
	}
	return out
}

// Effective resolves Classification against Approval for each named Environment, and
// returns the columns where the two do not already agree.
//
// It answers only about `keep`, because `keep` is the only Classification an Approval
// can change the meaning of: a `fake.<generator>` fabricates its value wherever it
// lands, and a `drop` sends none. An approved `keep` is likewise absent — it resolves
// to itself, and a list of every column that resolves to what the file already says is
// a list of nothing worth reading.
//
// The answer is computed and never written. A column carries one `action` and never a
// shadow second answer per destination, so adding an Environment changes no line of
// `brama.yaml` and removing one leaves nothing behind.
//
// cfg is expected to have been through Resolve, so the columns a Preset classifies are
// resolved here too. A Preset's `keep` is knowledge and not Approval, and this is where
// that distinction becomes a difference.
func Effective(cfg *config.Config, environments []string) Fallbacks {
	if cfg == nil || cfg.Anonymize == nil {
		return nil
	}

	classified := entries(cfg.Anonymize)
	var out Fallbacks
	for _, name := range environments {
		env, known := cfg.Environments[name]
		if !known {
			continue
		}
		for _, e := range classified {
			if e.column.Action != config.Keep {
				continue
			}
			fallback := Fallback{
				Environment:   name,
				Column:        e.name,
				Table:         e.table,
				Field:         e.field,
				Discriminator: e.discriminator,
			}
			// A Discriminator value is resolved against the Approval that names its key
			// and never against one naming a column. The column holding these holds every
			// other key's value too, so reading `usermeta.meta_value` as an answer about
			// `admin_color` would approve every key at once — which is why Check refuses
			// that form and says to name the key instead.
			if env.ApprovesRef(fallback.Ref()) {
				continue
			}
			// Claimed by name alone. The type half of a claim needs a Schema, and this
			// has to answer on a CI runner with no route to a database — and where a
			// Schema is in reach, a Generator that cannot fit the column it claims is
			// already Cover's to report.
			if g, claimed := generator.ClaimName(e.field); claimed {
				fallback.Action = config.Classification(fakePrefix + g.Name)
			}
			out = append(out, fallback)
		}
	}
	return out
}

// fakePrefix is what a faking Classification carries in front of its Generator name.
// config owns the word; this is the one place outside it that has to write one.
const fakePrefix = "fake."

// Refuse is the Refusal a Pull to this Environment raises for the `keep` columns it
// cannot resolve.
//
// Falling back to `drop` here would be brama choosing between emptying a column and
// preserving it, on its own authority, about data whose meaning it could not determine
// — the same judgement ADR 0012 keeps out of `init`. So it stops, names the columns,
// and gives the three ways out. Two of them widen what is sent and one narrows it, and
// which of those a project wants is exactly the thing brama cannot know.
//
// It returns nil when nothing is stranded, so a caller can raise it unconditionally.
func Refuse(environment string, stranded Fallbacks) *refusal.Refusal {
	if len(stranded) == 0 {
		return nil
	}

	exits := strings.Join([]string{
		fmt.Sprintf("approve it for %s, under environments.%s.anonymize.approved", environment, environment),
		"classify it fake.<generator>, where one fits the column",
		"classify it drop, where the column need not travel at all",
	}, "\n    - ")

	var detail string
	if len(stranded) == 1 {
		detail = fmt.Sprintf(
			"cannot pull to %s — %s is classified keep, %s approves nothing for it, and no generator claims the column, so there is nothing to send instead",
			environment, stranded[0].Column, environment)
	} else {
		columns := make([]string, 0, len(stranded))
		for _, f := range stranded {
			columns = append(columns, f.Column)
		}
		// Always more than one here, so the sentence is written plural outright rather
		// than pluralized — the other branch is the singular, spelled out in full.
		detail = fmt.Sprintf(
			"cannot pull to %s — %d columns classified keep that %s approves nothing for, and no generator claims any of them:\n  - %s",
			environment, len(stranded), environment, strings.Join(columns, "\n  - "))
	}

	return refusal.New(refusal.NoFallback, detail+"\n  choose one:\n    - "+exits, "")
}

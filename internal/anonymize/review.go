package anonymize

import (
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/preset"
)

// Review is one run of `brama anonymize review`, divided the way the model divides it:
// what brama may carry out on its own, and what only a human may decide.
//
// The line between the two lanes is exposure. Narrowing what leaves production needs
// nobody's permission — a Preset tightening is often a security fix, and a column a
// migration added is classified by what a Generator declares it claims. Widening it
// needs a person, every time: a Preset loosening and an unapproved `keep` both send real
// production data somewhere, and no amount of shipped knowledge is authorization to.
// See ADR 0010 and ADR 0011.
//
// The two are reported side by side and never merged. "brama did this" and "brama is
// refusing to do this until you say so" are opposite instructions to whoever is reading.
type Review struct {
	// Tightenings are the Preset answers stricter than the file's. They are applied
	// already — Resolve applies them in memory on every run — and writing them down is
	// what stops brama.yaml saying `keep` about a column brama fabricates.
	Tightenings preset.Drifts
	// New are the columns the Schema has, the file did not, and a Generator claims.
	// Same lane as a tightening: nothing is widened, and it lands in `git diff`.
	New []config.TableClassification
	// Held are the Preset answers looser than the file's, which never apply on their
	// own.
	Held preset.Drifts
	// Unapproved are the `keep` columns a destination approves nothing for, one entry
	// per destination: approved for staging is not approved for local, so the same
	// column is two decisions where it lands in two places.
	//
	// All of them, and not only the ones with no Generator to fall back on. `check` is
	// right to call a substitution the model working — a Pull runs, and sends a
	// fabricated value. `review` asks a different question: "do you approve exposing
	// this column as real data?", which is unanswered whether or not brama has
	// something to send in the meantime. A run that listed only the columns with no
	// fallback would quietly decide, by omission, every exposure a Generator happens to
	// be covering for. See ADR 0010.
	Unapproved Fallbacks
	// Unclassified are the columns the Schema has that nothing classifies and no
	// Generator claims. They are in neither lane. Deciding what a column means when
	// nothing claims it is a judgement, so this cannot be automatic; and it is not a
	// `keep` anybody has to approve either, so answering it is the interactive
	// review's, not this one's.
	Unclassified []Uncovered
}

// Plan divides one project's state into the two lanes.
//
// resolved is the config with its Preset read in, and drift the disagreement Resolve
// reported on the way. coverage is the comparison against a Schema, and nil where no
// Environment was reachable — which leaves New and Unclassified empty rather than
// guessing that a database brama could not read has nothing new in it.
func Plan(resolved *config.Config, drift preset.Drifts, environments []string, coverage *Coverage) Review {
	review := Review{
		Tightenings: drift.Applied(),
		Held:        drift.Held(),
		Unapproved:  Effective(resolved, environments),
	}
	if coverage != nil {
		review.New = Bootstrap(*coverage)
		review.Unclassified = Unclaimed(*coverage, review.New)
	}
	return review
}

// Pending reports whether anything is left for a human — the exit condition, and the
// only question a CI runner has to ask.
//
// Unclassified columns are not among them. A pull refuses on one either way, and that is
// reported; what makes these two pending is that `review` is the command that answers
// them and this run could not.
func (r Review) Pending() bool { return len(r.Held) > 0 || len(r.Unapproved) > 0 }

// Amendments is the automatic half as config writes it, in the order it is applied:
// what a Preset changed about columns the file already answers for, then what a
// migration added.
func (r Review) Amendments() []config.Amendment {
	out := make([]config.Amendment, 0, len(r.Tightenings)+Claimed(r.New))
	for _, d := range r.Tightenings {
		out = append(out, config.Amendment{
			Table:  d.Table,
			Column: d.Column,
			Key:    d.Key,
			Action: d.Shipped,
			// The whole of the Preset's answer, not the action out of it. A column
			// written back into agreement about what it fabricates and out of the group
			// it fabricates it with would break the joins the group exists to keep.
			Correlate: d.Correlate,
		})
	}
	for _, t := range r.New {
		for _, c := range t.Columns {
			out = append(out, config.Amendment{
				Table:    t.Name,
				Column:   c.Name,
				Action:   c.Action,
				Declared: c.Declared,
			})
		}
	}
	return out
}

// Automatic is how many columns this run wrote for. A tightening and a new column are
// one line of the file each, and they are counted together because the person reading
// is about to read one diff, not two.
func (r Review) Automatic() int { return len(r.Tightenings) + Claimed(r.New) }

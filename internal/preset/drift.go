package preset

import "github.com/bramaos/brama/internal/config"

// Drift is one column the Preset and the recorded Classification disagree about.
//
// A Preset is referenced and never expanded, so upgrading brama can change what one says
// about a column brama.yaml already answers for. The two directions are not equally safe,
// and the stricter of the two wins: a Preset moving a column off `keep` takes effect on
// the next Pull, a Preset moving one onto `keep` is held until a human passes it through
// `brama anonymize review`. The recorded Classification is itself the baseline, which is
// why this needs no version, no hash and no history.
// See docs/adr/0011-the-stricter-of-preset-and-record-wins.md.
//
// Only a disagreement about exposure is Drift. A Preset saying `fake.full_name` where the
// file says `fake.first_name` widens nothing and narrows nothing, so it is the ordinary
// per-column override and is not reported here.
type Drift struct {
	// Name is the column it is about, spelled the way the rest of brama spells one —
	// `wp_users.user_email`, or `wp_usermeta.meta_key=first_name` for a Discriminator
	// value.
	Name string
	// Table is the table the disagreement is about.
	Table string
	// Column is the ordinary column it is about, and is empty when Key is set.
	Column string
	// Key is the Discriminator value it is about, and is empty when Column is set.
	//
	// Name reads as a sentence and these three write into a file, which is why both
	// exist. `brama anonymize review` has to find the entry a Drift refers to under
	// `anonymize.tables.<table>.columns.<column>` or `.keys.<key>`, and taking a
	// printable name apart again to get there is a parser for a format nobody defined.
	Key string
	// Recorded is what brama.yaml says about the column.
	Recorded config.Classification
	// Shipped is what the Preset says about it.
	Shipped config.Classification
	// Correlate is the Correlation group the Preset's answer belongs to, and is empty
	// where it belongs to none.
	//
	// It travels with the answer because it is part of it. A tightening applied without
	// it drops the column out of its group, and a group of one correlates nothing — so
	// writing the action alone would turn a Preset improvement into a file `check`
	// refuses.
	Correlate string
	// Applied is whether the Preset's answer is the one brama acts on. A tightening is
	// applied and needs nothing from anybody; a loosening is held, and the recorded
	// answer stands until a human accepts the change.
	Applied bool
}

// String is the disagreement in one line — `wp_users.user_email: keep → fake.email`.
func (d Drift) String() string {
	return d.Name + ": " + string(d.Recorded) + " → " + string(d.Shipped)
}

// Drifts is every column one Preset and one file disagree about.
type Drifts []Drift

// Applied is the Drift brama has already acted on — the columns the Preset classifies
// more strictly than the file does.
func (d Drifts) Applied() Drifts {
	var out Drifts
	for _, drift := range d {
		if drift.Applied {
			out = append(out, drift)
		}
	}
	return out
}

// Held is the Drift brama has declined to act on, and a human still has to decide about.
//
// It is kept apart from Applied everywhere it is reported. "brama is already doing this"
// and "brama is refusing to do this until you say so" are opposite instructions to
// whoever is reading, and one list of column names would have them act on the wrong half.
func (d Drifts) Held() Drifts {
	var out Drifts
	for _, drift := range d {
		if !drift.Applied {
			out = append(out, drift)
		}
	}
	return out
}

// Tightened reports whether the Preset overrode what the file records for this column.
//
// It is how the rest of brama tells a Classification the project wrote from one an
// upgrade substituted for it — which is the difference between a contradiction in the
// file and a consequence of brama's own decision, and those are not reported the same
// way.
func (d Drifts) Tightened(column string) bool {
	for _, drift := range d {
		if drift.Applied && drift.Name == column {
			return true
		}
	}
	return false
}

// exposes reports whether a Classification sends the real value.
//
// Exposure is the whole axis the comparison runs on. `keep` sends what production holds;
// `fake.<generator>` sends something fabricated and `drop` sends nothing, and neither can
// leak what the other would not. So `keep` is the loose answer and the other two are the
// strict ones, with no ordering between them to invent.
func exposes(c config.Classification) bool { return c == config.Keep }

// drifted says which answer brama acts on for one column, and what to report about it.
//
// The record is the baseline. Where the Preset is stricter it wins outright: that is a
// Preset improvement — often a security fix — reaching a project that has not been
// edited, and holding it for review would leave it inert in every project until somebody
// thought to look. Where the Preset is looser the record stands: widening what leaves
// production is a decision only a human makes.
//
// Where the two expose the same thing the record wins, silently. That is the deliberate
// per-column override, and reporting it every run would bury the two cases that matter.
// at carries the identity of the column — everything about the Drift that is true
// before the two answers are compared.
func drifted(at Drift, recorded, shipped config.Column) (config.Column, *Drift) {
	if exposes(recorded.Action) == exposes(shipped.Action) {
		return recorded, nil
	}
	d := at
	d.Recorded = recorded.Action
	d.Shipped = shipped.Action
	d.Correlate = shipped.Correlate
	d.Applied = exposes(recorded.Action)
	if d.Applied {
		return shipped, &d
	}
	return recorded, &d
}

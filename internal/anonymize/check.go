// Package anonymize checks the Classification a project commits to brama.yaml.
//
// What lives here is everything that can be settled without reading a database:
// whether every `fake.<generator>` names a Generator brama has, whether a `correlate`
// sits beside a Classification it means anything on, whether a Correlation group has
// anyone to correlate with, and whether an Environment approves columns the file
// actually classifies. That split is the point — `brama anonymize check` has to mean
// something on a CI runner with no route to production, which is what ADR 0013 closes
// the Generator vocabulary for. The half that needs a Schema is Cover, in coverage.go,
// and a run that could not reach one says so rather than reporting a clean bill of
// health it did not earn.
//
// It reports every problem it finds rather than the first. Someone fixing a
// classification is reading the whole block anyway, and one problem per run is how a
// ten-minute edit becomes ten CI runs.
package anonymize

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/generator"
	"github.com/bramaos/brama/internal/preset"
)

// Problem is one way the committed Classification contradicts itself.
//
// It names the key it is about the way brama.yaml spells it, rather than a line
// number: the file is being scrolled anyway, and a path survives the reformatting
// that would move the line.
type Problem struct {
	// At is the key the problem is about — `anonymize.tables.users.columns.email`.
	At string
	// Detail says what is wrong with it, and what to write instead.
	Detail string
}

func (p Problem) String() string { return p.At + ": " + p.Detail }

// Summary is what the file classifies. It is the Result's half of a run: the counts a
// person reads to tell "this checked my project" from "this checked an empty block".
type Summary struct {
	// Tables is how many tables the file classifies.
	Tables int
	// Columns is how many columns and Discriminator values it classifies between them.
	Columns int
	// Groups is how many Correlation groups those columns name.
	Groups int
}

// Check reports what the file classifies, and every way that classification
// contradicts itself.
//
// environments names the Environments whose Approvals are checked — every one of them
// by default, one when `--env` narrows it. Approval is the only part of the model that
// differs between destinations, so it is the only part this takes a name for.
//
// cfg is expected to have been through config.Validate and then Resolve: this answers
// the questions config cannot, because they need the Generator vocabulary and a view of
// the whole file at once, and it does not repeat the ones it can. Resolve is what makes
// the columns a Preset classifies visible here — unresolved, they are columns this
// would report as classified by nobody.
//
// drift is what Resolve reported about the same file, and it is here for one reason: a
// Classification an upgrade substituted for the recorded one is not something the project
// wrote, and an Approval left standing beside it is not a contradiction the project can
// be asked to fix. See checkApprovals.
//
// present is the tables the Schema actually has, and nil where no Schema was in reach.
// It narrows the Summary and nothing else: a Preset classifies the twelve tables its
// framework creates, and counting the ones this database does not have would report
// twelve tables classified on a project that has three. Problems are found against the
// whole file either way — a contradiction in a table nobody has is still a
// contradiction, and it is one somebody wrote down.
func Check(cfg *config.Config, environments []string, drift preset.Drifts, present []string) (Summary, []Problem) {
	var problems []Problem
	add := func(at, detail string) {
		problems = append(problems, Problem{At: at, Detail: detail})
	}

	classified := entries(cfg.Anonymize)
	groups := map[string][]string{}
	// reported holds the columns whose `correlate` has already been spoken for, so one
	// wrong line is one problem. A `correlate` beside a `keep` is usually also the only
	// member of its group, and saying so twice reads as two edits to make.
	reported := map[string]bool{}

	for _, e := range classified {
		if e.keyed {
			if detail, malformed := malformedKey(e.field); malformed {
				add(e.at, detail)
			}
		}

		if name, fakes := e.column.Action.Generator(); fakes {
			// An unknown name is refused here and nowhere later. `fake.e_mail` is a
			// typo in a committed file, and the safe reading of a typo is that nobody
			// decided what this column should hold — not that anything string-shaped
			// will do halfway through a dump on a production Server.
			if _, err := generator.Lookup(name); err != nil {
				add(e.at+".action", err.Error())
			}
		}

		if e.column.Correlate == "" {
			continue
		}
		switch e.column.Action {
		case config.Keep:
			reported[e.at] = true
			add(e.at+".correlate", fmt.Sprintf(
				"correlate says nothing beside keep — real values already correlate; remove it, "+
					"or classify %s as fake.<generator>", e.name))
		case config.Drop:
			reported[e.at] = true
			add(e.at+".correlate", fmt.Sprintf(
				"correlate says nothing beside drop — a dropped column has no value to relate; "+
					"remove it, or classify %s as fake.<generator>", e.name))
		}
		// A member that will be reported for its own Classification still counts
		// towards the group. Dropping it would make every column it correlates with a
		// group of one, and one mistaken line would print as three problems.
		groups[e.column.Correlate] = append(groups[e.column.Correlate], e.at)
	}

	// A group of one is the typo guard. `correlate: custmer` is otherwise a group
	// nobody else is in, which fabricates exactly what it would have without the
	// line — silently, and only for the column that misspelled it.
	for _, name := range slices.Sorted(maps.Keys(groups)) {
		members := groups[name]
		if len(members) > 1 || reported[members[0]] {
			continue
		}
		detail := fmt.Sprintf("%q is a correlation group of one, and a group of one correlates nothing", name)
		if near, ok := nearest(name, groups); ok {
			detail += fmt.Sprintf(" — did you mean %q?", near)
		} else {
			detail += " — name the same group on the column it shares an identity with, or remove it"
		}
		add(members[0]+".correlate", detail)
	}

	problems = append(problems, checkApprovals(cfg, environments, drift)...)
	return summarize(cfg.Anonymize, present), problems
}

// checkApprovals reports Approvals that cannot mean what they say.
//
// An Approval is a human's decision to send real values somewhere, so the one thing it
// must never be is inert. Approving a column the file classifies `fake.email` reads,
// in a diff, exactly like approving one it classifies `keep` — and does nothing.
func checkApprovals(cfg *config.Config, environments []string, drift preset.Drifts) []Problem {
	var problems []Problem
	for _, name := range environments {
		env, known := cfg.Environments[name]
		if !known || env.Anonymize == nil {
			continue
		}
		for i, ref := range env.Anonymize.Approved {
			at := fmt.Sprintf("environments.%s.anonymize.approved[%d]", name, i)
			// An empty reference is config.Validate's to report, and it has.
			if ref.Table == "" || ref.Column == "" {
				continue
			}

			column, classified := classificationOf(cfg.Anonymize, ref)
			discriminator := discriminatorOf(cfg.Anonymize, ref.Table)
			switch {
			case classified && column.Action != config.Keep && drift.Tightened(ref.String()):
				// The file classifies this `keep` and approves it, and a Preset brama
				// tightened since has overridden the classification. The approval is
				// inert, and saying so as a Refusal would blame a pair of lines that
				// agreed with each other when they were written — and would stop a Pull
				// over a tightening that is meant to resolve on its own. It is reported
				// as the Drift it is instead.
			case classified && column.Action != config.Keep:
				problems = append(problems, Problem{At: at, Detail: fmt.Sprintf(
					"%s is classified %s, and approval only means something beside keep — "+
						"an approved fake column is still fabricated", ref, column.Action)})
			case !classified && ref.Keyed() && discriminator == "":
				problems = append(problems, Problem{At: at, Detail: fmt.Sprintf(
					"%s names a key, and %s has no discriminator — nothing in it is classified "+
						"per key, so there is no key here to approve", ref, ref.Table)})
			case !classified && ref.Keyed() && discriminator != ref.Column:
				problems = append(problems, Problem{At: at, Detail: fmt.Sprintf(
					"%s names %s as the discriminator, and %s's is %s — write %s.%s=%s",
					ref, ref.Column, ref.Table, discriminator, ref.Table, discriminator, ref.Key)})
			case !classified && ref.Keyed():
				detail := fmt.Sprintf("%s is not a key this file classifies — approval names what to "+
					"send as real data", ref)
				// Approval addresses the entry, not the keys under it, so a key a prefix
				// covers is answered by approving the prefix, and by nothing narrower.
				if entry, covered := cfg.Anonymize.Tables[ref.Table].Match(ref.Entry()); covered {
					detail += fmt.Sprintf(", and %s is classified by %s — write %s.%s=%s",
						ref.Key, entry, ref.Table, ref.Column, entry)
				} else {
					detail += fmt.Sprintf(", and nothing here says what %s holds", ref.Key)
				}
				problems = append(problems, Problem{At: at, Detail: detail})
			case !classified && discriminated(cfg.Anonymize, ref):
				problems = append(problems, Problem{At: at, Detail: fmt.Sprintf(
					"%s is classified per key under keys, so approving the column says nothing "+
						"about which keys it covers — name the key instead, as in %s.%s=<key>",
					ref, ref.Table, discriminator)})
			case !classified:
				// Reached only after Resolve, so a Preset has already answered for
				// every column it knows. A column still missing here is one nothing
				// classifies, and approving it decides nothing at all.
				problems = append(problems, Problem{At: at, Detail: fmt.Sprintf(
					"%s is not a column this file classifies — approval names what to send as "+
						"real data, and nothing here says what this column holds", ref)})
			}
		}
	}
	return problems
}

// malformedKey says why a `keys` entry cannot mean what it looks like it means.
func malformedKey(key string) (string, bool) {
	// Two quote characters are how the file spells the empty value, so an entry that
	// really is them could not be told apart from it in a path or an Approval.
	if key == config.EmptyKey {
		return "a key of two quote characters reads as the empty value, " + config.EmptyKey +
			", in every path and approval — write \"\" for rows with no value", true
	}
	// A `*` only ends a key. Anywhere else it reads as a glob brama does not match,
	// and the entry would quietly classify one literal key nobody writes.
	i := strings.Index(key, "*")
	switch {
	case i < 0 || i == len(key)-1:
		return "", false
	case i == 0:
		// Cutting `*_email` at its star leaves `*`, which classifies every key there
		// will ever be. That is not a fix to offer for a typo.
		return "a * only ends a key, where it matches every key starting with what comes " +
			"before it — name each key exactly", true
	}
	return fmt.Sprintf("a * only ends a key, where it matches every key starting with what "+
		"comes before it — write %s for that prefix, or name each key exactly", key[:i+1]), true
}

// entry is one classified thing: an ordinary column, or one Discriminator value.
type entry struct {
	// at is the key it was written under.
	at string
	// name is what to call it in a sentence — `users.email`.
	name string
	// table is the table it belongs to, and field is the column name, or the
	// Discriminator value for a keyed entry. The two are what an Approval is matched
	// against and what a Generator is asked to claim, neither of which can be read
	// back out of name.
	table string
	field string
	// discriminator is the column whose value field is, for a keyed entry, and empty
	// otherwise. An Approval of a Discriminator value names it, so what records one
	// needs it carried rather than read back out of name.
	discriminator string
	// value is the column the discriminator selects for, for a keyed entry, and empty
	// otherwise. Writing a keyed entry into a table the file does not classify yet has
	// to write the table's own two facts with it, and they are facts about the table
	// rather than about this key.
	value string
	// keyed says the entry is one Discriminator value rather than a column. It is
	// approved by a `table.column=key` reference and never a `table.column` one: the
	// column a bare reference would name holds every key's value at once, which is why
	// approving that is still refused.
	keyed  bool
	column config.Column
}

// entries walks the classification in the order the file's keys sort, so two runs over
// the same file report the same problems in the same order.
func entries(a *config.Anonymize) []entry {
	if a == nil {
		return nil
	}
	var out []entry
	for _, table := range slices.Sorted(maps.Keys(a.Tables)) {
		at := "anonymize.tables." + table
		t := a.Tables[table]
		for _, key := range slices.Sorted(maps.Keys(t.Keys)) {
			out = append(out, entry{
				at:            at + ".keys." + config.SpellKey(key),
				name:          table + "." + t.Discriminator + "=" + config.SpellKey(key),
				table:         table,
				field:         key,
				discriminator: t.Discriminator,
				value:         t.Value,
				keyed:         true,
				column:        t.Keys[key],
			})
		}
		for _, column := range slices.Sorted(maps.Keys(t.Columns)) {
			out = append(out, entry{
				at:     at + ".columns." + column,
				name:   table + "." + column,
				table:  table,
				field:  column,
				column: t.Columns[column],
			})
		}
	}
	return out
}

// classificationOf returns what the file says about one column or one Discriminator
// value, and whether it says anything at all.
//
// A keyed reference is only answered by the table's own Discriminator. One naming a
// different column selects for nothing, and reading it as though it named the right one
// would approve a key on the strength of a line that does not say so.
func classificationOf(a *config.Anonymize, ref config.ColumnRef) (config.Column, bool) {
	if a == nil {
		return config.Column{}, false
	}
	table := a.Tables[ref.Table]
	if ref.Keyed() {
		// A table with no Discriminator fails this too: a reference always names a
		// column, so the empty string never matches one.
		if table.Discriminator != ref.Column {
			return config.Column{}, false
		}
		column, ok := table.Keys[ref.Entry()]
		return column, ok
	}
	column, ok := table.Columns[ref.Column]
	return column, ok
}

// discriminatorOf is the column whose value selects a Classification for a row of this
// table, and empty for a table that classifies no keys.
func discriminatorOf(a *config.Anonymize, table string) string {
	if a == nil {
		return ""
	}
	return a.Tables[table].Discriminator
}

// discriminated reports whether a reference names the structural half of a key/value
// table — the Discriminator, or the column whose value it selects for.
func discriminated(a *config.Anonymize, ref config.ColumnRef) bool {
	if a == nil {
		return false
	}
	table := a.Tables[ref.Table]
	return table.Discriminator == ref.Column || (table.Value != "" && table.Value == ref.Column)
}

// summarize counts what the file classifies, over the tables a Schema says are there.
//
// present nil is "no Schema was read", and everything counts — a run that could not
// reach a database has nothing to say about which tables exist, and silently counting
// none of them would report a file that classifies nothing.
//
// The Correlation groups are counted here rather than taken from the map Check built,
// because that map is every group the file names and this is a count of the ones the
// database has a table for. The two answer different questions on the same entries.
func summarize(a *config.Anonymize, present []string) Summary {
	if a == nil {
		return Summary{}
	}

	has := func(string) bool { return true }
	if present != nil {
		known := make(map[string]bool, len(present))
		for _, table := range present {
			known[table] = true
		}
		has = func(table string) bool { return known[table] }
	}

	var summary Summary
	groups := map[string]bool{}
	for table := range a.Tables {
		if has(table) {
			summary.Tables++
		}
	}
	for _, e := range entries(a) {
		if !has(e.table) {
			continue
		}
		summary.Columns++
		if e.column.Correlate != "" {
			groups[e.column.Correlate] = true
		}
	}
	summary.Groups = len(groups)
	return summary
}

// nearest returns the group a lone member was most likely meant to join.
//
// The suggestion is the whole value of refusing a group of one: the name is right there
// in the file and looks deliberate, and what a reader needs is the other spelling of it
// on the screen at the same time.
func nearest(name string, groups map[string][]string) (string, bool) {
	// Short names have to match more closely. Two edits away from `eu` is most of the
	// two-letter names there are, and a suggestion that confident about nothing is
	// worse than no suggestion.
	limit := 2
	if len(name) <= 4 {
		limit = 1
	}

	best, closest := "", limit+1
	for _, other := range slices.Sorted(maps.Keys(groups)) {
		if other == name {
			continue
		}
		if d := distance(name, other); d < closest {
			best, closest = other, d
		}
	}
	return best, best != ""
}

// distance is the Levenshtein distance between two group names, over bytes rather than
// runes. Group names are written by hand beside ASCII column names; the worst a
// multi-byte one costs is a suggestion not offered, never a wrong one.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			substitution := prev[j-1]
			if a[i-1] != b[j-1] {
				substitution++
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, substitution)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

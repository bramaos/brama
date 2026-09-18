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

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/generator"
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
// cfg is expected to have been through config.Validate: this answers the questions that
// package cannot, because they need the Generator vocabulary and a view of the whole
// file at once, and it does not repeat the ones it can.
func Check(cfg *config.Config, environments []string) (Summary, []Problem) {
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

	problems = append(problems, checkApprovals(cfg, environments)...)
	return summarize(cfg.Anonymize, groups), problems
}

// checkApprovals reports Approvals that cannot mean what they say.
//
// An Approval is a human's decision to send real values somewhere, so the one thing it
// must never be is inert. Approving a column the file classifies `fake.email` reads,
// in a diff, exactly like approving one it classifies `keep` — and does nothing.
func checkApprovals(cfg *config.Config, environments []string) []Problem {
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
			switch {
			case classified && column.Action != config.Keep:
				problems = append(problems, Problem{At: at, Detail: fmt.Sprintf(
					"%s is classified %s, and approval only means something beside keep — "+
						"an approved fake column is still fabricated", ref, column.Action)})
			case !classified && discriminated(cfg.Anonymize, ref):
				problems = append(problems, Problem{At: at, Detail: fmt.Sprintf(
					"%s is classified per key under keys, so approving the column says nothing "+
						"about which keys it covers", ref)})
			case !classified && cfg.Anonymize.Preset == "":
				// With a Preset named, the column may well be one the Preset
				// classifies. Presets are referenced and not expanded here, so the
				// honest answer is silence rather than a refusal brama cannot support.
				problems = append(problems, Problem{At: at, Detail: fmt.Sprintf(
					"%s is not a column this file classifies — approval names what to send as "+
						"real data, and nothing here says what this column holds", ref)})
			}
		}
	}
	return problems
}

// entry is one classified thing: an ordinary column, or one Discriminator value.
type entry struct {
	// at is the key it was written under.
	at string
	// name is what to call it in a sentence — `users.email`.
	name   string
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
				at:     at + ".keys." + key,
				name:   table + "." + t.Discriminator + "=" + key,
				column: t.Keys[key],
			})
		}
		for _, column := range slices.Sorted(maps.Keys(t.Columns)) {
			out = append(out, entry{
				at:     at + ".columns." + column,
				name:   table + "." + column,
				column: t.Columns[column],
			})
		}
	}
	return out
}

// classificationOf returns what the file says about one column, and whether it says
// anything at all.
func classificationOf(a *config.Anonymize, ref config.ColumnRef) (config.Column, bool) {
	if a == nil {
		return config.Column{}, false
	}
	column, ok := a.Tables[ref.Table].Columns[ref.Column]
	return column, ok
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

func summarize(a *config.Anonymize, groups map[string][]string) Summary {
	if a == nil {
		return Summary{}
	}
	return Summary{Tables: len(a.Tables), Columns: len(entries(a)), Groups: len(groups)}
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

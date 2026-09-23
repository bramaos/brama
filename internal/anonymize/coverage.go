package anonymize

import (
	"fmt"
	"strings"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/generator"
	"github.com/bramaos/brama/internal/schema"
)

// Coverage is how far the committed Classification goes towards answering for a
// Schema: how many columns there are to decide about, and which ones nobody has.
//
// It is the half of `check` that needs a database, and the reason the split exists.
// Check reads the file against itself and can run on a CI runner; only a Schema can
// say that `users` has a column called `internal_note` at all.
type Coverage struct {
	// Columns is how many columns of the Schema a Classification has to answer for.
	// Generated columns are not among them: the database computes those, nothing may
	// write to them, and no Classification could be carried out on one.
	Columns int
	// Unclassified are the columns the Schema has and the file says nothing about, in
	// the order the Schema declares them. Each carries its Schema entry rather than
	// its name alone, because the caller that acts on this — `anonymize init` — picks
	// a Generator by name *and* type, and a name on its own cannot be claimed.
	Unclassified []Uncovered
	// Keys is how many distinct Discriminator values the Schema holds for the tables
	// the file classifies per key. It is zero where none were read.
	Keys int
	// UnclassifiedKeys are the Discriminator values no `keys` entry matches, table by
	// table in Schema order and in byte order within one. Every one is listed: brama
	// does not group them or guess a prefix for them, because a `*` is a decision about
	// every key a plugin will ever write, and that is a human's to make.
	UnclassifiedKeys []UncoveredKey
}

// Complete reports whether every column of the Schema, and every Discriminator value
// read with it, is answered for.
func (c Coverage) Complete() bool { return len(c.Unclassified) == 0 && len(c.UnclassifiedKeys) == 0 }

// Uncovered is one column the Schema has and the Classification does not.
type Uncovered struct {
	// Table is the table it belongs to, unqualified, the way the Schema names it.
	Table string
	// Column is the Schema's own entry — type, length, nullability and all.
	Column schema.Column
}

func (u Uncovered) String() string { return u.Table + "." + u.Column.Name }

// UncoveredKey is one Discriminator value the Schema holds and the Classification does
// not.
type UncoveredKey struct {
	Table         string
	Discriminator string
	// Value is the key as the database holds it, byte for byte. "" is a row with no
	// key, NULL or empty.
	Value string
	// Selects is the column the key selects a value in — the table's `value` — as the
	// Schema has it, and zero where the Schema does not. A Generator claiming the key
	// has to fill it.
	Selects schema.Column
}

// Nameable reports whether the file can name this key by itself. A `*` in a `keys` entry
// makes it a prefix, or a file `check` refuses, so a key holding one would be written as a
// classification of every key it prefixes, or not written at all. A person decides it in
// the file.
func (k UncoveredKey) Nameable() bool { return !strings.Contains(k.Value, "*") }

// String is the key the way a Refusal names it: `wp_usermeta.meta_key='stripe_customer_id'`.
// The empty value is spelled as the file spells its entry, `wp_usermeta.meta_key=""`, so
// what the Refusal names is what a person writes.
func (k UncoveredKey) String() string {
	if k.Value == "" {
		return k.Table + "." + k.Discriminator + "=" + config.EmptyKey
	}
	return k.Table + "." + k.Discriminator + "='" + k.Value + "'"
}

// Cover compares a Schema against the committed Classification.
//
// It answers the two questions a file cannot answer about itself: which columns the
// database has that nobody classified, and whether a Generator the file names could
// actually fill the column it was given. The second is the refusal ADR 0013 promises —
// `fake.email` on a `varchar(20)` stops in the editor, rather than truncating or
// erroring partway through a dump on a production Server. Where the Schema carries a
// Discriminator's values, it answers the first question for them too: which keys the
// data holds that no `keys` entry matches.
//
// It is deliberately separate from Check, and returns the comparison as data rather
// than only as problems, because `anonymize init` walks the same ground: what init
// writes a Classification for is exactly this Unclassified list.
//
// a is the resolved Classification — what the file says with the Preset it names read
// in, from Resolve. A Preset answers for columns brama.yaml never mentions, and passing
// the unresolved file here would report every one of them as Unclassified.
//
// a may be nil, which classifies nothing and leaves every column of the Schema
// uncovered.
func Cover(a *config.Anonymize, s schema.Schema) (Coverage, []Problem) {
	if a == nil {
		a = &config.Anonymize{}
	}

	var coverage Coverage
	var problems []Problem

	for _, t := range s.Tables {
		classified := a.Tables[t.Name]
		coverKeys(&coverage, classified, t)
		for _, col := range t.Columns {
			// A generated column cannot be written to at all, so its absence from the
			// file is not a decision anybody failed to make.
			if col.Generated {
				continue
			}
			coverage.Columns++

			// A key/value table classifies its value column one Discriminator value at
			// a time, so neither that column nor the Discriminator selecting for it is
			// answered for by a `columns` entry. Its keys are, by coverKeys.
			if structural(classified, col.Name) {
				continue
			}

			column, ok := classified.Columns[col.Name]
			if !ok {
				coverage.Unclassified = append(coverage.Unclassified, Uncovered{Table: t.Name, Column: col})
				continue
			}

			name, fakes := column.Action.Generator()
			if !fakes {
				// `keep` writes the real value and `drop` writes none, so neither has
				// a fabricated value that has to fit anywhere.
				continue
			}
			g, err := generator.Lookup(name)
			if err != nil {
				// An unknown name is Check's to report, and it has. Saying it again
				// here reads as two edits to make.
				continue
			}
			if err := g.Fits(col); err != nil {
				problems = append(problems, Problem{
					At:     fmt.Sprintf("anonymize.tables.%s.columns.%s.action", t.Name, col.Name),
					Detail: err.Error(),
				})
			}
		}
	}

	return coverage, problems
}

// coverKeys counts the Discriminator values read for one table, and adds every one no
// `keys` entry matches to the coverage.
//
// Values read off a column the file does not call the table's Discriminator are not
// keys of it. The file's Discriminator is what its `keys` entries are about, and
// matching another column's values against them would answer a question nobody asked.
func coverKeys(coverage *Coverage, classified config.Table, t schema.Table) {
	if classified.Discriminator == "" || t.Discriminator.Column != classified.Discriminator {
		return
	}
	selects, _ := t.Column(classified.Value)
	for _, value := range t.Discriminator.Values {
		coverage.Keys++
		if _, ok := classified.Match(value); !ok {
			coverage.UnclassifiedKeys = append(coverage.UnclassifiedKeys, UncoveredKey{
				Table: t.Name, Discriminator: classified.Discriminator, Value: value, Selects: selects,
			})
		}
	}
}

// structural reports whether a column is the Discriminator of a key/value table, or
// the column whose value it selects for.
func structural(t config.Table, column string) bool {
	if t.Discriminator == "" {
		return false
	}
	return t.Discriminator == column || t.Value == column
}

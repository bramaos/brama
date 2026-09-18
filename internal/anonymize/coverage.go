package anonymize

import (
	"fmt"

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
	// Unexpanded names the Preset holding the part of the answer brama could not
	// read, and is empty when the file names none. While it is set, Unclassified is
	// not the whole story and Complete says nothing.
	Unexpanded string
}

// Complete reports whether every column of the Schema is answered for.
func (c Coverage) Complete() bool { return len(c.Unclassified) == 0 }

// Uncovered is one column the Schema has and the Classification does not.
type Uncovered struct {
	// Table is the table it belongs to, unqualified, the way the Schema names it.
	Table string
	// Column is the Schema's own entry — type, length, nullability and all.
	Column schema.Column
}

func (u Uncovered) String() string { return u.Table + "." + u.Column.Name }

// Cover compares a Schema against the committed Classification.
//
// It answers the two questions a file cannot answer about itself: which columns the
// database has that nobody classified, and whether a Generator the file names could
// actually fill the column it was given. The second is the refusal ADR 0013 promises —
// `fake.email` on a `varchar(20)` stops in the editor, rather than truncating or
// erroring partway through a dump on a production Server.
//
// It is deliberately separate from Check, and returns the comparison as data rather
// than only as problems, because `anonymize init` walks the same ground: what init
// writes a Classification for is exactly this Unclassified list.
//
// a may be nil, which classifies nothing and leaves every column of the Schema
// uncovered.
func Cover(a *config.Anonymize, s schema.Schema) (Coverage, []Problem) {
	if a == nil {
		a = &config.Anonymize{}
	}

	coverage := Coverage{Unexpanded: a.Preset}
	var problems []Problem

	for _, t := range s.Tables {
		classified := a.Tables[t.Name]
		for _, col := range t.Columns {
			// A generated column cannot be written to at all, so its absence from the
			// file is not a decision anybody failed to make.
			if col.Generated {
				continue
			}
			coverage.Columns++

			// A key/value table classifies its value column one Discriminator value at
			// a time, so neither that column nor the Discriminator selecting for it is
			// answered for by a `columns` entry. Whether every key present in the data
			// has one is a question about rows, and a Schema holds none.
			if structural(classified, col.Name) {
				continue
			}

			column, ok := classified.Columns[col.Name]
			if !ok {
				// With a Preset named, this is most likely a column the Preset
				// classifies. Presets are referenced and never expanded here (ADR
				// 0011), so brama cannot tell those from the genuinely unanswered —
				// and the honest answer is the silence checkApprovals keeps for the
				// same reason, said out loud through Unexpanded.
				if coverage.Unexpanded == "" {
					coverage.Unclassified = append(coverage.Unclassified, Uncovered{Table: t.Name, Column: col})
				}
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

// structural reports whether a column is the Discriminator of a key/value table, or
// the column whose value it selects for.
func structural(t config.Table, column string) bool {
	if t.Discriminator == "" {
		return false
	}
	return t.Discriminator == column || t.Value == column
}

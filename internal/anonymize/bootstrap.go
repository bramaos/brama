package anonymize

import (
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/generator"
)

// Bootstrap is the Classification `brama anonymize init` would write for a set of
// columns nobody has classified yet.
//
// It takes Coverage's Unclassified list rather than a Schema, so that init and check
// walk the same ground: what check reports as unanswered is exactly what init offers
// an answer for. Passing Cover a nil Classification makes that list the whole schema,
// which is the state init runs in.
//
// There are two outcomes per column and no third (ADR 0012). A Generator declaring a
// claim on the column — its name and its type both — is written as `fake.<generator>`;
// everything else is left out of the result entirely, which leaves it Unclassified, a
// state that already has a defined consequence. Nothing is defaulted to `drop`: that is
// safe about privacy and reckless about everything else, zeroing `orders.total_amount`
// on brama's authority about data whose meaning brama could not determine.
//
// `keep` never appears here. It is a decision to send real production data, and it
// enters the file only where a human put it.
//
// Tables come out in the order their columns did, and carry only the columns something
// claimed. A table nothing claimed is absent rather than empty: an entry classifying
// nothing reads like a decision in the diff and is none.
func Bootstrap(c Coverage) []config.TableClassification {
	var tables []config.TableClassification
	at := map[string]int{}

	for _, u := range c.Unclassified {
		g, claimed := generator.Claim(u.Column)
		if !claimed {
			continue
		}

		i, seen := at[u.Table]
		if !seen {
			i = len(tables)
			at[u.Table] = i
			tables = append(tables, config.TableClassification{Name: u.Table})
		}
		tables[i].Columns = append(tables[i].Columns, config.ColumnClassification{
			Name:     u.Column.Name,
			Action:   config.Classification("fake." + g.Name),
			Declared: u.Column.Declared,
		})
	}

	return tables
}

// Unclaimed is what the Schema has that neither the file nor a Generator answers for:
// the Coverage a Bootstrap left behind.
//
// It is derived from what was written rather than recomputed from the Generators, so
// the two halves of a report — what brama classified, and what it left for a person —
// can never disagree about which columns those are.
func Unclaimed(c Coverage, classified []config.TableClassification) []Uncovered {
	written := make(map[string]bool, len(c.Unclassified))
	for _, t := range classified {
		for _, col := range t.Columns {
			written[t.Name+"."+col.Name] = true
		}
	}

	var left []Uncovered
	for _, u := range c.Unclassified {
		if !written[u.String()] {
			left = append(left, u)
		}
	}
	return left
}

// Claimed counts the columns a Classification was written for.
func Claimed(tables []config.TableClassification) int {
	var n int
	for _, t := range tables {
		n += len(t.Columns)
	}
	return n
}

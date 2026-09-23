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
// A Discriminator value is classified exactly as a column is. Its name is the key, and
// the type a Generator has to fill is the column the key selects a value in, so
// `billing_email` over a longtext `meta_value` is `fake.email`, and a key nothing claims
// is left out. Entries are exact keys: brama never writes a `*` pattern, which is a
// person's judgement about keys nobody has seen yet.
//
// Tables come out in the order their columns did, and carry only the columns and keys
// something claimed. A table nothing claimed is absent rather than empty: an entry
// classifying nothing reads like a decision in the diff and is none.
func Bootstrap(c Coverage) []config.TableClassification {
	var tables []config.TableClassification
	at := map[string]int{}
	table := func(name string) *config.TableClassification {
		i, seen := at[name]
		if !seen {
			i = len(tables)
			at[name] = i
			tables = append(tables, config.TableClassification{Name: name})
		}
		return &tables[i]
	}

	for _, u := range c.Unclassified {
		g, claimed := generator.Claim(u.Column)
		if !claimed {
			continue
		}
		t := table(u.Table)
		t.Columns = append(t.Columns, config.ColumnClassification{
			Name:     u.Column.Name,
			Action:   config.Classification("fake." + g.Name),
			Declared: u.Column.Declared,
		})
	}

	for _, k := range c.UnclassifiedKeys {
		g, claimed := claimKey(k)
		if !claimed {
			continue
		}
		t := table(k.Table)
		t.Discriminator, t.Value = k.Discriminator, k.Selects.Name
		t.Keys = append(t.Keys, config.ColumnClassification{
			Name:   k.Value,
			Action: config.Classification("fake." + g.Name),
		})
	}

	return tables
}

// claimKey is Claim for a Discriminator value: the key's name, and the column it
// selects a value in. A Schema without that column has nothing a Generator could fill,
// and a key the file cannot name has no entry to be written in.
func claimKey(k UncoveredKey) (generator.Generator, bool) {
	if k.Selects.Name == "" || !k.Nameable() {
		return generator.Generator{}, false
	}
	g, ok := generator.ClaimName(k.Value)
	if !ok || g.Fits(k.Selects) != nil {
		return generator.Generator{}, false
	}
	return g, true
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

// UnclaimedKeys is Unclaimed's counterpart for Discriminator values: the keys the
// Schema holds that neither the file nor a Generator answers for.
func UnclaimedKeys(c Coverage, classified []config.TableClassification) []UncoveredKey {
	written := map[[2]string]bool{}
	for _, t := range classified {
		for _, k := range t.Keys {
			written[[2]string{t.Name, k.Name}] = true
		}
	}

	var left []UncoveredKey
	for _, k := range c.UnclassifiedKeys {
		if !written[[2]string{k.Table, k.Value}] {
			left = append(left, k)
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

// ClaimedKeys counts the Discriminator values a Classification was written for.
func ClaimedKeys(tables []config.TableClassification) int {
	var n int
	for _, t := range tables {
		n += len(t.Keys)
	}
	return n
}

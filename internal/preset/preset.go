// Package preset holds the Classification brama ships for the frameworks it knows.
//
// A Preset is referenced by name and never expanded into brama.yaml. `anonymize.tables`
// then holds only what is specific to a project plus its deliberate overrides, the file
// stays short enough to review, and a Preset brama tightens reaches every existing
// project without anyone editing anything.
// See docs/adr/0011-the-stricter-of-preset-and-record-wins.md.
//
// A Preset names its tables, and the Discriminator keys that carry the prefix too, after
// the project's own table prefix, which its Adapter reads out of the project's config:
// `wp_` is what the WordPress installer writes and not what
// `$table_prefix` means, and a Preset matched against the wrong prefix would classify
// whatever table sorted into place. See Lookup, and
// docs/adr/0014-a-preset-is-named-for-the-projects-table-prefix.md.
//
// A Preset supplies knowledge and never authorization. It can say that the users table's
// `display_name` holds a public-facing name and still authorize no Environment
// to receive it: a Preset's `keep` is not pre-approved by virtue of being omitted from
// the file, because Presets are omitted for being reusable knowledge and not for being
// trusted. Approval lives on the Environment, and only a human grants one.
// See docs/adr/0010-classification-and-approval-are-separate-axes.md.
//
// The set is closed and it lives in brama rather than in Adapter code, for the reason
// ADR 0013 closes the Generator vocabulary: `brama anonymize check` has to mean
// something on a CI runner with no route to a database and no framework to ask. A
// Preset is named after the Adapter whose tables it knows, so `app.adapter: wordpress`
// and `anonymize.preset: wordpress` are the same word twice on purpose.
package preset

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/bramaos/brama/internal/config"
)

// Preset is the Classification an Adapter ships for the tables it already knows.
//
// It is held in the shape brama.yaml would have written it, rather than in a form of
// its own, so that a Preset answer and a project's answer are the same kind of thing
// and the merge between them is an override rather than a translation.
type Preset struct {
	// Name is what `anonymize.preset` names, and it is the Adapter's own name.
	Name string
	// Tables is the Classification, table by table.
	Tables map[string]config.Table
}

// presets is the set brama ships. Like the Generator vocabulary it is a package-level
// value with no way to append to it: a registry a caller could pass its own slice to
// would be a closed set written down and then left unenforced.
var presets = []Preset{wordpress}

// prefixMark is what a Preset writes where the project's own table prefix goes, in a
// table's name and in a Discriminator key alike.
//
// A Preset knows which tables a framework creates and what is in them; it does not know
// what they are called, because the name is half the framework's and half the project's.
// WordPress spells the project's half `$table_prefix`, and `wp_` is only its most common
// value. Some Discriminator values carry it too — `wp_capabilities` is `$table_prefix` +
// `capabilities`, written into `usermeta` — and they are the project's name for the same
// reason a table is. See Lookup.
const prefixMark = "{prefix}"

// Lookup returns the Preset shipped under this name, named for this project.
//
// prefix is what the project's own tables are prefixed with, read out of the project's
// config by its Adapter. It is required of every caller rather than defaulted here: a
// Preset applied under the wrong prefix does not classify nothing — it classifies
// whatever table sorted into the accounts table's place, which is worse than matching
// nothing at all.
//
// An unknown name is an error and never an empty Preset. `preset: wordpres` resolving
// to nothing would leave every column the Preset was carrying Unclassified, which reads
// in the output as a project that classified nothing rather than as a typo.
func Lookup(name, prefix string) (Preset, error) {
	p, ships := shipped(name)
	if !ships {
		return Preset{}, fmt.Errorf("no preset named %q — brama ships: %s", name, strings.Join(Names(), ", "))
	}
	return p.named(prefix)
}

// For returns the Preset brama ships for an Adapter, whether it ships one, and whether
// the prefix given could name its tables.
//
// This is `anonymize init`'s question rather than `check`'s: the file being
// bootstrapped names no Preset yet, and what decides whether it gets one is which
// framework the project already declared.
//
// The two negative answers are separate because they mean opposite things to the caller.
// No Preset is an ordinary run — init classifies from the Generators alone. A Preset
// that cannot be named is a run that must stop, because carrying on would write a file
// whose preset line covers tables this database does not have.
func For(adapter, prefix string) (Preset, bool, error) {
	p, ships := shipped(adapter)
	if !ships {
		return Preset{}, false, nil
	}
	named, err := p.named(prefix)
	return named, true, err
}

// NeedsPrefix reports whether the Preset shipped under this name is written against a
// prefix the project decides, and so cannot be resolved until one is read.
//
// It is asked before Lookup, by the caller that has to go and find the prefix, so that a
// project whose Preset names its tables outright is never sent looking through config
// files for an answer nothing needs.
func NeedsPrefix(name string) bool {
	p, ships := shipped(name)
	return ships && p.needsPrefix()
}

// shipped is the one scan of the set brama ships. Every way in asks it the same
// question, so a Preset cannot be found by one caller and missed by another.
func shipped(name string) (Preset, bool) {
	for _, p := range presets {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

func (p Preset) needsPrefix() bool {
	for name, t := range p.Tables {
		if strings.Contains(name, prefixMark) {
			return true
		}
		for key := range t.Keys {
			if strings.Contains(key, prefixMark) {
				return true
			}
		}
	}
	return false
}

// named resolves the prefixMark in every table name and every Discriminator key against
// this project's prefix.
//
// Only those two carry it. A column name is the framework's alone — WordPress names
// `meta_key` the same on every install — so a `{prefix}` written among the columns is a
// mistake in the Preset and is left to read as one.
func (p Preset) named(prefix string) (Preset, error) {
	if prefix == "" && p.needsPrefix() {
		return Preset{}, fmt.Errorf("the %s preset is written against this project's table prefix, "+
			"and none was determined", p.Name)
	}

	out := Preset{Name: p.Name, Tables: make(map[string]config.Table, len(p.Tables))}
	for name, t := range clone(p.Tables) {
		t.Keys = namedKeys(t.Keys, prefix)
		out.Tables[strings.ReplaceAll(name, prefixMark, prefix)] = t
	}
	return out, nil
}

// namedKeys resolves the prefixMark in a table's Discriminator keys.
func namedKeys(keys map[string]config.Column, prefix string) map[string]config.Column {
	if keys == nil {
		return nil
	}
	out := make(map[string]config.Column, len(keys))
	for key, col := range keys {
		out[strings.ReplaceAll(key, prefixMark, prefix)] = col
	}
	return out
}

// Names lists the Presets brama ships, sorted, for help text and error messages.
func Names() []string {
	names := make([]string, 0, len(presets))
	for _, p := range presets {
		names = append(names, p.Name)
	}
	slices.Sort(names)
	return names
}

// Apply returns the Classification a file means once this Preset is read in: the
// Preset's answers, overridden column by column by whatever the file itself says — and
// every column where the two disagree about exposure, which is the Drift.
//
// The override is per column and not per table. A project that wants real values in
// `wp_users.display_name` writes that one column, and does not thereby take
// responsibility for every other column of `wp_users` — which is the whole reason the
// Preset is referenced rather than expanded.
//
// What the file cannot do by writing a column is loosen the Preset. Where the two
// disagree the stricter answer wins, so a Preset moving a column off `keep` applies here
// and a Preset moving one onto `keep` is reported and not applied. See Drift.
//
// Nothing here writes back. The merge is what brama acts on; brama.yaml still holds
// only the project's own half, and the file is not touched by having been read.
//
// a may be nil, which is the state `anonymize init` runs in: the Preset on its own, and
// no record to drift from.
func (p Preset) Apply(a *config.Anonymize) (*config.Anonymize, Drifts) {
	out := &config.Anonymize{Preset: p.Name, Tables: clone(p.Tables)}
	if a == nil {
		return out, nil
	}

	// Sorted, here and below, so that two runs over one file report the same Drift in
	// the same order.
	var drift Drifts
	for _, name := range slices.Sorted(maps.Keys(a.Tables)) {
		over := a.Tables[name]
		shipped, known := out.Tables[name]
		if !known {
			// A table the Preset says nothing about is the project's alone — a plugin's
			// table, or one the application added. It carries over as written, and a
			// table the Preset never classified cannot have drifted from it.
			out.Tables[name] = over
			continue
		}
		merged, drifted := override(name, shipped, over)
		out.Tables[name] = merged
		drift = append(drift, drifted...)
	}
	return out, drift
}

// override merges one table's project answers over the Preset's.
//
// An empty field in the file is not an answer. Writing one column of a discriminated
// table must not silently unset the Discriminator the Preset named, because a table
// that names keys and no Discriminator classifies nothing at all.
func override(table string, shipped, over config.Table) (config.Table, Drifts) {
	if over.Discriminator != "" {
		shipped.Discriminator = over.Discriminator
	}
	if over.Value != "" {
		shipped.Value = over.Value
	}

	keys, keyDrift := mergeColumns(shipped.Keys, over.Keys, func(key string) Drift {
		return Drift{Name: table + "." + shipped.Discriminator + "=" + key, Table: table, Key: key}
	})
	columns, columnDrift := mergeColumns(shipped.Columns, over.Columns, func(column string) Drift {
		return Drift{Name: table + "." + column, Table: table, Column: column}
	})
	shipped.Keys, shipped.Columns = keys, columns
	return shipped, append(keyDrift, columnDrift...)
}

// mergeColumns merges one map of project answers over the Preset's. at says where one of
// them lives and what to call it in a sentence, which is the only thing that differs
// between a table's keys and its ordinary columns.
func mergeColumns(shipped, over map[string]config.Column, at func(string) Drift) (map[string]config.Column, Drifts) {
	if len(over) == 0 {
		return shipped, nil
	}
	out := maps.Clone(shipped)
	if out == nil {
		out = make(map[string]config.Column, len(over))
	}

	var drift Drifts
	for _, column := range slices.Sorted(maps.Keys(over)) {
		recorded := over[column]
		ships, known := shipped[column]
		if !known {
			// A column the Preset says nothing about — a plugin's key in usermeta — is
			// one more decision and not a disagreement with one.
			out[column] = recorded
			continue
		}
		merged, d := drifted(at(column), recorded, ships)
		out[column] = merged
		if d != nil {
			drift = append(drift, *d)
		}
	}
	return out, drift
}

// clone copies a Preset's tables down to their column maps.
//
// A shallow copy would hand every caller a writable alias of what brama ships, so one
// project's override would become every project's Preset for the life of the process.
func clone(tables map[string]config.Table) map[string]config.Table {
	out := make(map[string]config.Table, len(tables))
	for name, t := range tables {
		t.Keys = maps.Clone(t.Keys)
		t.Columns = maps.Clone(t.Columns)
		out[name] = t
	}
	return out
}

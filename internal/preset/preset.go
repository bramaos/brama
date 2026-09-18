// Package preset holds the Classification brama ships for the frameworks it knows.
//
// A Preset is referenced by name and never expanded into brama.yaml. `anonymize.tables`
// then holds only what is specific to a project plus its deliberate overrides, the file
// stays short enough to review, and a Preset brama tightens reaches every existing
// project without anyone editing anything.
// See docs/adr/0011-the-stricter-of-preset-and-record-wins.md.
//
// A Preset supplies knowledge and never authorization. It can say that
// `wp_users.display_name` holds a public-facing name and still authorize no Environment
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

// Lookup returns the Preset shipped under this name.
//
// An unknown name is an error and never an empty Preset. `preset: wordpres` resolving
// to nothing would leave every column the Preset was carrying Unclassified, which reads
// in the output as a project that classified nothing rather than as a typo.
func Lookup(name string) (Preset, error) {
	for _, p := range presets {
		if p.Name == name {
			return Preset{Name: p.Name, Tables: clone(p.Tables)}, nil
		}
	}
	return Preset{}, fmt.Errorf("no preset named %q — brama ships: %s", name, strings.Join(Names(), ", "))
}

// For returns the Preset brama ships for an Adapter, and whether it ships one.
//
// This is `anonymize init`'s question rather than `check`'s: the file being
// bootstrapped names no Preset yet, and what decides whether it gets one is which
// framework the project already declared.
func For(adapter string) (Preset, bool) {
	p, err := Lookup(adapter)
	return p, err == nil
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

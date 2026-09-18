package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

// Classification is the recorded decision of what happens to a column's values during
// Anonymization — written as a column's `action`. There are exactly three answers, and
// there is no fourth meaning "not sure": a column with no Classification is
// Unclassified, and Unclassified refuses the Pull.
//
// `mask` is not among them, and is refused by name. Partial preservation derives its
// output from the real value, which is the pseudonymization ADR 0002 exists to prevent.
type Classification string

const (
	// Keep transfers the value as-is — at a destination that approves it. See Approved.
	Keep Classification = "keep"
	// Drop transfers the value as empty or null.
	Drop Classification = "drop"
)

// fakePrefix is what a faking Classification carries in front of its Generator name.
//
// `fake` alone is not a Classification. Which Generator fabricates the value is part
// of the decision and not a detail to settle later: an address faked as a phone number
// fails to import, and `fake` with the choice left open is how that ships.
// See docs/adr/0013-brama-owns-the-generator-vocabulary.md.
const fakePrefix = "fake."

// generatorName is the shape of a Generator's name, not the list of them. Brama owns
// the vocabulary and `anonymize check` is where an unknown name is caught — it holds
// the Generators, and this package does not. What is refused here is a name no
// Generator could ever have, so that `fake.Email` fails at the file rather than
// halfway through a Pull.
var generatorName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Generator returns the Generator this Classification fabricates with, and whether it
// fabricates at all. `keep` and `drop` name no Generator.
func (c Classification) Generator() (string, bool) {
	name, fakes := strings.CutPrefix(string(c), fakePrefix)
	if !fakes {
		return "", false
	}
	return name, true
}

// Valid reports whether c is a Classification, and says why when it is not. The
// explanation is separate from the check because the three ways to get this wrong want
// three different sentences: `fake` is the old shorthand, `fake.Email` is a name no
// Generator has, and `mask` is not an answer at all.
func (c Classification) Valid() error {
	switch c {
	case Keep, Drop:
		return nil
	case "fake":
		return fmt.Errorf("%q does not say what to fabricate — name the generator, as in fake.email", string(c))
	}
	name, fakes := c.Generator()
	switch {
	case !fakes:
		return fmt.Errorf("%q is not an action — must be fake.<generator>, keep, or drop", string(c))
	case name == "":
		return fmt.Errorf("%q does not say what to fabricate — name the generator, as in fake.email", string(c))
	case !generatorName.MatchString(name):
		return fmt.Errorf("%q is not a generator name — lowercase letters, digits and underscores, as in fake.full_name", name)
	}
	return nil
}

// Anonymize is the Classification for this project: what its columns mean, project-wide
// and for every Environment alike. It says nothing about who may receive real values —
// that is Approval, and it lives on the Environment.
//
// It is written by `brama anonymize init`, reviewed in the diff like any other
// committed decision, and amended by `brama anonymize review`.
// See docs/adr/0010-classification-and-approval-are-separate-axes.md.
type Anonymize struct {
	// Preset names the Classification an Adapter ships for tables it already knows.
	// It is referenced, never expanded: `tables` holds what is specific to this
	// project and the deliberate overrides.
	Preset string `yaml:"preset,omitempty"`
	// Tables holds what the Preset does not cover.
	Tables map[string]Table `yaml:"tables,omitempty"`
}

// Table is the Classification for one table.
//
// Most tables classify per column:
//
//	users:
//	  columns:
//	    email:
//	      action: fake.email
//	      correlate: customer
//
// Key/value tables also name a Discriminator — the column whose value selects which
// Classification applies to the row — and the column holding the value it selects for,
// and classify per key. They have ordinary columns too, and `columns` describes those
// in the same breath:
//
//	usermeta:
//	  discriminator: meta_key
//	  value: meta_value
//	  keys:
//	    billing_phone:
//	      action: fake.phone
//	  columns:
//	    umeta_id:
//	      action: keep
//
// The two halves are not exclusive. A Table that could describe its keys or its columns
// but never both has nothing to say about `usermeta.umeta_id`, which leaves a column
// Unclassified for want of a place to write it down.
type Table struct {
	Discriminator string            `yaml:"discriminator,omitempty"`
	Value         string            `yaml:"value,omitempty"`
	Keys          map[string]Column `yaml:"keys,omitempty"`
	Columns       map[string]Column `yaml:"columns,omitempty"`
}

// Column is what the file says about one column, or about one Discriminator value.
//
// It is an object with no scalar shorthand. `email: fake.email` is a parse error rather
// than sugar for this: shorthand is what turns into the next migration problem, and a
// column already carries a second axis that a bare string has nowhere to put.
type Column struct {
	// Action is what happens to the values — the column's Classification.
	Action Classification `yaml:"action"`
	// Correlate names the identity mapping this column shares, so a real value
	// occurring in every member becomes the same fabricated value in all of them and
	// the joins between them survive. Meaningful only alongside `fake.*`; that, and
	// whether the group has more than one member, is settled by `anonymize check`.
	Correlate string `yaml:"correlate,omitempty"`
}

// UnmarshalYAML decodes a column, and answers the two ways of writing one wrong in the
// words of the way that is right.
//
// The decoder would raise "string was used where mapping is expected" for the scalar
// form and "unknown field" for an `approved` written beside an action. Both are true and
// neither says what to write instead, which is the whole of what a reader needs here.
func (c *Column) UnmarshalYAML(node ast.Node) error {
	mapping, ok := node.(ast.MapNode)
	if !ok {
		return fmt.Errorf("%s: a column is an object — write `action: %s` under it, not `%s`",
			yamlPath(node), node.String(), node.String())
	}

	// `approved` beside an action is the one unknown key worth a sentence of its own,
	// because it is the mistake the model is shaped to prevent: one project-wide answer
	// cannot mean different things at different destinations.
	for entries := mapping.MapRange(); entries.Next(); {
		if entries.Key().String() == "approved" {
			return fmt.Errorf("%s: approval is not a column's to give — list it under environments.<name>.anonymize.approved instead",
				yamlPath(node))
		}
	}

	// The named type sheds this method, so decoding the mapping does not re-enter it.
	// Strict is passed back in: an unknown key under a column is as much a typo as one
	// at the top of the file, and a custom unmarshaler is otherwise where strictness
	// would quietly stop.
	type plain Column
	var out plain
	if err := yaml.NodeToValue(node, &out, yaml.Strict()); err != nil {
		return fmt.Errorf("%s: %w", yamlPath(node), err)
	}
	*c = Column(out)
	return nil
}

// ColumnRef names one column of one table — `users.display_name`. It is how an
// Environment refers to a Classification that lives somewhere else, which is the whole
// point: the reference is per destination, the Classification is not.
type ColumnRef struct {
	Table  string
	Column string
}

func (r ColumnRef) String() string {
	return r.Table + "." + r.Column
}

// UnmarshalYAML reads the `table.column` form, and only that form.
//
// A bare column name is refused rather than matched loosely: `display_name` would
// approve real values in every table that happens to have one, which is a far larger
// decision than the one being written down.
func (r *ColumnRef) UnmarshalYAML(node ast.Node) error {
	var ref string
	if err := yaml.NodeToValue(node, &ref, yaml.Strict()); err != nil {
		return fmt.Errorf("%s: an approval is a table.column reference: %w", yamlPath(node), err)
	}

	table, column, found := strings.Cut(ref, ".")
	if !found || table == "" || column == "" || strings.Contains(column, ".") {
		return fmt.Errorf("%s: %q is not a table.column reference — name both, as in users.display_name",
			yamlPath(node), ref)
	}
	r.Table, r.Column = table, column
	return nil
}

// EnvironmentAnonymize is an Environment's half of the model. It holds Approval and
// nothing else: an Environment is a destination, and the only question a destination
// answers is which real values it may receive.
type EnvironmentAnonymize struct {
	// Approved lists the `keep` columns this Environment may receive as real data.
	// Only a human grants one. An absent block and an empty list mean the same thing —
	// nothing is approved — which is why neither is an error.
	Approved []ColumnRef `yaml:"approved,omitempty"`
}

// Approves reports whether this Environment may receive the real values of a column.
//
// An Environment that says nothing approves nothing. That is what makes adding one safe
// by default: a `keep` it has not approved falls back to what a Generator would have
// given the column, so a Pull produces less exposure than the file suggests, never more.
func (e Environment) Approves(table, column string) bool {
	if e.Anonymize == nil {
		return false
	}
	want := ColumnRef{Table: table, Column: column}
	for _, ref := range e.Anonymize.Approved {
		if ref == want {
			return true
		}
	}
	return false
}

// yamlPath is the node's location spelled the way the file spells it —
// `anonymize.tables.users.columns.email` — with the AST's `$.` root dropped. It names
// the offending key more usefully than a line number in a file the reader is scrolling
// anyway, and it survives the reformatting that would move that line.
func yamlPath(node ast.Node) string {
	return strings.TrimPrefix(node.GetPath(), "$.")
}

// validateAnonymize reports every way the classification model contradicts itself.
//
// What it does not check is anything needing knowledge this package lacks: whether a
// Generator exists under that name, whether a correlation group has more than one
// member, whether an approved column is one the Schema has. Those belong to
// `anonymize check`, which holds the Generators, the Preset and the Schema.
func (c *Config) validateAnonymize(add func(string, ...any)) {
	// An empty list entry — `- ` with nothing after it — never reaches ColumnRef's
	// unmarshaler: the decoder reads a null as the zero value and calls nothing. So the
	// one malformed reference the parse cannot catch is caught here, and an Approval
	// that approves an unnamed column in an unnamed table stays unwritable.
	for _, name := range sortedKeys(c.Environments) {
		env := c.Environments[name]
		if env.Anonymize == nil {
			continue
		}
		for i, ref := range env.Anonymize.Approved {
			if ref.Table == "" || ref.Column == "" {
				add("environments.%s.anonymize.approved[%d] is empty — an approval is a table.column reference, as in users.display_name",
					name, i)
			}
		}
	}

	if c.Anonymize == nil {
		return
	}
	for _, name := range sortedKeys(c.Anonymize.Tables) {
		table := c.Anonymize.Tables[name]
		at := "anonymize.tables." + name

		// The three discriminated keys are one decision written in three places, so a
		// Table carrying any of them has to carry all of them. Two out of three
		// classifies nothing, and does it silently.
		discriminated := table.Discriminator != "" || table.Value != "" || len(table.Keys) > 0
		if discriminated {
			if table.Discriminator == "" {
				add("%s.discriminator is required, because the table classifies by key", at)
			}
			if table.Value == "" {
				add("%s.value is required — the column holding the value the discriminator selects for", at)
			}
			if len(table.Keys) == 0 {
				add("%s.keys is required, because the table names a discriminator", at)
			}
		}

		for _, key := range sortedKeys(table.Keys) {
			validateColumn(add, at+".keys."+key, table.Keys[key])
		}
		for _, column := range sortedKeys(table.Columns) {
			validateColumn(add, at+".columns."+column, table.Columns[column])
		}
	}
}

func validateColumn(add func(string, ...any), at string, col Column) {
	if col.Action == "" {
		add("%s.action is required — one of fake.<generator>, keep, or drop", at)
		return
	}
	if err := col.Action.Valid(); err != nil {
		add("%s.action: %s", at, err)
	}
}

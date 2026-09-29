package config

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

// Classification is the recorded decision of what happens to a column's values during
// Anonymization — written as a column's `action`. There are exactly three answers, and
// there is no fourth meaning "not sure": a column with no Classification is
// Unclassified, and Unclassified refuses the Pull.
//
// `mask` is not among them, and is refused by name rather than as an unknown word: it
// derives its output from the real value, which is the pseudonymization ADR 0002 exists
// to prevent.
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

// Validate says why c is not a Classification, and nil when it is. The ways to get this
// wrong want different sentences: `mask` is a word this project refuses outright, `fake`
// is a decision left half-made, and `fake.Email` is a name no Generator could have.
func (c Classification) Validate() error {
	switch c {
	case Keep, Drop:
		return nil
	case "mask":
		return errors.New(
			`"mask" is not an action — masking derives its output from the real value, ` +
				"which is the pseudonymization brama does not do; use fake.<generator>, keep, or drop")
	}

	// Bare `fake` names no Generator, so it falls through to the same sentence as
	// `fake.` — the problem is identical, and the old shorthand deserves no special
	// treatment beyond being told what it is missing.
	name, fakes := c.Generator()
	if !fakes && c != "fake" {
		return fmt.Errorf("%q is not an action — must be fake.<generator>, keep, or drop", string(c))
	}
	if name == "" {
		return fmt.Errorf("%q does not say what to fabricate — name the generator, as in fake.email", string(c))
	}
	if !generatorName.MatchString(name) {
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
	Discriminator string `yaml:"discriminator,omitempty"`
	Value         string `yaml:"value,omitempty"`
	// Keys classifies Discriminator values, each entry by its exact name, by a name
	// holding NumberMark — `wp_{n}_capabilities` — or by a prefix ending in `*` —
	// `_transient_*`. Match says which entry answers for a value.
	Keys    map[string]Column `yaml:"keys,omitempty"`
	Columns map[string]Column `yaml:"columns,omitempty"`
}

// Match returns the `keys` entry that classifies one Discriminator value, as written, and
// whether any does.
//
// The entry naming the value exactly wins, so `_transient_doing_cron` can say something of
// its own under `_transient_*`. After it comes the entry holding NumberMark that matches
// with the most literal characters, and after that the longest prefix covering the value.
// Without prefixes every transient hash WordPress invents would be Unclassified and refuse
// the next Pull, and without NumberMark every site's copy of a key would. A numbered entry
// is literal everywhere but one bounded run of digits, so it outranks every open-ended
// prefix: ranked below, `wp_*` would shadow `wp_{n}_capabilities`. See
// docs/adr/0017-discriminator-values-are-read-from-production.md and
// docs/adr/0018-a-discriminator-value-may-hold-a-number-placeholder.md.
//
// Values are compared as bytes. A row with no value, NULL or empty, is the empty value,
// and is matched like any other.
func (t Table) Match(value string) (string, bool) {
	if _, ok := t.Keys[value]; ok && !strings.Contains(value, NumberMark) {
		return value, true
	}
	if key, ok := t.matchNumbered(value); ok {
		return key, true
	}
	// Two prefixes of one value that are the same length are the same prefix, so the
	// longest is never a tie and map order cannot pick between entries.
	best, longest := "", -1
	for key := range t.Keys {
		prefix, ok := pattern(key)
		if ok && !strings.Contains(key, NumberMark) && len(prefix) > longest && strings.HasPrefix(value, prefix) {
			best, longest = key, len(prefix)
		}
	}
	return best, longest >= 0
}

// matchNumbered returns the entry holding NumberMark that matches value with the most
// literal characters. Between two that match with as many, the one naming the rest of
// the value in full beats the one ending in `*`, and after that the order of the keys
// decides, so map order never does.
func (t Table) matchNumbered(value string) (string, bool) {
	best, longest, bestStar := "", -1, false
	for key := range t.Keys {
		before, after, ok := strings.Cut(key, NumberMark)
		if !ok {
			continue
		}
		after, star := pattern(after)
		if !matchNumber(value, before, after, star) {
			continue
		}
		n := len(before) + len(after)
		if n != longest {
			if n > longest {
				best, longest, bestStar = key, n, star
			}
			continue
		}
		if bestStar != star {
			if !star {
				best, bestStar = key, star
			}
			continue
		}
		if key < best {
			best = key
		}
	}
	return best, longest >= 0
}

// matchNumber reports whether value is before, one run of digits, then after — or, when
// star is set, anything starting with after. Every length of the run is tried, so a digit
// that after begins with is not swallowed by the run: `wp_10_x` matches `wp_{n}0_x`.
func matchNumber(value, before, after string, star bool) bool {
	rest, ok := strings.CutPrefix(value, before)
	if !ok {
		return false
	}
	for i := 0; i < len(rest) && isDigit(rest[i]); i++ {
		tail := rest[i+1:]
		if tail == after || star && strings.HasPrefix(tail, after) {
			return true
		}
	}
	return false
}

// pattern reports whether a `keys` entry is a prefix — `_transient_*` — and the prefix it
// matches: everything before the closing `*`. A `*` before that is taken literally here,
// and `anonymize check` refuses the entry.
func pattern(key string) (string, bool) {
	return strings.CutSuffix(key, "*")
}

// NumberMark is the number placeholder: in a `keys` entry it matches one run of one or
// more digits, and is never substituted. `wp_{n}_capabilities` answers for
// `wp_2_capabilities` and `wp_403_capabilities`, and not for `wp_admin_capabilities`.
const NumberMark = "{n}"

// placeholders returns every placeholder in a `keys` entry: a brace, a name, a brace. A
// name starts with a letter and holds letters, digits and underscores; braces around
// anything else — `{}`, `{"a":1}`, `{0}` — are literal text a key may hold.
func placeholders(key string) []string {
	var found []string
	for {
		i := strings.IndexByte(key, '{')
		if i < 0 {
			return found
		}
		key = key[i:]
		j := strings.IndexByte(key, '}')
		if j < 0 {
			return found
		}
		if name := key[1:j]; placeholderName(name) {
			found = append(found, key[:j+1])
			key = key[j+1:]
			continue
		}
		key = key[1:]
	}
}

// placeholderName reports whether what stands between two braces names a placeholder:
// a letter, then letters, digits and underscores.
func placeholderName(name string) bool {
	if name == "" || !isLetter(name[0]) {
		return false
	}
	for i := range len(name) {
		if c := name[i]; !isLetter(c) && !isDigit(c) && c != '_' {
			return false
		}
	}
	return true
}

func isLetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

// Literal reports whether a `keys` entry spelled as key would name key and nothing else:
// it holds no `*` and no placeholder. A Discriminator value that fails this cannot be
// written into the file as itself.
func Literal(key string) bool {
	return !strings.Contains(key, "*") && len(placeholders(key)) == 0
}

// EmptyKey is how the file spells the empty Discriminator value, in a `keys` entry's
// path and in an Approval: `usermeta.meta_key=""`. Written bare, a reference ending in
// `=` names nothing, and a path ending in `keys.` reads as a typo.
const EmptyKey = `""`

// SpellKey is a `keys` entry the way the file spells it: the entry itself, or EmptyKey for
// the empty Discriminator value.
func SpellKey(key string) string {
	if key == "" {
		return EmptyKey
	}
	return key
}

// Column is what the file says about one column, or about one Discriminator value.
//
// It is an object with no scalar shorthand. `email: fake.email` is a parse error rather
// than sugar for this: shorthand is what turns into the next migration problem, and a
// column already carries a second axis that a bare string has nowhere to put.
type Column struct {
	// Action is what happens to the values — the column's Classification.
	Action Classification `yaml:"action"`
	// Correlate names the Correlation group this column belongs to, so a real value
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
		// Only a scalar is quoted back. A sequence written here would drag its whole
		// block into the message, and "write `action: - keep`" is worse than silence.
		if scalar, ok := node.(ast.ScalarNode); ok {
			written := fmt.Sprint(scalar.GetValue())
			return fmt.Errorf("%s: a column is an object — write `action: %s` under it, not `%s`",
				yamlPath(node), written, written)
		}
		return fmt.Errorf("%s: a column is an object — write `action:`, and optionally `correlate:`, under it",
			yamlPath(node))
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

// ColumnRef names one classified thing in one table — `users.display_name`, or the one
// Discriminator value `usermeta.meta_key=admin_color`. It is how an Environment refers to
// a Classification that lives somewhere else, which is the whole point: the reference is
// per destination, the Classification is not.
//
// The two forms exist because Classification already has both. A Discriminator value is
// classified one key at a time under `anonymize.tables.<table>.keys.<key>`, and an
// Approval that could only name a column would have no way to answer what the file said
// about one key — the column holding `admin_color`'s value holds every other key's value
// too, so approving it would approve all of them at once. Naming the key is what makes
// the answer as narrow as the question. See
// docs/adr/0015-an-approval-names-a-discriminator-value-by-its-key.md.
type ColumnRef struct {
	Table  string
	Column string
	// Key is the `keys` entry this reference is for, spelled as the file spells it —
	// a prefix keeps its `*`, and the empty Discriminator value is EmptyKey — and empty
	// for an ordinary column. Where it is set, Column is the table's Discriminator rather
	// than the column whose real values are at stake.
	Key string
}

// Entry is the `keys` entry a keyed reference names, as a key of Table.Keys.
func (r ColumnRef) Entry() string {
	if r.Key == EmptyKey {
		return ""
	}
	return r.Key
}

func (r ColumnRef) String() string {
	if r.Key != "" {
		return r.Table + "." + r.Column + "=" + r.Key
	}
	return r.Table + "." + r.Column
}

// Keyed reports whether this reference names a Discriminator value rather than a column.
func (r ColumnRef) Keyed() bool { return r.Key != "" }

// UnmarshalYAML reads the `table.column` form and the `table.column=key` form, and only
// those two.
//
// A bare column name is refused rather than matched loosely: `display_name` would
// approve real values in every table that happens to have one, which is a far larger
// decision than the one being written down. A half-written key — `usermeta.meta_key=` —
// is refused for the same reason in reverse: it reads like it names something and names
// nothing.
func (r *ColumnRef) UnmarshalYAML(node ast.Node) error {
	var ref string
	if err := yaml.NodeToValue(node, &ref, yaml.Strict()); err != nil {
		return fmt.Errorf("%s: an approval is a table.column reference: %w", yamlPath(node), err)
	}

	// The key is cut off first. A Discriminator value is a value and may hold a dot —
	// `wp_user-settings-time` does not, but nothing says the next one will not — so
	// reading it before the table.column half keeps the dot rule where it belongs.
	head, key, keyed := strings.Cut(ref, "=")
	table, column, found := strings.Cut(head, ".")
	switch {
	case !found || table == "" || column == "" || strings.Contains(column, "."):
		return fmt.Errorf("%s: %q is not a table.column reference — name both, as in users.display_name",
			yamlPath(node), ref)
	case keyed && (key == "" || strings.Contains(key, "=")):
		return fmt.Errorf("%s: %q is not a table.column=key reference — name the discriminator and one of its values, as in usermeta.meta_key=admin_color",
			yamlPath(node), ref)
	}
	r.Table, r.Column, r.Key = table, column, key
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
	return e.ApprovesRef(ColumnRef{Table: table, Column: column})
}

// ApprovesRef reports whether this Environment may receive the real values the reference
// names, in either form.
//
// The match is the whole reference and never part of one, which is what keeps the two
// forms from answering for each other: `usermeta.meta_key=admin_color` says nothing about
// a column called `admin_color`, `usermeta.meta_key` says nothing about any key, and an
// approval naming the wrong Discriminator selects for nothing rather than for the key it
// looks like it meant. Approving by resemblance is the one thing an Approval must not do.
func (e Environment) ApprovesRef(ref ColumnRef) bool {
	if e.Anonymize == nil {
		return false
	}
	return slices.Contains(e.Anonymize.Approved, ref)
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

		// A table entry that classifies nothing is not a neutral statement — it reads
		// like a decision in the diff and is none, and every column in it stays
		// Unclassified. Whoever wrote the name meant to say something about it.
		if table.Discriminator == "" && table.Value == "" && len(table.Keys) == 0 && len(table.Columns) == 0 {
			add("%s classifies nothing — give it columns, or a discriminator and keys, or remove it", at)
			continue
		}

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
			validateKey(add, at+".keys."+SpellKey(key), key)
			validateColumn(add, at+".keys."+SpellKey(key), table.Keys[key])
		}
		for _, column := range sortedKeys(table.Columns) {
			validateColumn(add, at+".columns."+column, table.Columns[column])
		}
	}
}

// validateKey reports the placeholders a `keys` entry cannot hold. brama knows one,
// NumberMark, and an entry holds it at most once: two runs of digits side by side have no
// single way to split, and a key numbered twice is a decision nobody has made yet.
func validateKey(add func(string, ...any), at, key string) {
	numbers := 0
	for _, p := range placeholders(key) {
		if p != NumberMark {
			add("%s: %s is not a placeholder brama knows — %s matches one run of digits, "+
				"and the table prefix is written out in full", at, p, NumberMark)
			continue
		}
		numbers++
	}
	if numbers > 1 {
		add("%s holds %s %d times, and a key holds at most one %s", at, NumberMark, numbers, NumberMark)
	}
}

func validateColumn(add func(string, ...any), at string, col Column) {
	if col.Action == "" {
		add("%s.action is required — one of fake.<generator>, keep, or drop", at)
		return
	}
	if err := col.Action.Validate(); err != nil {
		add("%s.action: %s", at, err)
	}
}

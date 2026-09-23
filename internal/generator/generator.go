// Package generator holds brama's Generator vocabulary: the named recipes a `fake`
// Classification fabricates its values with.
//
// The set is closed and it lives here. A project cannot add to it from brama.yaml and
// an Adapter cannot add to it either, which is what lets `brama anonymize check` mean
// something on a CI runner with no route to a database and no Adapter code loaded, and
// what stops an unrecognised name degrading to "random string" partway through a dump
// on a production Server.
// See docs/adr/0013-brama-owns-the-generator-vocabulary.md.
package generator

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/bramaos/brama/internal/schema"
)

// Generator is one named recipe, and the declaration of what it can fill.
//
// A Generator is data, not a strategy object: what it claims has to be readable
// without running it, because `check` answers "could this Generator fill this column?"
// in the editor rather than mid-dump.
type Generator struct {
	// Name is what follows `fake.` in a Classification — `email` in `fake.email`.
	Name string
	// Patterns are the column names this Generator claims. They are anchored, and
	// they are matched and not scored: a pattern claims a column or it does not.
	// Nothing here is tunable, because a threshold is a number nobody can defend in
	// an incident review and a near miss is how `fake.email` ends up on
	// `email_verified_at`.
	Patterns []*regexp.Regexp
	// Types are the column types this Generator can fill, spelled the way a Schema
	// spells them — the database's own vocabulary, lowercased, MySQL's and
	// PostgreSQL's names side by side.
	Types []string
	// MinLength is the narrowest column this Generator can fill: the length of the
	// longest value its recipe produces. A column shorter than this is refused at
	// check time, because the alternative is a value truncated on insert, or an
	// error raised on a production Server partway through a dump.
	MinLength int64
	// AdapterSupplied reports that the name is brama's and the bytes are the
	// Adapter's. `fake.password` is the only one, and the only one there is meant to
	// be: a password column holds a hash, and only the Adapter knows whether that
	// means phpass or bcrypt. See Resolve.
	AdapterSupplied bool
}

// generators is the vocabulary. It is a package-level value with no way to append to
// it on purpose: ADR 0013 says the set is brama's, and a registry a caller could pass
// its own slice to would be that decision written down and then left unenforced.
//
// The patterns are deliberately tight. A column no Generator claims is left
// Unclassified, which is a question put to a human; a column the wrong Generator
// claims is a phone number fabricated into an address field, discovered on import.
var generators = []Generator{
	{
		Name:      "email",
		Patterns:  patterns(`^email$`, `^email_address$`, `^.+_email$`),
		Types:     text,
		MinLength: 64,
	},
	{
		Name: "full_name",
		// No `^name$`. A `name` column belongs to a product as often as to a
		// person, and there is nothing in a column name to tell them apart.
		Patterns:  patterns(`^full_name$`, `^display_name$`, `^.+_full_name$`),
		Types:     text,
		MinLength: 64,
	},
	{
		Name:      "first_name",
		Patterns:  patterns(`^first_name$`, `^given_name$`, `^.+_first_name$`),
		Types:     text,
		MinLength: 32,
	},
	{
		Name:      "last_name",
		Patterns:  patterns(`^last_name$`, `^family_name$`, `^surname$`, `^.+_last_name$`),
		Types:     text,
		MinLength: 32,
	},
	{
		Name:      "username",
		Patterns:  patterns(`^username$`, `^login$`, `^user_login$`, `^user_nicename$`, `^.+_username$`),
		Types:     text,
		MinLength: 32,
	},
	{
		Name:      "phone",
		Patterns:  patterns(`^phone$`, `^phone_number$`, `^telephone$`, `^.+_phone$`),
		Types:     text,
		MinLength: 24,
	},
	{
		Name:      "url",
		Patterns:  patterns(`^url$`, `^website$`, `^.+_url$`),
		Types:     text,
		MinLength: 64,
	},
	{
		Name:     "ip",
		Patterns: patterns(`^ip$`, `^ip_address$`, `^.+_ip$`),
		// PostgreSQL has a type for this and people use it. `inet` carries no
		// character bound, so MinLength never comes up there.
		Types:     append(slices.Clone(text), "inet"),
		MinLength: 45, // An IPv6 address with an embedded IPv4 one, written out in full.
	},
	{
		Name: "street_address",
		// `^.+_address$` is not here and must not be: it would swallow
		// `email_address` and `ip_address`, which are two other Generators' columns.
		Patterns: patterns(
			`^address$`, `^street_address$`,
			`^address_1$`, `^address_2$`, `^address_line_1$`, `^address_line_2$`,
			`^.+_address_1$`, `^.+_address_2$`, `^.+_address_line_1$`, `^.+_address_line_2$`,
		),
		Types:     text,
		MinLength: 64,
	},
	{
		Name:      "city",
		Patterns:  patterns(`^city$`, `^town$`, `^.+_city$`),
		Types:     text,
		MinLength: 48,
	},
	{
		Name: "postcode",
		Patterns: patterns(
			`^postcode$`, `^postal_code$`, `^zip$`, `^zip_code$`,
			`^.+_postcode$`, `^.+_postal_code$`, `^.+_zip$`, `^.+_zip_code$`,
		),
		Types:     text,
		MinLength: 16,
	},
	{
		Name:      "company",
		Patterns:  patterns(`^company$`, `^company_name$`, `^.+_company$`),
		Types:     text,
		MinLength: 64,
	},
	{
		Name: "password",
		// The exception ADR 0013 records: the name is brama's, the bytes are the
		// Adapter's. See Hasher, and Resolve.
		Patterns:        patterns(`^password$`, `^user_pass$`, `^password_hash$`, `^.+_password$`),
		Types:           text,
		MinLength:       60, // A bcrypt hash. phpass is shorter; argon2id is the Adapter's problem.
		AdapterSupplied: true,
	},
}

// text is the column types a fabricated value can be written to. Every Generator
// produces characters, so every one of them shares this list; it is spelled out
// rather than inferred because a Schema carries the database's own type names and
// "is this a string?" is not a question either database answers directly.
//
// MySQL's names and PostgreSQL's sit together: postgres reports `pg_type.typname`,
// so `character varying(100)` arrives here as `varchar` and `char(2)` as `bpchar`.
var text = []string{
	"varchar", "char", "bpchar", "text", "tinytext", "mediumtext", "longtext",
}

// patterns compiles the column-name patterns of one Generator. They are literals in
// this file, so a bad one is a bug in this build and not something a user can cause.
func patterns(exprs ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(exprs))
	for _, expr := range exprs {
		out = append(out, regexp.MustCompile(expr))
	}
	return out
}

// ClaimsName reports whether this Generator declares a claim on a column of this
// name. It is the half of a claim that needs no Schema, which is what lets `anonymize
// check` say something useful with no Environment reachable.
func (g Generator) ClaimsName(column string) bool {
	for _, p := range g.Patterns {
		if p.MatchString(column) {
			return true
		}
	}
	return false
}

// Fits says why this Generator cannot fill this column, and nil when it can.
//
// It is asked of a Generator the file already named, so it explains itself rather
// than answering yes or no: `check` prints this sentence to someone who has to decide
// what to write instead.
func (g Generator) Fits(col schema.Column) error {
	if !slices.Contains(g.Types, col.Type) {
		return fmt.Errorf("fake.%s cannot fill a %s column, and %s is %s",
			g.Name, col.Type, col.Name, col.Declared)
	}
	// Length zero is not a zero-length column: it is a column the type puts no
	// character bound on at all, which every value fits.
	if col.Length > 0 && col.Length < g.MinLength {
		return fmt.Errorf("fake.%s needs at least %d characters, and %s is %s",
			g.Name, g.MinLength, col.Name, col.Declared)
	}
	return nil
}

// Claim returns the Generator claiming this column, if exactly one does.
//
// A claim is both halves: the name matches a declared pattern and the Generator can
// fill the column's type. A column called `email` holding a bigint is a foreign key
// under a misleading name, and fabricating an address into it breaks the join it
// exists for.
//
// This answers `anonymize init`'s question, which has two outcomes and needs no more:
// write `fake.<generator>`, or leave the column out and therefore Unclassified.
// A caller that has to tell "nobody claims this column" from "fake.email claims it
// and will not fit" — `check`, reporting a varchar too short for the Generator the
// file already names — asks ClaimName and Fits separately, and gets a sentence.
func Claim(col schema.Column) (Generator, bool) {
	g, ok := ClaimName(col.Name)
	if !ok || g.Fits(col) != nil {
		return Generator{}, false
	}
	return g, true
}

// ClaimName returns the Generator claiming a column of this name, if exactly one
// does.
//
// Two claims is an ambiguity brama's own test suite is meant to have caught before
// release. If one reaches here anyway the column is claimed by nobody, which leaves
// it Unclassified — a review item rather than a silent pick between two Generators.
func ClaimName(column string) (Generator, bool) {
	var found Generator
	var count int
	for _, g := range generators {
		if g.ClaimsName(column) {
			found, count = g, count+1
		}
	}
	if count != 1 {
		return Generator{}, false
	}
	return found, true
}

// All returns the vocabulary, for help text and error messages.
//
// The slices inside each Generator are copied too. A shallow copy would hand every
// caller a writable alias of the claims, which is the closed set being open after all
// — quietly, and only for whoever reached in.
func All() []Generator {
	out := make([]Generator, 0, len(generators))
	for _, g := range generators {
		g.Patterns = slices.Clone(g.Patterns)
		g.Types = slices.Clone(g.Types)
		out = append(out, g)
	}
	return out
}

// Lookup returns the Generator with this name.
//
// An unknown name is an error and never a fallback. `fake.e_mail` is a typo in a
// committed file, and the safe reading of a typo is that nobody decided what this
// column should hold.
func Lookup(name string) (Generator, error) {
	for _, g := range generators {
		if g.Name == name {
			return g, nil
		}
	}
	return Generator{}, fmt.Errorf("no generator named %q — brama knows: %s", name, strings.Join(Names(), ", "))
}

// FixedPassword is the one password every fabricated hash is a hash of.
//
// It is documented and it is the same everywhere on purpose. Anonymized data is for
// people who have to log in to it, and a per-row random hash nobody holds the
// preimage of is a staging site with no usable accounts. The value is not a secret:
// nothing that carries it holds real data.
const FixedPassword = "brama"

// Hasher is the Adapter's half of `fake.password` — the seam ADR 0013 records.
//
// A password column holds a hash, and what a valid hash looks like is framework
// knowledge: WordPress writes phpass, Laravel writes bcrypt, and a value in the wrong
// form is an account nobody can sign in to. brama owns the name and the Adapter owns
// the bytes.
//
// No Adapter implements this yet. It exists so that `fake.password` has somewhere to
// fail, with a sentence saying what is missing, rather than nowhere in particular.
type Hasher interface {
	// Hash returns a hash of FixedPassword in the form this framework stores.
	Hash() (string, error)
}

// Resolve returns the Generator named, and reports when this project cannot fabricate
// with it.
//
// hasher is the Adapter's, and nil when the project has no Adapter. That is fine for
// every Generator brama implements itself, and a refusal for `fake.password`: an
// unimplemented Generator has to fail loudly, because both of the quiet answers are
// worse than stopping. A hash-shaped random string leaves staging with no way to log
// in, and falling back to the real value leaks hashes that are personal data and
// crackable offline.
func Resolve(name string, hasher Hasher) (Generator, error) {
	g, err := Lookup(name)
	if err != nil {
		return Generator{}, err
	}
	if g.AdapterSupplied && hasher == nil {
		return Generator{}, fmt.Errorf(
			"fake.%s is supplied by the adapter, and this project has none — "+
				"only the adapter knows what a valid password hash looks like here", g.Name)
	}
	return g, nil
}

// Names lists the Generator names, sorted, for help text and error messages.
func Names() []string {
	names := make([]string, 0, len(generators))
	for _, g := range generators {
		names = append(names, g.Name)
	}
	slices.Sort(names)
	return names
}

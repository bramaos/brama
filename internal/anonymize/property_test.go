package anonymize_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"testing"
	"testing/quick"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/generator"
	"github.com/bramaos/brama/internal/schema"
)

// The tests in this file are property-based: each one makes many random Schemas and
// Classifications, and checks one rule that must hold for all of them. The example
// tests next to this file check only the inputs somebody thought of.
//
// testing/quick prints the input that broke a rule, so a failure can be copied into
// an example test.

// propertyRuns is how many random inputs each property gets. The functions under test
// do no I/O, so this costs well under a second.
const propertyRuns = 500

// baseNames mixes column names that a Generator claims with names that no Generator
// claims, and with near misses such as `email_verified_at`. Random names alone would
// almost never hit a Generator pattern.
var baseNames = []string{
	"email", "email_address", "full_name", "display_name", "first_name", "given_name",
	"last_name", "surname", "username", "login", "user_login", "phone", "telephone",
	"url", "website", "ip", "ip_address", "address", "address_1", "address_line_2",
	"city", "town", "postcode", "zip", "company", "password", "user_pass",
	"id", "name", "internal_note", "email_verified_at", "created_at", "status",
	"total_amount", "meta_value", "phone_verified",
}

var namePrefixes = []string{"", "", "user_", "billing_", "shipping_", "wp_"}

// columnTypes mixes the text types a Generator can fill with types it cannot.
var columnTypes = []string{
	"varchar", "char", "bpchar", "text", "longtext", "inet", "bigint", "int", "datetime", "json",
}

// randomSchema is a Schema of up to four tables with random columns. Column names are
// unique inside a table.
type randomSchema struct {
	Schema schema.Schema
}

// Generate makes a randomSchema for testing/quick.
func (randomSchema) Generate(r *rand.Rand, _ int) reflect.Value {
	var s schema.Schema
	for i := range 1 + r.Intn(4) {
		t := schema.Table{Name: fmt.Sprintf("t%d", i)}
		seen := map[string]bool{}
		for range r.Intn(12) {
			col := randomColumn(r)
			if seen[col.Name] {
				continue
			}
			seen[col.Name] = true
			t.Columns = append(t.Columns, col)
		}
		s.Tables = append(s.Tables, t)
	}
	return reflect.ValueOf(randomSchema{Schema: s})
}

func randomColumn(r *rand.Rand) schema.Column {
	name := namePrefixes[r.Intn(len(namePrefixes))] + baseNames[r.Intn(len(baseNames))]
	if r.Intn(8) == 0 {
		name = randomWord(r)
	}
	col := schema.Column{
		Name: name,
		Type: columnTypes[r.Intn(len(columnTypes))],
		// One column in eight is generated, so the skip in Cover gets inputs too.
		Generated: r.Intn(8) == 0,
	}
	col.Declared = col.Type
	switch col.Type {
	case "varchar", "char", "bpchar":
		// Widths around the Generators' MinLength values, from 16 to 64.
		col.Length = 1 + r.Int63n(100)
		col.Declared = fmt.Sprintf("%s(%d)", col.Type, col.Length)
	case "text":
		col.Length = 65535
	}
	return col
}

func randomWord(r *rand.Rand) string {
	const letters = "abcdefghijklmnopqrstuvwxyz_"
	b := make([]byte, 1+r.Intn(12))
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}

// randomProject is a random Schema and a random Classification of some of its
// columns. A classified column gets `keep`, `drop`, or any `fake.<generator>`, also
// one that cannot fill the column.
type randomProject struct {
	Schema    schema.Schema
	Anonymize *config.Anonymize
}

// Generate makes a randomProject for testing/quick.
func (randomProject) Generate(r *rand.Rand, size int) reflect.Value {
	s := randomSchema{}.Generate(r, size).Interface().(randomSchema).Schema
	actions := []config.Classification{config.Keep, config.Drop}
	for _, name := range generator.Names() {
		actions = append(actions, config.Classification("fake."+name))
	}

	a := &config.Anonymize{Tables: map[string]config.Table{}}
	for _, t := range s.Tables {
		cols := map[string]config.Column{}
		for _, col := range t.Columns {
			if col.Generated || r.Intn(2) == 0 {
				continue
			}
			cols[col.Name] = config.Column{Action: actions[r.Intn(len(actions))]}
		}
		a.Tables[t.Name] = config.Table{Columns: cols}
	}
	return reflect.ValueOf(randomProject{Schema: s, Anonymize: a})
}

func checkProperty(t *testing.T, property any) {
	t.Helper()
	if err := quick.Check(property, &quick.Config{MaxCount: propertyRuns}); err != nil {
		t.Error(err)
	}
}

// Rule: Cover answers for every writable column exactly once. A column the file does
// not classify is in Unclassified, in Schema order, and a column it does classify is
// not. Nothing is skipped and nothing is defaulted (ADR 0003).
func TestPropertyCoverListsEveryUnclassifiedColumn(t *testing.T) {
	checkProperty(t, func(p randomProject) bool {
		coverage, _ := anonymize.Cover(p.Anonymize, p.Schema)

		var columns int
		var want []string
		for _, tbl := range p.Schema.Tables {
			for _, col := range tbl.Columns {
				if col.Generated {
					continue
				}
				columns++
				if _, ok := p.Anonymize.Tables[tbl.Name].Columns[col.Name]; !ok {
					want = append(want, tbl.Name+"."+col.Name)
				}
			}
		}

		got := uncovered(coverage)
		if coverage.Columns != columns || !slices.Equal(got, want) {
			t.Logf("Cover() = %d columns, unclassified %q; want %d columns, unclassified %q",
				coverage.Columns, got, columns, want)
			return false
		}
		return true
	})
}

// Rule: Cover reports a problem for a `fake` column exactly when its Generator cannot
// fill the column, and never for `keep` or `drop` (ADR 0013).
func TestPropertyCoverRefusesEveryFakeThatCannotFit(t *testing.T) {
	checkProperty(t, func(p randomProject) bool {
		_, problems := anonymize.Cover(p.Anonymize, p.Schema)

		var want []string
		for _, tbl := range p.Schema.Tables {
			for _, col := range tbl.Columns {
				classified, ok := p.Anonymize.Tables[tbl.Name].Columns[col.Name]
				if !ok {
					continue
				}
				name, fakes := classified.Action.Generator()
				if !fakes {
					continue
				}
				g, err := generator.Lookup(name)
				if err != nil {
					t.Fatalf("Lookup(%q) = %v; the test wrote a name brama does not know", name, err)
				}
				if g.Fits(col) != nil {
					want = append(want, fmt.Sprintf("anonymize.tables.%s.columns.%s.action", tbl.Name, col.Name))
				}
			}
		}

		got := make([]string, 0, len(problems))
		for _, problem := range problems {
			got = append(got, problem.At)
		}
		if !slices.Equal(got, want) {
			t.Logf("Cover() problems at %q, want %q", got, want)
			return false
		}
		return true
	})
}

// Rule: init writes only `fake.<generator>`, and only where that Generator claims the
// column by name and can fill its type and length. It never writes `keep` or `drop`
// (ADR 0012).
func TestPropertyBootstrapWritesOnlyAFakeThatClaimsAndFits(t *testing.T) {
	checkProperty(t, func(rs randomSchema) bool {
		coverage, _ := anonymize.Cover(nil, rs.Schema)
		written := anonymize.Bootstrap(coverage)

		for _, tbl := range written {
			st, ok := tableOf(rs.Schema, tbl.Name)
			if !ok {
				t.Logf("Bootstrap() wrote table %s, which the Schema does not have", tbl.Name)
				return false
			}
			for _, c := range tbl.Columns {
				col, ok := st.Column(c.Name)
				if !ok {
					t.Logf("Bootstrap() wrote %s.%s, which the Schema does not have", tbl.Name, c.Name)
					return false
				}
				g, claimed := generator.Claim(col)
				if !claimed || c.Action != config.Classification("fake."+g.Name) {
					t.Logf("Bootstrap() wrote %s.%s: %s for %+v; no Generator claims and fits it that way",
						tbl.Name, c.Name, c.Action, col)
					return false
				}
			}
		}
		return true
	})
}

// Rule: init and check walk the same ground. After init writes its Classification,
// Cover reports as Unclassified exactly what init left out, Unclaimed, and finds no
// Generator that cannot fit.
func TestPropertyCoverAfterBootstrapLeavesOnlyTheUnclaimed(t *testing.T) {
	checkProperty(t, func(rs randomSchema) bool {
		before, _ := anonymize.Cover(nil, rs.Schema)
		written := anonymize.Bootstrap(before)

		a := &config.Anonymize{Tables: map[string]config.Table{}}
		for _, tbl := range written {
			cols := map[string]config.Column{}
			for _, c := range tbl.Columns {
				cols[c.Name] = config.Column{Action: c.Action}
			}
			a.Tables[tbl.Name] = config.Table{Columns: cols}
		}
		after, problems := anonymize.Cover(a, rs.Schema)

		left := anonymize.Unclaimed(before, written)
		want := make([]string, 0, len(left))
		for _, u := range left {
			want = append(want, u.String())
		}
		got := uncovered(after)
		if len(problems) != 0 || !slices.Equal(got, want) {
			t.Logf("Cover() after Bootstrap() = unclassified %q, problems %v; want unclassified %q, no problems",
				got, problems, want)
			return false
		}
		if anonymize.Claimed(written)+len(left) != len(before.Unclassified) {
			t.Logf("Bootstrap() claimed %d and left %d of %d unclassified columns",
				anonymize.Claimed(written), len(left), len(before.Unclassified))
			return false
		}
		return true
	})
}

func tableOf(s schema.Schema, name string) (schema.Table, bool) {
	for _, t := range s.Tables {
		if t.Name == name {
			return t, true
		}
	}
	return schema.Table{}, false
}

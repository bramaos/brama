package anonymize

import (
	"reflect"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/schema"
)

// bootstrapped runs Bootstrap over a schema nothing classifies, which is the state
// `anonymize init` runs in.
func bootstrapped(t *testing.T, s schema.Schema) []config.TableClassification {
	t.Helper()
	coverage, problems := Cover(nil, s)
	if len(problems) > 0 {
		t.Fatalf("Cover(nil, …) = %v problems, want none — nothing named a generator", problems)
	}
	return Bootstrap(coverage)
}

// actions flattens a written classification to `table.column: action`, for assertions
// that care about the decision and not the shape.
func actions(tables []config.TableClassification) map[string]string {
	out := map[string]string{}
	for _, t := range tables {
		for _, c := range t.Columns {
			out[t.Name+"."+c.Name] = string(c.Action)
		}
	}
	return out
}

func text(name, declared string, length int64) schema.Column {
	return schema.Column{Name: name, Type: "varchar", Declared: declared, Length: length}
}

// A declared claim is written, and nothing else is. ADR 0012 has two outcomes.
func TestBootstrapWritesADeclaredClaimAndOmitsTheRest(t *testing.T) {
	s := schema.Schema{Tables: []schema.Table{{Name: "users", Columns: []schema.Column{
		text("email", "varchar(100)", 100),
		text("internal_note", "varchar(255)", 255),
		text("first_name", "varchar(64)", 64),
	}}}}

	written := actions(bootstrapped(t, s))

	if written["users.email"] != "fake.email" || written["users.first_name"] != "fake.first_name" {
		t.Errorf("written = %v, want both claimed columns classified", written)
	}
	if _, ok := written["users.internal_note"]; ok {
		t.Error("internal_note was classified — nothing claims it, and a guess is not a decision")
	}
}

// Not `drop`. Dropping is safe about privacy and reckless about everything else, and
// zeroing a column brama could not name is a product decision made on brama's authority.
func TestBootstrapNeverDropsWhatItCannotName(t *testing.T) {
	s := schema.Schema{Tables: []schema.Table{{Name: "orders", Columns: []schema.Column{
		{Name: "total_amount", Type: "decimal", Declared: "decimal(10,2)"},
		{Name: "retry_count", Type: "int", Declared: "int(11)"},
	}}}}

	tables := bootstrapped(t, s)

	if len(tables) != 0 {
		t.Errorf("Bootstrap() = %v, want nothing written for a table no generator claims", tables)
	}
}

// `keep` sends real production data. It enters the file only where a human put it.
func TestBootstrapNeverWritesKeep(t *testing.T) {
	s := schema.Schema{Tables: []schema.Table{{Name: "users", Columns: []schema.Column{
		text("email", "varchar(100)", 100),
		{Name: "id", Type: "bigint", Declared: "bigint(20) unsigned"},
	}}}}

	for ref, action := range actions(bootstrapped(t, s)) {
		if action == string(config.Keep) || action == string(config.Drop) {
			t.Errorf("%s = %q, want only fake.<generator> from init", ref, action)
		}
	}
}

// A claim is both halves. A column called `email` holding a bigint is a foreign key
// under a misleading name, and an address fabricated into it breaks the join it exists
// for. The same goes for one too short to hold what the generator produces.
func TestBootstrapLeavesAColumnItsGeneratorCannotFill(t *testing.T) {
	s := schema.Schema{Tables: []schema.Table{{Name: "users", Columns: []schema.Column{
		{Name: "email", Type: "bigint", Declared: "bigint(20)"},
		text("billing_email", "varchar(20)", 20),
	}}}}

	tables := bootstrapped(t, s)

	if len(tables) != 0 {
		t.Errorf("Bootstrap() = %v, want neither the wrong type nor the short column classified", tables)
	}
}

// The order is the schema's, table by table and column by column. brama.yaml is read
// in a diff, and two runs against the same database have to produce the same bytes.
func TestBootstrapKeepsTheSchemaOrder(t *testing.T) {
	s := schema.Schema{Tables: []schema.Table{
		{Name: "users", Columns: []schema.Column{
			text("username", "varchar(60)", 60),
			text("email", "varchar(100)", 100),
		}},
		{Name: "orders", Columns: []schema.Column{text("billing_email", "varchar(100)", 100)}},
	}}

	tables := bootstrapped(t, s)

	if len(tables) != 2 || tables[0].Name != "users" || tables[1].Name != "orders" {
		t.Fatalf("tables = %v, want them in the order the schema declares", tables)
	}
	if tables[0].Columns[0].Name != "username" || tables[0].Columns[1].Name != "email" {
		t.Errorf("columns = %v, want the order the table declares", tables[0].Columns)
	}
	if got := Claimed(tables); got != 3 {
		t.Errorf("Claimed() = %d, want 3", got)
	}
}

// A generated column cannot be written to at all, so no classification could be carried
// out on one. Cover already skips it, and init inherits that.
func TestBootstrapSkipsAGeneratedColumn(t *testing.T) {
	s := schema.Schema{Tables: []schema.Table{{Name: "users", Columns: []schema.Column{
		{Name: "email", Type: "varchar", Declared: "varchar(100)", Length: 100, Generated: true},
	}}}}

	if tables := bootstrapped(t, s); len(tables) != 0 {
		t.Errorf("Bootstrap() = %v, want a generated column left alone", tables)
	}
}

// The declared type rides along, so the written file can say why this column got this
// generator without anyone reopening the database.
func TestBootstrapCarriesTheDeclaredType(t *testing.T) {
	s := schema.Schema{Tables: []schema.Table{{Name: "users", Columns: []schema.Column{
		text("email", "varchar(100)", 100),
	}}}}

	tables := bootstrapped(t, s)

	if len(tables) != 1 || !strings.Contains(tables[0].Columns[0].Declared, "varchar(100)") {
		t.Errorf("tables = %v, want the declared type carried through", tables)
	}
}

// usermeta is a key/value table the Classification keys by meta_key, holding the keys
// read off it and none of them classified.
func usermeta(value schema.Column, keys ...string) (*config.Anonymize, schema.Schema) {
	a := &config.Anonymize{Tables: map[string]config.Table{"usermeta": {
		Discriminator: "meta_key", Value: value.Name,
		Keys: map[string]config.Column{"nickname": {Action: "fake.username"}},
	}}}
	s := schema.Schema{Tables: []schema.Table{{
		Name:          "usermeta",
		Columns:       []schema.Column{text("meta_key", "varchar(255)", 255), value},
		Discriminator: schema.Discriminator{Column: "meta_key", Values: keys},
	}}}
	return a, s
}

// A discovered key is classified exactly as a column is: written where a Generator
// claims it by name, and left out, Unclassified, where none does.
func TestBootstrapClassifiesAKeyAsItDoesAColumn(t *testing.T) {
	a, s := usermeta(schema.Column{Name: "meta_value", Type: "longtext", Declared: "longtext"},
		"", "billing_email", "nickname", "stripe_customer_id")
	coverage, problems := Cover(a, s)
	if len(problems) > 0 {
		t.Fatalf("Cover() problems = %v, want none", problems)
	}

	tables := Bootstrap(coverage)

	want := []config.TableClassification{{
		Name: "usermeta", Discriminator: "meta_key", Value: "meta_value",
		Keys: []config.ColumnClassification{{Name: "billing_email", Action: "fake.email"}},
	}}
	if !reflect.DeepEqual(tables, want) {
		t.Errorf("Bootstrap() = %+v, want %+v", tables, want)
	}
	left := UnclaimedKeys(coverage, tables)
	if len(left) != 2 || left[0].Value != "" || left[1].Value != "stripe_customer_id" {
		t.Errorf("UnclaimedKeys() = %+v, want the empty key and stripe_customer_id", left)
	}
}

// The value column is what a key's Generator fills, so a claim on the name alone is half
// a claim. A column too short for an address leaves billing_email Unclassified.
func TestBootstrapLeavesAKeyItsGeneratorCannotFill(t *testing.T) {
	a, s := usermeta(text("meta_value", "varchar(3)", 3), "billing_email")
	coverage, _ := Cover(a, s)

	if tables := Bootstrap(coverage); len(tables) != 0 {
		t.Errorf("Bootstrap() = %+v, want nothing written — fake.email cannot fill a varchar(3)", tables)
	}
}

// A `*` in a `keys` entry is a prefix, or refused, so a key holding one cannot be named by
// itself. Writing it would classify every key it prefixes, or break the file.
func TestBootstrapLeavesAKeyTheFileCannotName(t *testing.T) {
	a, s := usermeta(schema.Column{Name: "meta_value", Type: "longtext", Declared: "longtext"}, "legacy*_email")
	coverage, _ := Cover(a, s)

	if tables := Bootstrap(coverage); len(tables) != 0 {
		t.Errorf("Bootstrap() = %+v, want nothing written for legacy*_email", tables)
	}
}

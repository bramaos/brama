package postgres

import (
	"reflect"
	"testing"

	"github.com/bramaos/brama/internal/schema"
)

func TestAssembleBuildsTablesInNameOrder(t *testing.T) {
	got := assemble("public", []string{"users", "accounts", "sessions"}, nil, nil, nil)

	if got.Database != "public" {
		t.Errorf("Database = %q, want public", got.Database)
	}
	var names []string
	for _, table := range got.Tables {
		names = append(names, table.Name)
	}
	want := []string{"accounts", "sessions", "users"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("table names = %v, want %v", names, want)
	}
}

func TestAssembleColumns(t *testing.T) {
	columns := []columnRow{
		{table: "users", name: "id", dataType: "int8", declared: "bigint", notNull: true},
		{table: "users", name: "email", dataType: "varchar", declared: "character varying(100)", notNull: true, length: 100},
		{table: "users", name: "display_name", dataType: "varchar", declared: "character varying(250)", length: 250},
	}

	got := assemble("public", []string{"users"}, columns, nil, nil)

	want := []schema.Column{
		{Name: "id", Type: "int8", Declared: "bigint"},
		{Name: "email", Type: "varchar", Declared: "character varying(100)", Length: 100},
		{Name: "display_name", Type: "varchar", Declared: "character varying(250)", Nullable: true, Length: 250},
	}
	if !reflect.DeepEqual(got.Tables[0].Columns, want) {
		t.Errorf("Columns = %+v, want %+v", got.Tables[0].Columns, want)
	}
}

// pg_attribute.attgenerated is 's' for a stored generated column and 'v' for the
// virtual ones PostgreSQL 18 added. Both mean the column cannot be written to, so
// neither can carry a Classification.
func TestAssembleRecognisesEveryGeneratedSpelling(t *testing.T) {
	for _, attgenerated := range []string{"s", "v"} {
		t.Run(attgenerated, func(t *testing.T) {
			columns := []columnRow{{table: "orders", name: "total", dataType: "numeric", declared: "numeric(10,2)", generated: attgenerated}}

			got := assemble("public", []string{"orders"}, columns, nil, nil)

			if !got.Tables[0].Columns[0].Generated {
				t.Errorf("attgenerated %q gave Generated = false, want true", attgenerated)
			}
		})
	}
}

// An identity column is PostgreSQL's `AUTO_INCREMENT`, and attgenerated is empty for
// it. It is written to like any other column — reading it as generated would drop
// every primary key out of Classification, which is how a whole table quietly goes
// unanonymized.
func TestAssembleLeavesOrdinaryColumnsUngenerated(t *testing.T) {
	// Empty is what an ordinary column, an identity column and a column with a
	// DEFAULT all report; a space is what the "char" type degrades to on a server
	// or driver that pads it rather than returning the empty string.
	for _, attgenerated := range []string{"", " ", "\x00"} {
		t.Run("attgenerated "+attgenerated, func(t *testing.T) {
			columns := []columnRow{{table: "users", name: "id", dataType: "int8", declared: "bigint", notNull: true, generated: attgenerated}}

			got := assemble("public", []string{"users"}, columns, nil, nil)

			if got.Tables[0].Columns[0].Generated {
				t.Errorf("attgenerated %q gave Generated = true, want false", attgenerated)
			}
		})
	}
}

func TestAssemblePutsThePrimaryKeyFirst(t *testing.T) {
	keys := []keyRow{
		{table: "users", name: "users_login_key", column: "login"},
		{table: "users", name: "users_pkey", column: "id", primary: true},
		{table: "users", name: "users_email_key", column: "email"},
	}

	got := assemble("public", []string{"users"}, nil, keys, nil)

	want := []schema.Key{
		{Name: "users_pkey", Columns: []string{"id"}, Primary: true},
		{Name: "users_email_key", Columns: []string{"email"}},
		{Name: "users_login_key", Columns: []string{"login"}},
	}
	if !reflect.DeepEqual(got.Tables[0].Keys, want) {
		t.Errorf("Keys = %+v, want %+v", got.Tables[0].Keys, want)
	}
}

// PostgreSQL names the primary key after the table rather than calling it PRIMARY, so
// the name says nothing and pg_index.indisprimary is the only thing that does. A key
// named like any other must still sort first and still report Primary.
func TestAssembleFindsThePrimaryKeyByItsFlagNotItsName(t *testing.T) {
	keys := []keyRow{
		{table: "users", name: "aaa_unique", column: "email"},
		{table: "users", name: "zzz_identifies_a_user", column: "id", primary: true},
	}

	got := assemble("public", []string{"users"}, nil, keys, nil)

	key, ok := got.Tables[0].PrimaryKey()
	if !ok || key.Name != "zzz_identifies_a_user" {
		t.Fatalf("PrimaryKey() = %+v, %v, want zzz_identifies_a_user", key, ok)
	}
	if got.Tables[0].Keys[0].Name != "zzz_identifies_a_user" {
		t.Errorf("first key = %q, want the primary key", got.Tables[0].Keys[0].Name)
	}
}

// A composite key arrives as one row per column, in key order. Losing that order
// would turn a key over (order_id, kind) into one over (kind, order_id), which is a
// different constraint.
func TestAssembleKeepsCompositeKeyColumnOrder(t *testing.T) {
	keys := []keyRow{
		{table: "order_addresses", name: "order_kind", column: "order_id"},
		{table: "order_addresses", name: "order_kind", column: "kind"},
	}

	got := assemble("public", []string{"order_addresses"}, nil, keys, nil)

	want := []schema.Key{{Name: "order_kind", Columns: []string{"order_id", "kind"}}}
	if !reflect.DeepEqual(got.Tables[0].Keys, want) {
		t.Errorf("Keys = %+v, want %+v", got.Tables[0].Keys, want)
	}
}

// A unique index over an expression — UNIQUE (lower(slug), kind) — stores 0 in the
// pg_index column list where a column number would go, and no attribute answers to
// it. Keeping the named half would record a unique key over (kind) alone, which is a
// constraint the table does not have.
func TestAssembleDropsKeysOverAnExpression(t *testing.T) {
	keys := []keyRow{
		{table: "users", name: "users_pkey", column: "id", primary: true},
		{table: "users", name: "lower_slug_kind", column: ""},
		{table: "users", name: "lower_slug_kind", column: "kind"},
	}

	got := assemble("public", []string{"users"}, nil, keys, nil)

	want := []schema.Key{{Name: "users_pkey", Columns: []string{"id"}, Primary: true}}
	if !reflect.DeepEqual(got.Tables[0].Keys, want) {
		t.Errorf("Keys = %+v, want only the primary key", got.Tables[0].Keys)
	}
	if got.Tables[0].Unique("kind") {
		t.Errorf("Unique(kind) = true, want false — it is half of an expression key")
	}
}

func TestAssembleForeignKeys(t *testing.T) {
	fks := []foreignKeyRow{
		{table: "order_addresses", name: "fk_address_order", column: "order_id", referencedTable: "orders", referencedColumn: "id"},
		{table: "order_addresses", name: "fk_address_user", column: "user_id", referencedTable: "users", referencedColumn: "id"},
	}

	got := assemble("public", []string{"order_addresses", "orders", "users"}, nil, nil, fks)

	want := []schema.ForeignKey{
		{
			Name:       "fk_address_order",
			Columns:    []string{"order_id"},
			References: schema.Reference{Table: "orders", Columns: []string{"id"}},
		},
		{
			Name:       "fk_address_user",
			Columns:    []string{"user_id"},
			References: schema.Reference{Table: "users", Columns: []string{"id"}},
		},
	}
	if !reflect.DeepEqual(got.Tables[0].ForeignKeys, want) {
		t.Errorf("ForeignKeys = %+v, want %+v", got.Tables[0].ForeignKeys, want)
	}
}

func TestAssembleKeepsCompositeForeignKeyColumnOrder(t *testing.T) {
	fks := []foreignKeyRow{
		{table: "order_items", name: "fk_item_address", column: "order_id", referencedTable: "order_addresses", referencedColumn: "order_id"},
		{table: "order_items", name: "fk_item_address", column: "kind", referencedTable: "order_addresses", referencedColumn: "kind"},
	}

	got := assemble("public", []string{"order_items", "order_addresses"}, nil, nil, fks)

	items, ok := got.Table("order_items")
	if !ok {
		t.Fatalf("Table(order_items) = _, false, want the table")
	}
	want := []schema.ForeignKey{{
		Name:       "fk_item_address",
		Columns:    []string{"order_id", "kind"},
		References: schema.Reference{Table: "order_addresses", Columns: []string{"order_id", "kind"}},
	}}
	if !reflect.DeepEqual(items.ForeignKeys, want) {
		t.Errorf("ForeignKeys = %+v, want %+v", items.ForeignKeys, want)
	}
}

// A foreign key may point at a partition, and partitions are deliberately absent from
// the tables list. The edge goes with them: Table() could never resolve the far end
// and Dependents() would never walk back to it, so recording it would leave the graph
// claiming a table the Schema does not contain.
func TestAssembleDropsForeignKeysPointingOutsideTheSchema(t *testing.T) {
	fks := []foreignKeyRow{
		{table: "order_items", name: "fk_item_order", column: "order_id", referencedTable: "orders", referencedColumn: "id"},
		{table: "order_items", name: "fk_item_partition", column: "event_id", referencedTable: "events_kept", referencedColumn: "id"},
	}

	got := assemble("public", []string{"order_items", "orders"}, nil, nil, fks)

	want := []schema.ForeignKey{{
		Name:       "fk_item_order",
		Columns:    []string{"order_id"},
		References: schema.Reference{Table: "orders", Columns: []string{"id"}},
	}}
	if !reflect.DeepEqual(got.Tables[0].ForeignKeys, want) {
		t.Errorf("ForeignKeys = %+v, want only the edge whose far end is here", got.Tables[0].ForeignKeys)
	}
}

// pg_attribute and pg_index answer about views, materialised views and the partitions
// of a partitioned table alike, and the tables query asks for none of them. Anything
// describing a table that is not in that list describes something with no rows of its
// own to anonymize, or rows already counted under their parent.
func TestAssembleDiscardsRowsForTablesItWasNotGiven(t *testing.T) {
	got := assemble("public",
		[]string{"users"},
		[]columnRow{{table: "active_users", name: "id", dataType: "int8", declared: "bigint", notNull: true}},
		[]keyRow{{table: "active_users", name: "active_users_pkey", column: "id", primary: true}},
		[]foreignKeyRow{{table: "active_users", name: "fk", column: "id", referencedTable: "users", referencedColumn: "id"}},
	)

	if len(got.Tables) != 1 {
		t.Fatalf("got %d tables, want only users", len(got.Tables))
	}
	users := got.Tables[0]
	if len(users.Columns) != 0 || len(users.Keys) != 0 || len(users.ForeignKeys) != 0 {
		t.Errorf("users = %+v, want nothing attached to it", users)
	}
}

func TestAssembleEmptyDatabase(t *testing.T) {
	got := assemble("public", nil, nil, nil, nil)

	if got.Database != "public" {
		t.Errorf("Database = %q, want public", got.Database)
	}
	if len(got.Tables) != 0 {
		t.Errorf("Tables = %+v, want none", got.Tables)
	}
}

package mysql

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/bramaos/brama/internal/schema"
)

func length(n int64) sql.NullInt64 { return sql.NullInt64{Int64: n, Valid: true} }

func TestAssembleBuildsTablesInNameOrder(t *testing.T) {
	got := assemble("wordpress", []string{"wp_users", "wp_options", "wp_postmeta"}, nil, nil, nil)

	if got.Database != "wordpress" {
		t.Errorf("Database = %q, want wordpress", got.Database)
	}
	var names []string
	for _, table := range got.Tables {
		names = append(names, table.Name)
	}
	want := []string{"wp_options", "wp_postmeta", "wp_users"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("table names = %v, want %v", names, want)
	}
}

func TestAssembleColumns(t *testing.T) {
	columns := []columnRow{
		{table: "wp_users", name: "ID", dataType: "bigint", declared: "bigint(20) unsigned", nullable: "NO"},
		{table: "wp_users", name: "user_email", dataType: "varchar", declared: "varchar(100)", nullable: "NO", length: length(100)},
		{table: "wp_users", name: "display_name", dataType: "varchar", declared: "varchar(250)", nullable: "YES", length: length(250)},
	}

	got := assemble("wordpress", []string{"wp_users"}, columns, nil, nil)

	want := []schema.Column{
		{Name: "ID", Type: "bigint", Declared: "bigint(20) unsigned"},
		{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100},
		{Name: "display_name", Type: "varchar", Declared: "varchar(250)", Nullable: true, Length: 250},
	}
	if !reflect.DeepEqual(got.Tables[0].Columns, want) {
		t.Errorf("Columns = %+v, want %+v", got.Tables[0].Columns, want)
	}
}

// MySQL and MariaDB have spelled a generated column's EXTRA four different ways
// across the versions brama will meet. All four mean the same thing: the column
// cannot be written to, so it can carry no Classification.
func TestAssembleRecognisesEveryGeneratedSpelling(t *testing.T) {
	for _, extra := range []string{"VIRTUAL GENERATED", "STORED GENERATED", "VIRTUAL", "PERSISTENT"} {
		t.Run(extra, func(t *testing.T) {
			columns := []columnRow{{table: "wp_orders", name: "total", dataType: "decimal", declared: "decimal(10,2)", nullable: "YES", extra: extra}}

			got := assemble("wordpress", []string{"wp_orders"}, columns, nil, nil)

			if !got.Tables[0].Columns[0].Generated {
				t.Errorf("EXTRA %q gave Generated = false, want true", extra)
			}
		})
	}
}

// DEFAULT_GENERATED is MySQL 8 saying the column has an expression for its default.
// The column is written to like any other, and reading it as a generated column
// would silently drop it from every Anonymization.
func TestAssembleLeavesOrdinaryColumnsUngenerated(t *testing.T) {
	for _, extra := range []string{"", "auto_increment", "DEFAULT_GENERATED", "on update CURRENT_TIMESTAMP"} {
		t.Run(extra, func(t *testing.T) {
			columns := []columnRow{{table: "wp_users", name: "ID", dataType: "bigint", declared: "bigint(20) unsigned", nullable: "NO", extra: extra}}

			got := assemble("wordpress", []string{"wp_users"}, columns, nil, nil)

			if got.Tables[0].Columns[0].Generated {
				t.Errorf("EXTRA %q gave Generated = true, want false", extra)
			}
		})
	}
}

func TestAssemblePutsThePrimaryKeyFirst(t *testing.T) {
	keys := []keyRow{
		{table: "wp_users", name: "user_login_key", column: "user_login"},
		{table: "wp_users", name: "PRIMARY", column: "ID"},
		{table: "wp_users", name: "user_email_key", column: "user_email"},
	}

	got := assemble("wordpress", []string{"wp_users"}, nil, keys, nil)

	want := []schema.Key{
		{Name: "PRIMARY", Columns: []string{"ID"}, Primary: true},
		{Name: "user_email_key", Columns: []string{"user_email"}},
		{Name: "user_login_key", Columns: []string{"user_login"}},
	}
	if !reflect.DeepEqual(got.Tables[0].Keys, want) {
		t.Errorf("Keys = %+v, want %+v", got.Tables[0].Keys, want)
	}
}

// A composite key arrives as one row per column, in key order. Losing that order
// would turn a key over (order_id, kind) into one over (kind, order_id), which is a
// different constraint.
func TestAssembleKeepsCompositeKeyColumnOrder(t *testing.T) {
	keys := []keyRow{
		{table: "wc_order_addresses", name: "order_kind", column: "order_id"},
		{table: "wc_order_addresses", name: "order_kind", column: "kind"},
	}

	got := assemble("wordpress", []string{"wc_order_addresses"}, nil, keys, nil)

	want := []schema.Key{{Name: "order_kind", Columns: []string{"order_id", "kind"}}}
	if !reflect.DeepEqual(got.Tables[0].Keys, want) {
		t.Errorf("Keys = %+v, want %+v", got.Tables[0].Keys, want)
	}
}

// MySQL 8 reports no column name for a key over an expression — UNIQUE ((LOWER(slug)),
// kind). Keeping the named half would record a unique key over (kind) alone, which is
// a constraint the table does not have.
func TestAssembleDropsKeysOverAnExpression(t *testing.T) {
	keys := []keyRow{
		{table: "wp_users", name: "PRIMARY", column: "ID"},
		{table: "wp_users", name: "lower_slug_kind", column: ""},
		{table: "wp_users", name: "lower_slug_kind", column: "kind"},
	}

	got := assemble("wordpress", []string{"wp_users"}, nil, keys, nil)

	want := []schema.Key{{Name: "PRIMARY", Columns: []string{"ID"}, Primary: true}}
	if !reflect.DeepEqual(got.Tables[0].Keys, want) {
		t.Errorf("Keys = %+v, want only the primary key", got.Tables[0].Keys)
	}
	if got.Tables[0].Unique("kind") {
		t.Errorf("Unique(kind) = true, want false — it is half of an expression key")
	}
}

func TestAssembleForeignKeys(t *testing.T) {
	fks := []foreignKeyRow{
		{table: "wc_order_addresses", name: "fk_address_order", column: "order_id", referencedTable: "wp_orders", referencedColumn: "ID"},
		{table: "wc_order_addresses", name: "fk_address_user", column: "user_id", referencedTable: "wp_users", referencedColumn: "ID"},
	}

	got := assemble("wordpress", []string{"wc_order_addresses", "wp_orders", "wp_users"}, nil, nil, fks)

	want := []schema.ForeignKey{
		{
			Name:       "fk_address_order",
			Columns:    []string{"order_id"},
			References: schema.Reference{Table: "wp_orders", Columns: []string{"ID"}},
		},
		{
			Name:       "fk_address_user",
			Columns:    []string{"user_id"},
			References: schema.Reference{Table: "wp_users", Columns: []string{"ID"}},
		},
	}
	if !reflect.DeepEqual(got.Tables[0].ForeignKeys, want) {
		t.Errorf("ForeignKeys = %+v, want %+v", got.Tables[0].ForeignKeys, want)
	}
}

func TestAssembleKeepsCompositeForeignKeyColumnOrder(t *testing.T) {
	fks := []foreignKeyRow{
		{table: "wc_order_items", name: "fk_item_address", column: "order_id", referencedTable: "wc_order_addresses", referencedColumn: "order_id"},
		{table: "wc_order_items", name: "fk_item_address", column: "kind", referencedTable: "wc_order_addresses", referencedColumn: "kind"},
	}

	got := assemble("wordpress", []string{"wc_order_items"}, nil, nil, fks)

	want := []schema.ForeignKey{{
		Name:       "fk_item_address",
		Columns:    []string{"order_id", "kind"},
		References: schema.Reference{Table: "wc_order_addresses", Columns: []string{"order_id", "kind"}},
	}}
	if !reflect.DeepEqual(got.Tables[0].ForeignKeys, want) {
		t.Errorf("ForeignKeys = %+v, want %+v", got.Tables[0].ForeignKeys, want)
	}
}

// information_schema answers about views and base tables alike, and the tables query
// asks only for base tables. Anything describing a table that is not in that list
// describes a view, and a view holds no rows to anonymize.
func TestAssembleDiscardsRowsForTablesItWasNotGiven(t *testing.T) {
	got := assemble("wordpress",
		[]string{"wp_users"},
		[]columnRow{{table: "wp_active_users", name: "ID", dataType: "bigint", declared: "bigint(20)", nullable: "NO"}},
		[]keyRow{{table: "wp_active_users", name: "PRIMARY", column: "ID"}},
		[]foreignKeyRow{{table: "wp_active_users", name: "fk", column: "ID", referencedTable: "wp_users", referencedColumn: "ID"}},
	)

	if len(got.Tables) != 1 {
		t.Fatalf("got %d tables, want only wp_users", len(got.Tables))
	}
	users := got.Tables[0]
	if len(users.Columns) != 0 || len(users.Keys) != 0 || len(users.ForeignKeys) != 0 {
		t.Errorf("wp_users = %+v, want nothing attached to it", users)
	}
}

func TestAssembleEmptyDatabase(t *testing.T) {
	got := assemble("wordpress", nil, nil, nil, nil)

	if got.Database != "wordpress" {
		t.Errorf("Database = %q, want wordpress", got.Database)
	}
	if len(got.Tables) != 0 {
		t.Errorf("Tables = %+v, want none", got.Tables)
	}
}

func TestQuoteDoublesABacktick(t *testing.T) {
	for in, want := range map[string]string{
		"meta_key": "`meta_key`",
		"odd`name": "`odd``name`",
	} {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
	}
}

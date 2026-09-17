package schema_test

import (
	"slices"
	"testing"

	"github.com/bramaos/brama/internal/schema"
)

// shop is a small schema with the shapes the lookups have to get right: a table with
// a primary key and a single-column unique key, a table with a composite unique key
// and no primary key, and two foreign keys pointing at the same table. Tables are in
// the order an Introspector promises to return them — sorted by name.
var shop = schema.Schema{
	Database: "wordpress",
	Tables: []schema.Table{
		{
			Name: "wc_order_addresses",
			Columns: []schema.Column{
				{Name: "order_id", Type: "bigint", Declared: "bigint(20) unsigned"},
				{Name: "user_id", Type: "bigint", Declared: "bigint(20) unsigned"},
				{Name: "kind", Type: "varchar", Declared: "varchar(20)", Length: 20},
			},
			Keys: []schema.Key{{Name: "order_kind", Columns: []string{"order_id", "kind"}}},
			ForeignKeys: []schema.ForeignKey{
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
			},
		},
		{
			Name: "wp_orders",
			Columns: []schema.Column{
				{Name: "ID", Type: "bigint", Declared: "bigint(20) unsigned"},
				{Name: "customer_id", Type: "bigint", Declared: "bigint(20) unsigned", Nullable: true},
			},
			Keys: []schema.Key{{Name: "PRIMARY", Columns: []string{"ID"}, Primary: true}},
			ForeignKeys: []schema.ForeignKey{{
				Name:       "fk_orders_customer",
				Columns:    []string{"customer_id"},
				References: schema.Reference{Table: "wp_users", Columns: []string{"ID"}},
			}},
		},
		{
			Name: "wp_users",
			Columns: []schema.Column{
				{Name: "ID", Type: "bigint", Declared: "bigint(20) unsigned"},
				{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100},
				{Name: "display_name", Type: "varchar", Declared: "varchar(250)", Length: 250, Nullable: true},
			},
			Keys: []schema.Key{
				{Name: "PRIMARY", Columns: []string{"ID"}, Primary: true},
				{Name: "user_email", Columns: []string{"user_email"}},
			},
		},
	},
}

func TestTableLookup(t *testing.T) {
	found, ok := shop.Table("wp_users")
	if !ok {
		t.Fatalf("Table(wp_users) = _, false, want the table")
	}
	if found.Name != "wp_users" {
		t.Errorf("Table(wp_users).Name = %q, want wp_users", found.Name)
	}

	if _, ok := shop.Table("wp_nothing"); ok {
		t.Errorf("Table(wp_nothing) = _, true, want false")
	}
}

func TestColumnLookup(t *testing.T) {
	users, _ := shop.Table("wp_users")

	column, ok := users.Column("user_email")
	if !ok {
		t.Fatalf("Column(user_email) = _, false, want the column")
	}
	if column.Length != 100 {
		t.Errorf("Column(user_email).Length = %d, want 100", column.Length)
	}

	if _, ok := users.Column("user_pass"); ok {
		t.Errorf("Column(user_pass) = _, true, want false")
	}
}

func TestPrimaryKey(t *testing.T) {
	users, _ := shop.Table("wp_users")
	key, ok := users.PrimaryKey()
	if !ok {
		t.Fatalf("PrimaryKey() = _, false, want PRIMARY")
	}
	if !slices.Equal(key.Columns, []string{"ID"}) {
		t.Errorf("PrimaryKey().Columns = %v, want [ID]", key.Columns)
	}
}

// A table without a primary key is ordinary in WordPress, so the absence has to be
// reportable rather than a zero Key the caller cannot tell apart from a real one.
func TestPrimaryKeyAbsent(t *testing.T) {
	addresses, _ := shop.Table("wc_order_addresses")
	if key, ok := addresses.PrimaryKey(); ok {
		t.Errorf("PrimaryKey() = %v, true, want no primary key", key)
	}
}

func TestUniqueCountsOnlySingleColumnKeys(t *testing.T) {
	users, _ := shop.Table("wp_users")
	if !users.Unique("user_email") {
		t.Errorf("Unique(user_email) = false, want true — it has a unique key of its own")
	}
	if !users.Unique("ID") {
		t.Errorf("Unique(ID) = false, want true — the primary key is a unique key")
	}
	if users.Unique("display_name") {
		t.Errorf("Unique(display_name) = true, want false")
	}

	// One column of a composite unique key is not unique on its own: order_id
	// repeats once per address kind, and a fabricated value owes it nothing.
	addresses, _ := shop.Table("wc_order_addresses")
	if addresses.Unique("order_id") {
		t.Errorf("Unique(order_id) = true, want false — it is one half of a composite key")
	}
}

func TestDependentsReadsTheGraphBackwards(t *testing.T) {
	got := shop.Dependents("wp_users")
	want := []string{"wc_order_addresses", "wp_orders"}
	if !slices.Equal(got, want) {
		t.Errorf("Dependents(wp_users) = %v, want %v", got, want)
	}

	// Two foreign keys from one table into another still name that table once.
	if got := shop.Dependents("wp_orders"); !slices.Equal(got, []string{"wc_order_addresses"}) {
		t.Errorf("Dependents(wp_orders) = %v, want [wc_order_addresses]", got)
	}

	if got := shop.Dependents("wc_order_addresses"); got != nil {
		t.Errorf("Dependents(wc_order_addresses) = %v, want nothing", got)
	}
}

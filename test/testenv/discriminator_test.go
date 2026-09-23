//go:build testenv

package testenv

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/bramaos/brama/internal/schema/mysql"
	"github.com/bramaos/brama/internal/schema/postgres"
)

// discriminatorTable is the fixture both servers read keys out of. Dropped when the test
// ends, like the introspection fixture.
const discriminatorTable = "brama_discriminator"

// discriminatorKeys is what the fixture's Discriminator holds, as DiscriminatorValues must
// return it: byte order, the two spellings a case-insensitive collation would fold into
// one kept apart, and NULL and empty read as one "".
var discriminatorKeys = []string{"", "Billing_Email", "_transient_doing_cron", "billing_email"}

// seedUsermetaKeys is how many distinct meta_key values the seed leaves in wp_usermeta:
// the keys core writes for every user, and the billing, shipping and session keys the
// seed gives its WooCommerce customers.
const seedUsermetaKeys = 35

// The seed's wp_usermeta is the key/value table the whole feature exists for, read as a
// connected `anonymize check` reads it.
func TestDiscriminatorValuesOfTheSeedsUsermeta(t *testing.T) {
	db := openDatabase(t, production)

	keys, err := mysql.New(db, databaseName).DiscriminatorValues(t.Context(), "wp_usermeta", "meta_key")
	if err != nil {
		t.Fatalf("DiscriminatorValues(wp_usermeta, meta_key) = %v, want no error", err)
	}

	if len(keys) != seedUsermetaKeys {
		t.Errorf("DiscriminatorValues(wp_usermeta, meta_key) = %d keys, want %d: %q", len(keys), seedUsermetaKeys, keys)
	}
	if !slices.IsSorted(keys) || len(slices.Compact(slices.Clone(keys))) != len(keys) {
		t.Errorf("DiscriminatorValues(wp_usermeta, meta_key) = %q, want distinct keys in byte order", keys)
	}
	for _, want := range []string{"billing_email", "nickname", "session_tokens"} {
		if !slices.Contains(keys, want) {
			t.Errorf("DiscriminatorValues(wp_usermeta, meta_key) = %q, want %q among them", keys, want)
		}
	}
}

// WordPress creates its tables case-insensitive, so the fixture is too: a DISTINCT under
// that collation is exactly what would hide one spelling behind the other.
func TestDiscriminatorValuesMySQLAreByteExact(t *testing.T) {
	db := openDatabase(t, production)
	createDiscriminatorFixture(t, db, `CREATE TABLE `+discriminatorTable+` (
		id       BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		meta_key VARCHAR(255) NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)

	keys, err := mysql.New(db, databaseName).DiscriminatorValues(t.Context(), discriminatorTable, "meta_key")
	if err != nil {
		t.Fatalf("DiscriminatorValues() = %v, want no error", err)
	}
	if !slices.Equal(keys, discriminatorKeys) {
		t.Errorf("DiscriminatorValues() = %q, want %q", keys, discriminatorKeys)
	}
}

func TestDiscriminatorValuesPostgreSQLAreByteExact(t *testing.T) {
	db := openPostgres(t)
	createDiscriminatorFixture(t, db, `CREATE TABLE `+discriminatorTable+` (
		id       BIGSERIAL PRIMARY KEY,
		meta_key VARCHAR(255) NULL
	)`)

	keys, err := postgres.New(db, postgresSchema).DiscriminatorValues(t.Context(), discriminatorTable, "meta_key")
	if err != nil {
		t.Fatalf("DiscriminatorValues() = %v, want no error", err)
	}
	if !slices.Equal(keys, discriminatorKeys) {
		t.Errorf("DiscriminatorValues() = %q, want %q", keys, discriminatorKeys)
	}
}

// createDiscriminatorFixture creates the fixture table with create, fills it with every
// key twice over plus a NULL, and drops it afterwards.
func createDiscriminatorFixture(t *testing.T, db *sql.DB, create string) {
	t.Helper()

	drop := func() {
		// WithoutCancel: the table outlives a cancelled test, and dropping it is the
		// point of running at all.
		statement := "DROP TABLE IF EXISTS " + discriminatorTable
		if _, err := db.ExecContext(context.WithoutCancel(t.Context()), statement); err != nil {
			t.Errorf("%s: %v", statement, err)
		}
	}
	drop()
	t.Cleanup(drop)

	statements := []string{
		create,
		`INSERT INTO ` + discriminatorTable + ` (meta_key) VALUES
			('billing_email'), ('Billing_Email'), ('_transient_doing_cron'), (''), (NULL),
			('billing_email'), ('Billing_Email'), ('_transient_doing_cron'), (''), (NULL)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("creating the fixture: %v\n%s", err, statement)
		}
	}
}

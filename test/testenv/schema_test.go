//go:build testenv

package testenv

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/bramaos/brama/internal/schema"
	"github.com/bramaos/brama/internal/schema/mysql"
)

// The database the Servers run, as compose.yml declares it. The password is not a
// secret — see the note there — and is read from the same environment variable, so
// a developer who overrode it does not have to override it twice.
const (
	databaseName = "wordpress"
	databaseUser = "wordpress"
	databasePort = 3306
)

func databasePassword() string {
	if password := os.Getenv("TESTENV_DB_PASSWORD"); password != "" {
		return password
	}
	return "brama-testenv"
}

// The fixture tables. Prefixed so they are unmistakably the test's own, and dropped
// when it ends — but `make testenv-seed` drops the whole database anyway, so a run
// killed halfway through leaves nothing a reseed does not clear.
const (
	parentTable = "brama_introspection_parent"
	childTable  = "brama_introspection_child"
	viewName    = "brama_introspection_view"
)

// TestIntrospectMySQL runs the MySQL introspector against a real MariaDB.
//
// This is the half of the introspector a fake cannot reach. assemble is pure and
// tested without a database; the four information_schema queries are strings, and a
// string is valid SQL right up until a server disagrees. Everything asserted below
// is something a query could get wrong while still compiling: a composite key read
// back in the wrong order, a view counted as a table, a generated column read as an
// ordinary one.
func TestIntrospectMySQL(t *testing.T) {
	db := openDatabase(t, production)
	createFixture(t, db)

	found, err := mysql.New(db, databaseName).Introspect(t.Context())
	if err != nil {
		t.Fatalf("Introspect() = %v, want no error", err)
	}

	if found.Database != databaseName {
		t.Errorf("Database = %q, want %q", found.Database, databaseName)
	}

	parent, ok := found.Table(parentTable)
	if !ok {
		t.Fatalf("Table(%s) = _, false, want the table", parentTable)
	}

	t.Run("columns in declared order", func(t *testing.T) {
		var names []string
		for _, column := range parent.Columns {
			names = append(names, column.Name)
		}
		want := []string{"id", "slug", "kind", "label", "slug_kind"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("columns = %v, want %v", names, want)
		}
	})

	t.Run("types", func(t *testing.T) {
		id, _ := parent.Column("id")
		if id.Type != "bigint" {
			t.Errorf("id.Type = %q, want bigint", id.Type)
		}
		// Declared carries what Type cannot. MariaDB writes "bigint(20) unsigned"
		// and MySQL 8 writes "bigint unsigned", so only the part both agree on is
		// asserted — the part that changes what a fabricated value may be.
		if !strings.Contains(id.Declared, "unsigned") {
			t.Errorf("id.Declared = %q, want it to say unsigned", id.Declared)
		}

		slug, _ := parent.Column("slug")
		if slug.Type != "varchar" || slug.Declared != "varchar(100)" {
			t.Errorf("slug = %q/%q, want varchar/varchar(100)", slug.Type, slug.Declared)
		}
	})

	t.Run("nullability", func(t *testing.T) {
		if slug, _ := parent.Column("slug"); slug.Nullable {
			t.Errorf("slug.Nullable = true, want false")
		}
		if label, _ := parent.Column("label"); !label.Nullable {
			t.Errorf("label.Nullable = false, want true")
		}
	})

	t.Run("length", func(t *testing.T) {
		if slug, _ := parent.Column("slug"); slug.Length != 100 {
			t.Errorf("slug.Length = %d, want 100", slug.Length)
		}
		// A number has no character length, and information_schema says so with
		// NULL. Scanned wrong, that NULL is an error rather than a zero.
		if id, _ := parent.Column("id"); id.Length != 0 {
			t.Errorf("id.Length = %d, want 0", id.Length)
		}
	})

	t.Run("generated columns", func(t *testing.T) {
		if slugKind, _ := parent.Column("slug_kind"); !slugKind.Generated {
			t.Errorf("slug_kind.Generated = false, want true")
		}
		// auto_increment lands in the same EXTRA field, and reading it as
		// generated would drop the primary key out of every Classification.
		if id, _ := parent.Column("id"); id.Generated {
			t.Errorf("id.Generated = true, want false")
		}
	})

	t.Run("unique keys", func(t *testing.T) {
		want := []schema.Key{
			{Name: "PRIMARY", Columns: []string{"id"}, Primary: true},
			{Name: "parent_slug", Columns: []string{"slug"}},
			{Name: "parent_slug_kind", Columns: []string{"slug", "kind"}},
		}
		if !reflect.DeepEqual(parent.Keys, want) {
			t.Errorf("Keys = %+v, want %+v", parent.Keys, want)
		}

		if !parent.Unique("slug") {
			t.Errorf("Unique(slug) = false, want true")
		}
		if parent.Unique("kind") {
			t.Errorf("Unique(kind) = true, want false — only half of a composite key")
		}
		// parent_label is an index, not a constraint. A fabricated label may
		// repeat, and reading the index as uniqueness would make the
		// Anonymization engine promise something the table never asked for.
		if parent.Unique("label") {
			t.Errorf("Unique(label) = true, want false — parent_label is not unique")
		}
	})

	t.Run("foreign key graph", func(t *testing.T) {
		child, ok := found.Table(childTable)
		if !ok {
			t.Fatalf("Table(%s) = _, false, want the table", childTable)
		}

		want := []schema.ForeignKey{{
			Name:    "brama_introspection_fk",
			Columns: []string{"parent_slug", "parent_kind"},
			References: schema.Reference{
				Table:   parentTable,
				Columns: []string{"slug", "kind"},
			},
		}}
		if !reflect.DeepEqual(child.ForeignKeys, want) {
			t.Errorf("ForeignKeys = %+v, want %+v", child.ForeignKeys, want)
		}

		if got := found.Dependents(parentTable); !reflect.DeepEqual(got, []string{childTable}) {
			t.Errorf("Dependents(%s) = %v, want [%s]", parentTable, got, childTable)
		}
	})

	t.Run("views are not tables", func(t *testing.T) {
		if _, ok := found.Table(viewName); ok {
			t.Errorf("Table(%s) = _, true, want the view to be absent", viewName)
		}
	})

	t.Run("tables sorted by name", func(t *testing.T) {
		for i := 1; i < len(found.Tables); i++ {
			if found.Tables[i-1].Name >= found.Tables[i].Name {
				t.Fatalf("tables out of order at %d: %s then %s",
					i, found.Tables[i-1].Name, found.Tables[i].Name)
			}
		}
	})

	// The fixture proves the queries; WordPress proves they hold against a schema
	// nobody wrote for this test. wp_options is the one that matters most: its
	// option_name is a Discriminator, and it is unique.
	t.Run("the real WordPress schema", func(t *testing.T) {
		options, ok := found.Table("wp_options")
		if !ok {
			t.Fatalf("no wp_options — is the Server seeded? run make testenv-seed")
		}
		if !options.Unique("option_name") {
			t.Errorf("wp_options.option_name is not unique, want unique")
		}

		users, ok := found.Table("wp_users")
		if !ok {
			t.Fatalf("no wp_users — is the Server seeded? run make testenv-seed")
		}
		key, ok := users.PrimaryKey()
		if !ok || !reflect.DeepEqual(key.Columns, []string{"ID"}) {
			t.Errorf("wp_users primary key = %+v, %v, want ID", key, ok)
		}
		if email, ok := users.Column("user_email"); !ok || email.Type != "varchar" {
			t.Errorf("wp_users.user_email = %+v, %v, want a varchar", email, ok)
		}

		// The column the seed plants for exactly this: something no Preset will
		// ever cover. Introspection that only reported columns WordPress ships
		// would hand `anonymize init` a schema with nothing left to refuse on.
		legacy, ok := users.Column("legacy_crm_reference")
		if !ok {
			t.Fatalf("wp_users.legacy_crm_reference is missing — is the Server seeded?")
		}
		if !legacy.Nullable || legacy.Length != 191 {
			t.Errorf("legacy_crm_reference = %+v, want a nullable varchar(191)", legacy)
		}
	})
}

// A database name that is a typo must fail loudly. Answering with an empty Schema
// would mean no column is Unclassified, which is `anonymize check` finding nothing
// to refuse on and a Pull that believes it anonymized everything.
func TestIntrospectUnknownDatabaseFails(t *testing.T) {
	db := openDatabase(t, production)

	_, err := mysql.New(db, "wordpres").Introspect(t.Context())
	if err == nil {
		t.Fatalf("Introspect() on a database that does not exist = no error, want one")
	}
	if !strings.Contains(err.Error(), "wordpres") {
		t.Errorf("error = %v, want it to name the database", err)
	}
}

// openDatabase connects to a Server's MariaDB through an ssh tunnel.
//
// The tunnel is this test's own plumbing, not brama's: on a Server the introspector
// runs inside the Shim, with the database on localhost, and nothing is forwarded.
// MariaDB here listens on the container's loopback interface exactly as a rented
// Server's does, so reaching it from the laptop needs a hole, and `ssh -L` is the
// smallest one — the rig publishes no database port to the host.
func openDatabase(t *testing.T, alias string) *sql.DB {
	t.Helper()
	requireReachable(t, alias)

	local := fmt.Sprintf("127.0.0.1:%d", freePort(t))

	// ExitOnForwardFailure, or ssh happily holds a connection open with no
	// listener and the failure surfaces as a timeout somewhere unrelated.
	//
	// WithoutCancel, because t.Context() is cancelled before cleanup runs: a
	// tunnel tied to it dies while the statements that drop the fixture still have
	// to cross it. The Cleanup below is what ends this process.
	cmd := exec.CommandContext(context.WithoutCancel(t.Context()), "ssh",
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-N",
		"-L", fmt.Sprintf("%s:127.0.0.1:%d", local, databasePort),
		alias)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("opening a tunnel to %s: %v", alias, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	waitForListener(t, local, &stderr)

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", databaseUser, databasePassword(), local, databaseName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("opening %s: %v", databaseName, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing the connection to %s: %v", databaseName, err)
		}
	})
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("reaching %s on %s: %v", databaseName, alias, err)
	}
	return db
}

// createFixture builds the tables the introspector is measured against, and removes
// them afterwards.
//
// A fixture rather than WordPress's own schema, because WordPress has no foreign key
// anywhere in it and no generated column either — so the half of the introspector
// that reads the graph would go unexercised against the one database this rig has.
func createFixture(t *testing.T, db *sql.DB) {
	t.Helper()

	drop := func() {
		// WithoutCancel: the tables outlive a cancelled test, and dropping them is
		// the point of running at all.
		ctx := context.WithoutCancel(t.Context())
		for _, statement := range []string{
			"DROP VIEW IF EXISTS " + viewName,
			"DROP TABLE IF EXISTS " + childTable,
			"DROP TABLE IF EXISTS " + parentTable,
		} {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				t.Errorf("%s: %v", statement, err)
			}
		}
	}
	drop()
	t.Cleanup(drop)

	// Every shape the introspector claims to read: a primary key, a single-column
	// unique key, a composite unique key, a non-unique index that must not be
	// mistaken for one, a nullable column, a generated column, and a composite
	// foreign key whose column order is the thing most easily lost.
	statements := []string{
		`CREATE TABLE ` + parentTable + ` (
			id        BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			slug      VARCHAR(100) NOT NULL,
			kind      VARCHAR(20) NOT NULL,
			label     VARCHAR(250) NULL,
			slug_kind VARCHAR(121) AS (CONCAT(slug, ':', kind)) VIRTUAL,
			PRIMARY KEY (id),
			UNIQUE KEY parent_slug (slug),
			UNIQUE KEY parent_slug_kind (slug, kind),
			KEY parent_label (label)
		) ENGINE=InnoDB`,
		`CREATE TABLE ` + childTable + ` (
			id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			parent_slug VARCHAR(100) NOT NULL,
			parent_kind VARCHAR(20) NOT NULL,
			note        TEXT NULL,
			PRIMARY KEY (id),
			CONSTRAINT brama_introspection_fk
				FOREIGN KEY (parent_slug, parent_kind)
				REFERENCES ` + parentTable + ` (slug, kind)
		) ENGINE=InnoDB`,
		`CREATE VIEW ` + viewName + ` AS SELECT id, slug FROM ` + parentTable,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("creating the fixture: %v\n%s", err, statement)
		}
	}
}

// freePort returns a loopback port nothing is listening on.
func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("releasing the port: %v", err)
	}
	return port
}

// waitForListener blocks until the tunnel's local end accepts a connection.
func waitForListener(t *testing.T, address string, stderr fmt.Stringer) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			if err := conn.Close(); err != nil {
				t.Fatalf("closing the probe connection: %v", err)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the tunnel never came up on %s\n%s", address, strings.TrimSpace(stderr.String()))
}

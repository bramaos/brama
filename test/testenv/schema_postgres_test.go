//go:build testenv

package testenv

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bramaos/brama/internal/schema"
	"github.com/bramaos/brama/internal/schema/postgres"
)

// The PostgreSQL the rig runs, as compose.yml declares it. Same database name, user
// and password as the Servers' MariaDB — what makes it a different test is the server
// answering, not the credentials — and the same note applies: none of it is a secret.
const (
	postgresDatabase = "wordpress"
	postgresUser     = "wordpress"
	// The namespace the introspector is pointed at. On PostgreSQL that is a schema
	// rather than a database, and on nearly every application database it is this
	// one.
	postgresSchema = "public"
)

// postgresPort is the published port, which the Makefile exports and compose
// publishes. Read from the environment so that overriding it moves both.
func postgresPort() string {
	if port := os.Getenv("POSTGRES_PORT"); port != "" {
		return port
	}
	return "5433"
}

// The fixture tables, named as their MariaDB counterparts are so the two tests read
// as the mirrors they are. Dropped when the test ends.
const (
	postgresParentTable      = "brama_introspection_parent"
	postgresChildTable       = "brama_introspection_child"
	postgresViewName         = "brama_introspection_view"
	postgresMatViewName      = "brama_introspection_matview"
	postgresPartitionedTable = "brama_introspection_events"
	postgresPartitionName    = "brama_introspection_events_kept"
)

// TestIntrospectPostgreSQL runs the PostgreSQL introspector against a real
// PostgreSQL.
//
// This is the half of the introspector a fake cannot reach. assemble is pure and
// tested without a database; the four pg_catalog queries are strings, and a string is
// valid SQL right up until a server disagrees. Everything asserted below is something
// a query could get wrong while still compiling: a composite key read back in the
// wrong order, a materialised view counted as a table, an identity column read as a
// generated one.
//
// There is no counterpart here to the MySQL test's pass over the real WordPress
// schema. WordPress does not run on PostgreSQL, so the fixture is the whole of what
// this rig can offer — which is why it carries every shape the introspector claims to
// read rather than only the ones MariaDB could not show.
func TestIntrospectPostgreSQL(t *testing.T) {
	db := openPostgres(t)
	createPostgresFixture(t, db)

	found, err := postgres.New(db, postgresSchema).Introspect(t.Context())
	if err != nil {
		t.Fatalf("Introspect() = %v, want no error", err)
	}

	if found.Database != postgresSchema {
		t.Errorf("Database = %q, want %q", found.Database, postgresSchema)
	}

	parent, ok := found.Table(postgresParentTable)
	if !ok {
		t.Fatalf("Table(%s) = _, false, want the table", postgresParentTable)
	}

	t.Run("columns in declared order", func(t *testing.T) {
		var names []string
		for _, column := range parent.Columns {
			names = append(names, column.Name)
		}
		want := []string{"id", "slug", "kind", "label", "note", "amount", "ref", "slug_kind"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("columns = %v, want %v", names, want)
		}
	})

	// Type is what PostgreSQL stores the type under and Declared is what it prints,
	// and on this server those are two different words for the same type: "int8"
	// and "bigint". Keeping both is the point — neither can be recovered from the
	// other, and the parenthesised half only ever appears in Declared.
	t.Run("types", func(t *testing.T) {
		id, _ := parent.Column("id")
		if id.Type != "int8" || id.Declared != "bigint" {
			t.Errorf("id = %q/%q, want int8/bigint", id.Type, id.Declared)
		}

		slug, _ := parent.Column("slug")
		if slug.Type != "varchar" || slug.Declared != "character varying(100)" {
			t.Errorf("slug = %q/%q, want varchar/character varying(100)", slug.Type, slug.Declared)
		}

		// The precision and scale of a numeric are exactly what reassembling a
		// declaration out of information_schema's separate columns gets wrong.
		amount, _ := parent.Column("amount")
		if amount.Type != "numeric" || amount.Declared != "numeric(10,2)" {
			t.Errorf("amount = %q/%q, want numeric/numeric(10,2)", amount.Type, amount.Declared)
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
		// A number has no character length, and neither has text — PostgreSQL
		// imposes no limit on it at all, so there is no upper bound to report.
		if id, _ := parent.Column("id"); id.Length != 0 {
			t.Errorf("id.Length = %d, want 0", id.Length)
		}
		if note, _ := parent.Column("note"); note.Length != 0 {
			t.Errorf("note.Length = %d, want 0", note.Length)
		}
	})

	t.Run("generated columns", func(t *testing.T) {
		if slugKind, _ := parent.Column("slug_kind"); !slugKind.Generated {
			t.Errorf("slug_kind.Generated = false, want true")
		}
		// An identity column is PostgreSQL's AUTO_INCREMENT. It is written to like
		// any other, and reading it as generated would drop the primary key out of
		// every Classification.
		if id, _ := parent.Column("id"); id.Generated {
			t.Errorf("id.Generated = true, want false")
		}
	})

	t.Run("unique keys", func(t *testing.T) {
		want := []schema.Key{
			{Name: postgresParentTable + "_pkey", Columns: []string{"id"}, Primary: true},
			{Name: "parent_ref", Columns: []string{"ref"}},
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
		// repeat, and reading the index as uniqueness would make the Anonymization
		// engine promise something the table never asked for.
		if parent.Unique("label") {
			t.Errorf("Unique(label) = true, want false — parent_label is not unique")
		}
	})

	// parent_ref is UNIQUE (ref) INCLUDE (label): the index stores label so a read
	// need not visit the table, and constrains only ref. PostgreSQL keeps both in one
	// array and says where the constrained half ends, so counting the whole array
	// gives a key over (ref, label) — which is not a promise about ref on its own,
	// and would have Unique(ref) answer false for a column the database will reject
	// duplicates from.
	t.Run("an INCLUDE list is not part of the key", func(t *testing.T) {
		if !parent.Unique("ref") {
			t.Errorf("Unique(ref) = false, want true")
		}
		if parent.Unique("label") {
			t.Errorf("Unique(label) = true, want false — label is only included, not constrained")
		}
	})

	// A unique index over lower(slug) reaches assemble as a key with one nameless
	// column, because no attribute answers to the zero PostgreSQL stores in its
	// place. The whole key goes, not just that column: keeping the rest would record
	// a unique key over (kind) alone, which this table does not have. The
	// Unique(kind) assertion above is the one that would catch it.
	t.Run("keys over an expression are absent", func(t *testing.T) {
		for _, key := range parent.Keys {
			if key.Name == "parent_lower_slug" {
				t.Errorf("Keys contains %+v, want the expression key absent", key)
			}
		}
	})

	t.Run("foreign key graph", func(t *testing.T) {
		child, ok := found.Table(postgresChildTable)
		if !ok {
			t.Fatalf("Table(%s) = _, false, want the table", postgresChildTable)
		}

		want := []schema.ForeignKey{{
			Name:    "brama_introspection_fk",
			Columns: []string{"parent_slug", "parent_kind"},
			References: schema.Reference{
				Table:   postgresParentTable,
				Columns: []string{"slug", "kind"},
			},
		}}
		if !reflect.DeepEqual(child.ForeignKeys, want) {
			t.Errorf("ForeignKeys = %+v, want %+v", child.ForeignKeys, want)
		}

		if got := found.Dependents(postgresParentTable); !reflect.DeepEqual(got, []string{postgresChildTable}) {
			t.Errorf("Dependents(%s) = %v, want [%s]", postgresParentTable, got, postgresChildTable)
		}
	})

	// A materialised view does hold rows, unlike a plain view, and it is still not a
	// table: they are a copy of rows that live somewhere else, and REFRESH puts them
	// back. Anonymizing them would be work undone by the next refresh, and
	// transferring them without the source is a Schema that lies.
	t.Run("views are not tables", func(t *testing.T) {
		for _, name := range []string{postgresViewName, postgresMatViewName} {
			if _, ok := found.Table(name); ok {
				t.Errorf("Table(%s) = _, true, want it absent", name)
			}
		}
	})

	// The rows of a partitioned table live in its partitions and are reachable
	// through its parent. Reporting both would present every row twice, once under
	// each name, and ask for a Classification on each of them.
	t.Run("a partitioned table is one table", func(t *testing.T) {
		if _, ok := found.Table(postgresPartitionedTable); !ok {
			t.Errorf("Table(%s) = _, false, want the partitioned table", postgresPartitionedTable)
		}
		if _, ok := found.Table(postgresPartitionName); ok {
			t.Errorf("Table(%s) = _, true, want the partition absent", postgresPartitionName)
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
}

// A schema name that is a typo must fail loudly. Answering with an empty Schema would
// mean no column is Unclassified, which is `anonymize check` finding nothing to refuse
// on and a Pull that believes it anonymized everything.
func TestIntrospectUnknownSchemaFails(t *testing.T) {
	db := openPostgres(t)

	_, err := postgres.New(db, "publik").Introspect(t.Context())
	if err == nil {
		t.Fatalf("Introspect() on a schema that does not exist = no error, want one")
	}
	if !strings.Contains(err.Error(), "publik") {
		t.Errorf("error = %v, want it to name the schema", err)
	}
}

// openPostgres connects to the rig's PostgreSQL on its published port.
//
// No ssh tunnel, unlike its MariaDB counterpart: this container is not a Server, holds
// no shell and no sudo, and publishes the port itself. See the note in compose.yml.
func openPostgres(t *testing.T) *sql.DB {
	t.Helper()

	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(postgresUser, databasePassword()),
		Host:     "127.0.0.1:" + postgresPort(),
		Path:     "/" + postgresDatabase,
		RawQuery: "sslmode=disable",
	}
	db, err := sql.Open("pgx", dsn.String())
	if err != nil {
		t.Fatalf("opening %s: %v", postgresDatabase, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing the connection to %s: %v", postgresDatabase, err)
		}
	})
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("reaching PostgreSQL on 127.0.0.1:%s: %v\n\n"+
			"The rig's postgres container is not up. Run make testenv-up.",
			postgresPort(), err)
	}
	return db
}

// createPostgresFixture builds the tables the introspector is measured against, and
// removes them afterwards.
func createPostgresFixture(t *testing.T, db *sql.DB) {
	t.Helper()

	drop := func() {
		// WithoutCancel: the tables outlive a cancelled test, and dropping them is
		// the point of running at all.
		ctx := context.WithoutCancel(t.Context())
		for _, statement := range []string{
			"DROP MATERIALIZED VIEW IF EXISTS " + postgresMatViewName,
			"DROP VIEW IF EXISTS " + postgresViewName,
			"DROP TABLE IF EXISTS " + postgresPartitionedTable + " CASCADE",
			"DROP TABLE IF EXISTS " + postgresChildTable,
			"DROP TABLE IF EXISTS " + postgresParentTable,
		} {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				t.Errorf("%s: %v", statement, err)
			}
		}
	}
	drop()
	t.Cleanup(drop)

	// Every shape the introspector claims to read: an identity column that is not a
	// generated one, a stored generated column that is, a numeric whose declaration
	// cannot be reassembled from its parts, a nullable column, a primary key, a
	// single-column unique constraint, a composite unique constraint, a non-unique
	// index that must not be mistaken for one, a unique index over an expression
	// that must be dropped whole, a unique index with an INCLUDE list that is not
	// part of the key, a composite foreign key whose column order is the thing most
	// easily lost, a view, a materialised view, and a partitioned table whose
	// partition must not be counted separately.
	//
	// The generated column is STORED rather than VIRTUAL even though the pinned
	// server has both: STORED is what every server back to PostgreSQL 12 has, so
	// this fixture still builds if the pin ever moves down. assemble_test.go covers
	// the virtual spelling without a server at all.
	statements := []string{
		`CREATE TABLE ` + postgresParentTable + ` (
			id        bigint GENERATED BY DEFAULT AS IDENTITY,
			slug      character varying(100) NOT NULL,
			kind      character varying(20) NOT NULL,
			label     character varying(250),
			note      text,
			amount    numeric(10,2),
			ref       character varying(40),
			slug_kind character varying(121) GENERATED ALWAYS AS (slug || ':' || kind) STORED,
			PRIMARY KEY (id),
			CONSTRAINT parent_slug UNIQUE (slug),
			CONSTRAINT parent_slug_kind UNIQUE (slug, kind)
		)`,
		`CREATE INDEX parent_label ON ` + postgresParentTable + ` (label)`,
		`CREATE UNIQUE INDEX parent_lower_slug ON ` + postgresParentTable + ` (lower(slug), kind)`,
		`CREATE UNIQUE INDEX parent_ref ON ` + postgresParentTable + ` (ref) INCLUDE (label)`,
		`CREATE TABLE ` + postgresChildTable + ` (
			id          bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			parent_slug character varying(100) NOT NULL,
			parent_kind character varying(20) NOT NULL,
			note        text,
			CONSTRAINT brama_introspection_fk
				FOREIGN KEY (parent_slug, parent_kind)
				REFERENCES ` + postgresParentTable + ` (slug, kind)
		)`,
		`CREATE VIEW ` + postgresViewName + ` AS SELECT id, slug FROM ` + postgresParentTable,
		`CREATE MATERIALIZED VIEW ` + postgresMatViewName + ` AS SELECT id, slug FROM ` + postgresParentTable,
		`CREATE TABLE ` + postgresPartitionedTable + ` (
			id   bigint GENERATED BY DEFAULT AS IDENTITY,
			kind character varying(20) NOT NULL,
			PRIMARY KEY (id, kind)
		) PARTITION BY LIST (kind)`,
		`CREATE TABLE ` + postgresPartitionName + ` PARTITION OF ` + postgresPartitionedTable +
			` FOR VALUES IN ('kept')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("creating the fixture: %v\n%s", err, statement)
		}
	}
}

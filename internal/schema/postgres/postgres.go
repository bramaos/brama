// Package postgres reads the shape of one PostgreSQL schema.
//
// It asks pg_catalog rather than information_schema, which is the one place this
// departs from its MySQL counterpart, and it departs for a reason that matters more
// here than portability does. information_schema shows a user only the objects that
// user holds some privilege on, silently. A connection that can read most of a
// database gets back a schema missing the tables and columns it cannot touch, with no
// error anywhere — and a column brama never saw is a column nobody was asked to
// classify, which is the failure `anonymize check` exists to prevent. pg_catalog
// hides nothing. It also answers about declared types exactly, where
// information_schema hands back the parts and leaves reassembling "numeric(10,2)" as
// guesswork.
//
// Every catalogue query is parameterised on the schema name, and no identifier is
// interpolated into one. DiscriminatorValues reads a table rather than the catalogue,
// and a table cannot be a parameter, so it quotes the identifiers it is given. Four
// catalogue queries, run once each, rather than one per table — a
// production database has hundreds of tables, and a hundred round trips over a
// connection the Shim opened on a rented Server is a visible wait.
//
// This package does not connect. The caller supplies an open *sql.DB, because who may
// open a connection to production, and with whose credentials, is not a decision an
// introspector gets to make.
//
// PostgreSQL 12 or later. Earlier servers have no generated columns and no
// pg_attribute.attgenerated to report them with, and both are long out of support.
//
// One known gap: a column whose type is a domain is reported as that domain, with no
// length. The domain is what the column is declared as and naming it is not wrong,
// but a domain over varchar(100) carries a limit this does not pass on, so a
// fabricated value is checked against nothing. Chasing pg_type.typbasetype to the
// underlying type would close it; nothing brama classifies yet needs it, and a
// Length that is honestly absent is better than one inherited from the wrong type.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/bramaos/brama/internal/schema"
	"github.com/bramaos/brama/internal/schema/catalog"
)

// Introspector reads one schema through an open connection.
type Introspector struct {
	db     *sql.DB
	schema string
}

// New returns an Introspector for the named schema on db.
//
// A PostgreSQL schema, not a PostgreSQL database — the namespace a table name is
// unqualified against, which on nearly every application database is "public". It is
// named separately from the connection for the same reason MySQL's is: search_path is
// a mutable property of the session rather than a fact about the Environment, and
// introspecting whatever the session happened to be pointed at is how the wrong
// schema ends up in brama.yaml.
func New(db *sql.DB, schemaName string) *Introspector {
	return &Introspector{db: db, schema: schemaName}
}

var _ schema.Introspector = (*Introspector)(nil)

// Introspect enumerates the schema's base tables, their columns, their unique keys and
// the foreign key graph between them.
func (i *Introspector) Introspect(ctx context.Context) (schema.Schema, error) {
	if i.schema == "" {
		return schema.Schema{}, fmt.Errorf("no schema to introspect")
	}
	if err := i.requireSchema(ctx); err != nil {
		return schema.Schema{}, err
	}

	tables, err := query(ctx, i.db, "the tables of "+i.schema, tablesQuery, scanTable, i.schema)
	if err != nil {
		return schema.Schema{}, err
	}
	columns, err := query(ctx, i.db, "the columns of "+i.schema, columnsQuery, scanColumn, i.schema)
	if err != nil {
		return schema.Schema{}, err
	}
	keys, err := query(ctx, i.db, "the unique keys of "+i.schema, keysQuery, scanKey, i.schema)
	if err != nil {
		return schema.Schema{}, err
	}
	foreignKeys, err := query(ctx, i.db, "the foreign keys of "+i.schema, foreignKeysQuery, scanForeignKey, i.schema, i.schema)
	if err != nil {
		return schema.Schema{}, err
	}

	return assemble(i.schema, tables, columns, keys, foreignKeys), nil
}

// DiscriminatorValues returns the distinct values of column in table, compared as
// bytes and sorted by them, with NULL and empty both read as "".
//
// COLLATE "C" is what makes DISTINCT byte-exact: a nondeterministic collation, a
// case-insensitive ICU one, returns one of `Billing_Email` and `billing_email` and
// drops the other. The cast to text lets a Discriminator of any type take a collation
// at all. COALESCE comes first, so a NULL and an empty value are one row rather than
// two that read the same.
func (i *Introspector) DiscriminatorValues(ctx context.Context, table, column string) ([]string, error) {
	statement := fmt.Sprintf(
		`SELECT DISTINCT COALESCE(%s::text, '') COLLATE "C" AS v FROM %s.%s ORDER BY v`,
		quote(column), quote(i.schema), quote(table))
	return catalog.Query(ctx, i.db, "the values of "+table+"."+column, statement, scanValue)
}

// quote is an identifier as PostgreSQL reads it between double quotes, where a double
// quote is written twice. Quoted, it is also taken as written rather than folded to
// lower case, which is what the catalogue reported it as.
func quote(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func scanValue(rows *sql.Rows) (string, error) {
	var value string
	if err := rows.Scan(&value); err != nil {
		return "", fmt.Errorf("reading a value: %w", err)
	}
	return value, nil
}

// requireSchema fails when the named schema does not exist.
//
// Without this, a typo in the schema name introspects successfully and reports
// nothing — no tables, no columns, nothing Unclassified — and every guardrail that
// depends on finding a column to refuse on has nothing to say. A Pull that anonymizes
// an empty schema is the one failure mode brama must not have.
//
// It is also the check a connection to the wrong database fails: PostgreSQL's
// pg_namespace is per-database, so a session pointed at the wrong one finds no
// "public" it recognises only if that database has none, and finds the wrong one
// otherwise. That second case is not detectable from here, which is why the database
// is chosen in the connection string and named in brama.yaml rather than guessed.
func (i *Introspector) requireSchema(ctx context.Context) error {
	var name string
	err := i.db.QueryRowContext(ctx, schemaQuery, i.schema).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("no schema named %s in this database", i.schema)
		}
		return fmt.Errorf("looking for the schema %s: %w", i.schema, err)
	}
	return nil
}

// pg_catalog is asked in the order the Schema is built, and every result set is sorted
// by the server so that assemble never has to re-derive an order the database already
// knows — in particular the column order of a composite key, which only the position
// in pg_index.indkey and pg_constraint.conkey carries.
const (
	schemaQuery = `
		SELECT n.nspname
		  FROM pg_catalog.pg_namespace n
		 WHERE n.nspname = $1`

	// Ordinary tables and partitioned tables, and neither views nor materialised
	// views: a view holds no rows of its own, so it has nothing to classify,
	// nothing to anonymize and nothing to transfer.
	//
	// relispartition excludes the individual partitions of a partitioned table.
	// They are 'r' like any other table and they are where the rows physically
	// live, but they are reachable — and writable — through their parent, which is
	// already in this list. Reporting both would present the same row twice, once
	// under each name, and ask for a Classification on each of them.
	tablesQuery = `
		SELECT c.relname
		  FROM pg_catalog.pg_class c
		  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1
		   AND c.relkind IN ('r', 'p')
		   AND NOT c.relispartition
		 ORDER BY c.relname`

	// typname is the bare name, format_type the full declaration; both are kept
	// because neither can be recovered from the other. They disagree on purpose:
	// typname is what PostgreSQL stores the type under ("int8", "varchar"), and
	// format_type is what it prints ("bigint", "character varying(100)"), which is
	// the spelling a person recognises from psql.
	//
	// The length arithmetic is PostgreSQL's own: for the two character types
	// atttypmod is the declared length plus the four-byte header, and for
	// everything else — including text, which has no limit — there is no character
	// length to report and the Schema records zero.
	//
	// attnum > 0 skips the system columns, which no user ever writes to, and
	// attisdropped skips the tombstones a dropped column leaves behind.
	columnsQuery = `
		SELECT c.relname, a.attname, t.typname,
		       pg_catalog.format_type(a.atttypid, a.atttypmod),
		       a.attnotnull,
		       CASE WHEN t.typname IN ('varchar', 'bpchar') AND a.atttypmod > 4
		            THEN a.atttypmod - 4 ELSE 0 END,
		       a.attgenerated
		  FROM pg_catalog.pg_attribute a
		  JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
		  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_catalog.pg_type t ON t.oid = a.atttypid
		 WHERE n.nspname = $1
		   AND c.relkind IN ('r', 'p')
		   AND a.attnum > 0
		   AND NOT a.attisdropped
		 ORDER BY c.relname, a.attnum`

	// pg_index rather than pg_constraint is the whole point: PostgreSQL enforces
	// uniqueness with an index, and a bare CREATE UNIQUE INDEX constrains every
	// value just as a UNIQUE constraint does while appearing in no constraint
	// catalogue. An index that permits duplicates says something about how the
	// table is read and nothing about what a fabricated value may be, so
	// indisunique is the only filter.
	//
	// A partial unique index is included. It constrains fewer rows than its
	// definition suggests, so reporting it overstates the obligation — and
	// overstating it costs the Anonymization engine some distinct values, where
	// understating it costs a failed write on a Server mid-Pull.
	//
	// An invalid index — one a CREATE UNIQUE INDEX CONCURRENTLY gave up on — is
	// reported like any other. It enforces nothing, so reporting it overstates the
	// obligation in the same harmless direction a partial index does, and the
	// alternative is brama deciding a column is unconstrained on the strength of a
	// flag that flips the moment someone reruns the statement.
	//
	// unnest ... WITH ORDINALITY is what turns indkey, a positional array, back
	// into rows that remember their position. The join to pg_attribute is a LEFT
	// join because an expression stores 0 there and no attribute answers to it; see
	// expressionKeys.
	//
	// indnkeyatts is where indkey stops constraining anything. The columns after it
	// are an INCLUDE list — payload carried in the index so a read need not visit
	// the table, not part of what must be unique. Taking them for key columns turns
	// UNIQUE (a) INCLUDE (b) into a key over (a, b), and a composite key is not a
	// promise about either column on its own: Table.Unique would then answer false
	// for a, and the Anonymization engine would fabricate duplicates into a column
	// the database will reject them from.
	keysQuery = `
		SELECT c.relname, i.relname, x.indisprimary, a.attname
		  FROM pg_catalog.pg_index x
		  JOIN pg_catalog.pg_class c ON c.oid = x.indrelid
		  JOIN pg_catalog.pg_class i ON i.oid = x.indexrelid
		  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		  CROSS JOIN LATERAL unnest(x.indkey) WITH ORDINALITY AS k(attnum, ord)
		  LEFT JOIN pg_catalog.pg_attribute a
		         ON a.attrelid = c.oid AND a.attnum = k.attnum AND NOT a.attisdropped
		 WHERE n.nspname = $1
		   AND x.indisunique
		   AND k.ord <= x.indnkeyatts
		 ORDER BY c.relname, i.relname, k.ord`

	// Both ends are pinned to the same schema. PostgreSQL permits a foreign key
	// across schemas, and brama introspects one — an edge whose other end is
	// outside what was introspected points at a table the Schema does not contain,
	// and a graph with dangling edges is worse than one that is honestly
	// incomplete.
	//
	// conkey and confkey are unnested together so that each referencing column
	// keeps the referenced column it is positionally matched to; unnesting them
	// separately and zipping them afterwards is how a composite key ends up
	// pointing at the right table through the wrong columns.
	foreignKeysQuery = `
		SELECT c.relname, t.conname, a.attname, rc.relname, ra.attname
		  FROM pg_catalog.pg_constraint t
		  JOIN pg_catalog.pg_class c ON c.oid = t.conrelid
		  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_catalog.pg_class rc ON rc.oid = t.confrelid
		  JOIN pg_catalog.pg_namespace rn ON rn.oid = rc.relnamespace
		  CROSS JOIN LATERAL unnest(t.conkey, t.confkey) WITH ORDINALITY AS k(att, ratt, ord)
		  JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.att
		  JOIN pg_catalog.pg_attribute ra ON ra.attrelid = rc.oid AND ra.attnum = k.ratt
		 WHERE t.contype = 'f'
		   AND n.nspname = $1
		   AND rn.nspname = $2
		 ORDER BY c.relname, t.conname, k.ord`
)

func scanTable(rows *sql.Rows) (string, error) {
	var name string
	if err := rows.Scan(&name); err != nil {
		return "", fmt.Errorf("reading a table name: %w", err)
	}
	return name, nil
}

func scanColumn(rows *sql.Rows) (columnRow, error) {
	var row columnRow
	if err := rows.Scan(&row.table, &row.name, &row.dataType, &row.declared,
		&row.notNull, &row.length, &row.generated); err != nil {
		return columnRow{}, fmt.Errorf("reading a column: %w", err)
	}
	return row, nil
}

// scanKey reads one column of one unique key.
//
// attname is the one nullable field in any of these result sets, and it is nullable
// because the LEFT join in keysQuery found nothing: the index constrains an expression
// rather than a column — UNIQUE (lower(slug)). Scanned into a string that would be an
// error, and a database with one functional index would fail introspection outright
// instead of reporting the other keys.
func scanKey(rows *sql.Rows) (keyRow, error) {
	var (
		row    keyRow
		column sql.NullString
	)
	if err := rows.Scan(&row.table, &row.name, &row.primary, &column); err != nil {
		return keyRow{}, fmt.Errorf("reading a unique key: %w", err)
	}
	row.column = column.String
	return row, nil
}

func scanForeignKey(rows *sql.Rows) (foreignKeyRow, error) {
	var row foreignKeyRow
	if err := rows.Scan(&row.table, &row.name, &row.column,
		&row.referencedTable, &row.referencedColumn); err != nil {
		return foreignKeyRow{}, fmt.Errorf("reading a foreign key: %w", err)
	}
	return row, nil
}

// query runs one statement and scans every row with scan. subject names what is being
// asked for, for the error.
//
// The loop itself is catalog.Query, shared with every other introspector. All this
// adds is which catalogue answered.
func query[T any](ctx context.Context, db *sql.DB, subject, statement string, scan func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	return catalog.Query(ctx, db, "pg_catalog for "+subject, statement, scan, args...)
}

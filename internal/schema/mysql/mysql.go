// Package mysql reads the shape of a MySQL or MariaDB database.
//
// It asks information_schema and nothing else. No WP-CLI, no PHP, no `SHOW CREATE
// TABLE` output to parse: brama talks to the database directly, so introspection
// works on a Server where WordPress is broken, where WP-CLI was never installed, and
// on a database no framework brama has an Adapter for.
//
// Every catalogue query is parameterised on the database name, and no identifier is
// interpolated into one. DiscriminatorValues reads a table rather than the
// catalogue, and a table cannot be a parameter, so it quotes the identifiers it is
// given. Four catalogue queries, run once each, rather than one per table —
// a WooCommerce database has hundreds of tables, and a hundred round trips over a
// connection the Shim opened on a rented Server is a visible wait.
//
// This package does not connect. The caller supplies an open *sql.DB, because who
// may open a connection to production, and with whose credentials, is not a decision
// an introspector gets to make.
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/bramaos/brama/internal/schema"
	"github.com/bramaos/brama/internal/schema/catalog"
)

// Introspector reads one database through an open connection.
type Introspector struct {
	db       *sql.DB
	database string
}

// New returns an Introspector for the named database on db.
//
// The database is named separately from the connection on purpose. A MySQL
// connection has a default database, but it is a mutable property of the session
// rather than a fact about the Environment, and introspecting whatever the session
// happened to be pointed at is how the wrong schema ends up in brama.yaml.
func New(db *sql.DB, database string) *Introspector {
	return &Introspector{db: db, database: database}
}

var _ schema.Introspector = (*Introspector)(nil)

// Introspect enumerates the database's base tables, their columns, their unique keys
// and the foreign key graph between them.
func (i *Introspector) Introspect(ctx context.Context) (schema.Schema, error) {
	if i.database == "" {
		return schema.Schema{}, fmt.Errorf("no database to introspect")
	}
	if err := i.requireDatabase(ctx); err != nil {
		return schema.Schema{}, err
	}

	tables, err := query(ctx, i.db, "the tables of "+i.database, tablesQuery, scanTable, i.database)
	if err != nil {
		return schema.Schema{}, err
	}
	columns, err := query(ctx, i.db, "the columns of "+i.database, columnsQuery, scanColumn, i.database)
	if err != nil {
		return schema.Schema{}, err
	}
	keys, err := query(ctx, i.db, "the unique keys of "+i.database, keysQuery, scanKey, i.database)
	if err != nil {
		return schema.Schema{}, err
	}
	foreignKeys, err := query(ctx, i.db, "the foreign keys of "+i.database, foreignKeysQuery, scanForeignKey, i.database, i.database)
	if err != nil {
		return schema.Schema{}, err
	}

	return assemble(i.database, tables, columns, keys, foreignKeys), nil
}

// DiscriminatorValues returns the distinct values of column in table, compared as
// bytes and sorted by them, with NULL and empty both read as "".
//
// The cast to BINARY is what makes DISTINCT byte-exact. WordPress's own tables are
// created case-insensitive, and DISTINCT under that collation returns one of
// `Billing_Email` and `billing_email` and drops the other. COALESCE comes first, so a
// NULL and an empty value are one row rather than two that read the same.
func (i *Introspector) DiscriminatorValues(ctx context.Context, table, column string) ([]string, error) {
	statement := fmt.Sprintf(
		"SELECT DISTINCT CAST(COALESCE(%s, '') AS BINARY) AS v FROM %s.%s ORDER BY v",
		quote(column), quote(i.database), quote(table))
	return catalog.Query(ctx, i.db, "the values of "+table+"."+column, statement, scanValue)
}

// quote is an identifier as MySQL reads it between backticks, where a backtick is
// written twice.
func quote(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

func scanValue(rows *sql.Rows) (string, error) {
	var value []byte
	if err := rows.Scan(&value); err != nil {
		return "", fmt.Errorf("reading a value: %w", err)
	}
	return string(value), nil
}

// requireDatabase fails when the named database does not exist, or exists and this
// user cannot see it.
//
// Without this, a typo in the database name introspects successfully and reports
// nothing — no tables, no columns, nothing Unclassified — and every guardrail that
// depends on finding a column to refuse on has nothing to say. A Pull that anonymizes
// an empty schema is the one failure mode brama must not have.
func (i *Introspector) requireDatabase(ctx context.Context) error {
	var name string
	err := i.db.QueryRowContext(ctx, databaseQuery, i.database).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("no database named %s, or this user cannot see it", i.database)
		}
		return fmt.Errorf("looking for the database %s: %w", i.database, err)
	}
	return nil
}

// information_schema is asked in the order the Schema is built, and every result set
// is sorted by the server so that assemble never has to re-derive an order the
// database already knows — in particular the column order of a composite key, which
// SEQ_IN_INDEX and ORDINAL_POSITION carry and nothing else does.
const (
	databaseQuery = `
		SELECT SCHEMA_NAME
		  FROM information_schema.SCHEMATA
		 WHERE SCHEMA_NAME = ?`

	// Base tables only. A view holds no rows of its own, so it has nothing to
	// classify, nothing to anonymize and nothing to transfer.
	tablesQuery = `
		SELECT TABLE_NAME
		  FROM information_schema.TABLES
		 WHERE TABLE_SCHEMA = ?
		   AND TABLE_TYPE = 'BASE TABLE'
		 ORDER BY TABLE_NAME`

	// DATA_TYPE is the bare name, COLUMN_TYPE the full declaration; both are kept
	// because neither can be recovered from the other.
	columnsQuery = `
		SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE,
		       CHARACTER_MAXIMUM_LENGTH, EXTRA
		  FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = ?
		 ORDER BY TABLE_NAME, ORDINAL_POSITION`

	// NON_UNIQUE = 0 is the whole point of asking STATISTICS rather than reading
	// COLUMN_KEY: an index that permits duplicates says something about how the
	// table is read and nothing about what a fabricated value may be, while a
	// unique one constrains every value Anonymization is allowed to write. See
	// scanKey for why COLUMN_NAME is read as nullable.
	keysQuery = `
		SELECT TABLE_NAME, INDEX_NAME, COLUMN_NAME
		  FROM information_schema.STATISTICS
		 WHERE TABLE_SCHEMA = ?
		   AND NON_UNIQUE = 0
		 ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`

	// Both ends are pinned to the same schema. MySQL permits a foreign key across
	// databases, and brama introspects one — an edge whose other end is outside
	// what was introspected points at a table the Schema does not contain, and a
	// graph with dangling edges is worse than one that is honestly incomplete.
	foreignKeysQuery = `
		SELECT TABLE_NAME, CONSTRAINT_NAME, COLUMN_NAME,
		       REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
		  FROM information_schema.KEY_COLUMN_USAGE
		 WHERE TABLE_SCHEMA = ?
		   AND REFERENCED_TABLE_SCHEMA = ?
		 ORDER BY TABLE_NAME, CONSTRAINT_NAME, ORDINAL_POSITION`
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
		&row.nullable, &row.length, &row.extra); err != nil {
		return columnRow{}, fmt.Errorf("reading a column: %w", err)
	}
	return row, nil
}

// scanKey reads one column of one unique key.
//
// COLUMN_NAME is the one nullable field in any of these result sets: MySQL 8 leaves
// it NULL for a key over an expression rather than a column — UNIQUE ((LOWER(slug))).
// Scanned into a string that would be an error, and a database with one functional
// index would fail introspection outright instead of reporting the other keys.
func scanKey(rows *sql.Rows) (keyRow, error) {
	var (
		row    keyRow
		column sql.NullString
	)
	if err := rows.Scan(&row.table, &row.name, &column); err != nil {
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
// The loop itself is catalog.Query, shared with every other introspector: the four
// result sets differ only in what a row is, and a copy of the same
// acquire-iterate-check-close dance per query per server is a place for one of them
// to quietly stop checking rows.Err. All this adds is which catalogue answered.
func query[T any](ctx context.Context, db *sql.DB, subject, statement string, scan func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	return catalog.Query(ctx, db, "information_schema for "+subject, statement, scan, args...)
}

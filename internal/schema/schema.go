// Package schema is what brama knows about the shape of a database.
//
// It holds structure and nothing else — table names, column names, types,
// nullability, keys, foreign keys. No row ever passes through here, and that is the
// point: a Classification is a decision made about a column, and `anonymize init`
// has to be able to make every one of them without reading a single value out of
// production.
//
// The model is the same whichever database answered. MySQL and PostgreSQL disagree
// about nearly everything below this line — which catalog holds the answer, what a
// type is called, how a unique constraint is spelled — and an Introspector's job is
// to end that disagreement here, so that the Anonymization engine above never has to
// ask which one it is looking at.
package schema

import (
	"context"
	"slices"
)

// Introspector reads the shape of one database.
//
// Implementations are expected to be deterministic: the same database introspected
// twice returns the same Schema, field for field and in the same order. `anonymize
// init` writes its output into brama.yaml, which is committed and reviewed in a
// diff, so an introspection that shuffled its tables between runs would produce
// churn no reviewer could read past.
type Introspector interface {
	Introspect(ctx context.Context) (Schema, error)
}

// Schema is the structure of one database.
type Schema struct {
	// Database is the namespace that was introspected, and the one a table name in
	// here is unqualified against: a MySQL database, a PostgreSQL schema. Exactly
	// one is introspected at a time, which is what lets Tables hold bare names.
	Database string
	// Tables are the base tables, sorted by name. Views are absent: brama
	// anonymizes and transfers what is stored, and a view stores nothing.
	Tables []Table
}

// Table is one base table.
type Table struct {
	Name string
	// Columns are in the order the table declares them, not sorted, because that
	// is the order a person reading the table sees and the order a dump writes.
	Columns []Column
	// Keys are the table's unique keys, primary first. Non-unique indexes are
	// absent: they say something about how the table is read, and nothing about
	// what a fabricated value is allowed to be.
	Keys []Key
	// ForeignKeys are the edges this table owns in the foreign key graph. Only the
	// referencing side records an edge; Dependents walks it the other way.
	ForeignKeys []ForeignKey
}

// Column is one column, described as far as Anonymization needs it and no further.
type Column struct {
	Name string
	// Type is the bare type name, lowercased — "varchar", "bigint", "text". The
	// vocabulary is the database's own: normalising MySQL's and PostgreSQL's type
	// names into a single invented set would lose exactly the detail a faked value
	// has to respect.
	Type string
	// Declared is the type as the table declares it — "varchar(255)",
	// "bigint(20) unsigned". Kept alongside Type because the parenthesised part is
	// what a human recognises, and because reconstructing it from the parts is
	// guesswork.
	Declared string
	// Nullable reports whether the column accepts NULL. A `drop` Classification
	// writes NULL where it can and the type's empty value where it cannot, so this
	// decides which.
	Nullable bool
	// Length is the declared maximum length of a string column, and zero for every
	// column where that does not apply. A fabricated value that overruns it is
	// either truncated or an error, depending on the server's mood.
	//
	// It is a character count for a declared type — varchar(100) is 100 characters
	// — but a byte budget for the blob-backed types, where MySQL derives it from a
	// TEXT's storage size and the same 65535 buys fewer characters in utf8mb4 than
	// in latin1. Treat it as an upper bound rather than a promise.
	Length int64
	// Generated reports whether the database computes this column from others. A
	// generated column cannot be written to at all, so it can carry no
	// Classification — not even `keep`.
	Generated bool
}

// Key is a uniqueness constraint over one or more columns.
type Key struct {
	// Name is the constraint's name in the database. MySQL calls the primary key
	// "PRIMARY"; PostgreSQL names it after the table. Neither is worth
	// normalising, because the name is only ever shown to a human.
	Name string
	// Columns are the constrained columns, in key order.
	Columns []string
	// Primary reports whether this is the table's primary key.
	Primary bool
}

// ForeignKey is one edge of the foreign key graph: these columns of this table point
// at those columns of another.
type ForeignKey struct {
	Name string
	// Columns are the referencing columns, in key order.
	Columns []string
	// References is what they point at, positionally matched to Columns.
	References Reference
}

// Reference is the far end of a ForeignKey.
type Reference struct {
	Table   string
	Columns []string
}

// Table returns the named table.
func (s Schema) Table(name string) (Table, bool) {
	for _, t := range s.Tables {
		if t.Name == name {
			return t, true
		}
	}
	return Table{}, false
}

// Dependents names the tables holding a foreign key into the named table, sorted.
//
// This is the foreign key graph read backwards, and it is the direction
// Anonymization cares about: faking a value in a referenced column breaks every row
// that points at it, so the engine has to know who points before it decides whether
// it may.
func (s Schema) Dependents(table string) []string {
	var names []string
	for _, t := range s.Tables {
		for _, fk := range t.ForeignKeys {
			if fk.References.Table == table {
				names = append(names, t.Name)
				break
			}
		}
	}
	slices.Sort(names)
	return names
}

// Column returns the named column.
func (t Table) Column(name string) (Column, bool) {
	for _, c := range t.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return Column{}, false
}

// PrimaryKey returns the table's primary key. A table may have none — WordPress
// ships several, and MySQL is happy to store them.
func (t Table) PrimaryKey() (Key, bool) {
	for _, k := range t.Keys {
		if k.Primary {
			return k, true
		}
	}
	return Key{}, false
}

// Unique reports whether the named column is unique on its own.
//
// Only single-column keys count. A column that is merely one part of a composite
// unique key may repeat as often as it likes, so fabricating a value for it carries
// no obligation this can express — and answering true there would send the
// Anonymization engine looking for uniqueness it does not need to preserve.
//
// Uniqueness binds values, not absence: MySQL and PostgreSQL both allow NULL to
// repeat under a unique key. Two fabricated values here must differ, and two dropped
// ones need not, so a `drop` on a unique Nullable column is not the collision it
// looks like.
func (t Table) Unique(column string) bool {
	for _, k := range t.Keys {
		if len(k.Columns) == 1 && k.Columns[0] == column {
			return true
		}
	}
	return false
}

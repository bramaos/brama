package mysql

import (
	"database/sql"
	"slices"
	"strings"

	"github.com/bramaos/brama/internal/schema"
)

// The rows the four queries produce, one type each, holding information_schema's
// answers as it phrases them. Nothing is interpreted while it is being scanned:
// turning "YES" into a bool, or an EXTRA string into "this column cannot be written
// to", is the part worth testing, and it does not need a database to be wrong.
type (
	columnRow struct {
		table    string
		name     string
		dataType string
		declared string
		nullable string
		length   sql.NullInt64
		extra    string
	}

	keyRow struct {
		table  string
		name   string
		column string
	}

	foreignKeyRow struct {
		table            string
		name             string
		column           string
		referencedTable  string
		referencedColumn string
	}
)

// primaryKeyName is what MySQL calls every primary key. It is not a constraint name
// a user chose, which is why it can be compared against rather than configured.
const primaryKeyName = "PRIMARY"

// assemble turns four flat result sets into one Schema.
//
// tables is the authority on what exists: a row describing anything absent from it
// is dropped. information_schema answers about views as readily as base tables, and a
// view has no rows to anonymize and no rows to transfer.
func assemble(database string, tables []string, columns []columnRow, keys []keyRow, foreignKeys []foreignKeyRow) schema.Schema {
	names := slices.Clone(tables)
	slices.Sort(names)

	// Built once and indexed by name, so the three loops below can attach to a
	// table in any order. The slice is never appended to after this, which is what
	// makes the pointers into it safe to hold.
	built := make([]schema.Table, len(names))
	byName := make(map[string]*schema.Table, len(names))
	for i, name := range names {
		built[i] = schema.Table{Name: name}
		byName[name] = &built[i]
	}

	for _, row := range columns {
		table, ok := byName[row.table]
		if !ok {
			continue
		}
		table.Columns = append(table.Columns, schema.Column{
			Name:      row.name,
			Type:      strings.ToLower(row.dataType),
			Declared:  row.declared,
			Nullable:  strings.EqualFold(row.nullable, "YES"),
			Length:    row.length.Int64,
			Generated: generated(row.extra),
		})
	}

	// A key over several columns arrives as one row per column, in key order, so
	// each row after the first extends the key it names rather than starting a new
	// one.
	overExpression := expressionKeys(keys)
	for _, row := range keys {
		table, ok := byName[row.table]
		if !ok {
			continue
		}
		if overExpression[key{row.table, row.name}] {
			continue
		}
		if i := slices.IndexFunc(table.Keys, func(k schema.Key) bool { return k.Name == row.name }); i >= 0 {
			table.Keys[i].Columns = append(table.Keys[i].Columns, row.column)
			continue
		}
		table.Keys = append(table.Keys, schema.Key{
			Name:    row.name,
			Columns: []string{row.column},
			Primary: row.name == primaryKeyName,
		})
	}

	for _, row := range foreignKeys {
		table, ok := byName[row.table]
		if !ok {
			continue
		}
		if i := slices.IndexFunc(table.ForeignKeys, func(fk schema.ForeignKey) bool { return fk.Name == row.name }); i >= 0 {
			table.ForeignKeys[i].Columns = append(table.ForeignKeys[i].Columns, row.column)
			table.ForeignKeys[i].References.Columns = append(table.ForeignKeys[i].References.Columns, row.referencedColumn)
			continue
		}
		table.ForeignKeys = append(table.ForeignKeys, schema.ForeignKey{
			Name:    row.name,
			Columns: []string{row.column},
			References: schema.Reference{
				Table:   row.referencedTable,
				Columns: []string{row.referencedColumn},
			},
		})
	}

	for i := range built {
		// Primary key first, then the rest by name. information_schema's own order
		// is whatever the storage engine felt like, and a Schema that reorders itself
		// between runs would churn the diff of every brama.yaml it is written into.
		slices.SortStableFunc(built[i].Keys, func(a, b schema.Key) int {
			switch {
			case a.Primary == b.Primary:
				return strings.Compare(a.Name, b.Name)
			case a.Primary:
				return -1
			default:
				return 1
			}
		})
		slices.SortStableFunc(built[i].ForeignKeys, func(a, b schema.ForeignKey) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	return schema.Schema{Database: database, Tables: built}
}

// key identifies one unique key: its name is only unique within its table.
type key struct {
	table string
	name  string
}

// expressionKeys names the unique keys that constrain an expression rather than a
// column — UNIQUE ((LOWER(slug)), kind) — which MySQL reports with no column name.
//
// The whole key is dropped, not the nameless part of it. Keeping the rest would
// record a unique key over (kind) alone, which is a constraint the table does not
// have and a promise the Anonymization engine would then try to keep. A key brama
// cannot describe is better absent than wrong.
func expressionKeys(keys []keyRow) map[key]bool {
	over := map[key]bool{}
	for _, row := range keys {
		if row.column == "" {
			over[key{row.table, row.name}] = true
		}
	}
	return over
}

// generated reports whether a column's EXTRA marks it as computed by the database.
//
// Four spellings, because MySQL and MariaDB never agreed and neither has dropped its
// older one. Matched whole rather than by substring on purpose: MySQL 8 writes
// DEFAULT_GENERATED for an ordinary column with an expression as its default, and
// that column is written to like any other.
func generated(extra string) bool {
	switch strings.ToUpper(strings.TrimSpace(extra)) {
	case "VIRTUAL GENERATED", "STORED GENERATED", "VIRTUAL", "PERSISTENT", "STORED":
		return true
	default:
		return false
	}
}

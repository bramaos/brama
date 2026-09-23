// Package catalog is the one loop that reads a database's catalogue, and the
// distinct values of a Discriminator: the one read of rows an introspector makes.
//
// It knows no dialect. Which catalogue holds the answer, and what to ask it, belongs
// to the introspector for that database system; what is here is the part that would
// otherwise be copied once per query per server — acquire, iterate, scan, check, close
// — and every copy of it is a place one of the four steps can quietly stop happening.
package catalog

import (
	"context"
	"database/sql"
	"fmt"
)

// Query runs one statement and scans every row with scan.
//
// subject names what is being asked for, and appears in every error this returns:
// "the columns of wordpress". The caller phrases it, because the catalogue it names
// is the caller's to know.
func Query[T any](ctx context.Context, db *sql.DB, subject, statement string, scan func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("asking %s: %w", subject, err)
	}
	defer func() { _ = rows.Close() }()

	var out []T
	for rows.Next() {
		row, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("asking %s: %w", subject, err)
		}
		out = append(out, row)
	}
	// Reported, never swallowed: a connection dropped halfway through the column
	// list otherwise reads as a table that simply has fewer columns, and a column
	// that was never seen is a column nobody was asked to classify.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("asking %s: %w", subject, err)
	}
	return out, nil
}

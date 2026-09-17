package catalog_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/schema/catalog"
)

func TestQueryScansEveryRowInOrder(t *testing.T) {
	db := open(t, &result{rows: [][]driver.Value{{"wp_options"}, {"wp_posts"}, {"wp_users"}}})

	got, err := catalog.Query(t.Context(), db, "the tables of wordpress", "SELECT 1", scanString)
	if err != nil {
		t.Fatalf("Query() = %v, want no error", err)
	}

	want := []string{"wp_options", "wp_posts", "wp_users"}
	if len(got) != len(want) {
		t.Fatalf("Query() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestQueryOnNoRows(t *testing.T) {
	db := open(t, &result{})

	got, err := catalog.Query(t.Context(), db, "the tables of wordpress", "SELECT 1", scanString)
	if err != nil {
		t.Fatalf("Query() = %v, want no error", err)
	}
	if len(got) != 0 {
		t.Errorf("Query() = %v, want nothing", got)
	}
}

// The failure this whole package exists for. A connection dropped partway through a
// result set leaves rows.Next reporting false exactly as exhaustion does, and the only
// thing that tells them apart is rows.Err. Unchecked, a column list truncated by a
// dead connection reads as a table that simply has fewer columns — and a column brama
// never saw is a column nobody was asked to classify.
func TestQueryReportsAnErrorAfterTheLastRow(t *testing.T) {
	dropped := errors.New("connection reset by peer")
	db := open(t, &result{rows: [][]driver.Value{{"wp_options"}}, err: dropped})

	_, err := catalog.Query(t.Context(), db, "the tables of wordpress", "SELECT 1", scanString)
	if !errors.Is(err, dropped) {
		t.Fatalf("Query() = %v, want it to wrap %v", err, dropped)
	}
	if !strings.Contains(err.Error(), "the tables of wordpress") {
		t.Errorf("error = %v, want it to name what was being asked for", err)
	}
}

func TestQueryReportsAFailedStatement(t *testing.T) {
	refused := errors.New("permission denied")
	db := open(t, &result{queryErr: refused})

	_, err := catalog.Query(t.Context(), db, "the columns of wordpress", "SELECT 1", scanString)
	if !errors.Is(err, refused) {
		t.Fatalf("Query() = %v, want it to wrap %v", err, refused)
	}
	if !strings.Contains(err.Error(), "the columns of wordpress") {
		t.Errorf("error = %v, want it to name what was being asked for", err)
	}
}

func TestQueryReportsAFailedScan(t *testing.T) {
	unreadable := errors.New("not a string")
	db := open(t, &result{rows: [][]driver.Value{{"wp_options"}}})

	_, err := catalog.Query(t.Context(), db, "the keys of wordpress", "SELECT 1",
		func(*sql.Rows) (string, error) { return "", unreadable })
	if !errors.Is(err, unreadable) {
		t.Fatalf("Query() = %v, want it to wrap %v", err, unreadable)
	}
	if !strings.Contains(err.Error(), "the keys of wordpress") {
		t.Errorf("error = %v, want it to name what was being asked for", err)
	}
}

func scanString(rows *sql.Rows) (string, error) {
	var s string
	err := rows.Scan(&s)
	return s, err
}

// A driver that answers with whatever the test handed it. Small enough to read in one
// sitting, and the only way to produce the one failure that matters here: a result set
// that ends early and says why.

// result is one canned answer: the rows to hand back, an error to fail the statement
// with, and an error to raise once the rows run out.
type result struct {
	rows     [][]driver.Value
	queryErr error
	err      error
}

type fakeDriver struct{ result *result }

func (d fakeDriver) Open(string) (driver.Conn, error) { return fakeConn(d), nil }

type fakeConn fakeDriver

func (c fakeConn) Prepare(string) (driver.Stmt, error) { return fakeStmt(c), nil }
func (c fakeConn) Close() error                        { return nil }
func (c fakeConn) Begin() (driver.Tx, error)           { return nil, errors.New("no transactions") }

type fakeStmt fakeDriver

func (s fakeStmt) Close() error  { return nil }
func (s fakeStmt) NumInput() int { return 0 }

func (s fakeStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("no writes")
}

func (s fakeStmt) Query([]driver.Value) (driver.Rows, error) {
	if s.result.queryErr != nil {
		return nil, s.result.queryErr
	}
	return &fakeRows{result: s.result}, nil
}

type fakeRows struct {
	result *result
	next   int
}

func (r *fakeRows) Columns() []string { return []string{"name"} }
func (r *fakeRows) Close() error      { return nil }

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.next >= len(r.result.rows) {
		// io.EOF is exhaustion; anything else is the failure that must not be
		// mistaken for it.
		if r.result.err != nil {
			return r.result.err
		}
		return io.EOF
	}
	copy(dest, r.result.rows[r.next])
	r.next++
	return nil
}

// open registers a driver answering with res and returns a connection to it. Each test
// gets its own driver name, because a name can only be registered once per process.
func open(t *testing.T, res *result) *sql.DB {
	t.Helper()

	name := "catalog-fake-" + t.Name()
	sql.Register(name, fakeDriver{result: res})

	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("opening the fake database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing the fake database: %v", err)
		}
	})
	return db
}

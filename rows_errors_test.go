package sqlcollect_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/raphael-foliveira/sqlcollect"
)

// A tiny driver makes errors that SQLite cannot reliably provoke deterministic.
type errorRows struct {
	nextErr  error
	closeErr error
	closed   bool
}

func (*errorRows) Columns() []string           { return []string{"value"} }
func (r *errorRows) Close() error              { r.closed = true; return r.closeErr }
func (r *errorRows) Next([]driver.Value) error { return r.nextErr }

type errorConn struct{ rows *errorRows }

func (*errorConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unsupported") }
func (*errorConn) Begin() (driver.Tx, error)           { return nil, errors.New("unsupported") }
func (*errorConn) Close() error                        { return nil }
func (c *errorConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return c.rows, nil
}

type errorConnector struct{ rows *errorRows }

func (c errorConnector) Connect(context.Context) (driver.Conn, error) {
	return &errorConn{rows: c.rows}, nil
}
func (errorConnector) Driver() driver.Driver { return errorDriver{} }

type errorDriver struct{}

func (errorDriver) Open(string) (driver.Conn, error) { return nil, errors.New("unsupported") }

func TestIterationAndCloseErrors(t *testing.T) {
	want := errors.New("driver failed")
	for _, one := range []bool{false, true} {
		for _, closeError := range []bool{false, true} {
			r := &errorRows{nextErr: want}
			if closeError {
				r.nextErr = io.EOF
				r.closeErr = want
			}
			db := sql.OpenDB(errorConnector{rows: r})
			rows := query(t, db, "ignored")
			var err error
			if one {
				_, err = sqlcollect.OneRow(rows, sqlcollect.RowTo[int])
			} else {
				_, err = sqlcollect.Rows(rows, sqlcollect.RowTo[int])
			}
			db.Close()
			if !errors.Is(err, want) || !r.closed {
				t.Fatalf("one=%v closeError=%v: got %v, closed=%v", one, closeError, err, r.closed)
			}
		}
	}
}

func TestCallbackAndCloseErrors(t *testing.T) {
	callbackErr := errors.New("callback failed")
	closeErr := errors.New("close failed")
	r := &errorRows{nextErr: nil, closeErr: closeErr}
	db := sql.OpenDB(errorConnector{rows: r})
	defer db.Close()
	values, err := sqlcollect.Rows(query(t, db, "ignored"), func(sqlcollect.CollectableRow) (int, error) {
		return 0, callbackErr
	})
	if values != nil || !errors.Is(err, callbackErr) || !errors.Is(err, closeErr) || !r.closed {
		t.Fatalf("got %#v, %v; closed=%v", values, err, r.closed)
	}
}

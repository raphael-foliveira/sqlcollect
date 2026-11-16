package sqlcollect_test

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"github.com/raphael-foliveira/sqlcollect"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func query(t *testing.T, db *sql.DB, q string) *sql.Rows {
	t.Helper()
	rows, err := db.Query(q)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

type Person struct {
	ID     int
	Name   string `db:"full_name"`
	Note   string `db:"-"`
	hidden string
}

func TestCollectStructs(t *testing.T) {
	db := openDB(t)
	rows := query(t, db, `select 'Alice' as full_name, 1 as ID union all select 'Bob', 2`)
	got, err := sqlcollect.CollectRows(rows, sqlcollect.RowToStructByName[Person])
	want := []Person{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v; want %#v", got, err, want)
	}
	if db.Stats().InUse != 0 {
		t.Fatal("rows were not closed")
	}

	person, err := sqlcollect.CollectOneRow(query(t, db, `select 3 as id, 'Carol' as full_name`), sqlcollect.RowToAddrOfStructByName[Person])
	if err != nil || person == nil || person.ID != 3 || person.Name != "Carol" {
		t.Fatalf("got %#v, %v", person, err)
	}

	people, err := sqlcollect.CollectRows(query(t, db, `select 1 as id, 'Alice' as full_name union all select 2, 'Bob'`), sqlcollect.RowToAddrOfStructByName[Person])
	if err != nil || len(people) != 2 || people[0] == people[1] || people[0].ID != 1 || people[1].ID != 2 {
		t.Fatalf("pointer results: %#v, %v", people, err)
	}
}

func TestEmptyAndScalarResults(t *testing.T) {
	db := openDB(t)
	values, err := sqlcollect.CollectRows(query(t, db, `select 1 where false`), sqlcollect.RowTo[int])
	if err != nil || values == nil || len(values) != 0 {
		t.Fatalf("empty result: %#v, %v", values, err)
	}
	_, err = sqlcollect.CollectOneRow(query(t, db, `select 1 where false`), sqlcollect.RowTo[int])
	if err != sql.ErrNoRows {
		t.Fatalf("got %v, want sql.ErrNoRows", err)
	}
	first, err := sqlcollect.CollectOneRow(query(t, db, `select 1 union all select 2`), sqlcollect.RowTo[int])
	if err != nil || first != 1 || db.Stats().InUse != 0 {
		t.Fatalf("first row: %d, %v; in use %d", first, err, db.Stats().InUse)
	}
}

func TestMappingErrors(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		fn      sqlcollect.RowToFunc[any]
		message string
	}{
		{"non struct", `select 1`, convert(sqlcollect.RowToStructByName[int]), "expected a struct"},
		{"pointer type", `select 1`, convert(sqlcollect.RowToStructByName[*Person]), "expected a struct"},
		{"missing column", `select 1 as id`, convert(sqlcollect.RowToStructByName[Person]), "columns do not match"},
		{"unknown column", `select 1 as id, 'Alice' as unknown`, convert(sqlcollect.RowToStructByName[Person]), "no field"},
		{"duplicate column", `select 1 as id, 2 as ID`, convert(sqlcollect.RowToStructByName[Person]), "duplicate column"},
		{"duplicate tag", `select 1 as id, 2 as other`, convert(sqlcollect.RowToStructByName[struct {
			A int `db:"id"`
			B int `db:"ID"`
		}]), "duplicate field mapping"},
		{"conversion", `select 'not an integer' as id, 'Alice' as full_name`, convert(sqlcollect.RowToStructByName[Person]), "converting"},
		{"null", `select NULL as id, 'Alice' as full_name`, convert(sqlcollect.RowToStructByName[Person]), "NULL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := openDB(t)
			got, err := sqlcollect.CollectRows(query(t, db, tt.query), tt.fn)
			if got != nil || err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("got %#v, %v; want error containing %q", got, err, tt.message)
			}
			if db.Stats().InUse != 0 {
				t.Fatal("rows were not closed on error")
			}
		})
	}
}

func convert[T any](fn sqlcollect.RowToFunc[T]) sqlcollect.RowToFunc[any] {
	return func(row sqlcollect.CollectableRow) (any, error) { return fn(row) }
}

func TestCallbackError(t *testing.T) {
	db := openDB(t)
	want := errors.New("callback failed")
	fn := func(row sqlcollect.CollectableRow) (int, error) {
		value, err := sqlcollect.RowTo[int](row)
		if value == 2 {
			return value, want
		}
		return value, err
	}
	values, err := sqlcollect.CollectRows(query(t, db, `select 1 union all select 2`), fn)
	if values != nil || !errors.Is(err, want) || db.Stats().InUse != 0 {
		t.Fatalf("got %#v, %v; in use %d", values, err, db.Stats().InUse)
	}
	value, err := sqlcollect.CollectOneRow(query(t, db, `select 2`), fn)
	if value != 0 || !errors.Is(err, want) || db.Stats().InUse != 0 {
		t.Fatalf("got %d, %v; in use %d", value, err, db.Stats().InUse)
	}
}

func TestEmbeddedAndScannerFields(t *testing.T) {
	type Identity struct{ ID int }
	type Record struct {
		*Identity
		Name sql.NullString `db:"name"`
		Age  *int           `db:"age"`
	}
	db := openDB(t)
	got, err := sqlcollect.CollectOneRow(query(t, db, `select NULL as name, 42 as age, 7 as id`), sqlcollect.RowToStructByName[Record])
	if err != nil || got.Identity == nil || got.ID != 7 || got.Name.Valid || got.Age == nil || *got.Age != 42 {
		t.Fatalf("got %#v, %v", got, err)
	}
	type Scanner struct{ sql.NullString }
	value, err := sqlcollect.CollectOneRow(query(t, db, `select 'hello' as NullString`), sqlcollect.RowToStructByName[Scanner])
	if err != nil || !value.Valid || value.String != "hello" {
		t.Fatalf("embedded scanner: %#v, %v", value, err)
	}
}

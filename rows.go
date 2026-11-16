package sqlcollect

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// CollectableRow exposes the current row without allowing a callback to advance it.
// *sql.Rows implements this interface.
type CollectableRow interface {
	Columns() ([]string, error)
	Scan(dest ...any) error
}

// RowToFunc converts the current row into a value. It must not retain data whose
// lifetime is limited to the current row (such as sql.RawBytes).
type RowToFunc[T any] func(CollectableRow) (T, error)

// Rows converts every row and closes rows before returning, including on
// errors. An empty result produces a non-nil empty slice; errors discard results.
func Rows[T any](rows *sql.Rows, fn RowToFunc[T]) (values []T, err error) {
	defer func() {
		err = closeRows(rows, err)
		if err != nil {
			values = nil
		}
	}()
	values = make([]T, 0)
	for rows.Next() {
		value, scanErr := fn(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// OneRow converts the first row and closes rows, discarding any remaining
// rows. It returns sql.ErrNoRows when the result is empty.
func OneRow[T any](rows *sql.Rows, fn RowToFunc[T]) (value T, err error) {
	defer func() {
		err = closeRows(rows, err)
		if err != nil {
			var zero T
			value = zero
		}
	}()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return value, err
		}
		return value, sql.ErrNoRows
	}
	value, err = fn(rows)
	if err != nil {
		var zero T
		return zero, err
	}
	return value, rows.Err()
}

func closeRows(rows *sql.Rows, err error) error {
	if closeErr := rows.Close(); closeErr != nil {
		return errors.Join(err, closeErr)
	}
	return err
}

// RowTo scans a single-column row into T, using database/sql's conversions and
// sql.Scanner support.
func RowTo[T any](row CollectableRow) (T, error) {
	var value T
	err := row.Scan(&value)
	return value, err
}

// RowToStructByName scans into a struct T by matching column names to exported
// field names, case-insensitively. A db tag overrides the name; db:"-" ignores a
// field. Every column and every non-ignored exported field must match exactly
// once. Exported embedded structs (including pointers) are flattened, while
// sql.Scanner fields are scanned as values. Struct metadata is cached safely
// across goroutines. T must be a struct; use RowToAddrOfStructByName for pointers.
func RowToStructByName[T any](row CollectableRow) (T, error) {
	var value T
	if err := scanStruct(row, reflect.ValueOf(&value).Elem()); err != nil {
		var zero T
		return zero, err
	}
	return value, nil
}

// RowToAddrOfStructByName is RowToStructByName with a pointer result. It returns
// nil on error. T must be a struct.
func RowToAddrOfStructByName[T any](row CollectableRow) (*T, error) {
	value, err := RowToStructByName[T](row)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

type structFields struct {
	byName map[string][]int
	err    error
}

var (
	fieldCache  sync.Map // map[reflect.Type]structFields
	scannerType = reflect.TypeFor[sql.Scanner]()
)

func fieldsFor(typ reflect.Type) structFields {
	if cached, ok := fieldCache.Load(typ); ok {
		return cached.(structFields)
	}
	fields := structFields{byName: make(map[string][]int)}
	var visit func(reflect.Type, []int, map[reflect.Type]bool)
	visit = func(typ reflect.Type, prefix []int, ancestors map[reflect.Type]bool) {
		if ancestors[typ] {
			fields.err = fmt.Errorf("sqlcollect: recursive embedded struct %s", typ)
			return
		}
		ancestors[typ] = true
		defer delete(ancestors, typ)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" || field.Tag.Get("db") == "-" {
				continue
			}
			path := append(append([]int(nil), prefix...), i)
			name := field.Tag.Get("db")
			base := field.Type
			if base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if field.Anonymous && name == "" && base.Kind() == reflect.Struct && !reflect.PointerTo(base).Implements(scannerType) {
				visit(base, path, ancestors)
				continue
			}
			if name == "" {
				name = field.Name
			}
			name = strings.ToLower(name)
			if _, exists := fields.byName[name]; exists {
				fields.err = fmt.Errorf("sqlcollect: duplicate field mapping %q in %s", name, typ)
			}
			fields.byName[name] = path
		}
	}
	visit(typ, nil, make(map[reflect.Type]bool))
	actual, _ := fieldCache.LoadOrStore(typ, fields)
	return actual.(structFields)
}

func scanStruct(row CollectableRow, value reflect.Value) error {
	if value.Kind() != reflect.Struct {
		return fmt.Errorf("sqlcollect: expected a struct, got %s", value.Type())
	}
	fields := fieldsFor(value.Type())
	if fields.err != nil {
		return fields.err
	}
	columns, err := row.Columns()
	if err != nil {
		return err
	}
	if len(columns) != len(fields.byName) {
		return fmt.Errorf("sqlcollect: %d columns do not match %d fields in %s", len(columns), len(fields.byName), value.Type())
	}
	dest := make([]any, len(columns))
	seen := make(map[string]bool, len(columns))
	for i, column := range columns {
		name := strings.ToLower(column)
		path, ok := fields.byName[name]
		if !ok {
			return fmt.Errorf("sqlcollect: no field for column %q in %s", column, value.Type())
		}
		if seen[name] {
			return fmt.Errorf("sqlcollect: duplicate column %q", column)
		}
		seen[name] = true
		field := value
		for _, index := range path {
			if field.Kind() == reflect.Pointer {
				if field.IsNil() {
					field.Set(reflect.New(field.Type().Elem()))
				}
				field = field.Elem()
			}
			field = field.Field(index)
		}
		dest[i] = field.Addr().Interface()
	}
	return row.Scan(dest...)
}

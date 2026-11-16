# sqlcollect

Collect `database/sql` rows into typed values with generic callbacks and struct tags.
Inspired by pgx's [`CollectRows`](https://pkg.go.dev/github.com/jackc/pgx/v5#CollectRows) API, built for standard `database/sql` drivers. The library uses only the Go standard library; SQLite is used by the tests.

Requires Go 1.22 or later.

## Install

```sh
go get github.com/raphael-foliveira/sqlcollect
```

## Collect structs

```go
import (
    "context"
    "database/sql"

    "github.com/raphael-foliveira/sqlcollect"
)

type Passenger struct {
    ID        int    `db:"id"`
    FirstName string `db:"first_name"`
    LastName  string `db:"last_name"`
}

func passengers(ctx context.Context, db *sql.DB) ([]Passenger, error) {
    rows, err := db.QueryContext(ctx, `
        SELECT id, first_name, last_name FROM passengers`)
    if err != nil {
        return nil, err
    }
    return sqlcollect.CollectRows(rows, sqlcollect.RowToStructByName[Passenger])
}
```

The collector closes rows automatically, including when scanning fails. Always check the query error before calling it; rows must be non-nil. Callbacks receive the current row and must not advance or close it.

For `[]*Passenger`, use `RowToAddrOfStructByName[Passenger]`.

## Collect one row

```go
rows, err := db.QueryContext(ctx, `
    SELECT id, first_name, last_name FROM passengers WHERE id = ?`, id)
if err != nil {
    return nil, err
}
return sqlcollect.CollectOneRow(rows, sqlcollect.RowToAddrOfStructByName[Passenger])
```

Use your driver's parameter syntax (`?`, `$1`, etc.). `CollectOneRow` returns the first result, discards additional rows, and closes rows. An empty result returns `sql.ErrNoRows`.

## Scalars and custom callbacks

Use `RowTo[T]` for a single column:

```go
ids, err := sqlcollect.CollectRows(rows, sqlcollect.RowTo[int])
```

A callback can also scan and transform values directly:

```go
names, err := sqlcollect.CollectRows(rows, func(row sqlcollect.CollectableRow) (string, error) {
    var first, last string
    if err := row.Scan(&first, &last); err != nil {
        return "", err
    }
    return first + " " + last, nil
})
```

## Struct mapping rules

- Match column names to exported Go field names, case-insensitively.
- Use `db:"column_name"` to override a field name; use `db:"-"` to ignore a field.
- Match every returned column and every eligible field exactly once. Unknown columns, missing columns, duplicate column names, and ambiguous field mappings return errors. Column order does not matter.
- Ignore unexported fields. Flatten exported anonymous embedded structs and allocate embedded pointers as needed. Recursive embedded structures return errors.
- Scan named struct fields as individual values; nested objects are not populated automatically. Fields implementing `sql.Scanner`, including embedded scanner types, use the standard scanning behavior.
- Use `sql.NullString`, other `sql.Null*` types, or pointer fields for nullable columns. Conversion behavior comes from `database/sql` and your driver.
- Pass a struct type to `RowToStructByName[T]` and `RowToAddrOfStructByName[T]`; use the latter to get pointers.

Struct metadata is cached and safe for concurrent use. Tags are literal column names; comma-separated tag options are not supported. There is no implicit snake_case conversion.

## Results and errors

`CollectRows` returns a non-nil empty slice for an empty result. On error, it returns a nil slice and discards any collected values. `CollectOneRow` returns the zero value on error.

Query errors belong to the caller. Collectors propagate callback, scan, iteration, and close errors; simultaneous callback and close errors are joined, so use `errors.Is` or `errors.As` to inspect them. `CollectOneRow` does not scan discarded rows or detect errors in those rows.

Callbacks must copy borrowed data such as `sql.RawBytes` before returning it. The collectors close rows before returning, so borrowed row memory cannot be retained.

## Development

```sh
go test -race ./...
go vet ./...
```

## License

MIT. See [LICENSE](LICENSE).

package sqlcollect_test

import (
	"database/sql"
	"fmt"

	_ "github.com/glebarez/go-sqlite"
	"github.com/raphael-foliveira/sqlcollect"
)

func ExampleCollectRows() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	type Person struct {
		ID   int    `db:"id"`
		Name string `db:"full_name"`
	}
	rows, err := db.Query(`SELECT 'Alice' AS full_name, 1 AS id UNION ALL SELECT 'Bob', 2`)
	if err != nil {
		panic(err)
	}
	people, err := sqlcollect.CollectRows(rows, sqlcollect.RowToStructByName[Person])
	if err != nil {
		panic(err)
	}
	for _, person := range people {
		fmt.Println(person.ID, person.Name)
	}
	// Output:
	// 1 Alice
	// 2 Bob
}

func ExampleCollectOneRow() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT 42`)
	if err != nil {
		panic(err)
	}
	answer, err := sqlcollect.CollectOneRow(rows, sqlcollect.RowTo[int])
	if err != nil {
		panic(err)
	}
	fmt.Println(answer)
	// Output: 42
}

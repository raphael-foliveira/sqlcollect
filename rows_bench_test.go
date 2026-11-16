package sqlcollect_test

import (
	"testing"

	"github.com/raphael-foliveira/sqlcollect"
)

// mappingRow isolates mapping and destination preparation from database work.
// Columns copies the names as database/sql.Rows.Columns does; Scan is a no-op.
type mappingRow struct {
	columns []string
}

func (r mappingRow) Columns() ([]string, error) {
	return append([]string(nil), r.columns...), nil
}

func (mappingRow) Scan(...any) error { return nil }

func benchmarkStructMapping[T any](b *testing.B, columns []string) {
	row := mappingRow{columns: columns}
	if _, err := sqlcollect.RowToStructByName[T](row); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sqlcollect.RowToStructByName[T](row); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStructMapping(b *testing.B) {
	b.Run("2Columns", func(b *testing.B) {
		type Record struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}
		benchmarkStructMapping[Record](b, []string{"id", "name"})
	})
	b.Run("10Columns", func(b *testing.B) {
		type Record struct {
			ID           int    `db:"id"`
			From         string `db:"from_location"`
			To           string `db:"to_location"`
			FlightNumber string `db:"flight_number"`
			Departure    string `db:"departure_time"`
			Arrival      string `db:"arrival_time"`
			PriceReais   int    `db:"price_reais"`
			PricePoints  int    `db:"price_points"`
			Seat         string `db:"seat"`
			PassengerID  int    `db:"passenger_id"`
		}
		benchmarkStructMapping[Record](b, []string{
			"id", "from_location", "to_location", "flight_number", "departure_time",
			"arrival_time", "price_reais", "price_points", "seat", "passenger_id",
		})
	})
}

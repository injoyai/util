package sqlite_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/injoyai/util/sqlitets"
	"github.com/injoyai/util/sqlitets/sqlite"
)

func ExampleOpen() {
	root, err := os.MkdirTemp("", "sqlitets-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)
	store, err := sqlite.Open(root, map[string]string{"readings": "sensors"})
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			panic(err)
		}
	}()
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i, temperature := range []float64{20, 24} {
		err := store.Write(sqlitets.Point{
			Measurement: "readings",
			Time:        at.Add(time.Duration(i) * time.Minute),
			Fields:      map[string]any{"temperature": temperature},
		})
		if err != nil {
			panic(err)
		}
	}
	rows, err := store.Query(ctx, sqlitets.Query{
		Selector:  sqlitets.Selector{Measurement: "readings"},
		Fields:    []string{"temperature"},
		Aggregate: sqlitets.AggregateMean,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(rows[0].Fields["temperature"])
	// Output: 22
}

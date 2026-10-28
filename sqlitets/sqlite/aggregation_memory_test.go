package sqlite

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/util/sqlitets"
)

func TestAggregatePageStopsAfterCompletedBucket(t *testing.T) {
	store, err := openTestStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: at.Add(time.Duration(i) * time.Minute), Fields: map[string]any{"value": i}}); err != nil {
			t.Fatal(err)
		}
	}
	err = store.shards.WithDB(ctx, "sensors", at, func(db *sql.DB) error {
		_, err := db.Exec("INSERT INTO points VALUES(?,?)", at.Add(3*time.Minute).UnixNano(), "invalid-json")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	q := sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, Fields: []string{"value"}, Aggregate: sqlitets.AggregateMean, Interval: time.Minute, Offset: 1, Limit: 1}
	rows, err := store.Query(ctx, q)
	if err != nil || len(rows) != 1 || rows[0].Fields["value"] != float64(1) {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	q.Limit = 0
	q.Offset = 0
	if _, err := store.Query(ctx, q); err == nil {
		t.Fatal("unbounded query did not visit invalid row")
	}
}

func TestSQLCountsMatchStreamingCounts(t *testing.T) {
	store, err := openTestStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := context.Background()
	at := time.Date(2026, 1, 31, 23, 57, 0, 0, time.UTC)
	key := `field".[]`
	for i, value := range []any{2, nil, "3", true, 4, 0, 5.5} {
		if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: at.Add(time.Duration(i) * time.Minute), Fields: map[string]any{key: value, "group": 1}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, desc := range []bool{false, true} {
		for _, interval := range []time.Duration{0, time.Minute, 5 * time.Minute, 7 * 24 * time.Hour} {
			for _, offset := range []int{0, 1, 100} {
				q := sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors", Start: at.Add(-time.Second), End: at.Add(7 * time.Minute)}, Fields: []string{key}, Aggregate: sqlitets.AggregateCount, Interval: interval, Desc: desc, Limit: 2, Offset: offset}
				got, err := store.Query(ctx, q)
				if err != nil {
					t.Fatal(err)
				}
				q.Filters = []sqlitets.Filter{{Field: "group", Op: sqlitets.FilterEqual, Value: 1}}
				want, err := store.Query(ctx, q)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("desc=%v interval=%v offset=%d got=%v want=%v", desc, interval, offset, got, want)
				}
			}
		}
	}
}

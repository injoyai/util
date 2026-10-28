package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/util/sqlitets"
)

func TestPageStopsBeforeUnneededRows(t *testing.T) {
	store, err := openTestStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: at, Fields: map[string]any{"value": 1}}); err != nil {
		t.Fatal(err)
	}
	err = store.shards.WithDB(ctx, "sensors", at, func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, "INSERT INTO points(time_ns,fields_json) VALUES(?,?)", at.Add(time.Second).UnixNano(), "invalid-json")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	query := sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, Fields: []string{"value"}, Limit: 1}
	rows, err := store.Query(ctx, query)
	if err != nil || len(rows) != 1 || rows[0].Fields["value"] != json.Number("1") {
		t.Fatalf("page must not decode rows beyond limit: rows=%v err=%v", rows, err)
	}
	query.Limit = 0
	if _, err := store.Query(ctx, query); err == nil {
		t.Fatal("unbounded query must encounter the invalid second row")
	}
}

func BenchmarkHistoryAggregation(b *testing.B) {
	store, err := openTestStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	err = store.shards.WithDB(ctx, "sensors", at, func(db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		stmt, err := tx.Prepare("INSERT INTO points(time_ns,fields_json) VALUES(?,?)")
		if err != nil {
			return err
		}
		defer stmt.Close()
		payload := `{"value":20,"_count_":1,"unused":"` + strings.Repeat("x", 4096) + `"}`
		for i := 0; i < 10000; i++ {
			if _, err := stmt.Exec(at.Add(time.Duration(i)*time.Minute).UnixNano(), payload); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if err != nil {
		b.Fatal(err)
	}
	for _, tc := range []struct {
		name, field string
		op          sqlitets.Aggregate
		interval    time.Duration
		limit       int
	}{
		{"paged_mean", "value", sqlitets.AggregateMean, time.Minute, 20},
		{"daily_count", "_count_", sqlitets.AggregateCount, 24 * time.Hour, 0},
	} {
		b.Run(tc.name, func(b *testing.B) {
			query := sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, Fields: []string{tc.field}, Aggregate: tc.op, Interval: tc.interval, Limit: tc.limit}
			b.ReportAllocs()
			for b.Loop() {
				rows, err := store.Query(ctx, query)
				if err != nil || len(rows) == 0 {
					b.Fatalf("rows=%d err=%v", len(rows), err)
				}
			}
		})
	}
}

func TestStreamingQueriesPreserveCrossMonthFilteringAndAggregation(t *testing.T) {
	store, err := openTestStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	at := []time.Time{
		time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 1, 13, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
	weirdKey := `field".[]`
	for i, ts := range at {
		fields := map[string]any{"value": i + 1, "group": i % 2, weirdKey: i}
		if i == 2 {
			fields[weirdKey] = nil
		}
		if err := store.Write(sqlitets.Point{Measurement: "custom", Time: ts, Fields: fields}); err != nil {
			t.Fatal(err)
		}
	}
	selector := sqlitets.Selector{Measurement: "custom"}
	rows, err := store.Query(ctx, sqlitets.Query{Selector: selector, Fields: []string{"value"}, Desc: true, Limit: 1, Offset: 2})
	if err != nil || len(rows) != 1 || !rows[0].Time.Equal(at[1]) || len(rows[0].Fields) != 1 {
		t.Fatalf("cross-month page=%v err=%v", rows, err)
	}
	selector.At = []time.Time{at[0], at[2], at[3]}
	selector.Filters = []sqlitets.Filter{{Field: "group", Op: sqlitets.FilterEqual, Value: 0}}
	rows, err = store.Query(ctx, sqlitets.Query{Selector: selector, AllFields: true, Limit: 1, Offset: 1})
	if err != nil || len(rows) != 1 || !rows[0].Time.Equal(at[2]) {
		t.Fatalf("filtered exact page=%v err=%v", rows, err)
	}
	count, err := store.Count(ctx, sqlitets.CountQuery{Selector: selector, Field: "value"})
	if err != nil || count != 2 {
		t.Fatalf("filtered count=%d err=%v", count, err)
	}
	selector = sqlitets.Selector{Measurement: "custom", Start: at[0], End: at[3]}
	count, err = store.Count(ctx, sqlitets.CountQuery{Selector: selector, Field: weirdKey})
	if err != nil || count != 1 {
		t.Fatalf("SQL count bounds/null/key handling=%d err=%v", count, err)
	}
	selector = sqlitets.Selector{Measurement: "custom"}
	for _, tc := range []struct {
		op   sqlitets.Aggregate
		want any
	}{
		{sqlitets.AggregateMean, 2.5}, {sqlitets.AggregateMin, 2.0}, {sqlitets.AggregateMax, 3.0},
		{sqlitets.AggregateSum, 5.0}, {sqlitets.AggregateCount, int64(2)},
	} {
		rows, err = store.Query(ctx, sqlitets.Query{Selector: selector, Fields: []string{"value"}, Aggregate: tc.op, Interval: 24 * time.Hour, Desc: true, Offset: 1, Limit: 1})
		if err != nil || len(rows) != 1 || rows[0].Fields["value"] != tc.want {
			t.Fatalf("aggregate %d page=%v err=%v", tc.op, rows, err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Query(cancelled, sqlitets.Query{Selector: selector, AllFields: true, Limit: 1}); err != context.Canceled {
		t.Fatalf("cancellation=%v", err)
	}
}

func BenchmarkPagedHistory(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			store, err := openTestStore(b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = store.Close() })
			ctx := context.Background()
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			err = store.shards.WithDB(ctx, "sensors", at, func(db *sql.DB) error {
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
				defer tx.Rollback()
				stmt, err := tx.PrepareContext(ctx, "INSERT INTO points(time_ns,fields_json) VALUES(?,?)")
				if err != nil {
					return err
				}
				defer stmt.Close()
				for i := 0; i < n; i++ {
					if _, err := stmt.ExecContext(ctx, at.Add(time.Duration(i)*time.Second).UnixNano(), `{"value":20}`); err != nil {
						return err
					}
				}
				return tx.Commit()
			})
			if err != nil {
				b.Fatal(err)
			}
			query := sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, Fields: []string{"value"}, Limit: 1}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				rows, err := store.Query(ctx, query)
				if err != nil || len(rows) != 1 {
					b.Fatalf("rows=%d err=%v", len(rows), err)
				}
			}
		})
	}
}

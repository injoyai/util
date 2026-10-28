package sqlite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/injoyai/util/sqlitets"
)

func openTestStore(root string) (*Store, error) {
	return Open(root, map[string]string{
		"sensors": "sensors",
		"metrics": "metrics",
		"custom":  "custom",
	})
}

func TestStoreShardsQueriesAndDeletesHistory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "history")
	store, err := openTestStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	jan := time.Date(2026, 1, 31, 23, 0, 0, 0, time.UTC)
	febEarly := time.Date(2026, 2, 1, 1, 0, 0, 0, time.UTC)
	febLate := time.Date(2026, 2, 20, 1, 0, 0, 0, time.UTC)
	for _, point := range []sqlitets.Point{
		{Measurement: "sensors", Time: jan, Fields: map[string]any{"temperature": 10.0, "_count_": 1}},
		{Measurement: "sensors", Time: febEarly, Fields: map[string]any{"temperature": 20.0, "_count_": 1}},
		{Measurement: "sensors", Time: febLate, Fields: map[string]any{"temperature": 30.0, "_count_": 1}},
		{Measurement: "sensors", Time: febLate, Fields: map[string]any{"humidity": 50.0}},
	} {
		if err := store.Write(point); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "sensors", "2026-01.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sensors", "2026-02.db")); err != nil {
		t.Fatal(err)
	}
	rows, err := store.Query(ctx, sqlitets.Query{
		Selector: sqlitets.Selector{Measurement: "sensors"}, AllFields: true, Desc: true, Limit: 1, Offset: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].Time.Equal(febEarly) {
		t.Fatalf("paged rows=%#v", rows)
	}
	merged, err := store.Query(ctx, sqlitets.Query{
		Selector: sqlitets.Selector{Measurement: "sensors", Start: febEarly}, AllFields: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 || merged[0].Fields["temperature"] != json.Number("30") || merged[0].Fields["humidity"] != json.Number("50") {
		t.Fatalf("merged row=%#v", merged)
	}
	if err := store.Delete(ctx, sqlitets.DeleteQuery{Selector: sqlitets.Selector{
		Measurement: "sensors", End: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sensors", "2026-01.db")); !os.IsNotExist(err) {
		t.Fatalf("expired January shard still exists: %v", err)
	}
	count, err := store.Count(ctx, sqlitets.CountQuery{
		Selector: sqlitets.Selector{Measurement: "sensors"}, Field: "temperature",
	})
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestStoreUsesUTCMonthsAndRejectsUnknownMeasurements(t *testing.T) {
	root := filepath.Join(t.TempDir(), "history")
	store, err := openTestStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	shanghai := time.FixedZone("CST", 8*60*60)
	at := time.Date(2026, 3, 1, 1, 0, 0, 0, shanghai)
	if err := store.Write(sqlitets.Point{Measurement: "metrics", Time: at, Fields: map[string]any{"cpu": 1.0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "metrics", "2026-02.db")); err != nil {
		t.Fatal(err)
	}
	for _, measurement := range []string{"../escape", "unregistered"} {
		if err := store.Write(sqlitets.Point{Measurement: measurement, Time: at, Fields: map[string]any{"value": 1}}); err == nil {
			t.Fatalf("unknown measurement %q accepted", measurement)
		}
	}
}

func TestStoreAggregatesAcrossMonthBoundary(t *testing.T) {
	store, err := openTestStore(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	for _, point := range []sqlitets.Point{
		{Measurement: "custom", Time: time.Date(2026, 1, 31, 23, 30, 0, 0, time.UTC), Fields: map[string]any{"x": 2.0}},
		{Measurement: "custom", Time: time.Date(2026, 2, 1, 0, 30, 0, 0, time.UTC), Fields: map[string]any{"x": 4.0}},
	} {
		if err := store.Write(point); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.Query(ctx, sqlitets.Query{
		Selector: sqlitets.Selector{Measurement: "custom"}, Fields: []string{"x"},
		Aggregate: sqlitets.AggregateMean,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Fields["x"] != 3.0 {
		t.Fatalf("aggregate rows=%#v", rows)
	}
}

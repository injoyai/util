package sqlite

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/injoyai/util/sqlitets"
)

func TestBoundedOperationsSkipUnrelatedShards(t *testing.T) {
	root := t.TempDir()
	s, err := openTestStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := s.Write(sqlitets.Point{Measurement: "sensors", Time: at, Fields: map[string]any{"value": 42}}); err != nil {
		t.Fatal(err)
	}
	for _, month := range []string{"2020-01", "2030-01"} {
		if err := os.WriteFile(filepath.Join(root, "sensors", month+".db"), []byte("invalid database"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, selector := range []sqlitets.Selector{
		{Measurement: "sensors", Start: at.Add(-time.Second), End: at.Add(time.Second)},
		{Measurement: "sensors", At: []time.Time{at}},
	} {
		for _, desc := range []bool{false, true} {
			for _, aggregate := range []sqlitets.Aggregate{sqlitets.AggregateNone, sqlitets.AggregateCount, sqlitets.AggregateMean} {
				rows, err := s.Query(ctx, sqlitets.Query{Selector: selector, Fields: []string{"value"}, Desc: desc, Aggregate: aggregate})
				if err != nil || len(rows) != 1 {
					t.Fatalf("bounded query opened unrelated shard: aggregate=%d desc=%v rows=%v err=%v", aggregate, desc, rows, err)
				}
			}
		}
		if count, err := s.Count(ctx, sqlitets.CountQuery{Selector: selector, Field: "value"}); err != nil || count != 1 {
			t.Fatalf("bounded count=%d err=%v", count, err)
		}
	}
	selector := sqlitets.Selector{Measurement: "sensors", Start: at.Add(-time.Second), End: at.Add(time.Second)}
	if err := s.Delete(ctx, sqlitets.DeleteQuery{Selector: selector}); err != nil {
		t.Fatal("bounded delete opened unrelated shard", err)
	}
	if count, err := s.Count(ctx, sqlitets.CountQuery{Selector: selector, Field: "value"}); err != nil || count != 0 {
		t.Fatalf("delete did not remove selected point: count=%d err=%v", count, err)
	}
	for _, month := range []string{"2020-01", "2030-01"} {
		data, err := os.ReadFile(filepath.Join(root, "sensors", month+".db"))
		if err != nil || string(data) != "invalid database" {
			t.Fatal("unrelated shard was modified", err)
		}
	}
	if _, err := s.Query(ctx, sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, AllFields: true}); err == nil {
		t.Fatal("unbounded query must still report corrupt shards")
	}
}

func TestTimestampRangeValidation(t *testing.T) {
	root := t.TempDir()
	s, err := openTestStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	minimum, maximum := time.Unix(0, math.MinInt64), time.Unix(0, math.MaxInt64)
	for _, at := range []time.Time{minimum.Add(-time.Nanosecond), maximum.Add(time.Nanosecond), time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC)} {
		if err := s.Write(sqlitets.Point{Measurement: "sensors", Time: at, Fields: map[string]any{"value": 1}}); err == nil {
			t.Fatal("unrepresentable timestamp accepted", at)
		}
		for _, selector := range []sqlitets.Selector{
			{Measurement: "sensors", Start: at},
			{Measurement: "sensors", End: at},
			{Measurement: "sensors", At: []time.Time{at}},
		} {
			if _, err := s.Query(ctx, sqlitets.Query{Selector: selector, AllFields: true}); err == nil {
				t.Fatal("invalid query timestamp accepted", selector)
			}
			if _, err := s.Count(ctx, sqlitets.CountQuery{Selector: selector, Field: "value"}); err == nil {
				t.Fatal("invalid count timestamp accepted", selector)
			}
			if err := s.Delete(ctx, sqlitets.DeleteQuery{Selector: selector}); err == nil {
				t.Fatal("invalid delete timestamp accepted", selector)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, "sensors")); !os.IsNotExist(err) {
		t.Fatal("invalid timestamps created shard files", err)
	}
	for _, at := range []time.Time{minimum, maximum} {
		if err := s.Write(sqlitets.Point{Measurement: "sensors", Time: at, Fields: map[string]any{"value": 1}}); err != nil {
			t.Fatal("valid boundary timestamp rejected", at, err)
		}
		rows, err := s.Query(ctx, sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors", At: []time.Time{at}}, AllFields: true})
		if err != nil || len(rows) != 1 || !rows[0].Time.Equal(at) {
			t.Fatal("boundary timestamp did not round-trip", rows, err)
		}
	}
}

package sqlite

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/injoyai/util/sqlitets"
)

func TestSelectorsFailBeforeScanning(t *testing.T) {
	store, err := openTestStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, populated := range []bool{false, true} {
		if populated {
			if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: at, Fields: map[string]any{"value": 1}}); err != nil {
				t.Fatal(err)
			}
		}
		for _, selector := range []sqlitets.Selector{
			{Start: at, End: at}, {Start: at.Add(time.Hour), End: at},
			{At: []time.Time{{}}},
			{Filters: []sqlitets.Filter{{Field: "", Op: sqlitets.FilterEqual, Value: 1}}},
			{Filters: []sqlitets.Filter{{Field: "missing", Op: sqlitets.FilterOp(255), Value: 1}}},
			{Filters: []sqlitets.Filter{{Field: "missing", Op: sqlitets.FilterContains, Value: 1}}},
			{Filters: []sqlitets.Filter{{Field: "missing", Op: sqlitets.FilterIn, Value: 1}}},
			{Filters: []sqlitets.Filter{{Field: "missing", Op: sqlitets.FilterIn, Value: []int(nil)}}},
			{Filters: []sqlitets.Filter{{Field: "missing", Op: sqlitets.FilterEqual, Value: math.NaN()}}},
		} {
			selector.Measurement = "sensors"
			if _, err := store.Query(context.Background(), sqlitets.Query{Selector: selector, AllFields: true}); err == nil {
				t.Errorf("populated=%v query accepted %+v", populated, selector)
			}
			if _, err := store.Count(context.Background(), sqlitets.CountQuery{Selector: selector, Field: "value"}); err == nil {
				t.Errorf("populated=%v count accepted %+v", populated, selector)
			}
			if err := store.Delete(context.Background(), sqlitets.DeleteQuery{Selector: selector}); err == nil {
				t.Errorf("populated=%v delete accepted %+v", populated, selector)
			}
		}
	}
	count, err := store.Count(context.Background(), sqlitets.CountQuery{Selector: sqlitets.Selector{Measurement: "sensors"}, Field: "value"})
	if err != nil || count != 1 {
		t.Fatalf("invalid deletion changed data: count=%d err=%v", count, err)
	}
}

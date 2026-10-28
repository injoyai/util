package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/util/sqlitets"
)

func TestJSONNumbersSurviveMergeAndReopen(t *testing.T) {
	root := t.TempDir()
	store, err := openTestStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fields := map[string]any{
		"signed":   int64(9007199254740993),
		"unsigned": uint64(math.MaxUint64),
		"decimal":  json.Number("1.234567890123456789"),
		"nested":   []any{map[string]any{"id": int64(math.MaxInt64)}},
		"text":     "hello", "flag": true, "null": nil,
	}
	ctx := context.Background()
	if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: at, Fields: fields}); err != nil {
		t.Fatal(err)
	}
	if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: at, Fields: map[string]any{"extra": "value"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openTestStore(root)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.Query(ctx, sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, AllFields: true})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	want := map[string]any{
		"signed":   json.Number("9007199254740993"),
		"unsigned": json.Number("18446744073709551615"),
		"decimal":  json.Number("1.234567890123456789"),
		"nested":   []any{map[string]any{"id": json.Number("9223372036854775807")}},
		"text":     "hello", "flag": true, "null": nil, "extra": "value",
	}
	if !reflect.DeepEqual(rows[0].Fields, want) {
		t.Fatalf("got=%#v want=%#v", rows[0].Fields, want)
	}
}

func TestFiltersUseJSONValueSemantics(t *testing.T) {
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
	fields := map[string]any{"small": uint64(7), "big": int64(9007199254740993), "array": []int{1, 2}, "object": map[string]any{"n": 1}, "flag": true, "null": nil}
	if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: at, Fields: fields}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, field string
		op          sqlitets.FilterOp
		value       any
		want        int
	}{
		{"unsigned", "small", sqlitets.FilterEqual, uint64(7), 1},
		{"decimal spelling", "small", sqlitets.FilterEqual, json.Number("7.0e0"), 1},
		{"integer exact", "big", sqlitets.FilterEqual, uint64(9007199254740993), 1},
		{"neighbor differs", "big", sqlitets.FilterEqual, int64(9007199254740992), 0},
		{"numeric string differs", "small", sqlitets.FilterEqual, "7", 0},
		{"array", "array", sqlitets.FilterEqual, []uint32{1, 2}, 1},
		{"object", "object", sqlitets.FilterEqual, map[string]int{"n": 1}, 1},
		{"struct pointer", "object", sqlitets.FilterEqual, &struct {
			N int `json:"n"`
		}{N: 1}, 1},
		{"in", "big", sqlitets.FilterIn, []uint64{1, 9007199254740993}, 1},
		{"empty in", "big", sqlitets.FilterIn, []int{}, 0},
		{"boolean", "flag", sqlitets.FilterEqual, true, 1},
		{"null", "null", sqlitets.FilterEqual, nil, 1},
		{"missing differs from null", "missing", sqlitets.FilterEqual, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selector := sqlitets.Selector{Measurement: "sensors", Filters: []sqlitets.Filter{{Field: tc.field, Op: tc.op, Value: tc.value}}}
			rows, err := store.Query(ctx, sqlitets.Query{Selector: selector, AllFields: true})
			if err != nil || len(rows) != tc.want {
				t.Fatalf("rows=%v err=%v want=%d", rows, err, tc.want)
			}
			count, err := store.Count(ctx, sqlitets.CountQuery{Selector: selector, Field: "small"})
			if err != nil || count != int64(tc.want) {
				t.Fatalf("count=%d err=%v", count, err)
			}
			if !reflect.DeepEqual(selector.Filters[0].Value, tc.value) {
				t.Fatal("query mutated caller's filter")
			}
		})
	}
	if err := store.Delete(ctx, sqlitets.DeleteQuery{Selector: sqlitets.Selector{Measurement: "sensors", Filters: []sqlitets.Filter{{Field: "big", Op: sqlitets.FilterEqual, Value: uint64(9007199254740993)}}}}); err != nil {
		t.Fatal(err)
	}
	count, err := store.Count(ctx, sqlitets.CountQuery{Selector: sqlitets.Selector{Measurement: "sensors"}, Field: "small"})
	if err != nil || count != 0 {
		t.Fatalf("filtered deletion count=%d err=%v", count, err)
	}
}

func TestDecodeFieldsRoundTrip(t *testing.T) {
	store, err := openTestStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	type Payload struct {
		ID       int64     `json:"id"`
		Sequence uint64    `json:"sequence"`
		At       time.Time `json:"at"`
		Bytes    []byte    `json:"bytes"`
	}
	want := Payload{ID: math.MaxInt64, Sequence: math.MaxUint64, At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Bytes: []byte{0, 1, 2, 255}}
	if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: want.At, Fields: map[string]any{"payload": want}}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.Query(context.Background(), sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, AllFields: true})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	var result struct {
		Payload Payload `json:"payload"`
	}
	if err := rows[0].DecodeFields(&result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Payload, want) {
		t.Fatalf("got=%#v want=%#v", result.Payload, want)
	}
	var generic map[string]any
	if err := rows[0].DecodeFields(&generic); err != nil {
		t.Fatal(err)
	}
	if generic["payload"].(map[string]any)["sequence"] != json.Number("18446744073709551615") {
		t.Fatal("decoding an interface rounded the number")
	}
	var narrow struct {
		Payload struct {
			ID int32 `json:"id"`
		} `json:"payload"`
	}
	if err := rows[0].DecodeFields(&narrow); err == nil {
		t.Fatal("integer overflow silently accepted")
	}
	if err := rows[0].DecodeFields(result); err == nil {
		t.Fatal("non-pointer decode target accepted")
	}
	if err := rows[0].DecodeFields(nil); err == nil {
		t.Fatal("nil decode target accepted")
	}
}

func TestLargeNumbersCountAndAggregateRange(t *testing.T) {
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
	for i := 0; i < 2; i++ {
		if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: at.Add(time.Duration(i) * time.Second), Fields: map[string]any{"huge": json.Number("1e400"), "large": 1e308, "group": true}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, filtered := range []bool{false, true} {
		q := sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, Fields: []string{"huge"}, Aggregate: sqlitets.AggregateCount}
		if filtered {
			q.Filters = []sqlitets.Filter{{Field: "group", Op: sqlitets.FilterEqual, Value: true}}
		}
		rows, err := store.Query(ctx, q)
		if err != nil || len(rows) != 1 || rows[0].Fields["huge"] != int64(2) {
			t.Fatalf("filtered=%v rows=%v err=%v", filtered, rows, err)
		}
	}
	for _, op := range []sqlitets.Aggregate{sqlitets.AggregateMean, sqlitets.AggregateSum, sqlitets.AggregateMin, sqlitets.AggregateMax} {
		_, err := store.Query(ctx, sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, Fields: []string{"huge"}, Aggregate: op})
		if err == nil {
			t.Fatalf("operation %v silently accepted float64 overflow", op)
		}
	}
	if _, err := store.Query(ctx, sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, Fields: []string{"large"}, Aggregate: sqlitets.AggregateSum}); err == nil {
		t.Fatal("sum overflow accepted")
	}
	rows, err := store.Query(ctx, sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, Fields: []string{"large"}, Aggregate: sqlitets.AggregateMax})
	if err != nil || len(rows) != 1 || rows[0].Fields["large"] != 1e308 {
		t.Fatalf("max rows=%v err=%v", rows, err)
	}
}

func TestMalformedStoredFieldsAreRejected(t *testing.T) {
	for _, encoded := range []string{"null", "[]", `{"value":1} true`} {
		t.Run(encoded, func(t *testing.T) {
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
			if err := store.shards.WithDB(ctx, "sensors", at, func(db *sql.DB) error {
				_, err := db.Exec("INSERT INTO points VALUES (?,?)", at.UnixNano(), encoded)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Query(ctx, sqlitets.Query{Selector: sqlitets.Selector{Measurement: "sensors"}, AllFields: true}); err == nil {
				t.Fatal("invalid stored fields accepted")
			}
			if err := store.Write(sqlitets.Point{Measurement: "sensors", Time: at, Fields: map[string]any{"extra": true}}); err == nil {
				t.Fatal("invalid stored fields overwritten")
			}
		})
	}
}

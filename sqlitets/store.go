package sqlitets

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

// Store is the contract for dynamic JSON history. Fixed-schema operational
// records use the monthly record store instead of this map-based boundary.
type Store interface {
	Write(Point) error
	WriteContext(context.Context, Point) error
	Query(context.Context, Query) ([]Row, error)
	Count(context.Context, CountQuery) (int64, error)
	Delete(context.Context, DeleteQuery) error
	Close() error
}

type Point struct {
	Measurement string
	Time        time.Time
	Fields      map[string]any
}

type Row struct {
	Time time.Time
	// Raw numeric values, including nested values, are json.Number.
	// Numeric aggregates return float64; aggregate counts return int64.
	Fields map[string]any
}

// DecodeFields decodes fields into a caller-supplied JSON target, such as a
// pointer to a struct. Integer overflow and incompatible types return errors.
// Numbers in interface fields remain json.Number. Time is not included;
// it remains available through Row.Time.
func (row Row) DecodeFields(target any) error {
	data, err := json.Marshal(row.Fields)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(target)
}

type Selector struct {
	Measurement string
	// Start and End are exclusive bounds. At selects exact timestamps.
	Start   time.Time
	End     time.Time
	At      []time.Time
	Filters []Filter
}

type Query struct {
	Selector
	Fields    []string
	AllFields bool
	Aggregate Aggregate
	Interval  time.Duration
	Desc      bool
	Limit     int
	Offset    int
}

func RowsToMaps(rows []Row) []map[string]any {
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := make(map[string]any, len(row.Fields)+1)
		for key, value := range row.Fields {
			item[key] = value
		}
		item["time"] = row.Time.UnixNano()
		result = append(result, item)
	}
	return result
}

type CountQuery struct {
	Selector
	Field string
}

type DeleteQuery struct {
	Selector
}

type Filter struct {
	Field string
	Op    FilterOp
	Value any
}

type FilterOp uint8

const (
	FilterEqual FilterOp = iota
	FilterContains
	FilterIn
)

type Aggregate uint8

const (
	AggregateNone Aggregate = iota
	AggregateMean
	AggregateMax
	AggregateMin
	AggregateSum
	AggregateCount
)

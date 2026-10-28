package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/injoyai/util/sqlitets"
	"github.com/injoyai/util/sqlitets/monthly"
)

const pointsSchema = `
CREATE TABLE IF NOT EXISTS points (
    time_ns INTEGER PRIMARY KEY,
    fields_json TEXT NOT NULL
) WITHOUT ROWID;`

type Store struct {
	shards       *monthly.Store
	measurements map[string]string
	owned        bool
}

var _ sqlitets.Store = (*Store)(nil)

// Open owns the monthly store. Measurements maps logical names to unique shard
// directories matching [a-z][a-z0-9-]*; the mapping is copied before use.
func Open(root string, measurements map[string]string) (*Store, error) {
	shards, err := monthly.New(root)
	if err != nil {
		return nil, err
	}
	store, err := Use(shards, measurements)
	if err != nil {
		_ = shards.Close()
		return nil, err
	}
	store.owned = true
	return store, nil
}

// Use borrows a monthly store. Its owner must close it after all consumers stop.
func Use(shards *monthly.Store, measurements map[string]string) (*Store, error) {
	if shards == nil {
		return nil, errors.New("monthly SQLite store is nil")
	}
	if len(measurements) == 0 {
		return nil, errors.New("time-series measurements are empty")
	}
	names := make(map[string]string, len(measurements))
	kinds := make(map[string]struct{}, len(measurements))
	for name, kind := range measurements {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("time-series measurement is empty")
		}
		if _, exists := kinds[kind]; exists {
			return nil, fmt.Errorf("time-series shard kind %q is shared by multiple measurements", kind)
		}
		names[name] = kind
		kinds[kind] = struct{}{}
	}
	for _, kind := range names {
		if err := shards.Register(kind, monthly.Spec{Schema: pointsSchema, Table: "points", TimeColumn: "time_ns"}); err != nil {
			return nil, err
		}
	}
	return &Store{shards: shards, measurements: names}, nil
}

// Write merges fields at the given time using a background context.
func (s *Store) Write(point sqlitets.Point) error {
	return s.WriteContext(context.Background(), point)
}

// WriteContext is Write with cancellation and deadline support.
func (s *Store) WriteContext(ctx context.Context, point sqlitets.Point) error {
	kind, err := s.kindFor(point.Measurement)
	if err != nil {
		return err
	}
	if point.Time.IsZero() {
		return errors.New("time-series point time is zero")
	}
	if err := validateTimestamp(point.Time); err != nil {
		return err
	}
	if len(point.Fields) == 0 {
		return errors.New("time-series fields are empty")
	}
	encodedFields, err := json.Marshal(point.Fields)
	if err != nil {
		return fmt.Errorf("encode time-series fields: %w", err)
	}
	incoming, err := decodeFields(encodedFields)
	if err != nil {
		return fmt.Errorf("decode time-series fields: %w", err)
	}
	return s.shards.WithDB(ctx, kind, point.Time, func(db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		merged := map[string]any{}
		var encoded string
		err = tx.QueryRowContext(ctx, "SELECT fields_json FROM points WHERE time_ns = ?", point.Time.UnixNano()).Scan(&encoded)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			merged, err = decodeFields([]byte(encoded))
			if err != nil {
				return err
			}
		}
		for key, value := range incoming {
			merged[key] = value
		}
		data, err := json.Marshal(merged)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO points(time_ns, fields_json) VALUES(?, ?)", point.Time.UnixNano(), string(data)); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// errPageComplete stops both the active cursor and subsequent month shards.
var errPageComplete = errors.New("time-series page complete")

func (s *Store) Query(ctx context.Context, query sqlitets.Query) ([]sqlitets.Row, error) {
	if err := s.validateQuery(&query); err != nil {
		return nil, err
	}
	rows := []sqlitets.Row{}
	offset := query.Offset
	appendRow := func(row sqlitets.Row) error {
		if offset > 0 {
			offset--
			return nil
		}
		rows = append(rows, row)
		if query.Limit > 0 && len(rows) == query.Limit {
			return errPageComplete
		}
		return nil
	}
	var bucket *aggregateBucket
	counts := query.Aggregate == sqlitets.AggregateCount && len(query.Fields) == 1 && len(query.Filters) == 0 && len(query.At) == 0
	visit := func(row sqlitets.Row) error {
		if query.Aggregate != sqlitets.AggregateNone {
			key := bucketTime(row.Time.UnixNano(), query.Interval)
			if bucket != nil && bucket.time != key {
				if err := appendRow(bucket.row(query.Aggregate)); err != nil {
					return err
				}
				bucket = nil
			}
			if bucket == nil {
				bucket = &aggregateBucket{time: key, fields: map[string]*aggregateState{}}
			}
			return bucket.add(row, query.Fields, query.Aggregate, counts)
		}
		if !query.AllFields {
			projected := make(map[string]any, len(query.Fields))
			for _, field := range query.Fields {
				if value, ok := row.Fields[field]; ok {
					projected[field] = value
				}
			}
			row.Fields = projected
		}
		return appendRow(row)
	}
	var err error
	if counts {
		err = s.walkCounts(ctx, query, visit)
	} else {
		err = s.walkRows(ctx, query.Selector, query.Desc, visit)
	}
	if err != nil && !errors.Is(err, errPageComplete) {
		return nil, err
	}
	if err == nil && bucket != nil {
		_ = appendRow(bucket.row(query.Aggregate))
	}
	return rows, nil
}

func (s *Store) Count(ctx context.Context, query sqlitets.CountQuery) (int64, error) {
	if query.Field == "" {
		return 0, errors.New("time-series count field is empty")
	}
	_, err := s.prepareSelector(&query.Selector)
	if err != nil {
		return 0, err
	}
	var count int64
	if len(query.Filters) == 0 && len(query.At) == 0 {
		// Count in SQLite without allocating a Go map for every historical row.
		err = s.walkShards(ctx, query.Selector, false, func(shard monthly.Shard) error {
			conditions, args := timeConditions(query.Selector)
			conditions = append(conditions, "EXISTS (SELECT 1 FROM json_each(points.fields_json) WHERE key = ? AND type <> 'null')")
			args = append(args, query.Field)
			var shardCount int64
			if err := shard.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM points WHERE "+strings.Join(conditions, " AND "), args...).Scan(&shardCount); err != nil {
				return err
			}
			count += shardCount
			return nil
		})
	} else {
		err = s.walkRows(ctx, query.Selector, false, func(row sqlitets.Row) error {
			if value, ok := row.Fields[query.Field]; ok && value != nil {
				count++
			}
			return nil
		})
	}
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) Delete(ctx context.Context, query sqlitets.DeleteQuery) error {
	kind, err := s.prepareSelector(&query.Selector)
	if err != nil {
		return err
	}
	selector := query.Selector
	if selector.Start.IsZero() && selector.End.IsZero() && len(selector.At) == 0 && len(selector.Filters) == 0 {
		return errors.New("refusing to delete all time-series data without a selector")
	}
	if selector.Start.IsZero() && !selector.End.IsZero() && len(selector.At) == 0 && len(selector.Filters) == 0 {
		return s.shards.DeleteBefore(ctx, kind, selector.End)
	}
	return s.walkShards(ctx, selector, false, func(shard monthly.Shard) error {
		rows, err := selectFromDB(ctx, shard.DB, selector, false)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		tx, err := shard.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, row := range rows {
			if _, err := tx.ExecContext(ctx, "DELETE FROM points WHERE time_ns = ?", row.Time.UnixNano()); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func (s *Store) Close() error {
	if !s.owned {
		return nil
	}
	return s.shards.Close()
}

func (s *Store) prepareSelector(selector *sqlitets.Selector) (string, error) {
	kind, err := s.kindFor(selector.Measurement)
	if err != nil {
		return "", err
	}
	for _, bound := range []time.Time{selector.Start, selector.End} {
		if !bound.IsZero() {
			if err := validateTimestamp(bound); err != nil {
				return "", err
			}
		}
	}
	if !selector.Start.IsZero() && !selector.End.IsZero() && !selector.Start.Before(selector.End) {
		return "", errors.New("time-series start must be before end")
	}
	for _, at := range selector.At {
		if at.IsZero() {
			return "", errors.New("time-series exact time is zero")
		}
		if err := validateTimestamp(at); err != nil {
			return "", err
		}
	}
	filters, err := prepareFilters(selector.Filters)
	if err != nil {
		return "", err
	}
	selector.Filters = filters
	return kind, nil
}

func validateTimestamp(at time.Time) error {
	if !time.Unix(0, at.UnixNano()).Equal(at) {
		return errors.New("time-series time is outside the Unix nanosecond range")
	}
	return nil
}

func (s *Store) walkShards(ctx context.Context, selector sqlitets.Selector, desc bool, visit func(monthly.Shard) error) error {
	kind, err := s.kindFor(selector.Measurement)
	if err != nil {
		return err
	}
	return s.shards.WalkFiltered(ctx, kind, desc, func(start time.Time) bool {
		return shardOverlaps(start, selector)
	}, visit)
}

func (s *Store) walkRows(ctx context.Context, selector sqlitets.Selector, desc bool, visit func(sqlitets.Row) error) error {
	return s.walkShards(ctx, selector, desc, func(shard monthly.Shard) error {
		return scanFromDB(ctx, shard.DB, selector, desc, visit)
	})
}

func timeConditions(selector sqlitets.Selector) ([]string, []any) {
	conditions := []string{}
	args := []any{}
	if !selector.Start.IsZero() {
		conditions = append(conditions, "time_ns > ?")
		args = append(args, selector.Start.UnixNano())
	}
	if !selector.End.IsZero() {
		conditions = append(conditions, "time_ns < ?")
		args = append(args, selector.End.UnixNano())
	}
	return conditions, args
}

func selectFromDB(ctx context.Context, db *sql.DB, selector sqlitets.Selector, desc bool) ([]sqlitets.Row, error) {
	result := []sqlitets.Row{}
	err := scanFromDB(ctx, db, selector, desc, func(row sqlitets.Row) error {
		result = append(result, row)
		return nil
	})
	return result, err
}

func scanFromDB(ctx context.Context, db *sql.DB, selector sqlitets.Selector, desc bool, visit func(sqlitets.Row) error) error {
	conditions, args := timeConditions(selector)
	statement := "SELECT time_ns, fields_json FROM points"
	if len(conditions) > 0 {
		statement += " WHERE " + strings.Join(conditions, " AND ")
	}
	if desc {
		statement += " ORDER BY time_ns DESC"
	} else {
		statement += " ORDER BY time_ns ASC"
	}
	queryRows, err := db.QueryContext(ctx, statement, args...)
	if err != nil {
		return err
	}
	defer queryRows.Close()
	exact := make(map[int64]struct{}, len(selector.At))
	for _, at := range selector.At {
		if at.IsZero() {
			return errors.New("time-series exact time is zero")
		}
		exact[at.UnixNano()] = struct{}{}
	}
	for queryRows.Next() {
		var timestamp int64
		var encoded string
		if err := queryRows.Scan(&timestamp, &encoded); err != nil {
			return err
		}
		if len(exact) > 0 {
			if _, ok := exact[timestamp]; !ok {
				continue
			}
		}
		fields, err := decodeFields([]byte(encoded))
		if err != nil {
			return err
		}
		match, err := matches(fields, selector.Filters)
		if err != nil {
			return err
		}
		if match {
			if err := visit(sqlitets.Row{Time: time.Unix(0, timestamp), Fields: fields}); err != nil {
				return err
			}
		}
	}
	return queryRows.Err()
}

func (s *Store) validateQuery(query *sqlitets.Query) error {
	if query.Aggregate > sqlitets.AggregateCount {
		return fmt.Errorf("unsupported time-series aggregate: %d", query.Aggregate)
	}
	if _, err := s.prepareSelector(&query.Selector); err != nil {
		return err
	}
	if len(query.Fields) == 0 && !query.AllFields {
		return errors.New("time-series query fields are empty")
	}
	if query.AllFields && len(query.Fields) > 0 {
		return errors.New("time-series query cannot select named fields and all fields")
	}
	for _, field := range query.Fields {
		if field == "" {
			return errors.New("time-series query field is empty")
		}
	}
	if query.Limit < 0 || query.Offset < 0 || query.Interval < 0 {
		return errors.New("time-series limit, offset and interval cannot be negative")
	}
	if query.Offset > 0 && query.Limit == 0 {
		return errors.New("time-series offset requires a limit")
	}
	if query.AllFields && query.Aggregate != sqlitets.AggregateNone {
		return errors.New("time-series aggregate requires named fields")
	}
	if query.Interval > 0 && query.Aggregate == sqlitets.AggregateNone {
		return errors.New("time-series interval requires an aggregate")
	}
	return nil
}

func (s *Store) kindFor(measurement string) (string, error) {
	kind, ok := s.measurements[measurement]
	if !ok {
		return "", fmt.Errorf("unsupported time-series measurement %q", measurement)
	}
	return kind, nil
}

func shardOverlaps(start time.Time, selector sqlitets.Selector) bool {
	end := start.AddDate(0, 1, 0)
	if !selector.Start.IsZero() && !end.After(selector.Start.UTC()) {
		return false
	}
	if !selector.End.IsZero() && !selector.End.UTC().After(start) {
		return false
	}
	if len(selector.At) > 0 {
		for _, at := range selector.At {
			utc := at.UTC()
			if !utc.Before(start) && utc.Before(end) {
				return true
			}
		}
		return false
	}
	return true
}

func matches(fields map[string]any, filters []sqlitets.Filter) (bool, error) {
	for _, filter := range filters {
		if filter.Field == "" {
			return false, errors.New("time-series filter field is empty")
		}
		value, ok := fields[filter.Field]
		if !ok {
			return false, nil
		}
		switch filter.Op {
		case sqlitets.FilterEqual:
			if !equal(value, filter.Value) {
				return false, nil
			}
		case sqlitets.FilterContains:
			needle, ok := filter.Value.(string)
			if !ok {
				return false, errors.New("time-series contains filter requires a string")
			}
			text, ok := value.(string)
			if !ok || !strings.Contains(text, needle) {
				return false, nil
			}
		case sqlitets.FilterIn:
			list := reflect.ValueOf(filter.Value)
			if list.Kind() != reflect.Array && list.Kind() != reflect.Slice {
				return false, errors.New("time-series in filter requires a slice")
			}
			found := false
			for i := 0; i < list.Len(); i++ {
				if equal(value, list.Index(i).Interface()) {
					found = true
					break
				}
			}
			if !found {
				return false, nil
			}
		default:
			return false, fmt.Errorf("unsupported time-series filter operation: %d", filter.Op)
		}
	}
	return true, nil
}

type aggregateState struct {
	sum   float64
	count int64
	min   float64
	max   float64
}

// Only the current time bucket is retained; ordered scans keep cross-month buckets together.
type aggregateBucket struct {
	time   int64
	fields map[string]*aggregateState
}

func bucketTime(timestamp int64, interval time.Duration) int64 {
	if interval == 0 {
		return 0
	}
	return timestamp / int64(interval) * int64(interval)
}

func (bucket *aggregateBucket) add(row sqlitets.Row, fields []string, operation sqlitets.Aggregate, counts bool) error {
	for _, field := range fields {
		var value float64
		count := int64(1)
		if counts {
			count = row.Fields[field].(int64)
			if count == 0 {
				continue
			}
		} else {
			numeric, ok := row.Fields[field].(json.Number)
			if !ok {
				continue
			}
			if operation != sqlitets.AggregateCount {
				var err error
				value, err = numeric.Float64()
				if err != nil {
					return fmt.Errorf("aggregate field %q exceeds float64 range: %w", field, err)
				}
			}
		}
		state := bucket.fields[field]
		if state == nil {
			state = &aggregateState{min: value, max: value}
			bucket.fields[field] = state
		}
		state.count += count
		if operation == sqlitets.AggregateMean || operation == sqlitets.AggregateSum {
			state.sum += value
			if math.IsInf(state.sum, 0) {
				return fmt.Errorf("aggregate field %q sum exceeds float64 range", field)
			}
		}
		state.min = math.Min(state.min, value)
		state.max = math.Max(state.max, value)
	}
	return nil
}

func (bucket *aggregateBucket) row(operation sqlitets.Aggregate) sqlitets.Row {
	values := make(map[string]any, len(bucket.fields))
	for field, state := range bucket.fields {
		switch operation {
		case sqlitets.AggregateMean:
			values[field] = state.sum / float64(state.count)
		case sqlitets.AggregateMax:
			values[field] = state.max
		case sqlitets.AggregateMin:
			values[field] = state.min
		case sqlitets.AggregateSum:
			values[field] = state.sum
		case sqlitets.AggregateCount:
			values[field] = state.count
		}
	}
	return sqlitets.Row{Time: time.Unix(0, bucket.time), Fields: values}
}

// Daily record statistics only need numeric-field counts, not decoded point payloads.
func (s *Store) walkCounts(ctx context.Context, query sqlitets.Query, visit func(sqlitets.Row) error) error {
	return s.walkShards(ctx, query.Selector, query.Desc, func(shard monthly.Shard) error {
		expression := "0 + 0"
		args := []any{}
		if query.Interval > 0 {
			expression = "time_ns / ? * ?"
			args = append(args, int64(query.Interval), int64(query.Interval))
		}
		args = append(args, query.Fields[0])
		conditions, bounds := timeConditions(query.Selector)
		args = append(args, bounds...)
		statement := "SELECT " + expression + " AS bucket, SUM((SELECT COUNT(*) FROM json_each(fields_json) WHERE key = ? AND type IN ('integer','real'))) FROM points"
		if len(conditions) > 0 {
			statement += " WHERE " + strings.Join(conditions, " AND ")
		}
		statement += " GROUP BY bucket ORDER BY bucket"
		if query.Desc {
			statement += " DESC"
		}
		rows, err := shard.DB.QueryContext(ctx, statement, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var timestamp, count int64
			if err := rows.Scan(&timestamp, &count); err != nil {
				return err
			}
			if err := visit(sqlitets.Row{Time: time.Unix(0, timestamp), Fields: map[string]any{query.Fields[0]: count}}); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

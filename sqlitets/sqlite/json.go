package sqlite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/injoyai/util/sqlitets"
)

// UseNumber must cover both reads and merges so adding a field cannot round old values.
func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if len(bytes.TrimSpace(data[decoder.InputOffset():])) != 0 {
		return errors.New("unexpected data after JSON value")
	}
	return nil
}

func decodeFields(data []byte) (map[string]any, error) {
	var fields map[string]any
	if err := decodeJSON(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("time-series fields must be a JSON object")
	}
	return fields, nil
}

func prepareFilters(filters []sqlitets.Filter) ([]sqlitets.Filter, error) {
	prepared := make([]sqlitets.Filter, len(filters))
	for i, filter := range filters {
		if filter.Field == "" {
			return nil, errors.New("time-series filter field is empty")
		}
		if filter.Op > sqlitets.FilterIn {
			return nil, fmt.Errorf("unsupported time-series filter operation: %d", filter.Op)
		}
		data, err := json.Marshal(filter.Value)
		if err != nil {
			return nil, fmt.Errorf("encode filter %q: %w", filter.Field, err)
		}
		var value any
		if err := decodeJSON(data, &value); err != nil {
			return nil, fmt.Errorf("decode filter %q: %w", filter.Field, err)
		}
		filter.Value = value
		switch filter.Op {
		case sqlitets.FilterContains:
			if _, ok := filter.Value.(string); !ok {
				return nil, errors.New("time-series contains filter requires a string")
			}
		case sqlitets.FilterIn:
			if _, ok := filter.Value.([]any); !ok {
				return nil, errors.New("time-series in filter requires a JSON array")
			}
		}
		prepared[i] = filter
	}
	return prepared, nil
}

// Values have already passed through the same JSON representation as stored fields.
func equal(left, right any) bool {
	switch left := left.(type) {
	case json.Number:
		right, ok := right.(json.Number)
		if !ok {
			return false
		}
		if left == right {
			return true
		}
		a, aOK := new(big.Rat).SetString(left.String())
		b, bOK := new(big.Rat).SetString(right.String())
		return aOK && bOK && a.Cmp(b) == 0
	case []any:
		right, ok := right.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for i, value := range left {
			if !equal(value, right[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		right, ok := right.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			other, exists := right[key]
			if !exists || !equal(value, other) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(left, right)
	}
}

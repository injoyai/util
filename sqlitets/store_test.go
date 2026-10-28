package sqlitets

import (
	"testing"
	"time"
)

func TestRowsToMapsPreservesTimestamp(t *testing.T) {
	at := time.Unix(1_700_000_000, 123)
	rows := RowsToMaps([]Row{{
		Time:   at,
		Fields: map[string]any{"value": 12.5, "time": "invalid"},
	}})
	if len(rows) != 1 || rows[0]["time"] != at.UnixNano() || rows[0]["value"] != 12.5 {
		t.Fatalf("rows = %#v", rows)
	}
}

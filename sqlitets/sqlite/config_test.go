package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/injoyai/util/sqlitets"
	"github.com/injoyai/util/sqlitets/monthly"
)

func TestMeasurementRegistration(t *testing.T) {
	for _, tc := range []struct {
		name         string
		measurements map[string]string
	}{
		{"missing", nil},
		{"empty", map[string]string{}},
		{"blank name", map[string]string{" ": "sensors"}},
		{"empty directory", map[string]string{"readings": ""}},
		{"path traversal", map[string]string{"readings": "../outside"}},
		{"absolute directory", map[string]string{"readings": "/outside"}},
		{"shared directory", map[string]string{"a": "sensors", "b": "sensors"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := Open(t.TempDir(), tc.measurements)
			if err == nil {
				_ = store.Close()
				t.Fatal("invalid registration accepted")
			}
		})
	}
	if _, err := Use(nil, map[string]string{"readings": "sensors"}); err == nil {
		t.Fatal("nil shared store accepted")
	}
}

func TestMeasurementsAreCopiedAndIsolated(t *testing.T) {
	mapping := map[string]string{"readings": "sensors"}
	root := t.TempDir()
	first, err := Open(root, mapping)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	mapping["readings"] = "changed"
	mapping["other"] = "other"
	second, err := Open(t.TempDir(), map[string]string{"other": "other"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	})
	at := time.Now()
	fields := map[string]any{"value": 1}
	if err := first.Write(sqlitets.Point{Measurement: "readings", Time: at, Fields: fields}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sensors")); err != nil {
		t.Fatal(err)
	}
	if err := second.Write(sqlitets.Point{Measurement: "readings", Time: at, Fields: fields}); err == nil {
		t.Fatal("measurement leaked into another store")
	}
	if err := first.Write(sqlitets.Point{Measurement: "other", Time: at, Fields: fields}); err == nil {
		t.Fatal("caller mutation changed registered measurements")
	}
}

func TestSharedStoreOwnership(t *testing.T) {
	shards, err := monthly.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := shards.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := Use(shards, map[string]string{"readings": "sensors"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	fields := map[string]any{"value": 1}
	if err := store.Write(sqlitets.Point{Measurement: "readings", Time: at, Fields: fields}); err != nil {
		t.Fatalf("borrowed store closed its owner's shards: %v", err)
	}
	if err := shards.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Write(sqlitets.Point{Measurement: "readings", Time: at, Fields: fields}); err == nil {
		t.Fatal("write succeeded after owner closed shards")
	}
}

func TestExistingEdgeShardCompatibility(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "device"), 0755); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	filename := filepath.Join(root, "device", "2026-01.db")
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		t.Fatal(err)
	}
	// Build a fixture using the original edge schema, independently of pointsSchema.
	_, schemaErr := db.Exec(`CREATE TABLE points (time_ns INTEGER PRIMARY KEY, fields_json TEXT NOT NULL) WITHOUT ROWID`)
	if schemaErr != nil {
		_ = db.Close()
		t.Fatal(schemaErr)
	}
	_, writeErr := db.Exec(`INSERT INTO points VALUES (?, ?)`, at.UnixNano(), `{"temperature":20,"humidity":50}`)
	closeErr := db.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("fixture write=%v close=%v", writeErr, closeErr)
	}
	store, err := Open(root, map[string]string{
		"device_nature_log": "device",
		"system_nature_log": "system",
		"user_nature_log":   "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := context.Background()
	query := sqlitets.Query{Selector: sqlitets.Selector{Measurement: "device_nature_log"}, AllFields: true}
	rows, err := store.Query(ctx, query)
	if err != nil || len(rows) != 1 || rows[0].Fields["temperature"] != json.Number("20") {
		t.Fatalf("legacy rows=%v err=%v", rows, err)
	}
	if err := store.Write(sqlitets.Point{Measurement: "device_nature_log", Time: at, Fields: map[string]any{"temperature": 21}}); err != nil {
		t.Fatal(err)
	}
	rows, err = store.Query(ctx, query)
	if err != nil || len(rows) != 1 || rows[0].Fields["temperature"] != json.Number("21") || rows[0].Fields["humidity"] != json.Number("50") {
		t.Fatalf("merged legacy rows=%v err=%v", rows, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", filename)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	readErr := db.QueryRow(`SELECT COUNT(*) FROM points WHERE time_ns = ? AND json_extract(fields_json, '$.temperature') = 21`, at.UnixNano()).Scan(&count)
	closeErr = db.Close()
	if readErr != nil || closeErr != nil || count != 1 {
		t.Fatalf("original SQL read=%v close=%v count=%d", readErr, closeErr, count)
	}
}

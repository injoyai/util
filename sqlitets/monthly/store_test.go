package monthly

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRejectsUnsafeKindsAndUseAfterClose(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "records"))
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{
		Schema: "CREATE TABLE IF NOT EXISTS records (id INTEGER PRIMARY KEY, time_ns INTEGER NOT NULL)",
		Table:  "records", TimeColumn: "time_ns",
	}
	if err := store.Register("../escape", spec); err == nil {
		t.Fatal("unsafe kind accepted")
	}
	if err := store.Register("test", spec); err != nil {
		t.Fatal(err)
	}
	if err := store.WithDB(context.Background(), "test", time.Now(), func(db *sql.DB) error {
		_, err := db.Exec("INSERT INTO records(time_ns) VALUES(?)", time.Now().UnixNano())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.WithDB(context.Background(), "test", time.Now(), func(*sql.DB) error { return nil }); err == nil {
		t.Fatal("closed store accepted a write")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPageOffsetOverflow(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Register("records", Spec{Schema: "CREATE TABLE IF NOT EXISTS records(id INTEGER PRIMARY KEY, time_ns INTEGER)", Table: "records", TimeColumn: "time_ns"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if err := s.WithDB(ctx, "records", at, func(db *sql.DB) error {
		_, err := db.Exec("INSERT INTO records(id,time_ns) VALUES(1,?)", at.UnixNano())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 1, 2} {
		rows, total, err := Page(ctx, s, "records", PageQuery{Columns: "id", PageNum: math.MaxInt, PageSize: size}, func(row Scanner) (int, error) {
			var id int
			err := row.Scan(&id)
			return id, err
		})
		if size > 0 && int64(math.MaxInt) > math.MaxInt64/int64(size) {
			if err == nil {
				t.Fatal("overflowing page accepted", rows)
			}
			continue
		}
		if err != nil || total != 1 {
			t.Fatal("valid page rejected", size, total, err)
		}
		if size == 0 && len(rows) != 1 || size > 0 && len(rows) != 0 {
			t.Fatal("wrong page", size, rows)
		}
	}
	if err := s.DeleteBefore(ctx, "records", time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("unrepresentable delete cutoff accepted")
	}
	rows, total, err := Page(ctx, s, "records", PageQuery{Columns: "id"}, func(row Scanner) (int, error) {
		var id int
		err := row.Scan(&id)
		return id, err
	})
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatal("invalid cutoff changed stored records", rows, total, err)
	}
}

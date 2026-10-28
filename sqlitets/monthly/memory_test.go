package monthly

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func TestShardEvictionPreservesActiveCallbacksAndPagination(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := s.Register("test", Spec{Schema: "CREATE TABLE IF NOT EXISTS records (id INTEGER PRIMARY KEY, time_ns INTEGER NOT NULL)", Table: "records", TimeColumn: "time_ns"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		err := s.WithDB(ctx, "test", at.AddDate(0, i, 0), func(db *sql.DB) error {
			_, err := db.Exec("INSERT INTO records(id,time_ns) VALUES(?,?)", i, at.AddDate(0, i, 0).UnixNano())
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(s.dbs) > maxIdleShards {
			t.Fatalf("retained %d shards", len(s.dbs))
		}
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- s.WithShard(ctx, "test", "2024-01", func(db *sql.DB) error {
			close(entered)
			<-release
			return db.Ping()
		})
	}()
	<-entered
	err = s.Walk(ctx, "test", false, func(shard Shard) error { return shard.DB.Ping() })
	close(release)
	if activeErr := <-done; activeErr != nil {
		t.Fatal("active shard evicted", activeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	rows, total, err := Page(ctx, s, "test", PageQuery{Columns: "id", PageSize: 3, PageNum: 4}, func(row Scanner) (int, error) { var id int; err := row.Scan(&id); return id, err })
	if err != nil || total != 20 || fmt.Sprint(rows) != "[7 6 5]" {
		t.Fatalf("rows=%v total=%d err=%v", rows, total, err)
	}
	if len(s.dbs) > maxIdleShards {
		t.Fatal("pagination retained unbounded shards")
	}
}

func TestReopenedConnectionPreservesPragmas(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := s.Register("test", Spec{Schema: "CREATE TABLE IF NOT EXISTS records(id INTEGER)", Table: "records", TimeColumn: "id"}); err != nil {
		t.Fatal(err)
	}
	err = s.WithDB(context.Background(), "test", time.Now(), func(db *sql.DB) error {
		// Force fresh connections so connection-local options cannot survive by accident.
		db.SetMaxIdleConns(0)
		for _, tc := range []struct {
			name string
			want int
		}{{"busy_timeout", 5000}, {"synchronous", 1}} {
			var got int
			if err := db.QueryRow("PRAGMA " + tc.name).Scan(&got); err != nil {
				return err
			}
			if got != tc.want {
				return fmt.Errorf("%s=%d want=%d", tc.name, got, tc.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

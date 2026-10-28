package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/injoyai/util/sqlitets"
)

func TestWriteContextCancellationDoesNotWrite(t *testing.T) {
	store, err := openTestStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = store.WriteContext(ctx, sqlitets.Point{Measurement: "sensors", Time: time.Now(), Fields: map[string]any{"value": 1}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("write returned %v, want context.Canceled", err)
	}
	count, err := store.Count(context.Background(), sqlitets.CountQuery{
		Selector: sqlitets.Selector{Measurement: "sensors"}, Field: "value",
	})
	if err != nil || count != 0 {
		t.Fatalf("cancelled write persisted data: count=%d err=%v", count, err)
	}
}

package db_test

import (
	"context"
	"testing"

	"erecept/internal/db"
	"erecept/internal/db/dbtest"
)

func TestMigrateTwice(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var n int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'prescriptions' AND column_name = 'claimed_at'`).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("claimed_at: n=%d err=%v", n, err)
	}
	err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name = 'item_photos'`).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("item_photos: n=%d err=%v", n, err)
	}
}

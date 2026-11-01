// Package dbtest: чистая тестовая база на embedded-postgres.
// Подключать только из _test.go, боевой код его не импортирует.
package dbtest

import (
	"context"
	"fmt"
	"net"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"erecept/internal/db"
)

// New поднимает отдельный Postgres на свободном порту во временном каталоге,
// применяет миграции и останавливает сервер в t.Cleanup.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dir := t.TempDir()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint32(l.Addr().(*net.TCPAddr).Port)
	l.Close()

	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V18).
		Port(port).
		DataPath(dir + "/data").
		RuntimePath(dir + "/runtime").
		Logger(nil))
	if err := pg.Start(); err != nil {
		t.Fatalf("тестовая база не запустилась: %v", err)
	}
	t.Cleanup(func() { pg.Stop() })

	ctx := context.Background()
	url := fmt.Sprintf("postgres://postgres:postgres@127.0.0.1:%d/postgres?sslmode=disable", port)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

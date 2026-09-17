package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestInitDBAppliesPragmaToAllConns(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pragma.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	conns := make([]*sql.Conn, 0, 4)
	for i := 0; i < 4; i++ {
		c, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("conn %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	t.Cleanup(func() {
		for _, c := range conns {
			_ = c.Close()
		}
	})

	for i, c := range conns {
		var timeout int
		if err := c.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&timeout); err != nil {
			t.Fatalf("busy_timeout conn %d: %v", i, err)
		}
		if timeout != 5000 {
			t.Fatalf("busy_timeout conn %d = %d", i, timeout)
		}
		var fk int
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatalf("foreign_keys conn %d: %v", i, err)
		}
		if fk != 1 {
			t.Fatalf("foreign_keys conn %d = %d", i, fk)
		}
	}
	if db.Stats().MaxOpenConnections != 8 {
		t.Fatalf("MaxOpenConnections=%d", db.Stats().MaxOpenConnections)
	}
}

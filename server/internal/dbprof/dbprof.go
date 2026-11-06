package dbprof

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"customer-support/internal/repository"

	sqlite "modernc.org/sqlite"
)

const (
	DriverName = "sqlite-mnt-profile"
	EnvKey     = "MNT_PROFILE"
)

var (
	registerOnce sync.Once
	active       atomic.Pointer[int64]
)

func Enabled() bool {
	v := strings.TrimSpace(os.Getenv(EnvKey))
	return v == "1" || strings.EqualFold(v, "true")
}

func Register() {
	registerOnce.Do(func() {
		sql.Register(DriverName, &qcountDriver{})
	})
}

func Begin(n *int64) {
	if n != nil {
		atomic.StoreInt64(n, 0)
	}
	active.Store(n)
}

func End() {
	active.Store(nil)
}

func Queries(n *int64) int64 {
	if n == nil {
		return 0
	}
	return atomic.LoadInt64(n)
}

func OpenAppDB(path string) (*sql.DB, error) {
	if Enabled() {
		Register()
		return repository.InitDBDriver(DriverName, path)
	}
	return repository.InitDB(path)
}

type qcountDriver struct{ sqlite.Driver }

func (d *qcountDriver) Open(name string) (driver.Conn, error) {
	c, err := d.Driver.Open(name)
	if err != nil {
		return nil, err
	}
	return &qcountConn{Conn: c}, nil
}

type qcountConn struct{ driver.Conn }

func bump() {
	if p := active.Load(); p != nil {
		atomic.AddInt64(p, 1)
	}
}

func (c *qcountConn) Prepare(query string) (driver.Stmt, error) {
	bump()
	return c.Conn.Prepare(query)
}

func (c *qcountConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	bump()
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return pc.PrepareContext(ctx, query)
	}
	return c.Conn.Prepare(query)
}

func (c *qcountConn) Begin() (driver.Tx, error) { return c.Conn.Begin() }
func (c *qcountConn) Close() error              { return c.Conn.Close() }

func (c *qcountConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if b, ok := c.Conn.(driver.ConnBeginTx); ok {
		return b.BeginTx(ctx, opts)
	}
	return c.Conn.Begin()
}

func (c *qcountConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

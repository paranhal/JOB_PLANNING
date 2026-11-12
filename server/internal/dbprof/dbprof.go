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
	sqlMu        sync.Mutex
	sqlLog       []string
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
	sqlMu.Lock()
	sqlLog = sqlLog[:0]
	sqlMu.Unlock()
	active.Store(n)
}

// RecordedSQL 이번 요청에서 Prepare 된 SQL. 원인 분석용.
func RecordedSQL() []string {
	sqlMu.Lock()
	defer sqlMu.Unlock()
	out := make([]string, len(sqlLog))
	copy(out, sqlLog)
	return out
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

func noteSQL(query string) {
	if active.Load() == nil {
		return
	}
	sqlMu.Lock()
	if len(sqlLog) < 4000 {
		sqlLog = append(sqlLog, query)
	}
	sqlMu.Unlock()
}

func (c *qcountConn) Prepare(query string) (driver.Stmt, error) {
	bump()
	noteSQL(query)
	return c.Conn.Prepare(query)
}

func (c *qcountConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	bump()
	noteSQL(query)
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

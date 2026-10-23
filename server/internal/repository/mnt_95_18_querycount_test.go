package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync"
	"sync/atomic"

	sqlite "modernc.org/sqlite"
)

const sqliteQueryCountDriver = "sqlite-qcount-95-18"

var (
	registerQCountOnce sync.Once
	qcountActive       atomic.Pointer[int64]
)

type qcountDriver struct {
	sqlite.Driver
}

func (d *qcountDriver) Open(name string) (driver.Conn, error) {
	c, err := d.Driver.Open(name)
	if err != nil {
		return nil, err
	}
	return &qcountConn{Conn: c}, nil
}

type qcountConn struct {
	driver.Conn
}

func bumpQCount() {
	if p := qcountActive.Load(); p != nil {
		atomic.AddInt64(p, 1)
	}
}

func (c *qcountConn) Prepare(query string) (driver.Stmt, error) {
	bumpQCount()
	return c.Conn.Prepare(query)
}

func (c *qcountConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	bumpQCount()
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return pc.PrepareContext(ctx, query)
	}
	return c.Conn.Prepare(query)
}

func (c *qcountConn) Begin() (driver.Tx, error) {
	return c.Conn.Begin()
}

func (c *qcountConn) Close() error {
	return c.Conn.Close()
}

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

func registerQueryCountDriver() {
	registerQCountOnce.Do(func() {
		sql.Register(sqliteQueryCountDriver, &qcountDriver{})
	})
}

func openQueryCountDB(path string, n *int64) (*sql.DB, error) {
	registerQueryCountDriver()
	qcountActive.Store(n)
	db, err := sql.Open(sqliteQueryCountDriver, path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func clearQueryCount() {
	qcountActive.Store(nil)
}

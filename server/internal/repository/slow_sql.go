package repository

import (
	"database/sql"
	"log"
	"sync/atomic"
	"time"
)

var sqlQueryN atomic.Int64

func ResetSQLQueryN() { sqlQueryN.Store(0) }

func SQLQueryN() int64 { return sqlQueryN.Load() }

const slowSQLThreshold = 200 * time.Millisecond

func logSlowSQL(path string, start time.Time) {
	d := time.Since(start)
	if d >= slowSQLThreshold {
		log.Printf("slow sql path=%s dur=%s", path, d.Round(time.Millisecond))
	}
}

func queryTimed(db *sql.DB, path, q string, args ...interface{}) (*sql.Rows, error) {
	sqlQueryN.Add(1)
	start := time.Now()
	rows, err := db.Query(q, args...)
	logSlowSQL(path, start)
	return rows, err
}

func queryRowTimed(db *sql.DB, path, q string, args ...interface{}) *sql.Row {
	start := time.Now()
	row := db.QueryRow(q, args...)
	logSlowSQL(path, start)
	return row
}

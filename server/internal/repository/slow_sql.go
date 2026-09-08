package repository

import (
	"database/sql"
	"log"
	"time"
)

const slowSQLThreshold = 200 * time.Millisecond

func logSlowSQL(path string, start time.Time) {
	d := time.Since(start)
	if d >= slowSQLThreshold {
		log.Printf("slow sql path=%s dur=%s", path, d.Round(time.Millisecond))
	}
}

func queryTimed(db *sql.DB, path, q string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := db.Query(q, args...)
	logSlowSQL(path, start)
	return rows, err
}

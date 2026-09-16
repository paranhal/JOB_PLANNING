package repository

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AppSchemaVersion 이 바이너리가 필요로 하는 마이그레이션 최대 번호. §40.5.1
const AppSchemaVersion = 46

// AppStart 기동 한 번. (version, commit, built_at) 이 빌드를 식별한다.
type AppStart struct {
	Version   string
	Commit    string
	BuiltAt   string
	StartedAt string
	Host      string
}

// AppVersionRow 배포 이력 한 줄.
type AppVersionRow struct {
	VersionID      string
	Version        string
	Commit         string
	BuiltAt        string
	FirstStartedAt string
	LastStartedAt  string
	StartCount     int
	SchemaVersion  int
	Host           string
	Note           string
	Rollback       bool
}

func formatSchemaNo(n int) string {
	if n < 0 {
		n = 0
	}
	return fmt.Sprintf("%03d", n)
}

func applyAppVersions(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		log.Printf("042 schema_migrations: %v", err)
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS app_versions (
			version_id       TEXT PRIMARY KEY,
			version          TEXT NOT NULL DEFAULT '',
			"commit"         TEXT NOT NULL DEFAULT '',
			built_at         TEXT NOT NULL DEFAULT '',
			first_started_at TEXT NOT NULL DEFAULT '',
			last_started_at  TEXT NOT NULL DEFAULT '',
			start_count      INTEGER NOT NULL DEFAULT 1,
			schema_version   INTEGER NOT NULL DEFAULT 0,
			host             TEXT NOT NULL DEFAULT '',
			note             TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		log.Printf("042 app_versions: %v", err)
		return
	}
	if _, err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_app_versions_build
			ON app_versions(version, "commit", built_at)`); err != nil {
		log.Printf("042 app_versions index: %v", err)
	}
	if err := markSchemaApplied(db, AppSchemaVersion); err != nil {
		log.Printf("042 schema_migrations stamp: %v", err)
	}
}

func markSchemaApplied(db *sql.DB, version int) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
		version, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// AppliedSchemaVersion DB에 찍힌 마이그레이션 최대 번호. 표가 없으면 0.
func AppliedSchemaVersion(db *sql.DB) int {
	if db == nil {
		return 0
	}
	var n sql.NullInt64
	err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&n)
	if err != nil || !n.Valid {
		return 0
	}
	return int(n.Int64)
}

// SchemaMismatchMessage 앱과 DB 번호가 다를 때만 문구. 맞으면 빈 문자열. §40.5.1
func SchemaMismatchMessage(db *sql.DB) string {
	app := AppSchemaVersion
	cur := AppliedSchemaVersion(db)
	if cur == app {
		return ""
	}
	if cur < app {
		return fmt.Sprintf("DB 스키마가 낡았습니다 — %s 필요, 현재 %s", formatSchemaNo(app), formatSchemaNo(cur))
	}
	return fmt.Sprintf("DB 스키마가 앱보다 새 것입니다 — 앱 %s, 현재 %s. 앱을 되돌린 상태일 수 있습니다", formatSchemaNo(app), formatSchemaNo(cur))
}

func SchemaNo(n int) string { return formatSchemaNo(n) }

// RecordAppStart 기동 기록. 실패해도 호출 쪽이 서버를 죽이면 안 된다. §40.5
func RecordAppStart(db *sql.DB, s AppStart) error {
	if db == nil {
		return fmt.Errorf("db nil")
	}
	s.Version = strings.TrimSpace(s.Version)
	s.Commit = strings.TrimSpace(s.Commit)
	s.BuiltAt = strings.TrimSpace(s.BuiltAt)
	s.StartedAt = strings.TrimSpace(s.StartedAt)
	s.Host = strings.TrimSpace(s.Host)
	if s.Version == "" {
		s.Version = "dev"
	}
	if s.Commit == "" {
		s.Commit = "unknown"
	}
	if s.BuiltAt == "" {
		s.BuiltAt = "unknown"
	}
	if s.StartedAt == "" {
		s.StartedAt = time.Now().UTC().Format("2006-01-02 15:04")
	}
	schema := AppliedSchemaVersion(db)

	var id string
	err := db.QueryRow(
		`SELECT version_id FROM app_versions WHERE version=? AND "commit"=? AND built_at=?`,
		s.Version, s.Commit, s.BuiltAt,
	).Scan(&id)
	if err == nil && id != "" {
		_, err = db.Exec(
			`UPDATE app_versions SET last_started_at=?, start_count=start_count+1, schema_version=?, host=?
			 WHERE version_id=?`,
			s.StartedAt, schema, s.Host, id,
		)
		return err
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	id = "AV-" + uuid.New().String()[:12]
	_, err = db.Exec(
		`INSERT INTO app_versions(
			version_id, version, "commit", built_at, first_started_at, last_started_at,
			start_count, schema_version, host, note)
		 VALUES (?,?,?,?,?,?,1,?,?,'')`,
		id, s.Version, s.Commit, s.BuiltAt, s.StartedAt, s.StartedAt, schema, s.Host,
	)
	return err
}

// ListAppVersions 최근 배포 이력. first_started_at 내림차순. 롤백 표시 포함.
func ListAppVersions(db *sql.DB, limit int) ([]AppVersionRow, error) {
	if db == nil {
		return nil, fmt.Errorf("db nil")
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(
		`SELECT version_id, version, "commit", built_at, first_started_at, last_started_at,
		        start_count, schema_version, host, note
		 FROM app_versions
		 ORDER BY first_started_at DESC, version_id DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []AppVersionRow
	for rows.Next() {
		var r AppVersionRow
		if err := rows.Scan(
			&r.VersionID, &r.Version, &r.Commit, &r.BuiltAt, &r.FirstStartedAt, &r.LastStartedAt,
			&r.StartCount, &r.SchemaVersion, &r.Host, &r.Note,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	markRollbackRows(list)
	return list, nil
}

func markRollbackRows(list []AppVersionRow) {
	if len(list) == 0 {
		return
	}
	idx := make([]int, len(list))
	for i := range list {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool {
		a, b := list[idx[i]], list[idx[j]]
		if a.FirstStartedAt != b.FirstStartedAt {
			return a.FirstStartedAt < b.FirstStartedAt
		}
		return a.VersionID < b.VersionID
	})
	for k := 1; k < len(idx); k++ {
		prev := list[idx[k-1]]
		cur := &list[idx[k]]
		if builtAtEarlier(cur.BuiltAt, prev.BuiltAt) {
			cur.Rollback = true
		}
	}
}

func builtAtEarlier(a, b string) bool {
	ta, oka := parseAppClock(a)
	tb, okb := parseAppClock(b)
	if oka && okb {
		return ta.Before(tb)
	}
	return a < b
}

func parseAppClock(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// CountAppVersions 테스트용.
func CountAppVersions(db *sql.DB) int {
	if db == nil {
		return 0
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM app_versions`).Scan(&n)
	return n
}

// SetAppliedSchemaVersionForTest 스키마 불일치 시험을 위해 최대 번호를 덮어쓴다.
func SetAppliedSchemaVersionForTest(db *sql.DB, version int) error {
	if db == nil {
		return fmt.Errorf("db nil")
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations`); err != nil {
		return err
	}
	if version <= 0 {
		return nil
	}
	return markSchemaApplied(db, version)
}

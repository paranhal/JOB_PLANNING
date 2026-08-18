package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const ArchiveMonths = 3

// ArchiveLogs 변경 이력을 파일로 옮긴 뒤 테이블을 비운다.
func ArchiveLogs(dataDir string) (string, int, error) {
	if dbRef == nil {
		return "", 0, fmt.Errorf("DB가 없습니다")
	}
	var n int
	if err := dbRef.QueryRow(`SELECT COUNT(*) FROM data_change_logs`).Scan(&n); err != nil {
		return "", 0, err
	}
	if n == 0 {
		return "", 0, fmt.Errorf("옮길 이력이 없습니다")
	}
	var fromAt, toAt string
	_ = dbRef.QueryRow(`SELECT MIN(occurred_at), MAX(occurred_at) FROM data_change_logs`).Scan(&fromAt, &toAt)

	loc := time.Now()
	name := fmt.Sprintf("관리로그_%s-%s", archiveStamp(fromAt, loc), archiveStamp(toAt, loc))
	dir := filepath.Join(dataDir, "backups", name)
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", 0, err
	}

	destDB := filepath.Join(dir, "change_logs.db")
	adb, err := sql.Open("sqlite", destDB)
	if err != nil {
		return "", 0, err
	}
	defer adb.Close()
	if _, err := adb.Exec(`
		CREATE TABLE data_change_logs (
			log_id TEXT PRIMARY KEY,
			occurred_at TEXT, user_id TEXT, username TEXT, user_name TEXT,
			action TEXT, table_name TEXT, pk_column TEXT, entity_id TEXT,
			entity_label TEXT, summary TEXT, before_json TEXT, after_json TEXT,
			rolled_back INTEGER DEFAULT 0, rolled_back_at TEXT
		)`); err != nil {
		return "", 0, err
	}

	rows, err := dbRef.Query(`
		SELECT log_id, occurred_at, user_id, username, user_name,
		       action, table_name, pk_column, entity_id, entity_label, summary,
		       before_json, after_json, rolled_back, rolled_back_at
		FROM data_change_logs`)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	type rec struct {
		LogID, OccurredAt, UserID, Username, UserName string
		Action, TableName, PKColumn, EntityID, EntityLabel, Summary string
		BeforeJSON, AfterJSON, RolledBackAt string
		RolledBack int
	}
	var all []rec
	for rows.Next() {
		var r rec
		var uid, un, um, pk, eid, el, sm, bj, aj, rbat sql.NullString
		if err := rows.Scan(&r.LogID, &r.OccurredAt, &uid, &un, &um,
			&r.Action, &r.TableName, &pk, &eid, &el, &sm, &bj, &aj, &r.RolledBack, &rbat); err != nil {
			return "", 0, err
		}
		r.UserID, r.Username, r.UserName = uid.String, un.String, um.String
		r.PKColumn, r.EntityID, r.EntityLabel, r.Summary = pk.String, eid.String, el.String, sm.String
		r.BeforeJSON, r.AfterJSON, r.RolledBackAt = bj.String, aj.String, rbat.String
		all = append(all, r)
	}
	rows.Close()

	tx, err := adb.Begin()
	if err != nil {
		return "", 0, err
	}
	st, err := tx.Prepare(`INSERT INTO data_change_logs VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return "", 0, err
	}
	for _, r := range all {
		if _, err := st.Exec(r.LogID, r.OccurredAt, r.UserID, r.Username, r.UserName,
			r.Action, r.TableName, r.PKColumn, r.EntityID, r.EntityLabel, r.Summary,
			r.BeforeJSON, r.AfterJSON, r.RolledBack, r.RolledBackAt); err != nil {
			st.Close()
			tx.Rollback()
			return "", 0, err
		}
	}
	st.Close()
	if err := tx.Commit(); err != nil {
		return "", 0, err
	}

	jb, _ := json.MarshalIndent(all, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "change_logs.json"), jb, 0644)
	manifest := fmt.Sprintf("kind=log_archive\nfrom=%s\nto=%s\ncount=%d\nbackup_at=%s\n",
		fromAt, toAt, n, time.Now().Format(time.RFC3339))
	_ = os.WriteFile(filepath.Join(dir, "backup.txt"), []byte(manifest), 0644)

	if _, err := dbRef.Exec(`DELETE FROM data_change_logs`); err != nil {
		return "", 0, err
	}
	_, _ = dbRef.Exec(`
		INSERT INTO data_log_archives (archive_id, folder_name, from_at, to_at, log_count, created_at)
		VALUES (?,?,?,?,?,?)`,
		"A"+uuid.New().String()[:12], name, fromAt, toAt, n,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	RecordBackup(KindLogArchive, name, fmt.Sprintf("변경 이력 %d건", n))
	return name, n, nil
}

func archiveStamp(s string, fallback time.Time) string {
	s = fmt.Sprintf("%s", s)
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return t.Format("2006년01월02일")
		}
	}
	return fallback.Format("2006년01월02일")
}

// DueForArchive 마지막 아카이브(또는 가장 오래된 이력)가 3개월을 넘었는지.
func DueForArchive() bool {
	if dbRef == nil {
		return false
	}
	var last string
	_ = dbRef.QueryRow(`SELECT COALESCE(MAX(created_at),'') FROM data_log_archives`).Scan(&last)
	var oldest string
	_ = dbRef.QueryRow(`SELECT COALESCE(MIN(occurred_at),'') FROM data_change_logs`).Scan(&oldest)
	if oldest == "" {
		return false
	}
	base := oldest
	if last != "" && last > oldest {
		base = last
	}
	t, err := time.Parse("2006-01-02 15:04:05", base)
	if err != nil {
		if t2, e2 := time.Parse("2006-01-02", base[:min(10, len(base))]); e2 == nil {
			t = t2
		} else {
			return false
		}
	}
	return time.Now().After(t.AddDate(0, ArchiveMonths, 0))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// StartArchiveScheduler 3개월마다 관리 로그를 파일로 옮긴다.
func StartArchiveScheduler(dataDir string) {
	go func() {
		if DueForArchive() {
			if name, n, err := ArchiveLogs(dataDir); err != nil {
				log.Printf("관리 로그 아카이브 실패: %v", err)
			} else {
				log.Printf("관리 로그 아카이브 완료: %s (%d건)", name, n)
			}
		}
		for {
			time.Sleep(24 * time.Hour)
			if DueForArchive() {
				if name, n, err := ArchiveLogs(dataDir); err != nil {
					log.Printf("관리 로그 아카이브 실패: %v", err)
				} else {
					log.Printf("관리 로그 아카이브 완료: %s (%d건)", name, n)
				}
			}
		}
	}()
}

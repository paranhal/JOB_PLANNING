package audit

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// SnapshotStats 백업 폴더 분석 결과.
type SnapshotStats struct {
	Name       string
	KindLabel  string
	Exists     bool
	IsLogArchive bool
	DBPath     string
	Customers  int
	Assets     int
	ASReceipts int
	Attachments int
	WorkTasks  int
	ChangeLogs int
	RecentAS   [][3]string // 번호, 기관, 상태
	LogByUser  [][2]string // 이름, 건수
	LogByTable [][2]string
	Error      string
}

// AnalyzeBackup 백업 폴더의 app.db 또는 change_logs.db를 읽기 전용으로 연다.
func AnalyzeBackup(dataDir, name string) SnapshotStats {
	st := SnapshotStats{Name: name}
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		st.Error = "잘못된 백업 이름입니다"
		return st
	}
	dir := filepath.Join(dataDir, "backups", name)
	if _, err := os.Stat(dir); err != nil {
		st.Error = "백업 폴더가 없습니다"
		return st
	}
	st.Exists = true
	if strings.HasPrefix(name, "관리로그_") {
		st.IsLogArchive = true
		st.KindLabel = KindLabel(KindLogArchive)
		st.DBPath = filepath.Join(dir, "change_logs.db")
		analyzeLogDB(&st)
		return st
	}
	if strings.HasPrefix(name, "수동저장_") {
		st.KindLabel = KindLabel(KindManual)
	} else {
		st.KindLabel = KindLabel(KindAuto)
	}
	st.DBPath = filepath.Join(dir, "app.db")
	analyzeAppDB(&st)
	return st
}

func openRO(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("DB 파일이 없습니다")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro")
}

func analyzeAppDB(st *SnapshotStats) {
	db, err := openRO(st.DBPath)
	if err != nil {
		st.Error = err.Error()
		return
	}
	defer db.Close()
	_ = db.QueryRow(`SELECT COUNT(*) FROM customers`).Scan(&st.Customers)
	_ = db.QueryRow(`SELECT COUNT(*) FROM assets`).Scan(&st.Assets)
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&st.ASReceipts)
	_ = db.QueryRow(`SELECT COUNT(*) FROM attachments`).Scan(&st.Attachments)
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&st.WorkTasks)
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs`).Scan(&st.ChangeLogs)
	rows, err := db.Query(`
		SELECT ar.as_number, COALESCE(c.org_name,''), ar.status
		FROM as_receipts ar
		LEFT JOIN customers c ON c.customer_id=ar.customer_id
		ORDER BY ar.receipt_datetime DESC LIMIT 8`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var a, b, c string
			if rows.Scan(&a, &b, &c) == nil {
				st.RecentAS = append(st.RecentAS, [3]string{a, b, c})
			}
		}
	}
}

func analyzeLogDB(st *SnapshotStats) {
	db, err := openRO(st.DBPath)
	if err != nil {
		st.Error = err.Error()
		return
	}
	defer db.Close()
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs`).Scan(&st.ChangeLogs)
	rows, err := db.Query(`
		SELECT COALESCE(NULLIF(user_name,''), NULLIF(username,''), '(시스템)') AS who, COUNT(*)
		FROM data_change_logs GROUP BY who ORDER BY COUNT(*) DESC LIMIT 8`)
	if err == nil {
		for rows.Next() {
			var who string
			var n int
			if rows.Scan(&who, &n) == nil {
				st.LogByUser = append(st.LogByUser, [2]string{who, fmt.Sprintf("%d", n)})
			}
		}
		rows.Close()
	}
	rows2, err := db.Query(`
		SELECT COALESCE(NULLIF(entity_label,''), table_name), COUNT(*)
		FROM data_change_logs GROUP BY 1 ORDER BY COUNT(*) DESC LIMIT 8`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var label string
			var n int
			if rows2.Scan(&label, &n) == nil {
				st.LogByTable = append(st.LogByTable, [2]string{label, fmt.Sprintf("%d", n)})
			}
		}
	}
}

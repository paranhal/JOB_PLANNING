package repository

import (
	"path/filepath"
	"regexp"
	"testing"
)

// 95-224 마이그레이션 115: 사업·업무 구분 일괄 매칭은 비어 있는 업무만 바꾸고 한 번만 실행한다.
func TestWorkTaskMatch115(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "m115.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// InitDB 가 이미 115 를 실행했다(빈 DB라 대상 없음). 완료 표시를 지우고 시험 데이터를 넣어 다시 실행한다.
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(q, err)
		}
	}
	var ids []string
	rows, err := db.Query(`SELECT task_id FROM work_task_match_115`)
	if err == nil {
		rows.Close()
	}
	first, err := leaveLedgerSQL("115_work_task_project_match.sql")
	if err != nil {
		t.Fatal(err)
	}
	_ = first
	// 매칭 목록의 첫 사업 항목·업무 구분 항목을 SQL 에서 읽어 와 같은 ID 로 업무를 만든다.
	var pTask, pProj, aTask, aKind string
	r := db.QueryRow(`SELECT 1`)
	_ = r
	pTask, pProj, aTask, aKind = pickMatchRows(t, first)
	ex(`INSERT OR IGNORE INTO work_projects(project_id,name,org_id) VALUES(?,?,?)`, pProj, "시험사업", "O01")
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id) VALUES(?, 'admin','사업 대상','waiting','O01')`, pTask)
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,activity_kind) VALUES(?, 'admin','구분 이미 있음','waiting','O01','mgmt')`, aTask)
	ex(`DELETE FROM app_settings WHERE setting_key=?`, settingMigration115Done)
	applyWorkTaskMatchV115(db)
	var gotProj, gotKind string
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, pTask).Scan(&gotProj)
	if gotProj != pProj {
		t.Fatalf("사업 매칭 안 됨: %q want %q", gotProj, pProj)
	}
	_ = db.QueryRow(`SELECT COALESCE(activity_kind,'') FROM work_tasks WHERE task_id=?`, aTask).Scan(&gotKind)
	if gotKind != "mgmt" {
		t.Fatalf("이미 분류된 업무를 덮어썼다: %q (매칭 %q)", gotKind, aKind)
	}
	// 한 번만: 사용자가 사업을 비운 뒤 다시 기동해도 되살리지 않는다.
	ex(`UPDATE work_tasks SET project_id=NULL WHERE task_id=?`, pTask)
	applyWorkTaskMatchV115(db)
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, pTask).Scan(&gotProj)
	if gotProj != "" {
		t.Fatalf("두 번째 실행이 다시 덮어썼다: %q", gotProj)
	}
	var before, after string
	_ = db.QueryRow(`SELECT COALESCE(before_project,''), COALESCE(after_project,'') FROM work_task_match_115 WHERE task_id=?`, pTask).Scan(&before, &after)
	if before != "" || after != pProj {
		t.Fatalf("전후 기록: before=%q after=%q", before, after)
	}
	_ = ids
}

// pickMatchRows SQL 의 값 목록에서 사업 항목 1건·업무 구분 항목 1건을 고른다.
func pickMatchRows(t *testing.T, sqlText string) (pTask, pProj, aTask, aKind string) {
	t.Helper()
	re := regexp.MustCompile(`\('(WT[^']*)','([^']*)','([^']*)'\)`)
	for _, m := range re.FindAllStringSubmatch(sqlText, -1) {
		if pTask == "" && m[2] != "" {
			pTask, pProj = m[1], m[2]
		}
		if aTask == "" && m[3] != "" {
			aTask, aKind = m[1], m[3]
		}
	}
	if pTask == "" || aTask == "" {
		t.Fatal("매칭 SQL 에서 시험 항목을 찾지 못했다")
	}
	return
}

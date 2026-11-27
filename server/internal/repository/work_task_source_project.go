package repository

import (
	"database/sql"
	"log"
	"strings"
	"time"

	"customer-support/internal/model"
)

// 95-227: 접수·방문의 사업(project_id)을 그 원본에서 나온 업무(work_tasks)로 복사한다. 원본이 진실이고 업무는 복사본이다.
// 생성 시(CreateTask), 원본 사업이 바뀔 때(아래 sync 호출처), 기존 건 보정(117 마이그레이션) 세 곳이 이 함수들을 같이 쓴다.

// sourceProjectID 원본(접수·하부업무·점검 방문)의 사업. 원본이 없거나 비어 있으면 "".
func sourceProjectID(db *sql.DB, sourceType, sourceID string) string {
	sourceID = strings.TrimSpace(sourceID)
	if db == nil || sourceID == "" {
		return ""
	}
	var pid string
	switch sourceType {
	case model.WBSourceAS:
		_ = db.QueryRow(`SELECT COALESCE(TRIM(project_id),'') FROM as_receipts WHERE as_id=?`, sourceID).Scan(&pid)
	case model.WBSourceASWork:
		_ = db.QueryRow(`SELECT COALESCE(TRIM(ar.project_id),'') FROM as_work_items w JOIN as_receipts ar ON ar.as_id=w.as_id WHERE w.work_id=?`, sourceID).Scan(&pid)
	case model.WBSourceMaintenance:
		_ = db.QueryRow(`SELECT COALESCE(TRIM(project_id),'') FROM maintenance_visits WHERE visit_id=?`, sourceID).Scan(&pid)
	}
	return strings.TrimSpace(pid)
}

// inheritProjectFromSource 업무를 만들 때 사업이 비어 있으면 원본의 사업을 넣는다. 활동 분류와는 함께 둘 수 없으므로 사업이 우선이다.
func (r *WBRepo) inheritProjectFromSource(t *model.WorkTask) {
	if r == nil || r.db == nil || t == nil || strings.TrimSpace(t.ProjectID) != "" {
		return
	}
	if pid := sourceProjectID(r.db, t.SourceType, t.SourceID); pid != "" {
		t.ProjectID = pid
	}
}

// syncTaskProjectsFromSource 원본 하나에서 나온 모든 업무의 사업을 원본과 같게 맞춘다.
//   - sourceType: as(접수; 하부업무 as_work 포함) · maintenance(방문)
//   - clear=false: 원본에 사업이 있을 때만 복사한다(원본이 비어 있어도 업무에 직접 둔 사업은 유지).
//   - clear=true : 사업을 「해제」한 경우. 연결된 업무의 사업도 비운다.
//
// 바뀐 업무 건수를 돌려준다. 이미 같으면 0건이라 여러 번 불러도 결과가 같다.
func syncTaskProjectsFromSource(db *sql.DB, sourceType, sourceID string, clear bool) (int, error) {
	sourceID = strings.TrimSpace(sourceID)
	if db == nil || sourceID == "" {
		return 0, nil
	}
	pid := sourceProjectID(db, sourceType, sourceID)
	if pid == "" && !clear {
		return 0, nil
	}
	var where string
	var args []interface{}
	label := ""
	switch sourceType {
	case model.WBSourceAS:
		where = `((t.source_type=? AND t.source_id=?) OR (t.source_type=? AND t.source_id IN (SELECT work_id FROM as_work_items WHERE as_id=?)))`
		args = []interface{}{model.WBSourceAS, sourceID, model.WBSourceASWork, sourceID}
		label = "접수 " + sourceID
	case model.WBSourceMaintenance:
		where = `(t.source_type=? AND t.source_id=?)`
		args = []interface{}{model.WBSourceMaintenance, sourceID}
		label = "점검 방문 " + sourceID
	default:
		return 0, nil
	}
	rows, err := db.Query(`SELECT t.task_id, COALESCE(t.project_id,'') FROM work_tasks t WHERE `+where+` AND COALESCE(TRIM(t.project_id),'')<>?`, append(args, pid)...)
	if err != nil {
		return 0, err
	}
	type chg struct{ id, before string }
	var list []chg
	for rows.Next() {
		var c chg
		if rows.Scan(&c.id, &c.before) == nil {
			list = append(list, c)
		}
	}
	rows.Close()
	n := 0
	for _, c := range list {
		if _, err := db.Exec(`UPDATE work_tasks SET project_id=NULLIF(?,''),
			activity_kind=CASE WHEN ?<>'' THEN NULL ELSE activity_kind END,
			updated_at=datetime('now','localtime') WHERE task_id=?`, pid, pid, c.id); err != nil {
			return n, err
		}
		n++
		reason := "원본(" + label + ")의 사업을 복사"
		if pid == "" {
			reason = "원본(" + label + ")의 사업 해제를 반영"
		}
		logUpdateWithReason(db, "work_tasks", "task_id", c.id, "사업 연결: "+workProjectName(db, c.before)+" → "+workProjectName(db, pid), c.before, reason)
	}
	return n, nil
}

// SyncTaskProjectsOfReceipt 접수의 사업을 연결된 업무(하부업무 포함)로 복사한다. clear 는 해제 동작일 때만 true.
func SyncTaskProjectsOfReceipt(db *sql.DB, asID string, clear bool) (int, error) {
	return syncTaskProjectsFromSource(db, model.WBSourceAS, asID, clear)
}

// SyncTaskProjectsOfVisit 점검 방문의 사업을 연결된 업무로 복사한다.
func SyncTaskProjectsOfVisit(db *sql.DB, visitID string, clear bool) (int, error) {
	return syncTaskProjectsFromSource(db, model.WBSourceMaintenance, visitID, clear)
}

// writeThroughSourceProject 접수·방문 기반 업무에서 사업을 바꿀 때: 원본(접수·방문)에 먼저 쓰고 같은 원본의 업무 전체에 복사한다.
// 원본이 두 곳이 되는 것을 막기 위함이다(95-227 §3.5). 원본이 없는 업무는 false 를 돌려 종전 경로를 따른다.
func (r *WBRepo) writeThroughSourceProject(taskID, projectID string) (bool, error) {
	var st, sid string
	if err := r.db.QueryRow(`SELECT COALESCE(source_type,''), COALESCE(source_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&st, &sid); err != nil {
		return false, nil
	}
	projectID = strings.TrimSpace(projectID)
	switch st {
	case model.WBSourceAS:
		if _, err := r.db.Exec(`UPDATE as_receipts SET project_id=NULLIF(?,''), updated_at=datetime('now','localtime') WHERE as_id=?`, projectID, sid); err != nil {
			return false, err
		}
		_, err := SyncTaskProjectsOfReceipt(r.db, sid, projectID == "")
		return true, err
	case model.WBSourceASWork:
		var asID string
		_ = r.db.QueryRow(`SELECT as_id FROM as_work_items WHERE work_id=?`, sid).Scan(&asID)
		if asID == "" {
			return false, nil
		}
		if _, err := r.db.Exec(`UPDATE as_receipts SET project_id=NULLIF(?,''), updated_at=datetime('now','localtime') WHERE as_id=?`, projectID, asID); err != nil {
			return false, err
		}
		_, err := SyncTaskProjectsOfReceipt(r.db, asID, projectID == "")
		return true, err
	case model.WBSourceMaintenance:
		if _, err := r.db.Exec(`UPDATE maintenance_visits SET project_id=NULLIF(?,'') WHERE visit_id=?`, projectID, sid); err != nil {
			return false, err
		}
		_, err := SyncTaskProjectsOfVisit(r.db, sid, projectID == "")
		return true, err
	}
	return false, nil
}

const settingMigration117Done = "migration_117_done"

// applyWorkTaskProjectCopyV117 95-227 §4: 기존 건 보정. 딱 한 번만 실행한다.
func applyWorkTaskProjectCopyV117(db *sql.DB) {
	if db == nil {
		return
	}
	var done string
	_ = db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, settingMigration117Done).Scan(&done)
	if done != "" {
		return
	}
	up, err := leaveLedgerSQL("117_work_task_project_copy.sql")
	if err != nil {
		log.Printf("117 work_task_copy: %v", err)
		return
	}
	if _, err := db.Exec(up); err != nil {
		log.Printf("117 work_task_copy apply: %v", err)
		return
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_task_copy_117`).Scan(&n)
	now := time.Now().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT OR REPLACE INTO app_settings(setting_key, setting_value, updated_at) VALUES(?,?,?)`, settingMigration117Done, now, now)
	log.Printf("117 work_task_copy: 접수·방문 사업을 업무 %d건에 복사", n)
}

// WriteThroughSourceProject 접수·방문 기반 업무의 사업을 원본(접수·방문)에 먼저 쓰고 같은 원본의 업무 전체에 복사한다.
// 일정표 수정 화면처럼 SetTaskProject 를 거치지 않는 경로가 쓴다(그대로 두면 다음 점검 동기화가 방문의 빈 사업으로 되돌린다).
func (r *WBRepo) WriteThroughSourceProject(taskID, projectID string) (bool, error) {
	return r.writeThroughSourceProject(taskID, projectID)
}

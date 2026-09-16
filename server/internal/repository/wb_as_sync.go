package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

func applyWorkTaskAssigneeSource(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`ALTER TABLE work_tasks ADD COLUMN assignee_source TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("046 assignee_source: %v", err)
	}
	_, _ = db.Exec(`
		UPDATE work_tasks
		   SET assignee_source = ?
		 WHERE source_type = ?
		   AND TRIM(COALESCE(assignee_source,'')) = ''`,
		model.WBAssigneeSourceAS, model.WBSourceAS)
}

// SyncASDailyTask 담당자가 있는 AS를 일일업무(work_tasks)에 맞춘다. §42.2 · §42.3
// 예정일이 없어도 행을 만든다(날짜는 비움). 담당자가 없으면 연결된 행을 지운다.
// assignee_source='manual' 인 담당자는 AS 쪽 변경으로 덮지 않는다.
// 조회 화면에서 부르지 않는다. 접수·수정·담당자·예정일·완료 저장과 일일 정리에서만.
func (r *WBRepo) SyncASDailyTask(as *model.ASReceipt) error {
	if r == nil || r.db == nil || as == nil || strings.TrimSpace(as.ASID) == "" {
		return nil
	}
	if strings.TrimSpace(as.Status) == "cancelled" {
		return r.DeleteTasksBySource(model.WBSourceAS, as.ASID)
	}
	assignee := strings.TrimSpace(as.AssignedTo)
	existing, err := r.GetTaskBySource(model.WBSourceAS, as.ASID)
	if err != nil {
		return err
	}
	if assignee == "" {
		if existing == nil {
			return nil
		}
		if existing.Status == model.WBTaskComplete || existing.Status == "cancelled" {
			return nil
		}
		return r.DeleteTasksBySource(model.WBSourceAS, as.ASID)
	}

	visit := strings.TrimSpace(as.VisitScheduledDate)
	title := model.FormatASWorkTitle(as.OrgName, as.ASNumber)
	if existing == nil {
		t := &model.WorkTask{
			WorkType:       model.WBWorkAS,
			Title:          title,
			Description:    as.Symptom,
			DueDate:        visit,
			WorkDate:       visit,
			DurationMin:    30,
			Status:         model.WBTaskWaiting,
			Priority:       model.WBPriorityNormal,
			Assignee:       assignee,
			AssigneeSource: model.WBAssigneeSourceAS,
			SourceType:     model.WBSourceAS,
			SourceID:       as.ASID,
			CustomerID:     as.CustomerID,
		}
		if model.CanReopenAS(as.Status) {
			t.Status = model.WBTaskComplete
			t.Progress = 100
		}
		return r.CreateTask(t)
	}

	existing.Title = title
	if strings.TrimSpace(existing.Description) == "" {
		existing.Description = as.Symptom
	}
	if strings.TrimSpace(existing.CustomerID) == "" {
		existing.CustomerID = as.CustomerID
	}
	existing.DueDate = visit
	if strings.TrimSpace(existing.StartTime) == "" {
		existing.WorkDate = visit
	}
	if !existing.AssigneeIsManual() {
		existing.Assignee = assignee
		existing.AssigneeSource = model.WBAssigneeSourceAS
	}
	if model.CanReopenAS(as.Status) {
		existing.Status = model.WBTaskComplete
		if existing.Progress < 100 {
			existing.Progress = 100
		}
	}
	return r.UpdateTask(existing)
}

// ListUndatedASTasks 예정일·배정일이 비어 있는 AS 일일업무. 일정표 위 「날짜 미정」 줄. §42.2
func (r *WBRepo) ListUndatedASTasks() ([]model.WorkTask, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(workTaskSelect+`
		WHERE t.source_type = ?
		  AND TRIM(COALESCE(t.assignee,'')) != ''
		  AND TRIM(COALESCE(t.work_date,'')) = ''
		  AND TRIM(COALESCE(t.due_date,'')) = ''
		  AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		ORDER BY t.title, t.task_id`, model.WBSourceAS)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanWorkTasks(rows)
}

func (r *WBRepo) deleteOrphanASDailyTasks() (int, error) {
	res, err := r.db.Exec(`
		DELETE FROM work_tasks
		 WHERE source_type = ?
		   AND TRIM(COALESCE(source_id,'')) != ''
		   AND source_id NOT IN (SELECT as_id FROM as_receipts)`, model.WBSourceAS)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ReconcileASDailyTasks 저장 경로에서 놓친 AS↔일일업무를 하루 한 번 맞춘다. §42.4
// 조회 GET 에서 부르지 않는다. RunWorkListHousekeeping 에서만.
func ReconcileASDailyTasks(db *sql.DB) (int, error) {
	if db == nil {
		return 0, nil
	}
	asRepo := NewASRepo(db)
	wb := NewWBRepo(db)
	rows, err := db.Query(`SELECT as_id FROM as_receipts`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return n, err
		}
		as, err := asRepo.GetByID(id)
		if err != nil || as == nil {
			continue
		}
		if err := wb.SyncASDailyTask(as); err != nil {
			return n, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	if _, err := wb.deleteOrphanASDailyTasks(); err != nil {
		return n, err
	}
	return n, nil
}

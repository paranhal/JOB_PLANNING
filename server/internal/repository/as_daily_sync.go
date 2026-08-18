package repository

import (
	"database/sql"
	"strings"

	"customer-support/internal/model"
)

// backfillASPlannedDailyTasks 방문예정일이 있는 미완료 AS에 일일업무 행이 없으면 1회 생성한다.
// (예: R2608-036처럼 예정일만 넣고 업무 미생성인 건 → 일일 업무 등록 시간표에 자동 배치)
func backfillASPlannedDailyTasks(db *sql.DB) {
	if metaDone(db, asPlannedDailyTaskMetaKey) {
		return
	}
	wb := NewWBRepo(db)
	rows, err := db.Query(`
		SELECT ar.as_id, ar.as_number, COALESCE(c.org_name,''), COALESCE(ar.symptom,''),
		       COALESCE(ar.assigned_to,''), ar.visit_scheduled_date
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE ar.status IN ` + model.SQLStatusOpenIncomplete + `
		  AND TRIM(COALESCE(ar.visit_scheduled_date,'')) != ''
		  AND NOT EXISTS (
			SELECT 1 FROM work_tasks t
			WHERE t.source_type = ? AND t.source_id = ar.as_id
		  )`, model.WBSourceAS)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var asID, asNumber, orgName, symptom, assignee, visit string
		if rows.Scan(&asID, &asNumber, &orgName, &symptom, &assignee, &visit) != nil {
			continue
		}
		visit = strings.TrimSpace(visit)
		if visit == "" {
			continue
		}
		t := &model.WorkTask{
			WorkType:    model.WBWorkAS,
			Title:       model.FormatASWorkTitle(orgName, asNumber),
			Description: symptom,
			DueDate:     visit,
			WorkDate:    visit,
			DurationMin: 30,
			Status:      model.WBTaskWaiting,
			Priority:    model.WBPriorityNormal,
			Assignee:    strings.TrimSpace(assignee),
			SourceType:  model.WBSourceAS,
			SourceID:    asID,
		}
		_ = wb.CreateTask(t)
	}
	markMetaDone(db, asPlannedDailyTaskMetaKey)
}

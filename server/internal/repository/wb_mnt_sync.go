package repository

import (
	"strings"

	"customer-support/internal/model"
)

// 정기점검 방문(maintenance_visits) ↔ 일일업무(work_tasks) 일정 동기화.
// 점검 일정 화면에서 방문일·담당자를 바꾸면 연결된 일일업무도 같은 날짜로 옮긴다.

const mntTaskLinkSelect = `
	SELECT t.task_id, COALESCE(t.work_date,''), COALESCE(t.due_date,''),
	       COALESCE(t.start_time,''), COALESCE(t.end_time,''), COALESCE(t.duration_min,30),
	       COALESCE(t.assignee,''), COALESCE(t.title,''), COALESCE(t.description,''),
	       COALESCE(t.status,'waiting'), COALESCE(t.project_id,''),
	       v.visit_date, COALESCE(v.assignee,''), COALESCE(v.product_type,''),
	       COALESCE(v.completed,0), COALESCE(NULLIF(TRIM(v.completed_date),''), ''),
	       COALESCE(v.project_id,''),
	       COALESCE(NULLIF(TRIM(cfg.short_name),''), c.org_name)
	FROM work_tasks t
	JOIN maintenance_visits v ON v.visit_id = t.source_id
	JOIN customers c ON c.customer_id = v.customer_id
	LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
	WHERE t.source_type = 'maintenance'`

type mntTaskLink struct {
	taskID, workDate, dueDate, startTime, endTime string
	durationMin                                   int
	assignee, title, desc, status, taskProject    string
	visitDate, visitAssignee, productType         string
	completed                                     bool
	completedDate, visitProject, siteName         string
}

func (r *WBRepo) queryMntTaskLinks(where string, args ...interface{}) ([]mntTaskLink, error) {
	rows, err := r.db.Query(mntTaskLinkSelect+where, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []mntTaskLink
	for rows.Next() {
		var l mntTaskLink
		var done int
		if err := rows.Scan(&l.taskID, &l.workDate, &l.dueDate, &l.startTime, &l.endTime,
			&l.durationMin, &l.assignee, &l.title, &l.desc, &l.status, &l.taskProject,
			&l.visitDate, &l.visitAssignee, &l.productType, &done, &l.completedDate,
			&l.visitProject, &l.siteName); err != nil {
			return nil, err
		}
		l.completed = done != 0
		out = append(out, l)
	}
	return out, rows.Err()
}

// SyncMaintenanceTaskFromVisit 방문 1건의 일정·담당자를 연결된 일일업무에 반영한다.
func (r *WBRepo) SyncMaintenanceTaskFromVisit(visitID string) (bool, error) {
	visitID = strings.TrimSpace(visitID)
	if visitID == "" {
		return false, nil
	}
	links, err := r.queryMntTaskLinks(` AND t.source_id = ?`, visitID)
	if err != nil || len(links) == 0 {
		return false, err
	}
	changed := false
	for _, l := range links {
		ok, err := r.applyMntTaskLink(l)
		if err != nil {
			return changed, err
		}
		changed = changed || ok
	}
	return changed, nil
}

// SyncMaintenanceBoard 정기점검 방문을 기준으로 일일업무를 정리한다.
// 끊긴 업무 삭제 → 미생성 방문 보충 → 완료 상태 → 일자·담당자 순으로 맞춘다.
func (r *WBRepo) SyncMaintenanceBoard() {
	_, _ = r.DeleteOrphanMaintenanceTasks()
	_, _ = r.EnsureMaintenanceTasks()
	_ = r.BackfillMaintenanceTaskStatuses()
	_, _ = r.SyncMaintenanceTaskDates()
}

// EnsureMaintenanceTasks 아직 일일업무가 없는 방문에 대해 work_tasks 를 만든다.
// 완료된 방문도 통계 처리와 업무처리현황 조치가 맞도록 보충한다.
// ×로 내린 건(행이 남아 배정일만 빈 경우)은 다시 만들지 않는다.
func (r *WBRepo) EnsureMaintenanceTasks() (int, error) {
	rows, err := r.db.Query(`
		SELECT v.visit_id, v.visit_date, COALESCE(v.assignee,''), COALESCE(v.product_type,''),
		       COALESCE(NULLIF(TRIM(cfg.short_name),''), c.org_name),
		       COALESCE(v.completed,0), COALESCE(NULLIF(TRIM(v.completed_date),''), ''),
		       COALESCE(v.project_id,'')
		FROM maintenance_visits v
		JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		WHERE NOT EXISTS (
		    SELECT 1 FROM work_tasks t
		    WHERE t.source_type = 'maintenance' AND t.source_id = v.visit_id
		  )
		ORDER BY v.visit_date, v.sort_order`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var id, date, assignee, product, site, doneDate, projectID string
		var completed int
		if err := rows.Scan(&id, &date, &assignee, &product, &site, &completed, &doneDate, &projectID); err != nil {
			return n, err
		}
		workDate := date
		status := model.WBTaskWaiting
		if completed == 1 {
			status = model.WBTaskComplete
			if strings.TrimSpace(doneDate) != "" {
				workDate = strings.TrimSpace(doneDate)
			}
		}
		num := model.FormatMaintenanceVisitNumber(workDate, product)
		t := &model.WorkTask{
			WorkType:    model.WBWorkMaintenance,
			ProjectID:   strings.TrimSpace(projectID),
			Title:       model.FormatMaintenanceWorkTitle(site, num),
			Description: model.FormatMaintenanceDescription(product, site),
			DueDate:     date,
			WorkDate:    workDate,
			DurationMin: 30,
			Status:      status,
			Priority:    model.WBPriorityNormal,
			Assignee:    assignee,
			SourceType:  model.WBSourceMaintenance,
			SourceID:    id,
			ReceiptDate: date,
		}
		if completed == 1 {
			t.CompleteDate = workDate
		}
		if err := r.CreateTask(t); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// SyncMaintenanceTaskDates 방문일과 어긋난 일일업무를 일괄 정렬한다(화면 진입 시 자가 보정).
func (r *WBRepo) SyncMaintenanceTaskDates() (int, error) {
	links, err := r.queryMntTaskLinks(``)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, l := range links {
		ok, err := r.applyMntTaskLink(l)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}

// applyMntTaskLink 방문 정보에 맞춰 업무 한 건을 고친다. 바꾼 게 있으면 true.
// 미완료: 방문 예정일을 바꾸면 배정일도 그 날로 옮긴다. ×로 내린 건만 배정일을 비워 둔다.
// 완료: 예정일(due)만 방문일에 맞추고 실적일(work_date)은 유지한다.
func (r *WBRepo) applyMntTaskLink(l mntTaskLink) (bool, error) {
	visitDate := strings.TrimSpace(l.visitDate)
	if visitDate == "" {
		return false, nil
	}

	done := l.completed || l.status == model.WBTaskComplete
	dueDate := visitDate
	workDate := strings.TrimSpace(l.workDate)
	moved := false
	if done {
		if workDate == "" {
			if strings.TrimSpace(l.completedDate) != "" {
				workDate = strings.TrimSpace(l.completedDate)
			} else {
				workDate = visitDate
			}
		}
	} else if workDate == "" {
		// ×로 내린 미완료 건은 대기열에 둔다.
	} else if workDate != visitDate {
		moved = true
		workDate = visitDate
	}

	assignee := strings.TrimSpace(l.assignee)
	if v := strings.TrimSpace(l.visitAssignee); v != "" {
		assignee = v
	}
	projectID := strings.TrimSpace(l.taskProject)
	if p := strings.TrimSpace(l.visitProject); p != "" {
		projectID = p
	}

	title := l.title
	if strings.HasPrefix(strings.TrimSpace(title), "[점검]") {
		title = model.FormatMaintenanceWorkTitle(l.siteName,
			model.FormatMaintenanceVisitNumber(visitDate, l.productType))
	}
	desc := strings.TrimSpace(l.desc)
	// 설명에서 점검대상(제품)을 읽어 카드 색을 정하므로 자동 생성 문구는 다시 만든다.
	if desc == "" || strings.HasSuffix(desc, "정기점검") || strings.HasSuffix(desc, "정기 점검") {
		desc = model.FormatMaintenanceDescription(l.productType, l.siteName)
	}

	start, end := l.startTime, l.endTime
	// 미완료 건을 옮긴 날짜에서 같은 담당자 일정과 겹치면 시각을 비워 자동 배치에 맡긴다.
	if moved && !done && start != "" {
		overlap, err := r.AssigneeTimeOverlaps(workDate, assignee, start, end, l.taskID)
		if err != nil {
			return false, err
		}
		if overlap {
			start, end = "", ""
		}
	}

	if workDate == strings.TrimSpace(l.workDate) && dueDate == strings.TrimSpace(l.dueDate) &&
		assignee == strings.TrimSpace(l.assignee) && title == l.title && desc == strings.TrimSpace(l.desc) &&
		start == l.startTime && end == l.endTime && projectID == strings.TrimSpace(l.taskProject) {
		return false, nil
	}

	dur := l.durationMin
	if start != "" && end != "" {
		dur = model.DurationFromTimes(start, end)
	}
	_, err := r.db.Exec(`
		UPDATE work_tasks
		SET work_date=?, due_date=?, start_time=?, end_time=?, duration_min=?,
		    assignee=?, title=?, description=?, project_id=?, updated_at=CURRENT_TIMESTAMP
		WHERE task_id=?`,
		workDate, dueDate, start, end, model.NormalizeDurationMin(dur),
		assignee, title, desc, nullStr(projectID), l.taskID)
	if err != nil {
		return false, err
	}
	return true, nil
}

// DeleteOrphanMaintenanceTasks 방문이 사라진 정기점검 업무를 지운다.
// 방문 자체가 하나도 없으면(테이블 초기화 등) 안전하게 아무것도 하지 않는다.
func (r *WBRepo) DeleteOrphanMaintenanceTasks() (int, error) {
	var visits int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM maintenance_visits`).Scan(&visits); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	if visits == 0 {
		return 0, nil
	}
	res, err := r.db.Exec(`
		DELETE FROM work_tasks
		WHERE source_type='maintenance'
		  AND TRIM(COALESCE(source_id,'')) != ''
		  AND source_id NOT IN (SELECT visit_id FROM maintenance_visits)`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteTasksBySource 원본(AS·정기점검)이 사라질 때 연결된 일일업무를 정리한다.
func (r *WBRepo) DeleteTasksBySource(sourceType, sourceID string) error {
	sourceType = strings.TrimSpace(sourceType)
	sourceID = strings.TrimSpace(sourceID)
	if sourceType == "" || sourceID == "" {
		return nil
	}
	_, err := r.db.Exec(`DELETE FROM work_tasks WHERE source_type=? AND source_id=?`, sourceType, sourceID)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}

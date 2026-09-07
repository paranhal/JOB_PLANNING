package repository

import (
	"strings"

	"customer-support/internal/model"
)

// MissingCompleteDates 완료 상태인데 완료일이 없는 건. §14.4
type MissingCompleteDates struct {
	AS    int
	Mnt   int
	Admin int
}

func (s MissingCompleteDates) Total() int { return s.AS + s.Mnt + s.Admin }

// ListCompletedForStatusPeriod 업무처리현황「조치」용 완료 실적.
// 통계 처리와 같은 날짜를 쓴다. 완료일이 있는 건만.
//
//	AS: 완료·종료·부분완료 + complete_datetime
//	정기점검: completed=1 + completed_date
//	행정/지원: status=complete + complete_date
func (r *WBRepo) ListCompletedForStatusPeriod(from, to string) ([]model.WorkTask, error) {
	toEx := nextDayExclusive(to)
	as, err := r.completedASBetween(from, toEx)
	if err != nil {
		return nil, err
	}
	mnt, err := r.completedMntBetween(from, toEx)
	if err != nil {
		return nil, err
	}
	admin, err := r.completedAdminBetween(from, toEx)
	if err != nil {
		return nil, err
	}
	out := make([]model.WorkTask, 0, len(as)+len(mnt)+len(admin))
	out = append(out, as...)
	out = append(out, mnt...)
	out = append(out, admin...)
	return out, nil
}

func (r *WBRepo) completedASBetween(from, toEx string) ([]model.WorkTask, error) {
	q := `
		SELECT ar.as_id, ar.as_number,
		       ` + asCompleteDateSQL + `,
		       COALESCE(ar.complete_datetime, ''),
		       COALESCE(ar.assigned_to,''), ar.status,
		       c.org_name, COALESCE(ar.symptom,''),
		       COALESCE(t.task_id,''), COALESCE(t.start_time,''), COALESCE(t.end_time,''),
		       COALESCE(t.duration_min,30), COALESCE(t.project_id,''), COALESCE(t.work_date,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN work_tasks t ON t.source_type='as' AND t.source_id=ar.as_id
		  AND COALESCE(t.parent_task_id,'')=''
		WHERE ar.status IN ` + model.SQLStatusStatsCompleted + `
		  AND ` + asCompleteDateSQL + ` >= date(?)
		  AND ` + asCompleteDateSQL + ` < date(?)
		ORDER BY ` + asCompleteDateSQL + `, ar.as_number`
	rows, err := r.db.Query(q, from, toEx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	var out []model.WorkTask
	for rows.Next() {
		var asID, asNum, doneDate, doneDT, assignee, status, org, symptom string
		var taskID, start, end, projectID, taskDate string
		var dur int
		if err := rows.Scan(&asID, &asNum, &doneDate, &doneDT, &assignee, &status, &org, &symptom,
			&taskID, &start, &end, &dur, &projectID, &taskDate); err != nil {
			return nil, err
		}
		if seen[asID] {
			continue
		}
		seen[asID] = true
		doneDate = strings.TrimSpace(doneDate)
		if len(doneDate) > 10 {
			doneDate = doneDate[:10]
		}
		if start == "" || end == "" || strings.TrimSpace(taskDate) != doneDate {
			_, start, end = splitReceiptDateTime(doneDT)
		}
		title := model.FormatASWorkTitle(org, asNum)
		id := taskID
		if id == "" {
			id = "done:" + asID
		}
		out = append(out, model.WorkTask{
			TaskID:      id,
			WorkType:    model.WBWorkAS,
			ProjectID:   projectID,
			Title:       title,
			Description: symptom,
			DueDate:     doneDate,
			WorkDate:    doneDate,
			StartTime:   start,
			EndTime:     end,
			DurationMin: dur,
			Status:      status,
			Assignee:    assignee,
			Tags:        asNum,
			SourceType:  model.WBSourceAS,
			SourceID:    asID,
			BoardHref:   "/as/" + asID + "/action",
		})
	}
	return out, rows.Err()
}

func (r *WBRepo) completedMntBetween(from, toEx string) ([]model.WorkTask, error) {
	doneExpr := mntCompleteDateSQL
	q := `
		SELECT v.visit_id, ` + doneExpr + `, COALESCE(v.assignee,''), COALESCE(v.product_type,''),
		       COALESCE(NULLIF(TRIM(cfg.short_name),''), c.org_name),
		       COALESCE(t.task_id,''), COALESCE(t.start_time,''), COALESCE(t.end_time,''),
		       COALESCE(t.duration_min,30), COALESCE(t.work_date,'')
		FROM maintenance_visits v
		JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		LEFT JOIN work_tasks t ON t.source_type='maintenance' AND t.source_id=v.visit_id
		  AND COALESCE(t.parent_task_id,'')=''
		WHERE COALESCE(v.completed,0)=1
		  AND ` + doneExpr + ` >= ? AND ` + doneExpr + ` < ?
		ORDER BY ` + doneExpr + `, v.sort_order`
	rows, err := r.db.Query(q, from, toEx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	var out []model.WorkTask
	for rows.Next() {
		var visitID, doneDate, assignee, product, site, taskID, start, end, taskDate string
		var dur int
		if err := rows.Scan(&visitID, &doneDate, &assignee, &product, &site,
			&taskID, &start, &end, &dur, &taskDate); err != nil {
			return nil, err
		}
		if seen[visitID] {
			continue
		}
		seen[visitID] = true
		doneDate = strings.TrimSpace(doneDate)
		if len(doneDate) > 10 {
			doneDate = doneDate[:10]
		}
		if start == "" || strings.TrimSpace(taskDate) != doneDate {
			start, end = "09:00", "09:30"
		}
		num := model.FormatMaintenanceVisitNumber(doneDate, product)
		id := taskID
		if id == "" {
			id = "done:" + visitID
		}
		out = append(out, model.WorkTask{
			TaskID:      id,
			WorkType:    model.WBWorkMaintenance,
			Title:       model.FormatMaintenanceWorkTitle(site, num),
			Description: model.FormatMaintenanceDescription(product, site),
			DueDate:     doneDate,
			WorkDate:    doneDate,
			StartTime:   start,
			EndTime:     end,
			DurationMin: dur,
			Status:      model.WBTaskComplete,
			Assignee:    assignee,
			Tags:        num,
			SourceType:  model.WBSourceMaintenance,
			SourceID:    visitID,
			BoardHref:   "/maintenance/visits/" + visitID + "/action",
		})
	}
	return out, rows.Err()
}

func (r *WBRepo) completedAdminBetween(from, toEx string) ([]model.WorkTask, error) {
	q := workTaskSelect + `
		WHERE t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + SQLRecurrenceWorkUnit + `
		  AND t.status = 'complete'
		  AND ` + adminTaskCompleteDateSQL + ` >= ? AND ` + adminTaskCompleteDateSQL + ` < ?
		ORDER BY ` + adminTaskCompleteDateSQL + `, t.task_id`
	rows, err := r.db.Query(q, from, toEx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	items, err := scanWorkTasks(rows)
	if err != nil {
		return nil, err
	}
	for i := range items {
		d := strings.TrimSpace(items[i].CompleteDate)
		if len(d) > 10 {
			d = d[:10]
		}
		if d == "" {
			continue
		}
		items[i].WorkDate = d
		items[i].DueDate = d
	}
	return items, nil
}

func (r *WBRepo) CountMissingCompleteDates() (MissingCompleteDates, error) {
	var st MissingCompleteDates
	if err := r.db.QueryRow(`
		SELECT COUNT(*) FROM as_receipts
		WHERE status IN ` + model.SQLStatusStatsCompleted + `
		  AND TRIM(COALESCE(complete_datetime,'')) = ''`).Scan(&st.AS); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return st, nil
		}
		return st, err
	}
	if err := r.db.QueryRow(`
		SELECT COUNT(*) FROM maintenance_visits
		WHERE COALESCE(completed,0)=1
		  AND TRIM(COALESCE(completed_date,'')) = ''`).Scan(&st.Mnt); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return st, nil
		}
		return st, err
	}
	if err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + SQLRecurrenceWorkUnit + `
		  AND t.status = 'complete'
		  AND TRIM(COALESCE(t.complete_date,'')) = ''`).Scan(&st.Admin); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return st, nil
		}
		return st, err
	}
	return st, nil
}

func (r *WBRepo) ListMissingCompleteDates() ([]model.WorkTask, error) {
	as, err := r.missingAS()
	if err != nil {
		return nil, err
	}
	mnt, err := r.missingMnt()
	if err != nil {
		return nil, err
	}
	admin, err := r.missingAdmin()
	if err != nil {
		return nil, err
	}
	out := make([]model.WorkTask, 0, len(as)+len(mnt)+len(admin))
	out = append(out, as...)
	out = append(out, mnt...)
	out = append(out, admin...)
	return out, nil
}

func (r *WBRepo) missingAS() ([]model.WorkTask, error) {
	q := `
		SELECT ar.as_id, ar.as_number, COALESCE(ar.assigned_to,''), ar.status,
		       c.org_name, COALESCE(ar.symptom,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE ar.status IN ` + model.SQLStatusStatsCompleted + `
		  AND TRIM(COALESCE(ar.complete_datetime,'')) = ''
		ORDER BY ar.as_number`
	rows, err := r.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTask
	for rows.Next() {
		var asID, asNum, assignee, status, org, symptom string
		if err := rows.Scan(&asID, &asNum, &assignee, &status, &org, &symptom); err != nil {
			return nil, err
		}
		out = append(out, model.WorkTask{
			TaskID:      "missing:" + asID,
			WorkType:    model.WBWorkAS,
			Title:       model.FormatASWorkTitle(org, asNum),
			Description: symptom,
			Status:      status,
			Assignee:    assignee,
			Tags:        asNum,
			SourceType:  model.WBSourceAS,
			SourceID:    asID,
			BoardHref:   "/as/" + asID + "/action",
		})
	}
	return out, rows.Err()
}

func (r *WBRepo) missingMnt() ([]model.WorkTask, error) {
	q := `
		SELECT v.visit_id, COALESCE(v.assignee,''), COALESCE(v.product_type,''),
		       COALESCE(NULLIF(TRIM(cfg.short_name),''), c.org_name), v.visit_date
		FROM maintenance_visits v
		JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		WHERE COALESCE(v.completed,0)=1
		  AND TRIM(COALESCE(v.completed_date,'')) = ''
		ORDER BY v.visit_date, v.sort_order`
	rows, err := r.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTask
	for rows.Next() {
		var visitID, assignee, product, site, visitDate string
		if err := rows.Scan(&visitID, &assignee, &product, &site, &visitDate); err != nil {
			return nil, err
		}
		num := model.FormatMaintenanceVisitNumber(visitDate, product)
		out = append(out, model.WorkTask{
			TaskID:      "missing:" + visitID,
			WorkType:    model.WBWorkMaintenance,
			Title:       model.FormatMaintenanceWorkTitle(site, num),
			Description: model.FormatMaintenanceDescription(product, site),
			WorkDate:    visitDate,
			Status:      model.WBTaskComplete,
			Assignee:    assignee,
			Tags:        num,
			SourceType:  model.WBSourceMaintenance,
			SourceID:    visitID,
			BoardHref:   "/maintenance/visits/" + visitID + "/action",
		})
	}
	return out, rows.Err()
}

func (r *WBRepo) CountMissingAssignees() (MissingCompleteDates, error) {
	var st MissingCompleteDates
	if err := r.db.QueryRow(`
		SELECT COUNT(*) FROM as_receipts
		WHERE status IN ` + model.SQLStatusStatsCompleted + `
		  AND TRIM(COALESCE(assigned_to,'')) = ''`).Scan(&st.AS); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return st, nil
		}
		return st, err
	}
	if err := r.db.QueryRow(`
		SELECT COUNT(*) FROM maintenance_visits
		WHERE COALESCE(completed,0)=1
		  AND TRIM(COALESCE(assignee,'')) = ''`).Scan(&st.Mnt); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return st, nil
		}
		return st, err
	}
	if err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + SQLRecurrenceWorkUnit + `
		  AND t.status = 'complete'
		  AND TRIM(COALESCE(t.assignee,'')) = ''`).Scan(&st.Admin); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return st, nil
		}
		return st, err
	}
	return st, nil
}

func (r *WBRepo) ListMissingAssignees() ([]model.WorkTask, error) {
	as, err := r.missingAssigneeAS()
	if err != nil {
		return nil, err
	}
	mnt, err := r.missingAssigneeMnt()
	if err != nil {
		return nil, err
	}
	admin, err := r.missingAssigneeAdmin()
	if err != nil {
		return nil, err
	}
	out := make([]model.WorkTask, 0, len(as)+len(mnt)+len(admin))
	out = append(out, as...)
	out = append(out, mnt...)
	out = append(out, admin...)
	return out, nil
}

func (r *WBRepo) missingAssigneeAS() ([]model.WorkTask, error) {
	q := `
		SELECT ar.as_id, ar.as_number, ar.status, c.org_name, COALESCE(ar.symptom,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE ar.status IN ` + model.SQLStatusStatsCompleted + `
		  AND TRIM(COALESCE(ar.assigned_to,'')) = ''
		ORDER BY ar.as_number`
	rows, err := r.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTask
	for rows.Next() {
		var asID, asNum, status, org, symptom string
		if err := rows.Scan(&asID, &asNum, &status, &org, &symptom); err != nil {
			return nil, err
		}
		out = append(out, model.WorkTask{
			TaskID:      "unassigned:" + asID,
			WorkType:    model.WBWorkAS,
			Title:       model.FormatASWorkTitle(org, asNum),
			Description: symptom,
			Status:      status,
			Tags:        asNum,
			SourceType:  model.WBSourceAS,
			SourceID:    asID,
			BoardHref:   "/as/" + asID + "/action",
		})
	}
	return out, rows.Err()
}

func (r *WBRepo) missingAssigneeMnt() ([]model.WorkTask, error) {
	q := `
		SELECT v.visit_id, COALESCE(v.product_type,''),
		       COALESCE(NULLIF(TRIM(cfg.short_name),''), c.org_name), v.visit_date
		FROM maintenance_visits v
		JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		WHERE COALESCE(v.completed,0)=1
		  AND TRIM(COALESCE(v.assignee,'')) = ''
		ORDER BY v.visit_date, v.sort_order`
	rows, err := r.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTask
	for rows.Next() {
		var visitID, product, site, visitDate string
		if err := rows.Scan(&visitID, &product, &site, &visitDate); err != nil {
			return nil, err
		}
		num := model.FormatMaintenanceVisitNumber(visitDate, product)
		out = append(out, model.WorkTask{
			TaskID:      "unassigned:" + visitID,
			WorkType:    model.WBWorkMaintenance,
			Title:       model.FormatMaintenanceWorkTitle(site, num),
			Description: model.FormatMaintenanceDescription(product, site),
			WorkDate:    visitDate,
			Status:      model.WBTaskComplete,
			Tags:        num,
			SourceType:  model.WBSourceMaintenance,
			SourceID:    visitID,
			BoardHref:   "/maintenance/visits/" + visitID + "/action",
		})
	}
	return out, rows.Err()
}

func (r *WBRepo) missingAssigneeAdmin() ([]model.WorkTask, error) {
	q := workTaskSelect + `
		WHERE t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + SQLRecurrenceWorkUnit + `
		  AND t.status = 'complete'
		  AND TRIM(COALESCE(t.assignee,'')) = ''
		ORDER BY t.task_id`
	rows, err := r.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanWorkTasks(rows)
}

func (r *WBRepo) missingAdmin() ([]model.WorkTask, error) {
	q := workTaskSelect + `
		WHERE t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + SQLRecurrenceWorkUnit + `
		  AND t.status = 'complete'
		  AND TRIM(COALESCE(t.complete_date,'')) = ''
		ORDER BY t.task_id`
	rows, err := r.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanWorkTasks(rows)
}

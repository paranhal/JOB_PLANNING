package repository

import (
	"strings"

	"customer-support/internal/model"
)

// ListPlannedSourcesBetween 기간 안에 예정돼 있지만 아직 일일업무로 만들지 않은 AS·정기점검.
// 업무처리현황「예정업무」에서 일정표에 없는 예정 건까지 함께 보여주기 위한 목록이다.
func (r *WBRepo) ListPlannedSourcesBetween(from, to string) ([]model.WorkTask, error) {
	out, err := r.plannedMaintenanceSources(from, to)
	if err != nil {
		return nil, err
	}
	as, err := r.plannedASSources(from, to)
	if err != nil {
		return nil, err
	}
	return append(out, as...), nil
}

func (r *WBRepo) plannedMaintenanceSources(from, to string) ([]model.WorkTask, error) {
	rows, err := r.db.Query(`
		SELECT v.visit_id, v.visit_date, COALESCE(v.assignee,''), COALESCE(v.product_type,''),
		       COALESCE(NULLIF(TRIM(cfg.short_name),''), c.org_name)
		FROM maintenance_visits v
		JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		WHERE v.visit_date >= ? AND v.visit_date <= ?
		  AND COALESCE(v.completed,0) = 0
		  AND NOT EXISTS (
		    SELECT 1 FROM work_tasks t
		    WHERE t.source_type = 'maintenance' AND t.source_id = v.visit_id
		  )
		ORDER BY v.visit_date, v.sort_order`, from, to)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var out []model.WorkTask
	for rows.Next() {
		var id, date, assignee, product, site string
		if err := rows.Scan(&id, &date, &assignee, &product, &site); err != nil {
			return nil, err
		}
		num := model.FormatMaintenanceVisitNumber(date, product)
		out = append(out, model.WorkTask{
			TaskID:      "plan:" + id,
			WorkType:    model.WBWorkMaintenance,
			Title:       model.FormatMaintenanceWorkTitle(site, num),
			Description: model.FormatMaintenanceDescription(product, site),
			DueDate:     date,
			WorkDate:    date,
			DurationMin: 30,
			Status:      model.WBTaskWaiting,
			Assignee:    assignee,
			Tags:        num,
			SourceType:  model.WBSourceMaintenance,
			SourceID:    id,
			BoardHref:   "/maintenance/visits/" + id + "/action",
		})
	}
	return out, rows.Err()
}

func (r *WBRepo) plannedASSources(from, to string) ([]model.WorkTask, error) {
	rows, err := r.db.Query(`
		SELECT ar.as_id, ar.as_number, COALESCE(ar.visit_scheduled_date,''),
		       COALESCE(ar.assigned_to,''), COALESCE(ar.status,''), c.org_name, COALESCE(ar.symptom,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE COALESCE(ar.visit_scheduled_date,'') >= ? AND COALESCE(ar.visit_scheduled_date,'') <= ?
		  AND NOT EXISTS (
		    SELECT 1 FROM work_tasks t
		    WHERE t.source_type = 'as' AND t.source_id = ar.as_id
		  )
		ORDER BY ar.visit_scheduled_date, ar.as_number`, from, to)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var out []model.WorkTask
	for rows.Next() {
		var id, num, date, assignee, status, org, symptom string
		if err := rows.Scan(&id, &num, &date, &assignee, &status, &org, &symptom); err != nil {
			return nil, err
		}
		if model.IsStatsCompletedStatus(status) {
			continue
		}
		out = append(out, model.WorkTask{
			TaskID:      "plan:" + id,
			WorkType:    model.WBWorkAS,
			Title:       model.FormatASWorkTitle(org, num),
			Description: symptom,
			DueDate:     date,
			WorkDate:    date,
			DurationMin: 30,
			Status:      status,
			Assignee:    assignee,
			Tags:        num,
			SourceType:  model.WBSourceAS,
			SourceID:    id,
			BoardHref:   "/as/" + id,
		})
	}
	return out, rows.Err()
}

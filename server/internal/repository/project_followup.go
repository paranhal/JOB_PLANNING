package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// salesFollowupExistsSQL 미완료 다음 행동(work_actions) 또는 날짜가 있는 사업 연결 업무.
const salesFollowupExistsSQL = `
	EXISTS (
		SELECT 1 FROM work_tasks t
		LEFT JOIN work_actions a ON a.task_id = t.task_id
			AND COALESCE(a.status,'') NOT IN ('complete','cancelled')
		WHERE t.source_type = 'project' AND t.source_id = p.project_id
		  AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		  AND (
			COALESCE(a.action_id,'') != ''
			OR (
				NOT EXISTS (SELECT 1 FROM work_actions ax WHERE ax.task_id = t.task_id)
				AND COALESCE(NULLIF(TRIM(t.due_date),''), NULLIF(TRIM(t.work_date),''), '') != ''
			)
		  )
	)`

func salesPreWonSQL() string {
	return `COALESCE(p.project_kind,'maintenance') != 'maintenance'
		AND COALESCE(p.sales_stage,'') NOT IN ('won','lost','dropped')`
}

func (r *WBRepo) EnsureProjectTask(p *model.WorkProject) (*model.WorkTask, error) {
	if p == nil || strings.TrimSpace(p.ProjectID) == "" {
		return nil, fmt.Errorf("project_id 필요")
	}
	t, err := r.GetTaskBySource(model.WBSourceProject, p.ProjectID)
	if err != nil {
		return nil, err
	}
	if t != nil {
		if t.Status == model.WBTaskComplete || t.Status == model.WBTaskCancelled {
			t.Status = model.WBTaskWaiting
			if err := r.UpdateTask(t); err != nil {
				return nil, err
			}
		}
		return t, nil
	}
	title := strings.TrimSpace(p.Name)
	if title == "" {
		title = "영업 건"
	}
	t = &model.WorkTask{
		WorkType:    model.WBWorkAdmin,
		ProjectID:   p.ProjectID,
		Title:       title,
		Status:      model.WBTaskWaiting,
		Priority:    model.WBPriorityNormal,
		Assignee:    p.SalesOwner,
		SourceType:  model.WBSourceProject,
		SourceID:    p.ProjectID,
		CustomerID:  p.CustomerID,
		CustomerName: p.CustomerLabel(),
		DurationMin: 30,
	}
	if err := r.CreateTask(t); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *WBRepo) AddProjectFollowup(p *model.WorkProject, title, due, assignee string) error {
	due, err := model.ParseAppDate(due)
	if err != nil {
		return err
	}
	if due == "" {
		return fmt.Errorf("다음 행동 기한을 입력하세요.")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "다음 행동"
	}
	t, err := r.EnsureProjectTask(p)
	if err != nil {
		return err
	}
	if strings.TrimSpace(assignee) == "" {
		assignee = p.SalesOwner
	}
	if strings.TrimSpace(t.DueDate) == "" || due < t.DueDate {
		t.DueDate = due
		if err := r.UpdateTask(t); err != nil {
			return err
		}
	}
	return r.CreateAction(&model.WorkAction{
		TaskID:   t.TaskID,
		Title:    title,
		Status:   model.WBActionTodo,
		Required: true,
		DueDate:  due,
		Assignee: assignee,
	})
}

func (r *WBRepo) ListOpenProjectActions(projectID string) ([]model.WorkAction, error) {
	t, err := r.GetTaskBySource(model.WBSourceProject, projectID)
	if err != nil || t == nil {
		return nil, err
	}
	all, err := r.ListActions(t.TaskID)
	if err != nil {
		return nil, err
	}
	var out []model.WorkAction
	for _, a := range all {
		if a.Status == model.WBActionComplete || a.Status == model.WBActionCancelled {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (r *WBRepo) NextActionDueMap() (map[string]string, error) {
	out := map[string]string{}
	rows, err := r.db.Query(`
		SELECT t.source_id,
		       MIN(COALESCE(NULLIF(TRIM(a.due_date),''), NULLIF(TRIM(a.scheduled_date),''),
		                    NULLIF(TRIM(t.due_date),''), NULLIF(TRIM(t.work_date),'')))
		FROM work_tasks t
		LEFT JOIN work_actions a ON a.task_id = t.task_id
			AND COALESCE(a.status,'') NOT IN ('complete','cancelled')
		WHERE t.source_type='project'
		  AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		GROUP BY t.source_id`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, due string
		if err := rows.Scan(&id, &due); err != nil {
			return nil, err
		}
		if strings.TrimSpace(id) != "" && strings.TrimSpace(due) != "" {
			out[id] = due
		}
	}
	return out, rows.Err()
}

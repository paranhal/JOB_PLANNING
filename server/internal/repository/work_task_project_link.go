package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type WorkHistoryRow struct {
	TaskID   string
	WorkDate string
	Title    string
}

func (r *WBRepo) WorkProjectName(projectID string) string {
	projectID = strings.TrimSpace(projectID)
	if r == nil || r.db == nil || projectID == "" {
		return ""
	}
	var name string
	_ = r.db.QueryRow(`SELECT COALESCE(name,'') FROM work_projects WHERE project_id=?`, projectID).Scan(&name)
	return strings.TrimSpace(name)
}

func (r *WBRepo) SetTaskProject(orgID, taskID, projectID string) (beforeName, afterName string, err error) {
	orgID, err = RequireInsertOrg(orgID)
	if err != nil {
		return "", "", err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", "", fmt.Errorf("업무가 없습니다")
	}
	var beforeID string
	if err := r.db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=? AND org_id=?`, taskID, orgID).Scan(&beforeID); err != nil {
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("업무가 없습니다")
		}
		return "", "", err
	}
	beforeName = r.WorkProjectName(beforeID)
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		var name string
		err := r.db.QueryRow(`SELECT COALESCE(name,'') FROM work_projects WHERE project_id=?`, projectID).Scan(&name)
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("사업을 찾을 수 없습니다")
		}
		if err != nil {
			return "", "", err
		}
		afterName = strings.TrimSpace(name)
	}
	if _, err := r.db.Exec(`UPDATE work_tasks SET project_id=?, updated_at=datetime('now','localtime') WHERE task_id=? AND org_id=?`,
		emptyToNullStr(projectID), taskID, orgID); err != nil {
		return "", "", err
	}
	sites, err := r.ListTaskSites(taskID)
	if err != nil {
		return beforeName, afterName, err
	}
	var ids []string
	for _, s := range sites {
		if id := strings.TrimSpace(s.SiteID); id != "" {
			ids = append(ids, id)
		}
	}
	if err := r.ReplaceTaskCustomers(taskID, projectID, ids); err != nil {
		return beforeName, afterName, err
	}
	label := "사업 연결: "
	if beforeName == "" {
		label += "없음"
	} else {
		label += beforeName
	}
	label += " → "
	if afterName == "" {
		label += "없음"
	} else {
		label += afterName
	}
	logUpdate(r.db, "work_tasks", "task_id", taskID, label, beforeID)
	return beforeName, afterName, nil
}

func (r *WBRepo) AddTaskCustomers(taskID, projectID string, add []string) error {
	cur, err := r.ListTaskSites(taskID)
	if err != nil {
		return err
	}
	var ids []string
	for _, s := range cur {
		if id := strings.TrimSpace(s.SiteID); id != "" {
			ids = append(ids, id)
		}
	}
	ids = append(ids, add...)
	return r.ReplaceTaskCustomers(taskID, projectID, ids)
}

func (r *WBRepo) ListProjectSitesForPick(projectID string) ([]WorkTaskSite, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT ps.customer_id, COALESCE(NULLIF(TRIM(ps.site_name),''), c.org_name),
		       COALESCE(p.org_name,''),
		       COALESCE(NULLIF(TRIM(c.addr_sigungu),''), COALESCE(c.addr_sido,'')),
		       COALESCE(c.main_phone,'')
		FROM project_sites ps
		JOIN customers c ON c.customer_id=ps.customer_id
		LEFT JOIN customers p ON p.customer_id=c.parent_customer_id
		WHERE ps.project_id=? AND ps.is_active=1 AND ps.status != 'excluded'
		ORDER BY COALESCE(NULLIF(TRIM(ps.site_name),''), c.org_name)`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkTaskSite
	for rows.Next() {
		var it WorkTaskSite
		if err := rows.Scan(&it.CustomerID, &it.SiteName, &it.ParentName, &it.Region, &it.Phone); err != nil {
			return nil, err
		}
		it.SiteID = it.CustomerID
		it.OnProject = true
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *WBRepo) ListWorkTargetCategories(orgID string) ([]model.EquipmentCategory, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`
		SELECT id, org_id, COALESCE(parent_id,''), code, name, sort_order, is_active
		FROM equipment_categories
		WHERE org_id=? AND is_active=1 AND TRIM(COALESCE(parent_id,''))=''
		ORDER BY sort_order, name`, orgID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.EquipmentCategory
	for rows.Next() {
		var c model.EquipmentCategory
		if err := rows.Scan(&c.ID, &c.OrgID, &c.ParentID, &c.Code, &c.Name, &c.SortOrder, &c.IsActive); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *WBRepo) ListPastWorkHistory(orgID, taskID, projectID string, siteIDs []string, limit int) ([]WorkHistoryRow, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return nil, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	var q string
	var args []interface{}
	var ids []string
	seen := map[string]bool{}
	for _, id := range siteIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		ph := strings.Repeat("?,", len(ids))
		ph = ph[:len(ph)-1]
		q = `
			SELECT t.task_id, COALESCE(NULLIF(t.work_date,''), t.due_date,''), COALESCE(t.title,'')
			FROM work_tasks t
			WHERE t.org_id=? AND t.work_type=? AND t.task_id!=?
			  AND t.task_id IN (SELECT w.task_id FROM work_task_sites w WHERE w.customer_id IN (` + ph + `))
			ORDER BY COALESCE(NULLIF(t.work_date,''), t.due_date,'') DESC, t.task_id DESC
			LIMIT ?`
		args = []interface{}{orgID, model.WBWorkProjectWork, taskID}
		for _, id := range ids {
			args = append(args, id)
		}
		args = append(args, limit)
	} else if strings.TrimSpace(projectID) != "" {
		q = `
			SELECT t.task_id, COALESCE(NULLIF(t.work_date,''), t.due_date,''), COALESCE(t.title,'')
			FROM work_tasks t
			WHERE t.org_id=? AND t.work_type=? AND t.task_id!=? AND t.project_id=?
			ORDER BY COALESCE(NULLIF(t.work_date,''), t.due_date,'') DESC, t.task_id DESC
			LIMIT ?`
		args = []interface{}{orgID, model.WBWorkProjectWork, taskID, strings.TrimSpace(projectID), limit}
	} else {
		return nil, nil
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkHistoryRow
	for rows.Next() {
		var it WorkHistoryRow
		if err := rows.Scan(&it.TaskID, &it.WorkDate, &it.Title); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

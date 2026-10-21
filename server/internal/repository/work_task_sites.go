package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type WorkTaskSite struct {
	SiteID   string `json:"site_id"`
	SiteName string `json:"site_name"`
	Region   string `json:"region,omitempty"`
	Already  bool   `json:"already,omitempty"`
}

func (r *WBRepo) ReplaceTaskSites(taskID string, siteIDs []string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("업무가 없습니다")
	}
	if _, err := r.db.Exec(`DELETE FROM work_task_sites WHERE task_id=?`, taskID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range siteIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := r.db.Exec(`INSERT INTO work_task_sites(task_id,site_id) VALUES(?,?)`, taskID, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *WBRepo) ListTaskSites(taskID string) ([]WorkTaskSite, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.site_name, COALESCE(s.region,'')
		FROM work_task_sites w
		JOIN project_sites s ON s.id=w.site_id
		WHERE w.task_id=?
		ORDER BY s.site_name`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkTaskSite
	for rows.Next() {
		var it WorkTaskSite
		if err := rows.Scan(&it.SiteID, &it.SiteName, &it.Region); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *WBRepo) TaskSiteLine(taskID string) (string, error) {
	sites, err := r.ListTaskSites(taskID)
	if err != nil {
		return "", err
	}
	var names []string
	for _, s := range sites {
		names = append(names, s.SiteName)
	}
	return model.FormatTaskSiteLine(names), nil
}

func (r *WBRepo) ProjectOrgID(projectID string) (string, error) {
	var org string
	err := r.db.QueryRow(`SELECT TRIM(COALESCE(org_id,'')) FROM work_projects WHERE project_id=?`, projectID).Scan(&org)
	if err != nil {
		return "", fmt.Errorf("사업을 찾을 수 없습니다")
	}
	return org, nil
}

func (r *WBRepo) AssertSitesForProject(orgID, projectID string, siteIDs []string) error {
	for _, id := range siteIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		var pid, st, sorg string
		err := r.db.QueryRow(`SELECT project_id, status, org_id FROM project_sites WHERE id=? AND is_active=1`, id).Scan(&pid, &st, &sorg)
		if err != nil {
			return fmt.Errorf("사이트를 찾을 수 없습니다")
		}
		if pid != projectID {
			return fmt.Errorf("다른 사업 사이트입니다")
		}
		if model.NormalizeSiteStatus(st) == model.SiteExcluded {
			return fmt.Errorf("제외된 사이트입니다")
		}
		if sorg != orgID {
			return fmt.Errorf("다른 조직 사이트입니다")
		}
	}
	return nil
}

func (r *WBRepo) SearchProjectSites(orgID, projectID, q string, limit int) ([]WorkTaskSite, error) {
	q = strings.TrimSpace(q)
	if utf8Len(q) < 2 {
		return nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	like := "%" + q + "%"
	rows, err := r.db.Query(`
		SELECT s.id, s.site_name, COALESCE(s.region,'')
		FROM project_sites s
		WHERE s.project_id=? AND s.org_id=? AND s.is_active=1
		  AND s.status != 'excluded'
		  AND (s.site_name LIKE ? OR COALESCE(s.region,'') LIKE ?)
		ORDER BY s.site_name
		LIMIT ?`, projectID, orgID, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkTaskSite
	for rows.Next() {
		var it WorkTaskSite
		if err := rows.Scan(&it.SiteID, &it.SiteName, &it.Region); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func utf8Len(s string) int {
	return len([]rune(s))
}

func (r *WBRepo) ListWorkPickProjects(orgID string) ([]model.WorkProject, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT p.project_id, p.name, COALESCE(p.project_kind,''), COALESCE(p.status,'active')
		FROM work_projects p
		WHERE TRIM(COALESCE(p.org_id,''))=?
		  AND COALESCE(p.status,'') NOT IN ('archived','complete')
		ORDER BY CASE COALESCE(p.project_kind,'')
			WHEN 'build' THEN 0 WHEN 'construction' THEN 1 ELSE 2 END, p.name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkProject
	for rows.Next() {
		var p model.WorkProject
		if err := rows.Scan(&p.ProjectID, &p.Name, &p.ProjectKind, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

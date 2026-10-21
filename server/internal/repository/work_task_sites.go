package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"

	"customer-support/internal/model"
)

type WorkTaskSite struct {
	SiteID        string `json:"site_id"`
	CustomerID    string `json:"-"`
	SiteName      string `json:"site_name"`
	ParentName    string `json:"parent_name,omitempty"`
	Region        string `json:"region,omitempty"`
	Phone         string `json:"phone,omitempty"`
	OnProject     bool   `json:"on_project"`
	ProjectSiteID string `json:"-"`
}

func (r *WBRepo) ReplaceTaskSites(taskID string, siteIDs []string) error {
	return r.ReplaceTaskCustomers(taskID, "", siteIDs)
}

func (r *WBRepo) ReplaceTaskCustomers(taskID, projectID string, customerIDs []string) error {
	taskID = strings.TrimSpace(taskID)
	projectID = strings.TrimSpace(projectID)
	if taskID == "" {
		return fmt.Errorf("업무가 없습니다")
	}
	ids, err := uniqueCustomerIDs(customerIDs, false)
	if err != nil {
		return err
	}
	if _, err := r.db.Exec(`DELETE FROM work_task_sites WHERE task_id=?`, taskID); err != nil {
		return err
	}
	for _, id := range ids {
		var psid sql.NullString
		if projectID != "" {
			err := r.db.QueryRow(`
				SELECT id FROM project_sites
				WHERE project_id=? AND customer_id=? AND is_active=1 AND status != 'excluded'
				LIMIT 1`, projectID, id).Scan(&psid)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
		}
		if _, err := r.db.Exec(
			`INSERT INTO work_task_sites(task_id,customer_id,project_site_id) VALUES(?,?,?)`,
			taskID, id, emptyToNullStr(psid.String),
		); err != nil {
			return err
		}
	}
	return nil
}

func emptyToNullStr(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func uniqueCustomerIDs(ids []string, rejectDup bool) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if seen[id] {
			if rejectDup {
				return nil, fmt.Errorf("같은 기관을 중복 선택할 수 없습니다")
			}
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) > 20 {
		return nil, fmt.Errorf("사이트는 최대 20곳입니다")
	}
	return out, nil
}

func (r *WBRepo) ListTaskSites(taskID string) ([]WorkTaskSite, error) {
	rows, err := r.db.Query(`
		SELECT w.customer_id, c.org_name, COALESCE(p.org_name,''),
		       COALESCE(NULLIF(TRIM(c.addr_sigungu),''), COALESCE(c.addr_sido,'')),
		       COALESCE(c.main_phone,''),
		       COALESCE(w.project_site_id,''),
		       CASE WHEN TRIM(COALESCE(t.project_id,'')) != '' AND EXISTS (
		         SELECT 1 FROM project_sites ps
		         WHERE ps.project_id=t.project_id AND ps.customer_id=w.customer_id
		           AND ps.is_active=1 AND ps.status != 'excluded'
		       ) THEN 1 ELSE 0 END
		FROM work_task_sites w
		JOIN customers c ON c.customer_id=w.customer_id
		LEFT JOIN customers p ON p.customer_id=c.parent_customer_id
		LEFT JOIN work_tasks t ON t.task_id=w.task_id
		WHERE w.task_id=?
		ORDER BY c.org_name`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkTaskSite
	for rows.Next() {
		var it WorkTaskSite
		var on int
		if err := rows.Scan(&it.CustomerID, &it.SiteName, &it.ParentName, &it.Region, &it.Phone, &it.ProjectSiteID, &on); err != nil {
			return nil, err
		}
		it.SiteID = it.CustomerID
		it.OnProject = on == 1
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
		names = append(names, customerDisplayName(s.SiteName, s.ParentName))
	}
	return model.FormatTaskSiteLine(names), nil
}

func customerDisplayName(name, parent string) string {
	name = strings.TrimSpace(name)
	parent = strings.TrimSpace(parent)
	if parent != "" {
		return name + "(" + parent + ")"
	}
	return name
}

func (r *WBRepo) ProjectOrgID(projectID string) (string, error) {
	var org string
	err := r.db.QueryRow(`SELECT TRIM(COALESCE(org_id,'')) FROM work_projects WHERE project_id=?`, projectID).Scan(&org)
	if err != nil {
		return "", fmt.Errorf("사업을 찾을 수 없습니다")
	}
	return org, nil
}

func (r *WBRepo) AssertCustomersActive(ids []string) error {
	uniq, err := uniqueCustomerIDs(ids, true)
	if err != nil {
		return err
	}
	for _, id := range uniq {
		var active int
		var kind string
		err := r.db.QueryRow(`
			SELECT CASE WHEN is_active=1 THEN 1 ELSE 0 END,
			       COALESCE(NULLIF(TRIM(party_kind),''),'customer')
			FROM customers WHERE customer_id=?`, id).Scan(&active, &kind)
		if err != nil || active != 1 || kind != "customer" {
			return fmt.Errorf("고객을 찾을 수 없습니다")
		}
	}
	return nil
}

// org-scope: SearchWorkTaskCustomers — 95-9A 작업 대상 사이트는 조직별 관리가 아니다. 활성 고객 전체 검색, 응답은 기관명·상위기관·지역만.
func (r *WBRepo) SearchWorkTaskCustomers(q, projectID string, selected []string, limit int) ([]WorkTaskSite, error) {
	q = strings.TrimSpace(q)
	projectID = strings.TrimSpace(projectID)
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	sel, _ := uniqueCustomerIDs(selected, false)
	if len(sel) == 0 && utf8.RuneCountInString(q) < 2 {
		return nil, nil
	}
	onProj := `CASE WHEN ? != '' AND EXISTS (
		SELECT 1 FROM project_sites ps
		WHERE ps.project_id=? AND ps.customer_id=c.customer_id AND ps.is_active=1 AND ps.status != 'excluded'
	) THEN 1 ELSE 0 END`
	base := `
		SELECT c.customer_id, c.org_name, COALESCE(p.org_name,''),
		       COALESCE(NULLIF(TRIM(c.addr_sigungu),''), COALESCE(c.addr_sido,'')),
		       ` + onProj + `
		FROM customers c
		LEFT JOIN customers p ON p.customer_id=c.parent_customer_id
		WHERE c.is_active=1
		  AND COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')='customer'`
	var rows *sql.Rows
	var err error
	if len(sel) > 0 {
		ph := strings.Repeat("?,", len(sel))
		ph = strings.TrimSuffix(ph, ",")
		args := []any{projectID, projectID}
		for _, id := range sel {
			args = append(args, id)
		}
		rows, err = r.db.Query(base+` AND c.customer_id IN (`+ph+`) ORDER BY c.org_name LIMIT ?`, append(args, limit)...)
	} else {
		pat := likeContains(q)
		rows, err = r.db.Query(base+`
		  AND (
			c.org_name LIKE ? ESCAPE '\'
			OR c.official_name LIKE ? ESCAPE '\'
			OR COALESCE(c.org_code,'') LIKE ? ESCAPE '\'
			OR COALESCE(c.addr_sido,'') LIKE ? ESCAPE '\'
			OR COALESCE(c.addr_sigungu,'') LIKE ? ESCAPE '\'
			OR COALESCE(p.org_name,'') LIKE ? ESCAPE '\'
		  )
		ORDER BY c.org_name
		LIMIT ?`, projectID, projectID, pat, pat, pat, pat, pat, pat, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkTaskSite
	for rows.Next() {
		var it WorkTaskSite
		var on int
		if err := rows.Scan(&it.CustomerID, &it.SiteName, &it.ParentName, &it.Region, &on); err != nil {
			return nil, err
		}
		it.SiteID = it.CustomerID
		it.OnProject = on == 1
		out = append(out, it)
	}
	return out, rows.Err()
}

type SiteWorkItem struct {
	SiteID       string
	TaskID       string
	Title        string
	WorkSubtype  string
	Status       string
	WorkDate     string
	CompleteDate string
	CompleteNote string
}

func (it SiteWorkItem) Line() string {
	var parts []string
	if lab := model.WorkSubtypeLabel(it.WorkSubtype); lab != "" {
		parts = append(parts, lab)
	}
	if strings.TrimSpace(it.Title) != "" {
		parts = append(parts, strings.TrimSpace(it.Title))
	}
	if it.Status == model.WBTaskComplete {
		parts = append(parts, "완료")
	}
	return strings.Join(parts, " · ")
}

func (r *WBRepo) ListSiteWorksByProject(projectID string) (map[string][]SiteWorkItem, error) {
	projectID = strings.TrimSpace(projectID)
	out := map[string][]SiteWorkItem{}
	if projectID == "" {
		return out, nil
	}
	rows, err := r.db.Query(`
		SELECT s.id, t.task_id, t.title, COALESCE(t.work_subtype,''), COALESCE(t.status,''),
		       COALESCE(t.work_date,''), COALESCE(t.complete_date,''), COALESCE(t.complete_note,'')
		FROM work_task_sites w
		JOIN work_tasks t ON t.task_id=w.task_id
		JOIN project_sites s ON s.project_id=? AND s.is_active=1
		  AND (
		    (TRIM(COALESCE(w.project_site_id,'')) != '' AND s.id=w.project_site_id)
		    OR (TRIM(COALESCE(w.project_site_id,'')) = '' AND s.customer_id=w.customer_id)
		  )
		WHERE t.work_type=? AND t.project_id=?
		ORDER BY s.id,
		         COALESCE(NULLIF(t.complete_date,''), NULLIF(t.work_date,''), t.updated_at) DESC`,
		projectID, model.WBWorkProjectWork, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it SiteWorkItem
		if err := rows.Scan(&it.SiteID, &it.TaskID, &it.Title, &it.WorkSubtype, &it.Status,
			&it.WorkDate, &it.CompleteDate, &it.CompleteNote); err != nil {
			return nil, err
		}
		out[it.SiteID] = append(out[it.SiteID], it)
	}
	return out, rows.Err()
}

func (r *WBRepo) SyncSitesOnProjectWorkComplete(t *model.WorkTask, userID string) error {
	if t == nil || t.WorkType != model.WBWorkProjectWork || t.Status != model.WBTaskComplete {
		return nil
	}
	if strings.TrimSpace(t.ProjectID) == "" {
		return nil
	}
	sites, err := r.ListTaskSites(t.TaskID)
	if err != nil {
		return err
	}
	pe := NewProjectEquipRepo(r.db)
	var ids []string
	for _, s := range sites {
		psid := strings.TrimSpace(s.ProjectSiteID)
		if psid == "" {
			continue
		}
		cur, err := pe.GetSite(psid)
		if err != nil || cur == nil {
			continue
		}
		if !model.SiteAcceptsWorkCompleteStatus(cur.Status) {
			continue
		}
		ids = append(ids, psid)
	}
	if len(ids) == 0 {
		return nil
	}
	eventDate := strings.TrimSpace(t.CompleteDate)
	if eventDate == "" {
		eventDate = strings.TrimSpace(t.WorkDate)
	}
	note := "일일업무 완료"
	if lab := model.WorkSubtypeLabel(t.WorkSubtype); lab != "" {
		note += " · " + lab
	}
	if strings.TrimSpace(t.Title) != "" {
		note += " · " + strings.TrimSpace(t.Title)
	}
	if strings.TrimSpace(t.CompleteNote) != "" {
		note += " · " + strings.TrimSpace(t.CompleteNote)
	}
	return pe.applyBulkSiteStatus(t.ProjectID, userID, model.WorkCompleteSiteStatus(), eventDate, note, "ui", ids)
}

// ListWorkPickProjects 작업 폼 사업 후보. 95-9A Q3: 조직과 무관하게 취소·종료가 아닌 계약 사업, 구축·공사 우선.
func (r *WBRepo) ListWorkPickProjects(orgID string) ([]model.WorkProject, error) {
	_ = orgID
	rows, err := r.db.Query(`
		SELECT p.project_id, p.name, COALESCE(p.project_kind,''), COALESCE(p.status,'active')
		FROM work_projects p
		WHERE COALESCE(p.status,'') NOT IN ('archived','complete','cancelled')
		ORDER BY CASE COALESCE(p.project_kind,'')
			WHEN 'build' THEN 0 WHEN 'construction' THEN 1 ELSE 2 END, p.name`)
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

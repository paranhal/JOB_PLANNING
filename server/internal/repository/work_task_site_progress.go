package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"customer-support/internal/model"
)

type SiteProgressCell struct {
	CustomerID string
	SiteName   string
	WorkDate   string
	Status     string
}

type ProjectSiteWorkRow struct {
	CustomerID string
	SiteName   string
	Region     string
	Done       int
	Total      int
}

func (r *ProjectEquipRepo) AssignedDuty(projectID, userID, duty string) bool {
	projectID = strings.TrimSpace(projectID)
	userID = strings.TrimSpace(userID)
	duty = strings.TrimSpace(duty)
	if r == nil || r.db == nil || projectID == "" || userID == "" || duty == "" {
		return false
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_assignments WHERE project_id=? AND user_id=? AND duty=?`,
		projectID, userID, duty).Scan(&n)
	return n > 0
}

func (r *ProjectEquipRepo) CountActiveSites(projectID string) int {
	projectID = strings.TrimSpace(projectID)
	if r == nil || r.db == nil || projectID == "" {
		return 0
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_sites
		WHERE project_id=? AND is_active=1 AND status != 'excluded'`, projectID).Scan(&n)
	return n
}

func (r *WBRepo) SearchProjectSitePool(projectID, q string, limit int) ([]WorkTaskSite, error) {
	projectID = strings.TrimSpace(projectID)
	q = strings.TrimSpace(q)
	if projectID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if utf8.RuneCountInString(q) < 2 {
		return nil, nil
	}
	like := "%" + escapeLike(q) + "%"
	rows, err := r.db.Query(`
		SELECT ps.customer_id, COALESCE(NULLIF(TRIM(ps.site_name),''), c.org_name),
		       COALESCE(p.org_name,''),
		       COALESCE(NULLIF(TRIM(c.addr_sigungu),''), COALESCE(c.addr_sido,''))
		FROM project_sites ps
		JOIN customers c ON c.customer_id=ps.customer_id
		LEFT JOIN customers p ON p.customer_id=c.parent_customer_id
		WHERE ps.project_id=? AND ps.is_active=1 AND ps.status != 'excluded'
		  AND (
		    c.org_name LIKE ? ESCAPE '\' OR COALESCE(c.official_name,'') LIKE ? ESCAPE '\'
		    OR COALESCE(c.org_code,'') LIKE ? ESCAPE '\'
		    OR COALESCE(p.org_name,'') LIKE ? ESCAPE '\'
		    OR COALESCE(c.addr_sigungu,'') LIKE ? ESCAPE '\'
		    OR COALESCE(ps.site_name,'') LIKE ? ESCAPE '\'
		  )
		ORDER BY COALESCE(NULLIF(TRIM(ps.site_name),''), c.org_name)
		LIMIT ?`, projectID, like, like, like, like, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkTaskSite
	for rows.Next() {
		var it WorkTaskSite
		if err := rows.Scan(&it.CustomerID, &it.SiteName, &it.ParentName, &it.Region); err != nil {
			return nil, err
		}
		it.SiteID = it.CustomerID
		it.OnProject = true
		out = append(out, it)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func (r *WBRepo) TrackProgress(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if r == nil || r.db == nil || taskID == "" {
		return true
	}
	var n sql.NullInt64
	err := r.db.QueryRow(`SELECT track FROM work_task_progress_flag WHERE task_id=?`, taskID).Scan(&n)
	if err != nil {
		return true
	}
	return !n.Valid || n.Int64 != 0
}

func (r *WBRepo) SetTrackProgress(taskID string, track bool) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("업무가 없습니다")
	}
	v := 1
	if !track {
		v = 0
	}
	_, err := r.db.Exec(`INSERT INTO work_task_progress_flag(task_id, track) VALUES(?,?)
		ON CONFLICT(task_id) DO UPDATE SET track=excluded.track`, taskID, v)
	return err
}

func (r *WBRepo) UpsertSiteCheck(taskID, customerID, workDate, status, userID string) error {
	return r.UpsertSiteCheckQty(taskID, customerID, workDate, status, userID, -1)
}

func (r *WBRepo) UpsertSiteCheckQty(taskID, customerID, workDate, status, userID string, qty int) error {
	taskID = strings.TrimSpace(taskID)
	customerID = strings.TrimSpace(customerID)
	workDate = strings.TrimSpace(workDate)
	status = strings.TrimSpace(status)
	if taskID == "" || customerID == "" || workDate == "" {
		return fmt.Errorf("사이트와 날짜가 필요합니다")
	}
	if !model.ValidSiteCheckStatus(status) {
		return fmt.Errorf("완료 구분을 확인하세요")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Format(time.RFC3339)
	checkedBy := strings.TrimSpace(userID)
	if status == model.SiteCheckPending {
		now = ""
		checkedBy = ""
	}
	if _, err := tx.Exec(`INSERT INTO work_task_site_progress(task_id,customer_id,work_date,status,checked_by,checked_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(task_id,customer_id,work_date) DO UPDATE SET
		  status=excluded.status, checked_by=excluded.checked_by, checked_at=excluded.checked_at`,
		taskID, customerID, workDate, status, checkedBy, now); err != nil {
		return err
	}
	if err := reflectPackageSiteTx(tx, taskID, customerID, workDate, status, userID, qty); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *WBRepo) ListSiteChecks(taskID string) ([]SiteProgressCell, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT p.customer_id, COALESCE(NULLIF(TRIM(ps.site_name),''), c.org_name, p.customer_id),
		       p.work_date, p.status
		FROM work_task_site_progress p
		LEFT JOIN work_tasks t ON t.task_id=p.task_id
		LEFT JOIN customers c ON c.customer_id=p.customer_id
		LEFT JOIN project_sites ps ON ps.project_id=t.project_id AND ps.customer_id=p.customer_id AND ps.is_active=1
		WHERE p.task_id=?
		ORDER BY p.work_date, 2`, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []SiteProgressCell
	for rows.Next() {
		var it SiteProgressCell
		if err := rows.Scan(&it.CustomerID, &it.SiteName, &it.WorkDate, &it.Status); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *WBRepo) progressFamilyIDs(taskID string) (root string, ids []string) {
	taskID = strings.TrimSpace(taskID)
	root = taskID
	if taskID == "" || r == nil || r.db == nil {
		if taskID != "" {
			ids = []string{taskID}
		}
		return root, ids
	}
	var role, parent string
	_ = r.db.QueryRow(`SELECT COALESCE(recurrence_role,''), COALESCE(parent_task_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&role, &parent)
	if role == model.RecurrenceRoleOccurrence && strings.TrimSpace(parent) != "" {
		root = strings.TrimSpace(parent)
	}
	ids = []string{root}
	rows, err := r.db.Query(`SELECT task_id FROM work_tasks WHERE parent_task_id=? AND COALESCE(recurrence_role,'')=?`,
		root, model.RecurrenceRoleOccurrence)
	if err != nil {
		return root, ids
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return root, ids
		}
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return root, ids
}

func (r *WBRepo) ListSiteChecksFamily(taskID string) ([]SiteProgressCell, error) {
	_, ids := r.progressFamilyIDs(taskID)
	if len(ids) == 0 {
		return nil, nil
	}
	var out []SiteProgressCell
	seen := map[string]bool{}
	for _, id := range ids {
		rows, err := r.ListSiteChecks(id)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			k := row.CustomerID + "|" + row.WorkDate + "|" + row.Status
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *WBRepo) TaskSiteCheckRate(taskID, workDate, dueDate string, sites []WorkTaskSite) (done, total int, label string) {
	root, _ := r.progressFamilyIDs(taskID)
	if r == nil || !r.TrackProgress(root) {
		return 0, 0, "—"
	}
	if len(sites) == 0 {
		sites, _ = r.ListTaskSites(root)
	}
	if len(sites) == 0 {
		return 0, 0, "—"
	}
	_ = workDate
	_ = dueDate
	checks, _ := r.ListSiteChecksFamily(taskID)
	doneSite := map[string]bool{}
	naSite := map[string]bool{}
	otherSite := map[string]bool{}
	for _, c := range checks {
		cid := strings.TrimSpace(c.CustomerID)
		if cid == "" {
			continue
		}
		switch c.Status {
		case model.SiteCheckDone:
			doneSite[cid] = true
		case model.SiteCheckNA:
			naSite[cid] = true
		default:
			otherSite[cid] = true
		}
	}
	for _, s := range sites {
		cid := strings.TrimSpace(s.CustomerID)
		if cid == "" {
			cid = strings.TrimSpace(s.SiteID)
		}
		if cid == "" {
			continue
		}
		if doneSite[cid] {
			total++
			done++
			continue
		}
		if naSite[cid] && !otherSite[cid] {
			continue
		}
		total++
	}
	return done, total, model.SiteCheckRateLabel(done, total)
}

func (r *WBRepo) ListProjectSiteWorkProgress(projectID string) ([]ProjectSiteWorkRow, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT ps.customer_id, COALESCE(NULLIF(TRIM(ps.site_name),''), c.org_name),
		       COALESCE(ps.region,''),
		       COUNT(DISTINCT t.task_id),
		       COUNT(DISTINCT CASE WHEN p.task_id IS NOT NULL THEN t.task_id END)
		FROM project_sites ps
		JOIN customers c ON c.customer_id=ps.customer_id
		LEFT JOIN work_task_sites w ON w.customer_id=ps.customer_id
		LEFT JOIN work_tasks t ON t.task_id=w.task_id AND t.project_id=ps.project_id
		  AND COALESCE(t.work_type,'') NOT IN ('as','maintenance')
		  AND COALESCE(t.source_type,'') NOT IN ('as','maintenance')
		  AND COALESCE(t.recurrence_role,'') != 'occurrence'
		  AND NOT EXISTS (SELECT 1 FROM work_task_progress_flag f WHERE f.task_id=t.task_id AND f.track=0)
		LEFT JOIN work_task_site_progress p ON p.customer_id=ps.customer_id AND p.status='done'
		  AND (
		    p.task_id=t.task_id
		    OR p.task_id IN (
		      SELECT o.task_id FROM work_tasks o
		      WHERE o.parent_task_id=t.task_id AND COALESCE(o.recurrence_role,'')='occurrence'
		    )
		  )
		WHERE ps.project_id=? AND ps.is_active=1 AND ps.status != 'excluded'
		GROUP BY ps.customer_id
		ORDER BY 2`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []ProjectSiteWorkRow
	for rows.Next() {
		var it ProjectSiteWorkRow
		var linked, done sql.NullInt64
		if err := rows.Scan(&it.CustomerID, &it.SiteName, &it.Region, &linked, &done); err != nil {
			return nil, err
		}
		it.Total = int(linked.Int64)
		it.Done = int(done.Int64)
		out = append(out, it)
	}
	return out, rows.Err()
}

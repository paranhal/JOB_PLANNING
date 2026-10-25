package repository

import (
	"strings"

	"customer-support/internal/model"
)

func (r *ASRepo) SearchOpenASForApply(orgID, q, exclude, assigneeUserID string, limit int) ([]WorkTaskSite, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	exclude = strings.TrimSpace(exclude)
	assigneeUserID = strings.TrimSpace(assigneeUserID)

	where := ` WHERE 1=1`
	args := []interface{}{}
	where, args = appendOrg(where, args, "ar", orgID)
	if exclude != "" {
		where += ` AND ar.as_id != ?`
		args = append(args, exclude)
	}
	where += ` AND ar.status NOT IN ('completed','closed','cancelled',?)`
	args = append(args, model.StatusAdminWork)
	if assigneeUserID != "" {
		where += ` AND (ar.assigned_user_id = ?)`
		args = append(args, assigneeUserID)
	}
	q = strings.TrimSpace(q)
	if q != "" {
		like := "%" + q + "%"
		where += ` AND (c.org_name LIKE ? OR ar.as_number LIKE ?)`
		args = append(args, like, like)
	}
	args = append(args, limit)
	rows, err := r.db.Query(`
		SELECT ar.as_id, COALESCE(c.org_name,'') || ' · ' || COALESCE(ar.as_number,'')
		  FROM as_receipts ar
		  JOIN customers c ON c.customer_id = ar.customer_id
		`+where+`
		 ORDER BY ar.updated_at DESC
		 LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkTaskSite
	for rows.Next() {
		var s WorkTaskSite
		if err := rows.Scan(&s.SiteID, &s.SiteName); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ASRepo) ListASForApplySelected(orgID string, ids []string, exclude string) ([]WorkTaskSite, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var out []WorkTaskSite
	seen := map[string]bool{}
	exclude = strings.TrimSpace(exclude)
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || id == exclude || seen[id] {
			continue
		}
		seen[id] = true
		as, err := r.GetByID(orgID, id)
		if err != nil || as == nil {
			continue
		}
		out = append(out, WorkTaskSite{
			SiteID:   as.ASID,
			SiteName: strings.TrimSpace(as.OrgName) + " · " + strings.TrimSpace(as.ASNumber),
		})
	}
	return out, nil
}

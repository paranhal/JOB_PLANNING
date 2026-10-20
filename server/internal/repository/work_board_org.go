package repository

import (
	"strings"

	"customer-support/internal/model"
)

func (r *WorkBoardRepo) ListBucketOnForOrg(bucket, date, mineUserID string, mineKeys []string, limit int, orgID string) ([]model.WorkListItem, error) {
	items, err := r.ListBucketOn(bucket, date, mineUserID, mineKeys, limit)
	if err != nil {
		return nil, err
	}
	return r.filterWorkListForOrg(items, orgID)
}

func (r *WorkBoardRepo) filterWorkListForOrg(items []model.WorkListItem, orgID string) ([]model.WorkListItem, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll || len(items) == 0 {
		return items, nil
	}
	var asIDs, visitIDs, taskIDs []string
	for _, it := range items {
		id := strings.TrimSpace(it.RefID)
		if id == "" {
			continue
		}
		switch it.Prefix {
		case model.WorkPrefixAS, model.WorkPrefixConfirm, model.WorkPrefixSupport:
			asIDs = append(asIDs, id)
		case model.WorkPrefixMaintenance:
			visitIDs = append(visitIDs, id)
		default:
			taskIDs = append(taskIDs, id)
		}
	}
	asOK := r.orgIDSet(`SELECT as_id FROM as_receipts WHERE as_id IN (`+placeholders(len(asIDs))+`) AND org_id=?`, append(iface(asIDs), orgID)...)
	visitOK := r.orgIDSet(`SELECT mv.visit_id FROM maintenance_visits mv
		JOIN customers c ON c.customer_id=mv.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id=mv.customer_id
		WHERE mv.visit_id IN (`+placeholders(len(visitIDs))+`)
		  AND COALESCE(NULLIF(TRIM(cfg.org_id),''), NULLIF(TRIM(c.org_id),''))=?`,
		append(iface(visitIDs), orgID)...)
	taskOK := map[string]bool{}
	if len(taskIDs) > 0 {
		frag, extra := workTaskVisibleOrgSQL("t", orgID)
		q := `SELECT t.task_id FROM work_tasks t WHERE t.task_id IN (` + placeholders(len(taskIDs)) + `)` + frag
		taskOK = r.orgIDSet(q, append(iface(taskIDs), extra...)...)
	}
	out := make([]model.WorkListItem, 0, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.RefID)
		keep := false
		switch it.Prefix {
		case model.WorkPrefixAS, model.WorkPrefixConfirm, model.WorkPrefixSupport:
			keep = asOK[id]
		case model.WorkPrefixMaintenance:
			keep = visitOK[id]
		default:
			keep = taskOK[id]
		}
		if keep {
			out = append(out, it)
		}
	}
	return out, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	s := strings.Repeat("?,", n)
	return s[:len(s)-1]
}

func iface(ids []string) []interface{} {
	out := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		out = append(out, id)
	}
	return out
}

func (r *WorkBoardRepo) orgIDSet(q string, args ...interface{}) map[string]bool {
	out := map[string]bool{}
	if r == nil || r.db == nil || strings.Contains(q, "IN (NULL)") {
		return out
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && strings.TrimSpace(id) != "" {
			out[id] = true
		}
	}
	return out
}

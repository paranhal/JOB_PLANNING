package repository

import (
	"strings"

	"customer-support/internal/model"
)

// ListBySiteCustomer 이 고객이 활성 사이트로 등록된 사업.
// includeID 가 닫힌 사업이어도 목록에 남긴다. recency 는 project_sites.rowid(최근 등록).
func (r *ProjectRepo) ListBySiteCustomer(customerID, includeID string) ([]model.WorkProject, map[string]int64, error) {
	recency := map[string]int64{}
	if r == nil || r.db == nil {
		return nil, recency, nil
	}
	customerID = strings.TrimSpace(customerID)
	includeID = strings.TrimSpace(includeID)
	if customerID == "" {
		return nil, recency, nil
	}
	rows, err := r.db.Query(`SELECT project_id, MAX(rowid) FROM project_sites
		WHERE customer_id=? AND is_active=1 GROUP BY project_id`, customerID)
	if err != nil {
		return nil, recency, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		var rid int64
		if err := rows.Scan(&id, &rid); err != nil {
			return nil, recency, err
		}
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		recency[id] = rid
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, recency, err
	}
	if len(ids) == 0 {
		return nil, recency, nil
	}
	args := make([]interface{}, 0, len(ids))
	ph := make([]string, 0, len(ids))
	for _, id := range ids {
		ph = append(ph, "?")
		args = append(args, id)
	}
	q := projectSelect + ` WHERE p.project_id IN (` + strings.Join(ph, ",") + `)`
	prows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, recency, err
	}
	defer prows.Close()
	items, err := scanProjectRows(prows)
	if err != nil {
		return nil, recency, err
	}
	var out []model.WorkProject
	for _, p := range items {
		if projectRowClosed(p.Status) && p.ProjectID != includeID {
			continue
		}
		out = append(out, p)
	}
	return out, recency, nil
}

// ListBySiteCustomers 여러 고객의 활성 사이트 등록 사업을 합친다. 95-148
func (r *ProjectRepo) ListBySiteCustomers(customerIDs []string, includeID string) ([]model.WorkProject, map[string]int64, error) {
	recency := map[string]int64{}
	ids := uniqueNonEmpty(customerIDs)
	if r == nil || r.db == nil || len(ids) == 0 {
		return nil, recency, nil
	}
	if len(ids) == 1 {
		return r.ListBySiteCustomer(ids[0], includeID)
	}
	ph := strings.Repeat("?,", len(ids))
	ph = ph[:len(ph)-1]
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.Query(`SELECT project_id, MAX(rowid) FROM project_sites
		WHERE customer_id IN (`+ph+`) AND is_active=1 GROUP BY project_id`, args...)
	if err != nil {
		return nil, recency, err
	}
	defer rows.Close()
	var pids []string
	for rows.Next() {
		var id string
		var rid int64
		if err := rows.Scan(&id, &rid); err != nil {
			return nil, recency, err
		}
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		recency[id] = rid
		pids = append(pids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, recency, err
	}
	if len(pids) == 0 {
		return nil, recency, nil
	}
	args = make([]interface{}, 0, len(pids))
	phs := make([]string, 0, len(pids))
	for _, id := range pids {
		phs = append(phs, "?")
		args = append(args, id)
	}
	q := projectSelect + ` WHERE p.project_id IN (` + strings.Join(phs, ",") + `)`
	prows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, recency, err
	}
	defer prows.Close()
	items, err := scanProjectRows(prows)
	if err != nil {
		return nil, recency, err
	}
	includeID = strings.TrimSpace(includeID)
	var out []model.WorkProject
	for _, p := range items {
		if projectRowClosed(p.Status) && p.ProjectID != includeID {
			continue
		}
		out = append(out, p)
	}
	return out, recency, nil
}

// CustomerOnActiveSite 이 고객이 해당 사업의 활성 사이트로 등록돼 있는지.
func (r *ProjectRepo) CustomerOnActiveSite(customerID, projectID string) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}
	customerID = strings.TrimSpace(customerID)
	projectID = strings.TrimSpace(projectID)
	if customerID == "" || projectID == "" {
		return false, nil
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM project_sites
		WHERE customer_id=? AND project_id=? AND is_active=1`, customerID, projectID).Scan(&n)
	return n > 0, err
}

func projectRowClosed(status string) bool {
	switch strings.TrimSpace(status) {
	case model.WBProjectArchived, model.WBProjectComplete, "cancelled":
		return true
	default:
		return false
	}
}

package repository

import (
	"strings"
	"unicode/utf8"

	"customer-support/internal/model"
)

// ASRegisterHit 일일업무 등록 AS 검색 한 건. 95-7
type ASRegisterHit struct {
	ASID       string
	ASNumber   string
	OrgName    string
	CustomerID string
	Symptom    string
	Status     string
	AssignedTo string
}

// LookupForRegister 현재 조직 AS를 이름·접수번호·증상으로 찾는다. 최대 20건.
func (r *ASRepo) LookupForRegister(orgID, q string, limit int) ([]ASRegisterHit, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	orgID = strings.TrimSpace(orgID)
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) < 2 {
		return []ASRegisterHit{}, nil
	}
	if err := RequireOrgID(orgID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	sqlq := `
		SELECT ar.as_id, ar.as_number, c.org_name, ar.customer_id, COALESCE(ar.symptom,''),
		       ar.status, COALESCE(ar.assigned_to,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE 1=1`
	orgFrag, orgArgs, err := AppendOrgSQL("ar", orgID)
	if err != nil {
		return nil, err
	}
	sqlq += orgFrag
	pat := likeContains(q)
	sqlq += ` AND (ar.as_number LIKE ? ESCAPE '\' OR c.org_name LIKE ? ESCAPE '\' OR ar.symptom LIKE ? ESCAPE '\')
		ORDER BY ar.receipt_datetime DESC
		LIMIT ?`
	args := append([]any{}, orgArgs...)
	args = append(args, pat, pat, pat, limit)
	rows, err := r.db.Query(sqlq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ASRegisterHit{}
	for rows.Next() {
		var h ASRegisterHit
		if err := rows.Scan(&h.ASID, &h.ASNumber, &h.OrgName, &h.CustomerID, &h.Symptom, &h.Status, &h.AssignedTo); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ExactCustomerIDsByOrgName 현재 조직에서 기관명이 정확히 일치하는 고객 ID.
func (r *CustomerRepo) ExactCustomerIDsByOrgName(orgID, name string) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	if err := RequireOrgID(orgID); err != nil {
		return nil, err
	}
	q := `SELECT customer_id FROM customers WHERE is_active=1 AND TRIM(org_name)=?`
	args := []any{name}
	orgFrag, orgArgs, err := AppendOrgSQL("", orgID)
	if err != nil {
		return nil, err
	}
	q += orgFrag + ` ORDER BY customer_id LIMIT 3`
	args = append(args, orgArgs...)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// LatestPlacedASTasks 해당 AS로 만든, 배정일이 있는 일일업무(AS당 1건).
func (r *WBRepo) LatestPlacedASTasks(asIDs []string) (map[string]model.WorkTask, error) {
	out := map[string]model.WorkTask{}
	if r == nil || r.db == nil || len(asIDs) == 0 {
		return out, nil
	}
	ph := make([]string, len(asIDs))
	args := make([]any, 0, len(asIDs)+1)
	args = append(args, model.WBSourceAS)
	for i, id := range asIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	q := `
		SELECT t.task_id, t.source_id, COALESCE(t.assignee,''), COALESCE(t.work_date,''),
		       COALESCE(t.start_time,''), COALESCE(t.end_time,'')
		FROM work_tasks t
		WHERE t.source_type = ?
		  AND t.source_id IN (` + strings.Join(ph, ",") + `)
		  AND TRIM(COALESCE(t.work_date,'')) != ''
		ORDER BY t.work_date DESC, t.task_id DESC`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t model.WorkTask
		if err := rows.Scan(&t.TaskID, &t.SourceID, &t.Assignee, &t.WorkDate, &t.StartTime, &t.EndTime); err != nil {
			return nil, err
		}
		if _, ok := out[t.SourceID]; ok {
			continue
		}
		out[t.SourceID] = t
	}
	return out, rows.Err()
}

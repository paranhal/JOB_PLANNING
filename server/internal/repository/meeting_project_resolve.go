package repository

import (
	"strings"

	"customer-support/internal/model"
)

// ListActiveSiteProjectIDsByCustomers 고객(사이트)별 활성·비테스트 사업 ID. 한 쿼리. 95-145
func (r *WBRepo) ListActiveSiteProjectIDsByCustomers(customerIDs []string) map[string][]string {
	out := map[string][]string{}
	if r == nil || r.db == nil {
		return out
	}
	seenCust := map[string]bool{}
	var ids []string
	for _, id := range customerIDs {
		id = strings.TrimSpace(id)
		if id == "" || seenCust[id] {
			continue
		}
		seenCust[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out
	}
	ph := strings.Repeat("?,", len(ids))
	ph = ph[:len(ph)-1]
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT s.customer_id, s.project_id
		FROM project_sites s
		LEFT JOIN work_projects p ON p.project_id = s.project_id
		WHERE s.is_active=1 AND COALESCE(p.is_test,0)=0
		  AND TRIM(COALESCE(s.customer_id,'')) != ''
		  AND s.customer_id IN (`+ph+`)
		ORDER BY s.customer_id, s.project_id`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	dup := map[string]map[string]bool{}
	for rows.Next() {
		var cid, pid string
		if err := rows.Scan(&cid, &pid); err != nil {
			return out
		}
		cid, pid = strings.TrimSpace(cid), strings.TrimSpace(pid)
		if cid == "" || pid == "" {
			continue
		}
		if dup[cid] == nil {
			dup[cid] = map[string]bool{}
		}
		if dup[cid][pid] {
			continue
		}
		dup[cid][pid] = true
		out[cid] = append(out[cid], pid)
	}
	return out
}

func (r *WBRepo) WorkProjectNames(ids []string) map[string]string {
	out := map[string]string{}
	if r == nil || r.db == nil {
		return out
	}
	seen := map[string]bool{}
	var uniq []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return out
	}
	ph := strings.Repeat("?,", len(uniq))
	ph = ph[:len(ph)-1]
	args := make([]interface{}, len(uniq))
	for i, id := range uniq {
		args[i] = id
	}
	rows, err := r.db.Query(`SELECT project_id, COALESCE(name,'') FROM work_projects WHERE project_id IN (`+ph+`)`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return out
		}
		out[strings.TrimSpace(id)] = strings.TrimSpace(name)
	}
	return out
}

// ResolveMeetingDisplayProject 저장된 project_id 우선. 비어 있으면 사이트 등록 1건만 보완. 소급 저장 없음. 95-145
func ResolveMeetingDisplayProject(it model.WorkListItem, siteProjects map[string][]string) (pid string, viaSite, needSelect bool) {
	pid = strings.TrimSpace(it.ProjectID)
	if pid != "" {
		return pid, false, false
	}
	cands := siteProjects[strings.TrimSpace(it.CustomerID)]
	if len(cands) == 1 {
		return cands[0], true, false
	}
	return "", false, true
}

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

// MeetingProjectMeta 사업별 보기 머리용. 병합하지 않고 번호·연도·발주처·사이트 수로 구분. 95-149
type MeetingProjectMeta struct {
	Name          string
	PlanYear      int
	OrderingParty string
	Status        string
	SiteCount     int
}

func (r *WBRepo) MeetingProjectMeta(ids []string) map[string]MeetingProjectMeta {
	out := map[string]MeetingProjectMeta{}
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
	rows, err := r.db.Query(`
		SELECT p.project_id, COALESCE(p.name,''), COALESCE(p.plan_year,0),
		       COALESCE(NULLIF(TRIM(p.ordering_party),''), COALESCE(c.org_name,'')),
		       COALESCE(p.status,''),
		       (SELECT COUNT(*) FROM project_sites s
		         WHERE s.project_id=p.project_id AND COALESCE(s.is_active,1)=1)
		FROM work_projects p
		LEFT JOIN customers c ON c.customer_id=p.ordering_party_id
		WHERE p.project_id IN (`+ph+`)`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var m MeetingProjectMeta
		if err := rows.Scan(&id, &m.Name, &m.PlanYear, &m.OrderingParty, &m.Status, &m.SiteCount); err != nil {
			return out
		}
		id = strings.TrimSpace(id)
		m.Name = strings.TrimSpace(m.Name)
		m.OrderingParty = strings.TrimSpace(m.OrderingParty)
		m.Status = strings.TrimSpace(m.Status)
		out[id] = m
	}
	return out
}

// ResolveMeetingDisplayProject 저장된 project_id 우선. 비어 있으면 사이트 등록 1건만 보완. 소급 저장 없음. 95-145
// needSelect는 후보가 2개 이상일 때만. 0건·고객 ID 없음은 사업 미연결. 95-149
func ResolveMeetingDisplayProject(it model.WorkListItem, siteProjects map[string][]string) (pid string, viaSite, needSelect bool) {
	pid = strings.TrimSpace(it.ProjectID)
	if pid != "" {
		return pid, false, false
	}
	cands := siteProjects[strings.TrimSpace(it.CustomerID)]
	if len(cands) == 1 {
		return cands[0], true, false
	}
	if len(cands) >= 2 {
		return "", false, true
	}
	return "", false, false
}

package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// 95-218 ①②: 방문(정기점검)·AS 의 「이 사업에 연결해도 되는가」와 「연결 후보는 무엇인가」를 한 곳에서 정한다.
// 기준은 둘이다. 둘 중 하나라도 맞으면 인정한다.
//   (가) 사업 사이트(project_sites): 그 고객 또는 상위 기관이 그 사업의 활성 사이트다.
//   (나) 범위 규칙(project_scope_rules): 방문 자동 판정(ResolveProjectID)이 가리키는 사업이다.
// 이미 그 사업이 연결돼 있으면(다) 그대로 통과한다.

// VisitProjectCandidate 연결 후보 한 건.
type VisitProjectCandidate struct {
	ProjectID string
	Name      string
	BySite    bool // 사업 사이트로 등록돼 있다
	ByRule    bool // 범위 규칙이 가리킨다
}

// VisitProjectCandidates 사이트가 등록된 사업을 먼저, 범위 규칙 사업을 뒤에 돌려준다(중복 제거).
func (r *ProjectRepo) VisitProjectCandidates(customerID, productType, workKind string) ([]VisitProjectCandidate, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, nil
	}
	var out []VisitProjectCandidate
	idx := map[string]int{}
	add := func(id, name string, site, rule bool) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if i, ok := idx[id]; ok {
			out[i].BySite = out[i].BySite || site
			out[i].ByRule = out[i].ByRule || rule
			return
		}
		idx[id] = len(out)
		out = append(out, VisitProjectCandidate{ProjectID: id, Name: name, BySite: site, ByRule: rule})
	}
	anc, err := r.customerAncestorIDs(customerID)
	if err != nil {
		return nil, err
	}
	if len(anc) > 0 {
		args := make([]interface{}, 0, len(anc))
		for _, a := range anc {
			args = append(args, a)
		}
		rows, err := r.db.Query(`
			SELECT DISTINCT p.project_id, p.name
			  FROM project_sites ps JOIN work_projects p ON p.project_id = ps.project_id
			 WHERE ps.customer_id IN (`+placeholders(len(anc))+`) AND ps.is_active=1 AND COALESCE(ps.status,'') != 'excluded'
			   AND COALESCE(p.status,'') NOT IN ('archived','complete','cancelled') AND COALESCE(p.is_test,0)=0
			 ORDER BY p.name, p.project_id`, args...)
		if err != nil && !strings.Contains(err.Error(), "no such table") {
			return nil, err
		}
		if err == nil {
			for rows.Next() {
				var id, name string
				if rows.Scan(&id, &name) == nil {
					add(id, name, true, false)
				}
			}
			rows.Close()
		}
	}
	if pid, err := r.ResolveProjectID(customerID, productType, workKind); err == nil && pid != "" {
		var name string
		_ = r.db.QueryRow(`SELECT COALESCE(name,'') FROM work_projects WHERE project_id=?`, pid).Scan(&name)
		add(pid, name, false, true)
	}
	return out, nil
}

// VisitMayLinkProject 방문·AS 를 projectID 사업에 연결해도 되는가. 거절이면 이유(사용자에게 보일 문장)를 함께 돌려준다.
func (r *ProjectRepo) VisitMayLinkProject(customerID, productType, workKind, projectID, existingID string) (bool, string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || projectID == strings.TrimSpace(existingID) {
		return true, "", nil
	}
	cands, err := r.VisitProjectCandidates(customerID, productType, workKind)
	if err != nil {
		return false, "", err
	}
	for _, c := range cands {
		if c.ProjectID == projectID {
			return true, "", nil
		}
	}
	var site, proj string
	_ = r.db.QueryRow(`SELECT COALESCE(org_name,'') FROM customers WHERE customer_id=?`, customerID).Scan(&site)
	_ = r.db.QueryRow(`SELECT COALESCE(name,'') FROM work_projects WHERE project_id=?`, projectID).Scan(&proj)
	if site == "" {
		site = "이 사이트"
	}
	if proj == "" {
		proj = "이 사업"
	}
	return false, fmt.Sprintf("이 사이트(%s)는 「%s」 사업의 사이트로 등록돼 있지 않습니다. 사업 상세의 사이트 등록에서 추가한 뒤 다시 연결하세요", site, proj), nil
}

// EnsureVisitProject 방문의 사업이 비어 있고 연결 후보가 정확히 1개면 그 사업을 채운다. 채웠으면 사업 ID 를 돌려준다.
// 후보가 여러 개이거나 없으면 건드리지 않는다. 다른 열은 바꾸지 않는다.
func (r *MaintenanceRepo) EnsureVisitProject(visitID string) (string, error) {
	if r == nil || r.db == nil {
		return "", nil
	}
	var cust, product, project string
	err := r.db.QueryRow(`SELECT COALESCE(customer_id,''), COALESCE(product_type,''), COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id=?`, visitID).Scan(&cust, &product, &project)
	if err == sql.ErrNoRows || strings.TrimSpace(project) != "" {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	cands, err := NewProjectRepo(r.db).VisitProjectCandidates(cust, product, model.ScopeWorkMaintenance)
	if err != nil || len(cands) != 1 {
		return "", err
	}
	before := rowJSON(r.db, "maintenance_visits", "visit_id", visitID)
	res, err := r.db.Exec(`UPDATE maintenance_visits SET project_id=? WHERE visit_id=? AND TRIM(COALESCE(project_id,''))=''`, cands[0].ProjectID, visitID)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", nil
	}
	logUpdateWithReason(r.db, "maintenance_visits", "visit_id", visitID, "방문 사업 자동 확정 "+visitID, before, "조치 완료 시 후보 1개 자동 저장(95-218)")
	_, _ = SyncTaskProjectsOfVisit(r.db, visitID, false) // 95-227
	return cands[0].ProjectID, nil
}

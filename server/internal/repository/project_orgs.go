package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"customer-support/internal/model"
)

func applyWorkProjectOrgsV100(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := workProjectOrgsMigrationSQL()
	if err != nil {
		log.Printf("100 work_project_orgs: %v", err)
		return
	}
	if err := execWorkGroupSQL(db, up); err != nil {
		log.Printf("100 work_project_orgs apply: %v", err)
	}
}

func workProjectOrgsMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("100_work_project_orgs.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("100_work_project_orgs.sql 없음")
}

func workProjectOrgsDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("100_work_project_orgs.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("100_work_project_orgs.down.sql 없음")
}

type ProjectRelatedOrg struct {
	OrgID   string
	OrgName string
}

func AssertProjectRegisterAccess(db *sql.DB, projectID, orgID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	p, err := NewProjectRepo(db).Get(projectID)
	if err != nil || p == nil {
		return fmt.Errorf("사업을 찾을 수 없습니다")
	}
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil
	}
	if !model.CanRegisterWorkOnProject(*p, orgID) {
		return fmt.Errorf("이 사업에 업무를 등록할 수 없습니다")
	}
	return nil
}

func (r *ProjectRepo) ListRelatedOrgs(projectID string) ([]model.Org, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT r.org_id, COALESCE(NULLIF(TRIM(o.org_name),''), r.org_id)
		  FROM work_project_orgs r
		  LEFT JOIN orgs o ON o.org_id=r.org_id
		 WHERE r.project_id=?
		 ORDER BY o.sort_order, r.org_id`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.Org
	for rows.Next() {
		var o model.Org
		if err := rows.Scan(&o.OrgID, &o.OrgName); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func stampRelated(p *model.WorkProject, orgs []model.Org) {
	if p == nil {
		return
	}
	p.RelatedOrgs = orgs
	p.RelatedOrgIDs = nil
	for _, o := range orgs {
		id := strings.TrimSpace(o.OrgID)
		if id == "" {
			continue
		}
		p.RelatedOrgIDs = append(p.RelatedOrgIDs, id)
	}
	p.RelatedLine = relatedLine(orgs)
}

func relatedLine(orgs []model.Org) string {
	if len(orgs) == 0 {
		return ""
	}
	name := strings.TrimSpace(orgs[0].OrgName)
	if name == "" {
		name = orgs[0].OrgID
	}
	if len(orgs) == 1 {
		return "유관: " + name
	}
	return fmt.Sprintf("유관: %s 외 %d", name, len(orgs)-1)
}

func (r *ProjectRepo) attachRelatedOrgs(items []model.WorkProject) {
	if r == nil || r.db == nil || len(items) == 0 {
		return
	}
	ids := make([]string, 0, len(items))
	idx := map[string][]int{}
	for i := range items {
		id := strings.TrimSpace(items[i].ProjectID)
		if id == "" {
			continue
		}
		if _, ok := idx[id]; !ok {
			ids = append(ids, id)
		}
		idx[id] = append(idx[id], i)
	}
	if len(ids) == 0 {
		return
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT r.project_id, r.org_id, COALESCE(NULLIF(TRIM(o.org_name),''), r.org_id)
		  FROM work_project_orgs r
		  LEFT JOIN orgs o ON o.org_id=r.org_id
		 WHERE r.project_id IN (`+ph+`)
		 ORDER BY r.project_id, o.sort_order, r.org_id`, args...)
	if err != nil {
		if !strings.Contains(err.Error(), "no such table") {
			log.Printf("attachRelatedOrgs: %v", err)
		}
		return
	}
	defer rows.Close()
	by := map[string][]model.Org{}
	for rows.Next() {
		var pid string
		var o model.Org
		if err := rows.Scan(&pid, &o.OrgID, &o.OrgName); err != nil {
			return
		}
		by[pid] = append(by[pid], o)
	}
	for pid, orgs := range by {
		for _, i := range idx[pid] {
			stampRelated(&items[i], orgs)
		}
	}
}

func (r *ProjectRepo) ReplaceRelatedOrgs(projectID, createdBy string, orgIDs []string, ownerOrg string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("project_id 필요")
	}
	ownerOrg = strings.TrimSpace(ownerOrg)
	clean, err := r.validateRelatedOrgIDs(orgIDs, ownerOrg)
	if err != nil {
		return err
	}
	before, _ := r.ListRelatedOrgs(projectID)
	beforeIDs := map[string]bool{}
	for _, o := range before {
		beforeIDs[o.OrgID] = true
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM work_project_orgs WHERE project_id=?`, projectID); err != nil {
		return err
	}
	for _, id := range clean {
		if _, err := tx.Exec(`INSERT INTO work_project_orgs(project_id, org_id, role, created_by) VALUES (?,?, 'related', ?)`,
			projectID, id, nullStr(createdBy)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	afterIDs := map[string]bool{}
	for _, id := range clean {
		afterIDs[id] = true
	}
	var added, removed []string
	for id := range afterIDs {
		if !beforeIDs[id] {
			added = append(added, id)
		}
	}
	for id := range beforeIDs {
		if !afterIDs[id] {
			removed = append(removed, id)
		}
	}
	reason := fmt.Sprintf("유관 부서 변경 추가=%s 제거=%s", strings.Join(added, ","), strings.Join(removed, ","))
	logUpdateWithReason(r.db, "work_projects", "project_id", projectID, "유관 부서", "", reason)
	return nil
}

func (r *ProjectRepo) validateRelatedOrgIDs(orgIDs []string, ownerOrg string) ([]string, error) {
	ownerOrg = strings.TrimSpace(ownerOrg)
	seen := map[string]bool{}
	var out []string
	for _, raw := range orgIDs {
		id := strings.TrimSpace(raw)
		if id == "" || id == OrgAll {
			continue
		}
		if id == ownerOrg {
			return nil, fmt.Errorf("주관 부서는 유관 부서로 넣을 수 없습니다")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		o, err := NewOrgRepo(r.db).Get(id)
		if err != nil || o == nil {
			return nil, fmt.Errorf("조직을 찾을 수 없습니다")
		}
		if !o.IsActive {
			return nil, fmt.Errorf("비활성 조직은 유관 부서로 넣을 수 없습니다")
		}
		out = append(out, id)
	}
	return out, nil
}

type ProjectOrgMoveOpts struct {
	ToOrg             string
	Reason            string
	ByUser            string
	MoveOpenTasks     bool
	KeepPrevAsRelated bool
	DropForeignAssign bool
}

type ProjectOrgMoveImpact struct {
	Tasks   int
	AS      int
	Mnt     int
	Assets  int
	Assigns int
}

func (r *ProjectRepo) OrgMoveImpact(projectID string) (ProjectOrgMoveImpact, error) {
	var out ProjectOrgMoveImpact
	id := strings.TrimSpace(projectID)
	if id == "" {
		return out, fmt.Errorf("project_id 필요")
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE project_id=? AND COALESCE(status,'') NOT IN ('complete','cancelled')`, id).Scan(&out.Tasks)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE project_id=?`, id).Scan(&out.AS)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM maintenance_visits WHERE project_id=?`, id).Scan(&out.Mnt)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE project_id=?`, id).Scan(&out.Assets)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_assignments WHERE project_id=?`, id).Scan(&out.Assigns)
	return out, nil
}

func (r *ProjectRepo) UpdateOrg(projectID string, opts ProjectOrgMoveOpts) error {
	p, err := r.Get(projectID)
	if err != nil || p == nil {
		return fmt.Errorf("사업이 없습니다")
	}
	from := strings.TrimSpace(p.OrgID)
	to := strings.TrimSpace(opts.ToOrg)
	reason := strings.TrimSpace(opts.Reason)
	if to == "" {
		return fmt.Errorf("소속 조직을 선택하세요")
	}
	if to == from {
		return fmt.Errorf("같은 팀으로는 이관할 수 없습니다")
	}
	if reason == "" {
		return fmt.Errorf("이관 사유를 입력하세요")
	}
	if len([]rune(reason)) > 200 {
		return fmt.Errorf("이관 사유는 200자까지입니다")
	}
	org, err := NewOrgRepo(r.db).Get(to)
	if err != nil || org == nil {
		return fmt.Errorf("대상 조직이 없습니다")
	}
	if !org.IsActive {
		return fmt.Errorf("비활성 조직으로는 이관할 수 없습니다")
	}
	if no := strings.TrimSpace(p.ContractNo); no != "" {
		var n int
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE org_id=? AND TRIM(COALESCE(contract_no,''))=? AND project_id!=?`, to, no, p.ProjectID).Scan(&n)
		if n > 0 {
			return fmt.Errorf("같은 팀의 사업번호가 이미 있습니다")
		}
	}
	impact, _ := r.OrgMoveImpact(p.ProjectID)
	before := rowJSON(r.db, "work_projects", "project_id", p.ProjectID)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE work_projects SET org_id=?, updated_at=CURRENT_TIMESTAMP WHERE project_id=?`, to, p.ProjectID); err != nil {
		return err
	}
	if sid := strings.TrimSpace(p.SalesProjectID); sid != "" {
		if _, err := tx.Exec(`UPDATE sales_projects SET org_id=? WHERE sales_id=?`, to, sid); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM work_project_orgs WHERE project_id=? AND org_id=?`, p.ProjectID, to); err != nil {
		return err
	}
	if opts.KeepPrevAsRelated && from != "" && from != to {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO work_project_orgs(project_id, org_id, role, created_by) VALUES (?,?,'related',?)`,
			p.ProjectID, from, nullStr(opts.ByUser)); err != nil {
			return err
		}
	}
	allowed := map[string]bool{to: true}
	if opts.KeepPrevAsRelated && from != "" {
		allowed[from] = true
	}
	relRows, _ := tx.Query(`SELECT org_id FROM work_project_orgs WHERE project_id=?`, p.ProjectID)
	if relRows != nil {
		for relRows.Next() {
			var oid string
			if relRows.Scan(&oid) == nil {
				allowed[strings.TrimSpace(oid)] = true
			}
		}
		relRows.Close()
	}
	if opts.MoveOpenTasks {
		if _, err := tx.Exec(`UPDATE work_tasks SET org_id=? WHERE project_id=? AND COALESCE(status,'') NOT IN ('complete','cancelled')`, to, p.ProjectID); err != nil {
			return err
		}
	}
	if opts.DropForeignAssign {
		if _, err := tx.Exec(`
			DELETE FROM project_assignments
			 WHERE project_id=?
			   AND user_id NOT IN (
			     SELECT user_id FROM users WHERE TRIM(COALESCE(org_id,'')) IN (`+sqlInOrgKeys(allowed)+`)
			   )`, append([]interface{}{p.ProjectID}, orgKeysArgs(allowed)...)...); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]interface{}{
		"from": from, "to": to, "reason": reason,
		"tasks": impact.Tasks, "as": impact.AS, "mnt": impact.Mnt, "assets": impact.Assets, "assigns": impact.Assigns,
		"move_open_tasks": opts.MoveOpenTasks, "keep_prev_related": opts.KeepPrevAsRelated, "drop_foreign_assign": opts.DropForeignAssign,
	})
	logUpdateWithReason(r.db, "work_projects", "project_id", p.ProjectID, p.Name, before, "주관 부서 이관 "+string(meta))
	return nil
}

func sqlInOrgKeys(m map[string]bool) string {
	if len(m) == 0 {
		return "?"
	}
	return strings.TrimRight(strings.Repeat("?,", len(m)), ",")
}

func orgKeysArgs(m map[string]bool) []interface{} {
	var out []interface{}
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

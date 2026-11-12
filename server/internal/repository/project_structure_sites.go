package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"

	"customer-support/internal/model"
)

// 95-179B: 1단계 업무·2단계 하부 업무에 사이트를 직접 등록한다.
//   - 1단계 업무(phase_sites): 그 사업의 사업 사이트 풀(project_sites, 제외 상태 제외)에서 고른다.
//   - 2단계 하부 업무(package_sites): 그 1단계 업무에 등록된 사이트(phase_sites) 중에서 고른다. 작업 계획 없이도
//     등록할 수 있고(plan_id=''), 계획에 담기는 것은 기존 작업 계획 기능이 한다.
// 후보 조회는 검색어·지역 필터·결과 전체 선택용이다. 시군→학교 트리는 쓰지 않는다(사용자 결정).

const siteCandPageSize = 100

// SiteCand 후보 한 줄.
type SiteCand struct {
	ID, Name, Region string
	Added            bool // 이미 등록됨(체크박스 비활성)
	PlanName         string
}

// SiteCandPage 후보 목록 한 쪽과 집계.
type SiteCandPage struct {
	Items               []SiteCand
	Total               int // 필터에 맞는 후보 전체(등록됨 포함)
	Addable             int // 그중 아직 등록 안 된 것
	Registered          int // 이 단계·하부 업무에 등록된 사이트 수(필터와 무관)
	Pool                int // 후보의 바탕이 되는 풀 크기(사업 사이트 수 또는 1단계 등록 수)
	Page, Pages         int
	Regions             []string
	Q, Region           string
	TargetName, PhaseID string
}

type siteScope struct {
	// 후보 풀 FROM/WHERE 조각과 인자, 「이미 등록됨」 판정 조각
	from  string
	where string
	args  []interface{}
	added string // 등록됨 여부를 내는 SQL 식(s = project_sites 별칭)
	aargs []interface{}
}

func likeArg(q string) string {
	q = strings.TrimSpace(q)
	q = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	return "%" + q + "%"
}

func (sc siteScope) filter(q, region string) (string, []interface{}) {
	w := sc.where
	args := append([]interface{}{}, sc.args...)
	if strings.TrimSpace(q) != "" {
		w += ` AND (s.site_name LIKE ? ESCAPE '\' OR s.region LIKE ? ESCAPE '\')`
		args = append(args, likeArg(q), likeArg(q))
	}
	if strings.TrimSpace(region) != "" {
		w += ` AND s.region=?`
		args = append(args, strings.TrimSpace(region))
	}
	return w, args
}

func (r *ProjectRepo) siteCandPage(sc siteScope, q, region string, page int) (SiteCandPage, error) {
	var out SiteCandPage
	out.Q, out.Region = strings.TrimSpace(q), strings.TrimSpace(region)
	w, args := sc.filter(q, region)
	cargs := append(append([]interface{}{}, sc.aargs...), args...)
	if err := r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN `+sc.added+` THEN 0 ELSE 1 END),0) FROM `+sc.from+` WHERE `+w, cargs...).
		Scan(&out.Total, &out.Addable); err != nil {
		return out, err
	}
	out.Pages = (out.Total + siteCandPageSize - 1) / siteCandPageSize
	if page < 1 {
		page = 1
	}
	if out.Pages > 0 && page > out.Pages {
		page = out.Pages
	}
	out.Page = page
	rows, err := r.db.Query(`SELECT s.id, s.site_name, COALESCE(s.region,''), CASE WHEN `+sc.added+` THEN 1 ELSE 0 END
		FROM `+sc.from+` WHERE `+w+` ORDER BY s.region, s.site_name, s.id LIMIT ? OFFSET ?`,
		append(append(append([]interface{}{}, sc.aargs...), args...), siteCandPageSize, (page-1)*siteCandPageSize)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var c SiteCand
		var added int
		if err := rows.Scan(&c.ID, &c.Name, &c.Region, &added); err != nil {
			return out, err
		}
		c.Added = added == 1
		out.Items = append(out.Items, c)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	regs, err := r.db.Query(`SELECT DISTINCT COALESCE(s.region,'') FROM `+sc.from+` WHERE `+sc.where+` AND TRIM(COALESCE(s.region,''))!='' ORDER BY 1 LIMIT 500`, append(append([]interface{}{}, sc.aargs...), sc.args...)...)
	if err != nil {
		return out, err
	}
	defer regs.Close()
	for regs.Next() {
		var g string
		if err := regs.Scan(&g); err != nil {
			return out, err
		}
		out.Regions = append(out.Regions, g)
	}
	return out, regs.Err()
}

// phaseOfProject 단계가 그 사업의 것인지 확인하고 이름을 돌려준다.
func (r *ProjectRepo) phaseOfProject(projectID, phaseID string) (string, error) {
	var name string
	err := r.db.QueryRow(`SELECT name FROM project_phases WHERE phase_id=? AND project_id=? AND is_active=1`,
		strings.TrimSpace(phaseID), strings.TrimSpace(projectID)).Scan(&name)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("단계를 찾을 수 없습니다")
	}
	return name, err
}

// packageOfProject 하부 업무가 그 사업의 것인지 확인하고 (이름, 단계ID, track_mode)를 돌려준다.
func (r *ProjectRepo) packageOfProject(projectID, packageID string) (name, phaseID, track string, err error) {
	err = r.db.QueryRow(`SELECT pk.name, pk.phase_id, pk.track_mode
		FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pk.package_id=? AND ph.project_id=? AND pk.is_active=1`,
		strings.TrimSpace(packageID), strings.TrimSpace(projectID)).Scan(&name, &phaseID, &track)
	if err == sql.ErrNoRows {
		err = fmt.Errorf("하부 업무를 찾을 수 없습니다")
	}
	return
}

func (r *ProjectRepo) phaseScope(projectID, phaseID string) siteScope {
	return siteScope{
		from:  `project_sites s LEFT JOIN phase_sites hs ON hs.phase_id=? AND hs.project_site_id=s.id AND hs.scope_status='active'`,
		where: `s.project_id=? AND s.is_active=1 AND s.status!='excluded'`,
		args:  []interface{}{projectID},
		added: `hs.phase_site_id IS NOT NULL`,
		aargs: []interface{}{phaseID},
	}
}

func (r *ProjectRepo) packageScope(phaseID, packageID string) siteScope {
	return siteScope{
		from: `phase_sites hs JOIN project_sites s ON s.id=hs.project_site_id
		       LEFT JOIN package_sites ps ON ps.package_id=? AND ps.project_site_id=s.id AND ps.status!='excluded'`,
		where: `hs.phase_id=? AND hs.scope_status='active' AND s.is_active=1 AND s.status!='excluded'`,
		args:  []interface{}{phaseID},
		added: `ps.package_site_id IS NOT NULL`,
		aargs: []interface{}{packageID},
	}
}

// PhaseSiteCandidates 1단계 업무 사이트 후보(사업 사이트 풀).
func (r *ProjectRepo) PhaseSiteCandidates(projectID, phaseID, q, region string, page int) (SiteCandPage, error) {
	name, err := r.phaseOfProject(projectID, phaseID)
	if err != nil {
		return SiteCandPage{}, err
	}
	out, err := r.siteCandPage(r.phaseScope(projectID, phaseID), q, region, page)
	out.TargetName, out.PhaseID = name, phaseID
	if err != nil {
		return out, err
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id=? AND is_active=1 AND status!='excluded'`, projectID).Scan(&out.Pool)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM phase_sites hs JOIN project_sites s ON s.id=hs.project_site_id
		WHERE hs.phase_id=? AND hs.scope_status='active' AND s.is_active=1 AND s.status!='excluded'`, phaseID).Scan(&out.Registered)
	return out, nil
}

// PackageSiteCandidates 2단계 하부 업무 사이트 후보(그 1단계 업무에 등록된 사이트).
func (r *ProjectRepo) PackageSiteCandidates(projectID, packageID, q, region string, page int) (SiteCandPage, error) {
	name, phaseID, track, err := r.packageOfProject(projectID, packageID)
	if err != nil {
		return SiteCandPage{}, err
	}
	if track != model.TrackModeSite {
		return SiteCandPage{}, fmt.Errorf("수량만 하부 업무에는 사이트를 등록하지 않습니다")
	}
	out, err := r.siteCandPage(r.packageScope(phaseID, packageID), q, region, page)
	out.TargetName, out.PhaseID = name, phaseID
	if err != nil {
		return out, err
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM phase_sites hs JOIN project_sites s ON s.id=hs.project_site_id
		WHERE hs.phase_id=? AND hs.scope_status='active' AND s.is_active=1 AND s.status!='excluded'`, phaseID).Scan(&out.Pool)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM package_sites WHERE package_id=? AND status!='excluded'`, packageID).Scan(&out.Registered)
	return out, nil
}

// candidateIDs 필터에 맞는 아직 등록 안 된 후보 전체의 ID(「조회 결과 전체 등록」).
func (r *ProjectRepo) candidateIDs(sc siteScope, q, region string) ([]string, error) {
	w, args := sc.filter(q, region)
	rows, err := r.db.Query(`SELECT s.id FROM `+sc.from+` WHERE `+w+` AND NOT (`+sc.added+`) ORDER BY s.region, s.site_name, s.id`,
		append(append([]interface{}{}, sc.aargs...), args...)...)
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

func dedupIDs(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// AddPhaseSites 1단계 업무에 사이트를 등록한다. all 이면 필터(q·region)에 맞는 후보 전체. 돌려주는 값은 새로 등록한 수.
func (r *ProjectRepo) AddPhaseSites(projectID, phaseID string, ids []string, all bool, q, region, actor string) (int, error) {
	if _, err := r.phaseOfProject(projectID, phaseID); err != nil {
		return 0, err
	}
	if all {
		var err error
		if ids, err = r.candidateIDs(r.phaseScope(projectID, phaseID), q, region); err != nil {
			return 0, err
		}
	}
	ids = dedupIDs(ids)
	if len(ids) == 0 {
		return 0, fmt.Errorf("등록할 사이트를 고르세요")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := model.LocalNowString()
	added := 0
	for _, id := range ids {
		var pid, status string
		err := tx.QueryRow(`SELECT project_id, status FROM project_sites WHERE id=? AND is_active=1`, id).Scan(&pid, &status)
		if err != nil || pid != projectID || status == "excluded" {
			return 0, fmt.Errorf("사업 사이트에 없는 사이트가 있습니다")
		}
		var cur string
		err = tx.QueryRow(`SELECT scope_status FROM phase_sites WHERE phase_id=? AND project_site_id=?`, phaseID, id).Scan(&cur)
		switch {
		case err == sql.ErrNoRows:
			n, nerr := nextSeqTx(tx, hsSeqKey)
			if nerr != nil {
				return 0, nerr
			}
			if _, err := tx.Exec(`INSERT INTO phase_sites(phase_site_id, phase_id, project_site_id, scope_status, added_at, added_by)
				VALUES (?,?,?, 'active', ?, ?)`, fmt.Sprintf("HS-%03d", n), phaseID, id, now, strings.TrimSpace(actor)); err != nil {
				return 0, err
			}
			added++
		case err != nil:
			return 0, err
		case cur != "active":
			if _, err := tx.Exec(`UPDATE phase_sites SET scope_status='active', added_at=?, added_by=?, removed_at='', removed_by='', reason=''
				WHERE phase_id=? AND project_site_id=?`, now, strings.TrimSpace(actor), phaseID, id); err != nil {
				return 0, err
			}
			added++
		}
	}
	return added, tx.Commit()
}

func removeReasonOK(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", fmt.Errorf("제외 사유를 입력하세요")
	}
	if utf8.RuneCountInString(reason) > 200 {
		return "", fmt.Errorf("제외 사유는 200자까지입니다")
	}
	return reason, nil
}

// RemovePhaseSites 1단계 업무에서 사이트를 뺀다(사유 필수). 하부 업무에 쓰이는 사이트는 거기서 먼저 빼야 한다.
func (r *ProjectRepo) RemovePhaseSites(projectID, phaseID string, ids []string, reason, actor string) (int, error) {
	if _, err := r.phaseOfProject(projectID, phaseID); err != nil {
		return 0, err
	}
	reason, err := removeReasonOK(reason)
	if err != nil {
		return 0, err
	}
	ids = dedupIDs(ids)
	if len(ids) == 0 {
		return 0, fmt.Errorf("제외할 사이트를 고르세요")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := model.LocalNowString()
	n := 0
	for _, id := range ids {
		var used int
		var name string
		_ = tx.QueryRow(`SELECT COALESCE(MAX(s.site_name),''), COUNT(ps.package_site_id)
			FROM project_sites s
			LEFT JOIN phase_packages pk ON pk.phase_id=? AND pk.is_active=1
			LEFT JOIN package_sites ps ON ps.package_id=pk.package_id AND ps.project_site_id=s.id AND ps.status!='excluded'
			WHERE s.id=? AND s.project_id=?`, phaseID, id, projectID).Scan(&name, &used)
		if used > 0 {
			return 0, fmt.Errorf("%s 는 하부 업무에 등록돼 있어 뺄 수 없습니다. 하부 업무에서 먼저 제외하세요", name)
		}
		res, err := tx.Exec(`UPDATE phase_sites SET scope_status='removed', removed_at=?, removed_by=?, reason=?
			WHERE phase_id=? AND project_site_id=? AND scope_status='active'`, now, strings.TrimSpace(actor), reason, phaseID, id)
		if err != nil {
			return 0, err
		}
		a, _ := res.RowsAffected()
		n += int(a)
	}
	return n, tx.Commit()
}

// AddPackageSites 2단계 하부 업무에 사이트를 등록한다(그 1단계 업무에 등록된 사이트만). 계획 없이(plan_id=”) 등록된다.
func (r *ProjectRepo) AddPackageSites(projectID, packageID string, ids []string, all bool, q, region, actor string) (int, error) {
	_, phaseID, track, err := r.packageOfProject(projectID, packageID)
	if err != nil {
		return 0, err
	}
	if track != model.TrackModeSite {
		return 0, fmt.Errorf("수량만 하부 업무에는 사이트를 등록하지 않습니다")
	}
	if all {
		if ids, err = r.candidateIDs(r.packageScope(phaseID, packageID), q, region); err != nil {
			return 0, err
		}
	}
	ids = dedupIDs(ids)
	if len(ids) == 0 {
		return 0, fmt.Errorf("등록할 사이트를 고르세요")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := model.LocalNowString()
	added := 0
	for _, id := range ids {
		var ok int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM phase_sites hs JOIN project_sites s ON s.id=hs.project_site_id
			WHERE hs.phase_id=? AND hs.project_site_id=? AND hs.scope_status='active' AND s.project_id=? AND s.is_active=1 AND s.status!='excluded'`,
			phaseID, id, projectID).Scan(&ok); err != nil {
			return 0, err
		}
		if ok == 0 {
			return 0, fmt.Errorf("1단계 업무에 등록된 사이트만 고를 수 있습니다")
		}
		var cur string
		err := tx.QueryRow(`SELECT status FROM package_sites WHERE package_id=? AND project_site_id=?`, packageID, id).Scan(&cur)
		switch {
		case err == sql.ErrNoRows:
			n, nerr := nextSeqTx(tx, psSeqKey)
			if nerr != nil {
				return 0, nerr
			}
			if _, err := tx.Exec(`INSERT INTO package_sites(package_site_id, package_id, project_site_id, status, target_qty, done_qty, plan_id, updated_at)
				VALUES (?,?,?, 'planned', 0, 0, '', ?)`, fmt.Sprintf("PS-%03d", n), packageID, id, now); err != nil {
				return 0, err
			}
			added++
		case err != nil:
			return 0, err
		case cur == model.SiteExcluded:
			if _, err := tx.Exec(`UPDATE package_sites SET status='planned', exclude_reason='', plan_id='', updated_at=? WHERE package_id=? AND project_site_id=?`, now, packageID, id); err != nil {
				return 0, err
			}
			added++
		}
	}
	return added, tx.Commit()
}

// RemovePackageSites 2단계 하부 업무에서 사이트를 제외한다(사유 필수). 작업 계획에 속한 사이트와 완료된 사이트는 뺄 수 없다.
func (r *ProjectRepo) RemovePackageSites(projectID, packageID string, ids []string, reason, actor string) (int, error) {
	if _, _, _, err := r.packageOfProject(projectID, packageID); err != nil {
		return 0, err
	}
	reason, err := removeReasonOK(reason)
	if err != nil {
		return 0, err
	}
	ids = dedupIDs(ids)
	if len(ids) == 0 {
		return 0, fmt.Errorf("제외할 사이트를 고르세요")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := model.LocalNowString()
	n := 0
	for _, id := range ids {
		var plan, status, name string
		err := tx.QueryRow(`SELECT ps.plan_id, ps.status, COALESCE(s.site_name,'') FROM package_sites ps
			JOIN project_sites s ON s.id=ps.project_site_id
			WHERE ps.package_id=? AND ps.project_site_id=? AND s.project_id=?`, packageID, id, projectID).Scan(&plan, &status, &name)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return 0, err
		}
		if status == model.SiteExcluded {
			continue
		}
		if status == model.SiteDone {
			return 0, fmt.Errorf("%s 는 완료된 사이트라 뺄 수 없습니다", name)
		}
		if strings.TrimSpace(plan) != "" {
			return 0, fmt.Errorf("%s 는 작업 계획에 속해 있습니다. 계획에서 먼저 제외하세요", name)
		}
		res, err := tx.Exec(`UPDATE package_sites SET status='excluded', exclude_reason=?, updated_at=? WHERE package_id=? AND project_site_id=?`,
			reason, now, packageID, id)
		if err != nil {
			return 0, err
		}
		a, _ := res.RowsAffected()
		n += int(a)
	}
	return n, tx.Commit()
}

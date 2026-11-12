package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"

	"customer-support/internal/model"
)

// 95-183 수행 구조: 2단계 하부 업무의 이름·유형·다음 할 일·상태·순서·삭제(비활성)·복구, 3단계 계획의 사이트·수량.
// 새 컬럼 없이 phase_packages 의 name·track_mode·next_note·status·done_at·done_by·seq·is_active 만 쓴다.

// PackageEdit 하부 업무의 이름·유형·다음 할 일과 일자·배지(기존 UpdatePackageMeta 와 같은 항목).
type PackageEdit struct {
	Name, Track, NextNote                  string
	PlanStart, PlanEnd, ActualStart, Badge string
}

// OpenSitesError 완료로 두려는 하부 업무에 끝나지 않은 사이트(또는 못 채운 수량)가 남았다. 확인하면 진행한다.
type OpenSitesError struct{ N int }

func (e *OpenSitesError) Error() string {
	return fmt.Sprintf("끝나지 않은 사이트가 %d곳 있습니다", e.N)
}

// PackageImpact 하부 업무에 딸린 것의 개수(삭제·유형 변경 판단용).
type PackageImpact struct {
	Tasks, Plans, Sites, PlanSites, PerfSites, Events int
}

// HasPerformance 실적이 하나라도 있으면 삭제 대신 비활성만 한다.
func (i PackageImpact) HasPerformance() bool {
	return i.Tasks > 0 || i.Plans > 0 || i.PlanSites > 0 || i.PerfSites > 0 || i.Events > 0
}

func packageImpactTx(tx *sql.Tx, packageID string) (PackageImpact, error) {
	var im PackageImpact
	q := func(dst *int, query string) error {
		return tx.QueryRow(query, packageID).Scan(dst)
	}
	if err := q(&im.Tasks, `SELECT COUNT(*) FROM work_tasks WHERE package_id=?`); err != nil {
		return im, err
	}
	if err := q(&im.Plans, `SELECT COUNT(*) FROM package_plans WHERE package_id=?`); err != nil {
		return im, err
	}
	if err := q(&im.Sites, `SELECT COUNT(*) FROM package_sites WHERE package_id=?`); err != nil {
		return im, err
	}
	if err := q(&im.PlanSites, `SELECT COUNT(*) FROM package_sites WHERE package_id=? AND plan_id<>''`); err != nil {
		return im, err
	}
	if err := q(&im.PerfSites, `SELECT COUNT(*) FROM package_sites WHERE package_id=? AND status IN ('done','in_progress')`); err != nil {
		return im, err
	}
	if err := q(&im.Events, `SELECT COUNT(*) FROM package_events WHERE package_id=?`); err != nil {
		return im, err
	}
	return im, nil
}

type pkgOwner struct {
	phaseID, name, track string
	active               bool
}

func packageOwnerTx(tx *sql.Tx, projectID, packageID string) (pkgOwner, error) {
	var o pkgOwner
	var act int
	err := tx.QueryRow(`
		SELECT pk.phase_id, pk.name, pk.track_mode, pk.is_active
		FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pk.package_id=? AND ph.project_id=?`,
		strings.TrimSpace(packageID), strings.TrimSpace(projectID)).Scan(&o.phaseID, &o.name, &o.track, &act)
	if err == sql.ErrNoRows {
		return o, fmt.Errorf("하부 업무를 찾을 수 없습니다")
	}
	o.active = act == 1
	return o, err
}

// packageNameFreeTx 같은 단위 업무 안에서 정규화한 이름이 겹치는 하부 업무가 없는지(비활성 포함). 겹치면 안내 문구를 돌려준다.
func packageNameFreeTx(tx *sql.Tx, phaseID, name, exceptID string) error {
	key := model.NormalizePlanMergeName(name)
	rows, err := tx.Query(`SELECT package_id, name, is_active FROM phase_packages WHERE phase_id=?`, phaseID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, n string
		var act int
		if err := rows.Scan(&id, &n, &act); err != nil {
			return err
		}
		if id == exceptID {
			continue
		}
		if model.NormalizePlanMergeName(n) == key || n == name {
			if act == 0 {
				return fmt.Errorf("비활성 하부 업무 「%s」와 이름이 같습니다. 아래 비활성 목록에서 되돌리거나 다른 이름을 쓰세요", n)
			}
			return fmt.Errorf("같은 이름의 하부 업무가 있습니다")
		}
	}
	return rows.Err()
}

// UpdatePackage 하부 업무의 이름·유형·다음 할 일·일자·배지를 한 번에 저장한다.
// 이름은 같은 단위 업무 안에서 정규화 기준으로 겹치면 안 된다. 유형은 사이트·계획·수량 기록이 하나도 없을 때만 바꾼다.
func (r *ProjectRepo) UpdatePackage(projectID, packageID string, e PackageEdit, ext DateChangeCtx) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := updatePackageTx(tx, projectID, packageID, e, ext); err != nil {
		return err
	}
	return tx.Commit()
}

func updatePackageTx(tx *sql.Tx, projectID, packageID string, e PackageEdit, ext DateChangeCtx) error {
	name, err := model.PackageNameOK(e.Name)
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(strings.TrimSpace(e.NextNote)) > 300 {
		return fmt.Errorf("다음 할 일은 300자까지입니다")
	}
	o, err := packageOwnerTx(tx, projectID, packageID)
	if err != nil {
		return err
	}
	if !o.active {
		return fmt.Errorf("비활성 하부 업무입니다. 되돌린 뒤 수정하세요")
	}
	if name != o.name {
		if err := packageNameFreeTx(tx, o.phaseID, name, packageID); err != nil {
			return err
		}
	}
	track := o.track
	if t := strings.TrimSpace(e.Track); t != "" && t != o.track {
		if !model.ValidTrackMode(t) {
			return fmt.Errorf("유형을 확인하세요")
		}
		im, err := packageImpactTx(tx, packageID)
		if err != nil {
			return err
		}
		if im.Plans+im.Sites+im.Events > 0 {
			parts := []string{}
			if im.Plans > 0 {
				parts = append(parts, fmt.Sprintf("계획 %d개", im.Plans))
			}
			if im.Sites > 0 {
				parts = append(parts, fmt.Sprintf("사이트 %d곳", im.Sites))
			}
			if im.Events > 0 {
				parts = append(parts, fmt.Sprintf("실적·수량 기록 %d건", im.Events))
			}
			return fmt.Errorf("%s이 있어 유형을 바꿀 수 없습니다. 새 하부 업무를 만드세요", strings.Join(parts, "·"))
		}
		track = t
	}
	if _, err := tx.Exec(`UPDATE phase_packages SET name=?, track_mode=?, next_note=?, updated_at=? WHERE package_id=?`,
		name, track, strings.TrimSpace(e.NextNote), model.LocalNowString(), packageID); err != nil {
		return err
	}
	return updatePackageMetaTx(tx, projectID, packageID, PackageMeta{
		PlanStart: e.PlanStart, PlanEnd: e.PlanEnd, ActualStart: e.ActualStart, Badge: e.Badge,
	}, ext)
}

// SetPackageStatus 하부 업무 상태: active(진행)·auto(자동으로 되돌리기)·done(완료 확정)·hold(보류). 완료 확정은 사람이 누른다.
// done 인데 끝나지 않은 사이트(수량만은 못 채운 수량)가 있으면 confirm 이 아닌 한 *OpenSitesError 를 돌려준다(막지는 않는다).
func (r *ProjectRepo) SetPackageStatus(projectID, packageID, status, actor string, confirm bool) error {
	status = strings.TrimSpace(status)
	if status != model.PkgAuto && status != model.PkgDone && status != model.PkgHold && status != model.PhaseActive {
		return fmt.Errorf("상태를 확인하세요")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := packageOwnerTx(tx, projectID, packageID)
	if err != nil {
		return err
	}
	if !o.active {
		return fmt.Errorf("비활성 하부 업무입니다. 되돌린 뒤 바꾸세요")
	}
	doneAt, doneBy := "", ""
	if status == model.PkgDone {
		open := 0
		if o.track == model.TrackModePhase {
			if st, err := qtyPackageTx(tx, projectID, packageID); err == nil && st.Total > 0 && st.Done < st.Total {
				open = 1
			}
		} else if err := tx.QueryRow(`SELECT COUNT(*) FROM package_sites WHERE package_id=? AND status NOT IN ('done','excluded')`, packageID).Scan(&open); err != nil {
			return err
		}
		if open > 0 && !confirm {
			return &OpenSitesError{N: open}
		}
		doneAt, doneBy = nowStamp(), strings.TrimSpace(actor)
	}
	if _, err := tx.Exec(`UPDATE phase_packages SET status=?, done_at=?, done_by=?, updated_at=? WHERE package_id=?`,
		status, doneAt, doneBy, nowStamp(), packageID); err != nil {
		return err
	}
	return tx.Commit()
}

// MovePackage 같은 단위 업무 안에서 순서를 한 칸 옮긴다(dir: up|down). 비활성은 순서 계산에서 뺀다.
func (r *ProjectRepo) MovePackage(projectID, packageID, dir string) error {
	if dir != "up" && dir != "down" {
		return fmt.Errorf("순서 방향이 아닙니다")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := packageOwnerTx(tx, projectID, packageID)
	if err != nil {
		return err
	}
	if !o.active {
		return fmt.Errorf("비활성 하부 업무는 순서를 바꿀 수 없습니다")
	}
	rows, err := tx.Query(`SELECT package_id FROM phase_packages WHERE phase_id=? AND is_active=1 ORDER BY seq, package_id`, o.phaseID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	at := -1
	for i, id := range ids {
		if id == packageID {
			at = i
		}
	}
	to := at - 1
	if dir == "down" {
		to = at + 1
	}
	if at < 0 || to < 0 || to >= len(ids) {
		return nil // 맨 위·맨 아래에서는 아무 일도 없다
	}
	ids[at], ids[to] = ids[to], ids[at]
	now := nowStamp()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE phase_packages SET seq=?, updated_at=? WHERE package_id=?`, i, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeletePackage 실적(연결 작업·계획·계획에 묶인 사이트·진행 사이트·일 이벤트/수량 기록)이 없으면 지우고,
// 있으면 지우지 않고 비활성으로 둔다. 하부 업무에 직접 등록한 사이트·마일스톤·일 스냅샷은 함께 지운다. 이력(structure_date_changes)은 남긴다.
func (r *ProjectRepo) DeletePackage(projectID, packageID string) (deactivated bool, im PackageImpact, err error) {
	tx, err := r.db.Begin()
	if err != nil {
		return false, im, err
	}
	defer tx.Rollback()
	o, err := packageOwnerTx(tx, projectID, packageID)
	if err != nil {
		return false, im, err
	}
	if !o.active {
		return true, im, fmt.Errorf("이미 비활성입니다")
	}
	if im, err = packageImpactTx(tx, packageID); err != nil {
		return false, im, err
	}
	if im.HasPerformance() {
		_, err = tx.Exec(`UPDATE phase_packages SET is_active=0, updated_at=? WHERE package_id=?`, nowStamp(), packageID)
		if err != nil {
			return false, im, err
		}
		return true, im, tx.Commit()
	}
	for _, q := range []string{
		`DELETE FROM package_milestones WHERE package_id=?`,
		`DELETE FROM package_progress_daily WHERE package_id=?`,
		`DELETE FROM package_sites WHERE package_id=?`,
		`DELETE FROM phase_packages WHERE package_id=?`,
	} {
		if _, err := tx.Exec(q, packageID); err != nil {
			return false, im, err
		}
	}
	return false, im, tx.Commit()
}

// RestorePackage 비활성 하부 업무를 되돌린다(순서는 맨 뒤).
func (r *ProjectRepo) RestorePackage(projectID, packageID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := packageOwnerTx(tx, projectID, packageID)
	if err != nil {
		return err
	}
	if o.active {
		return nil
	}
	var seq int
	_ = tx.QueryRow(`SELECT COALESCE(MAX(seq),-1)+1 FROM phase_packages WHERE phase_id=?`, o.phaseID).Scan(&seq)
	if _, err := tx.Exec(`UPDATE phase_packages SET is_active=1, seq=?, updated_at=? WHERE package_id=?`, seq, nowStamp(), packageID); err != nil {
		return err
	}
	return tx.Commit()
}

// InactivePackage 화면의 「비활성 하부 업무」 목록 한 줄.
type InactivePackage struct {
	ID, PhaseID, Name string
}

// ListInactivePackages 사업의 비활성 하부 업무(단위 업무 순서 → 하부 업무 순서).
func (r *ProjectRepo) ListInactivePackages(projectID string) ([]InactivePackage, error) {
	rows, err := r.db.Query(`
		SELECT pk.package_id, pk.phase_id, pk.name
		FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=? AND pk.is_active=0
		ORDER BY ph.seq, pk.seq, pk.package_id`, strings.TrimSpace(projectID))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []InactivePackage
	for rows.Next() {
		var p InactivePackage
		if err := rows.Scan(&p.ID, &p.PhaseID, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- 3단계 계획: 사이트·수량 관리 ----

// PlanSiteRow 계획 사이트 관리 화면의 한 줄(이 계획에 묶인 사이트).
type PlanSiteRow struct {
	ID, Name, Region, Status, Reason string
	Target, Done                     int
}

// PlanCandidate 하부 업무에 등록된 사이트 중 이 계획에 아직 없는 사이트.
type PlanCandidate struct {
	ID, Name, Region, Status, OtherPlanID, OtherPlan string
	Target                                           int
}

// PlanSiteView 계획 사이트 관리 화면 자료.
type PlanSiteView struct {
	PlanID, PlanName, PackageID, PackageName, PhaseName, Basis, Unit, Status string
	Active                                                                  bool
	Sites                                                                   []PlanSiteRow
	Excluded                                                                []PlanSiteRow
}

// PlanSites 계획에 묶인 사이트(제외 포함)와 계획 머리 정보. 사업 범위를 확인한다.
func (r *ProjectRepo) PlanSites(projectID, planID string) (PlanSiteView, error) {
	var v PlanSiteView
	var act int
	err := r.db.QueryRow(`
		SELECT pl.plan_id, pl.name, pl.progress_basis, pl.qty_unit, pl.status, pl.is_active, pk.package_id, pk.name, ph.name
		FROM package_plans pl JOIN phase_packages pk ON pk.package_id=pl.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pl.plan_id=? AND ph.project_id=?`, strings.TrimSpace(planID), strings.TrimSpace(projectID)).
		Scan(&v.PlanID, &v.PlanName, &v.Basis, &v.Unit, &v.Status, &act, &v.PackageID, &v.PackageName, &v.PhaseName)
	if err == sql.ErrNoRows {
		return v, fmt.Errorf("계획이 없습니다")
	}
	if err != nil {
		return v, err
	}
	v.Active = act == 1
	rows, err := r.db.Query(`
		SELECT s.id, COALESCE(s.site_name,''), COALESCE(s.region,''), ps.status, COALESCE(ps.exclude_reason,''), ps.target_qty, ps.done_qty
		FROM package_sites ps JOIN project_sites s ON s.id=ps.project_site_id
		WHERE ps.package_id=? AND ps.plan_id=?
		ORDER BY s.site_name, s.id`, v.PackageID, v.PlanID)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var x PlanSiteRow
		if err := rows.Scan(&x.ID, &x.Name, &x.Region, &x.Status, &x.Reason, &x.Target, &x.Done); err != nil {
			return v, err
		}
		if x.Status == model.SiteExcluded {
			v.Excluded = append(v.Excluded, x)
		} else {
			v.Sites = append(v.Sites, x)
		}
	}
	return v, rows.Err()
}

// PlanSiteCandidates 하부 업무에 등록된 사이트(제외 아님) 중 이 계획이 아닌 곳. 이름·지역 검색(초성 포함), 100건 단위.
func (r *ProjectRepo) PlanSiteCandidates(projectID, planID, q, region string, offset, limit int) ([]PlanCandidate, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.Query(`
		SELECT s.id, COALESCE(s.site_name,''), COALESCE(s.region,''), ps.status, ps.plan_id, COALESCE(pl.name,''), ps.target_qty
		FROM package_plans me
		JOIN phase_packages pk ON pk.package_id=me.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		JOIN package_sites ps ON ps.package_id=me.package_id
		JOIN project_sites s ON s.id=ps.project_site_id
		LEFT JOIN package_plans pl ON pl.plan_id=ps.plan_id
		WHERE me.plan_id=? AND ph.project_id=? AND ps.status<>'excluded' AND ps.plan_id<>me.plan_id
		  AND s.is_active=1 AND s.status<>'excluded'
		ORDER BY s.site_name, s.id`, strings.TrimSpace(planID), strings.TrimSpace(projectID))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var all []PlanCandidate
	for rows.Next() {
		var c PlanCandidate
		if err := rows.Scan(&c.ID, &c.Name, &c.Region, &c.Status, &c.OtherPlanID, &c.OtherPlan, &c.Target); err != nil {
			return nil, 0, err
		}
		if !model.RegionAuxMatch(c.Region, region) || !model.SiteImportMatch(c.Name, "", c.Region, q, "") {
			continue
		}
		all = append(all, c)
	}
	total := len(all)
	if offset >= total {
		return []PlanCandidate{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

// SetPlanSiteQty 계획에 묶인 사이트(제외 아님)의 목표 수량을 저장한다. 수량은 1 이상의 정수. 이 계획의 사이트가 아니면 거절.
func (r *ProjectRepo) SetPlanSiteQty(projectID, planID string, qty map[string]int) (packageID string, err error) {
	if len(qty) == 0 {
		return "", fmt.Errorf("저장할 수량이 없습니다")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	pkg, _, err := planOwnerTx(tx, strings.TrimSpace(planID), strings.TrimSpace(projectID))
	if err != nil {
		return "", err
	}
	now := nowStamp()
	for id, n := range qty {
		if n < 1 {
			return "", fmt.Errorf("목표 수량은 1 이상이어야 합니다")
		}
		res, err := tx.Exec(`UPDATE package_sites SET target_qty=?, updated_at=? WHERE package_id=? AND project_site_id=? AND plan_id=? AND status<>'excluded'`,
			n, now, pkg, id, planID)
		if err != nil {
			return "", err
		}
		if a, _ := res.RowsAffected(); a == 0 {
			return "", fmt.Errorf("이 계획의 사이트가 아닌 곳이 있습니다")
		}
	}
	return pkg, tx.Commit()
}

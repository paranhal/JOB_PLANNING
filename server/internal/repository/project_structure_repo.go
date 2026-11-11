package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"customer-support/internal/model"
)

const (
	pkgSeqKey  = "phase_package"
	planSeqKey = "package_plan"
	psSeqKey   = "package_site"
	hsSeqKey   = "phase_site"
)

func nowStamp() string { return time.Now().Format("2006-01-02 15:04:05") }

func (r *ProjectRepo) ProjectHasPackages(projectID string) bool {
	if r == nil || r.db == nil {
		return false
	}
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=? AND pk.is_active=1`, projectID).Scan(&n)
	return err == nil && n > 0
}

func (r *ProjectRepo) CreatePackage(phaseID, name, track string, weight float64) (string, error) {
	name, err := model.PackageNameOK(name)
	if err != nil {
		return "", err
	}
	if !model.ValidTrackMode(track) {
		track = model.TrackModeSite
	}
	if weight <= 0 {
		weight = 1
	}
	tx, err := r.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM phase_packages WHERE phase_id=? AND name=?`, phaseID, name).Scan(&n); err != nil {
		return "", err
	}
	if n > 0 {
		return "", fmt.Errorf("같은 이름의 하부 업무가 있습니다")
	}
	seqNo, err := nextSeqTx(tx, pkgSeqKey)
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("PK-%03d", seqNo)
	var seq int
	_ = tx.QueryRow(`SELECT COALESCE(MAX(seq),-1)+1 FROM phase_packages WHERE phase_id=?`, phaseID).Scan(&seq)
	now := nowStamp()
	if _, err := tx.Exec(`
		INSERT INTO phase_packages(
			package_id, phase_id, seq, name, track_mode, weight, status, is_active, created_at, updated_at)
		VALUES (?,?,?,?,?,?, 'auto', 1, ?, ?)`,
		id, phaseID, seq, name, track, weight, now, now); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

type PlanDraft struct {
	PackageID   string
	Name        string
	PlanStart   string
	PlanEnd     string
	Basis       string
	QtyUnit     string
	RegionLabel string
	NextNote    string
	SiteIDs     []string
	TargetQty   map[string]int
	Actor       string
}

func (r *ProjectRepo) CreatePlan(projectID string, d PlanDraft) (string, error) {
	name, err := model.PlanNameOK(d.Name)
	if err != nil {
		return "", err
	}
	ps, pe, _, _, err := model.PhaseDatesOK(d.PlanStart, d.PlanEnd, "", "")
	if err != nil {
		return "", err
	}
	if !model.ValidBasis(d.Basis) || d.Basis == "" {
		d.Basis = model.BasisSite
	}
	if utf8.RuneCountInString(strings.TrimSpace(d.QtyUnit)) > 20 {
		return "", fmt.Errorf("수량 단위는 20자까지입니다")
	}
	if utf8.RuneCountInString(d.NextNote) > 300 {
		return "", fmt.Errorf("다음 할 일은 300자까지입니다")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	phaseID, err := packagePhaseTx(tx, d.PackageID, projectID)
	if err != nil {
		return "", err
	}
	var dup int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM package_plans WHERE package_id=? AND name=?`, d.PackageID, name).Scan(&dup); err != nil {
		return "", err
	}
	if dup > 0 {
		return "", fmt.Errorf("같은 이름의 계획이 있습니다")
	}
	if d.Basis == model.BasisQty {
		if n := missingQty(d.SiteIDs, d.TargetQty); n > 0 {
			return "", fmt.Errorf("미입력 %d곳", n)
		}
	}
	seqNo, err := nextSeqTx(tx, planSeqKey)
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("PL-%03d", seqNo)
	var seq int
	_ = tx.QueryRow(`SELECT COALESCE(MAX(seq),-1)+1 FROM package_plans WHERE package_id=?`, d.PackageID).Scan(&seq)
	now := nowStamp()
	if _, err := tx.Exec(`
		INSERT INTO package_plans(
			plan_id, package_id, seq, name, plan_start, plan_end, status, progress_basis,
			qty_unit, region_label, next_note, is_active, created_at, updated_at)
		VALUES (?,?,?,?,?,?, 'auto', ?, ?, ?, ?, 1, ?, ?)`,
		id, d.PackageID, seq, name, ps, pe, d.Basis, strings.TrimSpace(d.QtyUnit),
		strings.TrimSpace(d.RegionLabel), strings.TrimSpace(d.NextNote), now, now); err != nil {
		return "", err
	}
	if err := attachSitesTx(tx, projectID, phaseID, d.PackageID, id, d.SiteIDs, d.TargetQty, d.Actor, false); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func packagePhaseTx(tx *sql.Tx, packageID, projectID string) (string, error) {
	var phaseID, pid string
	err := tx.QueryRow(`
		SELECT pk.phase_id, ph.project_id
		FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pk.package_id=?`, packageID).Scan(&phaseID, &pid)
	if err != nil {
		return "", fmt.Errorf("하부 업무가 없습니다")
	}
	if pid != projectID {
		return "", fmt.Errorf("다른 사업의 하부 업무입니다")
	}
	return phaseID, nil
}

func missingQty(ids []string, qty map[string]int) int {
	n := 0
	for _, id := range ids {
		if qty[id] <= 0 {
			n++
		}
	}
	return n
}

func attachSitesTx(tx *sql.Tx, projectID, phaseID, packageID, planID string, ids []string, qty map[string]int, actor string, move bool) error {
	now := nowStamp()
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var pid, status string
		err := tx.QueryRow(`SELECT project_id, status FROM project_sites WHERE id=? AND is_active=1`, id).Scan(&pid, &status)
		if err != nil || pid != projectID || status == "excluded" {
			return fmt.Errorf("사이트를 고를 수 없습니다")
		}
		var curPlan, curStatus string
		err = tx.QueryRow(`SELECT plan_id, status FROM package_sites WHERE package_id=? AND project_site_id=?`, packageID, id).Scan(&curPlan, &curStatus)
		if err == nil {
			if curStatus == model.SiteDone && curPlan != "" && curPlan != planID {
				return fmt.Errorf("완료된 사이트는 옮길 수 없습니다")
			}
			if curPlan != "" && curPlan != planID && !move {
				return fmt.Errorf("다른 계획에 있는 사이트입니다")
			}
		}
		if _, err := tx.Exec(`
			INSERT INTO phase_sites(phase_site_id, phase_id, project_site_id, scope_status, added_at, added_by)
			VALUES (?,?,?, 'active', ?, ?)
			ON CONFLICT(phase_id, project_site_id) DO NOTHING`,
			fmt.Sprintf("HS-%s", id), phaseID, id, now, actor); err != nil {
			n, nerr := nextSeqTx(tx, hsSeqKey)
			if nerr != nil {
				return err
			}
			if _, err2 := tx.Exec(`
				INSERT INTO phase_sites(phase_site_id, phase_id, project_site_id, scope_status, added_at, added_by)
				VALUES (?,?,?, 'active', ?, ?)
				ON CONFLICT(phase_id, project_site_id) DO NOTHING`,
				fmt.Sprintf("HS-%03d", n), phaseID, id, now, actor); err2 != nil {
				return err2
			}
		}
		tq := 0
		if qty != nil {
			tq = qty[id]
		}
		res, err := tx.Exec(`
			UPDATE package_sites SET plan_id=?, target_qty=CASE WHEN ?>0 THEN ? ELSE target_qty END, updated_at=?
			WHERE package_id=? AND project_site_id=?`,
			planID, tq, tq, now, packageID, id)
		if err != nil {
			return err
		}
		aff, _ := res.RowsAffected()
		if aff > 0 {
			continue
		}
		n, err := nextSeqTx(tx, psSeqKey)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO package_sites(package_site_id, package_id, project_site_id, status, target_qty, done_qty, plan_id, updated_at)
			VALUES (?,?,?, 'planned', ?, 0, ?, ?)`,
			fmt.Sprintf("PS-%03d", n), packageID, id, tq, planID, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *ProjectRepo) MovePlanSites(projectID, planID string, ids []string, actor string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	pkg, phase, err := planOwnerTx(tx, planID, projectID)
	if err != nil {
		return err
	}
	if err := attachSitesTx(tx, projectID, phase, pkg, planID, ids, nil, actor, true); err != nil {
		return err
	}
	return tx.Commit()
}

func planOwnerTx(tx *sql.Tx, planID, projectID string) (packageID, phaseID string, err error) {
	var pid string
	err = tx.QueryRow(`
		SELECT pl.package_id, pk.phase_id, ph.project_id
		FROM package_plans pl
		JOIN phase_packages pk ON pk.package_id=pl.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pl.plan_id=?`, planID).Scan(&packageID, &phaseID, &pid)
	if err != nil {
		return "", "", fmt.Errorf("계획이 없습니다")
	}
	if projectID != "" && pid != projectID {
		return "", "", fmt.Errorf("다른 사업의 계획입니다")
	}
	return packageID, phaseID, nil
}

func (r *ProjectRepo) ExcludePlanSite(projectID, planID, siteID, reason string, confirmDone bool) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("제외 사유를 입력하세요")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	pkg, _, err := planOwnerTx(tx, planID, projectID)
	if err != nil {
		return err
	}
	var status string
	err = tx.QueryRow(`SELECT status FROM package_sites WHERE package_id=? AND project_site_id=? AND plan_id=?`,
		pkg, siteID, planID).Scan(&status)
	if err != nil {
		return fmt.Errorf("이 계획의 사이트가 아닙니다")
	}
	if status == model.SiteDone && !confirmDone {
		return fmt.Errorf("완료 이력은 보존되며 달성률 분모·분자에서 빠집니다")
	}
	if _, err := tx.Exec(`UPDATE package_sites SET status='excluded', exclude_reason=?, updated_at=? WHERE package_id=? AND project_site_id=?`,
		reason, nowStamp(), pkg, siteID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ProjectRepo) DeletePlan(projectID, planID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	pkg, _, err := planOwnerTx(tx, planID, projectID)
	if err != nil {
		return err
	}
	var perf, tasks int
	_ = tx.QueryRow(`SELECT COUNT(*) FROM package_sites WHERE plan_id=? AND status IN ('done','in_progress')`, planID).Scan(&perf)
	_ = tx.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE plan_id=?`, planID).Scan(&tasks)
	now := nowStamp()
	if perf > 0 || tasks > 0 {
		if _, err := tx.Exec(`UPDATE package_plans SET is_active=0, updated_at=? WHERE plan_id=?`, now, planID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err := tx.Exec(`UPDATE package_sites SET plan_id='', updated_at=? WHERE plan_id=?`, now, planID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM package_plans WHERE plan_id=?`, planID); err != nil {
		return err
	}
	_ = pkg
	return tx.Commit()
}

func (r *ProjectRepo) SetPlanStatus(projectID, planID, status, actor string) error {
	if status != model.PkgDone && status != model.PkgHold && status != model.PkgAuto {
		return fmt.Errorf("상태를 확인하세요")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, _, err := planOwnerTx(tx, planID, projectID); err != nil {
		return err
	}
	if status == model.PkgDone {
		var open int
		if err := tx.QueryRow(`
			SELECT COUNT(*) FROM package_sites
			WHERE plan_id=? AND status NOT IN ('done','excluded')`, planID).Scan(&open); err != nil {
			return err
		}
		if open > 0 {
			return fmt.Errorf("끝나지 않은 사이트가 있습니다")
		}
	}
	doneAt, doneBy := "", ""
	if status == model.PkgDone {
		doneAt, doneBy = nowStamp(), actor
	}
	_, err = tx.Exec(`UPDATE package_plans SET status=?, done_at=?, done_by=?, updated_at=? WHERE plan_id=?`,
		status, doneAt, doneBy, nowStamp(), planID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ProjectRepo) MovePlan(projectID, planID, dir string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var pkg string
	var seq int
	err = tx.QueryRow(`
		SELECT pl.package_id, pl.seq FROM package_plans pl
		JOIN phase_packages pk ON pk.package_id=pl.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pl.plan_id=? AND ph.project_id=?`, planID, projectID).Scan(&pkg, &seq)
	if err != nil {
		return fmt.Errorf("계획이 없습니다")
	}
	op := "<"
	ord := "DESC"
	if dir != "up" {
		op = ">"
		ord = "ASC"
	}
	var other string
	var oseq int
	err = tx.QueryRow(`SELECT plan_id, seq FROM package_plans WHERE package_id=? AND seq `+op+` ? ORDER BY seq `+ord+` LIMIT 1`,
		pkg, seq).Scan(&other, &oseq)
	if err != nil {
		return tx.Commit()
	}
	if _, err := tx.Exec(`UPDATE package_plans SET seq=? WHERE plan_id=?`, oseq, planID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE package_plans SET seq=? WHERE plan_id=?`, seq, other); err != nil {
		return err
	}
	return tx.Commit()
}

type PlanSiteHit struct {
	ID, Name, Region, PlanID, PlanName, Status string
	Taken                                      bool
}

func (r *ProjectRepo) SearchPlanSites(projectID, packageID, q, region string, offset, limit int) ([]PlanSiteHit, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.Query(`
		SELECT ps.id, COALESCE(ps.site_name,''), COALESCE(ps.region,''),
		       COALESCE(pk.plan_id,''), COALESCE(pl.name,''), COALESCE(pk.status,'')
		FROM project_sites ps
		LEFT JOIN package_sites pk ON pk.project_site_id=ps.id AND pk.package_id=?
		LEFT JOIN package_plans pl ON pl.plan_id=pk.plan_id
		WHERE ps.project_id=? AND ps.is_active=1 AND ps.status!='excluded'
		ORDER BY ps.site_name`, packageID, projectID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var all []PlanSiteHit
	for rows.Next() {
		var h PlanSiteHit
		if err := rows.Scan(&h.ID, &h.Name, &h.Region, &h.PlanID, &h.PlanName, &h.Status); err != nil {
			return nil, 0, err
		}
		if !model.RegionAuxMatch(h.Region, region) {
			continue
		}
		if !model.SiteImportMatch(h.Name, "", h.Region, q, "") {
			continue
		}
		h.Taken = h.PlanID != ""
		all = append(all, h)
	}
	total := len(all)
	if offset >= total {
		return []PlanSiteHit{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (r *WBRepo) ProjectHasPackages(projectID string) bool {
	return NewProjectRepo(r.db).ProjectHasPackages(projectID)
}

func (r *WBRepo) SetTaskPlanLink(taskID, projectID, packageID, planID string) error {
	if r == nil || r.db == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	if !tableHasColumn(r.db, "work_tasks", "plan_id") {
		return nil
	}
	packageID = strings.TrimSpace(packageID)
	planID = strings.TrimSpace(planID)
	projectID = strings.TrimSpace(projectID)
	if planID != "" {
		var pkg, pid, status string
		var active int
		err := r.db.QueryRow(`
			SELECT pl.package_id, ph.project_id, pl.status, pl.is_active
			FROM package_plans pl
			JOIN phase_packages pk ON pk.package_id=pl.package_id
			JOIN project_phases ph ON ph.phase_id=pk.phase_id
			WHERE pl.plan_id=?`, planID).Scan(&pkg, &pid, &status, &active)
		if err != nil || pid != projectID || active == 0 {
			return fmt.Errorf("계획을 고를 수 없습니다")
		}
		if status == model.PkgHold {
			return fmt.Errorf("보류 계획은 고를 수 없습니다")
		}
		packageID = pkg
	} else if packageID != "" {
		var pid string
		err := r.db.QueryRow(`
			SELECT ph.project_id FROM phase_packages pk
			JOIN project_phases ph ON ph.phase_id=pk.phase_id
			WHERE pk.package_id=? AND pk.is_active=1`, packageID).Scan(&pid)
		if err != nil || pid != projectID {
			return fmt.Errorf("하부 업무를 고를 수 없습니다")
		}
	}
	_, err := r.db.Exec(`UPDATE work_tasks SET package_id=?, plan_id=? WHERE task_id=?`, packageID, planID, taskID)
	return err
}

func (r *WBRepo) TaskPlanIDs(taskID string) (string, string) {
	if r == nil || !tableHasColumn(r.db, "work_tasks", "plan_id") {
		return "", ""
	}
	var pkg, plan string
	_ = r.db.QueryRow(`SELECT COALESCE(package_id,''), COALESCE(plan_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&pkg, &plan)
	return pkg, plan
}

func (r *WBRepo) TaskPlanLabel(taskID string) string {
	pkg, plan := r.TaskPlanIDs(taskID)
	if pkg == "" && plan == "" {
		return ""
	}
	var phaseName, pkgName, planName string
	_ = r.db.QueryRow(`
		SELECT COALESCE(ph.name,''), COALESCE(pk.name,''), COALESCE(pl.name,'')
		FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		LEFT JOIN package_plans pl ON pl.plan_id=?
		WHERE pk.package_id=?`, plan, pkg).Scan(&phaseName, &pkgName, &planName)
	parts := []string{}
	for _, s := range []string{phaseName, pkgName, planName} {
		if strings.TrimSpace(s) != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " › ")
}

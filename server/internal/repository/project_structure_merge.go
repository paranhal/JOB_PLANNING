package repository

import (
	"database/sql"
	"encoding/json"
	"strings"

	"customer-support/internal/model"
)

type structureSnap struct {
	Packages []snapPkg  `json:"packages"`
	Plans    []snapPlan `json:"plans"`
	Sites    []snapPS   `json:"sites"`
	PhaseS   []snapHS   `json:"phase_sites"`
	Pool     []snapPool `json:"pool"`
	Tasks    []snapTask `json:"tasks"`
}

type snapPkg struct {
	ID, PhaseID, PhaseName, ProjectID, Name, Track, Status string
	Seq                                                    int
	Active                                                 int
}
type snapPlan struct {
	ID, PackageID, Name, Status, Start, End, Basis, Unit string
	Seq, Active                                          int
}
type snapPS struct {
	ID, PackageID, SiteID, Status, PlanID string
	Target, Done                          int
}
type snapHS struct {
	ID, PhaseID, SiteID, Status string
}
type snapPool struct {
	ID, ProjectID, Name, Region, Status, OrgID string
	Active                                     int
}
type snapTask struct {
	ID, PackageID, PlanID string
}

func dropHasPlans(tx *sql.Tx, projectID string) bool {
	var n int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM package_plans pl
		JOIN phase_packages pk ON pk.package_id=pl.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=?`, projectID).Scan(&n)
	return err == nil && n > 0
}

func preparePlanMerge(tx *sql.Tx, keepID, dropID string) (structureSnap, bool, error) {
	var s structureSnap
	if !dropHasPlans(tx, dropID) {
		return s, false, nil
	}
	s, err := readStructureSnap(tx, keepID, dropID)
	return s, err == nil, err
}

func dropHasPackages(tx *sql.Tx, projectID string) bool {
	var n int
	err := tx.QueryRow(`
		SELECT COUNT(*) FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=?`, projectID).Scan(&n)
	return err == nil && n > 0
}

// 계획이 없어도 하부 업무가 있으면 되돌리기 스냅샷을 남긴다. 사이트 풀은 옮기지 않는다.
func preparePackageOnlySnap(tx *sql.Tx, keepID, dropID string) (structureSnap, bool, error) {
	var s structureSnap
	if dropHasPlans(tx, dropID) || !dropHasPackages(tx, dropID) {
		return s, false, nil
	}
	s, err := readStructureSnap(tx, keepID, dropID)
	if err != nil || len(s.Packages) == 0 {
		return s, false, err
	}
	return s, true, nil
}

func recordPackageSnap(tx *sql.Tx, mergeID, keepID, dropID string, snap structureSnap) error {
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO project_merge_moves (merge_id, table_name, pk, old_project_id, new_project_id, payload)
		VALUES (?,?,?,?,?,?)`, mergeID, "package_plans", "*", dropID, keepID, string(raw))
	return err
}

func applyPlanMerge(tx *sql.Tx, mergeID, keepID, dropID string, snap structureSnap) error {
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO project_merge_moves (merge_id, table_name, pk, old_project_id, new_project_id, payload)
		VALUES (?,?,?,?,?,?)`, mergeID, "package_plans", "*", dropID, keepID, string(raw)); err != nil {
		return err
	}
	if err := moveSitePool(tx, keepID, dropID); err != nil {
		return err
	}
	return foldPackages(tx, keepID, snap)
}

func readStructureSnap(tx *sql.Tx, keepID, dropID string) (structureSnap, error) {
	var s structureSnap
	rows, err := tx.Query(`
		SELECT pk.package_id, pk.phase_id, ph.name, ph.project_id, pk.name, pk.track_mode, pk.status, pk.seq, pk.is_active
		FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id IN (?,?)`, keepID, dropID)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var p snapPkg
		if err := rows.Scan(&p.ID, &p.PhaseID, &p.PhaseName, &p.ProjectID, &p.Name, &p.Track, &p.Status, &p.Seq, &p.Active); err != nil {
			rows.Close()
			return s, err
		}
		s.Packages = append(s.Packages, p)
	}
	rows.Close()
	rows, err = tx.Query(`
		SELECT pl.plan_id, pl.package_id, pl.name, pl.status, pl.plan_start, pl.plan_end, pl.progress_basis, pl.qty_unit, pl.seq, pl.is_active
		FROM package_plans pl JOIN phase_packages pk ON pk.package_id=pl.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id IN (?,?)`, keepID, dropID)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var p snapPlan
		if err := rows.Scan(&p.ID, &p.PackageID, &p.Name, &p.Status, &p.Start, &p.End, &p.Basis, &p.Unit, &p.Seq, &p.Active); err != nil {
			rows.Close()
			return s, err
		}
		s.Plans = append(s.Plans, p)
	}
	rows.Close()
	rows, err = tx.Query(`
		SELECT ps.package_site_id, ps.package_id, ps.project_site_id, ps.status, ps.plan_id, ps.target_qty, ps.done_qty
		FROM package_sites ps JOIN phase_packages pk ON pk.package_id=ps.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id IN (?,?)`, keepID, dropID)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var p snapPS
		if err := rows.Scan(&p.ID, &p.PackageID, &p.SiteID, &p.Status, &p.PlanID, &p.Target, &p.Done); err != nil {
			rows.Close()
			return s, err
		}
		s.Sites = append(s.Sites, p)
	}
	rows.Close()
	rows, err = tx.Query(`
		SELECT id, project_id, site_name, region, status, org_id, is_active
		FROM project_sites WHERE project_id IN (?,?)`, keepID, dropID)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var p snapPool
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Region, &p.Status, &p.OrgID, &p.Active); err != nil {
			rows.Close()
			return s, err
		}
		s.Pool = append(s.Pool, p)
	}
	rows.Close()
	rows, err = tx.Query(`
		SELECT task_id, COALESCE(package_id,''), COALESCE(plan_id,'')
		FROM work_tasks WHERE project_id IN (?,?) AND (COALESCE(package_id,'')!='' OR COALESCE(plan_id,'')!='')`, keepID, dropID)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var t snapTask
		if err := rows.Scan(&t.ID, &t.PackageID, &t.PlanID); err != nil {
			rows.Close()
			return s, err
		}
		s.Tasks = append(s.Tasks, t)
	}
	rows.Close()
	return s, nil
}

func moveSitePool(tx *sql.Tx, keepID, dropID string) error {
	rows, err := tx.Query(`SELECT id, site_name FROM project_sites WHERE project_id=?`, dropID)
	if err != nil {
		return err
	}
	type pair struct{ id, name string }
	var list []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.name); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	rows.Close()
	for _, p := range list {
		var keepSite string
		err := tx.QueryRow(`SELECT id FROM project_sites WHERE project_id=? AND site_name=?`, keepID, p.name).Scan(&keepSite)
		if err == nil && keepSite != "" && keepSite != p.id {
			if _, err := tx.Exec(`UPDATE package_sites SET project_site_id=? WHERE project_site_id=?`, keepSite, p.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE phase_sites SET project_site_id=? WHERE project_site_id=?`, keepSite, p.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM project_sites WHERE id=?`, p.id); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(`UPDATE project_sites SET project_id=? WHERE id=?`, keepID, p.id); err != nil {
			return err
		}
	}
	return nil
}

func foldPackages(tx *sql.Tx, keepID string, snap structureSnap) error {
	for _, p := range snap.Packages {
		if p.ProjectID != "" && !phaseWasDrop(tx, keepID, p) {
			continue
		}
		target, err := keptPhaseID(tx, keepID, p.PhaseID, p.PhaseName)
		if err != nil {
			return err
		}
		var keepPkg string
		err = tx.QueryRow(`SELECT package_id FROM phase_packages WHERE phase_id=? AND name=? AND package_id!=?`,
			target, p.Name, p.ID).Scan(&keepPkg)
		if err == sql.ErrNoRows || keepPkg == "" {
			if _, err := tx.Exec(`UPDATE phase_packages SET phase_id=? WHERE package_id=?`, target, p.ID); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if err := foldPlans(tx, keepPkg, p.ID); err != nil {
			return err
		}
		if err := foldPackageSites(tx, keepPkg, p.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE work_tasks SET package_id=? WHERE package_id=?`, keepPkg, p.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM phase_packages WHERE package_id=?`, p.ID); err != nil {
			return err
		}
	}
	return nil
}

func phaseWasDrop(tx *sql.Tx, keepID string, p snapPkg) bool {
	var pid string
	err := tx.QueryRow(`SELECT project_id FROM project_phases WHERE phase_id=?`, p.PhaseID).Scan(&pid)
	if err == nil && pid == keepID && p.ProjectID == keepID {
		return false
	}
	return p.ProjectID != keepID
}

func keptPhaseID(tx *sql.Tx, keepID, dropPhase, phaseName string) (string, error) {
	var pid string
	err := tx.QueryRow(`SELECT project_id FROM project_phases WHERE phase_id=?`, dropPhase).Scan(&pid)
	if err == nil && pid == keepID {
		return dropPhase, nil
	}
	key := model.NormalizePhaseMergeName(phaseName)
	rows, err := tx.Query(`SELECT phase_id, name FROM project_phases WHERE project_id=?`, keepID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id, n string
		if err := rows.Scan(&id, &n); err != nil {
			return "", err
		}
		if model.NormalizePhaseMergeName(n) == key {
			return id, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return dropPhase, nil
}

func foldPlans(tx *sql.Tx, keepPkg, dropPkg string) error {
	rows, err := tx.Query(`SELECT plan_id, name FROM package_plans WHERE package_id=?`, dropPkg)
	if err != nil {
		return err
	}
	type pl struct{ id, name string }
	var list []pl
	for rows.Next() {
		var p pl
		if err := rows.Scan(&p.id, &p.name); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	rows.Close()
	for _, p := range list {
		var keepPlan string
		err := tx.QueryRow(`SELECT plan_id FROM package_plans WHERE package_id=? AND name=?`, keepPkg, p.name).Scan(&keepPlan)
		if err == sql.ErrNoRows {
			if _, err := tx.Exec(`UPDATE package_plans SET package_id=? WHERE plan_id=?`, keepPkg, p.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE package_sites SET package_id=?, plan_id=? WHERE plan_id=?`, keepPkg, p.id, p.id); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE package_sites SET plan_id=? WHERE plan_id=?`, keepPlan, p.id); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE work_tasks SET plan_id=? WHERE plan_id=?`, keepPlan, p.id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM package_plans WHERE plan_id=?`, p.id); err != nil {
			return err
		}
	}
	return nil
}

func foldPackageSites(tx *sql.Tx, keepPkg, dropPkg string) error {
	rows, err := tx.Query(`SELECT package_site_id, project_site_id, status, plan_id, target_qty, done_qty FROM package_sites WHERE package_id=?`, dropPkg)
	if err != nil {
		return err
	}
	type row struct {
		id, site, status, plan string
		target, done           int
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.site, &r.status, &r.plan, &r.target, &r.done); err != nil {
			rows.Close()
			return err
		}
		list = append(list, r)
	}
	rows.Close()
	for _, r := range list {
		var exist string
		err := tx.QueryRow(`SELECT package_site_id FROM package_sites WHERE package_id=? AND project_site_id=?`, keepPkg, r.site).Scan(&exist)
		if err == sql.ErrNoRows {
			if _, err := tx.Exec(`UPDATE package_sites SET package_id=? WHERE package_site_id=?`, keepPkg, r.id); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM package_sites WHERE package_site_id=?`, r.id); err != nil {
			return err
		}
	}
	return nil
}

func restoreStructureSnapshot(tx *sql.Tx, payload string) error {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil
	}
	var s structureSnap
	if err := json.Unmarshal([]byte(payload), &s); err != nil {
		return err
	}
	for _, p := range s.Pool {
		if _, err := tx.Exec(`
			INSERT INTO project_sites(id, project_id, site_name, region, status, org_id, is_active)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET project_id=excluded.project_id, site_name=excluded.site_name, status=excluded.status, is_active=excluded.is_active`,
			p.ID, p.ProjectID, p.Name, p.Region, p.Status, p.OrgID, p.Active); err != nil {
			return err
		}
	}
	for _, p := range s.Packages {
		if _, err := tx.Exec(`
			INSERT INTO phase_packages(package_id, phase_id, seq, name, track_mode, status, weight, is_active, created_at, updated_at)
			VALUES (?,?,?,?,?,?,1,?,'','')
			ON CONFLICT(package_id) DO UPDATE SET phase_id=excluded.phase_id, name=excluded.name, is_active=excluded.is_active`,
			p.ID, p.PhaseID, p.Seq, p.Name, p.Track, p.Status, p.Active); err != nil {
			return err
		}
	}
	for _, p := range s.Plans {
		if _, err := tx.Exec(`
			INSERT INTO package_plans(plan_id, package_id, seq, name, status, plan_start, plan_end, progress_basis, qty_unit, is_active, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,'','')
			ON CONFLICT(plan_id) DO UPDATE SET package_id=excluded.package_id, name=excluded.name, status=excluded.status, is_active=excluded.is_active`,
			p.ID, p.PackageID, p.Seq, p.Name, p.Status, p.Start, p.End, p.Basis, p.Unit, p.Active); err != nil {
			return err
		}
	}
	for _, p := range s.Sites {
		if _, err := tx.Exec(`
			INSERT INTO package_sites(package_site_id, package_id, project_site_id, status, plan_id, target_qty, done_qty, updated_at)
			VALUES (?,?,?,?,?,?,?,'')
			ON CONFLICT(package_site_id) DO UPDATE SET package_id=excluded.package_id, project_site_id=excluded.project_site_id, status=excluded.status, plan_id=excluded.plan_id`,
			p.ID, p.PackageID, p.SiteID, p.Status, p.PlanID, p.Target, p.Done); err != nil {
			return err
		}
	}
	for _, t := range s.Tasks {
		if _, err := tx.Exec(`UPDATE work_tasks SET package_id=?, plan_id=? WHERE task_id=?`, t.PackageID, t.PlanID, t.ID); err != nil {
			return err
		}
	}
	return nil
}

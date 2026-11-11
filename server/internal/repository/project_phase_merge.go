package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type phaseSnap struct {
	Rows []model.ProjectPhase `json:"rows"`
}

func mergeProjectPhases(tx *sql.Tx, mergeID, keepID, dropID string) error {
	keep, err := listPhasesTx(tx, keepID)
	if err != nil {
		return err
	}
	drop, err := listPhasesTx(tx, dropID)
	if err != nil {
		return err
	}
	if len(keep) == 0 && len(drop) == 0 {
		return nil
	}
	snap := phaseSnap{Rows: append(append([]model.ProjectPhase{}, keep...), drop...)}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO project_merge_moves (merge_id, table_name, pk, old_project_id, new_project_id, payload)
		VALUES (?,?,?,?,?,?)`, mergeID, "project_phases", "*", dropID, keepID, string(raw)); err != nil {
		return err
	}
	byName := map[string]model.ProjectPhase{}
	var order []string
	for _, p := range keep {
		byName[model.NormalizePhaseMergeName(p.Name)] = p
		order = append(order, p.PhaseID)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, d := range drop {
		key := model.NormalizePhaseMergeName(d.Name)
		if k, ok := byName[key]; ok {
			merged := mergePhaseFields(k, d)
			active := 0
			if merged.IsActive {
				active = 1
			}
			if _, err := tx.Exec(`
				UPDATE project_phases SET status=?, plan_start=?, plan_end=?, actual_start=?, actual_end=?,
					note=?, is_active=?, updated_at=?
				WHERE phase_id=?`,
				merged.Status, merged.PlanStart, merged.PlanEnd, merged.ActualStart, merged.ActualEnd,
				merged.Note, active, now, k.PhaseID); err != nil {
				return err
			}
			if err := rehomePhaseChildren(tx, d.PhaseID, k.PhaseID); err != nil {
				return err
			}
			if phaseStillHasChildren(tx, d.PhaseID) {
				continue
			}
			if _, err := tx.Exec(`DELETE FROM project_phases WHERE phase_id=?`, d.PhaseID); err != nil {
				return err
			}
			continue
		}
		var org string
		if len(order) > 0 {
			_ = tx.QueryRow(`SELECT org_id FROM project_phases WHERE phase_id=?`, order[0]).Scan(&org)
		}
		if org == "" {
			_ = tx.QueryRow(`SELECT org_id FROM work_projects WHERE project_id=?`, keepID).Scan(&org)
		}
		if _, err := tx.Exec(`UPDATE project_phases SET project_id=?, org_id=?, updated_at=? WHERE phase_id=?`,
			keepID, org, now, d.PhaseID); err != nil {
			return err
		}
		order = append(order, d.PhaseID)
		byName[key] = d
	}
	for i, id := range order {
		if _, err := tx.Exec(`UPDATE project_phases SET seq=? WHERE phase_id=?`, i, id); err != nil {
			return err
		}
	}
	return nil
}

func mergePhaseFields(keep, drop model.ProjectPhase) model.ProjectPhase {
	out := keep
	if model.PhaseStatusRank(drop.Status) > model.PhaseStatusRank(keep.Status) {
		out.Status = drop.Status
	}
	if strings.TrimSpace(out.PlanStart) == "" {
		out.PlanStart = drop.PlanStart
	}
	if strings.TrimSpace(out.PlanEnd) == "" {
		out.PlanEnd = drop.PlanEnd
	}
	if strings.TrimSpace(out.ActualStart) == "" {
		out.ActualStart = drop.ActualStart
	}
	if strings.TrimSpace(out.ActualEnd) == "" {
		out.ActualEnd = drop.ActualEnd
	}
	dn := strings.TrimSpace(drop.Note)
	if dn != "" && !strings.Contains(out.Note, dn) {
		if strings.TrimSpace(out.Note) == "" {
			out.Note = dn
		} else {
			out.Note = strings.TrimSpace(out.Note) + "\n" + dn
		}
	}
	out.IsActive = keep.IsActive || drop.IsActive
	return out
}

func listPhasesTx(tx *sql.Tx, projectID string) ([]model.ProjectPhase, error) {
	rows, err := tx.Query(`
		SELECT phase_id, project_id, org_id, seq, name,
			plan_start, plan_end, actual_start, actual_end, status, note, is_active,
			created_at, updated_at
		FROM project_phases WHERE project_id=? ORDER BY seq, phase_id`, projectID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.ProjectPhase
	for rows.Next() {
		p, err := scanPhase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func restorePhaseSnapshot(tx *sql.Tx, payload string) error {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil
	}
	var snap phaseSnap
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		return err
	}
	for _, p := range snap.Rows {
		active := 0
		if p.IsActive {
			active = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO project_phases(
				phase_id, project_id, org_id, seq, name,
				plan_start, plan_end, actual_start, actual_end, status, note, is_active,
				created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(phase_id) DO UPDATE SET
				project_id=excluded.project_id, org_id=excluded.org_id, seq=excluded.seq, name=excluded.name,
				plan_start=excluded.plan_start, plan_end=excluded.plan_end,
				actual_start=excluded.actual_start, actual_end=excluded.actual_end,
				status=excluded.status, note=excluded.note, is_active=excluded.is_active,
				updated_at=excluded.updated_at`,
			p.PhaseID, p.ProjectID, p.OrgID, p.Seq, p.Name,
			p.PlanStart, p.PlanEnd, p.ActualStart, p.ActualEnd, p.Status, p.Note, active,
			p.CreatedAt, p.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

// rehomePhaseChildren 같은 이름 단계가 지워지기 전에 하부 업무를 대상 단계로 옮긴다.
// 같은 이름 하부 업무는 하나로 합치고, 연결 작업의 package_id 를 따라가게 한다.
func rehomePhaseChildren(tx *sql.Tx, fromPhase, toPhase string) error {
	rows, err := tx.Query(`SELECT package_id, name FROM phase_packages WHERE phase_id=? ORDER BY seq, package_id`, fromPhase)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
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
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range list {
		keepPkg, err := packageByNormName(tx, toPhase, p.name, p.id)
		if err != nil {
			return err
		}
		if keepPkg == "" {
			if _, err := tx.Exec(`UPDATE phase_packages SET phase_id=? WHERE package_id=?`, toPhase, p.id); err != nil {
				return err
			}
			continue
		}
		if err := foldPlans(tx, keepPkg, p.id); err != nil {
			return err
		}
		if err := foldPackageSites(tx, keepPkg, p.id); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE work_tasks SET package_id=? WHERE package_id=?`, keepPkg, p.id); err != nil && !strings.Contains(err.Error(), "no such") {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM phase_packages WHERE package_id=?`, p.id); err != nil {
			return err
		}
	}
	return movePhaseSites(tx, fromPhase, toPhase)
}

func packageByNormName(tx *sql.Tx, phaseID, name, except string) (string, error) {
	key := model.NormalizePhaseMergeName(name)
	rows, err := tx.Query(`SELECT package_id, name FROM phase_packages WHERE phase_id=?`, phaseID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id, n string
		if err := rows.Scan(&id, &n); err != nil {
			return "", err
		}
		if id == except {
			continue
		}
		if model.NormalizePhaseMergeName(n) == key {
			return id, nil
		}
	}
	return "", rows.Err()
}

func movePhaseSites(tx *sql.Tx, fromPhase, toPhase string) error {
	rows, err := tx.Query(`SELECT project_site_id, scope_status, added_at, added_by FROM phase_sites WHERE phase_id=?`, fromPhase)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	type hs struct{ site, scope, at, by string }
	var list []hs
	for rows.Next() {
		var r hs
		if err := rows.Scan(&r.site, &r.scope, &r.at, &r.by); err != nil {
			rows.Close()
			return err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, r := range list {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM phase_sites WHERE phase_id=? AND project_site_id=?`, toPhase, r.site).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		seq, err := nextSeqTx(tx, hsSeqKey)
		if err != nil {
			return err
		}
		if r.at == "" {
			r.at = now
		}
		if _, err := tx.Exec(`
			INSERT INTO phase_sites(phase_site_id, phase_id, project_site_id, scope_status, added_at, added_by)
			VALUES (?,?,?,?,?,?)`,
			fmt.Sprintf("HS-%03d", seq), toPhase, r.site, r.scope, r.at, r.by); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`DELETE FROM phase_sites WHERE phase_id=?`, fromPhase)
	return err
}

// phaseStillHasChildren 하부 업무나 그 업무에 연결된 작업이 남아 있으면 단계를 지우지 않는다.
func phaseStillHasChildren(tx *sql.Tx, phaseID string) bool {
	var n int
	err := tx.QueryRow(`SELECT COUNT(*) FROM phase_packages WHERE phase_id=?`, phaseID).Scan(&n)
	if err != nil {
		return !strings.Contains(err.Error(), "no such table")
	}
	if n > 0 {
		return true
	}
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM work_tasks
		WHERE package_id IN (SELECT package_id FROM phase_packages WHERE phase_id=?)
		   OR plan_id IN (
		        SELECT pl.plan_id FROM package_plans pl
		        JOIN phase_packages pk ON pk.package_id=pl.package_id
		        WHERE pk.phase_id=?)`, phaseID, phaseID).Scan(&n)
	if err != nil {
		return !strings.Contains(err.Error(), "no such")
	}
	return n > 0
}

package repository

import (
	"database/sql"
	"encoding/json"
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

package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

const phaseSeqKey = "project_phase"

// ErrPhasePriorConfirm 앞 단계가 남아 있어 확인 전에는 진행으로 바꾸지 않는다.
var ErrPhasePriorConfirm = errors.New("앞 단계를 완료할지 확인이 필요합니다")

func (r *ProjectRepo) ListPhases(projectID string) ([]model.ProjectPhase, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("db nil")
	}
	rows, err := r.db.Query(`
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

func (r *ProjectRepo) GetPhase(phaseID string) (*model.ProjectPhase, error) {
	row := r.db.QueryRow(`
		SELECT phase_id, project_id, org_id, seq, name,
			plan_start, plan_end, actual_start, actual_end, status, note, is_active,
			created_at, updated_at
		FROM project_phases WHERE phase_id=?`, phaseID)
	p, err := scanPhase(row)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

type phaseScanner interface {
	Scan(dest ...any) error
}

func scanPhase(s phaseScanner) (model.ProjectPhase, error) {
	var p model.ProjectPhase
	var active int
	err := s.Scan(&p.PhaseID, &p.ProjectID, &p.OrgID, &p.Seq, &p.Name,
		&p.PlanStart, &p.PlanEnd, &p.ActualStart, &p.ActualEnd, &p.Status, &p.Note, &active,
		&p.CreatedAt, &p.UpdatedAt)
	p.IsActive = active != 0
	return p, err
}

// PhaseHasLinks 하부 업무·작업이 붙으면 여기서 센다. 지금은 항상 없다.
func (r *ProjectRepo) PhaseHasLinks(phaseID string) (bool, error) {
	_ = phaseID
	return false, nil
}

func (r *ProjectRepo) CreatePhase(p *model.ProjectPhase) error {
	if p == nil {
		return fmt.Errorf("단계가 없습니다")
	}
	name, err := model.PhaseNameOK(p.Name)
	if err != nil {
		return err
	}
	ps, pe, as, ae, err := model.PhaseDatesOK(p.PlanStart, p.PlanEnd, p.ActualStart, p.ActualEnd)
	if err != nil {
		return err
	}
	if !model.ValidPhaseStatus(p.Status) {
		p.Status = model.PhasePlanned
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := phaseNameTaken(tx, p.ProjectID, name, ""); err != nil {
		return err
	}
	n, err := nextSeqTx(tx, phaseSeqKey)
	if err != nil {
		return err
	}
	p.PhaseID = fmt.Sprintf("PH-%03d", n)
	p.Name = name
	p.PlanStart, p.PlanEnd, p.ActualStart, p.ActualEnd = ps, pe, as, ae
	now := time.Now().Format("2006-01-02 15:04:05")
	p.CreatedAt, p.UpdatedAt = now, now
	var seq int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(seq),-1)+1 FROM project_phases WHERE project_id=?`, p.ProjectID).Scan(&seq); err != nil {
		return err
	}
	p.Seq = seq
	active := 1
	p.IsActive = true
	_, err = tx.Exec(`
		INSERT INTO project_phases(
			phase_id, project_id, org_id, seq, name,
			plan_start, plan_end, actual_start, actual_end, status, note, is_active,
			created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.PhaseID, p.ProjectID, p.OrgID, p.Seq, p.Name,
		p.PlanStart, p.PlanEnd, p.ActualStart, p.ActualEnd, p.Status, p.Note, active,
		p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func phaseNameTaken(tx *sql.Tx, projectID, name, exceptID string) error {
	var n int
	err := tx.QueryRow(`SELECT COUNT(*) FROM project_phases WHERE project_id=? AND name=? AND phase_id<>?`,
		projectID, name, exceptID).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("같은 이름의 단계가 있습니다")
	}
	return nil
}

// UpdatePhase 단계를 저장한다. 마감일 변경 이력·연장 사유 검사는 하지 않는다(통합·복원 등 내부 경로).
// 화면의 저장은 UpdatePhaseExt 를 쓴다.
func (r *ProjectRepo) UpdatePhase(p *model.ProjectPhase) error {
	return r.UpdatePhaseExt(p, nil)
}

// UpdatePhaseExt 단계를 저장하고, 마감일(plan_end)이 바뀌면 이력을 남긴다. ext 가 nil 이면 이력·사유 검사를 건너뛴다.
// 연장(마감을 뒤로 미룸)은 사유가 필수다. 앞당김·해제는 이력만 남기고 사유는 선택이다. 95-179.
func (r *ProjectRepo) UpdatePhaseExt(p *model.ProjectPhase, ext *DateChangeCtx) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := updatePhaseTx(tx, p, ext); err != nil {
		return err
	}
	return tx.Commit()
}

// updatePhaseTx UpdatePhaseExt 의 본체. 호출한 쪽이 트랜잭션을 연다(「변경 내용 저장」 한 번에 묶기). 95-183.
func updatePhaseTx(tx *sql.Tx, p *model.ProjectPhase, ext *DateChangeCtx) error {
	if p == nil || strings.TrimSpace(p.PhaseID) == "" {
		return fmt.Errorf("단계가 없습니다")
	}
	name, err := model.PhaseNameOK(p.Name)
	if err != nil {
		return err
	}
	ps, pe, as, ae, err := model.PhaseDatesOK(p.PlanStart, p.PlanEnd, p.ActualStart, p.ActualEnd)
	if err != nil {
		return err
	}
	if !model.ValidPhaseStatus(p.Status) {
		return fmt.Errorf("단계 상태가 아닙니다")
	}
	cur, err := getPhaseTx(tx, p.PhaseID)
	if err != nil {
		return err
	}
	if err := phaseNameTaken(tx, cur.ProjectID, name, cur.PhaseID); err != nil {
		return err
	}
	if ext != nil {
		if err := recordEndChangeTx(tx, ext, cur.ProjectID, cur.OrgID, "phase", cur.PhaseID, name, cur.PlanEnd, pe); err != nil {
			return err
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	active := 0
	if p.IsActive {
		active = 1
	}
	_, err = tx.Exec(`
		UPDATE project_phases SET name=?, plan_start=?, plan_end=?, actual_start=?, actual_end=?,
			status=?, note=?, is_active=?, updated_at=?
		WHERE phase_id=?`,
		name, ps, pe, as, ae, p.Status, strings.TrimSpace(p.Note), active, now, cur.PhaseID)
	if err != nil {
		return err
	}
	p.Name, p.PlanStart, p.PlanEnd, p.ActualStart, p.ActualEnd = name, ps, pe, as, ae
	p.ProjectID, p.OrgID, p.Seq = cur.ProjectID, cur.OrgID, cur.Seq
	p.UpdatedAt = now
	return nil
}

func getPhaseTx(tx *sql.Tx, id string) (model.ProjectPhase, error) {
	row := tx.QueryRow(`
		SELECT phase_id, project_id, org_id, seq, name,
			plan_start, plan_end, actual_start, actual_end, status, note, is_active,
			created_at, updated_at
		FROM project_phases WHERE phase_id=?`, id)
	return scanPhase(row)
}

// SetPhaseStatus 진행 전환 때 앞 단계가 남으면 확인 전에는 저장하지 않는다.
// confirm 이면 앞 단계를 완료로 두고 이 단계를 진행으로 둔다.
func (r *ProjectRepo) SetPhaseStatus(phaseID, status, today string, confirm bool) (int, error) {
	if !model.ValidPhaseStatus(status) {
		return 0, fmt.Errorf("단계 상태가 아닙니다")
	}
	today = model.NormalizeAppDate(today)
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	cur, err := getPhaseTx(tx, phaseID)
	if err != nil {
		return 0, err
	}
	var priors []model.ProjectPhase
	if status == model.PhaseActive {
		rows, err := tx.Query(`
			SELECT phase_id, project_id, org_id, seq, name,
				plan_start, plan_end, actual_start, actual_end, status, note, is_active,
				created_at, updated_at
			FROM project_phases
			WHERE project_id=? AND is_active=1 AND seq<? AND status<>?
			ORDER BY seq`, cur.ProjectID, cur.Seq, model.PhaseDone)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			p, err := scanPhase(rows)
			if err != nil {
				rows.Close()
				return 0, err
			}
			priors = append(priors, p)
		}
		rows.Close()
		if len(priors) > 0 && !confirm {
			return len(priors), ErrPhasePriorConfirm
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if confirm {
		for _, p := range priors {
			end := p.ActualEnd
			if strings.TrimSpace(end) == "" {
				end = today
			}
			if _, err := tx.Exec(`UPDATE project_phases SET status=?, actual_end=?, updated_at=? WHERE phase_id=?`,
				model.PhaseDone, end, now, p.PhaseID); err != nil {
				return 0, err
			}
		}
	}
	start, end := cur.ActualStart, cur.ActualEnd
	if status == model.PhaseActive && strings.TrimSpace(start) == "" {
		start = today
	}
	if status == model.PhaseDone && strings.TrimSpace(end) == "" {
		end = today
	}
	if _, err := tx.Exec(`UPDATE project_phases SET status=?, actual_start=?, actual_end=?, updated_at=? WHERE phase_id=?`,
		status, start, end, now, cur.PhaseID); err != nil {
		return 0, err
	}
	return len(priors), tx.Commit()
}

func (r *ProjectRepo) DeletePhase(phaseID string) (deactivated bool, err error) {
	linked, err := r.PhaseHasLinks(phaseID)
	if err != nil {
		return false, err
	}
	if linked {
		_, err = r.db.Exec(`UPDATE project_phases SET is_active=0, updated_at=? WHERE phase_id=?`,
			time.Now().Format("2006-01-02 15:04:05"), phaseID)
		return true, err
	}
	res, err := r.db.Exec(`DELETE FROM project_phases WHERE phase_id=?`, phaseID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, sql.ErrNoRows
	}
	return false, nil
}

func (r *ProjectRepo) SetPhaseActive(phaseID string, active bool) error {
	v := 0
	if active {
		v = 1
	}
	_, err := r.db.Exec(`UPDATE project_phases SET is_active=?, updated_at=? WHERE phase_id=?`,
		v, time.Now().Format("2006-01-02 15:04:05"), phaseID)
	return err
}

func (r *ProjectRepo) MovePhase(phaseID string, dir int) error {
	if dir != -1 && dir != 1 {
		return fmt.Errorf("순서 방향이 아닙니다")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	cur, err := getPhaseTx(tx, phaseID)
	if err != nil {
		return err
	}
	var otherID string
	var otherSeq int
	q := `SELECT phase_id, seq FROM project_phases WHERE project_id=? AND seq`
	if dir < 0 {
		q += `<? ORDER BY seq DESC LIMIT 1`
	} else {
		q += `>? ORDER BY seq ASC LIMIT 1`
	}
	err = tx.QueryRow(q, cur.ProjectID, cur.Seq).Scan(&otherID, &otherSeq)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := tx.Exec(`UPDATE project_phases SET seq=?, updated_at=? WHERE phase_id=?`, otherSeq, now, cur.PhaseID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE project_phases SET seq=?, updated_at=? WHERE phase_id=?`, cur.Seq, now, otherID); err != nil {
		return err
	}
	return tx.Commit()
}

// CreatePhasesSkipDup 미리보기에서 고친 행을 저장한다. 같은 이름은 건너뛴다.
func (r *ProjectRepo) CreatePhasesSkipDup(projectID, orgID string, rows []model.ProjectPhase) (added, skipped int, err error) {
	for i := range rows {
		rows[i].ProjectID = projectID
		rows[i].OrgID = orgID
		rows[i].Status = model.PhasePlanned
		rows[i].IsActive = true
		err := r.CreatePhase(&rows[i])
		if err != nil && strings.Contains(err.Error(), "같은 이름") {
			skipped++
			continue
		}
		if err != nil {
			return added, skipped, err
		}
		added++
	}
	return added, skipped, nil
}

// PhaseSummaries 사업 ID 목록을 한 번에 집계한다. 호출당 쿼리 1번.
func (r *ProjectRepo) PhaseSummaries(projectIDs []string, today string) (map[string]model.PhaseListSummary, error) {
	out := map[string]model.PhaseListSummary{}
	if len(projectIDs) == 0 || r == nil || r.db == nil {
		return out, nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range projectIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
		out[id] = model.PhaseListSummary{Current: "—"}
	}
	q := `SELECT phase_id, project_id, org_id, seq, name,
		plan_start, plan_end, actual_start, actual_end, status, note, is_active,
		created_at, updated_at
		FROM project_phases WHERE project_id IN (` + strings.Join(ph, ",") + `) ORDER BY project_id, seq`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	grouped := map[string][]model.ProjectPhase{}
	for rows.Next() {
		p, err := scanPhase(rows)
		if err != nil {
			return nil, err
		}
		grouped[p.ProjectID] = append(grouped[p.ProjectID], p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for id, list := range grouped {
		out[id] = model.SummarizePhases(list, today)
	}
	return out, nil
}

func (r *ProjectRepo) deletePhasesTx(tx *sql.Tx, projectID string) error {
	for _, q := range []string{
		`DELETE FROM package_sites WHERE package_id IN (
			SELECT package_id FROM phase_packages WHERE phase_id IN (
				SELECT phase_id FROM project_phases WHERE project_id=?))`,
		`DELETE FROM package_plans WHERE package_id IN (
			SELECT package_id FROM phase_packages WHERE phase_id IN (
				SELECT phase_id FROM project_phases WHERE project_id=?))`,
		`DELETE FROM phase_packages WHERE phase_id IN (
			SELECT phase_id FROM project_phases WHERE project_id=?)`,
		`DELETE FROM phase_sites WHERE phase_id IN (
			SELECT phase_id FROM project_phases WHERE project_id=?)`,
	} {
		if _, err := tx.Exec(q, projectID); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return err
		}
	}
	_, err := tx.Exec(`DELETE FROM project_phases WHERE project_id=?`, projectID)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return nil
	}
	return err
}

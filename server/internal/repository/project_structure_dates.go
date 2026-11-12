package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// 95-179: 하부 업무 일자·배지와 마감일 변경(연장) 이력.

const dateChangeSeqKey = "structure_date_change"

// DateChangeCtx 마감일 변경 이력에 남길 사람과 사유.
type DateChangeCtx struct {
	Actor  string // 사용자 ID
	Reason string // 연장이면 필수
}

// DateChange 마감일 변경 이력 한 줄(「변경내역」 목록).
type DateChange struct {
	ID, ProjectID, Level, TargetID, TargetName, Field, Kind string
	OldValue, NewValue, Reason, ChangedBy, ChangedByName    string
	ChangedAt                                               string
}

// DateChangeKindLabel 화면용 종류 이름.
func DateChangeKindLabel(kind string) string {
	switch kind {
	case model.EndChangeExtend:
		return "연장"
	case model.EndChangeShorten:
		return "앞당김"
	case model.EndChangeClear:
		return "마감 해제"
	}
	return kind
}

// recordEndChangeTx 마감일이 바뀌었으면 이력을 한 줄 남긴다. 연장은 사유가 필수다. 처음 정하는 것은 이력에 남기지 않는다.
func recordEndChangeTx(tx *sql.Tx, ext *DateChangeCtx, projectID, orgID, level, targetID, targetName, oldEnd, newEnd string) error {
	kind := model.EndChangeKind(oldEnd, newEnd)
	if kind == model.EndChangeNone || kind == model.EndChangeSet {
		return nil
	}
	reason, err := model.ExtendReasonOK(ext.Reason)
	if err != nil {
		return err
	}
	if kind == model.EndChangeExtend && reason == "" {
		return fmt.Errorf("마감일을 연장하려면 연장 사유를 입력하세요")
	}
	seq, err := nextSeqTx(tx, dateChangeSeqKey)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO structure_date_changes(change_id, project_id, org_id, level, target_id, target_name, field, kind,
			old_value, new_value, reason, changed_by, changed_at)
		VALUES (?,?,?,?,?,?, 'plan_end', ?,?,?,?,?,?)`,
		fmt.Sprintf("DC-%03d", seq), projectID, orgID, level, targetID, targetName, kind,
		strings.TrimSpace(oldEnd), strings.TrimSpace(newEnd), reason, strings.TrimSpace(ext.Actor), model.LocalNowString())
	return err
}

// ListDateChanges 사업의 마감일 변경 이력(최근 순).
func (r *ProjectRepo) ListDateChanges(projectID string, limit int) ([]DateChange, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.db.Query(`
		SELECT c.change_id, c.project_id, c.level, c.target_id, c.target_name, c.field, c.kind,
		       c.old_value, c.new_value, c.reason, c.changed_by, COALESCE(NULLIF(TRIM(u.full_name),''), u.username, c.changed_by, ''), c.changed_at
		FROM structure_date_changes c
		LEFT JOIN users u ON u.user_id=c.changed_by
		WHERE c.project_id=?
		ORDER BY c.changed_at DESC, c.change_id DESC
		LIMIT ?`, projectID, limit)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []DateChange
	for rows.Next() {
		var d DateChange
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Level, &d.TargetID, &d.TargetName, &d.Field, &d.Kind,
			&d.OldValue, &d.NewValue, &d.Reason, &d.ChangedBy, &d.ChangedByName, &d.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PackageMeta 하부 업무의 일자·배지.
type PackageMeta struct {
	PlanStart, PlanEnd, ActualStart, Badge string
}

// PackageMetaOf 하부 업무의 현재 일자·배지(사업 범위 확인 포함). 없으면 sql.ErrNoRows.
func (r *ProjectRepo) PackageMetaOf(projectID, packageID string) (PackageMeta, string, error) {
	var m PackageMeta
	var name string
	err := r.db.QueryRow(`
		SELECT pk.name, pk.plan_start, pk.plan_end, pk.actual_start, pk.badge
		FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pk.package_id=? AND ph.project_id=? AND pk.is_active=1`,
		strings.TrimSpace(packageID), strings.TrimSpace(projectID)).Scan(&name, &m.PlanStart, &m.PlanEnd, &m.ActualStart, &m.Badge)
	return m, name, err
}

// UpdatePackageMeta 하부 업무의 예정·마감·실제 시작 일자와 배지를 저장한다.
// 마감이 바뀌면 이력을 남기고, 연장이면 ext.Reason 이 필수다. 하부 업무 마감이 1단계 마감을 넘어도 막지 않는다(사용자 결정).
func (r *ProjectRepo) UpdatePackageMeta(projectID, packageID string, m PackageMeta, ext DateChangeCtx) error {
	badge, err := model.BadgeOK(m.Badge)
	if err != nil {
		return err
	}
	ps, pe, as, _, err := model.PhaseDatesOK(m.PlanStart, m.PlanEnd, m.ActualStart, "")
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var name, oldEnd, org string
	err = tx.QueryRow(`
		SELECT pk.name, pk.plan_end, ph.org_id
		FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pk.package_id=? AND ph.project_id=? AND pk.is_active=1`,
		strings.TrimSpace(packageID), strings.TrimSpace(projectID)).Scan(&name, &oldEnd, &org)
	if err == sql.ErrNoRows {
		return fmt.Errorf("하부 업무를 찾을 수 없습니다")
	}
	if err != nil {
		return err
	}
	if err := recordEndChangeTx(tx, &ext, projectID, org, "package", packageID, name, oldEnd, pe); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE phase_packages SET plan_start=?, plan_end=?, actual_start=?, badge=?, updated_at=?
		WHERE package_id=?`, ps, pe, as, badge, model.LocalNowString(), strings.TrimSpace(packageID)); err != nil {
		return err
	}
	return tx.Commit()
}

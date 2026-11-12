package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"

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
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := updatePackageMetaTx(tx, projectID, packageID, m, ext); err != nil {
		return err
	}
	return tx.Commit()
}

// updatePackageMetaTx UpdatePackageMeta 의 본체. 호출한 쪽이 트랜잭션을 연다(「변경 내용 저장」 한 번에 묶기). 95-183.
func updatePackageMetaTx(tx *sql.Tx, projectID, packageID string, m PackageMeta, ext DateChangeCtx) error {
	badge, err := model.BadgeOK(m.Badge)
	if err != nil {
		return err
	}
	ps, pe, as, _, err := model.PhaseDatesOK(m.PlanStart, m.PlanEnd, m.ActualStart, "")
	if err != nil {
		return err
	}
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
	_, err = tx.Exec(`
		UPDATE phase_packages SET plan_start=?, plan_end=?, actual_start=?, badge=?, updated_at=?
		WHERE package_id=?`, ps, pe, as, badge, model.LocalNowString(), strings.TrimSpace(packageID))
	return err
}

// autoPackageStarts 하부 업무별 자동 실제 시작일 = 그 하부 업무에 연결된 작업이 처음 완료된 날(작업 완료일 기준).
// 수동 입력값(phase_packages.actual_start)이 있으면 호출한 쪽이 그 값을 우선한다. 사업 한 번에 묶어서 조회한다. 95-179C.
func (r *ProjectRepo) autoPackageStarts(projectID string) (map[string]string, error) {
	out := map[string]string{}
	rows, err := r.db.Query(`
		SELECT t.package_id, MIN(substr(t.complete_date,1,10))
		FROM work_tasks t
		JOIN phase_packages pk ON pk.package_id=t.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id=? AND t.status='complete' AND TRIM(COALESCE(t.complete_date,''))!=''
		GROUP BY t.package_id`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, d string
		if err := rows.Scan(&id, &d); err != nil {
			return out, err
		}
		out[id] = d
	}
	return out, rows.Err()
}

// PlanEdit 작업 계획(3단계)의 수정 항목. 95-180.
type PlanEdit struct {
	Name, PlanStart, PlanEnd, NextNote string
	// ChangeBasis 가 참일 때만 진행 기준(site|qty)·수량 단위를 바꾼다. 95-183.
	ChangeBasis     bool
	Basis, QtyUnit string
}

// UpdatePlan 작업 계획의 이름·예정·마감·다음 할 일을 고친다. 완료된 계획도 고칠 수 있다(사용자 결정, 이력으로 남김).
// 마감이 바뀌면 같은 트랜잭션에서 이력을 남기고, 연장이면 ext.Reason 이 필수다. 사이트 구성·진행 기준은 건드리지 않는다.
func (r *ProjectRepo) UpdatePlan(projectID, planID string, e PlanEdit, ext DateChangeCtx) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := updatePlanTx(tx, projectID, planID, e, ext); err != nil {
		return err
	}
	return tx.Commit()
}

// updatePlanTx UpdatePlan 의 본체. 호출한 쪽이 트랜잭션을 연다. 95-183.
func updatePlanTx(tx *sql.Tx, projectID, planID string, e PlanEdit, ext DateChangeCtx) error {
	name, err := model.PlanNameOK(e.Name)
	if err != nil {
		return err
	}
	ps, pe, _, _, err := model.PhaseDatesOK(e.PlanStart, e.PlanEnd, "", "")
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(e.NextNote) > 300 {
		return fmt.Errorf("다음 할 일은 300자까지입니다")
	}
	pkgID, _, err := planOwnerTx(tx, strings.TrimSpace(planID), strings.TrimSpace(projectID))
	if err != nil {
		return err
	}
	var oldEnd, oldName, org, oldBasis, oldUnit string
	if err := tx.QueryRow(`
		SELECT pl.plan_end, pl.name, ph.org_id, pl.progress_basis, pl.qty_unit
		FROM package_plans pl JOIN phase_packages pk ON pk.package_id=pl.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pl.plan_id=?`, planID).Scan(&oldEnd, &oldName, &org, &oldBasis, &oldUnit); err != nil {
		return err
	}
	if name != oldName {
		var dup int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM package_plans WHERE package_id=? AND name=? AND plan_id<>?`, pkgID, name, planID).Scan(&dup); err != nil {
			return err
		}
		if dup > 0 {
			return fmt.Errorf("같은 이름의 계획이 있습니다")
		}
	}
	basis, unit := oldBasis, oldUnit
	if e.ChangeBasis {
		b := strings.TrimSpace(e.Basis)
		if b != model.BasisSite && b != model.BasisQty {
			return fmt.Errorf("진행 기준을 확인하세요")
		}
		unit = strings.TrimSpace(e.QtyUnit)
		if utf8.RuneCountInString(unit) > 20 {
			return fmt.Errorf("수량 단위는 20자까지입니다")
		}
		if b == model.BasisQty {
			var miss int
			if err := tx.QueryRow(`
				SELECT COUNT(*) FROM package_sites
				WHERE plan_id=? AND status<>'excluded' AND target_qty<=0`, planID).Scan(&miss); err != nil {
				return err
			}
			if miss > 0 {
				return fmt.Errorf("수량 기준으로 바꾸려면 목표 수량이 필요합니다. 미입력 %d곳", miss)
			}
		}
		basis = b
	}
	if err := recordEndChangeTx(tx, &ext, projectID, org, "plan", planID, name, oldEnd, pe); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE package_plans SET name=?, plan_start=?, plan_end=?, next_note=?, progress_basis=?, qty_unit=?, updated_at=? WHERE plan_id=?`,
		name, ps, pe, strings.TrimSpace(e.NextNote), basis, unit, model.LocalNowString(), planID)
	return err
}

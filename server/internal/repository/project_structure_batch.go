package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// 95-183 C-2 「변경 내용 저장」: 수행 구조 화면에서 고친 1단계·하부 업무·작업 계획을 한 번의 트랜잭션으로 저장한다.
// 하나라도 검증에 걸리면 전부 취소한다. 검증은 개별 저장과 같은 함수(…Tx)를 쓴다.

const (
	BatchPhase   = "phase"
	BatchPackage = "package"
	BatchPlan    = "plan"
	batchMax     = 200
)

// BatchItem 고친 항목 하나. F 에는 폼 필드 이름 그대로 값이 들어 있다. 없는 필드는 현재 값을 유지한다.
type BatchItem struct {
	Kind string
	ID   string
	F    map[string]string
}

// BatchResult 저장된 항목(감사 로그용).
type BatchResult struct {
	Kind, ID, Label, Reason string
}

// BatchError 몇 번째 항목이 왜 실패했는지.
type BatchError struct {
	Index    int
	Kind, ID string
	Msg      string
}

func (e *BatchError) Error() string { return e.Msg }

func pick(f map[string]string, key, cur string) string {
	if v, ok := f[key]; ok {
		return v
	}
	return cur
}

// ApplyStructureBatch 항목을 순서대로 적용한다. 실패하면 *BatchError 를 돌려주고 아무것도 저장하지 않는다.
func (r *ProjectRepo) ApplyStructureBatch(projectID string, items []BatchItem, actor string) ([]BatchResult, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("저장할 변경이 없습니다")
	}
	if len(items) > batchMax {
		return nil, fmt.Errorf("한 번에 %d곳까지 저장할 수 있습니다", batchMax)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var out []BatchResult
	for i, it := range items {
		res, err := applyBatchItemTx(tx, projectID, it, actor)
		if err != nil {
			return nil, &BatchError{Index: i, Kind: it.Kind, ID: it.ID, Msg: err.Error()}
		}
		out = append(out, res)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func applyBatchItemTx(tx *sql.Tx, projectID string, it BatchItem, actor string) (BatchResult, error) {
	id := strings.TrimSpace(it.ID)
	f := it.F
	ext := DateChangeCtx{Actor: actor, Reason: f["extend_reason"]}
	switch it.Kind {
	case BatchPhase:
		cur, err := getPhaseTx(tx, id)
		if err != nil || cur.ProjectID != projectID {
			return BatchResult{}, fmt.Errorf("단계를 찾을 수 없습니다")
		}
		cur.Name = pick(f, "name", cur.Name)
		cur.PlanStart = pick(f, "plan_start", cur.PlanStart)
		cur.PlanEnd = pick(f, "plan_end", cur.PlanEnd)
		cur.ActualStart = pick(f, "actual_start", cur.ActualStart)
		cur.ActualEnd = pick(f, "actual_end", cur.ActualEnd)
		cur.Note = pick(f, "note", cur.Note)
		if st := strings.TrimSpace(f["status"]); st != "" {
			cur.Status = st
		}
		if v, ok := f["is_active"]; ok {
			cur.IsActive = v != "0"
		}
		if err := updatePhaseTx(tx, &cur, &ext); err != nil {
			return BatchResult{}, err
		}
		return BatchResult{Kind: it.Kind, ID: id, Label: cur.Name, Reason: "단계 수정"}, nil
	case BatchPackage:
		var e PackageEdit
		err := tx.QueryRow(`
			SELECT pk.name, pk.track_mode, COALESCE(pk.next_note,''), pk.plan_start, pk.plan_end, pk.actual_start, pk.badge
			FROM phase_packages pk JOIN project_phases ph ON ph.phase_id=pk.phase_id
			WHERE pk.package_id=? AND ph.project_id=?`, id, projectID).
			Scan(&e.Name, &e.Track, &e.NextNote, &e.PlanStart, &e.PlanEnd, &e.ActualStart, &e.Badge)
		if err != nil {
			return BatchResult{}, fmt.Errorf("하부 업무를 찾을 수 없습니다")
		}
		e.Name = pick(f, "name", e.Name)
		e.Track = pick(f, "track_mode", e.Track)
		e.NextNote = pick(f, "next_note", e.NextNote)
		e.PlanStart = pick(f, "plan_start", e.PlanStart)
		e.PlanEnd = pick(f, "plan_end", e.PlanEnd)
		e.ActualStart = pick(f, "actual_start", e.ActualStart)
		e.Badge = pick(f, "badge", e.Badge)
		if err := updatePackageTx(tx, projectID, id, e, ext); err != nil {
			return BatchResult{}, err
		}
		return BatchResult{Kind: it.Kind, ID: id, Label: strings.TrimSpace(e.Name), Reason: "하부 업무 수정"}, nil
	case BatchPlan:
		var e PlanEdit
		var basis, unit string
		err := tx.QueryRow(`
			SELECT pl.name, pl.plan_start, pl.plan_end, COALESCE(pl.next_note,''), pl.progress_basis, pl.qty_unit
			FROM package_plans pl JOIN phase_packages pk ON pk.package_id=pl.package_id
			JOIN project_phases ph ON ph.phase_id=pk.phase_id
			WHERE pl.plan_id=? AND ph.project_id=?`, id, projectID).
			Scan(&e.Name, &e.PlanStart, &e.PlanEnd, &e.NextNote, &basis, &unit)
		if err != nil {
			return BatchResult{}, fmt.Errorf("계획을 찾을 수 없습니다")
		}
		e.Name = pick(f, "name", e.Name)
		e.PlanStart = pick(f, "plan_start", e.PlanStart)
		e.PlanEnd = pick(f, "plan_end", e.PlanEnd)
		e.NextNote = pick(f, "next_note", e.NextNote)
		nb, nu := pick(f, "progress_basis", basis), pick(f, "qty_unit", unit)
		if nb != basis || strings.TrimSpace(nu) != unit {
			e.ChangeBasis, e.Basis, e.QtyUnit = true, nb, nu
		}
		if err := updatePlanTx(tx, projectID, id, e, ext); err != nil {
			return BatchResult{}, err
		}
		return BatchResult{Kind: it.Kind, ID: id, Label: strings.TrimSpace(e.Name), Reason: "작업 계획 수정"}, nil
	}
	return BatchResult{}, fmt.Errorf("알 수 없는 항목입니다")
}

// BatchKindAllowed 항목 종류가 맞는 값인지.
func BatchKindAllowed(kind string) bool {
	return kind == BatchPhase || kind == BatchPackage || kind == BatchPlan
}

var _ = model.BasisSite

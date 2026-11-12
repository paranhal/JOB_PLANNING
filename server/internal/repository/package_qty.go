package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"customer-support/internal/model"
)

// 수량만(track_mode=phase) 하부 업무의 진행. 95-176 질문 1 추천안.
//
// 사이트·작업 계획 없이 「몇 개 중 몇 개」만 추적하는 하부 업무다.
//   - 목표: phase_packages.qty_total / qty_unit (95-120P 부터 있던 컬럼. 지금까지 읽는 곳이 없었다)
//   - 누적: package_events 의 event_type='qty' 합계(qty_delta). 사이트 이벤트와 같은 표·같은 ID 시퀀스를 쓰고
//     package_site_id·plan_id 는 비운다. 사이트 스냅샷 재계산은 이 이벤트를 건너뛴다.
//
// 표시는 model.SiteCheckRateLabel(분모 0 이면 「—」)을 쓴다. 사업·단계 진행률(사이트 기준)에는 넣지 않는다.

const (
	packageQtyEvent = "qty"
	packageQtyMax   = 99999999
)

// PackageQtyState 수량만 하부 업무의 목표와 누적.
type PackageQtyState struct {
	Unit        string
	Total, Done int
}

// packageQtyDone 사업의 수량만 하부 업무별 누적 수량.
func (r *ProjectRepo) packageQtyDone(projectID string) (map[string]int, error) {
	return r.packageQtyDoneIn([]string{projectID})
}

// packageQtyDoneIn 여러 사업의 수량만 하부 업무별 누적 수량(한 번에 묶어서 조회, N+1 없음).
func (r *ProjectRepo) packageQtyDoneIn(projectIDs []string) (map[string]int, error) {
	out := map[string]int{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	ph, args := inArgs(projectIDs)
	rows, err := r.db.Query(`
		SELECT e.package_id, COALESCE(SUM(e.qty_delta),0)
		FROM package_events e
		JOIN phase_packages pk ON pk.package_id=e.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id IN (`+ph+`) AND e.event_type=? AND e.package_site_id='' AND e.plan_id=''
		GROUP BY e.package_id`, append(args, packageQtyEvent)...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return out, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// qtyPackageTx 그 사업의 수량만 하부 업무인지 확인하고 현재 상태를 돌려준다. 사업 접근 권한은 핸들러(planContext)가 확인한다.
func qtyPackageTx(tx *sql.Tx, projectID, packageID string) (PackageQtyState, error) {
	var st PackageQtyState
	var track string
	err := tx.QueryRow(`
		SELECT pk.track_mode, COALESCE(pk.qty_unit,''), COALESCE(pk.qty_total,0)
		FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE pk.package_id=? AND pk.is_active=1 AND ph.project_id=?`,
		strings.TrimSpace(packageID), strings.TrimSpace(projectID)).Scan(&track, &st.Unit, &st.Total)
	if err == sql.ErrNoRows {
		return st, fmt.Errorf("하부 업무를 찾을 수 없습니다")
	}
	if err != nil {
		return st, err
	}
	if track != model.TrackModePhase {
		return st, fmt.Errorf("수량만 하부 업무에서만 수량을 입력합니다. 사이트 하부 업무는 작업 계획으로 진행을 셉니다")
	}
	if err := tx.QueryRow(`
		SELECT COALESCE(SUM(qty_delta),0) FROM package_events
		WHERE package_id=? AND event_type=? AND package_site_id='' AND plan_id=''`,
		strings.TrimSpace(packageID), packageQtyEvent).Scan(&st.Done); err != nil {
		return st, err
	}
	return st, nil
}

// SetPackageQtyTarget 수량만 하부 업무의 목표 수량·단위. 이미 입력된 누적보다 작게는 못 줄인다.
func (r *ProjectRepo) SetPackageQtyTarget(projectID, packageID, unit string, total int) error {
	unit = strings.TrimSpace(unit)
	if utf8.RuneCountInString(unit) > 20 {
		return fmt.Errorf("단위는 20자까지입니다")
	}
	if total < 1 || total > packageQtyMax {
		return fmt.Errorf("목표 수량은 1 이상 %s 이하로 입력하세요", model.FormatIntComma(packageQtyMax))
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := qtyPackageTx(tx, projectID, packageID)
	if err != nil {
		return err
	}
	if total < st.Done {
		return fmt.Errorf("이미 입력된 수량(%s)보다 작게 줄일 수 없습니다", model.FormatIntComma(st.Done))
	}
	now := model.LocalNowString()
	if _, err := tx.Exec(`
		UPDATE phase_packages SET qty_total=?, qty_unit=?, updated_at=?
		WHERE package_id=? AND phase_id IN (SELECT phase_id FROM project_phases WHERE project_id=?)`,
		total, unit, now, strings.TrimSpace(packageID), strings.TrimSpace(projectID)); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordPackageQty 처리 수량을 더한다(보정은 음수). 누적은 0 이상 목표 이하여야 하고, 목표를 먼저 정해야 한다.
// 돌려주는 값은 입력 뒤 누적 수량.
func (r *ProjectRepo) RecordPackageQty(projectID, packageID, date string, delta int, userID, note string) (int, error) {
	if delta == 0 {
		return 0, fmt.Errorf("처리 수량을 입력하세요(0은 저장하지 않습니다)")
	}
	date = strings.TrimSpace(date)
	if date == "" {
		date = time.Now().In(model.KST()).Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return 0, fmt.Errorf("날짜는 YYYY-MM-DD 형식입니다")
	}
	if err := model.RequireAppDateYear(date); err != nil {
		return 0, err
	}
	if utf8.RuneCountInString(note) > 200 {
		return 0, fmt.Errorf("메모는 200자까지입니다")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	st, err := qtyPackageTx(tx, projectID, packageID)
	if err != nil {
		return 0, err
	}
	if st.Total <= 0 {
		return 0, fmt.Errorf("목표 수량을 먼저 정하세요")
	}
	next := st.Done + delta
	if next < 0 {
		return 0, fmt.Errorf("누적 수량이 0보다 작아집니다(현재 %s)", model.FormatIntComma(st.Done))
	}
	if next > st.Total {
		return 0, fmt.Errorf("목표 수량(%s)을 넘습니다. 목표를 먼저 늘리세요", model.FormatIntComma(st.Total))
	}
	seq, err := nextSeqTx(tx, "package_event")
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`
		INSERT INTO package_events(
			event_id, package_id, package_site_id, plan_id, event_date, event_type,
			qty_delta, user_id, work_task_id, source, note, created_at)
		VALUES (?,?,'','',?,?,?,?,'','manual',?,?)`,
		fmt.Sprintf("EV-%03d", seq), strings.TrimSpace(packageID), date, packageQtyEvent, delta,
		strings.TrimSpace(userID), strings.TrimSpace(note), model.LocalNowString()); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return next, nil
}

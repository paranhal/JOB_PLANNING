package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

// 95-200: 하위(실행) 업무에서 「업무 모두 종료」 — 이 회차 완료 + 이후 일정 제외 + 상위 업무 자동 완료.

// sqlOccurrenceOpen 「열린 회차」 판정. CountOpenOccurrences 와 이 기능이 같은 조건을 쓴다.
// (status≠complete 이면서 occurrence_status ∉ {complete, skipped}). work_tasks 를 별칭 없이 쓴다.
const sqlOccurrenceOpen = `COALESCE(status,'') != 'complete' AND COALESCE(occurrence_status,'') NOT IN ('complete','skipped')`

var (
	ErrOccCloseAlready  = errors.New("이미 종료된 업무입니다")
	ErrOccCloseReason   = errors.New("종료 사유를 입력하세요")
	ErrOccCloseResult   = errors.New("최종 결과를 입력해야 완료됩니다")
	ErrOccCloseRule     = errors.New("종료할 수 없는 업무입니다")
	ErrOccCloseNotFound = errors.New("업무를 찾을 수 없습니다")
)

// CloseOccResult 일괄 종료 결과.
type CloseOccResult struct {
	Completed       int  // 이 회차를 새로 완료했으면 1
	Skipped         int  // 제외한 회차 수(이후 + 함께 제외한 이전)
	EarlierKept     int  // 이전 미완료로 남긴 수
	ParentCompleted bool // 상위 업무를 완료했는지
}

// CloseOccPreview 모달에 보여 줄 건수.
type CloseOccPreview struct {
	Later           int            `json:"later"`
	LaterByStatus   map[string]int `json:"laterByStatus"`
	Earlier         int            `json:"earlier"`
	NeedFinalResult bool           `json:"needFinalResult"`
	ParentTitle     string         `json:"parentTitle"`
	OccTitle        string         `json:"occTitle"`
	OccDate         string         `json:"occDate"`
	OccDone         bool           `json:"occDone"`
}

type closeOccRow struct {
	ID, Status, OccStatus, Date string
}

func closeOccDate(workDate, dueDate string) string {
	d := strings.TrimSpace(workDate)
	if d == "" {
		d = strings.TrimSpace(dueDate)
	}
	return d
}

type closeQuerier interface {
	QueryRow(query string, args ...interface{}) *sql.Row
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// loadCloseTargets 부모·기준 회차를 확인하고 부모의 다른 열린 회차를 이후/이전으로 가른다.
// 이후 = 날짜가 기준 회차보다 늦거나 날짜가 없는 것, 이전 = 날짜가 기준 회차와 같거나 앞선 것.
func loadCloseTargets(q closeQuerier, parentID, occID string) (parentTitle, parentStatus, parentPolicy, parentResult string, archived bool,
	occTitle, occDate string, occDone bool, later, earlier []closeOccRow, err error) {
	var pRole string
	if e := q.QueryRow(`SELECT COALESCE(recurrence_role,''), COALESCE(status,''), COALESCE(title,'') FROM work_tasks WHERE task_id=?`, parentID).
		Scan(&pRole, &parentStatus, &parentTitle); e != nil {
		if e == sql.ErrNoRows {
			err = ErrOccCloseNotFound
			return
		}
		err = e
		return
	}
	if pRole != model.RecurrenceRoleParent {
		err = ErrOccCloseNotFound
		return
	}
	var oParent, oRole, oStatus, oOccStatus, oWork, oDue string
	if e := q.QueryRow(`SELECT COALESCE(parent_task_id,''), COALESCE(recurrence_role,''), COALESCE(status,''), COALESCE(occurrence_status,''),
		COALESCE(work_date,''), COALESCE(due_date,''), COALESCE(title,'') FROM work_tasks WHERE task_id=?`, occID).
		Scan(&oParent, &oRole, &oStatus, &oOccStatus, &oWork, &oDue, &occTitle); e != nil {
		if e == sql.ErrNoRows {
			err = ErrOccCloseNotFound
			return
		}
		err = e
		return
	}
	if oParent != parentID || oRole != model.RecurrenceRoleOccurrence {
		err = ErrOccCloseNotFound
		return
	}
	occDate = closeOccDate(oWork, oDue)
	occDone = oStatus == model.WBTaskComplete || oOccStatus == model.OccurrenceComplete
	var arch int
	var policy, result string
	if e := q.QueryRow(`SELECT COALESCE(complete_policy,'manual'), COALESCE(final_result,''), COALESCE(archived,0) FROM work_recurrence WHERE task_id=?`, parentID).
		Scan(&policy, &result, &arch); e != nil && e != sql.ErrNoRows {
		err = e
		return
	}
	parentPolicy, parentResult, archived = policy, result, arch != 0
	rows, e := q.Query(`SELECT task_id, COALESCE(status,''), COALESCE(occurrence_status,''), COALESCE(work_date,''), COALESCE(due_date,'')
		FROM work_tasks WHERE parent_task_id=? AND COALESCE(recurrence_role,'')=? AND task_id != ? AND `+sqlOccurrenceOpen+`
		ORDER BY COALESCE(NULLIF(TRIM(work_date),''), due_date), task_id`, parentID, model.RecurrenceRoleOccurrence, occID)
	if e != nil {
		err = e
		return
	}
	defer rows.Close()
	for rows.Next() {
		var row closeOccRow
		var w, d string
		if e := rows.Scan(&row.ID, &row.Status, &row.OccStatus, &w, &d); e != nil {
			err = e
			return
		}
		row.Date = closeOccDate(w, d)
		if row.Date == "" || row.Date > occDate {
			later = append(later, row)
		} else {
			earlier = append(earlier, row)
		}
	}
	err = rows.Err()
	return
}

// PreviewCloseOccurrences 모달 요약 건수. DB 를 바꾸지 않는다.
func (r *WBRepo) PreviewCloseOccurrences(parentID, occID string) (CloseOccPreview, error) {
	out := CloseOccPreview{LaterByStatus: map[string]int{}}
	title, pStatus, policy, result, archived, occTitle, occDate, occDone, later, earlier, err := loadCloseTargets(r.db, strings.TrimSpace(parentID), strings.TrimSpace(occID))
	if err != nil {
		return out, err
	}
	if pStatus == model.WBTaskComplete || pStatus == model.WBTaskCancelled || archived {
		return out, ErrOccCloseAlready
	}
	for _, l := range later {
		out.LaterByStatus[model.NormalizeOccurrenceStatus(l.OccStatus)]++
	}
	out.Later = len(later)
	out.Earlier = len(earlier)
	out.ParentTitle, out.OccTitle, out.OccDate, out.OccDone = title, occTitle, occDate, occDone
	out.NeedFinalResult = model.NormalizeCompletePolicy(policy) == model.CompletePolicyRequireResult && strings.TrimSpace(result) == ""
	return out, nil
}

func earlyCloseNote(reason, actor string, now time.Time) string {
	note := "업무 조기 종료: " + strings.TrimSpace(reason)
	if a := strings.TrimSpace(actor); a != "" {
		note += " · " + a + " " + now.Format("2006-01-02")
	} else {
		note += " · " + now.Format("2006-01-02")
	}
	return note
}

// CloseOccurrencesEarly 한 트랜잭션으로 처리한다(중간 실패 시 전체 롤백).
//  1. 기준 회차 완료(이미 완료면 그대로)
//  2. 이후 열린 회차 → occurrence_status=skipped + status=cancelled(주간보고·목록에서 미완료로 남지 않게)
//  3. includeEarlier 이면 이전 열린 회차도 같은 방식으로 제외, 아니면 열린 채로 둔다
//  4. 남은 열린 회차가 0건이면 상위 업무를 완료(완료 정책과 무관). require_result 인데 최종 결과가 비면 finalResult 를 저장하고 완료
func (r *WBRepo) CloseOccurrencesEarly(parentID, occID, reason, finalResult string, includeEarlier bool, actor string) (CloseOccResult, error) {
	var res CloseOccResult
	parentID, occID = strings.TrimSpace(parentID), strings.TrimSpace(occID)
	reason = strings.TrimSpace(reason)
	finalResult = strings.TrimSpace(finalResult)
	if reason == "" {
		return res, ErrOccCloseReason
	}
	now := time.Now()
	note := earlyCloseNote(reason, actor, now)

	tx, err := r.db.Begin()
	if err != nil {
		return res, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	// 먼저 쓰기 잠금을 잡아 같은 요청이 겹쳐 와도 차례로 처리한다(두 번째는 이미 종료된 상태를 본다).
	if _, err := tx.Exec(`UPDATE work_tasks SET task_id=task_id WHERE task_id=?`, parentID); err != nil {
		return res, err
	}
	_, pStatus, policy, result, archived, _, _, occDone, later, earlier, err := loadCloseTargets(tx, parentID, occID)
	if err != nil {
		return res, err
	}
	if pStatus == model.WBTaskComplete || pStatus == model.WBTaskCancelled || archived {
		return res, ErrOccCloseAlready
	}
	// 이전 미완료를 남기면 상위 업무는 완료하지 않으므로 최종 결과는 필요 없다.
	willCompleteParent := includeEarlier || len(earlier) == 0
	needResult := model.NormalizeCompletePolicy(policy) == model.CompletePolicyRequireResult && strings.TrimSpace(result) == ""
	if needResult && willCompleteParent && finalResult == "" {
		return res, ErrOccCloseResult
	}

	// 1. 기준 회차 완료
	if !occDone {
		if _, err := tx.Exec(`UPDATE work_tasks SET status='complete', progress=100, occurrence_status='complete',
			complete_date=CASE WHEN TRIM(COALESCE(complete_date,''))!='' THEN complete_date ELSE date('now','localtime') END,
			updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, occID); err != nil {
			return res, err
		}
		res.Completed = 1
	}
	// 2·3. 제외
	skip := func(rows []closeOccRow) error {
		for _, o := range rows {
			if _, err := tx.Exec(`UPDATE work_tasks SET occurrence_status='skipped', status='cancelled', not_done_reason=?,
				updated_at=CURRENT_TIMESTAMP WHERE task_id=? AND COALESCE(recurrence_role,'')=? AND `+sqlOccurrenceOpen,
				note, o.ID, model.RecurrenceRoleOccurrence); err != nil {
				return err
			}
			res.Skipped++
		}
		return nil
	}
	if err := skip(later); err != nil {
		return res, err
	}
	if includeEarlier {
		if err := skip(earlier); err != nil {
			return res, err
		}
	} else {
		res.EarlierKept = len(earlier)
	}
	// 4. 상위 업무 완료
	var open int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE parent_task_id=? AND COALESCE(recurrence_role,'')=? AND `+sqlOccurrenceOpen,
		parentID, model.RecurrenceRoleOccurrence).Scan(&open); err != nil {
		return res, err
	}
	if open == 0 {
		if needResult && finalResult != "" {
			if _, err := tx.Exec(`UPDATE work_recurrence SET final_result=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, finalResult, parentID); err != nil {
				return res, err
			}
		}
		if _, err := tx.Exec(`UPDATE work_tasks SET status='complete', progress=100,
			complete_note=CASE WHEN TRIM(COALESCE(complete_note,''))='' THEN ? ELSE complete_note || char(10) || ? END,
			complete_date=CASE WHEN TRIM(COALESCE(complete_date,''))='' THEN date('now','localtime') ELSE complete_date END,
			updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, note, note, parentID); err != nil {
			return res, err
		}
		res.ParentCompleted = true
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	committed = true

	// 변경 이력(감사 로그)은 커밋 뒤에 남긴다.
	label := fmt.Sprintf("업무 모두 종료: %s (이 회차 완료, 제외 %d건%s)", reason, res.Skipped, map[bool]string{true: ", 상위 업무 완료", false: ""}[res.ParentCompleted])
	logUpdate(r.db, "work_tasks", "task_id", parentID, label, "")
	logUpdate(r.db, "work_tasks", "task_id", occID, label, "")
	return res, nil
}

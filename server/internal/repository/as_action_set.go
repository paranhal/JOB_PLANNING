package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"customer-support/internal/model"
)

func applyASActionSetsV99(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := asActionSetsMigrationSQL()
	if err != nil {
		log.Printf("099 as_action_sets: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("099 as_action_sets apply: %v", err)
	}
}

func asActionSetsMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("099_as_action_sets.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("099_as_action_sets.sql 없음")
}

func asActionSetsDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("099_as_action_sets.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("099_as_action_sets.down.sql 없음")
}

type ASActionSetRepo struct{ db *sql.DB }

func NewASActionSetRepo(db *sql.DB) *ASActionSetRepo { return &ASActionSetRepo{db: db} }

const asActionSetSelect = `
	SELECT set_id, as_id, seq, COALESCE(state,''), COALESCE(assigned_to,''), COALESCE(assigned_user_id,''),
	       COALESCE(work_place,''), COALESCE(process_type,''), COALESCE(process_type_reason,''), COALESCE(visit_date,''),
	       COALESCE(cause_cat1,''), COALESCE(cause_cat1_custom,''), COALESCE(cause_cat2,''), COALESCE(cause_cat3,''),
	       COALESCE(cause_detail,''), COALESCE(complete_datetime,''), COALESCE(work_date,''), COALESCE(start_time,''),
	       COALESCE(duration_min,0), COALESCE(end_time,''), COALESCE(leave_same,1), COALESCE(leave_time,''),
	       COALESCE(action_taken,''), COALESCE(action_na,0), COALESCE(action_na_reason,''), COALESCE(result_code,''),
	       COALESCE(result_extra,''), COALESCE(created_by,''), COALESCE(created_at,''), COALESCE(updated_at,'')
	  FROM as_action_sets`

func scanActionSet(scanner interface {
	Scan(dest ...any) error
}) (*model.ASActionSet, error) {
	var s model.ASActionSet
	var leaveSame, actionNA int
	if err := scanner.Scan(&s.SetID, &s.ASID, &s.Seq, &s.State, &s.AssignedTo, &s.AssignedUserID,
		&s.WorkPlace, &s.ProcessType, &s.ProcessTypeReason, &s.VisitDate,
		&s.CauseCat1, &s.CauseCat1Custom, &s.CauseCat2, &s.CauseCat3,
		&s.CauseDetail, &s.CompleteDatetime, &s.WorkDate, &s.StartTime,
		&s.DurationMin, &s.EndTime, &leaveSame, &s.LeaveTime,
		&s.ActionTaken, &actionNA, &s.ActionNAReason, &s.ResultCode,
		&s.ResultExtra, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.LeaveSame = leaveSame != 0
	s.ActionNA = actionNA != 0
	return &s, nil
}

func (r *ASActionSetRepo) ListByAS(asID string) ([]model.ASActionSet, error) {
	if r == nil || r.db == nil || strings.TrimSpace(asID) == "" {
		return nil, nil
	}
	rows, err := r.db.Query(asActionSetSelect+` WHERE as_id=? ORDER BY seq`, asID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.ASActionSet
	for rows.Next() {
		s, err := scanActionSet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r *ASActionSetRepo) Get(setID string) (*model.ASActionSet, error) {
	s, err := scanActionSet(r.db.QueryRow(asActionSetSelect+` WHERE set_id=?`, setID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (r *ASActionSetRepo) Upsert(s *model.ASActionSet) error {
	if r == nil || s == nil || strings.TrimSpace(s.ASID) == "" {
		return fmt.Errorf("세트 저장 대상이 없습니다")
	}
	if s.Seq < 1 {
		s.Seq = 1
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	s.UpdatedAt = now
	model.FillActionSetComputedTimes(s)
	leaveSame, actionNA := 0, 0
	if s.LeaveSame {
		leaveSame = 1
	}
	if s.ActionNA {
		actionNA = 1
	}
	if strings.TrimSpace(s.SetID) == "" {
		var existing string
		_ = r.db.QueryRow(`SELECT set_id FROM as_action_sets WHERE as_id=? AND seq=?`, s.ASID, s.Seq).Scan(&existing)
		if strings.TrimSpace(existing) != "" {
			s.SetID = existing
		}
	}
	if strings.TrimSpace(s.SetID) == "" {
		n, err := NextSeq(r.db, "as_action_set")
		if err != nil {
			return err
		}
		s.SetID = fmt.Sprintf("ASET%05d", n)
		s.CreatedAt = now
		_, err = r.db.Exec(`
			INSERT INTO as_action_sets(
				set_id, as_id, seq, state, assigned_to, assigned_user_id, work_place, process_type, process_type_reason,
				visit_date, cause_cat1, cause_cat1_custom, cause_cat2, cause_cat3, cause_detail, complete_datetime,
				work_date, start_time, duration_min, end_time, leave_same, leave_time, action_taken, action_na,
				action_na_reason, result_code, result_extra, created_by, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			s.SetID, s.ASID, s.Seq, firstNonEmptyState(s.State), s.AssignedTo, s.AssignedUserID,
			s.WorkPlace, s.ProcessType, s.ProcessTypeReason, s.VisitDate, s.CauseCat1, s.CauseCat1Custom, s.CauseCat2, s.CauseCat3,
			s.CauseDetail, s.CompleteDatetime, s.WorkDate, s.StartTime, s.DurationMin, s.EndTime, leaveSame, s.LeaveTime,
			s.ActionTaken, actionNA, s.ActionNAReason, s.ResultCode, s.ResultExtra, s.CreatedBy, s.CreatedAt, s.UpdatedAt)
		return err
	}
	_, err := r.db.Exec(`
		UPDATE as_action_sets SET seq=?, state=?, assigned_to=?, assigned_user_id=?, work_place=?, process_type=?,
			process_type_reason=?, visit_date=?, cause_cat1=?, cause_cat1_custom=?, cause_cat2=?, cause_cat3=?,
			cause_detail=?, complete_datetime=?, work_date=?, start_time=?, duration_min=?, end_time=?, leave_same=?,
			leave_time=?, action_taken=?, action_na=?, action_na_reason=?, result_code=?, result_extra=?, updated_at=?
		 WHERE set_id=?`,
		s.Seq, s.State, s.AssignedTo, s.AssignedUserID, s.WorkPlace, s.ProcessType, s.ProcessTypeReason, s.VisitDate,
		s.CauseCat1, s.CauseCat1Custom, s.CauseCat2, s.CauseCat3, s.CauseDetail, s.CompleteDatetime, s.WorkDate,
		s.StartTime, s.DurationMin, s.EndTime, leaveSame, s.LeaveTime, s.ActionTaken, actionNA, s.ActionNAReason,
		s.ResultCode, s.ResultExtra, s.UpdatedAt, s.SetID)
	return err
}

func (r *ASActionSetRepo) ReplaceFinal(asID string, sets []model.ASActionSet) error {
	if r == nil {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM as_action_sets WHERE as_id=?`, asID); err != nil {
		return err
	}
	for i := range sets {
		s := &sets[i]
		s.ASID = asID
		s.Seq = i + 1
		s.State = model.ActionSetStateFinal
		if err := r.upsertTx(tx, s); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *ASActionSetRepo) upsertTx(tx *sql.Tx, s *model.ASActionSet) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	s.UpdatedAt = now
	model.FillActionSetComputedTimes(s)
	leaveSame, actionNA := 0, 0
	if s.LeaveSame {
		leaveSame = 1
	}
	if s.ActionNA {
		actionNA = 1
	}
	if strings.TrimSpace(s.SetID) == "" {
		n, err := nextSeqTx(tx, "as_action_set")
		if err != nil {
			return err
		}
		s.SetID = fmt.Sprintf("ASET%05d", n)
		s.CreatedAt = now
	}
	if s.CreatedAt == "" {
		s.CreatedAt = now
	}
	_, err := tx.Exec(`
		INSERT INTO as_action_sets(
			set_id, as_id, seq, state, assigned_to, assigned_user_id, work_place, process_type, process_type_reason,
			visit_date, cause_cat1, cause_cat1_custom, cause_cat2, cause_cat3, cause_detail, complete_datetime,
			work_date, start_time, duration_min, end_time, leave_same, leave_time, action_taken, action_na,
			action_na_reason, result_code, result_extra, created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(set_id) DO UPDATE SET
			seq=excluded.seq, state=excluded.state, assigned_to=excluded.assigned_to, assigned_user_id=excluded.assigned_user_id,
			work_place=excluded.work_place, process_type=excluded.process_type, process_type_reason=excluded.process_type_reason,
			visit_date=excluded.visit_date, cause_cat1=excluded.cause_cat1, cause_cat1_custom=excluded.cause_cat1_custom,
			cause_cat2=excluded.cause_cat2, cause_cat3=excluded.cause_cat3, cause_detail=excluded.cause_detail,
			complete_datetime=excluded.complete_datetime, work_date=excluded.work_date, start_time=excluded.start_time,
			duration_min=excluded.duration_min, end_time=excluded.end_time, leave_same=excluded.leave_same,
			leave_time=excluded.leave_time, action_taken=excluded.action_taken, action_na=excluded.action_na,
			action_na_reason=excluded.action_na_reason, result_code=excluded.result_code, result_extra=excluded.result_extra,
			updated_at=excluded.updated_at`,
		s.SetID, s.ASID, s.Seq, s.State, s.AssignedTo, s.AssignedUserID, s.WorkPlace, s.ProcessType, s.ProcessTypeReason,
		s.VisitDate, s.CauseCat1, s.CauseCat1Custom, s.CauseCat2, s.CauseCat3, s.CauseDetail, s.CompleteDatetime,
		s.WorkDate, s.StartTime, s.DurationMin, s.EndTime, leaveSame, s.LeaveTime, s.ActionTaken, actionNA,
		s.ActionNAReason, s.ResultCode, s.ResultExtra, s.CreatedBy, s.CreatedAt, s.UpdatedAt)
	return err
}

func (r *ASActionSetRepo) DeleteByAS(asID string) error {
	_, err := r.db.Exec(`DELETE FROM as_action_sets WHERE as_id=?`, asID)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}

func firstNonEmptyState(s string) string {
	if strings.TrimSpace(s) == "" {
		return model.ActionSetStateSaved
	}
	return s
}

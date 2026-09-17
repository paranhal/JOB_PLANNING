package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type ASProcessRepo struct{ db *sql.DB }

func NewASProcessRepo(db *sql.DB) *ASProcessRepo { return &ASProcessRepo{db: db} }

const asProcessSelect = `
		SELECT process_id, COALESCE(process_number,''), as_id, COALESCE(process_datetime,''),
		       COALESCE(worker,''), COALESCE(work_type,''), COALESCE(cause_type,''),
		       COALESCE(work_content,''), COALESCE(parts_used,''),
		       COALESCE(time_spent,0), COALESCE(notes,''),
		       COALESCE(result_code,''), COALESCE(transfer_detail,''),
		       COALESCE(next_action_date,''), COALESCE(wait_reason,''), COALESCE(prep_notes,'')
		FROM as_processes`

func scanASProcesses(rows *sql.Rows) ([]model.ASProcess, error) {
	var items []model.ASProcess
	for rows.Next() {
		var p model.ASProcess
		var dt string
		if err := rows.Scan(&p.ProcessID, &p.ProcessNumber, &p.ASID, &dt,
			&p.Worker, &p.WorkType, &p.CauseType, &p.WorkContent,
			&p.PartsUsed, &p.TimeSpent, &p.Notes,
			&p.ResultCode, &p.TransferDetail, &p.NextActionDate, &p.WaitReason, &p.PrepNotes); err != nil {
			return nil, err
		}
		p.ProcessDatetime = parseTime(dt)
		items = append(items, p)
	}
	return items, rows.Err()
}

func (r *ASProcessRepo) ListByAS(asID string) ([]model.ASProcess, error) {
	rows, err := r.db.Query(asProcessSelect+`
		WHERE as_id=? ORDER BY process_datetime ASC, process_id ASC`, asID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanASProcesses(rows)
}

func (r *ASProcessRepo) Create(p *model.ASProcess) error {
	at := p.ProcessDatetime
	if at.IsZero() {
		at = time.Now()
	}
	p.ProcessDatetime = at

	asNumber := p.ASID
	var num string
	if err := r.db.QueryRow(`SELECT as_number FROM as_receipts WHERE as_id=?`, p.ASID).Scan(&num); err == nil && num != "" {
		asNumber = num
	}
	procNum, err := NextProcessNumber(r.db, asNumber, at)
	if err != nil {
		return err
	}
	p.ProcessNumber = procNum
	p.ProcessID = procNum
	worker, workerUID := bindStaff(r.db, p.Worker, "")
	p.Worker = worker
	p.TimeSpent = model.NormalizeDurationMin(p.TimeSpent)
	_, err = r.db.Exec(`
		INSERT INTO as_processes
		(process_id,process_number,as_id,process_datetime,worker,worker_user_id,work_type,cause_type,work_content,parts_used,time_spent,notes,
		 result_code,transfer_detail,next_action_date,wait_reason,prep_notes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ProcessID, p.ProcessNumber, p.ASID, at.Format("2006-01-02 15:04:05"),
		p.Worker, workerUID, p.WorkType, p.CauseType, p.WorkContent, p.PartsUsed, p.TimeSpent, p.Notes,
		nullIfEmpty(p.ResultCode), nullIfEmpty(p.TransferDetail), nullIfEmpty(p.NextActionDate),
		nullIfEmpty(p.WaitReason), nullIfEmpty(p.PrepNotes))
	if err != nil {
		return err
	}
	reindexASSearch(r.db, p.ASID)
	// §25.2 AS 조치 등록은 이력 미기록(○)
	return NewASRepo(r.db).SetStartDatetimeFromFirstProcess(p.ASID)
}

func (r *ASProcessRepo) Delete(id string) error {
	var asID string
	_ = r.db.QueryRow(`SELECT as_id FROM as_processes WHERE process_id=?`, id).Scan(&asID)
	err := touchDelete(r.db, "as_processes", "process_id", id, id, func() error {
		_, err := r.db.Exec(`DELETE FROM as_processes WHERE process_id=?`, id)
		return err
	})
	if err == nil {
		reindexASSearch(r.db, asID)
	}
	return err
}

// DeleteByASAndID 해당 접수에 속한 처리 이력만 삭제
func (r *ASProcessRepo) DeleteByASAndID(asID, processID string) error {
	err := touchDelete(r.db, "as_processes", "process_id", processID, processID, func() error {
		res, err := r.db.Exec(`DELETE FROM as_processes WHERE process_id=? AND as_id=?`, processID, asID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err == nil {
		reindexASSearch(r.db, asID)
	}
	return err
}

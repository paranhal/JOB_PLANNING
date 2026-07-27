package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type ASProcessRepo struct{ db *sql.DB }

func NewASProcessRepo(db *sql.DB) *ASProcessRepo { return &ASProcessRepo{db: db} }

func (r *ASProcessRepo) ListByAS(asID string) ([]model.ASProcess, error) {
	rows, err := r.db.Query(`
		SELECT process_id, COALESCE(process_number,''), as_id, COALESCE(process_datetime,''),
		       COALESCE(worker,''), COALESCE(work_type,''),
		       COALESCE(work_content,''), COALESCE(parts_used,''),
		       COALESCE(time_spent,0), COALESCE(notes,'')
		FROM as_processes WHERE as_id=? ORDER BY process_datetime DESC, process_id DESC`, asID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.ASProcess
	for rows.Next() {
		var p model.ASProcess
		var dt string
		if err := rows.Scan(&p.ProcessID, &p.ProcessNumber, &p.ASID, &dt,
			&p.Worker, &p.WorkType, &p.WorkContent,
			&p.PartsUsed, &p.TimeSpent, &p.Notes); err != nil {
			return nil, err
		}
		p.ProcessDatetime = parseTime(dt)
		items = append(items, p)
	}
	return items, rows.Err()
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
	_, err = r.db.Exec(`
		INSERT INTO as_processes
		(process_id,process_number,as_id,process_datetime,worker,work_type,work_content,parts_used,time_spent,notes)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.ProcessID, p.ProcessNumber, p.ASID, at.Format("2006-01-02 15:04:05"),
		p.Worker, p.WorkType, p.WorkContent, p.PartsUsed, p.TimeSpent, p.Notes)
	return err
}

func (r *ASProcessRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM as_processes WHERE process_id=?`, id)
	return err
}

// DeleteByASAndID 해당 접수에 속한 처리 이력만 삭제
func (r *ASProcessRepo) DeleteByASAndID(asID, processID string) error {
	res, err := r.db.Exec(`DELETE FROM as_processes WHERE process_id=? AND as_id=?`, processID, asID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

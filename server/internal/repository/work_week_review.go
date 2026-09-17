package repository

import (
	"strings"
	"time"

	"customer-support/internal/model"
)

// WeekCompleteRow 지난주 완료 한 건. 소요일은 호출 쪽에서 §4.14 로 센다.
type WeekCompleteRow struct {
	Receipt  string
	Complete string
	Due      string
}

func (r *WorkBoardRepo) ListWeekCompletes(mineUserID string, mineKeys []string, from, to string) ([]WeekCompleteRow, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	q := `
		SELECT COALESCE(NULLIF(TRIM(t.receipt_date),''), date(t.created_at)),
		       COALESCE(t.complete_date,''),
		       COALESCE(NULLIF(TRIM(t.due_date),''), COALESCE(t.work_date,''))
		  FROM work_tasks t
		 WHERE t.status='complete'
		   AND TRIM(COALESCE(t.complete_date,'')) != ''
		   AND t.complete_date >= ?
		   AND t.complete_date <= ?`
	args := []interface{}{from, to}
	if cond, mineArgs := mineTaskAssigneeCond(mineUserID, mineKeys); cond != "" {
		q += cond
		args = append(args, mineArgs...)
	}
	rows, err := queryTimed(r.db, "work.weekReviewComplete", q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []WeekCompleteRow
	for rows.Next() {
		var row WeekCompleteRow
		if err := rows.Scan(&row.Receipt, &row.Complete, &row.Due); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *WorkBoardRepo) CountWeekCarried(mineUserID string, mineKeys []string, thisMonday string) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	q := `
		SELECT COUNT(*)
		  FROM work_tasks t
		 WHERE COALESCE(t.status,'') NOT IN ('complete','cancelled')
		   AND TRIM(COALESCE(t.blocked_reason,'')) = ''
		   AND TRIM(COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''))) != ''
		   AND COALESCE(NULLIF(TRIM(t.work_date),''), t.due_date) < ?`
	args := []interface{}{thisMonday}
	if cond, mineArgs := mineTaskAssigneeCond(mineUserID, mineKeys); cond != "" {
		q += cond
		args = append(args, mineArgs...)
	}
	var n int
	if err := r.db.QueryRow(q, args...).Scan(&n); err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

func (r *WorkBoardRepo) WeekReviewParts(mineUserID string, mineKeys []string, now time.Time) ([]WeekCompleteRow, int, error) {
	if now.IsZero() {
		now = time.Now()
	}
	from, to := model.LastISOWeekRange(now)
	thisMon := model.CalendarMonday(now).Format("2006-01-02")
	rows, err := r.ListWeekCompletes(mineUserID, mineKeys, from, to)
	if err != nil {
		return nil, 0, err
	}
	carried, err := r.CountWeekCarried(mineUserID, mineKeys, thisMon)
	if err != nil {
		return nil, 0, err
	}
	return rows, carried, nil
}

package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type StaffLeaveRepo struct{ db *sql.DB }

func NewStaffLeaveRepo(db *sql.DB) *StaffLeaveRepo {
	if db == nil {
		return nil
	}
	return &StaffLeaveRepo{db: db}
}

func (r *MaintenanceRepo) Leaves() *StaffLeaveRepo {
	if r == nil || r.db == nil {
		return nil
	}
	return NewStaffLeaveRepo(r.db)
}

func (r *StaffLeaveRepo) Get(id string) (*model.StaffLeave, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	id = strings.TrimSpace(id)
	row := r.db.QueryRow(`
		SELECT l.leave_id, l.user_id, COALESCE(NULLIF(TRIM(u.full_name),''), COALESCE(u.username,''), ''),
		       l.leave_date, l.leave_kind, COALESCE(l.note,''), COALESCE(l.created_by,''), IFNULL(l.created_at,'')
		FROM staff_leaves l
		LEFT JOIN users u ON u.user_id = l.user_id
		WHERE l.leave_id=?`, id)
	item, err := scanStaffLeave(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (r *StaffLeaveRepo) ListByYear(year int) ([]model.StaffLeave, error) {
	if r == nil || r.db == nil || year <= 0 {
		return nil, nil
	}
	from := fmt.Sprintf("%04d-01-01", year)
	to := fmt.Sprintf("%04d-12-31", year)
	return r.ListInRange(from, to)
}

func (r *StaffLeaveRepo) ListInRange(from, toInclusive string) ([]model.StaffLeave, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	from = strings.TrimSpace(from)
	toInclusive = strings.TrimSpace(toInclusive)
	if len(from) >= 10 {
		from = from[:10]
	}
	if len(toInclusive) >= 10 {
		toInclusive = toInclusive[:10]
	}
	rows, err := r.db.Query(`
		SELECT l.leave_id, l.user_id, COALESCE(NULLIF(TRIM(u.full_name),''), COALESCE(u.username,''), ''),
		       l.leave_date, l.leave_kind, COALESCE(l.note,''), COALESCE(l.created_by,''), IFNULL(l.created_at,'')
		FROM staff_leaves l
		LEFT JOIN users u ON u.user_id = l.user_id
		WHERE l.leave_date >= ? AND l.leave_date <= ?
		ORDER BY l.leave_date, u.full_name`, from, toInclusive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.StaffLeave
	for rows.Next() {
		item, err := scanStaffLeave(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *StaffLeaveRepo) Create(item model.StaffLeave) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("저장소가 없습니다")
	}
	item.UserID = strings.TrimSpace(item.UserID)
	item.Date = strings.TrimSpace(item.Date)
	if len(item.Date) >= 10 {
		item.Date = item.Date[:10]
	}
	if item.UserID == "" {
		return fmt.Errorf("직원을 선택하세요")
	}
	if item.Date == "" {
		return fmt.Errorf("날짜를 입력하세요")
	}
	item.Kind = strings.TrimSpace(item.Kind)
	if item.Kind == "" {
		item.Kind = model.LeaveKindAnnual
	}
	if !model.ValidLeaveKind(item.Kind) {
		return fmt.Errorf("연차 구분이 올바르지 않습니다")
	}
	if item.LeaveID == "" {
		item.LeaveID = newID("LV")
	}
	_, err := r.db.Exec(`
		INSERT INTO staff_leaves (leave_id, user_id, leave_date, leave_kind, note, created_by)
		VALUES (?,?,?,?,?,?)`,
		item.LeaveID, item.UserID, item.Date, item.Kind, strings.TrimSpace(item.Note), strings.TrimSpace(item.CreatedBy))
	if err != nil {
		if isUniqueErr(err) {
			return fmt.Errorf("이미 등록된 연차입니다")
		}
		return err
	}
	logCreate(r.db, "staff_leaves", "leave_id", item.LeaveID, model.LeaveBadgeText(item.UserName, item.Kind))
	return nil
}

func (r *StaffLeaveRepo) Delete(id string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("저장소가 없습니다")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("연차를 선택하세요")
	}
	h, _ := r.Get(id)
	label := id
	if h != nil {
		label = model.LeaveBadgeText(h.UserName, h.Kind)
	}
	return touchDelete(r.db, "staff_leaves", "leave_id", id, label, func() error {
		_, err := r.db.Exec(`DELETE FROM staff_leaves WHERE leave_id=?`, id)
		return err
	})
}

// IsNameOnLeave 담당자 표시명(또는 user_id)이 그날 연차이면 true. §23.13.6
func (r *StaffLeaveRepo) IsNameOnLeave(nameOrID, date string) bool {
	if r == nil || r.db == nil {
		return false
	}
	nameOrID = strings.TrimSpace(nameOrID)
	date = strings.TrimSpace(date)
	if len(date) >= 10 {
		date = date[:10]
	}
	if nameOrID == "" || date == "" {
		return false
	}
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM staff_leaves l
		LEFT JOIN users u ON u.user_id = l.user_id
		WHERE l.leave_date=?
		  AND (
		        l.user_id = ?
		     OR LOWER(TRIM(COALESCE(u.full_name,''))) = LOWER(?)
		     OR LOWER(TRIM(COALESCE(u.username,''))) = LOWER(?)
		  )`, date, nameOrID, nameOrID, nameOrID).Scan(&n)
	return err == nil && n > 0
}

func scanStaffLeave(row interface {
	Scan(dest ...interface{}) error
}) (*model.StaffLeave, error) {
	var it model.StaffLeave
	if err := row.Scan(&it.LeaveID, &it.UserID, &it.UserName, &it.Date, &it.Kind, &it.Note, &it.CreatedBy, &it.CreatedAt); err != nil {
		return nil, err
	}
	if len(it.Date) >= 10 {
		it.Date = it.Date[:10]
	}
	return &it, nil
}

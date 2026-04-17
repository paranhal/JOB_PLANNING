package repository

import (
	"database/sql"

	"customer-support/internal/model"
)

type CodeRepo struct{ db *sql.DB }

func NewCodeRepo(db *sql.DB) *CodeRepo { return &CodeRepo{db: db} }

func (r *CodeRepo) ListAll() ([]model.Code, error) {
	rows, err := r.db.Query(
		`SELECT code_id, code_group, code_value, code_name, sort_order, is_active
		 FROM codes ORDER BY code_group, sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodes(rows)
}

func (r *CodeRepo) ListByGroup(group string) ([]model.Code, error) {
	rows, err := r.db.Query(
		`SELECT code_id, code_group, code_value, code_name, sort_order, is_active
		 FROM codes WHERE code_group=? ORDER BY sort_order`, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodes(rows)
}

func (r *CodeRepo) ActiveByGroup(group string) ([]model.Code, error) {
	rows, err := r.db.Query(
		`SELECT code_id, code_group, code_value, code_name, sort_order, is_active
		 FROM codes WHERE code_group=? AND is_active=1 ORDER BY sort_order`, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodes(rows)
}

func (r *CodeRepo) Groups() ([]string, error) {
	rows, err := r.db.Query(`SELECT DISTINCT code_group FROM codes ORDER BY code_group`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (r *CodeRepo) Create(c *model.Code) error {
	c.CodeID = newID("CD")
	_, err := r.db.Exec(
		`INSERT INTO codes (code_id,code_group,code_value,code_name,sort_order,is_active)
		 VALUES (?,?,?,?,?,?)`,
		c.CodeID, c.CodeGroup, c.CodeValue, c.CodeName, c.SortOrder, boolToInt(c.IsActive))
	return err
}

func (r *CodeRepo) Update(c *model.Code) error {
	_, err := r.db.Exec(
		`UPDATE codes SET code_group=?,code_value=?,code_name=?,sort_order=?,is_active=?
		 WHERE code_id=?`,
		c.CodeGroup, c.CodeValue, c.CodeName, c.SortOrder, boolToInt(c.IsActive), c.CodeID)
	return err
}

func (r *CodeRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM codes WHERE code_id=?`, id)
	return err
}

func scanCodes(rows *sql.Rows) ([]model.Code, error) {
	var items []model.Code
	for rows.Next() {
		var c model.Code
		var active int
		if err := rows.Scan(&c.CodeID, &c.CodeGroup, &c.CodeValue, &c.CodeName, &c.SortOrder, &active); err != nil {
			return nil, err
		}
		c.IsActive = active == 1
		items = append(items, c)
	}
	return items, rows.Err()
}

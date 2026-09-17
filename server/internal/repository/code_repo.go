package repository

import (
	"database/sql"

	"customer-support/internal/model"
)

type CodeRepo struct{ db *sql.DB }

func NewCodeRepo(db *sql.DB) *CodeRepo { return &CodeRepo{db: db} }

func (r *CodeRepo) ListAll() ([]model.Code, error) {
	if items, ok := codesSnapshot(); ok {
		return items, nil
	}
	return scanCodesQuery(r.db, `SELECT code_id, code_group, code_value, code_name, sort_order, is_active
		 FROM codes ORDER BY code_group, sort_order`)
}

func (r *CodeRepo) ListByGroup(group string) ([]model.Code, error) {
	if items, ok := codesSnapshot(); ok {
		var out []model.Code
		for _, c := range items {
			if c.CodeGroup == group {
				out = append(out, c)
			}
		}
		return out, nil
	}
	return scanCodesQuery(r.db, `SELECT code_id, code_group, code_value, code_name, sort_order, is_active
		 FROM codes WHERE code_group=? ORDER BY sort_order`, group)
}

func (r *CodeRepo) ActiveByGroup(group string) ([]model.Code, error) {
	if items, ok := codesSnapshot(); ok {
		var out []model.Code
		for _, c := range items {
			if c.CodeGroup == group && c.IsActive {
				out = append(out, c)
			}
		}
		return out, nil
	}
	return scanCodesQuery(r.db, `SELECT code_id, code_group, code_value, code_name, sort_order, is_active
		 FROM codes WHERE code_group=? AND is_active=1 ORDER BY sort_order`, group)
}

func (r *CodeRepo) Groups() ([]string, error) {
	if items, ok := codesSnapshot(); ok {
		seen := map[string]bool{}
		var groups []string
		for _, c := range items {
			if !seen[c.CodeGroup] {
				seen[c.CodeGroup] = true
				groups = append(groups, c.CodeGroup)
			}
		}
		return groups, nil
	}
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
	if err != nil {
		return err
	}
	logCreate(r.db, "codes", "code_id", c.CodeID, c.CodeName)
	reloadCodes(r.db)
	return nil
}

func (r *CodeRepo) Update(c *model.Code) error {
	err := touchUpdate(r.db, "codes", "code_id", c.CodeID, c.CodeName, func() error {
		_, err := r.db.Exec(
			`UPDATE codes SET code_group=?,code_value=?,code_name=?,sort_order=?,is_active=?
		 WHERE code_id=?`,
			c.CodeGroup, c.CodeValue, c.CodeName, c.SortOrder, boolToInt(c.IsActive), c.CodeID)
		return err
	})
	if err == nil {
		reloadCodes(r.db)
	}
	return err
}

func (r *CodeRepo) Delete(id string) error {
	err := touchDelete(r.db, "codes", "code_id", id, "코드", func() error {
		_, err := r.db.Exec(`DELETE FROM codes WHERE code_id=?`, id)
		return err
	})
	if err == nil {
		reloadCodes(r.db)
	}
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

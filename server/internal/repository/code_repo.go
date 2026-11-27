package repository

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

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
	if c != nil && c.CodeGroup == model.WorkCategoryCodeGroup {
		if err := r.checkWorkCategory(c, ""); err != nil {
			return err
		}
	}
	if c != nil && isProductCategoryGroup(c.CodeGroup) {
		return r.CreateProductCategory(c)
	}
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
	if c != nil && c.CodeGroup == model.WorkCategoryCodeGroup {
		if err := r.checkWorkCategory(c, c.CodeID); err != nil {
			return err
		}
	}
	if c != nil && isProductCategoryGroup(c.CodeGroup) {
		return r.UpdateProductCategory(c)
	}
	cur, _ := r.Get(c.CodeID)
	if cur != nil && isProductCategoryGroup(cur.CodeGroup) {
		return r.UpdateProductCategory(c)
	}
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
	if cur, err := r.Get(id); err == nil && cur != nil && cur.CodeGroup == model.WorkCategoryCodeGroup {
		return fmt.Errorf("업무 구분은 삭제할 수 없습니다. 사용 안 함(비활성)으로 바꾸세요")
	}
	if cur, err := r.Get(id); err == nil && cur != nil && isProductCategoryGroup(cur.CodeGroup) {
		return r.DeleteProductCategory(id)
	}
	return r.deleteCodeRow(id)
}

func (r *CodeRepo) deleteCodeRow(id string) error {
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

var workCategoryKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,30}$`)

// checkWorkCategory 업무 구분 코드: 값은 영문 소문자 키(work_tasks.activity_kind), 이름·값은 그룹 안에서 겹치지 않는다.
func (r *CodeRepo) checkWorkCategory(c *model.Code, selfID string) error {
	c.CodeValue = strings.TrimSpace(c.CodeValue)
	c.CodeName = strings.TrimSpace(c.CodeName)
	if !workCategoryKeyRe.MatchString(c.CodeValue) {
		return fmt.Errorf("업무 구분 코드 값은 영문 소문자·숫자·밑줄 2~31자로 적어 주세요(예: field_ops)")
	}
	if c.CodeName == "" {
		return fmt.Errorf("업무 구분 이름을 입력하세요")
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group=? AND code_id<>? AND (code_value=? OR code_name=?)`,
		model.WorkCategoryCodeGroup, selfID, c.CodeValue, c.CodeName).Scan(&n)
	if n > 0 {
		return fmt.Errorf("이미 있는 업무 구분 코드 또는 이름입니다")
	}
	return nil
}

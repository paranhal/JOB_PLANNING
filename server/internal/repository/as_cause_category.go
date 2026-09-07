package repository

import "customer-support/internal/model"

// ListCauseCategories 원인분류(활성). §34.3.3
func (r *ASRepo) ListCauseCategories() ([]model.CauseCategory, error) {
	rows, err := r.db.Query(`
		SELECT code, level, COALESCE(parent_code,''), label, COALESCE(sort_order,0), COALESCE(is_active,1),
		       COALESCE(cause_type_map,''), COALESCE(is_fault,1)
		  FROM as_cause_categories
		 WHERE COALESCE(is_active,1)=1
		 ORDER BY level, sort_order, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CauseCategory
	for rows.Next() {
		var c model.CauseCategory
		var active, fault int
		if err := rows.Scan(&c.Code, &c.Level, &c.ParentCode, &c.Label, &c.SortOrder, &active, &c.CauseTypeMap, &fault); err != nil {
			return nil, err
		}
		c.IsActive = active == 1
		c.IsFault = fault == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

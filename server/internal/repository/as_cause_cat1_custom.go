package repository

import (
	"database/sql"
	"strings"

	"customer-support/internal/model"
)

func applyASCauseCat1CustomV88(db *sql.DB) {
	if db == nil {
		return
	}
	addTextColumn(db, "as_receipts", "cause_cat1_custom")
}

func ApplyASCauseCat1CustomV88ForTest(db *sql.DB) { applyASCauseCat1CustomV88(db) }

func (r *ASRepo) ListCauseCat1CustomRecent(orgID string, limit int) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	orgID = strings.TrimSpace(orgID)
	q := `
		SELECT TRIM(cause_cat1_custom) AS lab, MAX(updated_at)
		  FROM as_receipts
		 WHERE cause_cat1 = ?
		   AND TRIM(COALESCE(cause_cat1_custom,'')) != ''`
	args := []interface{}{model.CauseCatCustom}
	if orgID != "" && orgID != OrgAll {
		q += ` AND org_id = ?`
		args = append(args, orgID)
	}
	q += `
		 GROUP BY lower(TRIM(cause_cat1_custom))
		 ORDER BY MAX(updated_at) DESC
		 LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such column") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var lab, at string
		if err := rows.Scan(&lab, &at); err != nil {
			return nil, err
		}
		lab = model.NormalizeCauseCatCustom(lab)
		if lab != "" {
			out = append(out, lab)
		}
	}
	return out, rows.Err()
}

type CauseCat1Count struct {
	Key   string
	Label string
	N     int
}

func (r *ASRepo) CountCauseCat1(orgID string) ([]CauseCat1Count, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	cats, _ := r.ListCauseCategories()
	labels := model.CauseCategoryLabels(cats)
	q := `
		SELECT TRIM(COALESCE(cause_cat1,'')), TRIM(COALESCE(cause_cat1_custom,'')), COUNT(*)
		  FROM as_receipts
		 WHERE TRIM(COALESCE(cause_cat1,'')) != ''`
	args := []interface{}{}
	orgID = strings.TrimSpace(orgID)
	if orgID != "" && orgID != OrgAll {
		q += ` AND org_id = ?`
		args = append(args, orgID)
	}
	q += ` GROUP BY TRIM(COALESCE(cause_cat1,'')), lower(TRIM(COALESCE(cause_cat1_custom,'')))`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sum := map[string]*CauseCat1Count{}
	var order []string
	for rows.Next() {
		var cat1, custom string
		var n int
		if err := rows.Scan(&cat1, &custom, &n); err != nil {
			return nil, err
		}
		key := model.CauseCat1GroupKey(cat1, custom)
		lab := model.CauseCat1Label(cat1, custom, labels)
		if cur, ok := sum[key]; ok {
			cur.N += n
			continue
		}
		sum[key] = &CauseCat1Count{Key: key, Label: lab, N: n}
		order = append(order, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]CauseCat1Count, 0, len(order))
	for _, k := range order {
		out = append(out, *sum[k])
	}
	return out, nil
}

package repository

import (
	"database/sql"
	"log"
	"strings"
	"unicode/utf8"

	"customer-support/internal/model"
)

const salesActivitySearchCreateSQL = `CREATE VIRTUAL TABLE sales_activity_search USING fts5(
  activity_id UNINDEXED, title, sales_name, customer_name,
  tokenize='trigram'
)`

func applySalesActivitySearch(db *sql.DB) {
	if db == nil {
		return
	}
	var ddl string
	_ = db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='sales_activity_search'`).Scan(&ddl)
	if ddl != "" && !strings.Contains(strings.ToLower(ddl), "trigram") {
		if _, err := db.Exec(`DROP TABLE sales_activity_search`); err != nil {
			log.Printf("sales_activity_search drop: %v", err)
		}
		ddl = ""
	}
	if ddl == "" {
		if _, err := db.Exec(salesActivitySearchCreateSQL); err != nil {
			log.Printf("sales_activity_search: FTS5+trigram 없음, LIKE 로 검색한다 (§12.11.3): %v", err)
			return
		}
	}
	if err := RebuildSalesActivitySearch(db); err != nil {
		log.Printf("sales_activity_search rebuild: %v", err)
	}
}

func salesActivitySearchHasFTS(db *sql.DB) bool {
	if db == nil {
		return false
	}
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sales_activity_search'`).Scan(&n)
	return err == nil && n > 0
}

func RebuildSalesActivitySearch(db *sql.DB) error {
	if !salesActivitySearchHasFTS(db) {
		return nil
	}
	if _, err := db.Exec(`DELETE FROM sales_activity_search`); err != nil {
		return err
	}
	_, err := db.Exec(`
		INSERT INTO sales_activity_search(activity_id, title, sales_name, customer_name)
		SELECT a.activity_id, COALESCE(a.title,''), COALESCE(s.name,''),
			COALESCE(NULLIF(TRIM(cu.org_name),''), NULLIF(TRIM(s.prospect_name),''), '')
		FROM sales_activities a
		LEFT JOIN sales_projects s ON s.sales_id = a.sales_id
		LEFT JOIN customers cu ON cu.customer_id = s.customer_id`)
	return err
}

func reindexSalesActivitySearch(db *sql.DB, activityID string) {
	activityID = strings.TrimSpace(activityID)
	if db == nil || activityID == "" || !salesActivitySearchHasFTS(db) {
		return
	}
	_, _ = db.Exec(`DELETE FROM sales_activity_search WHERE activity_id=?`, activityID)
	_, _ = db.Exec(`
		INSERT INTO sales_activity_search(activity_id, title, sales_name, customer_name)
		SELECT a.activity_id, COALESCE(a.title,''), COALESCE(s.name,''),
			COALESCE(NULLIF(TRIM(cu.org_name),''), NULLIF(TRIM(s.prospect_name),''), '')
		FROM sales_activities a
		LEFT JOIN sales_projects s ON s.sales_id = a.sales_id
		LEFT JOIN customers cu ON cu.customer_id = s.customer_id
		WHERE a.activity_id=?`, activityID)
}

type SalesActivityFilter struct {
	Month  string
	Types  []string
	Query  string
	Member string // 참여한 사람 정확 일치. 40-F
}

func (r *SalesRepo) ListActivitiesFilter(f SalesActivityFilter) ([]model.SalesActivity, error) {
	q := salesActivitySelect + ` WHERE 1=1`
	var args []interface{}
	if m := strings.TrimSpace(f.Month); len(m) >= 7 {
		q += ` AND a.activity_date >= ? AND a.activity_date < date(?, '+1 month')`
		args = append(args, m+"-01", m+"-01")
	}
	if ids := r.activitySearchIDs(f.Query); ids != nil {
		if len(ids) == 0 {
			return nil, nil
		}
		ph := strings.Repeat("?,", len(ids))
		q += ` AND a.activity_id IN (` + strings.TrimSuffix(ph, ",") + `)`
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if m := strings.TrimSpace(f.Member); m != "" {
		q += ` AND EXISTS (SELECT 1 FROM sales_activity_members sm WHERE sm.activity_id=a.activity_id AND sm.member=?)`
		args = append(args, m)
	}
	if types := compactTypes(f.Types); len(types) > 0 {
		ph := strings.Repeat("?,", len(types))
		q += ` AND a.activity_type IN (` + strings.TrimSuffix(ph, ",") + `)`
		for _, t := range types {
			args = append(args, t)
		}
	}
	q += ` ORDER BY a.activity_date DESC, COALESCE(NULLIF(a.start_time,''),'00:00') DESC, a.activity_id DESC`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return r.scanActivityRows(rows)
}

func compactTypes(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// activitySearchIDs nil = 검색 없음, 빈 슬라이스 = 0건.
func (r *SalesRepo) activitySearchIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if salesActivitySearchHasFTS(r.db) && utf8.RuneCountInString(raw) >= 3 {
		ids, err := r.activitySearchFTS(raw)
		if err == nil {
			return ids
		}
		log.Printf("sales_activity_search FTS: %v — LIKE", err)
	}
	return r.activitySearchLike(raw)
}

func (r *SalesRepo) activitySearchFTS(q string) ([]string, error) {
	rows, err := r.db.Query(`SELECT activity_id FROM sales_activity_search WHERE sales_activity_search MATCH ?`, fts5Query(q))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, rows.Err()
}

func (r *SalesRepo) activitySearchLike(q string) []string {
	like := "%" + q + "%"
	rows, err := r.db.Query(`
		SELECT a.activity_id FROM sales_activities a
		LEFT JOIN sales_projects s ON s.sales_id = a.sales_id
		LEFT JOIN customers cu ON cu.customer_id = s.customer_id
		WHERE a.title LIKE ? OR COALESCE(s.name,'') LIKE ?
		   OR COALESCE(cu.org_name,'') LIKE ? OR COALESCE(s.prospect_name,'') LIKE ?
		   OR EXISTS (SELECT 1 FROM sales_activity_members sm
		              WHERE sm.activity_id=a.activity_id AND (sm.member=? OR sm.member LIKE ?))`,
		like, like, like, like, q, like)
	if err != nil {
		return []string{}
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	if ids == nil {
		ids = []string{}
	}
	return ids
}

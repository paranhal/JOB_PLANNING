package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

func applyWorkTaskOrgFromSourceV87(db *sql.DB) {
	if db == nil {
		return
	}
	n, byOrg, err := CorrectWorkTaskOrgFromSource(db)
	if err != nil {
		log.Printf("087 work_task org_id: %v", err)
		return
	}
	if n == 0 {
		log.Printf("087 work_task org_id: 보정 0건")
		return
	}
	log.Printf("087 work_task org_id: 보정 %d건 %v", n, byOrg)
}

// CorrectWorkTaskOrgFromSource AS·방문·영업 출처의 work_tasks.org_id 를 맞춘다. 멱등.
func CorrectWorkTaskOrgFromSource(db *sql.DB) (int, map[string]int, error) {
	if db == nil {
		return 0, nil, nil
	}
	before, err := countMismatchedSourceTaskOrgs(db)
	if err != nil {
		return 0, nil, err
	}
	if _, err := db.Exec(`
		UPDATE work_tasks
		   SET org_id = (
		        SELECT ar.org_id FROM as_receipts ar WHERE ar.as_id = work_tasks.source_id
		       )
		 WHERE source_type = ?
		   AND TRIM(COALESCE(source_id,'')) != ''
		   AND source_id IN (SELECT as_id FROM as_receipts)
		   AND TRIM(COALESCE(org_id,'')) != (
		        SELECT TRIM(COALESCE(ar.org_id,'')) FROM as_receipts ar WHERE ar.as_id = work_tasks.source_id
		       )`, model.WBSourceAS); err != nil {
		return 0, nil, err
	}
	if _, err := db.Exec(`
		UPDATE work_tasks
		   SET org_id = (
		        SELECT COALESCE(NULLIF(TRIM(cfg.org_id),''), NULLIF(TRIM(c.org_id),''))
		          FROM maintenance_visits v
		          JOIN customers c ON c.customer_id = v.customer_id
		          LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		         WHERE v.visit_id = work_tasks.source_id
		       )
		 WHERE source_type = ?
		   AND TRIM(COALESCE(source_id,'')) != ''
		   AND source_id IN (SELECT visit_id FROM maintenance_visits)
		   AND TRIM(COALESCE((
		        SELECT COALESCE(NULLIF(TRIM(cfg.org_id),''), NULLIF(TRIM(c.org_id),''))
		          FROM maintenance_visits v
		          JOIN customers c ON c.customer_id = v.customer_id
		          LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		         WHERE v.visit_id = work_tasks.source_id
		       ),'')) != ''
		   AND TRIM(COALESCE(org_id,'')) != (
		        SELECT TRIM(COALESCE(NULLIF(TRIM(cfg.org_id),''), NULLIF(TRIM(c.org_id),''), ''))
		          FROM maintenance_visits v
		          JOIN customers c ON c.customer_id = v.customer_id
		          LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		         WHERE v.visit_id = work_tasks.source_id
		       )`, model.WBSourceMaintenance); err != nil && !strings.Contains(err.Error(), "no such table") {
		return 0, nil, err
	}
	if _, err := db.Exec(`
		UPDATE work_tasks
		   SET org_id = (
		        SELECT p.org_id
		          FROM sales_activities a
		          JOIN work_projects p ON p.project_id = a.sales_id
		         WHERE a.activity_id = work_tasks.source_id
		       )
		 WHERE source_type = ?
		   AND TRIM(COALESCE(source_id,'')) != ''
		   AND source_id IN (SELECT activity_id FROM sales_activities)
		   AND TRIM(COALESCE((
		        SELECT TRIM(COALESCE(p.org_id,''))
		          FROM sales_activities a
		          JOIN work_projects p ON p.project_id = a.sales_id
		         WHERE a.activity_id = work_tasks.source_id
		       ),'')) != ''
		   AND TRIM(COALESCE(org_id,'')) != (
		        SELECT TRIM(COALESCE(p.org_id,''))
		          FROM sales_activities a
		          JOIN work_projects p ON p.project_id = a.sales_id
		         WHERE a.activity_id = work_tasks.source_id
		       )`, model.WBSourceSalesActivity); err != nil && !strings.Contains(err.Error(), "no such table") {
		return 0, nil, err
	}
	after, err := countMismatchedSourceTaskOrgs(db)
	if err != nil {
		return 0, nil, err
	}
	n := 0
	byOrg := map[string]int{}
	for org, b := range before {
		a := after[org]
		d := b - a
		if d > 0 {
			byOrg[org] = d
			n += d
		}
	}
	return n, byOrg, nil
}

func countMismatchedSourceTaskOrgs(db *sql.DB) (map[string]int, error) {
	out := map[string]int{}
	rows, err := db.Query(`
		SELECT COALESCE(ar.org_id,''), COUNT(*)
		  FROM work_tasks t
		  JOIN as_receipts ar ON ar.as_id = t.source_id
		 WHERE t.source_type = ?
		   AND TRIM(COALESCE(t.org_id,'')) != TRIM(COALESCE(ar.org_id,''))
		 GROUP BY ar.org_id`, model.WBSourceAS)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var org string
		var n int
		if err := rows.Scan(&org, &n); err != nil {
			return nil, err
		}
		out[org] += n
	}
	return out, rows.Err()
}

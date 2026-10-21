package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyWorkTaskSitesCustomer(db *sql.DB) {
	if db == nil {
		return
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_task_sites'`).Scan(&n); err != nil {
		log.Printf("077 work_task_sites: %v", err)
		return
	}
	if n == 0 {
		if err := execSQLIgnoreDupCol(db, workTaskSitesCustomerCreateSQL); err != nil {
			log.Printf("077 work_task_sites create: %v", err)
		}
		return
	}
	if pragmaHasColumn(db, "work_task_sites", "customer_id") && !pragmaHasColumn(db, "work_task_sites", "site_id") {
		return
	}
	up, err := workTaskSitesCustomerMigrationSQL()
	if err != nil {
		log.Printf("077 work_task_sites_customer: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("077 work_task_sites_customer apply: %v", err)
	}
}

func pragmaHasColumn(db *sql.DB, table, col string) bool {
	var n int
	q := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name='%s'`, table, col)
	_ = db.QueryRow(q).Scan(&n)
	return n > 0
}

const workTaskSitesCustomerCreateSQL = `
CREATE TABLE IF NOT EXISTS work_task_sites (
  task_id TEXT NOT NULL,
  customer_id TEXT NOT NULL,
  project_site_id TEXT,
  PRIMARY KEY (task_id, customer_id)
);
CREATE INDEX IF NOT EXISTS ix_wts_customer ON work_task_sites(customer_id);
CREATE INDEX IF NOT EXISTS ix_wts_psite ON work_task_sites(project_site_id);
`

func workTaskSitesCustomerMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("077_work_task_sites_customer.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("077_work_task_sites_customer.sql 없음")
}

func workTaskSitesCustomerDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("077_work_task_sites_customer.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("077_work_task_sites_customer.down.sql 없음")
}

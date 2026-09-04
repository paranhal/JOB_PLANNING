package repository

import (
	"database/sql"
	"log"
	"strings"
)

// applySalesDealTypeV228 마이그레이션 034. §36.4 deal_type · §36.5 단품 5단계.
func applySalesDealTypeV228(db *sql.DB) {
	if db == nil {
		return
	}
	addSalesDealTypeColumn(db, "deal_type", `ALTER TABLE sales_projects ADD COLUMN deal_type TEXT NOT NULL DEFAULT 'build'`)
	addSalesDealTypeColumn(db, "po_no", `ALTER TABLE sales_projects ADD COLUMN po_no TEXT NOT NULL DEFAULT ''`)
	addSalesDealTypeColumn(db, "delivered_at", `ALTER TABLE sales_projects ADD COLUMN delivered_at TEXT NOT NULL DEFAULT ''`)
	for _, q := range []string{
		`UPDATE sales_projects SET deal_type='build' WHERE TRIM(COALESCE(deal_type,''))=''`,
		`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
			('SSS01','sales_supply_stage','inquiry','문의 접수',1,1),
			('SSS02','sales_supply_stage','quoted','견적 제출',2,1),
			('SSS03','sales_supply_stage','ordered','수주',3,1),
			('SSS04','sales_supply_stage','delivered','납품 완료',4,1),
			('SSS05','sales_supply_stage','dropped','취소·실주',5,1)`,
		`UPDATE codes SET code_name='문의 접수', sort_order=1, is_active=1 WHERE code_id='SSS01'`,
		`UPDATE codes SET code_name='견적 제출', sort_order=2, is_active=1 WHERE code_id='SSS02'`,
		`UPDATE codes SET code_name='수주', sort_order=3, is_active=1 WHERE code_id='SSS03'`,
		`UPDATE codes SET code_name='납품 완료', sort_order=4, is_active=1 WHERE code_id='SSS04'`,
		`UPDATE codes SET code_name='취소·실주', sort_order=5, is_active=1 WHERE code_id='SSS05'`,
		`CREATE INDEX IF NOT EXISTS idx_sales_projects_deal_type ON sales_projects(deal_type)`,
	} {
		if _, err := db.Exec(q); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return
			}
			log.Printf("034 sales deal_type: %v", err)
		}
	}
}

func addSalesDealTypeColumn(db *sql.DB, name, ddl string) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sales_projects') WHERE name=?`, name).Scan(&n)
	if err == nil && n > 0 {
		return
	}
	if _, err := db.Exec(ddl); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("034 sales_projects.%s: %v", name, err)
	}
}

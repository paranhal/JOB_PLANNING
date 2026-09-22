package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"customer-support/internal/model"
)

// applySalesStagesV227 마이그레이션 031. §32.3 8단계 → 6단계.
func applySalesStagesV227(db *sql.DB) {
	if db == nil {
		return
	}
	if metaDone(db, sales4StageMetaKey) {
		return
	}
	addSalesProjectColumn(db, "legacy_stage", `ALTER TABLE sales_projects ADD COLUMN legacy_stage TEXT NOT NULL DEFAULT ''`)
	addSalesProjectColumn(db, "won_at", `ALTER TABLE sales_projects ADD COLUMN won_at TEXT NOT NULL DEFAULT ''`)
	addSalesProjectColumn(db, "contracted_at", `ALTER TABLE sales_projects ADD COLUMN contracted_at TEXT NOT NULL DEFAULT ''`)

	for _, q := range []string{
		`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
			('SST10','sales_stage','negotiation','협상',4,1),
			('SSP10','sales_stage_prob','negotiation','70',4,1),
			('SAT11','sales_activity_type','rfp','제안서제출(RFP)',8,1)`,
		`UPDATE codes SET sort_order=1, is_active=1 WHERE code_id='SST02'`,
		`UPDATE codes SET sort_order=2, is_active=1 WHERE code_id='SST01'`,
		`UPDATE codes SET sort_order=3, is_active=1 WHERE code_id='SST03'`,
		`UPDATE codes SET code_name='협상', sort_order=4, is_active=1 WHERE code_id='SST10'`,
		`UPDATE codes SET code_name='수주', sort_order=5, is_active=1 WHERE code_id='SST07'`,
		`UPDATE codes SET code_name='실주', sort_order=6, is_active=1 WHERE code_id='SST09'`,
		`UPDATE codes SET is_active=0 WHERE code_id IN ('SST04','SST05','SST06','SST08')`,
		`UPDATE codes SET code_name='10', sort_order=1, is_active=1 WHERE code_id='SSP02'`,
		`UPDATE codes SET code_name='25', sort_order=2, is_active=1 WHERE code_id='SSP01'`,
		`UPDATE codes SET code_name='40', sort_order=3, is_active=1 WHERE code_id='SSP03'`,
		`UPDATE codes SET code_name='70', sort_order=4, is_active=1 WHERE code_id='SSP10'`,
		`UPDATE codes SET code_name='100', sort_order=5, is_active=1 WHERE code_id='SSP07'`,
		`UPDATE codes SET code_name='0', sort_order=6, is_active=1 WHERE code_id='SSP09'`,
		`UPDATE codes SET is_active=0 WHERE code_id IN ('SSP04','SSP05','SSP06','SSP08')`,
		`UPDATE codes SET is_active=1, code_name='제안서제출(RFP)', sort_order=8 WHERE code_id='SAT11'`,
		`UPDATE sales_projects SET legacy_stage = stage WHERE TRIM(COALESCE(legacy_stage,'')) = ''`,
		`UPDATE sales_projects SET stage='proposal' WHERE stage IN ('quote','rfp','submit')`,
		`UPDATE sales_projects SET stage='won' WHERE stage='contracted'`,
		`UPDATE sales_projects SET contracted_at = COALESCE((
			SELECT date(MAX(h.changed_at)) FROM sales_stage_history h
			WHERE h.sales_id = sales_projects.sales_id AND h.to_stage='contracted'
		), date(updated_at), date(created_at), '')
		WHERE TRIM(COALESCE(legacy_stage,''))='contracted'
		  AND TRIM(COALESCE(contracted_at,''))=''`,
		`UPDATE sales_projects SET won_at = COALESCE((
			SELECT date(MAX(h.changed_at)) FROM sales_stage_history h
			WHERE h.sales_id = sales_projects.sales_id AND h.to_stage IN ('won','contracted')
		), date(updated_at), date(created_at), '')
		WHERE stage='won' AND TRIM(COALESCE(won_at,''))=''`,
		`UPDATE sales_projects SET probability = CASE stage
			WHEN 'contact' THEN 10
			WHEN 'lead' THEN 25
			WHEN 'proposal' THEN 40
			WHEN 'negotiation' THEN 70
			WHEN 'won' THEN 100
			WHEN 'lost' THEN 0
			ELSE probability
		END
		WHERE probability_override IS NULL`,
	} {
		if _, err := db.Exec(q); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return
			}
			log.Printf("031 sales stages v227: %v", err)
		}
	}
	seedMigratedSalesActivities(db)
}

func addSalesProjectColumn(db *sql.DB, name, ddl string) {
	if salesProjectsHasColumn(db, name) {
		return
	}
	if _, err := db.Exec(ddl); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("031 sales_projects.%s: %v", name, err)
	}
}

func salesProjectsHasColumn(db *sql.DB, name string) bool {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sales_projects') WHERE name=?`, name).Scan(&n)
	return err == nil && n > 0
}

func seedMigratedSalesActivities(db *sql.DB) {
	rows, err := db.Query(`
		SELECT sales_id, legacy FROM (
			SELECT sales_id, legacy_stage AS legacy FROM sales_projects
			WHERE legacy_stage IN ('quote','rfp','submit')
			UNION
			SELECT sales_id, to_stage AS legacy FROM sales_stage_history
			WHERE to_stage IN ('quote','rfp','submit')
		)`)
	if err != nil {
		if !strings.Contains(err.Error(), "no such table") {
			log.Printf("031 migrated activities query: %v", err)
		}
		return
	}
	defer rows.Close()
	type pair struct{ salesID, legacy string }
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.salesID, &p.legacy); err != nil {
			log.Printf("031 migrated activities scan: %v", err)
			return
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		log.Printf("031 migrated activities rows: %v", err)
		return
	}
	for _, p := range pairs {
		actType, title, ok := model.SalesMigratedActivity(p.legacy)
		if !ok {
			continue
		}
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sales_activities WHERE sales_id=? AND title=?`, p.salesID, title,
		).Scan(&n); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return
			}
			log.Printf("031 migrated activity exists: %v", err)
			continue
		}
		if n > 0 {
			continue
		}
		day := salesHistoryDate(db, p.salesID, p.legacy)
		if day == "" {
			_ = db.QueryRow(`SELECT COALESCE(date(created_at), date('now','localtime'), '') FROM sales_projects WHERE sales_id=?`, p.salesID).Scan(&day)
		}
		if strings.TrimSpace(day) == "" {
			day = "1970-01-01"
		}
		seq, err := NextSeq(db, "sales_activity")
		if err != nil {
			log.Printf("031 migrated activity seq: %v", err)
			continue
		}
		id := fmt.Sprintf("SA-%03d", seq)
		if _, err := db.Exec(`
			INSERT INTO sales_activities (
				activity_id, sales_id, activity_date, start_time, duration_min, activity_type,
				title, content, place, our_members, counterparts,
				next_action, next_action_date, stage_at_time, created_by)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, p.salesID, day, "", 0, actType,
			title, "8단계 이관 시 자동 생성", "", "", "",
			"", "", p.legacy, "이관"); err != nil {
			log.Printf("031 migrated activity insert: %v", err)
		}
	}
}

func salesHistoryDate(db *sql.DB, salesID, toStage string) string {
	var day string
	err := db.QueryRow(`
		SELECT COALESCE(date(MAX(changed_at)), '') FROM sales_stage_history
		WHERE sales_id=? AND to_stage=?`, salesID, toStage).Scan(&day)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(day)
}

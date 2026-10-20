package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"customer-support/internal/model"
)

func applySalesActivitySplit91(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`ALTER TABLE sales_activities ADD COLUMN activity_method TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("065 activity_method: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sales_act_method ON sales_activities(activity_method)`); err != nil {
		log.Printf("065 idx_sales_act_method: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sales_activities_pre91 (
			activity_id TEXT PRIMARY KEY,
			activity_type TEXT NOT NULL DEFAULT '',
			captured_at TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		log.Printf("065 pre91: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT OR IGNORE INTO sales_activities_pre91 (activity_id, activity_type, captured_at)
		SELECT activity_id, COALESCE(activity_type,''), ? FROM sales_activities`, now); err != nil {
		log.Printf("065 pre91 copy: %v", err)
	}
	for _, q := range []string{
		`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
			('SAM01','sales_activity_method','visit','방문',1,1),
			('SAM02','sales_activity_method','call','전화',2,1),
			('SAM03','sales_activity_method','mail','이메일',3,1),
			('SAM04','sales_activity_method','online','온라인미팅',4,1),
			('SAM05','sales_activity_method','internal','내부회의',5,1),
			('SAM06','sales_activity_method','other','기타',6,1)`,
		`UPDATE codes SET code_name='정보 수집', sort_order=1, is_active=1 WHERE code_group='sales_activity_type' AND code_value='research'`,
		`UPDATE codes SET code_name='자료송부', sort_order=2, is_active=1 WHERE code_group='sales_activity_type' AND code_value='material'`,
		`UPDATE codes SET code_name='견적제출', sort_order=3, is_active=1 WHERE code_group='sales_activity_type' AND code_value='quote'`,
		`UPDATE codes SET code_name='요구사항 파악', sort_order=4, is_active=1 WHERE code_group='sales_activity_type' AND code_value='requirement'`,
		`UPDATE codes SET code_name='RFP 접수', sort_order=5, is_active=1 WHERE code_group='sales_activity_type' AND code_value='rfp_received'`,
		`UPDATE codes SET code_name='제안서 제출', sort_order=6, is_active=1 WHERE code_group='sales_activity_type' AND code_value='proposal'`,
		`UPDATE codes SET code_name='기타', sort_order=7, is_active=1 WHERE code_group='sales_activity_type' AND code_value='other'`,
		`UPDATE codes SET is_active=0 WHERE code_group='sales_activity_type' AND code_value IN ('visit','call','mail','online','internal','rfp','bid')`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("065 codes: %v", err)
		}
	}
	if err := migrateSalesActivitySplit91Data(db); err != nil {
		log.Printf("065 data: %v", err)
	}
	if err := RebuildSalesActivitySearch(db); err != nil {
		log.Printf("065 reindex: %v", err)
	}
}

func migrateSalesActivitySplit91Data(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		UPDATE sales_activities SET activity_type='other', activity_method=activity_type
		WHERE TRIM(COALESCE(activity_method,''))='' AND activity_type IN ('visit','call','mail','online','internal')`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE sales_activities SET activity_type='proposal', activity_method=''
		WHERE TRIM(COALESCE(activity_method,''))='' AND activity_type='rfp'`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE sales_activities SET activity_type='other', activity_method=''
		WHERE TRIM(COALESCE(activity_method,''))='' AND activity_type='bid'`); err != nil {
		return err
	}
	return tx.Commit()
}

type SalesAct91Count struct {
	ActivityType string
	N            int
}

func CountSalesActivitiesByType(db *sql.DB) ([]SalesAct91Count, int, error) {
	if db == nil {
		return nil, 0, fmt.Errorf("db")
	}
	rows, err := db.Query(`SELECT COALESCE(activity_type,'(empty)'), COUNT(*) FROM sales_activities GROUP BY 1 ORDER BY COUNT(*) DESC, 1`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []SalesAct91Count
	total := 0
	for rows.Next() {
		var c SalesAct91Count
		if err := rows.Scan(&c.ActivityType, &c.N); err != nil {
			return nil, 0, err
		}
		total += c.N
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func salesActivitySplit91SQLFiles() (up, down string, err error) {
	candidates := []string{
		filepath.Join("migrations", "065_sales_activity_method.sql"),
		filepath.Join("..", "..", "migrations", "065_sales_activity_method.sql"),
	}
	for _, p := range candidates {
		b, e := os.ReadFile(p)
		if e == nil && len(b) > 0 {
			up = string(b)
			d, _ := os.ReadFile(strings.TrimSuffix(p, ".sql") + ".down.sql")
			return up, string(d), nil
		}
		err = e
	}
	return "", "", err
}

func (r *SalesRepo) ActivityMethods() ([]model.Code, error) {
	return r.codeRepo.ActiveByGroup(model.SalesCodeGroupActivityMethod)
}

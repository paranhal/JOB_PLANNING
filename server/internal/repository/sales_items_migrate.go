package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"customer-support/internal/model"
)

// applySalesItems 마이그레이션 035. §36.6 품목 마스터 · assets 초안.
func applySalesItems(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sales_items (
			item_id           TEXT PRIMARY KEY,
			item_kind         TEXT NOT NULL DEFAULT 'goods',
			category          TEXT NOT NULL DEFAULT '',
			name              TEXT NOT NULL,
			spec              TEXT NOT NULL DEFAULT '',
			model             TEXT NOT NULL DEFAULT '',
			manufacturer      TEXT NOT NULL DEFAULT '',
			unit              TEXT NOT NULL DEFAULT 'EA',
			gov_item_no       TEXT NOT NULL DEFAULT '',
			list_price        INTEGER NOT NULL DEFAULT 0,
			last_price        INTEGER NOT NULL DEFAULT 0,
			last_quoted_at    TEXT NOT NULL DEFAULT '',
			last_customer     TEXT NOT NULL DEFAULT '',
			default_supplier  TEXT NOT NULL DEFAULT '',
			is_active         INTEGER NOT NULL DEFAULT 1,
			needs_review      INTEGER NOT NULL DEFAULT 0,
			notes             TEXT NOT NULL DEFAULT '',
			created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return
		}
		log.Printf("035 sales_items: %v", err)
		return
	}
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS idx_sales_items_kind ON sales_items(item_kind)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_items_name ON sales_items(name)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_items_active ON sales_items(is_active, needs_review)`,
		`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
			('SIK01','sales_item_kind','goods','물품',1,1),
			('SIK02','sales_item_kind','service','AS·작업',2,1),
			('SIK03','sales_item_kind','labor','개발 용역',3,1),
			('SIK04','sales_item_kind','etc','기타',4,1),
			('SIC01','sales_item_category','RFID장비','RFID장비',1,1),
			('SIC02','sales_item_category','소모품','소모품',2,1),
			('SIC03','sales_item_category','SW','SW',3,1),
			('SIC04','sales_item_category','공사','공사',4,1),
			('SIU01','sales_item_unit','EA','EA',1,1),
			('SIU02','sales_item_unit','식','식',2,1),
			('SIU03','sales_item_unit','대','대',3,1),
			('SIU04','sales_item_unit','Box','Box',4,1),
			('SIU05','sales_item_unit','인','인',5,1),
			('SIU06','sales_item_unit','M/M','M/M',6,1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("035 sales_items seed: %v", err)
		}
	}
	if err := SeedSalesItemsFromAssets(db); err != nil {
		log.Printf("035 sales_items from assets: %v", err)
	}
	applySalesQuotes(db)
}

// SeedSalesItemsFromAssets assets 의 품명·모델·제조사를 distinct 로 모아 초안을 넣는다.
// 이미 같은 키가 있으면 건너뛴다. is_active=0, needs_review=1, notes=확인 필요.
func SeedSalesItemsFromAssets(db *sql.DB) error {
	return seedSalesItemsFromAssets(db)
}

func seedSalesItemsFromAssets(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT TRIM(product_name), TRIM(COALESCE(model_name,'')),
			TRIM(COALESCE(manufacturer,'')), TRIM(COALESCE(product_category,''))
		FROM assets
		WHERE TRIM(COALESCE(product_name,'')) != ''
		GROUP BY TRIM(product_name), TRIM(COALESCE(model_name,'')),
			TRIM(COALESCE(manufacturer,'')), TRIM(COALESCE(product_category,''))`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	defer rows.Close()
	repo := NewSalesItemRepo(db)
	for rows.Next() {
		var name, modelName, mfr, cat string
		if err := rows.Scan(&name, &modelName, &mfr, &cat); err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var exists int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM sales_items
			WHERE lower(name)=lower(?) AND lower(COALESCE(model,''))=lower(?) AND lower(COALESCE(manufacturer,''))=lower(?)`,
			name, modelName, mfr).Scan(&exists)
		if err != nil || exists > 0 {
			continue
		}
		kind, category := model.MapAssetProductToSalesItem(cat)
		it := &model.SalesItem{
			ItemKind:     kind,
			Category:     category,
			Name:         name,
			Spec:         modelName,
			Model:        modelName,
			Manufacturer: mfr,
			Unit:         "EA",
			IsActive:     false,
			NeedsReview:  true,
			Notes:        "확인 필요",
		}
		if err := repo.Create(it); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return rows.Err()
}

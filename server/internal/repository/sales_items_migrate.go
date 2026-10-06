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
			manufacturer_id   TEXT NOT NULL DEFAULT '',
			supplier_id       TEXT NOT NULL DEFAULT '',
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
	addNamedColumn(db, "sales_items", "manufacturer_id", `ALTER TABLE sales_items ADD COLUMN manufacturer_id TEXT NOT NULL DEFAULT ''`)
	addNamedColumn(db, "sales_items", "supplier_id", `ALTER TABLE sales_items ADD COLUMN supplier_id TEXT NOT NULL DEFAULT ''`)
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS idx_sales_items_kind ON sales_items(item_kind)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_items_name ON sales_items(name)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_items_active ON sales_items(is_active, needs_review)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_items_mfr ON sales_items(manufacturer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_items_sup ON sales_items(supplier_id)`,
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
	linkSalesItemParties(db)
	applySalesQuotes(db)
	applySalesItemSpecs(db)
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

func linkSalesItemParties(db *sql.DB) {
	if db == nil {
		return
	}
	mfr, err := db.Exec(`
		UPDATE sales_items SET manufacturer_id=(
			SELECT c.customer_id FROM customers c
			WHERE TRIM(c.org_name)!='' AND (
				lower(trim(c.org_name))=lower(trim(sales_items.manufacturer))
				OR lower(trim(c.official_name))=lower(trim(sales_items.manufacturer))
			)
			LIMIT 1
		)
		WHERE COALESCE(manufacturer_id,'')='' AND TRIM(COALESCE(manufacturer,''))!=''
		  AND EXISTS (
			SELECT 1 FROM customers c
			WHERE TRIM(c.org_name)!='' AND (
				lower(trim(c.org_name))=lower(trim(sales_items.manufacturer))
				OR lower(trim(c.official_name))=lower(trim(sales_items.manufacturer))
			)
		)`)
	if err != nil {
		log.Printf("sales_items link manufacturer: %v", err)
		return
	}
	sup, err := db.Exec(`
		UPDATE sales_items SET supplier_id=(
			SELECT c.customer_id FROM customers c
			WHERE TRIM(c.org_name)!='' AND (
				lower(trim(c.org_name))=lower(trim(sales_items.default_supplier))
				OR lower(trim(c.official_name))=lower(trim(sales_items.default_supplier))
			)
			LIMIT 1
		)
		WHERE COALESCE(supplier_id,'')='' AND TRIM(COALESCE(default_supplier,''))!=''
		  AND EXISTS (
			SELECT 1 FROM customers c
			WHERE TRIM(c.org_name)!='' AND (
				lower(trim(c.org_name))=lower(trim(sales_items.default_supplier))
				OR lower(trim(c.official_name))=lower(trim(sales_items.default_supplier))
			)
		)`)
	if err != nil {
		log.Printf("sales_items link supplier: %v", err)
		return
	}
	nm, ns := int64(0), int64(0)
	if mfr != nil {
		nm, _ = mfr.RowsAffected()
	}
	if sup != nil {
		ns, _ = sup.RowsAffected()
	}
	if nm+ns > 0 {
		log.Printf("sales_items party link: manufacturer=%d supplier=%d", nm, ns)
		logCreate(db, "sales_items", "item_id", "party-link",
			fmt.Sprintf("제조사 연결 %d건 · 공급사 연결 %d건", nm, ns))
	}
}

const itemSpecsMetaKey = "__meta:sales_item_specs_v1"

func applySalesItemSpecs(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sales_item_specs (
			spec_id    TEXT PRIMARY KEY,
			item_id    TEXT NOT NULL,
			spec       TEXT NOT NULL DEFAULT '',
			unit       TEXT NOT NULL DEFAULT 'EA',
			price      INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
			is_active  INTEGER NOT NULL DEFAULT 1
		)`); err != nil {
		log.Printf("sales_item_specs: %v", err)
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sales_item_supplier_prices (
			sp_id       TEXT PRIMARY KEY,
			item_id     TEXT NOT NULL,
			supplier_id TEXT NOT NULL,
			spec_id     TEXT NOT NULL DEFAULT '',
			unit        TEXT NOT NULL DEFAULT '',
			price       INTEGER NOT NULL DEFAULT 0,
			sort_order  INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
		log.Printf("sales_item_supplier_prices: %v", err)
		return
	}
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS idx_sales_item_specs_item ON sales_item_specs(item_id, sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_item_prices_item ON sales_item_supplier_prices(item_id, supplier_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sales_item_prices_spec ON sales_item_supplier_prices(spec_id)`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("sales_item_specs idx: %v", err)
		}
	}
	addNamedColumn(db, "sales_quote_lines", "spec_id", `ALTER TABLE sales_quote_lines ADD COLUMN spec_id TEXT NOT NULL DEFAULT ''`)
	if metaDone(db, itemSpecsMetaKey) {
		return
	}
	n, err := migrateSalesItemSpecsFromCatalog(db)
	if err != nil {
		log.Printf("sales_item_specs migrate: %v", err)
		return
	}
	log.Printf("sales_item_specs 이관: %d건", n)
	markMetaDone(db, itemSpecsMetaKey)
}

func migrateSalesItemSpecsFromCatalog(db *sql.DB) (int, error) {
	rows, err := db.Query(`
		SELECT item_id, COALESCE(spec,''), COALESCE(unit,'EA'), COALESCE(list_price,0)
		FROM sales_items
		WHERE TRIM(COALESCE(spec,'')) != '' OR COALESCE(list_price,0) != 0`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var itemID, spec, unit string
		var price int
		if err := rows.Scan(&itemID, &spec, &unit, &price); err != nil {
			return n, err
		}
		var exists int
		_ = db.QueryRow(`SELECT COUNT(*) FROM sales_item_specs WHERE item_id=?`, itemID).Scan(&exists)
		if exists > 0 {
			continue
		}
		seq, err := NextSeq(db, "sales_item_specs")
		if err != nil {
			return n, err
		}
		if strings.TrimSpace(unit) == "" {
			unit = "EA"
		}
		if _, err := db.Exec(`
			INSERT INTO sales_item_specs (spec_id, item_id, spec, unit, price, sort_order, is_active)
			VALUES (?,?,?,?,?,0,1)`,
			fmt.Sprintf("SS-%03d", seq), itemID, spec, unit, price); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"customer-support/internal/model"
)

func applyMaintenanceVisitActionV90(db *sql.DB) {
	if db == nil {
		return
	}
	addTextColumnDefault(db, "maintenance_visits", "start_time", "")
	addTextColumnDefault(db, "maintenance_visits", "end_time", "")
	addIntColumnDefault(db, "maintenance_visits", "travel_min", 0)
	addIntColumnDefault(db, "maintenance_visits", "duration_min", 0)
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS maintenance_visit_targets (
			target_id TEXT PRIMARY KEY,
			visit_id TEXT NOT NULL,
			asset_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			memo TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			org_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE INDEX IF NOT EXISTS idx_mnt_visit_targets_visit
			ON maintenance_visit_targets(visit_id, sort_order);`)
	if err != nil {
		log.Printf("090 visit action: %v", err)
	}
}

func ApplyMaintenanceVisitActionV90ForTest(db *sql.DB) { applyMaintenanceVisitActionV90(db) }

func addTextColumnDefault(db *sql.DB, table, col, def string) {
	if db == nil || tableHasColumn(db, table, col) {
		return
	}
	q := `ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT ''`
	if def != "" {
		q = `ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT '` + strings.ReplaceAll(def, "'", "''") + `'`
	}
	if _, err := db.Exec(q); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("090 add %s.%s: %v", table, col, err)
	}
}

func addIntColumnDefault(db *sql.DB, table, col string, def int) {
	if db == nil || tableHasColumn(db, table, col) {
		return
	}
	q := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s INTEGER NOT NULL DEFAULT %d`, table, col, def)
	if _, err := db.Exec(q); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("090 add %s.%s: %v", table, col, err)
	}
}

func (r *MaintenanceRepo) ListVisitTargets(visitID string) ([]model.MaintenanceVisitTarget, error) {
	visitID = strings.TrimSpace(visitID)
	if visitID == "" || r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT target_id, visit_id, COALESCE(asset_id,''), COALESCE(name,''), COALESCE(memo,''),
		       COALESCE(sort_order,0), COALESCE(org_id,'')
		FROM maintenance_visit_targets WHERE visit_id=? ORDER BY sort_order, target_id`, visitID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.MaintenanceVisitTarget
	for rows.Next() {
		var it model.MaintenanceVisitTarget
		if err := rows.Scan(&it.TargetID, &it.VisitID, &it.AssetID, &it.Name, &it.Memo, &it.SortOrder, &it.OrgID); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *MaintenanceRepo) ReplaceVisitTargets(visitID, orgID string, items []model.MaintenanceVisitTarget) error {
	visitID = strings.TrimSpace(visitID)
	if visitID == "" {
		return fmt.Errorf("방문 ID가 없습니다")
	}
	if len(items) > model.MaxVisitTargets {
		return fmt.Errorf("점검 장비는 최대 %d개까지입니다", model.MaxVisitTargets)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM maintenance_visit_targets WHERE visit_id=?`, visitID); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	orgID = strings.TrimSpace(orgID)
	for i, it := range items {
		name := strings.TrimSpace(it.Name)
		assetID := strings.TrimSpace(it.AssetID)
		if name == "" && assetID == "" {
			continue
		}
		id := strings.TrimSpace(it.TargetID)
		if id == "" {
			id = randomID("mvt_")
		}
		if _, err := tx.Exec(`
			INSERT INTO maintenance_visit_targets(target_id, visit_id, asset_id, name, memo, sort_order, org_id)
			VALUES (?,?,?,?,?,?,?)`,
			id, visitID, assetID, name, strings.TrimSpace(it.Memo), i, orgID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MaintenanceRepo) AssetBelongsToCustomer(assetID, customerID string) (name string, ok bool) {
	assetID = strings.TrimSpace(assetID)
	customerID = strings.TrimSpace(customerID)
	if assetID == "" || customerID == "" {
		return "", false
	}
	var n string
	err := r.db.QueryRow(`
		SELECT TRIM(COALESCE(NULLIF(TRIM(product_name),''), NULLIF(TRIM(model_name),''), asset_id))
		FROM assets WHERE asset_id=? AND customer_id=? AND operation_status!='disposed'`,
		assetID, customerID).Scan(&n)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(n), true
}

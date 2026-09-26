package repository

import (
	"database/sql"
	"sort"
	"strings"

	"customer-support/internal/model"
)

const classifyHintCodeID = "WCH001"

func applyWorkTaskAssets(db *sql.DB) {
	if db == nil {
		return
	}
	db.Exec(`
		CREATE TABLE IF NOT EXISTS work_task_assets (
			task_id  TEXT NOT NULL,
			asset_id TEXT NOT NULL,
			PRIMARY KEY (task_id, asset_id)
		)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_work_task_assets_asset ON work_task_assets(asset_id)`)
	addNamedColumn(db, "as_receipts", "moved_task_id", `ALTER TABLE as_receipts ADD COLUMN moved_task_id TEXT`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_receipts_moved_task ON as_receipts(moved_task_id)`)
	db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active)
		VALUES (?, ?, ?, ?, 1, 1)`,
		classifyHintCodeID, model.CodeGroupClassifyHint, model.CodeValueClassifyBanner, model.ClassifyHintDefault)
	db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active)
		VALUES ('AS_S007','as_status',?, '행정/지원 이관', 7, 1)`, model.StatusAdminWork)
}

func (r *WBRepo) ListCustomerAssets(customerID string) ([]model.Asset, error) {
	if strings.TrimSpace(customerID) == "" {
		return nil, nil
	}
	return NewAssetRepo(r.db).ListByCustomer(customerID)
}

func (r *WBRepo) ListTaskAssets(taskID string) ([]model.Asset, error) {
	if r == nil || strings.TrimSpace(taskID) == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT a.asset_id, a.customer_id, a.product_name, COALESCE(a.product_type,''),
		       COALESCE(a.model_name,''), COALESCE(a.install_location,'')
		FROM work_task_assets wta
		JOIN assets a ON a.asset_id = wta.asset_id
		WHERE wta.task_id=?
		ORDER BY a.product_name, a.asset_id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(&a.AssetID, &a.CustomerID, &a.ProductName, &a.ProductType, &a.ModelName, &a.InstallLocation); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *WBRepo) ReplaceTaskAssets(taskID string, assetIDs []string) error {
	if r == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM work_task_assets WHERE task_id=?`, taskID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range assetIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := tx.Exec(`INSERT INTO work_task_assets (task_id, asset_id) VALUES (?,?)`, taskID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *WBRepo) fillTaskAssets(t *model.WorkTask) {
	if t == nil {
		return
	}
	items, err := r.ListTaskAssets(t.TaskID)
	if err == nil {
		t.LinkedAssets = items
	}
}

func (r *WBRepo) ListAssetSupportHistory(assetID string, limit int) ([]model.AssetSupportEvent, error) {
	if strings.TrimSpace(assetID) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 80
	}
	var out []model.AssetSupportEvent
	asRows, err := r.db.Query(`
		SELECT ar.as_id, ar.as_number, COALESCE(ar.receipt_datetime,''), COALESCE(ar.status,''),
		       COALESCE(ar.symptom,'')
		FROM as_receipts ar
		WHERE ar.asset_id=?`, assetID)
	if err != nil {
		return nil, err
	}
	defer asRows.Close()
	for asRows.Next() {
		var ev model.AssetSupportEvent
		var at string
		if err := asRows.Scan(&ev.ID, &ev.Number, &at, &ev.Status, &ev.Title); err != nil {
			return nil, err
		}
		ev.Kind = "as"
		ev.Href = "/as/" + ev.ID
		ev.SortAt = at
		if t := parseTime(at); !t.IsZero() {
			ev.Date = t.Format("2006-01-02")
		}
		ev.StatusLabel = statsStatusLabel(ev.Status)
		out = append(out, ev)
	}
	if err := asRows.Err(); err != nil {
		return nil, err
	}

	taskRows, err := r.db.Query(`
		SELECT t.task_id, t.title, COALESCE(t.status,''),
		       COALESCE(NULLIF(TRIM(t.receipt_date),''), date(t.created_at), t.created_at)
		FROM work_task_assets wta
		JOIN work_tasks t ON t.task_id = wta.task_id
		WHERE wta.asset_id=?`, assetID)
	if err != nil {
		return nil, err
	}
	defer taskRows.Close()
	for taskRows.Next() {
		var ev model.AssetSupportEvent
		var at string
		if err := taskRows.Scan(&ev.ID, &ev.Title, &ev.Status, &at); err != nil {
			return nil, err
		}
		ev.Kind = "admin"
		ev.Number = ev.ID
		ev.Href = "/workboard/tasks/" + ev.ID
		ev.SortAt = at
		if t := parseTime(at); !t.IsZero() {
			ev.Date = t.Format("2006-01-02")
		} else if len(at) >= 10 {
			ev.Date = at[:10]
		}
		ev.StatusLabel = ev.Status
		out = append(out, ev)
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].SortAt == out[j].SortAt {
			return out[i].Number > out[j].Number
		}
		return out[i].SortAt > out[j].SortAt
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *ASRepo) GetByMovedTaskID(taskID string) (*model.ASReceipt, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, nil
	}
	var asID string
	err := r.db.QueryRow(`SELECT as_id FROM as_receipts WHERE moved_task_id=? LIMIT 1`, taskID).Scan(&asID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.GetByID(asID)
}

func (r *StatsRepo) CountASAdminMoved(from, toEx string, f model.StatsMeetingFilter) (int, error) {
	asSQL, extra := r.filterAS(f)
	q := `
		SELECT COUNT(*)
		FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status = '` + model.StatusAdminWork + `'
		  AND TRIM(COALESCE(ar.moved_task_id,'')) != ''
		  AND COALESCE(ar.complete_datetime, ar.updated_at, ar.receipt_datetime) >= ?
		  AND COALESCE(ar.complete_datetime, ar.updated_at, ar.receipt_datetime) < ?` + asSQL
	args := append([]interface{}{from, toEx}, extra...)
	var n int
	err := r.db.QueryRow(q, args...).Scan(&n)
	return n, err
}

func sqlExcludeASAdminMoved() string {
	return ` AND TRIM(COALESCE(ar.moved_task_id,'')) = '' AND ar.status != '` + model.StatusAdminWork + `'`
}

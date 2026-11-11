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

// ensureASTaskAsset 접수 자산 1건을 업무에 넣는다. 비어 있으면 아무 것도 하지 않는다.
// 이미 있는 다른 자산 연결은 지우지 않는다.
func (r *WBRepo) ensureASTaskAsset(taskID, assetID string) error {
	taskID = strings.TrimSpace(taskID)
	assetID = strings.TrimSpace(assetID)
	if r == nil || r.db == nil || taskID == "" || assetID == "" {
		return nil
	}
	_, err := r.db.Exec(`INSERT OR IGNORE INTO work_task_assets (task_id, asset_id) VALUES (?,?)`, taskID, assetID)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}

// MissingASTaskAsset 접수 자산이 있는데 업무 연결이 없는 건.
type MissingASTaskAsset struct {
	TaskID  string
	ASID    string
	AssetID string
}

// ListMissingASTaskAssets 이미 만들어진 AS 업무 중 접수 자산 연결이 빠진 건. 고치지 않는다.
func (r *WBRepo) ListMissingASTaskAssets() ([]MissingASTaskAsset, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT t.task_id, ar.as_id, ar.asset_id
		  FROM work_tasks t
		  JOIN as_receipts ar ON ar.as_id = t.source_id
		 WHERE t.source_type = ?
		   AND TRIM(COALESCE(t.source_role,'')) NOT LIKE 'asset:%'
		   AND TRIM(COALESCE(ar.asset_id,'')) != ''
		   AND NOT EXISTS (
		     SELECT 1 FROM work_task_assets wta
		      WHERE wta.task_id = t.task_id AND wta.asset_id = ar.asset_id
		   )
		 ORDER BY t.task_id`, model.WBSourceAS)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []MissingASTaskAsset
	for rows.Next() {
		var row MissingASTaskAsset
		if err := rows.Scan(&row.TaskID, &row.ASID, &row.AssetID); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ASReceiptWithoutTask 자산은 있는데 원 업무가 없는 접수. 도구는 목록만 내고 만들지 않는다.
type ASReceiptWithoutTask struct {
	ASID        string
	Number      string
	Status      string
	Assignee    string
	HasWorkItem bool
	ReceiptAt   string
}

// ASAssetBackfillReport 기존 AS 업무에 접수 자산을 잇는 결과.
type ASAssetBackfillReport struct {
	Missing       int
	Linked        int
	AlreadyLinked int
	NoTask        []ASReceiptWithoutTask
	DryRun        bool
}

// BackfillASTaskAssets dryRun이면 쓰지 않는다. 쓰기는 한 트랜잭션이고, 같은 연결이 있으면 건너뛴다.
// 분리 업무(source_role=asset:*)와 자산 없는 접수는 대상이 아니다. 업무가 없는 접수는 만들지 않는다.
func (r *WBRepo) BackfillASTaskAssets(dryRun bool) (*ASAssetBackfillReport, error) {
	rep := &ASAssetBackfillReport{DryRun: dryRun}
	if r == nil || r.db == nil {
		return rep, nil
	}
	rows, err := r.ListMissingASTaskAssets()
	if err != nil {
		return nil, err
	}
	rep.Missing = len(rows)
	already, err := r.countLinkedASTaskAssets()
	if err != nil {
		return nil, err
	}
	rep.AlreadyLinked = already
	noTask, err := r.listASReceiptsWithoutPrimaryTask()
	if err != nil {
		return nil, err
	}
	rep.NoTask = noTask
	if dryRun || len(rows) == 0 {
		return rep, nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	linked := 0
	for _, row := range rows {
		res, err := tx.Exec(`INSERT OR IGNORE INTO work_task_assets (task_id, asset_id) VALUES (?,?)`, row.TaskID, row.AssetID)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		linked += int(n)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	rep.Linked = linked
	return rep, nil
}

func (r *WBRepo) countLinkedASTaskAssets() (int, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		  FROM work_tasks t
		  JOIN as_receipts ar ON ar.as_id = t.source_id
		 WHERE t.source_type = ?
		   AND TRIM(COALESCE(t.source_role,'')) NOT LIKE 'asset:%'
		   AND TRIM(COALESCE(ar.asset_id,'')) != ''
		   AND EXISTS (
		     SELECT 1 FROM work_task_assets wta
		      WHERE wta.task_id = t.task_id AND wta.asset_id = ar.asset_id
		   )`, model.WBSourceAS).Scan(&n)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return 0, nil
	}
	return n, err
}

func (r *WBRepo) listASReceiptsWithoutPrimaryTask() ([]ASReceiptWithoutTask, error) {
	rows, err := r.db.Query(`
		SELECT ar.as_id, COALESCE(ar.as_number,''), COALESCE(ar.status,''), COALESCE(ar.assigned_to,''),
		       COALESCE(ar.receipt_datetime,''),
		       CASE WHEN EXISTS (SELECT 1 FROM as_work_items w WHERE w.as_id = ar.as_id) THEN 1 ELSE 0 END
		  FROM as_receipts ar
		 WHERE TRIM(COALESCE(ar.asset_id,'')) != ''
		   AND NOT EXISTS (
		     SELECT 1 FROM work_tasks t
		      WHERE t.source_type = ? AND t.source_id = ar.as_id
		        AND TRIM(COALESCE(t.source_role,'')) NOT LIKE 'asset:%'
		   )
		 ORDER BY ar.receipt_datetime, ar.as_id`, model.WBSourceAS)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []ASReceiptWithoutTask
	for rows.Next() {
		var row ASReceiptWithoutTask
		var has int
		if err := rows.Scan(&row.ASID, &row.Number, &row.Status, &row.Assignee, &row.ReceiptAt, &has); err != nil {
			return nil, err
		}
		row.HasWorkItem = has == 1
		out = append(out, row)
	}
	return out, rows.Err()
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
		ev.Href = model.ASOpenHref(ev.ID)
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
	return r.GetByID(OrgAll, asID)
}

func (r *StatsRepo) CountASAdminMoved(from, toEx string, f model.StatsMeetingFilter) (int, error) {
	asSQL, extra := r.filterAS(f)
	q := `
		SELECT COUNT(*)
		FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status = '` + model.StatusAdminWork + `'
		  AND TRIM(COALESCE(ar.moved_task_id,'')) != ''
		  AND TRIM(COALESCE(ar.complete_datetime,'')) != ''
		  AND ar.complete_datetime >= ?
		  AND ar.complete_datetime < ?` + asSQL
	args := append([]interface{}{from, toEx}, extra...)
	var n int
	err := r.db.QueryRow(q, args...).Scan(&n)
	return n, err
}

func sqlExcludeASAdminMoved() string {
	return ` AND TRIM(COALESCE(ar.moved_task_id,'')) = '' AND ar.status != '` + model.StatusAdminWork + `'`
}

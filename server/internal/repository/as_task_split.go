package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// asSplitBeforeInsert 는 AS 분리 트랜잭션에서 새 업무를 넣기 직전에 호출된다.
var asSplitBeforeInsert func(i int) error

// ASSplitTarget 접수 하나에 묶을 자산. Title 이 있으면 그 작업명을 쓴다.
type ASSplitTarget struct {
	AssetID string
	Name    string
	Title   string
}

// SplitASReceipt 접수 업무를 자산별로 나눈다.
// 원 업무는 source_role 을 비워 두고 첫 자산을 맡는다.
// 나머지는 source_type=as, source_id=접수번호, source_role=asset:<id> 다.
// 같은 역할이 있으면 건너뛴다. 삽입과 연결은 한 트랜잭션이다.
func (r *WBRepo) SplitASReceipt(asID, baseTitle string, assets []ASSplitTarget) ([]*model.WorkTask, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("업무가 없습니다")
	}
	asID = strings.TrimSpace(asID)
	assets = uniqueASSplitTargets(assets)
	if asID == "" || len(assets) < 2 {
		return nil, fmt.Errorf("나눌 자산이 2개 미만입니다")
	}
	var status string
	if err := r.db.QueryRow(`SELECT COALESCE(status,'') FROM as_receipts WHERE as_id=?`, asID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("접수를 찾을 수 없습니다")
		}
		return nil, err
	}
	switch status {
	case "completed", "closed", "cancelled":
		return nil, fmt.Errorf("완료된 접수는 나누지 않습니다")
	}
	primary, err := r.GetTaskBySource(model.WBSourceAS, asID)
	if err != nil {
		return nil, err
	}
	if primary == nil {
		return nil, fmt.Errorf("나눌 업무가 없습니다")
	}
	var children int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE parent_task_id=?`, primary.TaskID).Scan(&children); err != nil {
		return nil, err
	}
	if children > 0 {
		return nil, fmt.Errorf("하위 업무가 있어 나눌 수 없습니다")
	}
	baseTitle = strings.TrimSpace(baseTitle)
	if baseTitle == "" {
		baseTitle = strings.TrimSpace(primary.Title)
	}
	names := map[string]string{}
	for _, a := range assets {
		if a.Name == "" {
			a.Name = r.assetLabel(a.AssetID)
		}
		if a.Name == "" {
			a.Name = a.AssetID
		}
		names[a.AssetID] = a.Name
	}
	for i := range assets {
		if assets[i].Name == "" {
			assets[i].Name = names[assets[i].AssetID]
		}
	}
	copies := make([]*model.WorkTask, len(assets))
	primary.Title = asSplitTitle(baseTitle, assets[0])
	copies[0] = primary
	for i := 1; i < len(assets); i++ {
		role := "asset:" + assets[i].AssetID
		if id, err := r.findASSplitCopy(asID, role); err != nil {
			return nil, err
		} else if id != "" {
			got, err := r.GetTask(id)
			if err != nil {
				return nil, err
			}
			copies[i] = got
			continue
		}
		cp := *primary
		cp.TaskID = ""
		cp.Title = asSplitTitle(baseTitle, assets[i])
		cp.SourceType = model.WBSourceAS
		cp.SourceID = asID
		cp.SourceRole = role
		cp.ParentTaskID = ""
		cp.Status = model.WBTaskWaiting
		cp.Progress = 0
		cp.RecurrenceRole = ""
		cp.OccurrenceSeq = 0
		cp.OccurrenceStatus = ""
		cp.CompleteDate = ""
		if err := r.prepareSplitTask(&cp); err != nil {
			return nil, err
		}
		copies[i] = &cp
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE work_tasks SET title=? WHERE task_id=?`, copies[0].Title, copies[0].TaskID); err != nil {
		return nil, err
	}
	if err := replaceOneTaskAssetTx(tx, copies[0].TaskID, assets[0].AssetID); err != nil {
		return nil, err
	}
	for i := 1; i < len(assets); i++ {
		task := copies[i]
		if task == nil {
			return nil, fmt.Errorf("업무를 만들지 못했습니다")
		}
		role := "asset:" + assets[i].AssetID
		if task.SourceRole == role && strings.TrimSpace(task.TaskID) != "" {
			var exists string
			err := tx.QueryRow(`SELECT task_id FROM work_tasks WHERE source_type=? AND source_id=? AND source_role=?`,
				model.WBSourceAS, asID, role).Scan(&exists)
			if err == nil && exists != "" {
				continue
			}
			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}
		}
		if asSplitBeforeInsert != nil {
			if err := asSplitBeforeInsert(i); err != nil {
				return nil, err
			}
		}
		if err := execWorkTaskInsert(tx, task); err != nil {
			return nil, err
		}
		if err := replaceOneTaskAssetTx(tx, task.TaskID, assets[i].AssetID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return copies, nil
}

func (r *WBRepo) findASSplitCopy(asID, role string) (string, error) {
	var id string
	err := r.db.QueryRow(`SELECT task_id FROM work_tasks WHERE source_type=? AND source_id=? AND source_role=?`,
		model.WBSourceAS, asID, role).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (r *WBRepo) assetLabel(id string) string {
	var name string
	_ = r.db.QueryRow(`
		SELECT TRIM(COALESCE(NULLIF(TRIM(product_name),''), NULLIF(TRIM(model_name),''), asset_id))
		FROM assets WHERE asset_id=?`, id).Scan(&name)
	return strings.TrimSpace(name)
}

func asSplitTitle(base string, a ASSplitTarget) string {
	if v := strings.TrimSpace(a.Title); v != "" {
		return v
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = strings.TrimSpace(a.AssetID)
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return name
	}
	return base + " - " + name
}

func uniqueASSplitTargets(in []ASSplitTarget) []ASSplitTarget {
	seen := map[string]bool{}
	var out []ASSplitTarget
	for _, a := range in {
		a.AssetID = strings.TrimSpace(a.AssetID)
		a.Name = strings.TrimSpace(a.Name)
		a.Title = strings.TrimSpace(a.Title)
		if a.AssetID == "" || seen[a.AssetID] {
			continue
		}
		seen[a.AssetID] = true
		out = append(out, a)
	}
	return out
}

func replaceOneTaskAssetTx(tx *sql.Tx, taskID, assetID string) error {
	if _, err := tx.Exec(`DELETE FROM work_task_assets WHERE task_id=?`, taskID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil
		}
		return err
	}
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return nil
	}
	_, err := tx.Exec(`INSERT INTO work_task_assets (task_id, asset_id) VALUES (?,?)`, taskID, assetID)
	return err
}

// HasASAssetSplit 이 접수에 asset: 역할의 분리 업무가 있는지.
func (r *WBRepo) HasASAssetSplit(asID string) (bool, error) {
	if r == nil || r.db == nil || strings.TrimSpace(asID) == "" {
		return false, nil
	}
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks
		 WHERE source_type=? AND source_id=? AND source_role LIKE 'asset:%'`,
		model.WBSourceAS, strings.TrimSpace(asID)).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CountASSplitTasks 분리된 접수의 업무 수(원 업무 포함). 분리가 없으면 0.
func (r *WBRepo) CountASSplitTasks(asIDs []string) (map[string]int, error) {
	out := map[string]int{}
	ids := uniqueNonEmpty(asIDs)
	if r == nil || r.db == nil || len(ids) == 0 {
		return out, nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, model.WBSourceAS)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.Query(`
		SELECT source_id, COUNT(*)
		  FROM work_tasks
		 WHERE source_type=? AND source_id IN (`+ph+`)
		   AND source_id IN (
		     SELECT source_id FROM work_tasks
		      WHERE source_type=? AND source_role LIKE 'asset:%'
		   )
		 GROUP BY source_id`, append(args, model.WBSourceAS)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ListASLinkedAssets 원 업무와 분리 업무에 연결된 자산을 접수 순서로 모은다.
func (r *WBRepo) ListASLinkedAssets(asID string) ([]model.Asset, error) {
	if r == nil || r.db == nil || strings.TrimSpace(asID) == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT a.asset_id, COALESCE(a.product_name,''), COALESCE(a.model_name,''), COALESCE(a.customer_id,'')
		  FROM work_tasks t
		  JOIN work_task_assets wta ON wta.task_id = t.task_id
		  JOIN assets a ON a.asset_id = wta.asset_id
		 WHERE t.source_type=? AND t.source_id=?
		 ORDER BY CASE WHEN TRIM(COALESCE(t.source_role,''))='' THEN 0 ELSE 1 END, t.source_role, wta.asset_id`,
		model.WBSourceAS, strings.TrimSpace(asID))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.Asset
	seen := map[string]bool{}
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(&a.AssetID, &a.ProductName, &a.ModelName, &a.CustomerID); err != nil {
			return nil, err
		}
		if seen[a.AssetID] {
			continue
		}
		seen[a.AssetID] = true
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkPrimaryASTaskComplete 분리된 접수의 원 업무(role=”)만 완료로 둔다.
func (r *WBRepo) MarkPrimaryASTaskComplete(asID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.Exec(`
		UPDATE work_tasks
		   SET status=?, progress=100
		 WHERE source_type=? AND source_id=?
		   AND TRIM(COALESCE(source_role,'')) NOT LIKE 'asset:%'`,
		model.WBTaskComplete, model.WBSourceAS, strings.TrimSpace(asID))
	return err
}

// SyncASReceiptStatusFromSplits 분리 업무가 모두 끝났을 때만 접수를 완료로 둔다.
// 일부만 끝났으면 진행 중으로 되돌린다. 분리가 없으면 아무 것도 하지 않는다.
func (r *WBRepo) SyncASReceiptStatusFromSplits(asID string) error {
	asID = strings.TrimSpace(asID)
	if r == nil || r.db == nil || asID == "" {
		return nil
	}
	ok, err := r.HasASAssetSplit(asID)
	if err != nil || !ok {
		return err
	}
	var open int
	if err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks
		 WHERE source_type=? AND source_id=?
		   AND COALESCE(status,'') NOT IN ('complete','cancelled')`,
		model.WBSourceAS, asID).Scan(&open); err != nil {
		return err
	}
	if open == 0 {
		_, err = r.db.Exec(`
			UPDATE as_receipts
			   SET status='completed',
			       complete_datetime=COALESCE(complete_datetime, datetime('now','localtime'))
			 WHERE as_id=? AND status NOT IN ('completed','closed','cancelled')`, asID)
		return err
	}
	_, err = r.db.Exec(`
		UPDATE as_receipts
		   SET status='in_progress', complete_datetime=NULL
		 WHERE as_id=? AND status IN ('completed','closed')`, asID)
	return err
}

// deleteASDailyTasksExceptSplits 동기화가 지우는 대상에서 분리 업무를 뺀다.
func (r *WBRepo) deleteASDailyTasksExceptSplits(asID string) error {
	rows, err := r.db.Query(`
		SELECT task_id FROM work_tasks
		 WHERE source_type=? AND source_id=?
		   AND TRIM(COALESCE(source_role,'')) NOT LIKE 'asset:%'`,
		model.WBSourceAS, strings.TrimSpace(asID))
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.DeleteTask(id); err != nil {
			return err
		}
	}
	return nil
}

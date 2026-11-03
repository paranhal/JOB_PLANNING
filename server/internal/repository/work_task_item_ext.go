package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func (r *WBRepo) listItemAssignees(itemIDs []string) (map[string][]model.WorkTaskItemAssignee, error) {
	out := map[string][]model.WorkTaskItemAssignee{}
	if r == nil || r.db == nil || len(itemIDs) == 0 {
		return out, nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(itemIDs)), ",")
	args := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		args[i] = id
	}
	rows, err := r.db.Query(`SELECT item_id, COALESCE(assignee_name,''), COALESCE(assignee_user_id,'')
		FROM work_task_item_assignees WHERE item_id IN (`+ph+`) ORDER BY assignee_name`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, uid string
		if err := rows.Scan(&id, &name, &uid); err != nil {
			return nil, err
		}
		out[id] = append(out[id], model.WorkTaskItemAssignee{Name: name, UserID: uid})
	}
	return out, rows.Err()
}

func (r *WBRepo) replaceItemAssignees(itemID string, list []model.WorkTaskItemAssignee) error {
	if r == nil || r.db == nil || strings.TrimSpace(itemID) == "" {
		return nil
	}
	if _, err := r.db.Exec(`DELETE FROM work_task_item_assignees WHERE item_id=?`, itemID); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	seen := map[string]bool{}
	for _, a := range list {
		name := strings.TrimSpace(a.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if _, err := r.db.Exec(`INSERT INTO work_task_item_assignees (item_id, assignee_name, assignee_user_id) VALUES (?,?,?)`,
			itemID, name, strings.TrimSpace(a.UserID)); err != nil {
			return err
		}
	}
	return nil
}

func (r *WBRepo) listItemAssets(itemIDs []string, items []model.WorkTaskItem) (map[string][]string, map[string]string, map[string]bool, error) {
	assets := map[string][]string{}
	roles := map[string]string{}
	missing := map[string]bool{}
	kindBy := map[string]string{}
	qtyBy := map[string]int{}
	for _, it := range items {
		kindBy[it.ItemID] = it.Kind
		qtyBy[it.ItemID] = it.Qty
		if model.NormalizeItemKind(it.Kind) == model.ItemKindInstall {
			missing[it.ItemID] = true
		}
	}
	if r == nil || r.db == nil || len(itemIDs) == 0 {
		return assets, roles, missing, nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(itemIDs)), ",")
	args := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		args[i] = id
	}
	rows, err := r.db.Query(`SELECT item_id, asset_id, COALESCE(role,'ref') FROM work_task_item_assets WHERE item_id IN (`+ph+`)`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return assets, roles, missing, nil
		}
		return assets, roles, missing, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, asset, role string
		if err := rows.Scan(&id, &asset, &role); err != nil {
			return assets, roles, missing, err
		}
		assets[id] = append(assets[id], asset)
		roles[id] = role
		if len(assets[id]) > 0 {
			missing[id] = false
		}
	}
	for id, list := range assets {
		if model.NormalizeItemKind(kindBy[id]) == model.ItemKindInstall && len(list) == 0 {
			missing[id] = true
		}
	}
	return assets, roles, missing, rows.Err()
}

func stampWorkListItemKinds(r *WorkBoardRepo, items []model.WorkListItem) {
	if r == nil || len(items) == 0 {
		return
	}
	var ids []string
	for _, it := range items {
		if strings.HasPrefix(strings.TrimSpace(it.RefID), "WT") || strings.Contains(it.Href, "/workboard/tasks/") {
			ids = append(ids, it.RefID)
		}
	}
	sum, err := r.ItemKindSummaries(ids)
	if err != nil {
		return
	}
	for i := range items {
		if s := sum[items[i].RefID]; s.Line != "" {
			items[i].ItemKindLine = s.Line
			items[i].ItemProgress = s.Progress
		}
	}
}

func (r *WorkBoardRepo) ItemKindSummaries(taskIDs []string) (map[string]model.ItemKindCounts, error) {
	if r == nil {
		return map[string]model.ItemKindCounts{}, nil
	}
	return itemKindSummaries(r.db, taskIDs)
}

func (r *WorkBoardRepo) ItemKindSummary(taskIDs []string) (map[string]model.ItemKindCounts, error) {
	return r.ItemKindSummaries(taskIDs)
}

func (r *WBRepo) ItemKindSummaries(taskIDs []string) (map[string]model.ItemKindCounts, error) {
	if r == nil {
		return map[string]model.ItemKindCounts{}, nil
	}
	return itemKindSummaries(r.db, taskIDs)
}

func (r *WBRepo) ItemKindSummary(taskIDs []string) (map[string]model.ItemKindCounts, error) {
	return r.ItemKindSummaries(taskIDs)
}

func itemKindSummaries(db *sql.DB, taskIDs []string) (map[string]model.ItemKindCounts, error) {
	out := map[string]model.ItemKindCounts{}
	if db == nil || len(taskIDs) == 0 {
		return out, nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range taskIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}
	byTask := map[string][]model.WorkTaskItem{}
	const chunk = 400
	for i := 0; i < len(ids); i += chunk {
		end := i + chunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[i:end]
		ph := strings.TrimRight(strings.Repeat("?,", len(part)), ",")
		args := make([]interface{}, len(part))
		for j, id := range part {
			args[j] = id
		}
		rows, err := db.Query(`SELECT i.task_id, COALESCE(i.kind,'etc'), COALESCE(i.status,'todo'), COALESCE(i.qty,1),
			COALESCE(i.as_id,''), COALESCE(ar.status,''), COALESCE(i.unit,'')
			FROM work_task_items i
			LEFT JOIN as_receipts ar ON ar.as_id=i.as_id AND TRIM(i.as_id)!=''
			WHERE i.task_id IN (`+ph+`)`, args...)
		if err != nil {
			if strings.Contains(err.Error(), "no such table") || strings.Contains(strings.ToLower(err.Error()), "no such column") {
				return out, nil
			}
			return nil, err
		}
		for rows.Next() {
			var it model.WorkTaskItem
			var asSt string
			if err := rows.Scan(&it.TaskID, &it.Kind, &it.Status, &it.Qty, &it.ASID, &asSt, &it.Unit); err != nil {
				rows.Close()
				return nil, err
			}
			if strings.TrimSpace(it.ASID) != "" && asSt != "" {
				it.Status = model.ItemStatusFromAS(asSt)
			}
			byTask[it.TaskID] = append(byTask[it.TaskID], it)
		}
		rows.Close()
	}
	for id, items := range byTask {
		out[id] = model.SummarizeItemKinds(items)
	}
	return out, nil
}

func (r *WBRepo) ItemAssigneeUnion(taskIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if r == nil || r.db == nil || len(taskIDs) == 0 {
		return out, nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(taskIDs)), ",")
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		args[i] = id
	}
	rows, err := r.db.Query(`SELECT i.task_id, a.assignee_name
		FROM work_task_item_assignees a
		JOIN work_task_items i ON i.item_id=a.item_id
		WHERE i.task_id IN (`+ph+`) AND TRIM(a.assignee_name)!=''
		ORDER BY a.assignee_name`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	seen := map[string]map[string]bool{}
	for rows.Next() {
		var tid, name string
		if err := rows.Scan(&tid, &name); err != nil {
			return nil, err
		}
		if seen[tid] == nil {
			seen[tid] = map[string]bool{}
		}
		if seen[tid][name] {
			continue
		}
		seen[tid][name] = true
		out[tid] = append(out[tid], name)
	}
	return out, rows.Err()
}

func (r *WBRepo) ASLinkedToWorkItem(asID string) bool {
	if r == nil || r.db == nil || strings.TrimSpace(asID) == "" {
		return false
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM work_task_items WHERE TRIM(as_id)=?`, strings.TrimSpace(asID)).Scan(&n)
	return err == nil && n > 0
}

func (r *WBRepo) SetItemAS(itemID, asID string) error {
	if r == nil || strings.TrimSpace(itemID) == "" {
		return fmt.Errorf("항목이 없습니다")
	}
	_, err := r.db.Exec(`UPDATE work_task_items SET as_id=?, kind='as', updated_at=CURRENT_TIMESTAMP WHERE item_id=?`,
		strings.TrimSpace(asID), itemID)
	return err
}

func (r *WBRepo) ReplaceItemAssets(itemID, role string, assetIDs []string) error {
	if r == nil || strings.TrimSpace(itemID) == "" {
		return fmt.Errorf("항목이 없습니다")
	}
	role = model.NormalizeItemAssetRole("", role)
	if _, err := r.db.Exec(`DELETE FROM work_task_item_assets WHERE item_id=?`, itemID); err != nil &&
		!strings.Contains(err.Error(), "no such table") {
		return err
	}
	seen := map[string]bool{}
	for _, id := range assetIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := r.db.Exec(`INSERT INTO work_task_item_assets (item_id, asset_id, role) VALUES (?,?,?)`, itemID, id, role); err != nil {
			return err
		}
	}
	return nil
}

func (r *WBRepo) ListWorkHistoryForAsset(assetID string) ([]AssetWorkHistory, error) {
	var out []AssetWorkHistory
	if r == nil || strings.TrimSpace(assetID) == "" {
		return out, nil
	}
	rows, err := r.db.Query(`
		SELECT ia.role, i.title, i.kind, i.status, i.as_id, COALESCE(ar.as_number,''),
		       t.task_id, t.title, COALESCE(t.work_date,''), COALESCE(t.due_date,''), COALESCE(t.assignee,''),
		       COALESCE(p.name,''), COALESCE(p.short_name,'')
		FROM work_task_item_assets ia
		JOIN work_task_items i ON i.item_id=ia.item_id
		JOIN work_tasks t ON t.task_id=i.task_id
		LEFT JOIN as_receipts ar ON ar.as_id=i.as_id
		LEFT JOIN work_projects p ON p.project_id=t.project_id
		WHERE ia.asset_id=?
		ORDER BY COALESCE(NULLIF(TRIM(t.work_date),''), t.due_date, ia.created_at) DESC`, assetID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	seenAS := map[string]bool{}
	for rows.Next() {
		var h AssetWorkHistory
		var short string
		if err := rows.Scan(&h.Role, &h.ItemTitle, &h.Kind, &h.Status, &h.ASID, &h.ASNumber,
			&h.TaskID, &h.TaskTitle, &h.WorkDate, &h.DueDate, &h.Assignee, &h.ProjectName, &short); err != nil {
			return nil, err
		}
		if short != "" {
			h.ProjectName = short
		}
		if h.ASID != "" {
			if seenAS[h.ASID] {
				continue
			}
			seenAS[h.ASID] = true
		}
		h.KindLabel = model.ItemKindLabel(h.Kind)
		h.StatusLabel = model.ItemStatusLabel(h.Status)
		h.Href = "/workboard/tasks/" + h.TaskID
		if h.ASID != "" {
			h.ASHref = "/as/" + h.ASID
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

type AssetWorkHistory struct {
	Role        string
	ItemTitle   string
	Kind        string
	KindLabel   string
	Status      string
	StatusLabel string
	ASID        string
	ASNumber    string
	ASHref      string
	TaskID      string
	TaskTitle   string
	WorkDate    string
	DueDate     string
	Assignee    string
	ProjectName string
	Href        string
}

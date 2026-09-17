package repository

import (
	"database/sql"
	"log"
	"strings"
	"time"

	"customer-support/internal/model"
)

func applyWorkTaskBlocked(db *sql.DB) {
	if db == nil {
		return
	}
	for _, q := range []string{
		`ALTER TABLE work_tasks ADD COLUMN blocked_reason TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE work_tasks ADD COLUMN blocked_at TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(q); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			log.Printf("048 work_task_blocked: %v", err)
		}
	}
}

// SetTaskBlocked 막힌 이유를 남기거나 지운다. 조회 화면에서 부르지 않는다. §42.7
func (r *WBRepo) SetTaskBlocked(taskID, reason string) error {
	if r == nil || r.db == nil {
		return nil
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		_, err := r.db.Exec(`
			UPDATE work_tasks
			   SET blocked_reason='', blocked_at='', updated_at=CURRENT_TIMESTAMP
			 WHERE task_id=?`, taskID)
		return err
	}
	at := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		UPDATE work_tasks
		   SET blocked_reason=?, blocked_at=?, updated_at=CURRENT_TIMESTAMP
		 WHERE task_id=?`, reason, at, taskID)
	return err
}

func (r *WorkBoardRepo) fillBlockedReasons(items []model.WorkListItem) {
	if r == nil || r.db == nil || len(items) == 0 {
		return
	}
	ids := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		id := strings.TrimSpace(it.RefID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return
	}
	byTask := map[string]model.WorkListItem{}
	bySource := map[string]model.WorkListItem{}
	const chunk = 200
	for i := 0; i < len(ids); i += chunk {
		end := i + chunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[i:end]
		ph := strings.Repeat("?,", len(part))
		ph = ph[:len(ph)-1]
		q := `
		SELECT task_id, COALESCE(source_type,''), COALESCE(source_id,''),
		       COALESCE(blocked_reason,''), COALESCE(blocked_at,'')
		  FROM work_tasks
		 WHERE TRIM(COALESCE(blocked_reason,'')) != ''
		   AND (task_id IN (` + ph + `) OR source_id IN (` + ph + `))`
		args := make([]interface{}, 0, len(part)*2)
		for _, id := range part {
			args = append(args, id)
		}
		for _, id := range part {
			args = append(args, id)
		}
		rows, err := queryTimed(r.db, "work.fillBlockedReasons", q, args...)
		if err != nil {
			if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
				return
			}
			log.Printf("fillBlockedReasons: %v", err)
			return
		}
		for rows.Next() {
			var taskID, sourceType, sourceID, reason, at string
			if err := rows.Scan(&taskID, &sourceType, &sourceID, &reason, &at); err != nil {
				rows.Close()
				return
			}
			row := model.WorkListItem{BlockedReason: reason, BlockedAt: at}
			if taskID != "" {
				byTask[taskID] = row
			}
			if sourceType != "" && sourceID != "" {
				bySource[sourceType+"|"+sourceID] = row
			}
		}
		rows.Close()
	}
	for i := range items {
		if strings.TrimSpace(items[i].BlockedReason) != "" {
			continue
		}
		if row, ok := byTask[items[i].RefID]; ok {
			items[i].BlockedReason = row.BlockedReason
			items[i].BlockedAt = row.BlockedAt
			continue
		}
		key := blockedSourceKey(items[i].Prefix, items[i].RefID)
		if key == "" {
			continue
		}
		if row, ok := bySource[key]; ok {
			items[i].BlockedReason = row.BlockedReason
			items[i].BlockedAt = row.BlockedAt
		}
	}
}

func blockedSourceKey(prefix, refID string) string {
	refID = strings.TrimSpace(refID)
	if refID == "" {
		return ""
	}
	switch strings.TrimSpace(prefix) {
	case model.WorkPrefixAS:
		return model.WBSourceAS + "|" + refID
	case model.WorkPrefixMaintenance:
		return model.WBSourceMaintenance + "|" + refID
	default:
		return ""
	}
}

func (r *WorkBoardRepo) withoutWaiting(items []model.WorkListItem) []model.WorkListItem {
	r.fillBlockedReasons(items)
	out := make([]model.WorkListItem, 0, len(items))
	for _, it := range items {
		if it.IsWaiting() {
			continue
		}
		out = append(out, it)
	}
	return out
}

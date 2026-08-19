package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// ListMembers 업무 참여자. 주담당(owner)이 앞, 지원은 sort_order.
func (r *WBRepo) ListMembers(taskID string) ([]model.WorkTaskMember, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	m, err := r.ListMembersByTaskIDs([]string{taskID})
	if err != nil {
		return nil, err
	}
	return m[taskID], nil
}

// ListMembersByTaskIDs 여러 업무의 참여자. 키가 없는 업무는 빈 슬라이스.
func (r *WBRepo) ListMembersByTaskIDs(ids []string) (map[string][]model.WorkTaskMember, error) {
	out := map[string][]model.WorkTaskMember{}
	var clean []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := out[id]; !ok {
			out[id] = nil
			clean = append(clean, id)
		}
	}
	if r == nil || r.db == nil || len(clean) == 0 {
		return out, nil
	}
	ph := make([]string, len(clean))
	args := make([]interface{}, len(clean))
	for i, id := range clean {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT task_id, COALESCE(assignee,''), COALESCE(member_role,''), duration_min, COALESCE(sort_order,0)
		  FROM work_task_members
		 WHERE task_id IN (`+strings.Join(ph, ",")+`)
		 ORDER BY CASE member_role WHEN 'owner' THEN 0 ELSE 1 END, sort_order, assignee`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m model.WorkTaskMember
		var dur sql.NullInt64
		if err := rows.Scan(&m.TaskID, &m.Assignee, &m.Role, &dur, &m.SortOrder); err != nil {
			return nil, err
		}
		if dur.Valid {
			m.DurationMin = int(dur.Int64)
		}
		out[m.TaskID] = append(out[m.TaskID], m)
	}
	return out, rows.Err()
}

// ReplaceSupportMembers 지원 참여자만 갈아끼운다. owner 행은 트리거·work_tasks.assignee 가 유지한다.
func (r *WBRepo) ReplaceSupportMembers(taskID string, supports []model.WorkTaskMember) error {
	taskID = strings.TrimSpace(taskID)
	if r == nil || r.db == nil || taskID == "" {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM work_task_members WHERE task_id=? AND member_role=?`, taskID, model.WBMemberSupport); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	seen := map[string]bool{}
	for i, m := range supports {
		name := strings.TrimSpace(m.Assignee)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		var dur interface{}
		if m.DurationMin > 0 {
			dur = m.DurationMin
		}
		order := m.SortOrder
		if order <= 0 {
			order = i + 1
		}
		if _, err := tx.Exec(`
			INSERT INTO work_task_members (task_id, assignee, member_role, duration_min, sort_order)
			VALUES (?,?,?,?,?)`, taskID, name, model.WBMemberSupport, dur, order); err != nil {
			return fmt.Errorf("참여자 저장: %w", err)
		}
	}
	return tx.Commit()
}

func personOnTask(t model.WorkTask, name string, members map[string][]model.WorkTaskMember) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return strings.TrimSpace(t.Assignee) == ""
	}
	if ms := members[t.TaskID]; len(ms) > 0 {
		for _, m := range ms {
			if strings.TrimSpace(m.Assignee) == name {
				return true
			}
		}
		return false
	}
	return strings.TrimSpace(t.Assignee) == name
}

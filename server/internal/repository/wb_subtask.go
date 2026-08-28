package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

const sqlSubtaskRoleNull = `t.recurrence_role IS NULL`

// ListSubtasks 직계 하위 업무. 실행 작업(occurrence)은 넣지 않는다(§33.3.2).
func (r *WBRepo) ListSubtasks(parentID string) ([]model.WorkTask, error) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(workTaskSelect+`
		WHERE t.parent_task_id=? AND `+sqlSubtaskRoleNull+`
		ORDER BY t.task_id`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWorkTasks(rows)
}

func (r *WBRepo) CountSubtasks(parentID string) (int, error) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return 0, nil
	}
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.parent_task_id=? AND t.recurrence_role IS NULL`, parentID).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

func (r *WBRepo) TaskDepth(taskID string) int {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return 0
	}
	seen := map[string]bool{}
	depth := 1
	id := taskID
	for i := 0; i < model.MaxWorkTaskDepth+4; i++ {
		t, err := r.GetTask(id)
		if err != nil || t == nil || strings.TrimSpace(t.ParentTaskID) == "" {
			return depth
		}
		if seen[t.ParentTaskID] {
			return depth
		}
		seen[id] = true
		id = t.ParentTaskID
		depth++
	}
	return depth
}

func (r *WBRepo) FillTaskDepth(items []model.WorkTask) {
	cache := map[string]int{}
	for i := range items {
		id := items[i].TaskID
		if d, ok := cache[id]; ok {
			items[i].Depth = d
			continue
		}
		d := r.TaskDepth(id)
		cache[id] = d
		items[i].Depth = d
	}
}

func (r *WBRepo) SubtaskDescendantIDs(rootID string) (map[string]bool, error) {
	out := map[string]bool{}
	var walk func(string) error
	walk = func(id string) error {
		kids, err := r.ListSubtasks(id)
		if err != nil {
			return err
		}
		for _, k := range kids {
			if out[k.TaskID] {
				continue
			}
			out[k.TaskID] = true
			if err := walk(k.TaskID); err != nil {
				return err
			}
		}
		return nil
	}
	return out, walk(strings.TrimSpace(rootID))
}

// ListSubtaskParentCandidates 부모 후보. excludeID(자기)와 그 자손은 뺀다(§33.3.2).
func (r *WBRepo) ListSubtaskParentCandidates(excludeID string) ([]model.WorkTask, error) {
	items, err := r.ListAdminWork("", "")
	if err != nil {
		return nil, err
	}
	excludeID = strings.TrimSpace(excludeID)
	desc := map[string]bool{}
	if excludeID != "" {
		desc, err = r.SubtaskDescendantIDs(excludeID)
		if err != nil {
			return nil, err
		}
	}
	var out []model.WorkTask
	for _, it := range items {
		if excludeID != "" && (it.TaskID == excludeID || desc[it.TaskID]) {
			continue
		}
		if it.RecurrenceRole == model.RecurrenceRoleOccurrence {
			continue
		}
		d := r.TaskDepth(it.TaskID)
		if d >= model.MaxWorkTaskDepth {
			continue
		}
		it.Depth = d
		out = append(out, it)
	}
	return out, nil
}

func (r *WBRepo) CanAttachSubtask(parentID string) error {
	return r.CanAttachSubtaskTo(parentID, "")
}

func (r *WBRepo) CanAttachSubtaskTo(parentID, childID string) error {
	parentID = strings.TrimSpace(parentID)
	childID = strings.TrimSpace(childID)
	if childID != "" && parentID == childID {
		return model.ErrSubtaskCycle
	}
	parent, err := r.GetTask(parentID)
	if err != nil || parent == nil {
		return model.ErrSubtaskParent
	}
	if err := model.CanAddSubtaskUnder(*parent, r.TaskDepth(parentID)); err != nil {
		return err
	}
	if childID == "" {
		return nil
	}
	desc, err := r.SubtaskDescendantIDs(childID)
	if err != nil {
		return err
	}
	if desc[parentID] {
		return model.ErrSubtaskCycle
	}
	return nil
}

func (r *WBRepo) nextChildTaskID(parentID string) (string, error) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return "", fmt.Errorf("상위 번호가 없습니다")
	}
	rows, err := r.db.Query(`
		SELECT task_id FROM work_tasks
		WHERE parent_task_id=? AND recurrence_role IS NULL`, parentID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return model.ChildTaskID(parentID, model.NextChildSeq(parentID, ids)), nil
}

func (r *WBRepo) assignNewTaskID(t *model.WorkTask) error {
	if t == nil {
		return fmt.Errorf("업무가 없습니다")
	}
	if strings.TrimSpace(t.ParentTaskID) != "" && model.IsSubtaskRole(t.RecurrenceRole) {
		if err := r.CanAttachSubtaskTo(t.ParentTaskID, ""); err != nil {
			return err
		}
		id, err := r.nextChildTaskID(t.ParentTaskID)
		if err != nil {
			return err
		}
		t.TaskID = id
		return nil
	}
	id, err := r.nextID("work_task", "WT")
	if err != nil {
		return err
	}
	t.TaskID = id
	return nil
}

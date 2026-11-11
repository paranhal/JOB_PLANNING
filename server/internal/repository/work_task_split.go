package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// 각각 작업은 원 업무를 첫 대상으로 재사용하고, 나머지는 source_type=split 으로 만든다.
// AS·정기점검은 여기서 나누지 않는다.
const splitSourceType = "split"

// splitBeforeInsert 는 트랜잭션 안에서 새 업무를 넣기 직전에 호출된다. 테스트가 중간 실패를 만든다.
var splitBeforeInsert func(i int) error

type SplitTarget struct {
	Kind string
	ID   string
	Name string
}

type SplitLinks struct {
	Assets []string
	Sites  []string
}

func (r *WBRepo) SplitByTargets(src *model.WorkTask, targets []SplitTarget, names map[string]string, inherit SplitLinks) ([]*model.WorkTask, error) {
	if r == nil || src == nil {
		return nil, fmt.Errorf("업무가 없습니다")
	}
	targets = uniqueSplitTargets(targets)
	if len(targets) < 2 {
		return nil, fmt.Errorf("나눌 대상이 2개 미만입니다")
	}
	if err := r.splitBlocked(src); err != nil {
		return nil, err
	}
	baseTitle := strings.TrimSpace(src.Title)
	kind := targets[0].Kind
	newSrc := strings.TrimSpace(src.TaskID) == ""
	hadCustomer := strings.TrimSpace(src.CustomerID) != ""
	src.Title = splitTitle(baseTitle, targets[0], names)
	if newSrc && kind == "site" && !hadCustomer {
		src.CustomerID = targets[0].ID
	}
	if newSrc {
		if err := r.prepareSplitTask(src); err != nil {
			return nil, err
		}
	}
	copies := make([]*model.WorkTask, len(targets))
	copies[0] = src
	for i := 1; i < len(targets); i++ {
		if id, err := r.findSplitCopy(src.TaskID, targets[i]); err != nil {
			return nil, err
		} else if id != "" {
			got, err := r.GetTask(id)
			if err != nil {
				return nil, err
			}
			copies[i] = got
			continue
		}
		cp := *src
		cp.TaskID = ""
		cp.Title = splitTitle(baseTitle, targets[i], names)
		cp.SourceType = splitSourceType
		cp.SourceID = src.TaskID
		cp.SourceRole = splitRole(targets[i])
		cp.ParentTaskID = ""
		cp.RecurrenceRole = ""
		cp.OccurrenceSeq = 0
		cp.OccurrenceStatus = ""
		if kind == "site" && !hadCustomer {
			cp.CustomerID = targets[i].ID
		}
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
	for i, tg := range targets {
		task := copies[i]
		if task == nil {
			return nil, fmt.Errorf("업무를 만들지 못했습니다")
		}
		assets, sites := linksFor(kind, tg, inherit)
		if i == 0 && !newSrc {
			if _, err := tx.Exec(`UPDATE work_tasks SET title=? WHERE task_id=?`, task.Title, task.TaskID); err != nil {
				return nil, err
			}
			if err := writeSplitLinks(tx, task.TaskID, task.ProjectID, assets, sites); err != nil {
				return nil, err
			}
			continue
		}
		if i > 0 && task.SourceType == splitSourceType && strings.TrimSpace(task.TaskID) != "" {
			var exists string
			err := tx.QueryRow(`SELECT task_id FROM work_tasks WHERE source_type=? AND source_id=? AND source_role=?`,
				splitSourceType, src.TaskID, splitRole(tg)).Scan(&exists)
			if err == nil && exists != "" {
				continue
			}
			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}
		}
		if splitBeforeInsert != nil {
			if err := splitBeforeInsert(i); err != nil {
				return nil, err
			}
		}
		if err := execWorkTaskInsert(tx, task); err != nil {
			return nil, err
		}
		if err := writeSplitLinks(tx, task.TaskID, task.ProjectID, assets, sites); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, task := range copies {
		if task != nil {
			replaceTaskTags(r.db, task.TaskID, task.Tags)
		}
	}
	return copies, nil
}

func (r *WBRepo) splitBlocked(src *model.WorkTask) error {
	switch strings.TrimSpace(src.WorkType) {
	case model.WBWorkAS, model.WBWorkMaintenance:
		return fmt.Errorf("AS·정기점검 업무는 여기서 나누지 않습니다")
	}
	switch strings.TrimSpace(src.SourceType) {
	case model.WBSourceAS, model.WBSourceMaintenance:
		return fmt.Errorf("AS·정기점검 업무는 여기서 나누지 않습니다")
	}
	if src.Status == model.WBTaskComplete {
		return fmt.Errorf("완료된 업무는 나누지 않습니다")
	}
	if src.RecurrenceRole == model.RecurrenceRoleParent {
		return fmt.Errorf("반복 업무는 회차별로 이미 나뉘어 있습니다")
	}
	if id := strings.TrimSpace(src.TaskID); id != "" {
		var n int
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE parent_task_id=?`, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("하위 업무가 있어 나눌 수 없습니다")
		}
	}
	return nil
}

func (r *WBRepo) prepareSplitTask(t *model.WorkTask) error {
	normalizeWorkTask(t)
	t.Assignee, t.AssigneeUserID = bindStaff(r.db, t.Assignee, t.AssigneeUserID)
	stampNewWorkTaskDates(t)
	if strings.TrimSpace(t.TaskID) == "" {
		return r.assignNewTaskID(t)
	}
	return nil
}

func (r *WBRepo) findSplitCopy(srcID string, tg SplitTarget) (string, error) {
	srcID = strings.TrimSpace(srcID)
	if srcID == "" {
		return "", nil
	}
	var id string
	err := r.db.QueryRow(`SELECT task_id FROM work_tasks WHERE source_type=? AND source_id=? AND source_role=?`,
		splitSourceType, srcID, splitRole(tg)).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func splitRole(tg SplitTarget) string {
	return tg.Kind + ":" + strings.TrimSpace(tg.ID)
}

func splitTitle(base string, tg SplitTarget, names map[string]string) string {
	if names != nil {
		if v := strings.TrimSpace(names[splitRole(tg)]); v != "" {
			return v
		}
	}
	label := strings.TrimSpace(tg.Name)
	if label == "" {
		label = strings.TrimSpace(tg.ID)
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return label
	}
	return base + " - " + label
}

func uniqueSplitTargets(in []SplitTarget) []SplitTarget {
	seen := map[string]bool{}
	var out []SplitTarget
	for _, tg := range in {
		tg.Kind = strings.TrimSpace(tg.Kind)
		tg.ID = strings.TrimSpace(tg.ID)
		tg.Name = strings.TrimSpace(tg.Name)
		if tg.ID == "" || (tg.Kind != "asset" && tg.Kind != "site") {
			continue
		}
		key := splitRole(tg)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, tg)
	}
	return out
}

func linksFor(kind string, tg SplitTarget, inherit SplitLinks) (assets, sites []string) {
	if kind == "site" {
		return append([]string{}, inherit.Assets...), []string{tg.ID}
	}
	return []string{tg.ID}, append([]string{}, inherit.Sites...)
}

func writeSplitLinks(tx *sql.Tx, taskID, projectID string, assets, sites []string) error {
	if _, err := tx.Exec(`DELETE FROM work_task_assets WHERE task_id=?`, taskID); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return err
		}
	} else {
		seen := map[string]bool{}
		for _, id := range assets {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			if _, err := tx.Exec(`INSERT INTO work_task_assets (task_id, asset_id) VALUES (?,?)`, taskID, id); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`DELETE FROM work_task_sites WHERE task_id=?`, taskID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range sites {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var psid string
		if strings.TrimSpace(projectID) != "" {
			err := tx.QueryRow(`
				SELECT id FROM project_sites
				WHERE project_id=? AND customer_id=? AND is_active=1 AND status != 'excluded'
				LIMIT 1`, projectID, id).Scan(&psid)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
		}
		if _, err := tx.Exec(`INSERT INTO work_task_sites(task_id,customer_id,project_site_id) VALUES(?,?,?)`,
			taskID, id, emptyToNullStr(psid)); err != nil {
			return err
		}
	}
	return nil
}

type sqlExec interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func execWorkTaskInsert(ex sqlExec, t *model.WorkTask) error {
	byID, byName := stampCreatedBy()
	_, err := ex.Exec(`
		INSERT INTO work_tasks (task_id, work_type, work_subtype, project_id, title, description, due_date,
			work_date, start_time, end_time, duration_min, status, priority, assignee, assignee_user_id, assignee_source, tags, progress,
			source_type, source_id, source_role, parent_task_id, customer_id, customer_name,
			hold_reason, review_date, cancel_reason, wait_party_kind, wait_party, wait_request,
			reply_due_date, next_check_date, complete_note, receipt_date, complete_date,
			recurrence_role, occurrence_seq, occurrence_status, not_done_reason, org_id, work_group_id, created_by_user_id, created_by_name, is_test, project_match_pending)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.TaskID, t.WorkType, t.WorkSubtype, nullStr(t.ProjectID), t.Title, t.Description, t.DueDate,
		t.WorkDate, t.StartTime, t.EndTime, t.DurationMin, t.Status, t.Priority, t.Assignee, t.AssigneeUserID, t.AssigneeSource, t.Tags, t.Progress,
		t.SourceType, t.SourceID, t.SourceRole, nullStr(t.ParentTaskID), nullStr(t.CustomerID), nullIfEmpty(t.CustomerName),
		nullIfEmpty(t.HoldReason), nullIfEmpty(t.ReviewDate), nullIfEmpty(t.CancelReason),
		nullIfEmpty(t.WaitPartyKind), nullIfEmpty(t.WaitParty), nullIfEmpty(t.WaitRequest),
		nullIfEmpty(t.ReplyDueDate), nullIfEmpty(t.NextCheckDate), nullIfEmpty(t.CompleteNote),
		nullIfEmpty(t.ReceiptDate), nullIfEmpty(t.CompleteDate),
		nullIfEmpty(t.RecurrenceRole), t.OccurrenceSeq, nullIfEmpty(t.OccurrenceStatus), nullIfEmpty(t.NotDoneReason), t.OrgID, nullIfEmpty(t.WorkGroupID), byID, byName, stampIsTest(), boolToInt(t.ProjectMatchPending))
	return err
}

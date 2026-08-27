package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func (r *WBRepo) GetRecurrence(taskID string) (*model.WorkRecurrence, error) {
	taskID = strings.TrimSpace(taskID)
	if r == nil || r.db == nil || taskID == "" {
		return nil, nil
	}
	row := r.db.QueryRow(`
		SELECT task_id, COALESCE(start_date,''), COALESCE(end_date,''), COALESCE(rule_type,'none'),
		       COALESCE(interval_n,1), COALESCE(weekdays,''), COALESCE(holiday_policy,'as_is'),
		       COALESCE(complete_policy,'manual'), COALESCE(progress_include_future,0), COALESCE(final_result,'')
		FROM work_recurrence WHERE task_id=?`, taskID)
	var w model.WorkRecurrence
	var include int
	err := row.Scan(&w.TaskID, &w.StartDate, &w.EndDate, &w.RuleType, &w.IntervalN, &w.Weekdays,
		&w.HolidayPolicy, &w.CompletePolicy, &include, &w.FinalResult)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	w.ProgressIncludeFuture = include != 0
	w.RuleType = model.NormalizeRecurrenceRuleType(w.RuleType)
	w.HolidayPolicy = model.NormalizeHolidayPolicy(w.HolidayPolicy)
	w.CompletePolicy = model.NormalizeCompletePolicy(w.CompletePolicy)
	if w.IntervalN < 1 {
		w.IntervalN = 1
	}
	return &w, nil
}

func (r *WBRepo) UpsertRecurrence(w model.WorkRecurrence) error {
	if r == nil || r.db == nil || strings.TrimSpace(w.TaskID) == "" {
		return fmt.Errorf("상위 업무가 없습니다")
	}
	w.RuleType = model.NormalizeRecurrenceRuleType(w.RuleType)
	w.HolidayPolicy = model.NormalizeHolidayPolicy(w.HolidayPolicy)
	w.CompletePolicy = model.NormalizeCompletePolicy(w.CompletePolicy)
	if w.IntervalN < 1 {
		w.IntervalN = 1
	}
	include := 0
	if w.ProgressIncludeFuture {
		include = 1
	}
	_, err := r.db.Exec(`
		INSERT INTO work_recurrence (
			task_id, start_date, end_date, rule_type, interval_n, weekdays,
			holiday_policy, complete_policy, progress_include_future, final_result, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(task_id) DO UPDATE SET
			start_date=excluded.start_date,
			end_date=excluded.end_date,
			rule_type=excluded.rule_type,
			interval_n=excluded.interval_n,
			weekdays=excluded.weekdays,
			holiday_policy=excluded.holiday_policy,
			complete_policy=excluded.complete_policy,
			progress_include_future=excluded.progress_include_future,
			final_result=CASE WHEN TRIM(excluded.final_result)='' THEN work_recurrence.final_result ELSE excluded.final_result END,
			updated_at=CURRENT_TIMESTAMP`,
		w.TaskID, w.StartDate, w.EndDate, w.RuleType, w.IntervalN, w.Weekdays,
		w.HolidayPolicy, w.CompletePolicy, include, w.FinalResult)
	return err
}

func (r *WBRepo) CountOccurrences(parentID string) (int, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks
		WHERE parent_task_id=? AND COALESCE(recurrence_role,'')=?`,
		parentID, model.RecurrenceRoleOccurrence).Scan(&n)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

func (r *WBRepo) GenerateOccurrences(parent *model.WorkTask, rule model.WorkRecurrence, dates []string) (model.OccurrenceGenerateResult, error) {
	var out model.OccurrenceGenerateResult
	if parent == nil || parent.TaskID == "" {
		return out, fmt.Errorf("상위 업무가 없습니다")
	}
	n, err := r.CountOccurrences(parent.TaskID)
	if err != nil {
		return out, err
	}
	if n > 0 {
		return out, fmt.Errorf("이미 실행 작업이 있습니다. 재생성 버튼을 쓰세요")
	}
	return r.writeOccurrences(parent, rule, dates, false)
}

func (r *WBRepo) RegenerateOccurrences(parent *model.WorkTask, rule model.WorkRecurrence, dates []string) (model.OccurrenceGenerateResult, error) {
	var out model.OccurrenceGenerateResult
	if parent == nil || parent.TaskID == "" {
		return out, fmt.Errorf("상위 업무가 없습니다")
	}
	return r.writeOccurrences(parent, rule, dates, true)
}

func (r *WBRepo) writeOccurrences(parent *model.WorkTask, rule model.WorkRecurrence, dates []string, regenerate bool) (model.OccurrenceGenerateResult, error) {
	var out model.OccurrenceGenerateResult
	rule.TaskID = parent.TaskID
	if err := r.UpsertRecurrence(rule); err != nil {
		return out, err
	}
	finalDue := strings.TrimSpace(parent.DueDate)
	if finalDue == "" {
		finalDue = strings.TrimSpace(rule.EndDate)
	}
	if _, err := r.db.Exec(`
		UPDATE work_tasks
		SET recurrence_role=?,
		    due_date=CASE WHEN TRIM(COALESCE(due_date,''))='' THEN ? ELSE due_date END,
		    updated_at=CURRENT_TIMESTAMP
		WHERE task_id=?`, model.RecurrenceRoleParent, finalDue, parent.TaskID); err != nil {
		return out, err
	}

	keepDates := map[string]bool{}
	maxSeq := 0
	if regenerate {
		children, err := r.ListChildren(parent.TaskID)
		if err != nil {
			return out, err
		}
		sqlCount, err := r.CountOccurrences(parent.TaskID)
		if err != nil {
			return out, err
		}
		listedOcc := 0
		for _, ch := range children {
			if ch.RecurrenceRole != model.RecurrenceRoleOccurrence {
				continue
			}
			listedOcc++
			if ch.ParentTaskID != parent.TaskID {
				out.ParentMismatch++
			}
			if ch.OccurrenceSeq > maxSeq {
				maxSeq = ch.OccurrenceSeq
			}
			complete := ch.Status == model.WBTaskComplete || ch.OccurrenceStatus == model.OccurrenceComplete
			if complete {
				out.KeptComplete++
				if d := strings.TrimSpace(ch.WorkDate); d != "" {
					keepDates[d] = true
				}
			}
		}
		if listedOcc != sqlCount {
			out.ParentMismatch++
		}
		res, err := r.db.Exec(`
			DELETE FROM work_tasks
			WHERE parent_task_id=?
			  AND COALESCE(recurrence_role,'')=?
			  AND COALESCE(status,'') != ?
			  AND COALESCE(occurrence_status,'') != ?`,
			parent.TaskID, model.RecurrenceRoleOccurrence, model.WBTaskComplete, model.OccurrenceComplete)
		if err != nil {
			return out, err
		}
		if res != nil {
			n, _ := res.RowsAffected()
			out.Deleted = int(n)
		}
	}

	supports := occurrenceSupports(parent.TaskID, r)
	seq := maxSeq
	for _, d := range dates {
		d = strings.TrimSpace(d)
		if d == "" || keepDates[d] {
			continue
		}
		seq++
		child := occurrenceFromParent(parent, finalDue, d, seq)
		if err := r.CreateTask(child); err != nil {
			return out, err
		}
		if err := r.ReplaceSupportMembers(child.TaskID, supports); err != nil {
			return out, err
		}
		out.Created++
	}
	linked, err := r.CountOccurrences(parent.TaskID)
	if err != nil {
		return out, err
	}
	if linked != out.Created+out.KeptComplete {
		out.ParentMismatch++
	}
	return out, nil
}

func occurrenceFromParent(parent *model.WorkTask, finalDue, workDate string, seq int) *model.WorkTask {
	return &model.WorkTask{
		WorkType:         parent.WorkType,
		ProjectID:        parent.ProjectID,
		Title:            parent.Title,
		Description:      parent.Description,
		DueDate:          finalDue,
		WorkDate:         workDate,
		DurationMin:      parent.DurationMin,
		Status:           model.WBTaskWaiting,
		Priority:         parent.Priority,
		Assignee:         parent.Assignee,
		Tags:             parent.Tags,
		CustomerID:       parent.CustomerID,
		CustomerName:     parent.CustomerName,
		ParentTaskID:     parent.TaskID,
		RecurrenceRole:   model.RecurrenceRoleOccurrence,
		OccurrenceSeq:    seq,
		OccurrenceStatus: model.OccurrenceScheduled,
	}
}

func occurrenceSupports(parentID string, r *WBRepo) []model.WorkTaskMember {
	members, err := r.ListMembers(parentID)
	if err != nil {
		return nil
	}
	var out []model.WorkTaskMember
	for _, m := range members {
		if m.IsSupport() {
			out = append(out, m)
		}
	}
	return out
}

func (r *WBRepo) MarkPastOccurrencesOverdue(today string) (int, error) {
	return markPastOccurrencesOverdue(r.db, today)
}

func markPastOccurrencesOverdue(db *sql.DB, today string) (int, error) {
	if db == nil {
		return 0, nil
	}
	today = strings.TrimSpace(today)
	if today == "" {
		return 0, nil
	}
	res, err := db.Exec(`
		UPDATE work_tasks
		SET occurrence_status=?, updated_at=CURRENT_TIMESTAMP
		WHERE COALESCE(recurrence_role,'')=?
		  AND COALESCE(status,'') NOT IN ('complete','cancelled')
		  AND COALESCE(occurrence_status,'') NOT IN (?,?,?)
		  AND (
			(COALESCE(occurrence_status,'') IN ('scheduled','in_progress','')
			 AND TRIM(COALESCE(work_date,'')) != ''
			 AND date(work_date) < date(?))
			OR
			(COALESCE(occurrence_status,'')=?
			 AND (TRIM(COALESCE(next_check_date,''))='' OR date(next_check_date) < date(?)))
		  )`,
		model.OccurrenceOverdue, model.RecurrenceRoleOccurrence,
		model.OccurrenceComplete, model.OccurrenceSkipped, model.OccurrenceOverdue,
		today, model.OccurrenceDeferred, today)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return 0, nil
		}
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *WBRepo) CountOpenOccurrences(parentID string) (int, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks
		WHERE parent_task_id=?
		  AND COALESCE(recurrence_role,'')=?
		  AND COALESCE(status,'') != ?
		  AND COALESCE(occurrence_status,'') NOT IN (?,?)`,
		parentID, model.RecurrenceRoleOccurrence, model.WBTaskComplete,
		model.OccurrenceComplete, model.OccurrenceSkipped).Scan(&n)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

func (r *WBRepo) UpdateOccurrenceFields(taskID, occStatus, reason, nextCheck string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("업무가 없습니다")
	}
	occStatus = model.NormalizeOccurrenceStatus(occStatus)
	reason = strings.TrimSpace(reason)
	nextCheck = strings.TrimSpace(nextCheck)
	if occStatus == model.OccurrenceSkipped && reason == "" {
		return fmt.Errorf("제외 사유를 입력하세요")
	}
	if occStatus == model.OccurrenceDeferred && nextCheck == "" {
		return fmt.Errorf("다음 조치일을 입력하세요")
	}
	status := ""
	progress := -1
	switch occStatus {
	case model.OccurrenceComplete:
		status = model.WBTaskComplete
		progress = 100
	case model.OccurrenceInProgress:
		status = model.WBTaskInProgress
	}
	q := `
		UPDATE work_tasks SET
			occurrence_status=?,
			not_done_reason=?,
			next_check_date=?,
			updated_at=CURRENT_TIMESTAMP`
	args := []interface{}{occStatus, nullIfEmpty(reason), nullIfEmpty(nextCheck)}
	if status != "" {
		q += `, status=?`
		args = append(args, status)
	}
	if progress >= 0 {
		q += `, progress=?, complete_date=CASE WHEN TRIM(COALESCE(complete_date,''))!='' THEN complete_date ELSE date('now','localtime') END`
		args = append(args, progress)
	}
	q += ` WHERE task_id=? AND COALESCE(recurrence_role,'')=?`
	args = append(args, taskID, model.RecurrenceRoleOccurrence)
	_, err := r.db.Exec(q, args...)
	return err
}

func (r *WBRepo) MaybeAutoCompleteParent(parentID string) error {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil
	}
	rule, err := r.GetRecurrence(parentID)
	if err != nil || rule == nil {
		return err
	}
	if model.NormalizeCompletePolicy(rule.CompletePolicy) == model.CompletePolicyManual {
		return nil
	}
	open, err := r.CountOpenOccurrences(parentID)
	if err != nil || open > 0 {
		return err
	}
	if model.NormalizeCompletePolicy(rule.CompletePolicy) == model.CompletePolicyRequireResult &&
		strings.TrimSpace(rule.FinalResult) == "" {
		return nil
	}
	_, err = r.db.Exec(`
		UPDATE work_tasks
		SET status=?, progress=100,
		    complete_note=CASE WHEN TRIM(COALESCE(complete_note,''))='' THEN ? ELSE complete_note END,
		    complete_date=CASE WHEN TRIM(COALESCE(complete_date,''))='' THEN date('now','localtime') ELSE complete_date END,
		    updated_at=CURRENT_TIMESTAMP
		WHERE task_id=? AND COALESCE(status,'') NOT IN ('complete','cancelled')`,
		model.WBTaskComplete, "실행 작업이 모두 완료·제외되어 자동 완료", parentID)
	return err
}

func (r *WBRepo) SetRecurrenceFinalResult(taskID, result string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("상위 업무가 없습니다")
	}
	_, err := r.db.Exec(`
		UPDATE work_recurrence SET final_result=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		strings.TrimSpace(result), taskID)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}

func (r *WBRepo) UpdateRecurrenceSettings(taskID, policy string, includeFuture bool, finalResult string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("상위 업무가 없습니다")
	}
	include := 0
	if includeFuture {
		include = 1
	}
	_, err := r.db.Exec(`
		UPDATE work_recurrence
		SET complete_policy=?, progress_include_future=?, final_result=?, updated_at=CURRENT_TIMESTAMP
		WHERE task_id=?`,
		model.NormalizeCompletePolicy(policy), include, strings.TrimSpace(finalResult), taskID)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil
	}
	return err
}

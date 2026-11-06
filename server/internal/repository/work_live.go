package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

const liveSegCols = `segment_id, task_id, kind, scope, seq, work_date, start_time, end_time, duration_min,
		       travel_before_min, travel_after_min, user_id, assignee, worker_user_id, worker_name,
		       interrupt_task_id, note,
		       COALESCE(source,'manual'), COALESCE(ended_by,''),
		       COALESCE(travel_origin,''), COALESCE(travel_dest,''),
		       COALESCE(travel_plan_depart,''), COALESCE(travel_plan_arrive,''),
		       COALESCE(travel_actual_depart,''), COALESCE(travel_actual_arrive,''),
		       COALESCE(travel_source,'manual'), COALESCE(travel_unknown_reason,'')`

func scanLiveSegment(s *model.WorkTaskSegment, rows interface {
	Scan(dest ...any) error
}) error {
	return rows.Scan(&s.SegmentID, &s.TaskID, &s.Kind, &s.Scope, &s.Seq, &s.WorkDate,
		&s.StartTime, &s.EndTime, &s.DurationMin, &s.TravelBeforeMin, &s.TravelAfterMin,
		&s.UserID, &s.Assignee, &s.WorkerUserID, &s.WorkerName, &s.InterruptTaskID, &s.Note,
		&s.Source, &s.EndedBy, &s.TravelOrigin, &s.TravelDest, &s.TravelPlanDepart, &s.TravelPlanArrive,
		&s.TravelActualDepart, &s.TravelActualArrive, &s.TravelSource, &s.TravelUnknownReason)
}

func (r *WBRepo) LoadTaskLiveFields(t *model.WorkTask) error {
	if r == nil || r.db == nil || t == nil || strings.TrimSpace(t.TaskID) == "" {
		return nil
	}
	var est sql.NullInt64
	err := r.db.QueryRow(`SELECT est_minutes, COALESCE(next_action_date,''), COALESCE(completion_condition,'')
		FROM work_tasks WHERE task_id=?`, t.TaskID).Scan(&est, &t.NextActionDate, &t.CompletionCondition)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil && strings.Contains(err.Error(), "no such column") {
		return nil
	}
	if err != nil {
		return err
	}
	if est.Valid {
		t.EstMinutes = int(est.Int64)
	}
	return nil
}

func (r *WBRepo) SetTaskLiveFields(taskID string, estMinutes int, nextAction, condition string) error {
	taskID = strings.TrimSpace(taskID)
	if r == nil || r.db == nil || taskID == "" {
		return nil
	}
	var est any
	if estMinutes > 0 {
		est = estMinutes
	}
	_, err := r.db.Exec(`UPDATE work_tasks SET est_minutes=?, next_action_date=?, completion_condition=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		est, strings.TrimSpace(nextAction), strings.TrimSpace(condition), taskID)
	if err != nil && strings.Contains(err.Error(), "no such column") {
		return nil
	}
	return err
}

func (r *WBRepo) SetTaskNextActionDate(taskID, next string) error {
	taskID = strings.TrimSpace(taskID)
	if r == nil || r.db == nil || taskID == "" {
		return nil
	}
	_, err := r.db.Exec(`UPDATE work_tasks SET next_action_date=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		strings.TrimSpace(next), taskID)
	if err != nil && strings.Contains(err.Error(), "no such column") {
		return nil
	}
	return err
}

func (r *WBRepo) ListOpenActualForWorker(workerID string) ([]model.WorkTaskSegment, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`SELECT `+liveSegCols+`
		FROM work_task_segments WHERE kind='actual' AND TRIM(end_time)=''
		AND (worker_user_id=? OR user_id=?)`, workerID, workerID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTaskSegment
	for rows.Next() {
		var s model.WorkTaskSegment
		if err := scanLiveSegment(&s, rows); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *WBRepo) ListOpenActualTaskIDs(taskIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	ids := uniqueNonEmpty(taskIDs)
	if r == nil || r.db == nil || len(ids) == 0 {
		return out, nil
	}
	for i := 0; i < len(ids); i += inClauseChunk {
		end := i + inClauseChunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[i:end]
		rows, err := r.db.Query(`
			SELECT DISTINCT task_id FROM work_task_segments
			WHERE task_id IN (`+placeholders(len(part))+`)
			  AND kind=?
			  AND TRIM(COALESCE(end_time,''))=''`,
			append(iface(part), model.SegKindActual)...)
		if err != nil {
			if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
				return out, nil
			}
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			if strings.TrimSpace(id) != "" {
				out[id] = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *WBRepo) ListOpenActualForTask(taskID, workerID string) ([]model.WorkTaskSegment, error) {
	all, err := r.ListActualSegments(taskID)
	if err != nil {
		return nil, err
	}
	var out []model.WorkTaskSegment
	for _, s := range all {
		if strings.TrimSpace(s.EndTime) != "" {
			continue
		}
		if workerID != "" && s.WorkerUserID != workerID && s.UserID != workerID {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *WBRepo) CloseSegment(seg model.WorkTaskSegment, endHHMM, endedBy string) error {
	if r == nil || r.db == nil || strings.TrimSpace(seg.SegmentID) == "" {
		return fmt.Errorf("구간이 없습니다")
	}
	endHHMM = model.ClockHHMM(endHHMM)
	dur := model.DurationFromTimes(seg.StartTime, endHHMM)
	if strings.TrimSpace(endHHMM) == "" {
		dur = 0
	}
	_, err := r.db.Exec(`UPDATE work_task_segments SET end_time=?, duration_min=?, ended_by=?, updated_at=CURRENT_TIMESTAMP WHERE segment_id=?`,
		endHHMM, dur, strings.TrimSpace(endedBy), seg.SegmentID)
	return err
}

func (r *WBRepo) SetSegmentInterrupt(segmentID, interruptID string) error {
	_, err := r.db.Exec(`UPDATE work_task_segments SET interrupt_task_id=?, updated_at=CURRENT_TIMESTAMP WHERE segment_id=?`,
		strings.TrimSpace(interruptID), strings.TrimSpace(segmentID))
	return err
}

func (r *WBRepo) InsertLiveActual(s model.WorkTaskSegment) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("저장할 수 없습니다")
	}
	s.Kind = model.SegKindActual
	if strings.TrimSpace(s.Source) == "" {
		s.Source = model.SegSourceLive
	}
	if strings.TrimSpace(s.Scope) == "" {
		s.Scope = model.SegScopeCommon
	}
	if strings.TrimSpace(s.SegmentID) == "" {
		id, err := r.nextID("work_segment", "WS")
		if err != nil {
			return "", err
		}
		s.SegmentID = id
	}
	seq := s.Seq
	if seq <= 0 {
		_ = r.db.QueryRow(`SELECT COALESCE(MAX(seq),0)+1 FROM work_task_segments WHERE task_id=? AND kind=?`, s.TaskID, model.SegKindActual).Scan(&seq)
	}
	if err := r.insertSegment(s.TaskID, model.SegKindActual, seq, s); err != nil {
		return "", err
	}
	return s.SegmentID, nil
}

func (r *WBRepo) ActualOverlapsForWorker(workerID, date, start, end, excludeSegmentID string) ([]model.WorkTaskSegment, error) {
	if r == nil || r.db == nil || strings.TrimSpace(workerID) == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`SELECT `+liveSegCols+`
		FROM work_task_segments WHERE kind='actual'
		AND (worker_user_id=? OR user_id=?)
		AND work_date=?`, workerID, workerID, strings.TrimSpace(date))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTaskSegment
	for rows.Next() {
		var s model.WorkTaskSegment
		if err := scanLiveSegment(&s, rows); err != nil {
			return nil, err
		}
		if excludeSegmentID != "" && s.SegmentID == excludeSegmentID {
			continue
		}
		en := strings.TrimSpace(s.EndTime)
		if en == "" {
			en = "23:59"
		}
		occStart := occupyStartHHMM(s)
		occEnd := occupyEndHHMM(s, en)
		if model.RangesOverlapHHMM(occStart, occEnd, start, end) {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

func occupyStartHHMM(s model.WorkTaskSegment) string {
	st := model.ClockHHMM(s.StartTime)
	sm := model.ParseHHMMMinutes(st)
	if sm < 0 {
		return st
	}
	sm -= model.ClampTravelMin(s.TravelBeforeMin)
	if sm < 0 {
		sm = 0
	}
	return model.FormatHHMMMinutes(sm)
}

func occupyEndHHMM(s model.WorkTaskSegment, end string) string {
	en := model.ClockHHMM(end)
	em := model.ParseHHMMMinutes(en)
	if em < 0 {
		return en
	}
	// after 이동은 점유 안내에만. 겹침 판정은 실제 종료까지(귀속은 다음 업무 before).
	return model.FormatHHMMMinutes(em)
}

func (r *WBRepo) SumTravel(taskID string) int {
	if r == nil || r.db == nil {
		return 0
	}
	var n int
	_ = r.db.QueryRow(`SELECT COALESCE(SUM(travel_before_min),0) FROM work_task_segments WHERE task_id=? AND kind='actual'`, taskID).Scan(&n)
	return n
}

func (r *WBRepo) SumActualMin(taskID string) int {
	if r == nil || r.db == nil {
		return 0
	}
	var n int
	_ = r.db.QueryRow(`SELECT COALESCE(SUM(duration_min),0) FROM work_task_segments WHERE task_id=? AND kind='actual' AND TRIM(end_time)!=''`, taskID).Scan(&n)
	return n
}

func (r *WBRepo) CloseStaleOpenSegments(today string) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	today = strings.TrimSpace(today)
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}
	rows, err := r.db.Query(`SELECT `+liveSegCols+` FROM work_task_segments
		WHERE kind='actual' AND TRIM(end_time)='' AND TRIM(work_date)!='' AND work_date<?`, today)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()
	n := 0
	var segs []model.WorkTaskSegment
	for rows.Next() {
		var s model.WorkTaskSegment
		if err := scanLiveSegment(&s, rows); err != nil {
			return n, err
		}
		segs = append(segs, s)
	}
	for _, s := range segs {
		if err := r.CloseSegment(s, "23:59", model.EndedByAuto); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (r *WBRepo) InsertDeparture(d model.WorkTaskDeparture) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("저장할 수 없습니다")
	}
	id := strings.TrimSpace(d.DepartureID)
	if id == "" {
		var err error
		id, err = r.nextID("work_departure", "WD")
		if err != nil {
			return "", err
		}
	}
	if d.ReturnStatus == "" {
		d.ReturnStatus = model.ReturnPending
	}
	var rem any
	if d.RemainingMin != nil {
		rem = *d.RemainingMin
	}
	_, err := r.db.Exec(`
		INSERT INTO work_task_departures (
			departure_id, task_id, user_id, assignee, work_date, start_time, end_time, interrupt_task_id, return_status,
			reason, next_action_date, remaining_min, force_reason)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, d.TaskID, d.UserID, d.Assignee, d.WorkDate, d.StartTime, d.EndTime, d.InterruptTaskID, d.ReturnStatus,
		d.Reason, d.NextActionDate, rem, d.ForceReason)
	if err != nil && strings.Contains(err.Error(), "no such column") {
		_, err = r.db.Exec(`
			INSERT INTO work_task_departures (departure_id, task_id, user_id, assignee, work_date, start_time, end_time, interrupt_task_id, return_status)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			id, d.TaskID, d.UserID, d.Assignee, d.WorkDate, d.StartTime, d.EndTime, d.InterruptTaskID, d.ReturnStatus)
	}
	return id, err
}

func (r *WBRepo) GetDeparture(id string) (*model.WorkTaskDeparture, error) {
	if r == nil || r.db == nil || strings.TrimSpace(id) == "" {
		return nil, nil
	}
	row := r.db.QueryRow(`
		SELECT departure_id, task_id, user_id, assignee, work_date, start_time, end_time, interrupt_task_id, return_status,
		       COALESCE(reason,''), COALESCE(next_action_date,''), remaining_min, COALESCE(force_reason,'')
		FROM work_task_departures WHERE departure_id=?`, id)
	var d model.WorkTaskDeparture
	var rem sql.NullInt64
	err := row.Scan(&d.DepartureID, &d.TaskID, &d.UserID, &d.Assignee, &d.WorkDate, &d.StartTime, &d.EndTime, &d.InterruptTaskID, &d.ReturnStatus,
		&d.Reason, &d.NextActionDate, &rem, &d.ForceReason)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil && strings.Contains(err.Error(), "no such column") {
		row = r.db.QueryRow(`SELECT departure_id, task_id, user_id, assignee, work_date, start_time, end_time, interrupt_task_id, return_status
			FROM work_task_departures WHERE departure_id=?`, id)
		err = row.Scan(&d.DepartureID, &d.TaskID, &d.UserID, &d.Assignee, &d.WorkDate, &d.StartTime, &d.EndTime, &d.InterruptTaskID, &d.ReturnStatus)
		if err == sql.ErrNoRows {
			return nil, nil
		}
	}
	if err != nil {
		return nil, err
	}
	if rem.Valid {
		v := int(rem.Int64)
		d.RemainingMin = &v
	}
	return &d, nil
}

func (r *WBRepo) UpdateDepartureLive(d model.WorkTaskDeparture) error {
	var rem any
	if d.RemainingMin != nil {
		rem = *d.RemainingMin
	}
	_, err := r.db.Exec(`UPDATE work_task_departures SET return_status=?, reason=?, next_action_date=?, remaining_min=?, force_reason=?,
		end_time=?, updated_at=CURRENT_TIMESTAMP WHERE departure_id=?`,
		d.ReturnStatus, d.Reason, d.NextActionDate, rem, d.ForceReason, d.EndTime, d.DepartureID)
	if err != nil && strings.Contains(err.Error(), "no such column") {
		return r.SetDepartureReturn(d.DepartureID, d.ReturnStatus)
	}
	return err
}

func (r *WBRepo) ListPendingUrgentForWorker(workerID, workerName string) ([]model.WorkTask, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	where := `
		WHERE COALESCE(t.status,'') NOT IN ('complete','cancelled')
		  AND COALESCE(t.priority,'') = ?
		  AND (TRIM(COALESCE(t.assignee_user_id,''))=? OR t.assignee=?)` + workTaskOrder
	return r.listStoredTasks(where, model.WBPriorityUrgent, workerID, workerName)
}

func (r *WBRepo) RestoreActualSegmentDates(taskID string, dates map[string]string) error {
	if r == nil || r.db == nil || len(dates) == 0 {
		return nil
	}
	for id, d := range dates {
		if _, err := r.db.Exec(`UPDATE work_task_segments SET work_date=? WHERE segment_id=? AND kind='actual'`, d, id); err != nil {
			return err
		}
	}
	return nil
}

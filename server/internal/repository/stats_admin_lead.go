package repository

import (
	"strings"
	"time"

	"customer-support/internal/model"
)

type adminLeadRow struct {
	TaskID    string
	Receipt   string
	Complete  string
	WorkMin   int
}

type adminLeadAct struct {
	TaskID   string
	ActionID string
	Type     string
	At       string
}

type adminLeadAction struct {
	TaskID    string
	ActionID  string
	CreatedAt string
	WaitParty string
	Status    string
}

func (r *StatsRepo) loadAdminLeadBreakdown(from, toEx string, f model.StatsMeetingFilter) (model.AdminLeadBreakdown, error) {
	var out model.AdminLeadBreakdown
	f = normalizeMeetingFilter(f)
	adminSQL, adminArgs := r.filterAdmin(f)
	q := `
		SELECT t.task_id,
		       COALESCE(` + adminTaskReceiptDateSQL + `,''),
		       COALESCE(` + adminTaskCompleteDateSQL + `,''),
		       COALESCE((SELECT SUM(COALESCE(y.spent_minutes,0)) FROM work_activities y WHERE y.task_id=t.task_id),0)
		FROM work_tasks t
		WHERE t.work_type IN ('admin','support') AND t.status='complete'
		  AND ` + adminTaskCompleteDateSQL + ` >= ? AND ` + adminTaskCompleteDateSQL + ` < ?` + adminSQL
	args := append([]interface{}{from, toEx}, adminArgs...)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			out.FillDisplays()
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	var tasks []adminLeadRow
	ids := make([]string, 0)
	for rows.Next() {
		var it adminLeadRow
		if err := rows.Scan(&it.TaskID, &it.Receipt, &it.Complete, &it.WorkMin); err != nil {
			return out, err
		}
		tasks = append(tasks, it)
		ids = append(ids, it.TaskID)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	actsByTask, err := r.listAdminLeadActivities(ids)
	if err != nil {
		return out, err
	}
	actionsByTask, err := r.listAdminLeadActions(ids)
	if err != nil {
		return out, err
	}

	for _, t := range tasks {
		lead, ok := model.AdminLeadTimeDays(t.Receipt, t.Complete)
		if !ok {
			continue
		}
		closeAt := completeCloseAt(t.Complete)
		var spans []model.DateInterval
		acts := actsByTask[t.TaskID]
		byAction := map[string][]model.WorkActivity{}
		var unlinked []model.WorkActivity
		for _, a := range acts {
			wa := model.WorkActivity{ActivityType: a.Type, CreatedAt: a.At, ActionID: a.ActionID, TaskID: a.TaskID}
			if strings.TrimSpace(a.ActionID) == "" {
				unlinked = append(unlinked, wa)
				continue
			}
			byAction[a.ActionID] = append(byAction[a.ActionID], wa)
		}
		seen := map[string]bool{}
		for _, ac := range actionsByTask[t.TaskID] {
			seen[ac.ActionID] = true
			fb := time.Time{}
			if strings.TrimSpace(ac.WaitParty) != "" || ac.Status == model.WBActionWaiting {
				fb, _ = model.ParseActivityTime(ac.CreatedAt)
				if fb.IsZero() {
					fb, _ = model.ParseActivityTime(t.Receipt)
				}
			}
			spans = append(spans, model.PairActionWaitIntervals(byAction[ac.ActionID], fb, closeAt)...)
		}
		for aid, list := range byAction {
			if seen[aid] {
				continue
			}
			fb, _ := model.ParseActivityTime(t.Receipt)
			spans = append(spans, model.PairActionWaitIntervals(list, fb, closeAt)...)
		}
		if len(unlinked) > 0 {
			fb, _ := model.ParseActivityTime(t.Receipt)
			spans = append(spans, model.PairActionWaitIntervals(unlinked, fb, closeAt)...)
		}
		waitDays := model.IntervalDays(spans)
		leadDays := float64(lead)
		if waitDays > leadDays {
			waitDays = leadDays
		}
		workMin := t.WorkMin
		if workMin < 0 {
			workMin = 0
		}
		idle := int(leadDays*24*60+0.5) - workMin - int(waitDays*24*60+0.5)
		if idle < 0 {
			idle = 0
		}
		out.Sample++
		out.LeadDaysSum += leadDays
		out.WorkMinutes += workMin
		out.WaitDaysSum += waitDays
		out.IdleMinutes += idle
	}
	if out.Sample > 0 {
		out.LeadDaysAvg = out.LeadDaysSum / float64(out.Sample)
	}
	out.FillDisplays()
	return out, nil
}

func completeCloseAt(complete string) time.Time {
	t, ok := model.ParseActivityTime(complete)
	if !ok {
		return time.Time{}
	}
	return t.Add(24 * time.Hour)
}

func (r *StatsRepo) listAdminLeadActivities(ids []string) (map[string][]adminLeadAct, error) {
	out := map[string][]adminLeadAct{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT task_id, COALESCE(action_id,''), COALESCE(activity_type,''), COALESCE(created_at,'')
		FROM work_activities
		WHERE task_id IN (`+strings.Join(ph, ",")+`)
		ORDER BY created_at ASC, activity_id ASC`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a adminLeadAct
		if err := rows.Scan(&a.TaskID, &a.ActionID, &a.Type, &a.At); err != nil {
			return nil, err
		}
		out[a.TaskID] = append(out[a.TaskID], a)
	}
	return out, rows.Err()
}

func (r *StatsRepo) listAdminLeadActions(ids []string) (map[string][]adminLeadAction, error) {
	out := map[string][]adminLeadAction{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT task_id, action_id, COALESCE(created_at,''), COALESCE(wait_party,''), COALESCE(status,'')
		FROM work_actions
		WHERE task_id IN (`+strings.Join(ph, ",")+`)
		ORDER BY created_at ASC, action_id ASC`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a adminLeadAction
		if err := rows.Scan(&a.TaskID, &a.ActionID, &a.CreatedAt, &a.WaitParty, &a.Status); err != nil {
			return nil, err
		}
		out[a.TaskID] = append(out[a.TaskID], a)
	}
	return out, rows.Err()
}

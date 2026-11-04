package repository

import (
	"strings"
	"time"

	"customer-support/internal/model"
)

// AssigneeTimeOverlap 기간 안 날짜별 담당자 시간 겹침. 95-94.
type AssigneeTimeOverlap struct {
	Task     model.WorkTask
	Date     string
	Assignee string
}

// ListAssigneeTimeOverlapsRange 기간을 한 번 조회한 뒤 메모리에서 날짜별로 나눈다. N+1 금지.
func (r *WBRepo) ListAssigneeTimeOverlapsRange(from, to, start, end, excludeTaskID string, names []string) ([]AssigneeTimeOverlap, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if r == nil || from == "" || to == "" || to < from {
		return nil, nil
	}
	sm := model.ParseHHMMMinutes(start)
	em := model.ParseHHMMMinutes(end)
	if sm < 0 || em <= sm {
		return nil, nil
	}
	want := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			want[n] = true
		}
	}
	if len(want) == 0 {
		return nil, nil
	}
	placed, err := r.listTasksSpanningRange(from, to)
	if err != nil {
		return nil, err
	}
	members, _ := r.membersForTasks(placed)
	ids := taskIDsOfWork(placed)
	segMap, _ := r.ListSegmentsByTaskIDs(ids)
	var out []AssigneeTimeOverlap
	seen := map[string]bool{}
	for _, t := range placed {
		if excludeTaskID != "" && t.TaskID == excludeTaskID {
			continue
		}
		plan := segMap[t.TaskID]
		if len(plan) == 0 {
			v := model.VirtualPlanSegment(t)
			if v.StartTime != "" {
				plan = []model.WorkTaskSegment{v}
			}
		}
		deps, _ := r.ListDepartures(t.TaskID)
		mem := members[t.TaskID]
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" || !want[name] {
				continue
			}
			if !personOccupiesAt(t, name, mem, plan, deps, start, end) {
				continue
			}
			for _, day := range datesTaskOccupies(t, from, to) {
				key := t.TaskID + "|" + day + "|" + name
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, AssigneeTimeOverlap{Task: t, Date: day, Assignee: name})
			}
		}
	}
	return out, nil
}

func (r *WBRepo) listTasksSpanningRange(from, to string) ([]model.WorkTask, error) {
	where := `
		WHERE COALESCE(t.recurrence_role,'') != 'parent'
		  AND (
		    (TRIM(COALESCE(t.due_date,'')) = '' AND TRIM(COALESCE(t.work_date,'')) != '' AND t.work_date >= ? AND t.work_date <= ?)
		    OR (TRIM(COALESCE(t.due_date,'')) != '' AND COALESCE(NULLIF(TRIM(t.work_date),''), t.due_date) <= ? AND t.due_date >= ?)
		  )`
	return r.listStoredTasks(where+workTaskOrder, from, to, to, from)
}

func datesTaskOccupies(t model.WorkTask, from, to string) []string {
	var out []string
	for _, d := range enumerateInclusiveDates(from, to, 366) {
		if model.TaskSpansDate(t.WorkDate, t.DueDate, d) {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		day := strings.TrimSpace(t.WorkDate)
		if day == "" {
			day = strings.TrimSpace(t.DueDate)
		}
		if day >= from && day <= to {
			out = append(out, day)
		}
	}
	return out
}

func enumerateInclusiveDates(from, to string, max int) []string {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" || to < from {
		return nil
	}
	a, err1 := time.ParseInLocation("2006-01-02", from, time.Local)
	b, err2 := time.ParseInLocation("2006-01-02", to, time.Local)
	if err1 != nil || err2 != nil {
		return nil
	}
	var out []string
	for i := 0; !a.After(b); i++ {
		if i >= max {
			break
		}
		out = append(out, a.Format("2006-01-02"))
		a = a.AddDate(0, 0, 1)
	}
	return out
}

func taskIDsOfWork(tasks []model.WorkTask) []string {
	var ids []string
	for _, t := range tasks {
		if t.TaskID != "" {
			ids = append(ids, t.TaskID)
		}
	}
	return ids
}

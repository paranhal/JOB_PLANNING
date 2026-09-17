package repository

import (
	"strings"

	"customer-support/internal/model"
)

// §13.15.9 집계 단위. 조건을 빠뜨리면 상위와 실행 작업이 둘 다 세어져 이중 계상된다.
//
// 실행 작업 단위 (부모 제외): 예정 건수, 담당자별 일일업무, 일정표, 미계획함
// 상위 1건 단위 (실행 작업 접기): 주간보고 완료 건수, 전사 주간보고 F열
const (
	SQLRecurrenceWorkUnit   = ` AND COALESCE(t.recurrence_role,'') != 'parent'`
	SQLRecurrenceParentUnit = ` AND COALESCE(t.recurrence_role,'') != 'occurrence'`
)

func (r *StatsRepo) countAdminCompletedParents(from, toEx string, f model.StatsMeetingFilter) (int, error) {
	adminSQL, adminArgs := r.filterAdmin(f)
	q := `
		SELECT COUNT(*) FROM (
			SELECT DISTINCT COALESCE(NULLIF(TRIM(t.parent_task_id),''), t.task_id)
			FROM work_tasks t
			WHERE t.work_type IN ('admin','support') AND t.status='complete'
			  AND ` + adminTaskCompleteDateSQL + ` >= ? AND ` + adminTaskCompleteDateSQL + ` < ?` + adminSQL + `
		)`
	return r.countSQL(q, append([]interface{}{from, toEx}, adminArgs...)...)
}

func (r *StatsRepo) collapseWeeklyReportEvents(events []model.WeeklyEventRow) []model.WeeklyEventRow {
	parents := map[string]bool{}
	for _, ev := range events {
		if ev.Kind == model.WeeklyEventAdmin && strings.TrimSpace(ev.ParentTaskID) != "" {
			parents[ev.ParentTaskID] = true
		}
	}
	if len(parents) == 0 {
		return events
	}
	wb := NewWBRepo(r.db)
	totalBy := map[string]int{}
	doneBy := map[string]int{}
	for id := range parents {
		n, _ := wb.CountOccurrences(id)
		totalBy[id] = n
		d, _ := wb.CountCompleteOccurrences(id)
		doneBy[id] = d
	}
	return collapseWeeklyAdminRecurrence(events, totalBy, doneBy)
}

func collapseWeeklyAdminRecurrence(events []model.WeeklyEventRow, totalByParent, doneByParent map[string]int) []model.WeeklyEventRow {
	if len(events) == 0 {
		return events
	}
	type grp struct {
		first model.WeeklyEventRow
		n     int
	}
	order := []string{}
	bag := map[string]*grp{}
	var rest []model.WeeklyEventRow
	for _, ev := range events {
		if ev.Kind != model.WeeklyEventAdmin || strings.TrimSpace(ev.ParentTaskID) == "" {
			rest = append(rest, ev)
			continue
		}
		key := ev.ParentTaskID
		g, ok := bag[key]
		if !ok {
			g = &grp{first: ev}
			bag[key] = g
			order = append(order, key)
		}
		g.n++
		if ev.OccurDate < g.first.OccurDate {
			g.first.OccurDate = ev.OccurDate
		}
	}
	out := append([]model.WeeklyEventRow{}, rest...)
	for _, key := range order {
		g := bag[key]
		ev := g.first
		ev.WorkNo = key
		total := totalByParent[key]
		if total < g.n {
			total = g.n
		}
		done := doneByParent[key]
		if done < 1 {
			done = g.n
		}
		ev.Content = model.FormatRecurrenceProgress(ev.Content, total, done)
		out = append(out, ev)
	}
	return out
}

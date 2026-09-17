package repository

import (
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

// BuildDailyAssigneeReport §16.4 담당자별 일일업무. 완료 건수·소요분 매트릭스 + 일일 상세.
func (r *StatsRepo) BuildDailyAssigneeReport(from, toEx, fileDay string) (model.DailyAssigneeReport, error) {
	toInc := exclusiveToInclusive(toEx)
	wd, yearMissing, err := r.CountWorkingDays(from, toEx)
	if err != nil {
		return model.DailyAssigneeReport{}, err
	}
	out := model.DailyAssigneeReport{
		From:               from,
		To:                 toInc,
		ToEx:               toEx,
		FileDay:            fileDay,
		WorkingDays:        wd,
		HolidayYearMissing: yearMissing,
	}
	if out.FileDay == "" {
		out.FileDay = strings.ReplaceAll(toInc, "-", "")
	}

	holi, err := r.holidayDatesInRange(from, toEx)
	if err != nil {
		return out, err
	}
	start, err := time.ParseInLocation("2006-01-02", from, time.Local)
	if err != nil {
		return out, err
	}
	endEx, err := time.ParseInLocation("2006-01-02", toEx, time.Local)
	if err != nil {
		return out, err
	}
	for d := start; !d.After(endEx.AddDate(0, 0, -1)); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		off := d.Weekday() == time.Saturday || d.Weekday() == time.Sunday || holi[ds]
		out.Days = append(out.Days, model.DailyAssigneeDay{
			Date: ds, Label: model.DateLabelMDW(ds), Off: off,
		})
	}

	counts, err := r.listCompletedByAssigneeDate(from, toEx)
	if err != nil {
		return out, err
	}
	minutes, err := r.listMinutesByAssigneeDate(from, toEx)
	if err != nil {
		return out, err
	}

	leaves, err := r.listLeavesByAssigneeDate(from, toEx)
	if err != nil {
		return out, err
	}

	names := map[string]bool{}
	for a := range counts {
		names[a] = true
	}
	for a := range minutes {
		names[a] = true
	}
	for a := range leaves {
		if a != "" && a != model.StatsUnassignedLabel {
			names[a] = true
		}
	}
	var people []string
	for a := range names {
		people = append(people, a)
	}
	sort.SliceStable(people, func(i, j int) bool {
		ui, uj := people[i] == model.StatsUnassignedLabel, people[j] == model.StatsUnassignedLabel
		if ui != uj {
			return !ui
		}
		ci, cj := sumWorking(counts[people[i]], out.Days), sumWorking(counts[people[j]], out.Days)
		if ci != cj {
			return ci > cj
		}
		return people[i] < people[j]
	})

	nDay := len(out.Days)
	out.TeamCounts = make([]int, nDay)
	out.TeamMinutes = make([]int, nDay)
	for _, name := range people {
		p := model.DailyAssigneePerson{
			Label:      name,
			Unassigned: name == model.StatsUnassignedLabel,
			Counts:     make([]int, nDay),
			Minutes:    make([]int, nDay),
			LeaveMarks: make([]string, nDay),
		}
		personWd := 0
		for i, day := range out.Days {
			c := counts[name][day.Date]
			m := minutes[name][day.Date]
			p.Counts[i] = c
			p.Minutes[i] = m
			out.TeamCounts[i] += c
			out.TeamMinutes[i] += m
			onLeave := !day.Off && leaves[name][day.Date] != ""
			if onLeave {
				p.LeaveMarks[i] = model.LeaveCellMark(leaves[name][day.Date])
			}
			if day.Off {
				if c > p.CountMax {
					p.CountMax = c
				}
				if m > p.MinuteMax {
					p.MinuteMax = m
				}
				continue
			}
			if onLeave {
				continue
			}
			personWd++
			p.CountTotal += c
			p.MinuteTotal += m
			if c > p.CountMax {
				p.CountMax = c
			}
			if m > p.MinuteMax {
				p.MinuteMax = m
			}
		}
		if avg, ok := model.DailyAvgCompleted(p.CountTotal, personWd); ok {
			p.HasCountAvg = true
			p.CountAvg = mathRound1(avg)
		}
		if avg, ok := model.DailyAvgCompleted(p.MinuteTotal, personWd); ok {
			p.HasMinuteAvg = true
			p.MinuteAvg = mathRound1(avg)
		}
		out.People = append(out.People, p)
	}
	for i, day := range out.Days {
		if !day.Off {
			out.TeamCountTotal += out.TeamCounts[i]
			out.TeamMinuteTotal += out.TeamMinutes[i]
		}
		if out.TeamCounts[i] > out.TeamCountMax {
			out.TeamCountMax = out.TeamCounts[i]
		}
		if out.TeamMinutes[i] > out.TeamMinuteMax {
			out.TeamMinuteMax = out.TeamMinutes[i]
		}
	}
	if avg, ok := model.DailyAvgCompleted(out.TeamCountTotal, wd); ok {
		out.HasTeamCountAvg = true
		out.TeamCountAvg = mathRound1(avg)
	}
	if avg, ok := model.DailyAvgCompleted(out.TeamMinuteTotal, wd); ok {
		out.HasTeamMinuteAvg = true
		out.TeamMinuteAvg = mathRound1(avg)
	}

	events, err := r.ListWeeklyEventRows(from, toEx)
	if err != nil {
		return out, err
	}
	out.Details = buildDailyDetailRows(events)
	return out, nil
}

func sumWorking(byDate map[string]int, days []model.DailyAssigneeDay) int {
	n := 0
	for _, d := range days {
		if d.Off {
			continue
		}
		n += byDate[d.Date]
	}
	return n
}

func (r *StatsRepo) holidayDatesInRange(from, toEx string) (map[string]bool, error) {
	out := map[string]bool{}
	rows, err := r.db.Query(`SELECT holiday_date FROM holidays WHERE holiday_date >= ? AND holiday_date < ?`, from, toEx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return out, err
		}
		d = strings.TrimSpace(d)
		if len(d) >= 10 {
			d = d[:10]
		}
		if d != "" {
			out[d] = true
		}
	}
	return out, rows.Err()
}

// listLeavesByAssigneeDate 담당자 표시명 → 날짜 → leave_kind. §23.13.6 §16.4
func (r *StatsRepo) listLeavesByAssigneeDate(from, toEx string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	rows, err := r.db.Query(`
		SELECT COALESCE(NULLIF(TRIM(u.full_name),''), COALESCE(u.username,''), l.user_id),
		       l.leave_date, l.leave_kind
		FROM staff_leaves l
		LEFT JOIN users u ON u.user_id = l.user_id
		WHERE l.leave_date >= ? AND l.leave_date < ?`, from, toEx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, date, kind string
		if err := rows.Scan(&name, &date, &kind); err != nil {
			return out, err
		}
		name = weeklyAssigneeLabel(name)
		date = trimDate(date)
		if name == "" || date == "" {
			continue
		}
		if out[name] == nil {
			out[name] = map[string]string{}
		}
		out[name][date] = kind
	}
	return out, rows.Err()
}

func (r *StatsRepo) listCompletedByAssigneeDate(from, toEx string) (map[string]map[string]int, error) {
	out := map[string]map[string]int{}
	add := func(name, date string, n int) {
		name = weeklyAssigneeLabel(name)
		date = trimDate(date)
		if date == "" || n == 0 {
			return
		}
		if out[name] == nil {
			out[name] = map[string]int{}
		}
		out[name][date] += n
	}
	asSQL, asArgs := r.filterAS(model.StatsMeetingFilter{})
	qAS := `
		SELECT TRIM(COALESCE(ar.assigned_to,'')), ` + asCompleteDateSQL + `, COUNT(*)
		FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status IN ` + model.SQLStatusStatsCompleted + asSQL + `
		  AND ` + asCompleteDT + ` >= ?
		  AND ` + asCompleteDT + ` < ?
		GROUP BY 1, 2`
	if err := r.scanNamedDayCounts(qAS, from, toEx, add, asArgs...); err != nil {
		return out, err
	}
	mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{})
	qMnt := `
		SELECT TRIM(COALESCE(v.assignee,'')), COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date), COUNT(*)
		FROM maintenance_visits v
		WHERE COALESCE(v.completed,0)=1` + mntSQL + `
		  AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) >= ?
		  AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) < ?
		GROUP BY 1, 2`
	if err := r.scanNamedDayCounts(qMnt, from, toEx, add, mntArgs...); err != nil {
		return out, err
	}
	adminSQL, adminArgs := r.filterAdmin(model.StatsMeetingFilter{})
	qAdmin := `
		SELECT TRIM(COALESCE(t.assignee,'')), ` + adminTaskCompleteDateSQL + `, COUNT(*)
		FROM work_tasks t
		WHERE t.work_type IN ('admin','support') AND t.status='complete'
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + SQLRecurrenceWorkUnit + adminSQL + `
		  AND ` + adminTaskCompleteDateSQL + ` >= ? AND ` + adminTaskCompleteDateSQL + ` < ?
		GROUP BY 1, 2`
	if err := r.scanNamedDayCounts(qAdmin, from, toEx, add, adminArgs...); err != nil {
		return out, err
	}
	return out, nil
}

func (r *StatsRepo) listMinutesByAssigneeDate(from, toEx string) (map[string]map[string]int, error) {
	out := map[string]map[string]int{}
	add := func(name, date string, n int) {
		name = weeklyAssigneeLabel(name)
		date = trimDate(date)
		if date == "" || n == 0 {
			return
		}
		if out[name] == nil {
			out[name] = map[string]int{}
		}
		out[name][date] += n
	}
	asSQL, asArgs := r.filterAS(model.StatsMeetingFilter{})
	qAS := `
		SELECT TRIM(COALESCE(NULLIF(TRIM(p.worker),''), ar.assigned_to, '')), date(p.process_datetime), SUM(COALESCE(p.time_spent,0))
		FROM as_processes p
		JOIN as_receipts ar ON ar.as_id = p.as_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status != 'cancelled'` + asSQL + `
		  AND p.process_datetime IS NOT NULL
		  AND p.process_datetime >= ? AND p.process_datetime < ?
		GROUP BY 1, 2`
	if err := r.scanNamedDayCounts(qAS, from, toEx, add, asArgs...); err != nil {
		return out, err
	}
	adminSQL, adminArgs := r.filterAdmin(model.StatsMeetingFilter{})
	qAct := `
		SELECT TRIM(COALESCE(NULLIF(TRIM(a.actor),''), t.assignee, '')),
		       date(a.created_at),
		       SUM(COALESCE(a.spent_minutes,0))
		FROM work_activities a
		JOIN work_tasks t ON t.task_id = a.task_id
		WHERE t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + adminSQL + `
		  AND a.created_at >= ?
		  AND a.created_at < ?
		GROUP BY 1, 2`
	if err := r.scanNamedDayCounts(qAct, from, toEx, add, adminArgs...); err != nil {
		if !strings.Contains(err.Error(), "no such table") && !strings.Contains(err.Error(), "no such column") {
			return out, err
		}
	}
	return out, nil
}

func (r *StatsRepo) scanNamedDayCounts(q, from, toEx string, add func(name, date string, n int), extra ...interface{}) error {
	rows, err := r.db.Query(q, append(append([]interface{}{}, extra...), from, toEx)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, date string
		var n int
		if err := rows.Scan(&name, &date, &n); err != nil {
			return err
		}
		add(name, date, n)
	}
	return rows.Err()
}

func buildDailyDetailRows(events []model.WeeklyEventRow) []model.DailyAssigneeDetailRow {
	type key struct{ a, d string }
	grouped := map[key][]model.WeeklyEventRow{}
	var keys []key
	seen := map[key]bool{}
	for _, ev := range events {
		k := key{weeklyAssigneeLabel(ev.Assignee), trimDate(ev.OccurDate)}
		if k.d == "" {
			continue
		}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
		grouped[k] = append(grouped[k], ev)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].a != keys[j].a {
			ui, uj := keys[i].a == model.StatsUnassignedLabel, keys[j].a == model.StatsUnassignedLabel
			if ui != uj {
				return !ui
			}
			return keys[i].a < keys[j].a
		}
		return keys[i].d < keys[j].d
	})

	var out []model.DailyAssigneeDetailRow
	prevAssignee := ""
	for _, k := range keys {
		list := grouped[k]
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].OccurDate != list[j].OccurDate {
				return list[i].OccurDate < list[j].OccurDate
			}
			oi, oj := weeklyEventKindOrder(list[i].Kind), weeklyEventKindOrder(list[j].Kind)
			if oi != oj {
				return oi < oj
			}
			return list[i].WorkNo < list[j].WorkNo
		})
		wd := ""
		if t, err := time.ParseInLocation("2006-01-02", k.d, time.Local); err == nil {
			wd = model.WeekdayKR(t)
		}
		sumMin := 0
		cum := 0
		for i, ev := range list {
			cum += ev.Minutes
			sumMin += ev.Minutes
			row := model.DailyAssigneeDetailRow{
				Assignee:   k.a,
				Date:       k.d,
				Weekday:    wd,
				Kind:       ev.WorkType,
				WorkNo:     ev.WorkNo,
				Customer:   ev.Customer,
				Content:    ev.Content,
				Status:     dailyDetailStatus(ev),
				Minutes:    ev.Minutes,
				Cumulative: cum,
				NewPerson:  prevAssignee != k.a && i == 0,
			}
			out = append(out, row)
		}
		out = append(out, model.DailyAssigneeDetailRow{
			Assignee:   k.a,
			Date:       k.d,
			Weekday:    wd,
			Kind:       model.DailyDetailKindTotal,
			Status:     "",
			Minutes:    sumMin,
			Cumulative: sumMin,
			IsDayTotal: true,
		})
		prevAssignee = k.a
	}
	return out
}

func dailyDetailStatus(ev model.WeeklyEventRow) string {
	res := strings.TrimSpace(ev.Result)
	switch ev.Kind {
	case model.WeeklyEventComplete:
		return model.DailyStatusComplete
	case model.WeeklyEventReceipt:
		return model.DailyStatusReceipt
	case model.WeeklyEventAction:
		return model.DailyStatusProgress
	case model.WeeklyEventMnt:
		if res == "완료" || res == "종료" || res == "completed" {
			return model.DailyStatusComplete
		}
		return model.DailyStatusProgress
	}
	if res == "회신 대기" || res == "이월" {
		return model.DailyStatusCarry
	}
	if res == "완료" || res == "종료" {
		return model.DailyStatusComplete
	}
	if res == "접수" {
		return model.DailyStatusReceipt
	}
	return model.DailyStatusProgress
}

package repository

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"customer-support/internal/model"
)

// 소요시간(분): 처리이력 time_spent 합 > 0 이면 그 값, 아니면 조치시작→완료.
const statsDurationMinSQL = `COALESCE(
	(SELECT CASE WHEN SUM(COALESCE(p.time_spent,0)) > 0 THEN SUM(COALESCE(p.time_spent,0)) END
	 FROM as_processes p WHERE p.as_id = ar.as_id),
	CASE WHEN TRIM(COALESCE(ar.start_datetime,'')) != ''
	      AND TRIM(COALESCE(ar.complete_datetime,'')) != ''
	     THEN CAST(MAX(0, (julianday(ar.complete_datetime) - julianday(ar.start_datetime)) * 24 * 60) AS INTEGER)
	END
)`

const statsVisitRoundsSQL = `CASE
	WHEN (SELECT COUNT(*) FROM as_processes p WHERE p.as_id = ar.as_id) > 0
		THEN (SELECT COUNT(*) FROM as_processes p WHERE p.as_id = ar.as_id)
	WHEN TRIM(COALESCE(ar.start_datetime,'')) != '' THEN 1
	ELSE 0
END`

const statsVisitDateSQL = `COALESCE(date(NULLIF(TRIM(ar.start_datetime),'')), NULLIF(TRIM(ar.visit_scheduled_date),''))`

func statsCaseSelectSQL() string {
	return `
		ar.as_id, ar.as_number,
		COALESCE(date(ar.receipt_datetime),''),
		COALESCE(` + statsVisitDateSQL + `,''),
		COALESCE(date(ar.complete_datetime),''),
		COALESCE(c.org_name,''),
		COALESCE(ar.assigned_to,''),
		COALESCE(ar.symptom,''),
		COALESCE(ar.status,''),
		COALESCE(ar.urgency,''),
		COALESCE(ar.priority,''),
		COALESCE((` + statsDurationMinSQL + `), 0),
		(` + statsVisitRoundsSQL + `)`
}

// LoadStatsWorkAnalysis 선택 기간 접수·방문(조치시작)·완료·이월. AS+정기점검+행정.
func (r *StatsRepo) LoadStatsWorkAnalysis(from, toEx string, f model.StatsMeetingFilter) (model.StatsWorkAnalysis, error) {
	var out model.StatsWorkAnalysis
	f = normalizeMeetingFilter(f)

	asS, err := r.countASSlice(from, toEx, f)
	if err != nil {
		return out, err
	}
	mntS, err := r.countMntSlice(from, toEx, f)
	if err != nil {
		return out, err
	}
	adminS, err := r.countAdminSlice(from, toEx, f)
	if err != nil {
		return out, err
	}
	out.AS.Receipt = asS.Receipt
	out.AS.Completed = asS.Process
	out.Mnt.Receipt = mntS.Receipt
	out.Mnt.Visit = mntS.Process
	out.Mnt.Completed = mntS.Process
	out.Admin.Receipt = adminS.Receipt
	if n, err := r.countAdminCompletedParents(from, toEx, f); err != nil {
		return out, err
	} else {
		out.Admin.Completed = n
	}

	if out.AS.Visit, err = r.countASVisit(from, toEx, f); err != nil {
		return out, err
	}
	if out.Admin.Visit, err = r.countAdminVisit(from, toEx, f); err != nil {
		return out, err
	}
	if out.AS.CarryIn, err = r.countASCarry(from, from, f); err != nil {
		return out, err
	}
	if out.AS.CarryOut, err = r.countASCarry(toEx, toEx, f); err != nil {
		return out, err
	}
	if out.Mnt.CarryIn, err = r.countMntCarry(from, from, f); err != nil {
		return out, err
	}
	if out.Mnt.CarryOut, err = r.countMntCarry(toEx, toEx, f); err != nil {
		return out, err
	}
	if out.Admin.CarryIn, err = r.countAdminCarry(from, f); err != nil {
		return out, err
	}
	if out.Admin.CarryOut, err = r.countAdminCarry(toEx, f); err != nil {
		return out, err
	}
	out.Rollup()
	if lead, err := r.loadAdminLeadBreakdown(from, toEx, f); err != nil {
		return out, err
	} else {
		out.AdminLead = lead
	}
	return out, nil
}

func (r *StatsRepo) countASVisit(from, toEx string, f model.StatsMeetingFilter) (int, error) {
	asSQL, asArgs := r.filterAS(f)
	args := append(append([]interface{}{}, asArgs...), from, toEx)
	return r.countSQL(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status != 'cancelled'`+asSQL+`
		  AND TRIM(COALESCE(ar.start_datetime,'')) != ''
		  AND ar.start_datetime >= ? AND ar.start_datetime < ?`, args...)
}

func (r *StatsRepo) countASCarry(receiptBefore, stillOpenAt string, f model.StatsMeetingFilter) (int, error) {
	asSQL, asArgs := r.filterAS(f)
	args := append(append([]interface{}{}, asArgs...), receiptBefore, stillOpenAt)
	n, err := r.countSQL(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status != 'cancelled'`+asSQL+`
		  AND ar.receipt_datetime < ?
		  AND ar.status IN `+model.SQLStatusStatsOpen+`
		  AND (TRIM(COALESCE(ar.complete_datetime,'')) = ''
		       OR ar.complete_datetime >= ?)`, args...)
	if err != nil {
		return 0, err
	}
	wArgs := append(append([]interface{}{}, asArgs...), receiptBefore)
	w, err := r.countSQL(`
		SELECT COUNT(*) FROM as_work_items w
		JOIN as_receipts ar ON ar.as_id = w.as_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status = 'partial_complete'
		  AND COALESCE(w.status,'open') = 'open'
		  AND w.created_at < ?`+asSQL, wArgs...)
	return n + w, err
}

func (r *StatsRepo) countAdminVisit(from, toEx string, f model.StatsMeetingFilter) (int, error) {
	adminSQL, adminArgs := r.filterAdmin(f)
	args := append([]interface{}{from, toEx}, adminArgs...)
	return r.countSQL(`
		SELECT COUNT(*) FROM work_tasks t
		JOIN (
			SELECT task_id, MIN(created_at) AS first_at
			FROM work_activities
			GROUP BY task_id
		) y ON y.task_id = t.task_id
		WHERE t.work_type IN ('admin','support')
		  AND y.first_at >= ? AND y.first_at < ?`+adminSQL, args...)
}

func (r *StatsRepo) countMntCarry(visitBefore, notDoneBefore string, f model.StatsMeetingFilter) (int, error) {
	mntSQL, mntArgs := r.filterMnt(f)
	args := append([]interface{}{visitBefore, notDoneBefore}, mntArgs...)
	return r.countSQL(`
		SELECT COUNT(*) FROM maintenance_visits v
		WHERE v.visit_date < ?
		  AND NOT (
		    COALESCE(v.completed,0)=1
		    AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) < ?
		  )`+mntSQL, args...)
}

func (r *StatsRepo) countAdminCarry(dateBefore string, f model.StatsMeetingFilter) (int, error) {
	adminSQL, adminArgs := r.filterAdmin(f)
	dateExpr := `COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), date(t.created_at))`
	args := append([]interface{}{dateBefore}, adminArgs...)
	return r.countSQL(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND COALESCE(t.status,'') != 'complete'
		  AND `+dateExpr+` < ?
		  AND `+dateExpr+` IS NOT NULL AND TRIM(COALESCE(`+dateExpr+`,'')) != ''`+adminSQL, args...)
}

// LoadStatsSpotlight 최장 소요·1시간 초과·방문 차수·긴급·중요 상·일자 목록.
func (r *StatsRepo) LoadStatsSpotlight(from, toEx string, f model.StatsMeetingFilter, periodCount int) (model.StatsSpotlight, error) {
	f = normalizeMeetingFilter(f)
	out := model.StatsSpotlight{
		PeriodCount: periodCount,
		TopN:        model.StatsLongestTopN(periodCount),
	}
	var err error
	if out.Longest, err = r.listStatsCases(from, toEx, f, statsCaseLongest, out.TopN); err != nil {
		return out, err
	}
	if mntL, err := r.listMntCases(from, toEx, f, statsCaseLongest, out.TopN); err != nil {
		return out, err
	} else {
		out.Longest = mergeStatsRowsByDuration(out.TopN, out.Longest, mntL)
	}
	if admL, err := r.listAdminCases(from, toEx, f, statsCaseLongest, out.TopN); err != nil {
		return out, err
	} else {
		out.Longest = mergeStatsRowsByDuration(out.TopN, out.Longest, admL)
	}
	if out.OverHour, err = r.listStatsCases(from, toEx, f, statsCaseOverHour, 50); err != nil {
		return out, err
	}
	if mntO, err := r.listMntCases(from, toEx, f, statsCaseOverHour, 50); err != nil {
		return out, err
	} else {
		out.OverHour = mergeStatsRowsByDuration(50, out.OverHour, mntO)
	}
	if admO, err := r.listAdminCases(from, toEx, f, statsCaseOverHour, 50); err != nil {
		return out, err
	} else {
		out.OverHour = mergeStatsRowsByDuration(50, out.OverHour, admO)
	}
	if out.MostVisits, err = r.listStatsCases(from, toEx, f, statsCaseMostVisits, 20); err != nil {
		return out, err
	}
	if mntV, err := r.listMntCases(from, toEx, f, statsCaseMostVisits, 20); err != nil {
		return out, err
	} else {
		out.MostVisits = mergeStatsRowsByRounds(20, out.MostVisits, mntV)
	}
	if out.HighPriority, err = r.listStatsCases(from, toEx, f, statsCaseHighPriority, 50); err != nil {
		return out, err
	}
	if out.Timeline, err = r.listStatsCases(from, toEx, f, statsCaseTimeline, 100); err != nil {
		return out, err
	}
	if mntT, err := r.listMntCases(from, toEx, f, statsCaseTimeline, 100); err != nil {
		return out, err
	} else {
		out.Timeline = mergeStatsRowsByReceipt(100, out.Timeline, mntT)
	}
	if admT, err := r.listAdminCases(from, toEx, f, statsCaseTimeline, 100); err != nil {
		return out, err
	} else {
		out.Timeline = mergeStatsRowsByReceipt(100, out.Timeline, admT)
	}
	return out, nil
}

type statsCaseKind int

const (
	statsCaseLongest statsCaseKind = iota
	statsCaseOverHour
	statsCaseMostVisits
	statsCaseHighPriority
	statsCaseTimeline
	statsCaseCompleted
)

func (r *StatsRepo) listStatsCases(from, toEx string, f model.StatsMeetingFilter, kind statsCaseKind, limit int) ([]model.StatsRow, error) {
	if limit < 1 {
		limit = 1
	}
	asSQL, asArgs := r.filterAS(f)
	where := `
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status != 'cancelled'` + asSQL
	touched := `
		  AND ar.receipt_datetime < ?
		  AND (TRIM(COALESCE(ar.complete_datetime,'')) = ''
		       OR ar.complete_datetime >= ?
		       OR ar.receipt_datetime >= ?)`
	order := "date(ar.receipt_datetime) DESC, ar.as_number DESC"
	extra := ""
	var extraArgs []interface{}

	switch kind {
	case statsCaseLongest, statsCaseOverHour:
		extra = `
		  AND ` + asCompleteDT + ` >= ?
		  AND ` + asCompleteDT + ` < ?
		  AND (` + statsDurationMinSQL + `) IS NOT NULL`
		if kind == statsCaseOverHour {
			extra += fmt.Sprintf(` AND (`+statsDurationMinSQL+`) > %d`, model.StatsDurationOverMin)
		}
		order = `(` + statsDurationMinSQL + `) DESC, ar.as_number DESC`
		extraArgs = []interface{}{from, toEx}
	case statsCaseMostVisits:
		extra = touched + ` AND (` + statsVisitRoundsSQL + `) >= 1`
		order = `(` + statsVisitRoundsSQL + `) DESC, ar.as_number DESC`
		extraArgs = []interface{}{toEx, from, from}
	case statsCaseHighPriority:
		extra = touched + ` AND COALESCE(ar.urgency,'') = 'high' AND COALESCE(ar.priority,'') = 'high'`
		extraArgs = []interface{}{toEx, from, from}
	default:
		extra = `
		  AND (
		    (ar.receipt_datetime >= ? AND ar.receipt_datetime < ?)
		    OR (TRIM(COALESCE(ar.start_datetime,'')) != '' AND ar.start_datetime >= ? AND ar.start_datetime < ?)
		    OR (TRIM(COALESCE(ar.complete_datetime,'')) != '' AND ar.complete_datetime >= ? AND ar.complete_datetime < ?)
		  )`
		extraArgs = []interface{}{from, toEx, from, toEx, from, toEx}
	}
	args := append(append([]interface{}{}, asArgs...), extraArgs...)

	q := `SELECT ` + statsCaseSelectSQL() + where + extra + ` ORDER BY ` + order + ` LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStatsCaseRows(rows)
}

func scanStatsCaseRows(rows *sql.Rows) ([]model.StatsRow, error) {
	var items []model.StatsRow
	for rows.Next() {
		var it model.StatsRow
		var receipt, visit, complete string
		var dur int
		if err := rows.Scan(
			&it.ASID, &it.ASNumber,
			&receipt, &visit, &complete,
			&it.CustomerName, &it.Assignee, &it.Symptom, &it.Status,
			&it.Urgency, &it.Priority, &dur, &it.VisitRounds,
		); err != nil {
			return nil, err
		}
		it.ReceiptDate = strings.TrimSpace(receipt)
		it.VisitDate = strings.TrimSpace(visit)
		it.CompleteDate = strings.TrimSpace(complete)
		it.DurationMin = dur
		if dur > 0 {
			it.DurationLabel = model.FormatStatsDuration(dur)
		}
		it.StatusLabel = statsStatusLabel(it.Status)
		if it.Assignee == "" {
			it.Assignee = "(미배정)"
		}
		if it.WorkForm == "" {
			it.WorkForm = "AS"
		}
		if it.Href == "" && it.ASID != "" {
			it.Href = "/as/" + it.ASID + "/action"
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

const mntDurationSQL = `COALESCE((
	SELECT CASE WHEN SUM(COALESCE(t.duration_min,0)) > 0 THEN SUM(COALESCE(t.duration_min,0)) END
	FROM work_tasks t WHERE t.source_type='maintenance' AND t.source_id=v.visit_id
), 0)`

func (r *StatsRepo) listMntCases(from, toEx string, f model.StatsMeetingFilter, kind statsCaseKind, limit int) ([]model.StatsRow, error) {
	if limit < 1 {
		limit = 1
	}
	mntSQL, mntArgs := r.filterMnt(f)
	extra := ""
	order := "v.visit_date DESC"
	var extraArgs []interface{}
	doneDate := `COALESCE(NULLIF(TRIM(v.completed_date),''), CASE WHEN COALESCE(v.completed,0)=1 THEN v.visit_date ELSE '' END)`
	switch kind {
	case statsCaseLongest, statsCaseOverHour, statsCaseCompleted:
		extra = ` AND COALESCE(v.completed,0)=1
		  AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) >= ?
		  AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) < ?`
		if kind == statsCaseLongest || kind == statsCaseOverHour {
			extra += ` AND (` + mntDurationSQL + `) > 0`
		}
		if kind == statsCaseOverHour {
			extra += fmt.Sprintf(` AND (`+mntDurationSQL+`) > %d`, model.StatsDurationOverMin)
		}
		order = `(` + mntDurationSQL + `) DESC, v.visit_date DESC`
		extraArgs = []interface{}{from, toEx}
	case statsCaseMostVisits:
		extra = ` AND v.visit_date >= ? AND v.visit_date < ?`
		order = "v.visit_date DESC"
		extraArgs = []interface{}{from, toEx}
	case statsCaseHighPriority:
		return nil, nil
	default:
		extra = `
		  AND (
		    (v.visit_date >= ? AND v.visit_date < ?)
		    OR (TRIM(COALESCE(v.completed_date,'')) != '' AND v.completed_date >= ? AND v.completed_date < ?)
		  )`
		extraArgs = []interface{}{from, toEx, from, toEx}
	}
	args := append(append([]interface{}{}, extraArgs...), mntArgs...)
	q := `
		SELECT v.visit_id, COALESCE(v.visit_id,''),
		       COALESCE(v.visit_date,''), COALESCE(v.visit_date,''),
		       COALESCE(` + doneDate + `,''),
		       COALESCE(c.org_name,''), COALESCE(v.assignee,''),
		       COALESCE(v.notes,''),
		       CASE WHEN COALESCE(v.completed,0)=1 THEN 'completed' ELSE 'in_progress' END,
		       '', '',
		       (` + mntDurationSQL + `), 1
		FROM maintenance_visits v
		JOIN customers c ON c.customer_id = v.customer_id
		WHERE 1=1` + extra + mntSQL + `
		ORDER BY ` + order + ` LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanStatsCaseRows(rows)
	for i := range items {
		items[i].WorkForm = "정기점검"
		items[i].Href = "/maintenance/visits/" + items[i].ASID + "/action"
		items[i].VisitRounds = 1
	}
	return items, err
}

func (r *StatsRepo) listAdminCases(from, toEx string, f model.StatsMeetingFilter, kind statsCaseKind, limit int) ([]model.StatsRow, error) {
	if limit < 1 {
		limit = 1
	}
	adminSQL, adminArgs := r.filterAdmin(f)
	planExpr := `COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), date(t.created_at))`
	extra := ""
	order := planExpr + " DESC"
	var extraArgs []interface{}
	switch kind {
	case statsCaseLongest, statsCaseOverHour, statsCaseCompleted:
		extra = ` AND t.status = 'complete'
		  AND ` + adminTaskCompleteDateSQL + ` >= ? AND ` + adminTaskCompleteDateSQL + ` < ?`
		if kind == statsCaseLongest || kind == statsCaseOverHour {
			extra += ` AND COALESCE(t.duration_min,0) > 0`
		}
		if kind == statsCaseOverHour {
			extra += fmt.Sprintf(` AND COALESCE(t.duration_min,0) > %d`, model.StatsDurationOverMin)
		}
		order = `COALESCE(t.duration_min,0) DESC`
		extraArgs = []interface{}{from, toEx}
	case statsCaseMostVisits, statsCaseHighPriority:
		return nil, nil
	default:
		extra = ` AND ` + planExpr + ` >= ? AND ` + planExpr + ` < ?`
		extraArgs = []interface{}{from, toEx}
	}
	args := append(append([]interface{}{}, extraArgs...), adminArgs...)
	q := `
		SELECT t.task_id, COALESCE(t.task_id,''),
		       COALESCE(` + adminTaskReceiptDateSQL + `,''),
		       COALESCE((SELECT date(MIN(y.created_at)) FROM work_activities y WHERE y.task_id=t.task_id), ''),
		       CASE WHEN t.status='complete' THEN COALESCE(` + adminTaskCompleteDateSQL + `,'') ELSE '' END,
		       COALESCE(NULLIF(TRIM(c.org_name),''), NULLIF(TRIM(t.customer_name),''), ''),
		       COALESCE(t.assignee,''),
		       COALESCE(t.title,''),
		       COALESCE(t.status,''),
		       CASE WHEN COALESCE(t.priority,'') IN ('high','urgent') THEN 'high' ELSE COALESCE(t.priority,'') END,
		       COALESCE(t.priority,''),
		       COALESCE(t.duration_min,0),
		       CASE WHEN TRIM(COALESCE(t.work_date,'')) != '' THEN 1 ELSE 0 END
		FROM work_tasks t
		LEFT JOIN customers c ON c.customer_id = t.customer_id
		WHERE t.work_type IN ('admin','support')` + extra + adminSQL + `
		ORDER BY ` + order + ` LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanStatsCaseRows(rows)
	for i := range items {
		items[i].WorkForm = "행정"
		items[i].Href = "/workboard/tasks/" + items[i].ASID
		items[i].StatusLabel = model.WBTaskStatusLabel(items[i].Status)
	}
	return items, err
}

func mergeStatsRowsByDuration(limit int, groups ...[]model.StatsRow) []model.StatsRow {
	return mergeStatsRows(limit, func(a, b model.StatsRow) bool {
		if a.DurationMin != b.DurationMin {
			return a.DurationMin > b.DurationMin
		}
		return a.ASNumber < b.ASNumber
	}, groups...)
}

func mergeStatsRowsByRounds(limit int, groups ...[]model.StatsRow) []model.StatsRow {
	return mergeStatsRows(limit, func(a, b model.StatsRow) bool {
		if a.VisitRounds != b.VisitRounds {
			return a.VisitRounds > b.VisitRounds
		}
		return a.ASNumber < b.ASNumber
	}, groups...)
}

func mergeStatsRowsByReceipt(limit int, groups ...[]model.StatsRow) []model.StatsRow {
	return mergeStatsRows(limit, func(a, b model.StatsRow) bool {
		if a.ReceiptDate != b.ReceiptDate {
			return a.ReceiptDate > b.ReceiptDate
		}
		return a.ASNumber < b.ASNumber
	}, groups...)
}

func mergeStatsRows(limit int, less func(a, b model.StatsRow) bool, groups ...[]model.StatsRow) []model.StatsRow {
	var all []model.StatsRow
	for _, g := range groups {
		all = append(all, g...)
	}
	sort.SliceStable(all, func(i, j int) bool { return less(all[i], all[j]) })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

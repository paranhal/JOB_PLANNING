package repository

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"customer-support/internal/model"
)

// BuildStatsPeriodColumns 일/주/월 기준 전·현·다음 3개 기간 열을 만든다.
func BuildStatsPeriodColumns(view string, anchor time.Time) []model.StatsPeriodColumn {
	loc := anchor.Location()
	y, m, d := anchor.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)

	switch view {
	case model.StatsViewWeek:
		wd := int(today.Weekday())
		if wd == 0 {
			wd = 7
		}
		mon := today.AddDate(0, 0, -(wd - 1))
		return []model.StatsPeriodColumn{
			weekCol("prev", "전주", mon.AddDate(0, 0, -7), true),
			weekCol("current", "금주", mon, false),
			weekCol("next", "차주", mon.AddDate(0, 0, 7), false),
		}
	case model.StatsViewMonth:
		start := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		return []model.StatsPeriodColumn{
			monthCol("prev", "전월", start.AddDate(0, -1, 0), true),
			monthCol("current", "당월", start, false),
			monthCol("next", "익월", start.AddDate(0, 1, 0), false),
		}
	default:
		return []model.StatsPeriodColumn{
			dayCol("prev", "전일", today.AddDate(0, 0, -1), true),
			dayCol("current", "오늘", today, false),
			dayCol("next", "익일", today.AddDate(0, 0, 1), false),
		}
	}
}

func dayCol(key, label string, day time.Time, showActual bool) model.StatsPeriodColumn {
	return model.StatsPeriodColumn{
		Key: key, Label: label, ShowActual: showActual,
		From: day.Format("2006-01-02"), ToExclusive: day.AddDate(0, 0, 1).Format("2006-01-02"),
		RangeLabel: day.Format("2006/01/02"),
	}
}

func weekCol(key, label string, mon time.Time, showActual bool) model.StatsPeriodColumn {
	sun := mon.AddDate(0, 0, 6)
	return model.StatsPeriodColumn{
		Key: key, Label: label, ShowActual: showActual,
		From: mon.Format("2006-01-02"), ToExclusive: mon.AddDate(0, 0, 7).Format("2006-01-02"),
		RangeLabel: fmt.Sprintf("%s~%s", mon.Format("01/02"), sun.Format("01/02")),
	}
}

func monthCol(key, label string, start time.Time, showActual bool) model.StatsPeriodColumn {
	return model.StatsPeriodColumn{
		Key: key, Label: label, ShowActual: showActual,
		From: start.Format("2006-01-02"), ToExclusive: start.AddDate(0, 1, 0).Format("2006-01-02"),
		RangeLabel: start.Format("2006/01"),
	}
}

// FillPeriodOverview 각 열 집계. 과거 열은 daily_meeting_stats에 저장.
func (r *StatsRepo) FillPeriodOverview(cols []model.StatsPeriodColumn, f model.StatsMeetingFilter) error {
	f = normalizeMeetingFilter(f)
	today := time.Now().Format("2006-01-02")
	for i := range cols {
		c, err := r.countBucket(cols[i].From, cols[i].ToExclusive, f)
		if err != nil {
			return err
		}
		cols[i].Counts = c
		// 하루 단위·이미 지난 날만 daily_meeting_stats에 저장
		if isSingleDayCol(cols[i]) && cols[i].From < today {
			_ = r.UpsertDailyMeetingStat(cols[i], f)
		}
	}
	return nil
}

func isSingleDayCol(col model.StatsPeriodColumn) bool {
	from, err1 := time.ParseInLocation("2006-01-02", col.From, time.Local)
	toEx, err2 := time.ParseInLocation("2006-01-02", col.ToExclusive, time.Local)
	if err1 != nil || err2 != nil {
		return false
	}
	return toEx.Equal(from.AddDate(0, 0, 1))
}

func normalizeMeetingFilter(f model.StatsMeetingFilter) model.StatsMeetingFilter {
	f.ProjectID = strings.TrimSpace(f.ProjectID)
	switch f.Scope {
	case model.StatsScopeAssignee, model.StatsScopeWorkType, model.StatsScopeProduct:
		f.Key = strings.TrimSpace(f.Key)
	default:
		f.Scope = model.StatsScopeTeam
		f.Key = ""
	}
	return f
}

func (r *StatsRepo) countBucket(from, toEx string, f model.StatsMeetingFilter) (model.StatsBucketCounts, error) {
	var b model.StatsBucketCounts
	f = normalizeMeetingFilter(f)
	wt := ""
	if f.Scope == model.StatsScopeWorkType {
		wt = f.Key
	}

	var err error
	if wt == "" || wt == "as" {
		if b.AS, err = r.countASSlice(from, toEx, f); err != nil {
			return b, err
		}
	}
	if wt == "" || wt == "maintenance" {
		if b.Mnt, err = r.countMntSlice(from, toEx, f); err != nil {
			return b, err
		}
	}
	if wt == "" || wt == "admin" {
		if b.Admin, err = r.countAdminSlice(from, toEx, f); err != nil {
			return b, err
		}
	}
	return b, nil
}

func (r *StatsRepo) countASSlice(from, toEx string, f model.StatsMeetingFilter) (model.StatsWorkSlice, error) {
	var s model.StatsWorkSlice
	if f.Scope == model.StatsScopeWorkType && f.Key != "" && f.Key != "as" {
		return s, nil
	}
	asSQL, asArgs := asFilterSQL(f)
	argsP := append([]interface{}{from, toEx}, asArgs...)
	n, err := r.countSQL(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE TRIM(COALESCE(ar.visit_scheduled_date,'')) != ''
		  AND ar.visit_scheduled_date >= ? AND ar.visit_scheduled_date < ?
		  AND ar.status NOT IN ('cancelled')`+asSQL, argsP...)
	if err != nil {
		return s, err
	}
	s.Planned = n

	argsR := append([]interface{}{from, toEx}, asArgs...)
	if s.Receipt, err = r.countSQL(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE date(ar.receipt_datetime) >= date(?) AND date(ar.receipt_datetime) < date(?)`+asSQL, argsR...); err != nil {
		return s, err
	}
	// 부분완료 하위업무도 접수로 합산
	workRec, err := r.countSQL(`
		SELECT COUNT(*) FROM as_work_items w
		JOIN as_receipts ar ON ar.as_id = w.as_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status = 'partial_complete'
		  AND date(w.created_at) >= date(?) AND date(w.created_at) < date(?)`+asSQL, argsR...)
	if err != nil {
		return s, err
	}
	s.Receipt += workRec
	if s.Process, err = r.countSQL(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status IN ` + model.SQLStatusStatsCompleted + `
		  AND date(COALESCE(ar.complete_datetime, ar.updated_at)) >= date(?)
		  AND date(COALESCE(ar.complete_datetime, ar.updated_at)) < date(?)`+asSQL, argsR...); err != nil {
		return s, err
	}
	if s.Modified, err = r.countSQL(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status NOT IN ('cancelled')
		  AND TRIM(COALESCE(ar.visit_scheduled_date,'')) != ''
		  AND ar.visit_scheduled_date >= ? AND ar.visit_scheduled_date < ?
		  AND ar.status IN ` + model.SQLStatusStatsCompleted + `
		  AND date(COALESCE(ar.complete_datetime, ar.updated_at)) != date(ar.visit_scheduled_date)`+asSQL, argsP...); err != nil {
		return s, err
	}
	// 계획대로: 예정일에 그대로 완료
	if s.OnPlan, err = r.countSQL(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE TRIM(COALESCE(ar.visit_scheduled_date,'')) != ''
		  AND ar.visit_scheduled_date >= ? AND ar.visit_scheduled_date < ?
		  AND ar.status IN ` + model.SQLStatusStatsCompleted + `
		  AND date(COALESCE(ar.complete_datetime, ar.updated_at)) = date(ar.visit_scheduled_date)`+asSQL, argsP...); err != nil {
		return s, err
	}
	return s, nil
}

func (r *StatsRepo) countMntSlice(from, toEx string, f model.StatsMeetingFilter) (model.StatsWorkSlice, error) {
	var s model.StatsWorkSlice
	mntSQL, mntArgs := mntFilterSQL(f)
	args := append([]interface{}{from, toEx}, mntArgs...)
	n, err := r.countSQL(`
		SELECT COUNT(*) FROM maintenance_visits v
		WHERE v.visit_date >= ? AND v.visit_date < ?`+mntSQL, args...)
	if err != nil {
		return s, err
	}
	s.Planned = n
	s.Receipt = n
	if s.Process, err = r.countSQL(`
		SELECT COUNT(*) FROM maintenance_visits v
		WHERE COALESCE(v.completed,0)=1
		  AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) >= ?
		  AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) < ?`+mntSQL, args...); err != nil {
		return s, err
	}
	if s.Modified, err = r.countSQL(`
		SELECT COUNT(*) FROM maintenance_visits v
		WHERE v.visit_date >= ? AND v.visit_date < ?
		  AND COALESCE(v.completed,0)=1
		  AND TRIM(COALESCE(v.completed_date,'')) != ''
		  AND v.completed_date != v.visit_date`+mntSQL, args...); err != nil {
		return s, err
	}
	// 계획대로: 방문일에 완료(완료일 비어 있으면 방문일 완료로 간주)
	if s.OnPlan, err = r.countSQL(`
		SELECT COUNT(*) FROM maintenance_visits v
		WHERE v.visit_date >= ? AND v.visit_date < ?
		  AND COALESCE(v.completed,0)=1
		  AND (TRIM(COALESCE(v.completed_date,'')) = '' OR v.completed_date = v.visit_date)`+mntSQL, args...); err != nil {
		return s, err
	}
	return s, nil
}

func (r *StatsRepo) countAdminSlice(from, toEx string, f model.StatsMeetingFilter) (model.StatsWorkSlice, error) {
	var s model.StatsWorkSlice
	adminSQL, adminArgs := adminFilterSQL(f)
	args := append([]interface{}{from, toEx}, adminArgs...)
	n, err := r.countSQL(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') >= ?
		  AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') < ?`+adminSQL, args...)
	if err != nil {
		return s, err
	}
	s.Planned = n
	s.Receipt = n
	if s.Process, err = r.countSQL(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support') AND t.status = 'complete'
		  AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') >= ?
		  AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') < ?`+adminSQL, args...); err != nil {
		return s, err
	}
	modArgs := append([]interface{}{from, toEx, from, toEx}, adminArgs...)
	if s.Modified, err = r.countSQL(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.due_date,'')) != '' AND TRIM(COALESCE(t.work_date,'')) != ''
		  AND t.due_date != t.work_date
		  AND ((t.work_date >= ? AND t.work_date < ?) OR (t.due_date >= ? AND t.due_date < ?))`+adminSQL, modArgs...); err != nil {
		return s, err
	}
	// 계획대로: 예정일(due) 기준 기간 안이며 배정일=예정일로 완료
	if s.OnPlan, err = r.countSQL(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support') AND t.status = 'complete'
		  AND TRIM(COALESCE(t.due_date,'')) != ''
		  AND t.due_date >= ? AND t.due_date < ?
		  AND (TRIM(COALESCE(t.work_date,'')) = '' OR t.work_date = t.due_date)`+adminSQL, args...); err != nil {
		return s, err
	}
	return s, nil
}

func (r *StatsRepo) countSQL(q string, args ...interface{}) (int, error) {
	var n int
	err := r.db.QueryRow(q, args...).Scan(&n)
	return n, err
}

// UpsertDailyMeetingStat 과거 일자 회의 집계 저장
func (r *StatsRepo) UpsertDailyMeetingStat(col model.StatsPeriodColumn, f model.StatsMeetingFilter) error {
	f = normalizeMeetingFilter(f)
	c := col.Counts
	rate := math.Round(c.ExecutionRatePct()*10) / 10
	_, err := r.db.Exec(`
		INSERT INTO daily_meeting_stats (
			stat_date, scope, scope_key,
			planned, receipt, process, modified, execution_rate,
			as_planned, as_receipt, as_process,
			mnt_planned, mnt_receipt, mnt_process,
			admin_planned, admin_receipt, admin_process,
			computed_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,datetime('now','localtime'))
		ON CONFLICT(stat_date, scope, scope_key) DO UPDATE SET
			planned=excluded.planned, receipt=excluded.receipt, process=excluded.process,
			modified=excluded.modified, execution_rate=excluded.execution_rate,
			as_planned=excluded.as_planned, as_receipt=excluded.as_receipt, as_process=excluded.as_process,
			mnt_planned=excluded.mnt_planned, mnt_receipt=excluded.mnt_receipt, mnt_process=excluded.mnt_process,
			admin_planned=excluded.admin_planned, admin_receipt=excluded.admin_receipt, admin_process=excluded.admin_process,
			computed_at=excluded.computed_at`,
		col.From, f.Scope, f.Key,
		c.PlannedTotal(), c.ReceiptTotal(), c.ProcessTotal(), c.ModifiedTotal(), rate,
		c.AS.Planned, c.AS.Receipt, c.AS.Process,
		c.Mnt.Planned, c.Mnt.Receipt, c.Mnt.Process,
		c.Admin.Planned, c.Admin.Receipt, c.Admin.Process,
	)
	return err
}

// GetDailyMeetingStat 저장된 일 단위 집계
func (r *StatsRepo) GetDailyMeetingStat(statDate string, f model.StatsMeetingFilter) (*model.DailyMeetingStat, error) {
	f = normalizeMeetingFilter(f)
	row := r.db.QueryRow(`
		SELECT stat_date, scope, scope_key, planned, receipt, process, modified, execution_rate,
		       as_planned, as_receipt, as_process, mnt_planned, mnt_receipt, mnt_process,
		       admin_planned, admin_receipt, admin_process
		FROM daily_meeting_stats WHERE stat_date=? AND scope=? AND scope_key=?`,
		statDate, f.Scope, f.Key)
	var s model.DailyMeetingStat
	err := row.Scan(
		&s.StatDate, &s.Scope, &s.ScopeKey, &s.Planned, &s.Receipt, &s.Process, &s.Modified, &s.ExecutionRate,
		&s.ASPlanned, &s.ASReceipt, &s.ASProcess, &s.MntPlanned, &s.MntReceipt, &s.MntProcess,
		&s.AdminPlanned, &s.AdminReceipt, &s.AdminProcess,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ParseStatsAnchor 요약 화면 기준일(또는 월) 파싱
func ParseStatsAnchor(view, dateStr, monthStr string, now time.Time) time.Time {
	loc := now.Location()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)
	switch view {
	case model.StatsViewMonth:
		ms := strings.TrimSpace(monthStr)
		if len(ms) >= 7 {
			if t, err := time.ParseInLocation("2006-01", ms[:7], loc); err == nil {
				return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
			}
		}
		if ds := strings.TrimSpace(dateStr); ds != "" {
			if t, err := time.ParseInLocation("2006-01-02", ds, loc); err == nil {
				return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
			}
		}
		return time.Date(y, m, 1, 0, 0, 0, 0, loc)
	default:
		if t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(dateStr), loc); err == nil {
			return t
		}
		return today
	}
}

// ParseMeetingFilter 쿼리 → 필터
func ParseMeetingFilter(scope, key, projectID string) model.StatsMeetingFilter {
	return normalizeMeetingFilter(model.StatsMeetingFilter{Scope: scope, Key: key, ProjectID: projectID})
}

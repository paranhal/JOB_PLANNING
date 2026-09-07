package repository

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

// StatsProductOptions 업무별(제품) 선택 목록
func StatsProductOptions() []string {
	return []string{"앤로보틱스", "KLAS", "세종 K-LAS"}
}

// StatsChartBucket 차트 X축 1칸(일·주·월)
type StatsChartBucket struct {
	Label string // 표시 라벨
	From  string // inclusive YYYY-MM-DD
	ToEx  string // exclusive YYYY-MM-DD
}

func chartRangeLabel(from, toEx string, monthMode bool) string {
	start, err1 := time.ParseInLocation("2006-01-02", from, time.Local)
	endEx, err2 := time.ParseInLocation("2006-01-02", toEx, time.Local)
	if err1 != nil || err2 != nil {
		return from
	}
	if monthMode {
		return start.Format("2006-01")
	}
	end := endEx.AddDate(0, 0, -1)
	return start.Format("01/02") + "~" + end.Format("01/02")
}

// BuildStatsChartBuckets 차트 X축.
// 기준일(anchor) 기준:
//
//	일별: 기준일 포함 최근 14일(MM/DD)
//	주별: 기준일이 속한 주의 월요일을「이번주」로, 전전주·전주·이번주(월~일)
//	월별: 기준일이 속한 달을「이번달」로, 전전월·전월·이번달
func BuildStatsChartBuckets(view string, anchor time.Time) (from, toEx string, buckets []StatsChartBucket) {
	loc := anchor.Location()
	y, m, d := anchor.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, loc)
	switch view {
	case model.StatsViewWeek:
		wd := int(day.Weekday())
		if wd == 0 {
			wd = 7
		}
		thisMon := day.AddDate(0, 0, -(wd - 1))
		names := []string{"전전주", "전주", "이번주"}
		for i := 0; i < 3; i++ {
			mon := thisMon.AddDate(0, 0, -7*(2-i))
			sunEx := mon.AddDate(0, 0, 7)
			// 축: 전전주·전주·이번주 + 해당 주 시작일(월요일)
			buckets = append(buckets, StatsChartBucket{
				Label: names[i] + " " + mon.Format("01/02"),
				From:  mon.Format("2006-01-02"),
				ToEx:  sunEx.Format("2006-01-02"),
			})
		}
		from = buckets[0].From
		toEx = buckets[2].ToEx
		return from, toEx, buckets
	case model.StatsViewMonth:
		thisMonth := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		labels := []string{"전전월", "전월", "이번달"}
		for i := 0; i < 3; i++ {
			start := thisMonth.AddDate(0, -(2 - i), 0)
			end := start.AddDate(0, 1, 0)
			buckets = append(buckets, StatsChartBucket{
				Label: labels[i],
				From:  start.Format("2006-01-02"),
				ToEx:  end.Format("2006-01-02"),
			})
		}
		from = buckets[0].From
		toEx = buckets[2].ToEx
		return from, toEx, buckets
	default:
		// 일별: 기준일 포함 최근 14일
		end := day.AddDate(0, 0, 1)
		start := end.AddDate(0, 0, -14)
		from = start.Format("2006-01-02")
		toEx = end.Format("2006-01-02")
		for t := start; t.Before(end); t = t.AddDate(0, 0, 1) {
			buckets = append(buckets, StatsChartBucket{
				Label: t.Format("01/02"),
				From:  t.Format("2006-01-02"),
				ToEx:  t.AddDate(0, 0, 1).Format("2006-01-02"),
			})
		}
		return from, toEx, buckets
	}
}

// BuildStatsChartBucketsWindow §15.7 range 구간을 view 단위로 쪼갠다.
func BuildStatsChartBucketsWindow(view string, fromIncl, toIncl time.Time) (from, toEx string, buckets []StatsChartBucket) {
	loc := fromIncl.Location()
	start := time.Date(fromIncl.Year(), fromIncl.Month(), fromIncl.Day(), 0, 0, 0, 0, loc)
	endDay := time.Date(toIncl.Year(), toIncl.Month(), toIncl.Day(), 0, 0, 0, 0, loc)
	if endDay.Before(start) {
		endDay = start
	}
	endEx := endDay.AddDate(0, 0, 1)
	winFrom := start.Format("2006-01-02")
	winToEx := endEx.Format("2006-01-02")
	switch view {
	case model.StatsViewWeek:
		wd := int(start.Weekday())
		if wd == 0 {
			wd = 7
		}
		mon := start.AddDate(0, 0, -(wd - 1))
		for t := mon; t.Before(endEx); t = t.AddDate(0, 0, 7) {
			sunEx := t.AddDate(0, 0, 7)
			bFrom, bToEx := t, sunEx
			if bFrom.Before(start) {
				bFrom = start
			}
			if bToEx.After(endEx) {
				bToEx = endEx
			}
			if !bFrom.Before(bToEx) {
				continue
			}
			labelEnd := bToEx.AddDate(0, 0, -1)
			buckets = append(buckets, StatsChartBucket{
				Label: bFrom.Format("01/02") + "~" + labelEnd.Format("01/02"),
				From:  bFrom.Format("2006-01-02"),
				ToEx:  bToEx.Format("2006-01-02"),
			})
		}
	case model.StatsViewMonth:
		cur := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, loc)
		for cur.Before(endEx) {
			next := cur.AddDate(0, 1, 0)
			bFrom, bToEx := cur, next
			if bFrom.Before(start) {
				bFrom = start
			}
			if bToEx.After(endEx) {
				bToEx = endEx
			}
			if !bFrom.Before(bToEx) {
				cur = next
				continue
			}
			buckets = append(buckets, StatsChartBucket{
				Label: bFrom.Format("2006-01"),
				From:  bFrom.Format("2006-01-02"),
				ToEx:  bToEx.Format("2006-01-02"),
			})
			cur = next
		}
	default:
		for t := start; t.Before(endEx); t = t.AddDate(0, 0, 1) {
			buckets = append(buckets, StatsChartBucket{
				Label: t.Format("01/02"),
				From:  t.Format("2006-01-02"),
				ToEx:  t.AddDate(0, 0, 1).Format("2006-01-02"),
			})
		}
	}
	if len(buckets) == 0 {
		buckets = []StatsChartBucket{{
			Label: start.Format("01/02"),
			From:  winFrom,
			ToEx:  winToEx,
		}}
	}
	return winFrom, winToEx, buckets
}

// BuildStatsChartBucketsForRange 사용자 지정 기간 차트 버킷. ≤31일=일, ≤98일=주, 그 외=월.
func BuildStatsChartBucketsForRange(fromIncl, toIncl time.Time) (from, toEx string, buckets []StatsChartBucket) {
	loc := fromIncl.Location()
	start := time.Date(fromIncl.Year(), fromIncl.Month(), fromIncl.Day(), 0, 0, 0, 0, loc)
	endDay := time.Date(toIncl.Year(), toIncl.Month(), toIncl.Day(), 0, 0, 0, 0, loc)
	endEx := endDay.AddDate(0, 0, 1)
	days := int(endDay.Sub(start).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	switch {
	case days <= 31:
		from = start.Format("2006-01-02")
		toEx = endEx.Format("2006-01-02")
		for t := start; t.Before(endEx); t = t.AddDate(0, 0, 1) {
			buckets = append(buckets, StatsChartBucket{
				Label: t.Format("01/02"),
				From:  t.Format("2006-01-02"),
				ToEx:  t.AddDate(0, 0, 1).Format("2006-01-02"),
			})
		}
		return from, toEx, buckets
	case days <= 98:
		wd := int(start.Weekday())
		if wd == 0 {
			wd = 7
		}
		mon := start.AddDate(0, 0, -(wd - 1))
		for t := mon; t.Before(endEx); t = t.AddDate(0, 0, 7) {
			sunEx := t.AddDate(0, 0, 7)
			sun := sunEx.AddDate(0, 0, -1)
			buckets = append(buckets, StatsChartBucket{
				Label: t.Format("01/02") + "~" + sun.Format("01/02"),
				From:  t.Format("2006-01-02"),
				ToEx:  sunEx.Format("2006-01-02"),
			})
		}
		from = buckets[0].From
		toEx = buckets[len(buckets)-1].ToEx
		return from, toEx, buckets
	default:
		monthStart := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, loc)
		lastMonthEx := time.Date(endDay.Year(), endDay.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, 1, 0)
		for t := monthStart; t.Before(lastMonthEx); t = t.AddDate(0, 1, 0) {
			next := t.AddDate(0, 1, 0)
			buckets = append(buckets, StatsChartBucket{
				Label: t.Format("2006-01"),
				From:  t.Format("2006-01-02"),
				ToEx:  next.Format("2006-01-02"),
			})
		}
		from = buckets[0].From
		toEx = buckets[len(buckets)-1].ToEx
		return from, toEx, buckets
	}
}

// BuildStatsChartRange 하위호환: 버킷 From 목록을 일자 키로 반환.
func BuildStatsChartRange(view string, anchor time.Time) (from, toEx string, labels []string) {
	from, toEx, buckets := BuildStatsChartBuckets(view, anchor)
	for _, b := range buckets {
		labels = append(labels, b.From)
	}
	return from, toEx, labels
}

// LoadStatsKPI 현재·이전 기간 KPI
func (r *StatsRepo) LoadStatsKPI(view string, cols []model.StatsPeriodColumn, f model.StatsMeetingFilter) (model.StatsKPICard, error) {
	var out model.StatsKPICard
	f = normalizeMeetingFilter(f)
	if len(cols) < 2 {
		return out, nil
	}
	cur := cols[1]
	prev := cols[0]

	curRate := cur.Counts.ExecutionRatePct()
	prevRate := prev.Counts.ExecutionRatePct()
	out.ExecutionRate = math.Round(curRate*10) / 10
	out.ExecutionSample = cur.Counts.ExecPlanned()
	out.HasExecution = cur.Counts.HasExecutionRate()
	out.ExecDisplay = model.StatsReliability(out.ExecutionSample, !out.HasExecution, out.ExecutionRate).
		CapGradeIfImport(f.IncludeImport)
	prevExec := model.StatsReliability(prev.Counts.ExecPlanned(), !prev.Counts.HasExecutionRate(), prevRate)
	if out.ExecDisplay.ShowDelta && prevExec.ShowDelta {
		out.ExecutionDelta = math.Round((curRate-prevRate)*10) / 10
	} else {
		out.ExecDisplay.ShowDelta = false
	}

	visitCur, nVisitCur, err := r.avgASDays(cur.From, cur.ToExclusive, f, "visit")
	if err != nil {
		return out, err
	}
	visitPrev, nVisitPrev, err := r.avgASDays(prev.From, prev.ToExclusive, f, "visit")
	if err != nil {
		return out, err
	}
	out.VisitSample = nVisitCur
	out.HasVisit = nVisitCur > 0
	out.VisitAvgDays = math.Round(visitCur*10) / 10
	out.VisitDisplay = model.StatsReliability(nVisitCur, nVisitCur == 0, out.VisitAvgDays).
		CapGradeIfImport(f.IncludeImport).
		WithReason("해당 기간 방문 데이터 없음")
	prevVisit := model.StatsReliability(nVisitPrev, nVisitPrev == 0, visitPrev)
	if out.VisitDisplay.ShowDelta && prevVisit.ShowDelta {
		out.VisitDelta = math.Round((visitCur-visitPrev)*10) / 10
	} else {
		out.VisitDisplay.ShowDelta = false
	}

	compCur, nCompCur, err := r.avgASDays(cur.From, cur.ToExclusive, f, "complete")
	if err != nil {
		return out, err
	}
	compPrev, nCompPrev, err := r.avgASDays(prev.From, prev.ToExclusive, f, "complete")
	if err != nil {
		return out, err
	}
	out.CompleteSample = nCompCur
	out.HasComplete = nCompCur > 0
	out.CompleteAvgDays = math.Round(compCur*10) / 10
	out.CompleteDisplay = model.StatsReliability(nCompCur, nCompCur == 0, out.CompleteAvgDays).
		CapGradeIfImport(f.IncludeImport).
		WithReason("해당 기간 완료 데이터 없음")
	prevComp := model.StatsReliability(nCompPrev, nCompPrev == 0, compPrev)
	if out.CompleteDisplay.ShowDelta && prevComp.ShowDelta {
		out.CompleteDelta = math.Round((compCur-compPrev)*10) / 10
	} else {
		out.CompleteDisplay.ShowDelta = false
	}
	if nVisitCur > 0 && nCompCur > 0 && out.VisitAvgDays > out.CompleteAvgDays {
		out.LeadTimeWarn = fmt.Sprintf("지표 계산 오류: 방문(%.1f일)이 완료(%.1f일)보다 큽니다",
			out.VisitAvgDays, out.CompleteAvgDays)
	}
	return out, nil
}

// LoadStatsChartSeries 기간축 접수·미완료·완료 + 실행률 (일/주/월 버킷 집계)
func (r *StatsRepo) LoadStatsChartSeries(view string, anchor time.Time, f model.StatsMeetingFilter) ([]model.StatsChartPoint, error) {
	f = normalizeMeetingFilter(f)
	from, toEx, buckets := BuildStatsChartBuckets(view, anchor)
	return r.chartSeriesFromBuckets(view, from, toEx, buckets, f)
}

func (r *StatsRepo) LoadStatsChartSeriesWindow(view string, fromIncl, toIncl time.Time, f model.StatsMeetingFilter) ([]model.StatsChartPoint, error) {
	f = normalizeMeetingFilter(f)
	from, toEx, buckets := BuildStatsChartBucketsWindow(view, fromIncl, toIncl)
	return r.chartSeriesFromBuckets(view, from, toEx, buckets, f)
}

// LoadStatsChartSeriesRange 사용자 지정 기간 차트
func (r *StatsRepo) LoadStatsChartSeriesRange(fromIncl, toIncl time.Time, f model.StatsMeetingFilter) ([]model.StatsChartPoint, error) {
	from, toEx, buckets := BuildStatsChartBucketsForRange(fromIncl, toIncl)
	return r.chartSeriesFromBuckets(model.StatsViewRange, from, toEx, buckets, f)
}

func (r *StatsRepo) chartSeriesFromBuckets(view, from, toEx string, buckets []StatsChartBucket, f model.StatsMeetingFilter) ([]model.StatsChartPoint, error) {
	f = normalizeMeetingFilter(f)
	recMap, err := r.mapDayCountsAS(from, toEx, f, "received")
	if err != nil {
		return nil, err
	}
	openAS, err := r.mapDayCountsAS(from, toEx, f, "open")
	if err != nil {
		return nil, err
	}
	doneAS, err := r.mapDayCountsAS(from, toEx, f, "completed")
	if err != nil {
		return nil, err
	}
	// 부분완료 접수의 하위업무: 부모는 완료 집계, 하위는 접수·미완료로 별도 집계
	workRec, err := r.mapDayCountsASWork(from, toEx, f, "received")
	if err != nil {
		return nil, err
	}
	workOpen, err := r.mapDayCountsASWork(from, toEx, f, "open")
	if err != nil {
		return nil, err
	}
	addMaps(recMap, workRec)
	addMaps(openAS, workOpen)
	mntRec, err := r.mapDayCountsMnt(from, toEx, f, false)
	if err != nil {
		return nil, err
	}
	mntOpen, err := r.mapDayCountsMntOpen(from, toEx, f)
	if err != nil {
		return nil, err
	}
	mntDone, err := r.mapDayCountsMnt(from, toEx, f, true)
	if err != nil {
		return nil, err
	}
	adminRec, err := r.mapDayCountsAdmin(from, toEx, f, false)
	if err != nil {
		return nil, err
	}
	adminOpen, err := r.mapDayCountsAdminOpen(from, toEx, f)
	if err != nil {
		return nil, err
	}
	adminDone, err := r.mapDayCountsAdmin(from, toEx, f, true)
	if err != nil {
		return nil, err
	}
	plannedMap, onPlanMap, err := r.mapDayPlannedOnPlan(from, toEx, f)
	if err != nil {
		return nil, err
	}

	sumRange := func(m map[string]int, bFrom, bToEx string) int {
		n := 0
		for d, v := range m {
			if d >= bFrom && d < bToEx {
				n += v
			}
		}
		return n
	}
	monthMode := view == model.StatsViewMonth || view == model.StatsViewRange && len(buckets) > 0 && strings.Count(buckets[0].Label, "-") == 1 && len(buckets[0].Label) == 7

	out := make([]model.StatsChartPoint, 0, len(buckets))
	for _, b := range buckets {
		planned := sumRange(plannedMap, b.From, b.ToEx)
		onPlan := sumRange(onPlanMap, b.From, b.ToEx)
		if onPlan > planned {
			onPlan = planned
		}
		rate := math.Round(model.StatsWorkSlice{Planned: planned, OnPlan: onPlan}.ExecutionRatePct()*10) / 10
		rangeLbl := chartRangeLabel(b.From, b.ToEx, monthMode)
		if view == model.StatsViewDay || (view == model.StatsViewRange && b.ToEx == nextDay(b.From)) {
			rangeLbl = b.From
		}
		out = append(out, model.StatsChartPoint{
			Label: b.Label, RangeLabel: rangeLbl, Date: b.From,
			Received:  sumRange(recMap, b.From, b.ToEx) + sumRange(mntRec, b.From, b.ToEx) + sumRange(adminRec, b.From, b.ToEx),
			Open:      sumRange(openAS, b.From, b.ToEx) + sumRange(mntOpen, b.From, b.ToEx) + sumRange(adminOpen, b.From, b.ToEx),
			Completed: sumRange(doneAS, b.From, b.ToEx) + sumRange(mntDone, b.From, b.ToEx) + sumRange(adminDone, b.From, b.ToEx),
			Planned:   planned,
			Process:   onPlan,
			ExecRate:  rate,
		})
	}
	return out, nil
}

// ListCompletedDetail 기간 내 완료·종료 건(조치내용 포함)
func (r *StatsRepo) ListCompletedDetail(from, toEx string, f model.StatsMeetingFilter) ([]model.StatsRow, error) {
	f = normalizeMeetingFilter(f)
	asSQL, asArgs := r.filterAS(f)
	q := `
		SELECT ar.as_id, ar.as_number,
		       COALESCE(ar.receipt_datetime,''),
		       COALESCE(date(COALESCE(ar.complete_datetime, ar.updated_at)),''),
		       COALESCE(a.product_category,''), COALESCE(a.product_type,''),
		       COALESCE(a.model_name,''), COALESCE(a.product_name,''),
		       COALESCE(ar.receipt_channel,''), COALESCE(ar.requester_type,''),
		       COALESCE(c.org_name,''),
		       COALESCE(ar.assigned_to,''),
		       COALESCE(ar.symptom,''), COALESCE(ar.action_taken,''),
		       COALESCE(ar.status,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status IN ` + model.SQLStatusStatsCompleted + `
		  AND date(COALESCE(ar.complete_datetime, ar.updated_at)) >= date(?)
		  AND date(COALESCE(ar.complete_datetime, ar.updated_at)) < date(?)` + asSQL + `
		ORDER BY date(COALESCE(ar.complete_datetime, ar.updated_at)) DESC, ar.as_number DESC
		LIMIT 200`
	rows, err := r.db.Query(q, append([]interface{}{from, toEx}, asArgs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.StatsRow
	for rows.Next() {
		var it model.StatsRow
		var receiptStr, completeStr, productCategory, productType, modelName, productName, channel, reqType string
		if err := rows.Scan(
			&it.ASID, &it.ASNumber, &receiptStr, &completeStr,
			&productCategory, &productType, &modelName, &productName,
			&channel, &reqType,
			&it.CustomerName, &it.Assignee,
			&it.Symptom, &it.ActionTaken, &it.Status,
		); err != nil {
			return nil, err
		}
		if t := parseTime(receiptStr); !t.IsZero() {
			it.ReceiptDate = t.Format("2006-01-02")
		}
		it.CompleteDate = strings.TrimSpace(completeStr)
		it.ProductCategory = mapProductCategory(productCategory, productType, productName)
		it.ProductModel = strings.TrimSpace(modelName)
		if it.ProductModel == "" {
			it.ProductModel = strings.TrimSpace(productName)
		}
		it.WorkForm = "AS"
		it.Href = "/as/" + it.ASID + "/action"
		it.ReceiptForm = mapReceiptForm(channel, reqType)
		it.StatusLabel = statsStatusLabel(it.Status)
		if it.Assignee == "" {
			it.Assignee = "(미배정)"
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	mntDone, err := r.listMntCases(from, toEx, f, statsCaseCompleted, 200)
	if err != nil {
		return nil, err
	}
	for _, it := range mntDone {
		it.ActionTaken = it.Symptom
		items = append(items, it)
	}
	adminDone, err := r.listAdminCases(from, toEx, f, statsCaseCompleted, 200)
	if err != nil {
		return nil, err
	}
	for _, it := range adminDone {
		it.ActionTaken = it.Symptom
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CompleteDate != items[j].CompleteDate {
			return items[i].CompleteDate > items[j].CompleteDate
		}
		return items[i].ASNumber > items[j].ASNumber
	})
	if len(items) > 200 {
		items = items[:200]
	}
	return items, nil
}

func nextDay(d string) string {
	t, err := time.ParseInLocation("2006-01-02", d, time.Local)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

func (r *StatsRepo) avgASDays(from, toEx string, f model.StatsMeetingFilter, kind string) (avg float64, n int, err error) {
	visit, complete, n, err := r.avgASLeadTimes(from, toEx, f)
	if err != nil {
		return 0, 0, err
	}
	if kind == "visit" {
		return visit, n, nil
	}
	return complete, n, nil
}

// avgASLeadTimes §4.6 — 완료 건만, complete_datetime 기간축, 착수·완료 둘 다 있는 같은 모집단.
func (r *StatsRepo) avgASLeadTimes(from, toEx string, f model.StatsMeetingFilter) (visit, complete float64, n int, err error) {
	asSQL, args := r.filterAS(f)
	q := `
		SELECT AVG(julianday(date(ar.start_datetime)) - julianday(date(ar.receipt_datetime))),
		       AVG(julianday(date(ar.complete_datetime)) - julianday(date(ar.receipt_datetime))),
		       COUNT(*)
		FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status IN ` + model.SQLStatusStatsCompleted + `
		  AND TRIM(COALESCE(ar.start_datetime,'')) != ''
		  AND TRIM(COALESCE(ar.complete_datetime,'')) != ''
		  AND date(ar.complete_datetime) >= date(?)
		  AND date(ar.complete_datetime) < date(?)` + asSQL
	args = append([]interface{}{from, toEx}, args...)
	var visitNull, compNull interface{}
	var cnt int
	err = r.db.QueryRow(q, args...).Scan(&visitNull, &compNull, &cnt)
	if err != nil {
		return 0, 0, 0, err
	}
	n = cnt
	if cnt == 0 {
		return 0, 0, 0, nil
	}
	visit = scanAvgDays(visitNull)
	complete = scanAvgDays(compNull)
	return visit, complete, n, nil
}

func scanAvgDays(v interface{}) float64 {
	if v == nil {
		return 0
	}
	var avg float64
	switch t := v.(type) {
	case float64:
		avg = t
	case []byte:
		fmt.Sscanf(string(t), "%f", &avg)
	case string:
		fmt.Sscanf(t, "%f", &avg)
	}
	return avg
}

func (r *StatsRepo) mapDayCountsAS(from, toEx string, f model.StatsMeetingFilter, kind string) (map[string]int, error) {
	asSQL, asArgs := r.filterAS(f)
	var dateExpr, where string
	switch kind {
	case "open":
		// 미완료: 접수·담당자 배정·진행중·보류·이관 (접수일 기준)
		dateExpr = `date(ar.receipt_datetime)`
		// 통계 미완료: 부분완료는 완료로 잡히므로 제외(하위업무는 mapDayCountsASWork)
		where = `ar.status IN ` + model.SQLStatusStatsOpen
	case "completed":
		dateExpr = asCompleteDateSQL
		where = `ar.status IN ` + model.SQLStatusStatsCompleted
	default:
		dateExpr = `date(ar.receipt_datetime)`
		where = `1=1`
	}
	q := fmt.Sprintf(`
		SELECT %s AS d, COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE %s AND %s >= date(?) AND %s < date(?)`+asSQL+`
		GROUP BY d`, dateExpr, where, dateExpr, dateExpr)
	return r.scanDayCountMap(q, append([]interface{}{from, toEx}, asArgs...)...)
}

// mapDayCountsASWork 부분완료 접수의 하위업무(확인·재방문)를 통계 접수/미완료로 집계한다.
// 부모 접수는 completed 집계에 남기고, 추가된 하위업무만 별도로 센다.
func (r *StatsRepo) mapDayCountsASWork(from, toEx string, f model.StatsMeetingFilter, kind string) (map[string]int, error) {
	asSQL, asArgs := r.filterAS(f)
	statusFilter := ""
	if kind == "open" {
		statusFilter = ` AND COALESCE(w.status,'open') = 'open'`
	}
	q := `
		SELECT date(w.created_at) AS d, COUNT(*)
		FROM as_work_items w
		JOIN as_receipts ar ON ar.as_id = w.as_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status = 'partial_complete'
		  AND date(w.created_at) >= date(?) AND date(w.created_at) < date(?)` + statusFilter + asSQL + `
		GROUP BY d`
	return r.scanDayCountMap(q, append([]interface{}{from, toEx}, asArgs...)...)
}

func (r *StatsRepo) mapDayCountsMnt(from, toEx string, f model.StatsMeetingFilter, completed bool) (map[string]int, error) {
	mntSQL, mntArgs := r.filterMnt(f)
	var q string
	if completed {
		q = `
			SELECT ` + mntCompleteDateSQL + ` AS d, COUNT(*)
			FROM maintenance_visits v
			WHERE COALESCE(v.completed,0)=1
			  AND ` + mntCompleteDateSQL + ` >= ?
			  AND ` + mntCompleteDateSQL + ` < ?` + mntSQL + `
			GROUP BY d`
	} else {
		q = `
			SELECT v.visit_date AS d, COUNT(*)
			FROM maintenance_visits v
			WHERE v.visit_date >= ? AND v.visit_date < ?` + mntSQL + `
			GROUP BY d`
	}
	return r.scanDayCountMap(q, append([]interface{}{from, toEx}, mntArgs...)...)
}

func (r *StatsRepo) mapDayCountsAdmin(from, toEx string, f model.StatsMeetingFilter, completed bool) (map[string]int, error) {
	adminSQL, adminArgs := r.filterAdmin(f)
	base := `COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '')`
	status := ``
	if completed {
		base = adminTaskCompleteDateSQL
		status = ` AND t.status='complete'`
	}
	q := `
		SELECT ` + base + ` AS d, COUNT(*)
		FROM work_tasks t
		WHERE t.work_type IN ('admin','support')` + status + `
		  AND ` + base + ` >= ? AND ` + base + ` < ?` + adminSQL + `
		GROUP BY d`
	return r.scanDayCountMap(q, append([]interface{}{from, toEx}, adminArgs...)...)
}

func (r *StatsRepo) mapDayCountsMntOpen(from, toEx string, f model.StatsMeetingFilter) (map[string]int, error) {
	mntSQL, mntArgs := r.filterMnt(f)
	q := `
		SELECT v.visit_date AS d, COUNT(*)
		FROM maintenance_visits v
		WHERE COALESCE(v.completed,0)=0
		  AND v.visit_date >= ? AND v.visit_date < ?` + mntSQL + `
		GROUP BY d`
	return r.scanDayCountMap(q, append([]interface{}{from, toEx}, mntArgs...)...)
}

func (r *StatsRepo) mapDayCountsAdminOpen(from, toEx string, f model.StatsMeetingFilter) (map[string]int, error) {
	adminSQL, adminArgs := r.filterAdmin(f)
	base := `COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '')`
	q := `
		SELECT ` + base + ` AS d, COUNT(*)
		FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND t.status != 'complete'
		  AND ` + base + ` >= ? AND ` + base + ` < ?` + adminSQL + `
		GROUP BY d`
	return r.scanDayCountMap(q, append([]interface{}{from, toEx}, adminArgs...)...)
}

// mapDayPlannedOnPlan 일자별 예정 건수 · 계획대로(예정일 당일) 완료 건수
func (r *StatsRepo) mapDayPlannedOnPlan(from, toEx string, f model.StatsMeetingFilter) (planned, onPlan map[string]int, err error) {
	planned = map[string]int{}
	onPlan = map[string]int{}
	f = r.attachMetrics(f)
	asSQL, asArgs := r.filterAS(f)
	if f.Scope != model.StatsScopeWorkType || f.Key == "" || f.Key == "as" {
		m, e := r.scanDayCountMap(`
			SELECT ar.visit_scheduled_date AS d, COUNT(*) FROM as_receipts ar
			LEFT JOIN assets a ON a.asset_id = ar.asset_id
			WHERE TRIM(COALESCE(ar.visit_scheduled_date,'')) != ''
			  AND ar.visit_scheduled_date >= ? AND ar.visit_scheduled_date < ?
			  AND ar.status NOT IN ('cancelled')`+asSQL+` GROUP BY d`,
			append([]interface{}{from, toEx}, asArgs...)...)
		if e != nil {
			return nil, nil, e
		}
		addMaps(planned, m)
		m, e = r.scanDayCountMap(`
			SELECT ar.visit_scheduled_date AS d, COUNT(*) FROM as_receipts ar
			LEFT JOIN assets a ON a.asset_id = ar.asset_id
			WHERE TRIM(COALESCE(ar.visit_scheduled_date,'')) != ''
			  AND ar.visit_scheduled_date >= ? AND ar.visit_scheduled_date < ?
			  AND ar.status IN `+model.SQLStatusStatsCompleted+`
			  AND date(COALESCE(ar.complete_datetime, ar.updated_at)) = date(ar.visit_scheduled_date)`+asSQL+` GROUP BY d`,
			append([]interface{}{from, toEx}, asArgs...)...)
		if e != nil {
			return nil, nil, e
		}
		addMaps(onPlan, m)
	}
	mntSQL, mntArgs := r.filterMnt(f)
	if f.Scope != model.StatsScopeWorkType || f.Key == "" || f.Key == "maintenance" {
		m, e := r.scanDayCountMap(`
			SELECT v.visit_date AS d, COUNT(*) FROM maintenance_visits v
			WHERE v.visit_date >= ? AND v.visit_date < ?`+mntSQL+` GROUP BY d`,
			append([]interface{}{from, toEx}, mntArgs...)...)
		if e != nil {
			return nil, nil, e
		}
		addMaps(planned, m)
		m, e = r.scanDayCountMap(`
			SELECT v.visit_date AS d, COUNT(*) FROM maintenance_visits v
			WHERE v.visit_date >= ? AND v.visit_date < ?
			  AND COALESCE(v.completed,0)=1
			  AND (TRIM(COALESCE(v.completed_date,'')) = '' OR v.completed_date = v.visit_date)`+mntSQL+` GROUP BY d`,
			append([]interface{}{from, toEx}, mntArgs...)...)
		if e != nil {
			return nil, nil, e
		}
		addMaps(onPlan, m)
	}
	adminSQL, adminArgs := r.filterAdmin(f)
	if model.ProgressScopeIncludes(f.ProgressScope, "admin") && (f.Scope != model.StatsScopeWorkType || f.Key == "" || f.Key == "admin") {
		m, e := r.scanDayCountMap(`
			SELECT COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') AS d, COUNT(*)
			FROM work_tasks t
			WHERE t.work_type IN ('admin','support')
			  AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') >= ?
			  AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') < ?`+adminSQL+` GROUP BY d`,
			append([]interface{}{from, toEx}, adminArgs...)...)
		if e != nil {
			return nil, nil, e
		}
		addMaps(planned, m)
		m, e = r.scanDayCountMap(`
			SELECT t.work_date AS d, COUNT(*) FROM work_tasks t
			WHERE t.work_type IN ('admin','support') AND t.status='complete'
			  AND COALESCE(t.recurrence_role,'')='occurrence'
			  AND TRIM(COALESCE(t.work_date,'')) != ''
			  AND t.work_date >= ? AND t.work_date < ?
			  AND `+adminTaskCompleteDateSQL+` = t.work_date`+adminSQL+` GROUP BY d`,
			append([]interface{}{from, toEx}, adminArgs...)...)
		if e != nil {
			return nil, nil, e
		}
		addMaps(onPlan, m)
		m, e = r.scanDayCountMap(`
			SELECT t.due_date AS d, COUNT(*) FROM work_tasks t
			WHERE t.work_type IN ('admin','support') AND t.status='complete'
			  AND COALESCE(t.recurrence_role,'') != 'occurrence'
			  AND TRIM(COALESCE(t.due_date,'')) != ''
			  AND t.due_date >= ? AND t.due_date < ?
			  AND (TRIM(COALESCE(t.work_date,'')) = '' OR t.work_date = t.due_date)`+adminSQL+` GROUP BY d`,
			append([]interface{}{from, toEx}, adminArgs...)...)
		if e != nil {
			return nil, nil, e
		}
		addMaps(onPlan, m)
	}
	return planned, onPlan, nil
}

func addMaps(dst, src map[string]int) {
	for k, v := range src {
		dst[k] += v
	}
}

func (r *StatsRepo) scanDayCountMap(q string, args ...interface{}) (map[string]int, error) {
	out := map[string]int{}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		d = strings.TrimSpace(d)
		if len(d) >= 10 {
			d = d[:10]
		}
		if d != "" {
			out[d] = n
		}
	}
	return out, rows.Err()
}

// asFilterSQL AS 공통 필터 (assignee / product). ar·a 별칭 사용.
func asFilterSQL(f model.StatsMeetingFilter) (string, []interface{}) {
	var b strings.Builder
	var args []interface{}
	if f.Scope == model.StatsScopeAssignee && f.Key != "" {
		if f.Key == model.StatsUnassignedLabel {
			b.WriteString(` AND TRIM(COALESCE(ar.assigned_to,'')) = '' AND TRIM(COALESCE(ar.assigned_user_id,'')) = ''`)
		} else {
			b.WriteString(` AND (TRIM(COALESCE(ar.assigned_to,'')) = ? OR TRIM(COALESCE(ar.assigned_user_id,'')) = ?)`)
			args = append(args, f.Key, f.Key)
		}
	}
	if f.Scope == model.StatsScopeProduct && f.Key != "" {
		frag, a := productMatchSQL("a", "ar", f.Key)
		b.WriteString(frag)
		args = append(args, a...)
	}
	if f.ProjectID != "" {
		b.WriteString(` AND (
			TRIM(COALESCE(a.project_id,'')) = ?
			OR EXISTS (
				SELECT 1 FROM work_tasks t
				WHERE t.source_type='as' AND t.source_id=ar.as_id
				  AND TRIM(COALESCE(t.project_id,'')) = ?
			)
		)`)
		args = append(args, f.ProjectID, f.ProjectID)
	}
	if !f.IncludeImport {
		b.WriteString(` AND COALESCE(ar.data_origin,'app') != 'import'`)
	}
	if d := strings.TrimSpace(f.MetricsBaseDate); d != "" {
		b.WriteString(` AND date(ar.receipt_datetime) >= date(?)`)
		args = append(args, d)
	}
	return b.String(), args
}

func mntFilterSQL(f model.StatsMeetingFilter) (string, []interface{}) {
	var b strings.Builder
	var args []interface{}
	if f.Scope == model.StatsScopeAssignee && f.Key != "" {
		if f.Key == model.StatsUnassignedLabel {
			b.WriteString(` AND TRIM(COALESCE(v.assignee,'')) = ''`)
		} else {
			b.WriteString(` AND TRIM(COALESCE(v.assignee,'')) = ?`)
			args = append(args, f.Key)
		}
	}
	if f.Scope == model.StatsScopeProduct && f.Key != "" {
		b.WriteString(` AND (` + productTypeLike("v.product_type", f.Key) + `)`)
	}
	if f.Scope == model.StatsScopeWorkType {
		switch f.Key {
		case "as", "admin":
			b.WriteString(` AND 1=0`)
		}
	}
	if f.ProjectID != "" {
		b.WriteString(` AND TRIM(COALESCE(v.project_id,'')) = ?`)
		args = append(args, f.ProjectID)
	}
	if !f.IncludeImport {
		b.WriteString(` AND COALESCE(v.data_origin,'app') != 'import'`)
	}
	if d := strings.TrimSpace(f.MetricsBaseDate); d != "" {
		b.WriteString(` AND date(v.visit_date) >= date(?)`)
		args = append(args, d)
	}
	return b.String(), args
}

func adminFilterSQL(f model.StatsMeetingFilter) (string, []interface{}) {
	var b strings.Builder
	var args []interface{}
	// AS·정기점검 일일업무는 해당 유형에서 집계. work_type 기본값이 admin이라 중복될 수 있음.
	b.WriteString(` AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')`)
	if f.ExcludeSalesActivity {
		b.WriteString(` AND TRIM(COALESCE(t.source_type,'')) != 'sales_activity'`)
	}
	if f.Scope == model.StatsScopeAssignee && f.Key != "" {
		if f.Key == model.StatsUnassignedLabel {
			b.WriteString(` AND TRIM(COALESCE(t.assignee,'')) = ''`)
		} else {
			b.WriteString(` AND TRIM(COALESCE(t.assignee,'')) = ?`)
			args = append(args, f.Key)
		}
	}
	if f.Scope == model.StatsScopeProduct {
		b.WriteString(` AND 1=0`)
	}
	if f.Scope == model.StatsScopeWorkType {
		switch f.Key {
		case "as", "maintenance":
			b.WriteString(` AND 1=0`)
		}
	}
	if f.ProjectID != "" {
		b.WriteString(` AND t.project_id = ?`)
		args = append(args, f.ProjectID)
	}
	b.WriteString(SQLRecurrenceWorkUnit)
	if d := strings.TrimSpace(f.MetricsBaseDate); d != "" {
		b.WriteString(` AND date(` + adminTaskReceiptDateSQL + `) >= date(?)`)
		args = append(args, d)
	}
	return b.String(), args
}

func productMatchSQL(assetAlias, asAlias, product string) (string, []interface{}) {
	like := "%" + strings.TrimSpace(product) + "%"
	compact := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(product), "-", ""), " ", ""))
	up := "%" + compact + "%"
	// 자산 제품구분·제조사·모델 + 증상/접수내용 힌트
	frag := fmt.Sprintf(` AND (
		COALESCE(%[1]s.manufacturer,'') LIKE ? OR COALESCE(%[1]s.product_name,'') LIKE ?
		OR COALESCE(%[1]s.model_name,'') LIKE ? OR COALESCE(%[1]s.product_type,'') LIKE ?
		OR COALESCE(%[1]s.product_category,'') LIKE ?
		OR COALESCE(%[2]s.symptom,'') LIKE ?
		OR UPPER(REPLACE(REPLACE(COALESCE(%[1]s.manufacturer,''),'-',''),' ','')) LIKE ?
		OR UPPER(REPLACE(REPLACE(COALESCE(%[1]s.product_name,''),'-',''),' ','')) LIKE ?
		OR UPPER(REPLACE(REPLACE(COALESCE(%[1]s.product_type,''),'-',''),' ','')) LIKE ?
	)`, assetAlias, asAlias)
	return frag, []interface{}{like, like, like, like, like, like, up, up, up}
}

func productTypeLike(col, product string) string {
	p := strings.TrimSpace(product)
	esc := strings.ReplaceAll(p, "'", "''")
	if strings.Contains(p, "세종") {
		return fmt.Sprintf(`(%s LIKE '%%세종%%' OR %s LIKE '%%SEJONG%%')`, col, col)
	}
	u := strings.ToUpper(strings.ReplaceAll(p, "-", ""))
	if strings.Contains(u, "KLAS") || strings.Contains(u, "KLAS") {
		return fmt.Sprintf(`(%s LIKE '%%KLAS%%' OR %s LIKE '%%K-LAS%%' OR %s LIKE '%%klas%%') AND %s NOT LIKE '%%세종%%'`, col, col, col, col)
	}
	return fmt.Sprintf(`%s LIKE '%%%s%%'`, col, esc)
}

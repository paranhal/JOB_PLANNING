package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type StatsRepo struct{ db *sql.DB }

func NewStatsRepo(db *sql.DB) *StatsRepo { return &StatsRepo{db: db} }

// NormalizeStatsOffset 0~2 로 제한 (하위호환)
func NormalizeStatsOffset(n int) int {
	if n < 0 {
		return 0
	}
	if n > 2 {
		return 2
	}
	return n
}

// PeriodRange 기간 필터 → [from, to) (to exclusive). all 이면 ok=false
func PeriodRange(period string, offset int, now time.Time) (from, to time.Time, ok bool) {
	offset = NormalizeStatsOffset(offset)
	loc := now.Location()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)
	switch period {
	case model.StatsPeriodDay:
		day := today.AddDate(0, 0, -offset)
		return day, day.AddDate(0, 0, 1), true
	case model.StatsPeriodWeek:
		weekday := int(today.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		mon := today.AddDate(0, 0, -(weekday - 1)).AddDate(0, 0, -7*offset)
		return mon, mon.AddDate(0, 0, 7), true
	case model.StatsPeriodMonth:
		start := time.Date(y, m, 1, 0, 0, 0, 0, loc).AddDate(0, -offset, 0)
		return start, start.AddDate(0, 1, 0), true
	case model.StatsPeriodQuarter:
		q := ((int(m)-1)/3)*3 + 1
		start := time.Date(y, time.Month(q), 1, 0, 0, 0, 0, loc).AddDate(0, -3*offset, 0)
		return start, start.AddDate(0, 3, 0), true
	default:
		return time.Time{}, time.Time{}, false
	}
}

// ResolveStatsRange StatsQuery → [from, to) exclusive end
func ResolveStatsRange(q model.StatsQuery, now time.Time) (from, to time.Time, ok bool, label string) {
	loc := now.Location()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)

	switch q.Period {
	case model.StatsPeriodDay:
		day := today
		if t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(q.Date), loc); err == nil {
			day = t
		}
		return day, day.AddDate(0, 0, 1), true, day.Format("2006/01/02")

	case model.StatsPeriodWeek:
		ref := today
		if t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(q.Date), loc); err == nil {
			ref = t
		}
		wd := int(ref.Weekday())
		if wd == 0 {
			wd = 7
		}
		mon := ref.AddDate(0, 0, -(wd - 1))
		sun := mon.AddDate(0, 0, 6)
		label = fmt.Sprintf("월 %s ~ 일 %s", mon.Format("2006/01/02"), sun.Format("2006/01/02"))
		return mon, mon.AddDate(0, 0, 7), true, label

	case model.StatsPeriodMonth:
		start := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		if ms := strings.TrimSpace(q.Month); len(ms) >= 7 {
			if t, err := time.ParseInLocation("2006-01", ms[:7], loc); err == nil {
				start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
			}
		}
		return start, start.AddDate(0, 1, 0), true, start.Format("2006/01")

	case model.StatsPeriodQuarter:
		qm := ((int(m)-1)/3)*3 + 1
		start := time.Date(y, time.Month(qm), 1, 0, 0, 0, 0, loc)
		qs := strings.TrimSpace(q.Quarter)
		if len(qs) >= 6 { // 2026-Q3
			var yy, qn int
			if _, err := fmt.Sscanf(qs, "%d-Q%d", &yy, &qn); err == nil && qn >= 1 && qn <= 4 {
				start = time.Date(yy, time.Month((qn-1)*3+1), 1, 0, 0, 0, 0, loc)
			}
		}
		qn := ((int(start.Month())-1)/3 + 1)
		label = fmt.Sprintf("%d년 %d분기", start.Year(), qn)
		return start, start.AddDate(0, 3, 0), true, label

	case model.StatsPeriodRange:
		fromStr := strings.TrimSpace(q.From)
		toStr := strings.TrimSpace(q.To)
		if fromStr == "" {
			fromStr = today.Format("2006-01-02")
		}
		if toStr == "" {
			toStr = today.Format("2006-01-02")
		}
		f, err1 := time.ParseInLocation("2006-01-02", fromStr, loc)
		t, err2 := time.ParseInLocation("2006-01-02", toStr, loc)
		if err1 != nil || err2 != nil {
			return time.Time{}, time.Time{}, false, ""
		}
		if t.Before(f) {
			f, t = t, f
		}
		label = f.Format("2006/01/02") + " ~ " + t.Format("2006/01/02")
		return f, t.AddDate(0, 0, 1), true, label

	default:
		from, to, ok := PeriodRange(q.Period, 0, now)
		return from, to, ok, ""
	}
}

// PeriodDisplayLabels 좌측 기준 표기 라벨 (하위호환)
func PeriodDisplayLabels(period string, offset int, now time.Time) []string {
	offset = NormalizeStatsOffset(offset)
	from, to, ok := PeriodRange(period, offset, now)
	if !ok {
		return nil
	}
	switch period {
	case model.StatsPeriodDay:
		return []string{from.Format("2006/01/02")}
	case model.StatsPeriodWeek:
		sun := to.AddDate(0, 0, -1)
		return []string{
			"월 " + from.Format("2006/01/02"),
			"일 " + sun.Format("2006/01/02"),
		}
	case model.StatsPeriodMonth:
		return []string{from.Format("2006/01")}
	case model.StatsPeriodQuarter:
		tag := "현재"
		if offset == 1 {
			tag = "전분기"
		} else if offset == 2 {
			tag = "전전분기"
		}
		labels := quarterLabels(from, tag)
		if offset > 0 {
			curFrom, _, cok := PeriodRange(period, 0, now)
			if cok {
				labels = append(labels, quarterLabels(curFrom, "현재")...)
			}
		}
		return labels
	default:
		return nil
	}
}

func quarterLabels(start time.Time, tag string) []string {
	q := ((int(start.Month()) - 1) / 3) * 3
	qn := q/3 + 1
	_, week := start.ISOWeek()
	return []string{
		fmt.Sprintf("%s %d년 %d분기", tag, start.Year(), qn),
		start.Format("2006/01"),
		fmt.Sprintf("%d주차", week),
	}
}

func metricDateExpr(metric string) string {
	switch metric {
	case model.StatsMetricCompleted:
		return `date(COALESCE(ar.complete_datetime, ar.updated_at))`
	default:
		return `date(ar.receipt_datetime)`
	}
}

func metricStatusCond(metric string) string {
	switch metric {
	case model.StatsMetricProgress:
		// 미완료(업무진행)
		return ` AND ar.status IN ('received','assigned','in_progress','hold')`
	case model.StatsMetricCompleted:
		return ` AND ar.status IN ('completed','closed')`
	case model.StatsMetricReceived:
		return `` // 접수 전체(기간만)
	case model.StatsMetricOverdue:
		return ` AND ar.status IN ('received','assigned','in_progress')
		         AND julianday('now','localtime') - julianday(date(ar.receipt_datetime)) > 3`
	default:
		return ` AND ar.status IN ('received','assigned','in_progress','hold')`
	}
}

// ListDetail 통계 상세
func (r *StatsRepo) ListDetail(q model.StatsQuery) ([]model.StatsRow, error) {
	now := time.Now()
	sqlQ := `
		SELECT ar.as_id, ar.as_number,
		       COALESCE(ar.receipt_datetime,''),
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
		WHERE 1=1`
	args := []interface{}{}

	sqlQ += metricStatusCond(q.Metric)

	if from, to, ok, _ := ResolveStatsRange(q, now); ok {
		expr := metricDateExpr(q.Metric)
		sqlQ += fmt.Sprintf(` AND %s >= date(?) AND %s < date(?)`, expr, expr)
		args = append(args, from.Format("2006-01-02"), to.Format("2006-01-02"))
	}

	sqlQ += ` ORDER BY ar.receipt_datetime DESC, ar.as_number DESC`

	rows, err := r.db.Query(sqlQ, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.StatsRow
	for rows.Next() {
		var it model.StatsRow
		var receiptStr, productCategory, productType, modelName, productName, channel, reqType string
		if err := rows.Scan(
			&it.ASID, &it.ASNumber, &receiptStr,
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
		it.ProductCategory = mapProductCategory(productCategory, productType, productName)
		it.ProductModel = strings.TrimSpace(modelName)
		if it.ProductModel == "" {
			it.ProductModel = strings.TrimSpace(productName)
		}
		it.WorkForm = "AS"
		it.ReceiptForm = mapReceiptForm(channel, reqType)
		it.StatusLabel = statsStatusLabel(it.Status)
		if it.Assignee == "" {
			it.Assignee = "(미배정)"
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// ListByAssignee 담당자별 집계
func (r *StatsRepo) ListByAssignee(q model.StatsQuery) ([]model.StatsAssigneeRow, error) {
	details, err := r.ListDetail(q)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	order := []string{}
	for _, d := range details {
		name := d.Assignee
		if _, ok := counts[name]; !ok {
			order = append(order, name)
		}
		counts[name]++
	}
	var rows []model.StatsAssigneeRow
	for _, name := range order {
		rows = append(rows, model.StatsAssigneeRow{Assignee: name, Count: counts[name]})
	}
	return rows, nil
}

// 하위호환 래퍼
func (r *StatsRepo) ListDetailPeriod(period string, offset int) ([]model.StatsRow, error) {
	return r.ListDetail(model.StatsQuery{
		Metric: model.StatsMetricReceived,
		Period: period,
		Date:   time.Now().AddDate(0, 0, -NormalizeStatsOffset(offset)).Format("2006-01-02"),
	})
}

func mapProductCategory(productCategory, productType, productName string) string {
	c := strings.ToLower(strings.TrimSpace(productCategory))
	switch c {
	case "homepage", "홈페이지":
		return "홈페이지"
	case "elibrary", "전자도서관":
		return "전자도서관"
	case "mobile", "모바일":
		return "모바일"
	case "rfid", "rfid자동화", "rfid_auto":
		return "RFID자동화"
	case "materials", "자료관리":
		return "자료관리"
	case "other", "기타":
		return "기타"
	}
	t := strings.ToLower(strings.TrimSpace(productType))
	n := strings.ToLower(strings.TrimSpace(productName))
	switch t {
	case "homepage", "home", "홈페이지":
		return "홈페이지"
	case "elibrary", "library", "전자도서관":
		return "전자도서관"
	case "mobile", "모바일":
		return "모바일"
	case "rfid", "rfid자동화", "rfid_auto":
		return "RFID자동화"
	case "materials", "자료관리", "data_mgmt":
		return "자료관리"
	case "other", "기타":
		return "기타"
	}
	combined := t + " " + n
	switch {
	case strings.Contains(combined, "홈페이지") || strings.Contains(combined, "homepage"):
		return "홈페이지"
	case strings.Contains(combined, "전자도서") || strings.Contains(combined, "elibrary") || strings.Contains(combined, "도서관시스템"):
		return "전자도서관"
	case strings.Contains(combined, "모바일") || strings.Contains(combined, "mobile"):
		return "모바일"
	case strings.Contains(combined, "rfid"):
		return "RFID자동화"
	case strings.Contains(combined, "자료관리") || strings.Contains(combined, "klas"):
		return "자료관리"
	default:
		return "기타"
	}
}

func mapReceiptForm(channel, requesterType string) string {
	if strings.EqualFold(strings.TrimSpace(requesterType), "onecall") || requesterType == "원콜" {
		return "원콜"
	}
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "phone", "전화":
		return "전화"
	case "email", "메일", "이메일":
		return "메일"
	case "visit", "현장", "방문":
		return "현장"
	case "partner", "제조사요청", "manufacturer":
		return "제조사요청"
	case "onecall", "원콜":
		return "원콜"
	default:
		if channel == "" {
			return "전화"
		}
		return channel
	}
}

func statsStatusLabel(s string) string {
	m := map[string]string{
		"received": "접수", "assigned": "담당자 배정", "in_progress": "진행중", "hold": "보류",
		"transfer": "이관", "cancelled": "접수취소",
		"completed": "완료", "closed": "종료",
	}
	if l, ok := m[s]; ok {
		return l
	}
	if s == "" {
		return "—"
	}
	return s
}

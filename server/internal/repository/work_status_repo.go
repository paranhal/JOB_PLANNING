package repository

import (
	"database/sql"
	"fmt"
	"time"

	"customer-support/internal/model"
)

// WorkStatusRepo 월 캘린더(calendar.html)용 목록. 통계 집계가 아니다.
// filterAS 계열을 쓰지 않는다 — 전체 기간 · 이관 데이터 포함. §4.13 (나)
type WorkStatusRepo struct{ db *sql.DB }

func NewWorkStatusRepo(db *sql.DB) *WorkStatusRepo { return &WorkStatusRepo{db: db} }

// MonthSummary 선택 분류 기준 월 요약 (접수/완료/전월이관)
func (r *WorkStatusRepo) MonthSummary(year, month int, category string) (model.WorkCalSummary, error) {
	var s model.WorkCalSummary
	start := fmt.Sprintf("%04d-%02d-01", year, month)
	endEx := nextMonthStart(year, month)
	prevStart, prevEndEx := prevMonthRange(year, month)
	startDT, endDT := dayTimeStart(start), dayTimeStart(endEx)
	prevStartDT, prevEndDT := dayTimeStart(prevStart), dayTimeStart(prevEndEx)

	switch category {
	case "maintenance":
		q := `SELECT COUNT(*) FROM maintenance_visits WHERE visit_date >= ? AND visit_date < ?`
		_ = r.db.QueryRow(q, start, endEx).Scan(&s.ReceiptCount)
		_ = r.db.QueryRow(q+` AND COALESCE(completed,0)=1`, start, endEx).Scan(&s.CompleteCount)
		return s, nil
	case "other":
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_other WHERE work_date >= ? AND work_date < ? AND phase='receipt'`, start, endEx).Scan(&s.ReceiptCount)
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_other WHERE work_date >= ? AND work_date < ? AND phase='complete'`, start, endEx).Scan(&s.CompleteCount)
		return s, nil
	default: // as
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE receipt_datetime >= ? AND receipt_datetime < ?`, startDT, endDT).Scan(&s.ReceiptCount)
		var workRec int
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_work_items w
			JOIN as_receipts ar ON ar.as_id = w.as_id
			WHERE ar.status = 'partial_complete'
			  AND w.created_at >= ? AND w.created_at < ?`, startDT, endDT).Scan(&workRec)
		s.ReceiptCount += workRec
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts ar WHERE ar.status IN `+model.SQLStatusStatsCompleted+`
			AND `+asCompleteDT+` >= ? AND `+asCompleteDT+` < ?`, startDT, endDT).Scan(&s.CompleteCount)
		_ = r.db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE status='transfer'
			AND updated_at >= ? AND updated_at < ?`, prevStartDT, prevEndDT).Scan(&s.PrevTransferCount)
		return s, nil
	}
}

// ListMonthItems 월 단위 항목 (category: as|maintenance|other, phase: receipt|visit|complete)
func (r *WorkStatusRepo) ListMonthItems(year, month int, category, phase string) ([]model.WorkCalItem, error) {
	start := fmt.Sprintf("%04d-%02d-01", year, month)
	endEx := nextMonthStart(year, month)

	switch category {
	case "maintenance":
		return r.listMaintenance(start, endEx, phase)
	case "other":
		return r.listOther(start, endEx, phase)
	default:
		return r.listAS(start, endEx, phase)
	}
}

func (r *WorkStatusRepo) listAS(start, endEx, phase string) ([]model.WorkCalItem, error) {
	startDT, endDT := dayTimeStart(start), dayTimeStart(endEx)
	var dateExpr string
	var extra string
	var a, b string
	switch phase {
	case "visit":
		dateExpr = `ar.visit_scheduled_date`
		extra = ` AND ar.visit_scheduled_date != '' AND ar.visit_scheduled_date >= ? AND ar.visit_scheduled_date < ?`
		a, b = start, endEx
	case "complete":
		dateExpr = asCompleteDateSQL
		extra = ` AND ar.status IN ` + model.SQLStatusStatsCompleted + `
			AND ` + asCompleteDT + ` >= ?
			AND ` + asCompleteDT + ` < ?`
		a, b = startDT, endDT
	default: // receipt
		dateExpr = `date(ar.receipt_datetime)`
		extra = ` AND ar.receipt_datetime >= ? AND ar.receipt_datetime < ?`
		a, b = startDT, endDT
	}

	q := fmt.Sprintf(`
		SELECT ar.as_id, c.org_name, ar.as_number, %s
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE 1=1 %s
		ORDER BY %s, c.org_name, ar.as_number`, dateExpr, extra, dateExpr)

	rows, err := r.db.Query(q, a, b)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.WorkCalItem
	for rows.Next() {
		var it model.WorkCalItem
		var d string
		if err := rows.Scan(&it.ID, &it.Title, &it.Subtitle, &d); err != nil {
			return nil, err
		}
		it.Category = "as"
		it.Date = trimDate(d)
		it.Link = "/as/" + it.ID
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *WorkStatusRepo) listMaintenance(start, endEx, phase string) ([]model.WorkCalItem, error) {
	_ = phase
	q := `
		SELECT v.visit_id, COALESCE(NULLIF(cfg.short_name,''), c.org_name),
		       COALESCE(v.product_type,''), v.visit_date, v.plan_id
		FROM maintenance_visits v
		JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		WHERE v.visit_date >= ? AND v.visit_date < ?
		ORDER BY v.visit_date, cfg.short_name, c.org_name`
	rows, err := r.db.Query(q, start, endEx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.WorkCalItem
	for rows.Next() {
		var it model.WorkCalItem
		var d, planID string
		if err := rows.Scan(&it.ID, &it.Title, &it.ProductType, &d, &planID); err != nil {
			return nil, err
		}
		it.Category = "maintenance"
		it.Subtitle = it.ProductType
		it.Date = trimDate(d)
		if planID != "" {
			it.Link = "/maintenance/" + planID
		} else {
			it.Link = "/maintenance"
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *WorkStatusRepo) listOther(start, endEx, phase string) ([]model.WorkCalItem, error) {
	q := `
		SELECT other_id, title, COALESCE(org_name,''), work_date
		FROM work_other
		WHERE work_date >= ? AND work_date < ? AND phase = ?
		ORDER BY work_date, title`
	rows, err := r.db.Query(q, start, endEx, phase)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.WorkCalItem
	for rows.Next() {
		var it model.WorkCalItem
		var d string
		if err := rows.Scan(&it.ID, &it.Title, &it.Subtitle, &d); err != nil {
			return nil, err
		}
		it.Category = "other"
		it.Date = trimDate(d)
		it.Link = ""
		items = append(items, it)
	}
	return items, rows.Err()
}

func nextMonthStart(year, month int) string {
	t := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
	return t.Format("2006-01-02")
}

func prevMonthRange(year, month int) (string, string) {
	t := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)
	start := t.Format("2006-01-02")
	endEx := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	return start, endEx
}

func trimDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// BuildCalendar 월 달력 격자(월~일) 생성
func BuildCalendar(year, month int, items []model.WorkCalItem) []model.WorkCalDay {
	byDate := map[string][]model.WorkCalItem{}
	for _, it := range items {
		byDate[it.Date] = append(byDate[it.Date], it)
	}

	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	last := first.AddDate(0, 1, -1)
	startPad := (int(first.Weekday()) + 6) % 7
	today := time.Now().In(time.Local).Format("2006-01-02")

	var days []model.WorkCalDay
	for i := 0; i < startPad; i++ {
		days = append(days, model.WorkCalDay{})
	}
	for d := 1; d <= last.Day(); d++ {
		date := fmt.Sprintf("%04d-%02d-%02d", year, month, d)
		list := byDate[date]
		days = append(days, model.WorkCalDay{
			Day:     d,
			Date:    date,
			Count:   len(list),
			Items:   list,
			IsToday: date == today,
			InMonth: true,
		})
	}
	for len(days)%7 != 0 {
		days = append(days, model.WorkCalDay{})
	}
	return days
}

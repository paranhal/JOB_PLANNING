package repository

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

// ListASReceivedBetween 기간(포함)에 신규 접수된 AS → 업무처리현황(접수) 일정표용.
func (r *WBRepo) ListASReceivedBetween(from, to string) ([]model.WorkTask, error) {
	toEx := nextDayExclusive(to)
	q := `
		SELECT ar.as_id, ar.as_number, ar.receipt_datetime, COALESCE(ar.assigned_to,''), ar.status,
		       c.org_name, COALESCE(ar.symptom,''),
		       COALESCE((
		         SELECT t.project_id FROM work_tasks t
		         WHERE t.source_type='as' AND t.source_id=ar.as_id AND TRIM(COALESCE(t.project_id,''))!=''
		         ORDER BY t.updated_at DESC LIMIT 1
		       ),'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE ar.receipt_datetime >= ? AND ar.receipt_datetime < ?
		ORDER BY ar.receipt_datetime, ar.as_number`
	rows, err := r.db.Query(q, dayTimeStart(from), dayTimeStart(toEx))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var out []model.WorkTask
	for rows.Next() {
		var asID, asNum, receiptDT, assignee, status, org, symptom, projectID string
		if err := rows.Scan(&asID, &asNum, &receiptDT, &assignee, &status, &org, &symptom, &projectID); err != nil {
			return nil, err
		}
		workDate, start, end := splitReceiptDateTime(receiptDT)
		title := "[AS]" + org
		if strings.TrimSpace(symptom) != "" {
			title = "[AS]" + org + " " + truncateRunes(symptom, 24)
		}
		out = append(out, model.WorkTask{
			TaskID:      "rcpt:" + asID,
			WorkType:    model.WBWorkAS,
			ProjectID:   projectID,
			Title:       title,
			Description: symptom,
			WorkDate:    workDate,
			StartTime:   start,
			EndTime:     end,
			DurationMin: 30,
			Status:      status,
			Assignee:    assignee,
			Tags:        asNum,
			SourceType:  model.WBSourceAS,
			SourceID:    asID,
			BoardHref:   "/as/" + asID,
		})
	}
	return out, rows.Err()
}

func nextDayExclusive(day string) string {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(day), time.Local)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

func splitReceiptDateTime(raw string) (date, start, end string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "09:00", "09:30"
	}
	// SQLite: "2006-01-02 15:04:05" or RFC
	var t time.Time
	var err error
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		t, err = time.ParseInLocation(layout, raw, time.Local)
		if err == nil {
			break
		}
	}
	if err != nil {
		if len(raw) >= 10 {
			return raw[:10], "09:00", "09:30"
		}
		return "", "09:00", "09:30"
	}
	date = t.Format("2006-01-02")
	min := t.Hour()*60 + t.Minute()
	// 업무시간(07~20) 밖이면 09:00에 표시
	if min < 7*60 || min >= 20*60 {
		min = 9 * 60
	}
	min = (min / 15) * 15
	start = fmt.Sprintf("%02d:%02d", min/60, min%60)
	endMin := min + 30
	if endMin > 20*60 {
		endMin = 20 * 60
	}
	end = fmt.Sprintf("%02d:%02d", endMin/60, endMin%60)
	return date, start, end
}

func truncateRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

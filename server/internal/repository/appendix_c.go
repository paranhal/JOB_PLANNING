package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// AppendixCCheck 부록 C 정합성 점검 1항목.
type AppendixCCheck struct {
	ID       string
	Purpose  string
	Count    int
	Expected string
	OK       bool
	Detail   string
	Rows     []AppendixCDateRow
}

// AppendixCDateRow 연도가 2000~2100 밖인 날짜 1건 (§8.2.3 V-11).
type AppendixCDateRow struct {
	Table    string
	Column   string
	EntityID string
	Number   string
	Value    string
}

const appendixCYearOutSQL = `
TRIM(COALESCE(%s,'')) != ''
AND TRIM(%s) GLOB '[0-9][0-9][0-9][0-9]*'
AND CAST(substr(TRIM(%s),1,4) AS INTEGER) NOT BETWEEN 2000 AND 2100`

// RunAppendixC 부록 C V-1~V-12. appOnly 이면 data_origin='app' 기준(V-4~V-7 권장).
// V-1~V-8 은 §4.5 지표 기준일 하한을 같은 조건으로 건다. V-9~V-12 은 전 기간(옛 자료 오류를 찾기 위함).
func RunAppendixC(db *sql.DB, appOnly bool) []AppendixCCheck {
	appAS := appendixCASWhere(db, "", appOnly)
	appR := appendixCASWhere(db, "r", appOnly)
	checks := []AppendixCCheck{
		runCountCheck(db, "V-1", "예정일 없이 확정된 건", "0",
			`SELECT COUNT(*) FROM as_receipts WHERE schedule_confirmed=1 AND TRIM(COALESCE(visit_scheduled_date,''))=''`+appAS),
		runCountCheck(db, "V-2", "미계획 AS (예정일·사유 모두 없음)", "0",
			`SELECT COUNT(*) FROM as_receipts WHERE status NOT IN `+model.SQLStatusStatsCompleted+` AND status != 'cancelled'
			 AND TRIM(COALESCE(visit_scheduled_date,''))='' AND TRIM(COALESCE(schedule_no_date_reason,''))=''`+appAS),
		runCountCheck(db, "V-3", "조치는 있는데 착수시각 없음", "0",
			`SELECT COUNT(*) FROM as_receipts r WHERE TRIM(COALESCE(r.start_datetime,''))=''
			 AND EXISTS(SELECT 1 FROM as_processes p WHERE p.as_id=r.as_id)`+appR),
		runCountCheck(db, "V-4", "소요시간 미입력 조치", "신규분 0",
			`SELECT COUNT(*) FROM as_processes p JOIN as_receipts r ON r.as_id=p.as_id
			 WHERE COALESCE(p.time_spent,0)<=0`+appR),
		runCountCheck(db, "V-5", "근무구분 미입력 완료건", "신규분 0",
			`SELECT COUNT(*) FROM as_receipts WHERE status IN `+model.SQLStatusStatsCompleted+`
			 AND TRIM(COALESCE(work_place,''))=''`+appAS),
		runCountCheck(db, "V-6", "원인분류 미입력 완료건", "신규분 0",
			`SELECT COUNT(*) FROM as_receipts WHERE status IN `+model.SQLStatusStatsCompleted+`
			 AND TRIM(COALESCE(cause_type,''))=''`+appAS),
		runCountCheck(db, "V-7", "자산 미연결·사유도 없음", "신규분 0",
			`SELECT COUNT(*) FROM as_receipts WHERE TRIM(COALESCE(asset_id,''))=''
			 AND TRIM(COALESCE(asset_unlinked_reason,''))=''`+appAS),
	}

	v8 := AppendixCCheck{ID: "V-8", Purpose: "데이터 출처 분포", Expected: "app 비중 증가"}
	v8q := `SELECT COALESCE(data_origin,'app'), COUNT(*) FROM as_receipts WHERE 1=1` + appendixCASWhere(db, "", false) + ` GROUP BY 1 ORDER BY 2 DESC`
	rows, err := db.Query(v8q)
	if err == nil {
		defer rows.Close()
		var parts []string
		total := 0
		for rows.Next() {
			var origin string
			var n int
			if rows.Scan(&origin, &n) == nil {
				parts = append(parts, fmt.Sprintf("%s %d", origin, n))
				total += n
			}
		}
		v8.Count = total
		v8.Detail = strings.Join(parts, " · ")
		v8.OK = true
	} else {
		v8.Detail = err.Error()
	}
	checks = append(checks, v8)

	checks = append(checks,
		runCountCheck(db, "V-9", "정기점검 방문 중복", "0",
			`SELECT COUNT(*) FROM (
			   SELECT plan_id, visit_date, customer_id, COALESCE(product_type,'')
			   FROM maintenance_visits GROUP BY 1,2,3,4 HAVING COUNT(*)>1)`),
		runCountCheck(db, "V-10", "사업 연결된 정기점검 방문", "증가 추세",
			`SELECT COUNT(*) FROM maintenance_visits WHERE TRIM(COALESCE(project_id,''))<>''`),
	)

	v11 := AppendixCCheck{
		ID:       "V-11",
		Purpose:  "연도가 2000~2100 밖인 날짜",
		Expected: "0",
	}
	dateRows, n, err := listDateYearOutOfRange(db)
	if err != nil {
		v11.Detail = err.Error()
	} else {
		v11.Rows = dateRows
		v11.Count = n
		v11.OK = n == 0
		v11.Detail = "기존 값은 자동으로 고치지 않습니다. 목록에서 건별로 수정하세요."
		if n > len(dateRows) {
			v11.Detail += fmt.Sprintf(" (표시 %d / 전체 %d)", len(dateRows), n)
		}
	}
	checks = append(checks, v11)

	v12 := AppendixCCheck{
		ID:       "V-12",
		Purpose:  "착수가 완료보다 늦은 건",
		Expected: "0",
	}
	lateRows, n12, err := listStartAfterComplete(db)
	if err != nil {
		v12.Detail = err.Error()
	} else {
		v12.Rows = lateRows
		v12.Count = n12
		v12.OK = n12 == 0
		v12.Detail = "start_datetime > complete_datetime. 자동으로 고치지 않습니다."
		if n12 > len(lateRows) {
			v12.Detail += fmt.Sprintf(" (표시 %d / 전체 %d)", len(lateRows), n12)
		}
	}
	checks = append(checks, v12)
	return checks
}

func originOn(alias string, appOnly bool) string {
	if !appOnly {
		return ""
	}
	col := "data_origin"
	if alias != "" {
		col = alias + `.data_origin`
	}
	return ` AND COALESCE(` + col + `,'app')='app'`
}

func appendixCASWhere(db *sql.DB, alias string, appOnly bool) string {
	w := originOn(alias, appOnly)
	d := metricsBaseFromDB(db)
	if len(d) != 10 || d[4] != '-' || d[7] != '-' {
		return w
	}
	col := "receipt_datetime"
	if alias != "" {
		col = alias + `.receipt_datetime`
	}
	return w + ` AND date(` + col + `) >= date('` + d + `')`
}

func runCountCheck(db *sql.DB, id, purpose, expected, q string) AppendixCCheck {
	c := AppendixCCheck{ID: id, Purpose: purpose, Expected: expected}
	err := db.QueryRow(q).Scan(&c.Count)
	if err != nil {
		c.Detail = err.Error()
		return c
	}
	if expected == "0" {
		c.OK = c.Count == 0
	} else {
		c.OK = true
	}
	return c
}

func listDateYearOutOfRange(db *sql.DB) ([]AppendixCDateRow, int, error) {
	specs := [][4]string{
		{"as_receipts", "visit_scheduled_date", "as_id", "as_number"},
		{"as_receipts", "receipt_datetime", "as_id", "as_number"},
		{"as_receipts", "complete_datetime", "as_id", "as_number"},
		{"maintenance_visits", "visit_date", "visit_id", "visit_id"},
		{"work_tasks", "work_date", "task_id", "title"},
		{"work_tasks", "due_date", "task_id", "title"},
	}
	total := 0
	var out []AppendixCDateRow
	for _, s := range specs {
		table, col, idCol, numCol := s[0], s[1], s[2], s[3]
		where := fmt.Sprintf(appendixCYearOutSQL, col, col, col)
		var n int
		if err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s`, table, where)).Scan(&n); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "no such table") ||
				strings.Contains(strings.ToLower(err.Error()), "no such column") {
				continue
			}
			return out, total, err
		}
		total += n
		if len(out) >= 200 {
			continue
		}
		limit := 200 - len(out)
		q := fmt.Sprintf(`SELECT %s, COALESCE(%s,''), %s FROM %s WHERE %s LIMIT %d`,
			idCol, numCol, col, table, where, limit)
		rows, err := db.Query(q)
		if err != nil {
			return out, total, err
		}
		for rows.Next() {
			var r AppendixCDateRow
			r.Table, r.Column = table, col
			if rows.Scan(&r.EntityID, &r.Number, &r.Value) != nil {
				continue
			}
			out = append(out, r)
		}
		rows.Close()
	}
	return out, total, nil
}

func listStartAfterComplete(db *sql.DB) ([]AppendixCDateRow, int, error) {
	q := `
SELECT as_id, COALESCE(as_number,''), start_datetime, complete_datetime
FROM as_receipts
WHERE TRIM(COALESCE(start_datetime,'')) != ''
  AND TRIM(COALESCE(complete_datetime,'')) != ''
  AND datetime(start_datetime) > datetime(complete_datetime)
ORDER BY as_number`
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_receipts
		WHERE TRIM(COALESCE(start_datetime,'')) != ''
		  AND TRIM(COALESCE(complete_datetime,'')) != ''
		  AND datetime(start_datetime) > datetime(complete_datetime)`).Scan(&n); err != nil {
		return nil, 0, err
	}
	rows, err := db.Query(q)
	if err != nil {
		return nil, n, err
	}
	defer rows.Close()
	var out []AppendixCDateRow
	for rows.Next() {
		var r AppendixCDateRow
		var start, complete string
		r.Table, r.Column = "as_receipts", "start_datetime"
		if rows.Scan(&r.EntityID, &r.Number, &start, &complete) != nil {
			continue
		}
		r.Value = start + " > " + complete
		out = append(out, r)
		if len(out) >= 200 {
			break
		}
	}
	return out, n, rows.Err()
}

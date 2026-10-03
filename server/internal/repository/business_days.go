package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"customer-support/internal/model"
)

const businessDaysHolidayCountKeyPrefix = "__meta:bd_hcount:"

func rebuildBusinessDaysFrom(db *sql.DB, year int) {
	if db == nil {
		return
	}
	if err := RebuildBusinessDays(db, year); err != nil {
		log.Printf("business_days rebuild %d: %v", year, err)
	}
}

func applyBusinessDays(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS business_days (
		d             TEXT PRIMARY KEY,
		is_workday    INTEGER NOT NULL,
		bd_index      INTEGER NOT NULL,
		next_bd_index INTEGER NOT NULL
	)`); err != nil {
		log.Printf("business_days table: %v", err)
		return
	}
	if err := RebuildBusinessDays(db, 0); err != nil {
		log.Printf("business_days fill: %v", err)
	}
}

// RebuildBusinessDays 영업일 표를 채운다. fromYear==0 이면 공휴일 수가 바뀐 해부터.
// 1회성 가드를 쓰지 않는다. holidays 가 바뀌면 다시 계산하는 파생 표다. §4.14.2
func RebuildBusinessDays(db *sql.DB, fromYear int) error {
	if db == nil {
		return nil
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS business_days (
		d             TEXT PRIMARY KEY,
		is_workday    INTEGER NOT NULL,
		bd_index      INTEGER NOT NULL,
		next_bd_index INTEGER NOT NULL
	)`); err != nil {
		return err
	}
	rangeFrom, rangeTo := businessDayYearRange(db)
	if fromYear > 0 && fromYear < rangeFrom {
		rangeFrom = fromYear
	}
	holidays := loadHolidayDateSet(db)
	counts := holidayCountsByYear(db, rangeFrom, rangeTo)
	rebuildFrom := fromYear
	if rebuildFrom <= 0 {
		rebuildFrom = firstBusinessDayMismatchYear(db, rangeFrom, rangeTo, counts)
	}
	for y := rangeFrom; y <= rangeTo; y++ {
		if counts[y] == 0 {
			log.Printf("business_days: %d년 공휴일 미등록 — 주말만 반영", y)
		}
	}
	if rebuildFrom <= 0 {
		return stampHolidayCounts(db, rangeFrom, rangeTo, counts)
	}
	start := time.Date(rebuildFrom, 1, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(rangeTo, 12, 31, 0, 0, 0, 0, time.Local)
	if _, err := db.Exec(`DELETE FROM business_days WHERE d >= ?`, start.Format("2006-01-02")); err != nil {
		return err
	}
	bd := 0
	prev := start.AddDate(0, 0, -1).Format("2006-01-02")
	var prevBD sql.NullInt64
	if err := db.QueryRow(`SELECT bd_index FROM business_days WHERE d=?`, prev).Scan(&prevBD); err == nil && prevBD.Valid {
		bd = int(prevBD.Int64)
	}
	type row struct {
		d    string
		work int
		bd   int
	}
	var rows []row
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		_, hit := holidays[ds]
		work := 0
		if model.DateIsWorkingDay(ds, hit) {
			work = 1
			bd++
		}
		rows = append(rows, row{d: ds, work: work, bd: bd})
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT INTO business_days(d, is_workday, bd_index, next_bd_index) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	n := len(rows)
	nexts := make([]int, n)
	nextBD := bd
	for i := n - 1; i >= 0; i-- {
		if rows[i].work == 1 {
			nextBD = rows[i].bd
		}
		nexts[i] = nextBD
	}
	for i, r := range rows {
		if _, err := stmt.Exec(r.d, r.work, r.bd, nexts[i]); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return stampHolidayCounts(db, rangeFrom, rangeTo, counts)
}

func businessDayYearRange(db *sql.DB) (fromY, toY int) {
	now := time.Now()
	toY = now.Year() + 1
	fromY = now.Year() - 1
	var minS sql.NullString
	err := db.QueryRow(`
		SELECT MIN(d) FROM (
			SELECT date(receipt_datetime) AS d FROM as_receipts WHERE TRIM(COALESCE(receipt_datetime,'')) != ''
			UNION ALL SELECT date(complete_datetime) FROM as_receipts WHERE TRIM(COALESCE(complete_datetime,'')) != ''
			UNION ALL SELECT date(visit_date) FROM as_receipts WHERE TRIM(COALESCE(visit_date,'')) != ''
			UNION ALL SELECT holiday_date FROM holidays WHERE TRIM(COALESCE(holiday_date,'')) != ''
		) WHERE d IS NOT NULL AND TRIM(d) != ''`).Scan(&minS)
	if err == nil && minS.Valid && len(minS.String) >= 4 {
		if y, e := strconv.Atoi(minS.String[:4]); e == nil && y >= 1990 && y <= 2100 {
			fromY = y - 1
		}
	}
	if fromY > toY {
		fromY = toY
	}
	return fromY, toY
}

func loadHolidayDateSet(db *sql.DB) map[string]struct{} {
	out := map[string]struct{}{}
	rows, err := db.Query(`SELECT holiday_date FROM holidays`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			continue
		}
		d = strings.TrimSpace(d)
		if len(d) >= 10 {
			d = d[:10]
		}
		if d != "" {
			out[d] = struct{}{}
		}
	}
	return out
}

func holidayCountsByYear(db *sql.DB, fromY, toY int) map[int]int {
	out := map[int]int{}
	for y := fromY; y <= toY; y++ {
		out[y] = 0
	}
	rows, err := db.Query(`SELECT holiday_year, COUNT(*) FROM holidays
		WHERE holiday_year >= ? AND holiday_year <= ? GROUP BY holiday_year`, fromY, toY)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var y, n int
		if err := rows.Scan(&y, &n); err != nil {
			continue
		}
		out[y] = n
	}
	return out
}

func firstBusinessDayMismatchYear(db *sql.DB, fromY, toY int, counts map[int]int) int {
	var minD, maxD sql.NullString
	_ = db.QueryRow(`SELECT MIN(d), MAX(d) FROM business_days`).Scan(&minD, &maxD)
	wantMin := fmt.Sprintf("%d-01-01", fromY)
	wantMax := fmt.Sprintf("%d-12-31", toY)
	if !minD.Valid || minD.String > wantMin || !maxD.Valid || maxD.String < wantMax {
		return fromY
	}
	for y := fromY; y <= toY; y++ {
		stored := storedHolidayCount(db, y)
		if stored != counts[y] {
			return y
		}
	}
	return 0
}

func storedHolidayCount(db *sql.DB, year int) int {
	var n int
	err := db.QueryRow(`SELECT last_no FROM id_sequences WHERE seq_key=?`, businessDaysHolidayCountKey(year)).Scan(&n)
	if err != nil {
		return -1
	}
	return n
}

func stampHolidayCounts(db *sql.DB, fromY, toY int, counts map[int]int) error {
	for y := fromY; y <= toY; y++ {
		if _, err := db.Exec(`INSERT OR REPLACE INTO id_sequences(seq_key, last_no) VALUES(?,?)`,
			businessDaysHolidayCountKey(y), counts[y]); err != nil {
			return err
		}
	}
	return nil
}

func businessDaysHolidayCountKey(year int) string {
	return businessDaysHolidayCountKeyPrefix + strconv.Itoa(year)
}

func lookupBusinessDay(db *sql.DB, date string) (bdIndex, nextBD int, work bool, err error) {
	_, key, ok := model.ParseYMD(date)
	if !ok {
		return 0, 0, false, fmt.Errorf("날짜 아님")
	}
	var w int
	err = db.QueryRow(`SELECT is_workday, bd_index, next_bd_index FROM business_days WHERE d=?`, key).
		Scan(&w, &bdIndex, &nextBD)
	return bdIndex, nextBD, w == 1, err
}

func yearsMissingHolidays(db *sql.DB, from, toEx string) []int {
	_, a, ok := model.ParseYMD(from)
	if !ok {
		return nil
	}
	end := strings.TrimSpace(toEx)
	if len(end) >= 10 {
		end = end[:10]
	}
	t0, err0 := time.ParseInLocation("2006-01-02", a, time.Local)
	t1, err1 := time.ParseInLocation("2006-01-02", end, time.Local)
	if err0 != nil || err1 != nil {
		return nil
	}
	if !t1.After(t0) {
		t1 = t0.AddDate(0, 0, 1)
	}
	var out []int
	repo := NewHolidayRepo(db)
	for y := t0.Year(); y <= t1.AddDate(0, 0, -1).Year(); y++ {
		if repo.YearMissing(y) {
			out = append(out, y)
		}
	}
	return out
}

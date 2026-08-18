package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

const holidaySelectCols = `holiday_date, holiday_year, name, kind, IFNULL(created_at,''), IFNULL(source,''), IFNULL(synced_at,'')`

type HolidayRepo struct{ db *sql.DB }

func NewHolidayRepo(db *sql.DB) *HolidayRepo {
	if db == nil {
		return nil
	}
	return &HolidayRepo{db: db}
}

func (r *MaintenanceRepo) Holidays() *HolidayRepo {
	if r == nil || r.db == nil {
		return nil
	}
	return NewHolidayRepo(r.db)
}

func (r *HolidayRepo) CountAll() (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM holidays`).Scan(&n)
	return n, err
}

func (r *HolidayRepo) CountYear(year int) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM holidays WHERE holiday_year=?`, year).Scan(&n)
	return n, err
}

func (r *HolidayRepo) YearMissing(year int) bool {
	n, err := r.CountYear(year)
	return err == nil && n == 0
}

func (r *HolidayRepo) HasDate(date string) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}
	date = normalizeHolidayDate(date)
	if date == "" {
		return false, nil
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM holidays WHERE holiday_date=?`, date).Scan(&n)
	return n > 0, err
}

func (r *HolidayRepo) Get(date string) (*model.Holiday, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	date = normalizeHolidayDate(date)
	row := r.db.QueryRow(`SELECT `+holidaySelectCols+` FROM holidays WHERE holiday_date=?`, date)
	h, err := scanHoliday(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return h, err
}

func (r *HolidayRepo) ListByYear(year int) ([]model.Holiday, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`SELECT `+holidaySelectCols+`
		FROM holidays WHERE holiday_year=? ORDER BY holiday_date`, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHolidays(rows)
}

func (r *HolidayRepo) ListYears() ([]int, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`SELECT DISTINCT holiday_year FROM holidays ORDER BY holiday_year`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var years []int
	for rows.Next() {
		var y int
		if err := rows.Scan(&y); err != nil {
			return nil, err
		}
		years = append(years, y)
	}
	return years, rows.Err()
}

func (r *HolidayRepo) LastSyncedAt(year int) (string, error) {
	if r == nil || r.db == nil || year <= 0 {
		return "", nil
	}
	var s sql.NullString
	err := r.db.QueryRow(`
		SELECT MAX(synced_at) FROM holidays
		 WHERE holiday_year=? AND TRIM(COALESCE(synced_at,'')) != ''`, year).Scan(&s)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s.String), nil
}

func (r *HolidayRepo) DatesInRange(from, toInclusive string) (map[string]bool, error) {
	out := map[string]bool{}
	if r == nil || r.db == nil {
		return out, nil
	}
	from = normalizeHolidayDate(from)
	toInclusive = normalizeHolidayDate(toInclusive)
	if from == "" || toInclusive == "" {
		return out, nil
	}
	rows, err := r.db.Query(
		`SELECT holiday_date FROM holidays WHERE holiday_date >= ? AND holiday_date <= ?`,
		from, toInclusive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		d = normalizeHolidayDate(d)
		if d != "" {
			out[d] = true
		}
	}
	return out, rows.Err()
}

func (r *HolidayRepo) DatesSet(year int) (map[string]bool, error) {
	out := map[string]bool{}
	if r == nil || r.db == nil || year <= 0 {
		return out, nil
	}
	if r.YearMissing(year) {
		return out, nil
	}
	rows, err := r.db.Query(`SELECT holiday_date FROM holidays WHERE holiday_year=?`, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		d = normalizeHolidayDate(d)
		if d != "" {
			out[d] = true
		}
	}
	return out, rows.Err()
}

func (r *HolidayRepo) Create(h model.Holiday) error {
	h, err := normalizeHoliday(h)
	if err != nil {
		return err
	}
	if h.Source == "" {
		h.Source = model.HolidaySourceManual
	}
	_, err = r.db.Exec(
		`INSERT INTO holidays (holiday_date, holiday_year, name, kind, source) VALUES (?,?,?,?,?)`,
		h.Date, h.Year, h.Name, h.Kind, h.Source)
	if err != nil {
		if isUniqueErr(err) {
			return fmt.Errorf("이미 등록된 날짜입니다")
		}
		return err
	}
	logCreate(r.db, "holidays", "holiday_date", h.Date, h.Name)
	BumpHolidayData()
	return nil
}

func (r *HolidayRepo) Update(oldDate string, h model.Holiday) error {
	oldDate = normalizeHolidayDate(oldDate)
	h, err := normalizeHoliday(h)
	if err != nil {
		return err
	}
	if oldDate == "" {
		return fmt.Errorf("날짜를 입력하세요")
	}
	// 손으로 고치면 API 재동기화가 덮지 못하게 manual 로 둔다. §23.13.3
	h.Source = model.HolidaySourceManual
	label := h.Name
	if oldDate == h.Date {
		return touchUpdate(r.db, "holidays", "holiday_date", h.Date, label, func() error {
			res, err := r.db.Exec(
				`UPDATE holidays SET name=?, kind=?, holiday_year=?, source=? WHERE holiday_date=?`,
				h.Name, h.Kind, h.Year, h.Source, h.Date)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return fmt.Errorf("해당 휴무일을 찾을 수 없습니다")
			}
			BumpHolidayData()
			return nil
		})
	}
	exists, err := r.HasDate(h.Date)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("이미 등록된 날짜입니다")
	}
	before := rowJSON(r.db, "holidays", "holiday_date", oldDate)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`INSERT INTO holidays (holiday_date, holiday_year, name, kind, source) VALUES (?,?,?,?,?)`,
		h.Date, h.Year, h.Name, h.Kind, h.Source); err != nil {
		if isUniqueErr(err) {
			return fmt.Errorf("이미 등록된 날짜입니다")
		}
		return err
	}
	res, err := tx.Exec(`DELETE FROM holidays WHERE holiday_date=?`, oldDate)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("해당 휴무일을 찾을 수 없습니다")
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logUpdate(r.db, "holidays", "holiday_date", h.Date, label, before)
	BumpHolidayData()
	return nil
}

func (r *HolidayRepo) Delete(date string) error {
	date = normalizeHolidayDate(date)
	if date == "" {
		return fmt.Errorf("날짜를 입력하세요")
	}
	h, _ := r.Get(date)
	label := date
	if h != nil && h.Name != "" {
		label = h.Name
	}
	return touchDelete(r.db, "holidays", "holiday_date", date, label, func() error {
		_, err := r.db.Exec(`DELETE FROM holidays WHERE holiday_date=?`, date)
		if err == nil {
			BumpHolidayData()
		}
		return err
	})
}

// SeedBuiltinIfEmpty 테이블이 비어 있을 때만 넣는다. InitDB 시드 이후에는 0건이다.
func (r *HolidayRepo) SeedBuiltinIfEmpty(items []model.Holiday) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	n, err := r.CountAll()
	if err != nil || n > 0 {
		return 0, err
	}
	return r.insertIgnore(items)
}

// ImportYear 해당 연도 기본 공휴일을 넣는다. 이미 있는 날짜는 건너뛴다.
func (r *HolidayRepo) ImportYear(year int, items []model.Holiday) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var yearItems []model.Holiday
	for _, h := range items {
		if h.Year == year || (h.Year == 0 && strings.HasPrefix(h.Date, fmt.Sprintf("%d-", year))) {
			yearItems = append(yearItems, h)
		}
	}
	if len(yearItems) == 0 {
		return 0, fmt.Errorf("기본 데이터가 없습니다. 직접 등록하세요")
	}
	return r.insertIgnore(yearItems)
}

// ReplaceAPIYear API 결과로 해당 연도 source='api' 행만 맞춘다. manual 은 INSERT/UPDATE/DELETE 하지 않는다. §23.13.3
func (r *HolidayRepo) ReplaceAPIYear(year int, items []model.Holiday, syncedAt string) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	keep := map[string]bool{}
	upserted := 0
	for _, raw := range items {
		h, err := normalizeHoliday(raw)
		if err != nil {
			return upserted, err
		}
		if h.Year != year {
			continue
		}
		n, err := r.upsertAPI(h, syncedAt)
		if err != nil {
			return upserted, err
		}
		upserted += n
		keep[h.Date] = true
	}
	existing, err := r.ListByYear(year)
	if err != nil {
		return upserted, err
	}
	for _, h := range existing {
		if h.Source == model.HolidaySourceManual {
			continue
		}
		if keep[h.Date] {
			continue
		}
		if _, err := r.db.Exec(`DELETE FROM holidays WHERE holiday_date=? AND source=?`,
			h.Date, model.HolidaySourceAPI); err != nil {
			return upserted, err
		}
	}
	BumpHolidayData()
	return upserted, nil
}

func (r *HolidayRepo) upsertAPI(h model.Holiday, syncedAt string) (int, error) {
	res, err := r.db.Exec(`
		INSERT INTO holidays (holiday_date, holiday_year, name, kind, source, synced_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(holiday_date) DO UPDATE SET
			name=excluded.name,
			kind=excluded.kind,
			holiday_year=excluded.holiday_year,
			source='api',
			synced_at=excluded.synced_at
		WHERE COALESCE(holidays.source,'') != 'manual'`,
		h.Date, h.Year, h.Name, h.Kind, model.HolidaySourceAPI, syncedAt)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return 1, nil
	}
	return 0, nil
}

func (r *HolidayRepo) insertIgnore(items []model.Holiday) (int, error) {
	inserted := 0
	for _, raw := range items {
		h, err := normalizeHoliday(raw)
		if err != nil {
			return inserted, err
		}
		src := h.Source
		if src == "" {
			src = model.HolidaySourceAPI
		}
		res, err := r.db.Exec(
			`INSERT OR IGNORE INTO holidays (holiday_date, holiday_year, name, kind, source) VALUES (?,?,?,?,?)`,
			h.Date, h.Year, h.Name, h.Kind, src)
		if err != nil {
			return inserted, err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			inserted++
		}
	}
	if inserted > 0 {
		BumpHolidayData()
	}
	return inserted, nil
}

func normalizeHoliday(h model.Holiday) (model.Holiday, error) {
	h.Date = normalizeHolidayDate(h.Date)
	if h.Date == "" {
		return h, fmt.Errorf("날짜를 입력하세요")
	}
	t, err := time.ParseInLocation("2006-01-02", h.Date, time.Local)
	if err != nil {
		return h, fmt.Errorf("날짜 형식이 올바르지 않습니다")
	}
	h.Year = t.Year()
	h.Name = strings.TrimSpace(h.Name)
	if h.Name == "" {
		return h, fmt.Errorf("휴무일 이름을 입력하세요")
	}
	h.Kind = strings.TrimSpace(h.Kind)
	if h.Kind == "" {
		h.Kind = model.HolidayKindPublic
	}
	if !model.ValidHolidayKind(h.Kind) {
		return h, fmt.Errorf("구분이 올바르지 않습니다")
	}
	h.Source = strings.TrimSpace(h.Source)
	if h.Source != "" && !model.ValidHolidaySource(h.Source) {
		return h, fmt.Errorf("출처가 올바르지 않습니다")
	}
	return h, nil
}

func normalizeHolidayDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		s = s[:10]
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return ""
	}
	return s
}

func scanHoliday(row *sql.Row) (*model.Holiday, error) {
	var h model.Holiday
	if err := row.Scan(&h.Date, &h.Year, &h.Name, &h.Kind, &h.CreatedAt, &h.Source, &h.SyncedAt); err != nil {
		return nil, err
	}
	h.Date = normalizeHolidayDate(h.Date)
	return &h, nil
}

func scanHolidays(rows *sql.Rows) ([]model.Holiday, error) {
	var out []model.Holiday
	for rows.Next() {
		var h model.Holiday
		if err := rows.Scan(&h.Date, &h.Year, &h.Name, &h.Kind, &h.CreatedAt, &h.Source, &h.SyncedAt); err != nil {
			return nil, err
		}
		h.Date = normalizeHolidayDate(h.Date)
		out = append(out, h)
	}
	return out, rows.Err()
}

func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "constraint")
}

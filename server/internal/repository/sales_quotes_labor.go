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

const (
	SettingQuoteLastRound   = "quote_last_round_rule"
	SettingQuoteValid       = "quote_valid_until_text"
	SettingQuoteDue         = "quote_due_text"
	SettingQuotePlace       = "quote_place_text"
	SettingQuotePayment     = "quote_payment_text"
	SettingQuoteOverhead    = "quote_overhead_rate"
	SettingQuoteTechFee     = "quote_tech_fee_rate"
	SettingQuoteCompanyName = "quote_company_name"
)

func applyQuoteLabor(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS labor_rates (
			rate_id         TEXT PRIMARY KEY,
			year            INTEGER NOT NULL,
			job_code        TEXT NOT NULL,
			job_name        TEXT NOT NULL,
			monthly         INTEGER NOT NULL DEFAULT 0,
			daily           INTEGER NOT NULL DEFAULT 0,
			hourly          INTEGER NOT NULL DEFAULT 0,
			effective_from  TEXT NOT NULL DEFAULT '',
			effective_to    TEXT NOT NULL DEFAULT '',
			source_note     TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		log.Printf("037 labor_rates: %v", err)
		return
	}
	_, _ = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_labor_rates_year_job ON labor_rates(year, job_code)`)
	seedLaborRates2026(db)
	if v, _ := NewSettingsRepo(db).Get(SettingQuoteOverhead); strings.TrimSpace(v) == "" {
		_ = NewSettingsRepo(db).Set(SettingQuoteOverhead, "110")
	}
	if v, _ := NewSettingsRepo(db).Get(SettingQuoteTechFee); strings.TrimSpace(v) == "" {
		_ = NewSettingsRepo(db).Set(SettingQuoteTechFee, "20")
	}
}

func seedLaborRates2026(db *sql.DB) {
	jobs := []struct {
		code, name string
		monthly    int
	}{
		{"01", "IT기획자", 0},
		{"02", "IT컨설턴트", 0},
		{"03", "업무분석가", 0},
		{"04", "데이터분석가", 0},
		{"05", "IT PM", 0},
		{"06", "IT아키텍트", 0},
		{"07", "UI/UX기획/개발자", 6_901_660},
		{"08", "UI/UX디자이너", 5_159_246},
		{"09", "응용SW개발자", 7_754_124},
		{"10", "시스템SW개발자", 0},
		{"11", "정보시스템운용자", 0},
		{"12", "IT지원기술자", 0},
		{"13", "IT마케터", 0},
		{"14", "IT품질관리자", 0},
		{"15", "IT테스터", 0},
		{"16", "IT감리", 0},
		{"17", "정보보안전문가", 0},
	}
	for _, j := range jobs {
		id := fmt.Sprintf("LR2026-%s", j.code)
		_, err := db.Exec(`
			INSERT OR IGNORE INTO labor_rates (rate_id, year, job_code, job_name, monthly, effective_from, effective_to, source_note)
			VALUES (?,?,?,?,?,'2026-01-01','2026-12-31','한국소프트웨어산업협회 2026')`,
			id, 2026, j.code, j.name, j.monthly)
		if err != nil {
			log.Printf("037 labor seed %s: %v", j.code, err)
		}
	}
}

func (r *QuoteRepo) ListLaborRates(year int) ([]model.LaborRate, error) {
	rows, err := r.db.Query(`
		SELECT rate_id, year, job_code, job_name, monthly, daily, hourly,
			COALESCE(effective_from,''), COALESCE(effective_to,''), COALESCE(source_note,'')
		FROM labor_rates WHERE year=? ORDER BY job_code`, year)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.LaborRate
	for rows.Next() {
		var it model.LaborRate
		if err := rows.Scan(&it.RateID, &it.Year, &it.JobCode, &it.JobName, &it.Monthly, &it.Daily, &it.Hourly,
			&it.EffectiveFrom, &it.EffectiveTo, &it.SourceNote); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *QuoteRepo) ListLaborYears() ([]int, error) {
	rows, err := r.db.Query(`SELECT DISTINCT year FROM labor_rates ORDER BY year DESC`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var y int
		if err := rows.Scan(&y); err != nil {
			return nil, err
		}
		out = append(out, y)
	}
	return out, rows.Err()
}

func (r *QuoteRepo) ResolveLaborYear(want int) (use int, missing bool, note string) {
	if want <= 0 {
		want = time.Now().Year()
	}
	items, _ := r.ListLaborRates(want)
	if len(items) > 0 {
		return want, false, ""
	}
	years, _ := r.ListLaborYears()
	if len(years) == 0 {
		return want, true, fmt.Sprintf("%d년 단가 없음", want)
	}
	return years[0], true, fmt.Sprintf("%d년 단가 없음 — %d년 기준", want, years[0])
}

func (r *QuoteRepo) GetLaborRateByYearJob(year int, jobCode string) (*model.LaborRate, error) {
	var it model.LaborRate
	err := r.db.QueryRow(`
		SELECT rate_id, year, job_code, job_name, monthly, daily, hourly,
			COALESCE(effective_from,''), COALESCE(effective_to,''), COALESCE(source_note,'')
		FROM labor_rates WHERE year=? AND job_code=?`, year, strings.TrimSpace(jobCode)).Scan(
		&it.RateID, &it.Year, &it.JobCode, &it.JobName, &it.Monthly, &it.Daily, &it.Hourly,
		&it.EffectiveFrom, &it.EffectiveTo, &it.SourceNote)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (r *QuoteRepo) SaveLaborRates(year int, items []model.LaborRate) (added, changed int, err error) {
	if year < 2000 {
		return 0, 0, fmt.Errorf("연도가 필요합니다")
	}
	for _, it := range items {
		code := strings.TrimSpace(it.JobCode)
		name := strings.TrimSpace(it.JobName)
		if code == "" || name == "" {
			continue
		}
		id := fmt.Sprintf("LR%d-%s", year, code)
		cur, _ := r.GetLaborRateByYearJob(year, code)
		from := fmt.Sprintf("%d-01-01", year)
		to := fmt.Sprintf("%d-12-31", year)
		if cur == nil {
			_, err = r.db.Exec(`
				INSERT INTO labor_rates (rate_id, year, job_code, job_name, monthly, daily, hourly, effective_from, effective_to, source_note)
				VALUES (?,?,?,?,?,?,?,?,?,?)`,
				id, year, code, name, it.Monthly, it.Daily, it.Hourly, from, to, it.SourceNote)
			if err != nil {
				return added, changed, err
			}
			added++
			continue
		}
		if cur.JobName == name && cur.Monthly == it.Monthly && cur.Daily == it.Daily && cur.Hourly == it.Hourly {
			continue
		}
		_, err = r.db.Exec(`
			UPDATE labor_rates SET job_name=?, monthly=?, daily=?, hourly=?, source_note=CASE WHEN TRIM(?)!='' THEN ? ELSE source_note END
			WHERE year=? AND job_code=?`,
			name, it.Monthly, it.Daily, it.Hourly, it.SourceNote, it.SourceNote, year, code)
		if err != nil {
			return added, changed, err
		}
		changed++
	}
	if added+changed > 0 {
		logCreate(r.db, "labor_rates", "rate_id", fmt.Sprintf("LR%d", year),
			fmt.Sprintf("노임단가 %d년 %d건 반영(추가 %d · 변경 %d)", year, added+changed, added, changed))
	}
	return added, changed, nil
}

func (r *QuoteRepo) CopyLaborRates(from, to int) (copied int, err error) {
	src, err := r.ListLaborRates(from)
	if err != nil {
		return 0, err
	}
	var neu []model.LaborRate
	for _, it := range src {
		exist, _ := r.GetLaborRateByYearJob(to, it.JobCode)
		if exist != nil {
			continue
		}
		it.Year = to
		it.RateID = ""
		neu = append(neu, it)
	}
	added, _, err := r.SaveLaborRates(to, neu)
	return added, err
}

func (r *QuoteRepo) GetLaborRate(id string) (*model.LaborRate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	var it model.LaborRate
	err := r.db.QueryRow(`
		SELECT rate_id, year, job_code, job_name, monthly, daily, hourly,
			COALESCE(effective_from,''), COALESCE(effective_to,''), COALESCE(source_note,'')
		FROM labor_rates WHERE rate_id=?`, id).Scan(
		&it.RateID, &it.Year, &it.JobCode, &it.JobName, &it.Monthly, &it.Daily, &it.Hourly,
		&it.EffectiveFrom, &it.EffectiveTo, &it.SourceNote)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (r *QuoteRepo) StandardRates() (overhead, tech float64) {
	oh := settingFloat(r.db, SettingQuoteOverhead, 110)
	tech = settingFloat(r.db, SettingQuoteTechFee, 20)
	return oh, tech
}

func (r *QuoteRepo) SetStandardRates(overhead, tech float64) error {
	if err := NewSettingsRepo(r.db).Set(SettingQuoteOverhead, formatRate(overhead)); err != nil {
		return err
	}
	return NewSettingsRepo(r.db).Set(SettingQuoteTechFee, formatRate(tech))
}

func settingFloat(db *sql.DB, key string, fallback float64) float64 {
	v, err := NewSettingsRepo(db).Get(key)
	if err != nil || strings.TrimSpace(v) == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return fallback
	}
	return n
}

func formatRate(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

func LoadQuoteCompany(db *sql.DB) model.QuoteCompany {
	name, _ := NewSettingsRepo(db).Get(SettingQuoteCompanyName)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "㈜비젼아이티"
	}
	return model.QuoteCompany{Name: name}
}

package repository

import (
	"database/sql"
	"log"
	"strings"
	"time"

	"customer-support/internal/model"
)

// applyMetricsSettings 019. INSERT OR IGNORE — 관리자가 바꾼 값은 덮지 않는다.
// 시드 값은 마이그레이션 SQL 과 같다. 집계는 항상 app_settings 를 읽는다 (§4.5).
// progress_scope 에서 행정·지원을 뺀 것은 임시. 예정일 입력률 4주 연속 90% 이상이면 admin 을 다시 넣는다.
func applyMetricsSettings(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		INSERT OR IGNORE INTO app_settings(setting_key, setting_value, updated_at)
		VALUES
		  ('metrics_base_date', '2026-08-03', datetime('now','localtime')),
		  ('progress_scope', 'as,maintenance', datetime('now','localtime'))`); err != nil {
		log.Printf("019 metrics settings: %v", err)
	}
}

func (r *StatsRepo) MetricsPolicy() model.MetricsPolicy {
	var out model.MetricsPolicy
	if r == nil || r.db == nil {
		return out
	}
	s := NewSettingsRepo(r.db)
	base, _ := s.Get(SettingMetricsBaseDate)
	scope, _ := s.Get(SettingProgressScope)
	out.BaseDate = normalizeMetricsDate(base)
	out.ProgressScope = strings.TrimSpace(scope)
	return out
}

func normalizeMetricsDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		if t2, err2 := time.Parse("2006-01-02", s); err2 == nil {
			return t2.Format("2006-01-02")
		}
		return ""
	}
	return t.Format("2006-01-02")
}

func (r *StatsRepo) attachMetrics(f model.StatsMeetingFilter) model.StatsMeetingFilter {
	f = normalizeMeetingFilter(f)
	p := r.MetricsPolicy()
	f.MetricsBaseDate = p.BaseDate
	f.ProgressScope = p.ProgressScope
	return f
}

func (r *StatsRepo) filterAS(f model.StatsMeetingFilter) (string, []interface{}) {
	return asFilterSQL(r.attachMetrics(f))
}

func (r *StatsRepo) filterMnt(f model.StatsMeetingFilter) (string, []interface{}) {
	return mntFilterSQL(r.attachMetrics(f))
}

func (r *StatsRepo) filterAdmin(f model.StatsMeetingFilter) (string, []interface{}) {
	return adminFilterSQL(r.attachMetrics(f))
}

func metricsBaseFromDB(db *sql.DB) string {
	if db == nil {
		return ""
	}
	var v string
	err := db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, SettingMetricsBaseDate).Scan(&v)
	if err == sql.ErrNoRows || err != nil {
		return ""
	}
	return normalizeMetricsDate(v)
}

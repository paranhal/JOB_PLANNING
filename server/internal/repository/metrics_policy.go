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

const metricsBasesV59MetaKey = "metrics_bases_v59"

func applyMetricsBasesV59(db *sql.DB) {
	if db == nil || metaDone(db, metricsBasesV59MetaKey) {
		return
	}
	if _, err := db.Exec(`
		INSERT OR IGNORE INTO app_settings(setting_key, setting_value, updated_at)
		SELECT k, (SELECT setting_value FROM app_settings WHERE setting_key='metrics_base_date'),
		       datetime('now','localtime')
		  FROM (SELECT 'metrics_base_receipt' AS k
		        UNION ALL SELECT 'metrics_base_visit'
		        UNION ALL SELECT 'metrics_base_complete')`); err != nil {
		log.Printf("metrics_bases_v59: %v", err)
		return
	}
	markMetaDone(db, metricsBasesV59MetaKey)
}

func (r *StatsRepo) MetricsPolicy() model.MetricsPolicy {
	var out model.MetricsPolicy
	if r == nil || r.db == nil {
		return out
	}
	s := NewSettingsRepo(r.db)
	legacy, _ := s.Get(SettingMetricsBaseDate)
	legacy = normalizeMetricsDate(legacy)
	out.BaseDate = legacy
	out.BaseReceipt = metricsOrLegacy(s, SettingMetricsBaseReceipt, legacy)
	out.BaseVisit = metricsOrLegacy(s, SettingMetricsBaseVisit, legacy)
	out.BaseComplete = metricsOrLegacy(s, SettingMetricsBaseComplete, legacy)
	return out
}

func metricsOrLegacy(s *SettingsRepo, key, legacy string) string {
	if s == nil {
		return legacy
	}
	v, _ := s.Get(key)
	v = normalizeMetricsDate(v)
	if v != "" {
		return v
	}
	return legacy
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
	if strings.TrimSpace(f.OrgID) == "" {
		f.OrgID = OrgAll
	}
	p := r.MetricsPolicy()
	f.MetricsBaseDate = p.BaseDate
	f.MetricsBaseReceipt = p.BaseReceipt
	f.MetricsBaseVisit = p.BaseVisit
	f.MetricsBaseComplete = p.BaseComplete
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

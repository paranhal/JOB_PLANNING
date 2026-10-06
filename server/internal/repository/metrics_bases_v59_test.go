package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestMetricsBasesV59SeedsFourKeys(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT setting_key FROM app_settings WHERE setting_key LIKE 'metrics_base%' ORDER BY setting_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	want := []string{"metrics_base_complete", "metrics_base_date", "metrics_base_receipt", "metrics_base_visit"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("keys=%v want=%v", keys, want)
	}
	legacy, _ := NewSettingsRepo(db).Get(SettingMetricsBaseDate)
	if legacy != "2026-08-03" {
		t.Fatalf("옛 키=%q", legacy)
	}
}

func TestASFilterCompleteUsesCompleteNotReceipt(t *testing.T) {
	f := model.StatsMeetingFilter{
		MetricsBaseReceipt:  "2026-08-03",
		MetricsBaseComplete: "2026-08-03",
		DateBasis:           model.BasisComplete,
		OrgID:               OrgAll,
	}
	sql, args := asFilterSQL(f)
	if strings.Contains(sql, "receipt_datetime >= ?") {
		t.Fatalf("완료 집계에 접수일을 댔다: %s", sql)
	}
	if !strings.Contains(sql, "complete_datetime >= ?") {
		t.Fatalf("완료일을 안 댔다: %s args=%v", sql, args)
	}
}

func TestMntFilterCompletePairsDoneDateWithCompleteBase(t *testing.T) {
	f := model.StatsMeetingFilter{
		MetricsBaseVisit:    "2026-08-03",
		MetricsBaseComplete: "2026-09-01",
		DateBasis:           model.BasisComplete,
		OrgID:               OrgAll,
	}
	sql, args := mntFilterSQL(f)
	if !strings.Contains(sql, "CASE WHEN TRIM(COALESCE(v.completed_date,'')) <> '' THEN ? ELSE ? END") {
		t.Fatalf("완료·방문 짝이 없다: %s", sql)
	}
	found := false
	for _, a := range args {
		if a == "2026-09-01" {
			found = true
		}
	}
	if !found {
		t.Fatalf("완료 기준일이 안 실렸다 args=%v", args)
	}
}

func TestASLeadTimeRequiresReceiptAndComplete(t *testing.T) {
	f := model.StatsMeetingFilter{
		MetricsBaseReceipt:  "2026-08-03",
		MetricsBaseVisit:    "2026-08-03",
		MetricsBaseComplete: "2026-08-03",
		DateBasis:           model.BasisReceipt,
		AlsoBases:           []model.DateBasis{model.BasisComplete},
		OrgID:               OrgAll,
	}
	sql, _ := asFilterSQL(f)
	if !strings.Contains(sql, "receipt_datetime >= ?") || !strings.Contains(sql, "complete_datetime >= ?") {
		t.Fatalf("리드타임이 접수·완료를 모두 안 댄다: %s", sql)
	}
}

func TestMetricsBasesV59DoesNotOverwriteChangedComplete(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "m2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSettingsRepo(db)
	if err := s.Set(SettingMetricsBaseComplete, ""); err != nil {
		t.Fatal(err)
	}
	applyMetricsBasesV59(db)
	got, _ := s.Get(SettingMetricsBaseComplete)
	if got != "" {
		t.Fatalf("비운 완료 기준일을 덮었다=%q", got)
	}
}

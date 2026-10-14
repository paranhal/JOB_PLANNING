package handler

import (
	"path/filepath"
	"testing"

	"customer-support/internal/repository"
)

func TestReplacementSummaryAgingBuckets(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "aging.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name) VALUES ('c1','도서관','도서관')`); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		id, date string
	}{
		{"a6", "2020-10-12"},  // ~6년 — 제외
		{"a8", "2018-10-12"},  // ~8년 — 7년대
		{"a11", "2015-10-12"}, // ~11년 — 10년+
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, install_date, operation_status)
			VALUES (?,?,?,?,?)`, r.id, "c1", r.id, r.date, "operating"); err != nil {
			t.Fatal(err)
		}
	}
	s := (&AnalysisHandler{db: db}).ReplacementSummary()
	if s.Aging7Count != 1 || s.Aging10Count != 1 || s.AgingCount != 2 {
		t.Fatalf("aging7=%d aging10=%d total=%d", s.Aging7Count, s.Aging10Count, s.AgingCount)
	}
}

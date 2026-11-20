package repository

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func leadRow(t *testing.T, r *StatsRepo, kind, org, start, metric string) *LeadSnapshotRow {
	t.Helper()
	to := start + "z"
	rows, err := r.ListLeadSnapshots(kind, org, start, to)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if rows[i].Start == start && rows[i].Metric == metric {
			return &rows[i]
		}
	}
	return nil
}

func TestLeadSnapshots95_217(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "lead217.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('c1','도서관','도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-07-01", "as,maintenance")
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_date, complete_datetime, status, assigned_to, data_origin, process_type, org_id)
		VALUES
		('R1','R1','c1','2026-07-31','2026-08-03','2026-08-06','completed','양기헌','app','visit','O01'),
		('R2','R2','c1','2026-08-05','2026-08-05','2026-08-05','completed','양기헌','app','visit','O01'),
		('R3','R3','c1','2026-08-13','2026-08-17','2026-08-18','completed','양기헌','app','visit','O01'),
		('R4','R4','c1','2026-08-10','','2026-08-10','completed','양기헌','app','remote','O01')`); err != nil {
		t.Fatal(err)
	}
	r := NewStatsRepo(db)
	now := time.Date(2026, 9, 30, 9, 0, 0, 0, model.KST())
	c, _, err := r.RunLeadSnapshots(now)
	if err != nil || c == 0 {
		t.Fatalf("첫 실행 computed=%d err=%v", c, err)
	}

	// 월 단위 8월(전체): 기존 산식과 같은 값, 표본 수 ①②=3 ③=4
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	a1, a2, n12, _ := r.avgASVisitSpans("2026-08-01", "2026-09-01", f)
	a3, n3, _ := r.avgASCompleteLeadTime("2026-08-01", "2026-09-01", f)
	for _, tc := range []struct {
		m    string
		want float64
		n    int
	}{{LeadMetricReceiptToVisit, a1, n12}, {LeadMetricVisitToDone, a2, n12}, {LeadMetricReceiptToDone, a3, n3}} {
		row := leadRow(t, r, LeadKindMonth, LeadOrgAll, "2026-08-01", tc.m)
		if row == nil || row.AvgDays == nil {
			t.Fatalf("%s 월 값이 없다: %+v", tc.m, row)
		}
		if row.Sample != tc.n || math.Abs(*row.AvgDays-tc.want) > 0.01 {
			t.Errorf("%s 월 avg=%v n=%d want %.2f/%d", tc.m, *row.AvgDays, row.Sample, tc.want, tc.n)
		}
		if row.FinalAt == "" {
			t.Errorf("%s 8월은 끝난 지 7일이 지나 확정이어야 한다", tc.m)
		}
	}
	if n12 != 3 || n3 != 4 {
		t.Fatalf("시험 데이터 표본 n12=%d n3=%d", n12, n3)
	}
	// 일 단위(8/6) 표본 1, 주 단위(8/3 주)도 저장
	if row := leadRow(t, r, LeadKindDay, LeadOrgAll, "2026-08-06", LeadMetricReceiptToDone); row == nil || row.Sample != 1 || row.AvgDays == nil {
		t.Errorf("일 단위 8/6: %+v", row)
	}
	if row := leadRow(t, r, LeadKindWeek, LeadOrgAll, "2026-08-03", LeadMetricReceiptToDone); row == nil || row.Sample == 0 {
		t.Errorf("주 단위 8/3: %+v", row)
	}
	// 조직별 행(도서관 O01)
	if row := leadRow(t, r, LeadKindMonth, "O01", "2026-08-01", LeadMetricReceiptToDone); row == nil || row.Sample != n3 {
		t.Errorf("조직별 O01 월: %+v", row)
	}
	// 표본 0 이면 NULL(0 이 아니다)
	if row := leadRow(t, r, LeadKindDay, LeadOrgAll, "2026-08-08", LeadMetricReceiptToVisit); row == nil || row.AvgDays != nil || row.Sample != 0 {
		t.Errorf("표본 0 일 단위는 avg NULL 이어야 한다: %+v", row)
	}

	// 확정된 기간은 데이터가 바뀌어도 덮어쓰지 않는다. 오늘이 든 기간은 다시 계산한다.
	if _, err := db.Exec(`UPDATE as_receipts SET visit_date='2026-08-05' WHERE as_id='R3'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_date, complete_datetime, status, assigned_to, data_origin, process_type, org_id)
		VALUES ('R9','R9','c1','2026-09-28','2026-09-29','2026-09-30','completed','양기헌','app','visit','O01')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.RunLeadSnapshots(now); err != nil {
		t.Fatal(err)
	}
	if row := leadRow(t, r, LeadKindMonth, LeadOrgAll, "2026-08-01", LeadMetricReceiptToVisit); row == nil || row.AvgDays == nil || math.Abs(*row.AvgDays-a1) > 0.01 {
		t.Errorf("확정된 8월 값이 바뀌었다: %+v", row)
	}
	if row := leadRow(t, r, LeadKindDay, LeadOrgAll, "2026-09-30", LeadMetricReceiptToDone); row == nil || row.Sample != 1 || row.FinalAt != "" {
		t.Errorf("오늘 값은 다시 계산되고 미확정이어야 한다: %+v", row)
	}
	// 같은 날 다시 실행해도 행이 늘지 않는다(멱등)
	var before, after int
	_ = db.QueryRow(`SELECT COUNT(*) FROM lead_time_snapshots`).Scan(&before)
	if _, _, err := r.RunLeadSnapshots(now); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM lead_time_snapshots`).Scan(&after)
	if before != after {
		t.Errorf("재실행으로 행이 늘었다 %d → %d", before, after)
	}
}

// 지표 기준일 이전 구간은 legacy 로 표시하고 이관 데이터를 포함해 참고용으로 저장한다.
func TestLeadSnapshotsLegacyBeforeBase95_217(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "lead217b.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('c1','도서관','도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-09-13", "as,maintenance")
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_date, complete_datetime, status, assigned_to, data_origin, process_type, org_id)
		VALUES ('L1','L1','c1','2026-07-20','2026-07-21','2026-07-22','completed','양기헌','import','visit','O01')`); err != nil {
		t.Fatal(err)
	}
	r := NewStatsRepo(db)
	if _, _, err := r.RunLeadSnapshots(time.Date(2026, 9, 30, 9, 0, 0, 0, model.KST())); err != nil {
		t.Fatal(err)
	}
	row := leadRow(t, r, LeadKindMonth, LeadOrgAll, "2026-07-01", LeadMetricReceiptToDone)
	if row == nil || row.Quality != LeadQualityLegacy || row.Sample != 1 || row.AvgDays == nil {
		t.Fatalf("7월은 legacy 로 이관 건을 포함해 저장돼야 한다: %+v", row)
	}
	if row := leadRow(t, r, LeadKindMonth, LeadOrgAll, "2026-09-01", LeadMetricReceiptToDone); row == nil || row.Quality != LeadQualityBase {
		t.Errorf("기준일이 든 9월은 base: %+v", row)
	}
}

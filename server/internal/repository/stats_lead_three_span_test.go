package repository

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestLeadTimeThreeSpansBusinessDays(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "lead3.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-07-01", "as,maintenance")
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_date, complete_datetime,
			status, assigned_to, data_origin, process_type
		) VALUES
		('R1','R1','c1','2026-07-31','2026-08-03','2026-08-06','completed','양기헌','app','visit'),
		('R2','R2','c1','2026-08-05','2026-08-05','2026-08-05','completed','양기헌','app','visit'),
		('R3','R3','c1','2026-08-13','2026-08-17','2026-08-18','completed','양기헌','app','visit'),
		('R4','R4','c1','2026-08-10','','2026-08-10','completed','양기헌','app','remote'),
		('R5','R5','c1','2026-08-20','2026-08-26','2026-08-24','completed','양기헌','app','visit')
	`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	from, toEx := "2026-07-01", "2026-09-01"
	a1, a2, n12, err := repo.avgASVisitSpans(from, toEx, f)
	if err != nil {
		t.Fatal(err)
	}
	a3, n3, err := repo.avgASCompleteLeadTime(from, toEx, f)
	if err != nil {
		t.Fatal(err)
	}
	neg, err := repo.countNegativeLeadSpans(from, toEx, f)
	if err != nil {
		t.Fatal(err)
	}
	if n12 != 3 {
		t.Fatalf("①② 표본=%d want 3 (R4·R5 제외)", n12)
	}
	if n3 != 5 {
		t.Fatalf("③ 표본=%d want 5", n3)
	}
	if n3 <= n12 {
		t.Fatalf("③ 표본(%d)이 ①②(%d)보다 커야 한다", n3, n12)
	}
	if neg != 1 {
		t.Fatalf("NegativeSpanN=%d want 1", neg)
	}
	if math.Abs(a1-2.0/3.0) > 0.05 {
		t.Fatalf("① 평균=%.3f want 0.67 (R1=1 R2=0 R3=1)", a1)
	}
	if math.Abs(a2-1.0) > 0.05 {
		t.Fatalf("② 평균=%.3f want 1.0", a2)
	}
	if math.Abs((a1+a2)-a3) < 0.01 {
		t.Fatalf("①+② 가 ③과 같으면 안 된다: %.3f vs %.3f", a1+a2, a3)
	}
	if a1 < 0 || a2 < 0 || a3 < 0 {
		t.Fatalf("음수 평균 ①=%.2f ②=%.2f ③=%.2f", a1, a2, a3)
	}
	if got := bdMinusNext(t, db, "2026-08-03", "2026-07-31"); got != 1 {
		t.Fatalf("R1 ①=%d want 1", got)
	}
	if got := bdMinusNext(t, db, "2026-08-17", "2026-08-13"); got != 1 {
		t.Fatalf("R3 ①=%d want 1 (음수 아님)", got)
	}
}

func bdMinusNext(t *testing.T, db *sql.DB, end, start string) int {
	t.Helper()
	_, nextStart, _, err := lookupBusinessDay(db, start)
	if err != nil {
		t.Fatal(err)
	}
	bdEnd, _, _, err := lookupBusinessDay(db, end)
	if err != nil {
		t.Fatal(err)
	}
	return bdEnd - nextStart
}

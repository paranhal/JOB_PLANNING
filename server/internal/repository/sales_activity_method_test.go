package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestSalesActivitySplit91Migration(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "a91.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO sales_projects (sales_id, name, stage, status) VALUES ('S-91','이관','discover','active')`); err != nil {
		t.Fatal(err)
	}
	rows := []struct{ id, typ string }{
		{"SA-v", "visit"}, {"SA-c", "call"}, {"SA-m", "mail"}, {"SA-o", "online"}, {"SA-i", "internal"},
		{"SA-r", "research"}, {"SA-q", "quote"}, {"SA-p", "proposal"}, {"SA-rf", "rfp"}, {"SA-b", "bid"},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO sales_activities (activity_id, sales_id, activity_date, activity_type, title) VALUES (?,?, '2026-10-01', ?, ?)`,
			r.id, "S-91", r.typ, r.typ); err != nil {
			t.Fatal(err)
		}
	}
	var before int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_activities`).Scan(&before)
	applySalesActivitySplit91(db)
	applySalesActivitySplit91(db)
	var after int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sales_activities`).Scan(&after)
	if before != after {
		t.Fatalf("건수 %d → %d", before, after)
	}
	assertTypeMethod := func(id, typ, method string) {
		t.Helper()
		var gotT, gotM string
		if err := db.QueryRow(`SELECT activity_type, activity_method FROM sales_activities WHERE activity_id=?`, id).Scan(&gotT, &gotM); err != nil {
			t.Fatal(err)
		}
		if gotT != typ || gotM != method {
			t.Fatalf("%s type=%s method=%s want %s/%s", id, gotT, gotM, typ, method)
		}
	}
	assertTypeMethod("SA-v", "other", "visit")
	assertTypeMethod("SA-c", "other", "call")
	assertTypeMethod("SA-m", "other", "mail")
	assertTypeMethod("SA-o", "other", "online")
	assertTypeMethod("SA-i", "other", "internal")
	assertTypeMethod("SA-r", "research", "")
	assertTypeMethod("SA-q", "quote", "")
	assertTypeMethod("SA-p", "proposal", "")
	assertTypeMethod("SA-rf", "proposal", "")
	assertTypeMethod("SA-b", "other", "")
	var nType, nMethod, nInactive int
	_ = db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_activity_type' AND is_active=1`).Scan(&nType)
	_ = db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_activity_method' AND is_active=1`).Scan(&nMethod)
	_ = db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_activity_type' AND is_active=0 AND code_value IN ('visit','call','mail','online','internal','rfp','bid')`).Scan(&nInactive)
	if nType != 7 || nMethod != 6 || nInactive != 7 {
		t.Fatalf("codes type=%d method=%d inactive=%d", nType, nMethod, nInactive)
	}
	var name string
	_ = db.QueryRow(`SELECT code_name FROM codes WHERE code_group='sales_activity_type' AND code_value='proposal'`).Scan(&name)
	if name != "제안서 제출" {
		t.Fatalf("proposal name %q", name)
	}
	up, down, err := salesActivitySplit91SQLFiles()
	if err != nil || !strings.Contains(up, "activity_method") || !strings.Contains(down, "sales_activities_pre91") {
		t.Fatalf("sql files %v", err)
	}
	_, total, err := CountSalesActivitiesByType(db)
	if err != nil || total != after {
		t.Fatalf("preview %d %v", total, err)
	}
}

func TestSalesActivitySplit91CreateRejectsLegacyType(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a91c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)
	p := &model.SalesProject{Name: "검증91"}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	err = repo.CreateActivity(&model.SalesActivity{SalesID: p.SalesID, ActivityDate: "2026-10-01", ActivityType: "visit", Title: "옛유형"}, "테스터", nil)
	if err == nil || !strings.Contains(err.Error(), "예전") {
		t.Fatalf("옛 유형 허용: %v", err)
	}
	err = repo.CreateActivity(&model.SalesActivity{SalesID: p.SalesID, ActivityDate: "2026-10-01", ActivityType: "research", Title: "방법없음"}, "테스터", nil)
	if err == nil || !strings.Contains(err.Error(), "방법") {
		t.Fatalf("방법 생략 허용: %v", err)
	}
	if err := repo.CreateActivity(&model.SalesActivity{SalesID: p.SalesID, ActivityDate: "2026-10-01", ActivityType: "research", ActivityMethod: "visit", Title: "정보수집 방문"}, "테스터", nil); err != nil {
		t.Fatal(err)
	}
}

func TestSalesActivityFilterTypeAndMethodAND(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a91f.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)
	p := &model.SalesProject{Name: "필터91"}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	must := func(typ, method, title string) {
		if err := repo.CreateActivity(&model.SalesActivity{SalesID: p.SalesID, ActivityDate: "2026-10-02", ActivityType: typ, ActivityMethod: method, Title: title}, "테스터", nil); err != nil {
			t.Fatal(err)
		}
	}
	must("research", "visit", "조사방문")
	must("research", "call", "조사전화")
	must("quote", "mail", "견적메일")
	got, err := repo.ListActivitiesFilter(SalesActivityFilter{Types: []string{"research"}, Methods: []string{"visit"}})
	if err != nil || len(got) != 1 || got[0].Title != "조사방문" {
		t.Fatalf("AND %+v %v", got, err)
	}
}

package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestListMonthBannersPlacesQuarterOnFirstMonth(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "banner.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	p := &model.SalesProject{
		Name: "부여군도서관 제안", IsTentativeName: true,
		ExpectedYM: "2026-09", ExpectedPrecision: model.SalesPrecisionMonth,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	q := &model.SalesProject{
		Name: "분기사업", IsTentativeName: true,
		ExpectedYM: "2026-11", ExpectedPrecision: model.SalesPrecisionQuarter,
	}
	if err := repo.Create(q); err != nil {
		t.Fatal(err)
	}

	sep, err := repo.ListMonthBanners(2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(sep) != 1 || sep[0].SalesID != p.SalesID {
		t.Fatalf("9월: %+v", sep)
	}
	oct, err := repo.ListMonthBanners(2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(oct) != 1 || oct[0].SalesID != q.SalesID || !strings.Contains(oct[0].Title, "4분기 중") {
		t.Fatalf("10월: %+v", oct)
	}
	nov, err := repo.ListMonthBanners(2026, 11)
	if err != nil {
		t.Fatal(err)
	}
	if len(nov) != 0 {
		t.Fatalf("11월에 띠가 있다: %+v", nov)
	}

	p2 := &model.SalesProject{Name: "다음활동사업", IsTentativeName: true}
	if err := repo.Create(p2); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateActivity(&model.SalesActivity{
		SalesID: p2.SalesID, ActivityDate: "2026-08-03", ActivityType: "visit",
		Title: "방문", NextAction: "제안서 초안", NextActionDate: "2026-09",
	}, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	sep2, err := repo.ListMonthBanners(2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	foundNext := false
	for _, b := range sep2 {
		if b.SalesID == p2.SalesID && strings.Contains(b.Title, "제안서 초안") {
			foundNext = true
		}
		if b.Href != "/sales/"+b.SalesID {
			t.Fatalf("href=%s", b.Href)
		}
	}
	if !foundNext {
		t.Fatalf("next_action 월 띠 없음: %+v", sep2)
	}
}

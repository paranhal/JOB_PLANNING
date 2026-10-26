package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func Test95_59_MonthTargetPanelRegionQueryCheonan(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "t59-panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido, addr_sigungu)
		VALUES
		('cn1','성환도서관','성환도서관',1,'O01','충청남도','천안시 동남구'),
		('cn2','쌍용도서관','쌍용도서관',1,'O01','충청남도','천안시 서북구'),
		('h1','홍성도서관','홍성도서관',1,'O01','충청남도','홍성군')`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	for _, id := range []string{"cn1", "cn2", "h1"} {
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: id, ShortName: id, Region: "충남", InspectionCycle: "semi", OrgID: model.OrgIDLibrary,
		}); err != nil {
			t.Fatal(err)
		}
	}
	p, err := repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	_, all, err := repo.MonthTargetPanelFilter(p, 2, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("region 없이 %d", len(all))
	}
	_, cheonan, err := repo.MonthTargetPanelFilter(p, 2, "", "", "", "", "충남|천안시 동남구")
	if err != nil {
		t.Fatal(err)
	}
	if len(cheonan) != 1 || cheonan[0].CustomerID != "cn1" {
		t.Fatalf("천안시 동남구 필터 %+v", cheonan)
	}
	_, both, err := repo.MonthTargetPanelFilter(p, 2, "", "", "", "", "충남|천안시 동남구", "충남|천안시 서북구")
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 2 {
		t.Fatalf("천안 두 구 %d", len(both))
	}
	for _, s := range both {
		if s.CustomerID == "h1" {
			t.Fatal("홍성이 천안 필터에 포함")
		}
	}
	_, sido, err := repo.MonthTargetPanelFilter(p, 2, "", "", "", "", "충남")
	if err != nil {
		t.Fatal(err)
	}
	if len(sido) != 3 {
		t.Fatalf("충남 시·도 키 %d", len(sido))
	}
}

package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func Test95_49_MonthTargetPanelChildrenAndFilter(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "t49-panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido, addr_sigungu)
		VALUES
		('h1','홍성A','홍성A',1,'O01','충청남도','홍성군'),
		('h2','홍성B','홍성B',1,'O01','충청남도','홍성군'),
		('y1','예산A','예산A',1,'O01','충청남도','예산군'),
		('g1','공주A','공주A',1,'O01','충청남도','공주시'),
		('u1','충남미지','충남미지',1,'O01','충청남도',''),
		('sj1','세종A','세종A',1,'O01','세종특별자치시','')`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	for _, id := range []string{"h1", "h2", "y1", "g1", "u1"} {
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: id, ShortName: id, Region: "충남", InspectionCycle: "semi", OrgID: model.OrgIDLibrary,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "sj1", ShortName: "세종A", Region: "세종", InspectionCycle: "semi", OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRegionDistanceOrder([]model.RegionDistanceOrder{
		{Sido: "충청남도", Sigungu: "공주시", SortOrder: 0},
		{Sido: "충청남도", Sigungu: "홍성군", SortOrder: 1},
		{Sido: "충청남도", Sigungu: "예산군", SortOrder: 2},
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	stats, sites, err := repo.MonthTargetPanel(p, 2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var chungStat MonthTargetRegionStat
	var sawSejong bool
	for _, s := range stats {
		if s.Key == "충남" {
			chungStat = s
		}
		if s.Key == "세종" {
			sawSejong = true
			if len(s.Children) != 0 {
				t.Fatalf("세종 자식 %d", len(s.Children))
			}
		}
	}
	if !sawSejong {
		t.Fatal("세종 없음")
	}
	if chungStat.Total != 5 {
		t.Fatalf("충남 total=%d", chungStat.Total)
	}
	var ct, cr, co int
	if len(chungStat.Children) < 3 {
		t.Fatalf("충남 자식 %+v", chungStat.Children)
	}
	if chungStat.Children[0].Label != "공주시" {
		t.Fatalf("거리 정렬 첫 항목 %q want 공주시", chungStat.Children[0].Label)
	}
	for _, ch := range chungStat.Children {
		ct += ch.Total
		cr += ch.Registered
		co += ch.Open
	}
	if ct != chungStat.Total || cr != chungStat.Registered || co != chungStat.Open {
		t.Fatalf("시·도 합계 %d/%d/%d 자식 %d/%d/%d", chungStat.Total, chungStat.Registered, chungStat.Open, ct, cr, co)
	}
	if len(sites) < 6 {
		t.Fatalf("사이트 %d", len(sites))
	}
	_, onlyHongByGu, err := repo.MonthTargetPanelFilter(p, 2, "", "", "", "홍성군")
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyHongByGu) != 2 {
		t.Fatalf("시·군·구만 필터 %d", len(onlyHongByGu))
	}
	_, onlyHong, err := repo.MonthTargetPanelFilter(p, 2, "", "", "충남", "홍성군")
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyHong) != 2 {
		t.Fatalf("홍성 필터 %d", len(onlyHong))
	}
	for _, s := range onlyHong {
		if s.Sigungu != "홍성군" || s.RegionLabel != "충남 홍성군" {
			t.Fatalf("필터 행 %+v", s)
		}
	}
	stats2, onlyHong2, err := repo.MonthTargetPanelFilter(p, 2, "홍성", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyHong2) != 2 {
		t.Fatalf("검색 홍성 %d", len(onlyHong2))
	}
	var stillChung bool
	for _, s := range stats2 {
		if s.Key == "충남" && s.Total == 5 {
			stillChung = true
		}
	}
	if !stillChung {
		t.Fatal("검색이 통계를 줄임")
	}
	_, unsetSites, err := repo.MonthTargetPanelFilter(p, 2, "", "", "충남", model.RegionSigunguUnsetQuery)
	if err != nil {
		t.Fatal(err)
	}
	if len(unsetSites) != 1 || unsetSites[0].Sigungu != "" {
		t.Fatalf("시·군·구 미지정 필터 %+v", unsetSites)
	}
}

func Test95_49_RegisterBySigunguKey(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "t49-reg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido, addr_sigungu)
		VALUES
		('h1','홍성','홍성',1,'O01','충청남도','홍성군'),
		('y1','예산','예산',1,'O01','충청남도','예산군'),
		('u1','미지','미지',1,'O01','충청남도',''),
		('m1','월간','월간',1,'O01','충청남도','홍성군'),
		('med','미디어','미디어',1,'O02','충청남도','홍성군')`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	for _, id := range []string{"h1", "y1", "u1"} {
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: id, ShortName: id, Region: "충남", InspectionCycle: "semi", OrgID: model.OrgIDLibrary,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "m1", ShortName: "월간", Region: "충남", InspectionCycle: "monthly", OrgID: model.OrgIDLibrary, HasKlas: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "med", ShortName: "미디어", Region: "충남", InspectionCycle: "semi", OrgID: "O02",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	hong, err := repo.RegisterMonthTargets(p, 2, model.MonthTargetSourceRegion, "충남|홍성군", nil, "u", "테스터")
	if err != nil || hong.Added != 1 {
		t.Fatalf("홍성 added=%d skip=%v err=%v", hong.Added, hong.Skipped, err)
	}
	sido, err := repo.RegisterMonthTargets(p, 8, model.MonthTargetSourceRegion, "충남", nil, "u", "테스터")
	if err != nil || sido.Added < 2 {
		t.Fatalf("시·도 하반기 added=%d skip=%v err=%v", sido.Added, sido.Skipped, err)
	}
	dupHong, err := repo.RegisterMonthTargets(p, 2, model.MonthTargetSourceRegion, "충남|홍성군", nil, "u", "테스터")
	if err != nil || dupHong.Added != 0 {
		t.Fatalf("홍성 중복 added=%d err=%v", dupHong.Added, err)
	}
	bothKeys := model.CollapseMonthTargetRegionKeys([]string{"충남", "충남|홍성군"})
	if len(bothKeys) != 1 || bothKeys[0] != "충남" {
		t.Fatalf("동시 선택 %+v", bothKeys)
	}
	unset, err := repo.RegisterMonthTargets(p, 2, model.MonthTargetSourceRegion, "충남|", nil, "u", "테스터")
	if err != nil || unset.Added != 1 {
		t.Fatalf("미지정 added=%d skip=%v err=%v", unset.Added, unset.Skipped, err)
	}
	for _, sk := range unset.Skipped {
		if sk.CustomerID == "med" {
			t.Fatalf("타 조직 포함 %+v", sk)
		}
	}
	tgs, err := repo.ListActiveMonthTargets(p.PlanID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range tgs {
		if tg.CustomerID == "med" {
			t.Fatal("타 조직 target")
		}
		if tg.CustomerID == "m1" {
			t.Fatal("월 주기 target")
		}
	}
	badMonth, err := repo.RegisterMonthTargets(p, 13, model.MonthTargetSourceRegion, "충남", nil, "u", "테스터")
	if err == nil {
		t.Fatalf("기간 밖 월 허용 added=%d", badMonth.Added)
	}
}

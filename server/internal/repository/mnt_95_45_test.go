package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestMnt95_45MonthTargetRules(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "t45-rules.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('semi1','반기A','반기A',1,'O01'),('mon1','월간','월간',1,'O01'),('semi2','반기2회','반기2회',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "semi1", ShortName: "반기A", InspectionCycle: "semi", Region: "세종"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "mon1", ShortName: "월간", InspectionCycle: "monthly", HasKlas: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "semi2", ShortName: "반기2", InspectionCycle: "semi", RoundsPerPeriod: 2, Region: "세종"}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	res, err := repo.RegisterMonthTargets(p, 2, model.MonthTargetSourceSite, "", []string{"semi1"}, "u1", "테스터")
	if err != nil || res.Added < 1 {
		t.Fatalf("2월 등록 added=%d err=%v skip=%v", res.Added, err, res.Skipped)
	}
	res2, err := repo.RegisterMonthTargets(p, 8, model.MonthTargetSourceSite, "", []string{"semi1"}, "u1", "테스터")
	if err != nil || res2.Added < 1 {
		t.Fatalf("하반기 8월 재등록 added=%d err=%v skip=%v", res2.Added, err, res2.Skipped)
	}
	dup, err := repo.RegisterMonthTargets(p, 2, model.MonthTargetSourceSite, "", []string{"semi1"}, "u1", "테스터")
	if err != nil {
		t.Fatal(err)
	}
	if dup.Added != 0 {
		t.Fatal("같은 달 중복 등록")
	}
	mon, err := repo.RegisterMonthTargets(p, 3, model.MonthTargetSourceSite, "", []string{"mon1"}, "u1", "테스터")
	if err != nil {
		t.Fatal(err)
	}
	if mon.Added != 0 {
		t.Fatal("월 주기 target 허용")
	}
	a, err := repo.RegisterMonthTargets(p, 3, model.MonthTargetSourceSite, "", []string{"semi2"}, "u1", "테스터")
	if err != nil || a.Added < 1 {
		t.Fatalf("2회 1번째 %v %v", a, err)
	}
	b, err := repo.RegisterMonthTargets(p, 4, model.MonthTargetSourceSite, "", []string{"semi2"}, "u1", "테스터")
	if err != nil || b.Added < 1 {
		t.Fatalf("2회 2번째 %v %v", b, err)
	}
	c, err := repo.RegisterMonthTargets(p, 5, model.MonthTargetSourceSite, "", []string{"semi2"}, "u1", "테스터")
	if err != nil {
		t.Fatal(err)
	}
	if c.Added != 0 {
		t.Fatal("SiteRounds 초과")
	}
}

func TestMnt95_45OpenSlotsTargetVisitCarryUnconfirmed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "t45-slots.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('s1','반기','반기',1,'O01'),('m1','월간','월간',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "s1", ShortName: "반기", InspectionCycle: "semi"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "m1", ShortName: "월간", HasKlas: true, InspectionCycle: "monthly"}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	feb, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(feb, "s1"); n != 0 {
		t.Fatalf("target 전 2월 반기=%d", n)
	}
	if n := countSlotCustomer(feb, "m1"); n != 1 {
		t.Fatalf("월간 2월=%d", n)
	}
	if _, err := repo.RegisterMonthTargets(p, 2, "site", "", []string{"s1"}, "u", "테스터"); err != nil {
		t.Fatal(err)
	}
	invalidateOpenUnassignedSlotsCache()
	feb2, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(feb2, "s1"); n != 1 {
		t.Fatalf("2월 target 슬롯=%d", n)
	}
	var hit model.UnassignedMonthSlot
	for _, s := range feb2 {
		if s.CustomerID == "s1" {
			hit = s
		}
	}
	if hit.TargetMonth != 2 || hit.TargetCarry || hit.UnconfirmedQuarter {
		t.Fatalf("배지 %+v", hit)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{PlanID: p.PlanID, VisitDate: "2026-02-10", CustomerID: "s1"}); err != nil {
		t.Fatal(err)
	}
	invalidateOpenUnassignedSlotsCache()
	after, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(after, "s1"); n != 0 {
		t.Fatalf("방문 후 슬롯=%d", n)
	}

	p2, err := repo.CreatePlan(2027, "t2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RegisterMonthTargets(p2, 2, "site", "", []string{"s1"}, "u", "테스터"); err != nil {
		t.Fatal(err)
	}
	invalidateOpenUnassignedSlotsCache()
	mar, err := repo.ListOpenUnassignedSlots(p2.PlanID, 2027, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(mar, "s1"); n != 1 {
		t.Fatalf("이월 슬롯=%d", n)
	}
	for _, s := range mar {
		if s.CustomerID == "s1" && (!s.TargetCarry || s.TargetMonth != 2) {
			t.Fatalf("이월 배지 %+v", s)
		}
	}
	jun, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 6)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(jun, "s1"); n != 0 {
		t.Fatalf("방문 있는 상반기 6월 미확정=%d %v", n, jun)
	}
	dec, err := repo.ListOpenUnassignedSlots(p2.PlanID, 2027, 6)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(dec, "s1"); n != 1 {
		t.Fatalf("target 이월이 6월에도 보여야 함=%d", n)
	}
}

func TestMnt95_45SI300RegionAndQueryCap(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "t45-si.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O03','O03','SI사업팀','business',1,30),('O02','O02','미디어사업팀','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	ins, err := db.Prepare(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,?)`)
	if err != nil {
		t.Fatal(err)
	}
	regions := []string{"r0", "r1", "r2", "r3", "r4", "r5", "r6", ""}
	for i := 0; i < 300; i++ {
		id := fmt.Sprintf("si%03d", i)
		if _, err := ins.Exec(id, "학교"+id, "학교"+id, "O03"); err != nil {
			t.Fatal(err)
		}
		reg := regions[i%len(regions)]
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: id, ShortName: "학교" + id, Region: reg, InspectionCycle: "semi", OrgID: "O03",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('lib1','도서관','도서관',1,'O01'),('med1','미디어','미디어',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "lib1", ShortName: "도서관", InspectionCycle: "semi", Region: "r0", OrgID: model.OrgIDLibrary}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "med1", ShortName: "미디어", InspectionCycle: "semi", Region: "r0", OrgID: "O02"}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlanForOrg("O03", 2026, "SI")
	if err != nil {
		t.Fatal(err)
	}
	regA, err := repo.RegisterMonthTargets(p, 2, model.MonthTargetSourceRegion, "r0", nil, "u", "테스터")
	if err != nil {
		t.Fatal(err)
	}
	if regA.Added == 0 {
		t.Fatalf("지역 r0 등록 0 skip=%v", regA.Skipped)
	}
	for _, sk := range regA.Skipped {
		if sk.CustomerID == "lib1" || sk.CustomerID == "med1" {
			t.Fatalf("타 조직 시도: %+v", sk)
		}
	}
	stats, _, err := repo.MonthTargetPanel(p, 2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var unspec, r0 MonthTargetRegionStat
	for _, s := range stats {
		if s.Region == model.RegionUnspecified {
			unspec = s
		}
		if s.Region == "r0" {
			r0 = s
		}
	}
	if r0.Total == 0 || r0.Registered == 0 {
		t.Fatalf("r0 통계 %+v", r0)
	}
	if unspec.Total == 0 {
		t.Fatalf("지역 미지정 없음 %+v", stats)
	}
	var five []string
	for i := 0; i < 300 && len(five) < 5; i++ {
		if i%len(regions) == 1 {
			five = append(five, fmt.Sprintf("si%03d", i))
		}
	}
	ind, err := repo.RegisterMonthTargets(p, 3, model.MonthTargetSourceSite, "", five, "u", "테스터")
	if err != nil || ind.Added < 5 {
		t.Fatalf("개별 5곳 added=%d skip=%v err=%v", ind.Added, ind.Skipped, err)
	}
	invalidateOpenUnassignedSlotsCache()
	feb, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 2)
	if err != nil {
		t.Fatal(err)
	}
	mar, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	if countSlotCustomer(feb, "si000") == 0 && r0.Registered > 0 {
		// si000 is region r0 (0%8=0)
		t.Fatalf("2월 대기 목록 r0 없음 n=%d", len(feb))
	}
	if n := countSlotCustomer(mar, five[0]); n != 1 {
		t.Fatalf("3월 개별 슬롯=%d", n)
	}
	jul, err := repo.RegisterMonthTargets(p, 7, model.MonthTargetSourceRegion, "r0", nil, "u", "테스터")
	if err != nil || jul.Added == 0 {
		t.Fatalf("하반기 재등록 added=%d err=%v skip=%v", jul.Added, err, jul.Skipped)
	}
	tgs, err := repo.ListActiveMonthTargets(p.PlanID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	var cancelID string
	for _, tg := range tgs {
		if tg.CustomerID == five[0] && tg.Month == 3 {
			cancelID = tg.TargetID
			break
		}
	}
	if cancelID == "" {
		t.Fatal("3월 target id 없음")
	}
	if err := repo.CancelMonthTarget(cancelID); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{PlanID: p.PlanID, VisitDate: "2026-02-11", CustomerID: "si000"}); err != nil {
		t.Fatal(err)
	}
	var febID string
	tgs, _ = repo.ListActiveMonthTargets(p.PlanID, 2026)
	for _, tg := range tgs {
		if tg.CustomerID == "si000" && tg.Month == 2 {
			febID = tg.TargetID
		}
	}
	if febID == "" {
		t.Fatal("si000 2월 target 없음")
	}
	if err := repo.CancelMonthTarget(febID); err == nil {
		t.Fatal("방문 있는데 취소됨")
	}
	if err := repo.MoveMonthTarget(febID, 4); err == nil {
		t.Fatal("방문 있는데 이동됨")
	}
	dbPath := filepath.Join(dir, "t45-si.db")
	db.Close()
	var qn int64
	cdb, err := openQueryCountDB(dbPath, &qn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clearQueryCount()
		cdb.Close()
	})
	crepo := NewMaintenanceRepo(cdb)
	invalidateOpenUnassignedSlotsCache()
	qn = 0
	if _, err := crepo.ListOpenUnassignedSlots(p.PlanID, 2026, 2); err != nil {
		t.Fatal(err)
	}
	if qn > 30 {
		t.Errorf("쿼리 %d회 want ≤30", qn)
	}
}

func TestMnt95_45MigrationIdempotentAndOrg(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "t45-mig.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	applyMaintenanceMonthTargetsV89(db)
	applyMaintenanceMonthTargetsV89(db)
	repo := NewMaintenanceRepo(db)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('x1','X','X',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "x1", ShortName: "X", InspectionCycle: "yearly"}); err != nil {
		t.Fatal(err)
	}
	plan, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(plan.OrgID) == "" {
		t.Fatal("plan org_id 빈값")
	}
	res, err := repo.RegisterMonthTargets(plan, 4, "site", "", []string{"x1"}, "u", "테스터")
	if err != nil || res.Added < 1 {
		t.Fatalf("연 1회 4월 added=%d err=%v", res.Added, err)
	}
	tgs, _ := repo.ListActiveMonthTargets(plan.PlanID, 2026)
	if len(tgs) == 0 || strings.TrimSpace(tgs[0].OrgID) == "" {
		t.Fatalf("target org_id %+v", tgs)
	}
	if _, err := repo.EnsureYearPlan("", 2026); err == nil {
		t.Fatal("빈 org_id 계획 허용")
	}
}

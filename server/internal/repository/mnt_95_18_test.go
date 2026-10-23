package repository

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func slotSig(s model.UnassignedMonthSlot) string {
	return strings.Join([]string{
		s.CustomerID, s.SlotKey, s.ProductType, s.Cycle, s.LastAssignee, s.Region,
		s.RegionAssigneeName, fmt.Sprintf("%v", s.RegionPending), fmt.Sprintf("%v", s.UnconfirmedQuarter), s.OrgID,
	}, "|")
}

func sortSlots(in []model.UnassignedMonthSlot) []model.UnassignedMonthSlot {
	out := append([]model.UnassignedMonthSlot(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		a, b := slotSig(out[i]), slotSig(out[j])
		if a == b {
			return out[i].ShortName < out[j].ShortName
		}
		return a < b
	})
	return out
}

func assertSlotsEqual(t *testing.T, got, want []model.UnassignedMonthSlot) {
	t.Helper()
	g, w := sortSlots(got), sortSlots(want)
	if len(g) != len(w) {
		t.Fatalf("슬롯 수 got=%d want=%d", len(g), len(w))
	}
	for i := range g {
		if slotSig(g[i]) != slotSig(w[i]) {
			t.Fatalf("슬롯[%d]\ngot  %s\nwant %s", i, slotSig(g[i]), slotSig(w[i]))
		}
	}
}

func TestMnt95_18SameDateLaterRowWins(t *testing.T) {
	dst := map[string]string{}
	fillAssigneePicks([]visitAssigneeRow{
		{CustomerID: "c", ProductType: "KLAS", VisitDate: "2026-05-05", Assignee: "먼저"},
		{CustomerID: "c", ProductType: "klas", VisitDate: "2026-05-05", Assignee: "나중"},
	}, dst)
	if dst[assigneeIndexKey("c", "KLAS")] != "나중" {
		t.Fatalf("동일일 나중 행=%q", dst[assigneeIndexKey("c", "KLAS")])
	}
}

func TestMnt95_18LookupAssigneeIndexEqualsSlow(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "eq.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-empty','빈담당','빈담당',1,?),('c-tie','동일일','동일일',1,?),('c-prev','전년','전년',1,?),
		       ('c-year','올해우선','올해우선',1,?)`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	for _, id := range []string{"c-empty", "c-tie", "c-prev", "c-year"} {
		rounds := 1
		if id == "c-tie" {
			rounds = 2
		}
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: id, ShortName: id, HasKlas: true, InspectionCycle: "monthly",
			OrgID: model.OrgIDLibrary, RoundsPerPeriod: rounds,
		}); err != nil {
			t.Fatal(err)
		}
	}
	prev, err := repo.CreatePlan(2025, "prev")
	if err != nil {
		t.Fatal(err)
	}
	cur, err := repo.CreatePlan(2026, "cur")
	if err != nil {
		t.Fatal(err)
	}
	mustVisit := func(v model.MaintenanceVisit) {
		t.Helper()
		if err := repo.InsertVisitFull(v); err != nil {
			t.Fatal(err)
		}
	}
	mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-03-10", CustomerID: "c-empty", ProductType: "KLAS", Assignee: "", SortOrder: 1})
	mustVisit(model.MaintenanceVisit{PlanID: prev.PlanID, VisitDate: "2025-11-01", CustomerID: "c-empty", ProductType: "KLAS", Assignee: "전년담당", SortOrder: 1})
	mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-05-05", CustomerID: "c-tie", ProductType: "KLAS", Assignee: "먼저", SortOrder: 1, RoundNo: 1})
	mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-05-05", CustomerID: "c-tie", ProductType: "klas", Assignee: "나중", SortOrder: 2, RoundNo: 2})
	mustVisit(model.MaintenanceVisit{PlanID: prev.PlanID, VisitDate: "2025-08-01", CustomerID: "c-prev", ProductType: "KLAS", Assignee: "전년만", SortOrder: 1})
	mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-01-02", CustomerID: "c-year", ProductType: "KLAS", Assignee: "올해앞", SortOrder: 1})
	mustVisit(model.MaintenanceVisit{PlanID: cur.PlanID, VisitDate: "2026-09-09", CustomerID: "c-year", ProductType: "KLAS", Assignee: "올해뒤", SortOrder: 1})
	mustVisit(model.MaintenanceVisit{PlanID: prev.PlanID, VisitDate: "2025-12-31", CustomerID: "c-year", ProductType: "KLAS", Assignee: "전년무시", SortOrder: 1})

	lookup, err := repo.assigneeIndex(cur.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"c-empty", "c-tie", "c-prev", "c-year"} {
		got, want := lookup(id, "KLAS"), repo.lookupAssigneeSlow(cur.PlanID, id, "KLAS")
		if got != want {
			t.Fatalf("%s lookup=%q slow=%q", id, got, want)
		}
	}
	if lookup("c-empty", "KLAS") != "전년담당" {
		t.Fatalf("빈 담당자는 건너뛰고 전년=%q", lookup("c-empty", "KLAS"))
	}
	if lookup("c-tie", "KLAS") != "나중" {
		t.Fatalf("동일일 나중 행=%q", lookup("c-tie", "KLAS"))
	}
	if lookup("c-year", "KLAS") != "올해뒤" {
		t.Fatalf("올해 우선=%q", lookup("c-year", "KLAS"))
	}
}

func TestMnt95_18OpenSlotsFastEqualsSlow(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "open.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := NewUserRepo(db)
	_ = users.EnsureAdmin("x")
	siteU := &model.User{Username: "su", PasswordHash: "x", FullName: "사이트담당", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	regU := &model.User{Username: "ru", PasswordHash: "x", FullName: "지역담당", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{siteU, regU} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES
		('c-m','월간','월간',1,?),('c-q','분기','분기',1,?),('c-s','반기','반기',1,?),('c-y','연간','연간',1,?),
		('c-site','사이트예외','사이트예외',1,?),('c-reg','지역만','지역만',1,?)`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	cfgs := []model.MaintenanceSiteConfig{
		{CustomerID: "c-m", ShortName: "월간", HasKlas: true, InspectionCycle: "monthly", Region: "세종", OrgID: model.OrgIDLibrary},
		{CustomerID: "c-q", ShortName: "분기", HasKlas: true, InspectionCycle: "quarterly", Region: "세종", OrgID: model.OrgIDLibrary},
		{CustomerID: "c-s", ShortName: "반기", HasKlas: true, InspectionCycle: "semi", Region: "세종", OrgID: model.OrgIDLibrary},
		{CustomerID: "c-y", ShortName: "연간", HasKlas: true, InspectionCycle: "yearly", Region: "세종", OrgID: model.OrgIDLibrary},
		{CustomerID: "c-site", ShortName: "사이트예외", HasKlas: true, InspectionCycle: "monthly", Region: "세종", OrgID: model.OrgIDLibrary},
		{CustomerID: "c-reg", ShortName: "지역만", HasKlas: true, InspectionCycle: "monthly", Region: "세종", OrgID: model.OrgIDLibrary},
	}
	for i := range cfgs {
		if err := repo.UpsertSiteConfig(&cfgs[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.ReplaceRegionAssignees(model.OrgIDLibrary, "세종", []model.MaintenanceRegionAssignee{
		{UserID: regU.UserID, Role: model.WBMemberOwner},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceSiteAssignees("c-site", []model.MaintenanceSiteAssignee{
		{UserID: siteU.UserID, Role: model.WBMemberOwner},
	}); err != nil {
		t.Fatal(err)
	}
	cur, err := repo.CreatePlan(2026, "cur")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreatePlan(2025, "prev"); err != nil {
		t.Fatal(err)
	}
	invalidateOpenUnassignedSlotsCache()
	slow, err := repo.ListOpenUnassignedSlotsSlow(cur.PlanID, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	invalidateOpenUnassignedSlotsCache()
	fast, err := repo.ListOpenUnassignedSlots(cur.PlanID, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	assertSlotsEqual(t, fast, slow)
	octSlow, err := repo.ListOpenUnassignedSlotsSlow(cur.PlanID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	invalidateOpenUnassignedSlotsCache()
	octFast, err := repo.ListOpenUnassignedSlots(cur.PlanID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	assertSlotsEqual(t, octFast, octSlow)
}

func TestMnt95_18CacheInvalidateAndOrgFilter(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','A','A',1,?),('c2','B','B',1,?)`, model.OrgIDLibrary, "O02"); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c1", ShortName: "A", HasKlas: true, InspectionCycle: "monthly", OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c2", ShortName: "B", HasKlas: true, InspectionCycle: "monthly", OrgID: "O02",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	invalidateOpenUnassignedSlotsCache()
	all, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(FilterUnassignedSlotsByOrg(all, model.OrgIDLibrary)) == 0 {
		t.Fatal("O01 슬롯이 있어야 한다")
	}
	if len(FilterUnassignedSlotsByOrg(all, "O02")) == 0 {
		t.Fatal("O02 슬롯이 있어야 한다")
	}
	cached, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	assertSlotsEqual(t, cached, all)
	o1 := FilterUnassignedSlotsByOrg(cached, model.OrgIDLibrary)
	for _, s := range o1 {
		if s.OrgID == "O02" {
			t.Fatalf("조직 필터 후 O02가 남음: %+v", s)
		}
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-10-15", CustomerID: "c1", ProductType: "KLAS", Assignee: "배정됨",
	}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range after {
		if s.CustomerID == "c1" {
			t.Fatalf("배정 직후 캐시가 남음: %+v", s)
		}
	}
}

func seedMnt95_18Fixture(t *testing.T, dir string) (dbPath string, planID string) {
	t.Helper()
	dbPath = filepath.Join(dir, "fix.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _ = db.Exec(`PRAGMA synchronous=OFF`)
	_, _ = db.Exec(`PRAGMA journal_mode=MEMORY`)
	users := NewUserRepo(db)
	_ = users.EnsureAdmin("x")
	u1 := &model.User{Username: "t1", PasswordHash: "x", FullName: "담당1", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	u2 := &model.User{Username: "t2", PasswordHash: "x", FullName: "담당2", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{u1, u2} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewMaintenanceRepo(db)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	cst, err := tx.Prepare(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,?)`)
	if err != nil {
		t.Fatal(err)
	}
	regions := []string{"서울", "세종", "부산"}
	cycles := make([]string, 800)
	for i := 0; i < 400; i++ {
		cycles[i] = "semi"
	}
	for i := 400; i < 600; i++ {
		cycles[i] = "quarterly"
	}
	for i := 600; i < 700; i++ {
		cycles[i] = "yearly"
	}
	for i := 700; i < 800; i++ {
		cycles[i] = "monthly"
	}
	for i := 0; i < 800; i++ {
		id := fmt.Sprintf("s%03d", i)
		name := fmt.Sprintf("사이트%03d", i)
		if _, err := cst.Exec(id, name, name, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	cst.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 800; i++ {
		id := fmt.Sprintf("s%03d", i)
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: id, ShortName: id, HasKlas: true, InspectionCycle: cycles[i],
			Region: regions[i%len(regions)], OrgID: model.OrgIDLibrary,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.ReplaceRegionAssignees(model.OrgIDLibrary, "서울", []model.MaintenanceRegionAssignee{{UserID: u1.UserID, Role: model.WBMemberOwner}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRegionAssignees(model.OrgIDLibrary, "세종", []model.MaintenanceRegionAssignee{{UserID: u2.UserID, Role: model.WBMemberOwner}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRegionAssignees(model.OrgIDLibrary, "부산", []model.MaintenanceRegionAssignee{{UserID: u1.UserID, Role: model.WBMemberOwner}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("s%03d", i)
		uid := u1.UserID
		if i%2 == 1 {
			uid = u2.UserID
		}
		if err := repo.ReplaceSiteAssignees(id, []model.MaintenanceSiteAssignee{{UserID: uid, Role: model.WBMemberOwner}}); err != nil {
			t.Fatal(err)
		}
	}
	prev, err := repo.CreatePlan(2025, "prev")
	if err != nil {
		t.Fatal(err)
	}
	cur, err := repo.CreatePlan(2026, "cur")
	if err != nil {
		t.Fatal(err)
	}
	ins, err := db.Prepare(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, assignee, product_type, completed, round_no)
		VALUES (?,?,?,?,0,0,'normal',?, 'KLAS',0,1)`)
	if err != nil {
		t.Fatal(err)
	}
	defer ins.Close()
	assignees := []string{"담당1", "담당2", ""}
	n := 0
	for _, yp := range []struct {
		planID string
		year   int
	}{{prev.PlanID, 2025}, {cur.PlanID, 2026}} {
		for i := 0; i < 3000; i++ {
			cust := fmt.Sprintf("s%03d", i%800)
			mon := 1 + (i % 12)
			day := 1 + (i % 28)
			date := fmt.Sprintf("%d-%02d-%02d", yp.year, mon, day)
			a := assignees[i%len(assignees)]
			if _, err := ins.Exec(fmt.Sprintf("v%d-%d", yp.year, i), yp.planID, date, cust, a); err != nil {
				t.Fatal(err)
			}
			n++
		}
	}
	if n != 6000 {
		t.Fatalf("방문 %d", n)
	}
	return dbPath, cur.PlanID
}

func TestMnt95_18FixturePerfAndQueryCap(t *testing.T) {
	dir := t.TempDir()
	dbPath, planID := seedMnt95_18Fixture(t, dir)
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewMaintenanceRepo(db)
	invalidateOpenUnassignedSlotsCache()
	tSlow := time.Now()
	slow, err := repo.ListOpenUnassignedSlotsSlow(planID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	slowDur := time.Since(tSlow)
	invalidateOpenUnassignedSlotsCache()
	tFast := time.Now()
	fast, err := repo.ListOpenUnassignedSlots(planID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	fastDur := time.Since(tFast)
	assertSlotsEqual(t, fast, slow)
	t.Logf("95-18 ListOpenUnassignedSlots month=10 slots=%d slow=%s fast=%s", len(fast), slowDur, fastDur)
	invalidateOpenUnassignedSlotsCache()
	tSlow12 := time.Now()
	slow12, err := repo.ListOpenUnassignedSlotsSlow(planID, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	slow12Dur := time.Since(tSlow12)
	invalidateOpenUnassignedSlotsCache()
	tFast12 := time.Now()
	fast12, err := repo.ListOpenUnassignedSlots(planID, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	fast12Dur := time.Since(tFast12)
	assertSlotsEqual(t, fast12, slow12)
	t.Logf("95-18 ListOpenUnassignedSlots month=12 slots=%d slow=%s fast=%s", len(fast12), slow12Dur, fast12Dur)
	if fastDur > 300*time.Millisecond {
		t.Errorf("ListOpenUnassignedSlots fast=%s want <300ms", fastDur)
	}

	invalidateOpenUnassignedSlotsCache()
	tUnp := time.Now()
	_, _, err = NewWorkBoardRepo(db).ListUnplanned("", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	unpDur := time.Since(tUnp)
	t.Logf("95-18 listUnplanned dur=%s", unpDur)
	if unpDur > time.Second {
		t.Errorf("listUnplanned %s want <1s", unpDur)
	}

	invalidateOpenUnassignedSlotsCache()
	tDash := time.Now()
	if _, _, _, _, _, err := NewWorkBoardRepo(db).DashHomeOrg("", nil, OrgAll, 10, 8, 8, 8); err != nil {
		t.Fatal(err)
	}
	dashDur := time.Since(tDash)
	t.Logf("95-18 DashHomeOrg dur=%s", dashDur)
	if dashDur > 1500*time.Millisecond {
		t.Errorf("DashHomeOrg %s want <1.5s", dashDur)
	}

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
	if _, err := crepo.ListOpenUnassignedSlots(planID, 2026, 10); err != nil {
		t.Fatal(err)
	}
	queries := qn
	t.Logf("95-18 ListOpenUnassignedSlots queries=%d", queries)
	if queries > 30 {
		t.Errorf("쿼리 %d회 want ≤30", queries)
	}
	t.Logf("95-18 측정표 month10 slow_ms=%d fast_ms=%d slots=%d | month12 slow_ms=%d fast_ms=%d slots=%d | unplanned_ms=%d home_ms=%d queries=%d",
		slowDur.Milliseconds(), fastDur.Milliseconds(), len(fast),
		slow12Dur.Milliseconds(), fast12Dur.Milliseconds(), len(fast12),
		unpDur.Milliseconds(), dashDur.Milliseconds(), queries)
}

func seedOpenSlotsRepo(t *testing.T, name, planID string) *MaintenanceRepo {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-slot','슬롯사이트','슬롯사이트',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c-slot", ShortName: "슬롯", HasKlas: true, InspectionCycle: "monthly", OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status, org_id)
		VALUES (?, 2026, 't', 'draft', ?)`, planID, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestMnt95_18CacheIsolatesDatabases(t *testing.T) {
	const planID = "MP-CACHE"
	r1 := seedOpenSlotsRepo(t, "iso1.db", planID)
	r2 := seedOpenSlotsRepo(t, "iso2.db", planID)
	s1, err := r1.ListOpenUnassignedSlots(planID, 2026, 10)
	if err != nil || len(s1) == 0 {
		t.Fatalf("db1 슬롯 %d %v", len(s1), err)
	}
	if err := r2.InsertVisitFull(model.MaintenanceVisit{
		PlanID: planID, VisitDate: "2026-10-15", CustomerID: "c-slot", ProductType: "KLAS", Assignee: "배정",
	}); err != nil {
		t.Fatal(err)
	}
	again, err := r1.ListOpenUnassignedSlots(planID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) == 0 {
		t.Fatal("다른 DB 배정이 이 DB 캐시를 지웠다")
	}
	s2, err := r2.ListOpenUnassignedSlots(planID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range s2 {
		if s.CustomerID == "c-slot" {
			t.Fatalf("자기 DB 배정 후 캐시가 남음: %+v", s)
		}
	}
}

func TestMnt95_18CacheInvalidateOnVisitMutations(t *testing.T) {
	repo := seedOpenSlotsRepo(t, "mut.db", "MP-MUT")
	before, err := repo.ListOpenUnassignedSlots("MP-MUT", 2026, 10)
	if err != nil || len(before) == 0 {
		t.Fatalf("초기 슬롯 %d %v", len(before), err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: "MP-MUT", VisitDate: "2026-10-16", CustomerID: "c-slot", ProductType: "KLAS", Assignee: "배정",
	}); err != nil {
		t.Fatal(err)
	}
	afterAdd, err := repo.ListOpenUnassignedSlots("MP-MUT", 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range afterAdd {
		if s.CustomerID == "c-slot" {
			t.Fatal("추가 후 슬롯 캐시")
		}
	}
	visits, err := repo.ListVisits("MP-MUT")
	if err != nil || len(visits) != 1 {
		t.Fatalf("방문 %d %v", len(visits), err)
	}
	vid := visits[0].VisitID
	if err := repo.SetVisitDate(vid, "2026-11-02"); err != nil {
		t.Fatal(err)
	}
	oct, err := repo.ListOpenUnassignedSlots("MP-MUT", 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range oct {
		if s.CustomerID == "c-slot" {
			found = true
		}
	}
	if !found {
		t.Fatal("날짜 이동 후 10월 슬롯이 안 돌아옴")
	}
	nov, err := repo.ListOpenUnassignedSlots("MP-MUT", 2026, 11)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range nov {
		if s.CustomerID == "c-slot" {
			t.Fatal("11월 배정인데 슬롯 캐시")
		}
	}
	if err := repo.SetVisitCompleted(vid, true, "2026-11-02"); err != nil {
		t.Fatal(err)
	}
	novDone, err := repo.ListOpenUnassignedSlots("MP-MUT", 2026, 11)
	if err != nil {
		t.Fatal(err)
	}
	novDoneSlow, err := repo.ListOpenUnassignedSlotsSlow("MP-MUT", 2026, 11)
	if err != nil {
		t.Fatal(err)
	}
	assertSlotsEqual(t, novDone, novDoneSlow)
	for _, s := range novDone {
		if s.CustomerID == "c-slot" {
			t.Fatal("완료해도 방문일이 있으면 슬롯이 비면 안 된다")
		}
	}
	if err := repo.SetVisitCompleted(vid, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.AssignOpenVisit(vid, "2026-10-20", "재배정"); err != nil {
		t.Fatal(err)
	}
	octRe, err := repo.ListOpenUnassignedSlots("MP-MUT", 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	octReSlow, err := repo.ListOpenUnassignedSlotsSlow("MP-MUT", 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	assertSlotsEqual(t, octRe, octReSlow)
	for _, s := range octRe {
		if s.CustomerID == "c-slot" {
			t.Fatal("재배정 후 10월 슬롯 캐시")
		}
	}
	novAfterMove, err := repo.ListOpenUnassignedSlots("MP-MUT", 2026, 11)
	if err != nil {
		t.Fatal(err)
	}
	novAfterMoveSlow, err := repo.ListOpenUnassignedSlotsSlow("MP-MUT", 2026, 11)
	if err != nil {
		t.Fatal(err)
	}
	assertSlotsEqual(t, novAfterMove, novAfterMoveSlow)
	found = false
	for _, s := range novAfterMove {
		if s.CustomerID == "c-slot" {
			found = true
		}
	}
	if !found {
		t.Fatal("10월로 옮긴 뒤 11월 슬롯이 안 돌아옴")
	}
	if err := repo.SetVisitDate(vid, "2026-12-15"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteVisit(vid); err != nil {
		t.Fatal(err)
	}
	afterDel, err := repo.ListOpenUnassignedSlots("MP-MUT", 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, s := range afterDel {
		if s.CustomerID == "c-slot" {
			found = true
		}
	}
	if !found {
		t.Fatal("삭제 후 슬롯이 안 돌아옴")
	}
}

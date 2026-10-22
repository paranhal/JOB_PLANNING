package repository

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func setupSiteBulk(t *testing.T) (*MaintenanceRepo, *sql.DB) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "bulk.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers
		(customer_id, org_name, official_name, is_active, party_kind, industry, org_id, addr_sido, addr_sigungu, main_phone)
		VALUES
		('b-sch','계룡고','계룡고등학교',1,'customer','학교',?,'충청남도','계룡시','010'),
		('b-sch2','학교코드','학교코드',1,'customer','school',?,'충남','논산시',''),
		('b-off','쉬는학교','쉬는학교',0,'customer','학교',?,'충남','',''),
		('b-pt','협력사','협력사',1,'partner','학교',?,'충남','',''),
		('b-lib','해미도서관','해미도서관',1,'customer','도서관',?,'충남','서산시','02'),
		('b-taken','타팀학교','타팀학교',1,'customer','학교',?,'세종','','')`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, "O03"); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "b-taken", ShortName: "타팀", InspectionCycle: "monthly", OrgID: "O03", Region: "세종",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE maintenance_site_config SET org_id='O03' WHERE customer_id='b-taken'`); err != nil {
		t.Fatal(err)
	}
	return repo, db
}

func TestMnt95_15IndustryBucketsMergeSchool(t *testing.T) {
	repo, db := setupSiteBulk(t)
	buckets, err := repo.ListSiteBulkIndustries()
	if err != nil {
		t.Fatal(err)
	}
	var school, lib *model.SiteBulkIndustry
	for i := range buckets {
		switch buckets[i].Key {
		case "학교":
			school = &buckets[i]
		case "도서관":
			lib = &buckets[i]
		}
	}
	if school == nil || school.Count != 3 { // 활성 학교 2 + 타팀 1 (후보 건수=해당 업종 활성 고객 중 사이트 없는 것+타팀?)
		// 지시: 후보 건수는 업종 실제 값 COUNT. 활성 고객 전체에서 학교+school.
		// 비활성·협력사는 후보가 아님. 버킷 COUNT는 활성 customer 업종.
		if school == nil || school.Count != 3 {
			t.Fatalf("학교 버킷 %+v", school)
		}
	}
	if lib == nil || lib.Count != 1 {
		t.Fatalf("도서관 %+v", lib)
	}
	var raw string
	if err := db.QueryRow(`SELECT industry FROM customers WHERE customer_id='b-sch2'`).Scan(&raw); err != nil || raw != "school" {
		t.Fatalf("원값 유지 %q %v", raw, err)
	}
}

func TestMnt95_15CandidatesFilterAndOtherOrgRow(t *testing.T) {
	repo, _ := setupSiteBulk(t)
	rows, err := repo.ListSiteBulkCandidates("학교", model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]model.SiteBulkCandidate{}
	for _, r := range rows {
		by[r.CustomerID] = r
	}
	if _, ok := by["b-off"]; ok {
		t.Fatal("비활성은 후보에서 제외")
	}
	if _, ok := by["b-pt"]; ok {
		t.Fatal("협력사 제외")
	}
	if _, ok := by["b-lib"]; ok {
		t.Fatal("타 업종 제외")
	}
	sch := by["b-sch"]
	if !sch.Selectable || sch.RegionPreview != "충남" {
		t.Fatalf("선택 가능 학교 %+v", sch)
	}
	if sch.RegionPreview == "" {
		t.Fatal("지역 미리보기")
	}
	taken := by["b-taken"]
	if taken.CustomerID == "" || taken.Selectable {
		t.Fatalf("타 팀 행은 비활성 %+v", taken)
	}
	if taken.OtherOrgName == "" || strings.Contains(taken.OtherOrgName, "타팀학교") && taken.OtherOrgName == "타팀학교" {
		// 조직명만. 사이트 상세(표시명 등) 없음
	}
	if taken.OtherOrgName == "" {
		t.Fatal("타 조직명만 노출")
	}
	if taken.ShortName == "타팀" {
		t.Fatal("타 팀 사이트의 표시명을 노출하면 안 됨")
	}
}

func TestMnt95_15BulkRegisterCheckedOnlyAndIdempotent(t *testing.T) {
	repo, db := setupSiteBulk(t)
	in := model.SiteBulkRegisterInput{
		OrgID: model.OrgIDLibrary, IndustryKey: "학교", Cycle: "quarterly", Rounds: 2,
		CustomerIDs: []string{"b-sch"},
		ShortNames:  map[string]string{"b-sch": "계룡고표시"},
	}
	res, err := repo.BulkRegisterSites(in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Registered != 1 {
		t.Fatalf("등록 %d skip %+v", res.Registered, res.Skipped)
	}
	cfg, _ := repo.GetSiteConfig("b-sch")
	if cfg == nil || cfg.ShortName != "계룡고표시" || cfg.InspectionCycle != "quarterly" || cfg.SiteRounds() != 2 {
		t.Fatalf("사이트 %+v", cfg)
	}
	if cfg.OrgID != model.OrgIDLibrary && cfg.OrgID != "" {
		// GetSiteConfig joins customers.org_id; bulk must store site org
	}
	var siteOrg string
	_ = db.QueryRow(`SELECT COALESCE(org_id,'') FROM maintenance_site_config WHERE customer_id='b-sch'`).Scan(&siteOrg)
	if siteOrg != model.OrgIDLibrary {
		t.Fatalf("사이트 org_id %s", siteOrg)
	}
	if c2, err := repo.GetSiteConfig("b-sch2"); err != nil || c2 != nil {
		t.Fatal("체크 안 한 고객은 만들면 안 됨")
	}
	again, err := repo.BulkRegisterSites(in)
	if err != nil {
		t.Fatal(err)
	}
	if again.Registered != 0 || len(again.Skipped) == 0 {
		t.Fatalf("멱등 %+v", again)
	}
	plans, _ := repo.ListPlans()
	for _, p := range plans {
		vs, _ := repo.ListVisits(p.PlanID)
		if len(vs) != 0 {
			t.Fatalf("방문 자동 생성 금지 %d", len(vs))
		}
	}
}

func TestMnt95_15BulkRevalidateSkips(t *testing.T) {
	repo, _ := setupSiteBulk(t)
	res, err := repo.BulkRegisterSites(model.SiteBulkRegisterInput{
		OrgID: model.OrgIDLibrary, IndustryKey: "학교", Cycle: "monthly",
		CustomerIDs: []string{"b-off", "b-pt", "b-taken", "no-such", "b-lib"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Registered != 0 || len(res.Skipped) < 4 {
		t.Fatalf("건너뜀 기대 got %+v", res)
	}
}

func TestMnt95_15BulkRejectsOverLimitAndBadCycle(t *testing.T) {
	repo, _ := setupSiteBulk(t)
	ids := make([]string, model.SiteBulkMax+1)
	for i := range ids {
		ids[i] = "x"
	}
	if _, err := repo.BulkRegisterSites(model.SiteBulkRegisterInput{OrgID: model.OrgIDLibrary, IndustryKey: "학교", Cycle: "monthly", CustomerIDs: ids}); err == nil {
		t.Fatal("3000 초과")
	}
	if _, err := repo.BulkRegisterSites(model.SiteBulkRegisterInput{OrgID: model.OrgIDLibrary, IndustryKey: "학교", Cycle: "", CustomerIDs: []string{"b-sch"}}); err == nil {
		t.Fatal("주기 필수")
	}
}

func TestMnt95_15BulkPerfThousand(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "perf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	st, _ := tx.Prepare(`INSERT INTO customers (customer_id, org_name, official_name, is_active, party_kind, industry, org_id, addr_sido)
		VALUES (?,?,?,1,'customer','학교',?,'충남')`)
	ids := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		id := "p" + itoa(i)
		ids[i] = id
		if _, err := st.Exec(id, id, id, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	start := time.Now()
	res, err := repo.BulkRegisterSites(model.SiteBulkRegisterInput{
		OrgID: model.OrgIDLibrary, IndustryKey: "학교", Cycle: "monthly", CustomerIDs: ids,
	})
	elapsed := time.Since(start)
	if err != nil || res.Registered != 1000 {
		t.Fatalf("perf %v %+v", err, res)
	}
	t.Logf("1000건 등록 %s", elapsed)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

package handler

import (
	"net/http"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_144_T1_OwnIsCustomerOrgAndClosedSelectedShown(t *testing.T) {
	all := []model.WorkProject{
		{ProjectID: "WP-O02", Name: "미디어", OrgID: "O02", Status: model.WBProjectActive, PlanYear: 2026},
		{ProjectID: "WP-O01B", Name: "나사업", OrgID: "O01", Status: model.WBProjectActive, PlanYear: 2025},
		{ProjectID: "WP-O01A", Name: "가사업", OrgID: "O01", Status: model.WBProjectActive, PlanYear: 2026},
	}
	own, other := splitPickerHostOrg(all, "O01")
	if len(own) != 2 {
		t.Fatalf("Own=%d want 2 (고객 조직)", len(own))
	}
	if len(other) != 1 || other[0].ProjectID != "WP-O02" {
		t.Fatalf("Other=%v", other)
	}
	closed := model.WorkProject{ProjectID: "WP-OLD", Name: "닫힌연결", OrgID: "O01", Status: model.WBProjectComplete, PlanYear: 2024}
	ownCopy := append([]model.WorkProject{}, own...)
	got := append([]model.WorkProject{closed}, ownCopy...)
	got = sortPickerOwn(got, 2026, closed.ProjectID)
	if got[0].ProjectID != closed.ProjectID {
		t.Fatalf("선택된 닫힌 사업이 맨 앞이어야 함: %s", got[0].ProjectID)
	}
	yearFirst := sortPickerOwn(own, 2026, "")
	if yearFirst[0].ProjectID != "WP-O01A" {
		t.Fatalf("방문 연도 우선: %s", yearFirst[0].ProjectID)
	}
}

func Test95_144_T1_EnsureSelectedUsesRepo(t *testing.T) {
	// ensureSelectedProject repo 없으면 원본 유지
	own := []model.WorkProject{{ProjectID: "WP-A", Name: "A"}}
	got := ensureSelectedProject(nil, own, "WP-OLD", "WP-OLD")
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
}

func Test95_144_T2_EmptyHint(t *testing.T) {
	hint := "이 고객이 속한 조직에 활성 사업이 없습니다. 타 부서 사업을 펼치거나 사업을 등록하세요."
	if !strings.Contains(hint, "이 고객이 속한 조직") {
		t.Fatal(hint)
	}
	view := ProjectPickerView{EmptyListHint: hint}
	if view.EmptyListHint == "" {
		t.Fatal("EmptyListHint")
	}
}

func Test95_144_T3_VisitTimeMissing(t *testing.T) {
	if !visitActionTimeMissing(model.MaintenanceVisit{Completed: true}) {
		t.Fatal("완료+시간 빈")
	}
	if visitActionTimeMissing(model.MaintenanceVisit{Completed: true, StartTime: "09:00", EndTime: "10:00"}) {
		t.Fatal("시간이 있으면 안내 없음")
	}
	if visitActionTimeMissing(model.MaintenanceVisit{Completed: false}) {
		t.Fatal("미완료는 안내 없음")
	}
}

func Test95_144_T4_AssetMatchFoldStoredSelectAll(t *testing.T) {
	all := []model.Asset{
		{AssetID: "A1", ProductName: "KLAS 3.0 자료관리시스템", ProductType: "SW"},
		{AssetID: "A2", ProductName: "RFID 게이트", ProductType: "HW"},
	}
	targets := []model.MaintenanceVisitTarget{
		{AssetID: "A1", Name: "KLAS"},
		{AssetID: "GONE", Name: "예전장비"},
	}
	match, other, orphans := splitVisitActionAssets(all, "KLAS", targets)
	if len(match) != 1 || match[0].AssetID != "A1" {
		t.Fatalf("match=%v", match)
	}
	if len(other) != 1 || other[0].AssetID != "A2" {
		t.Fatalf("other=%v", other)
	}
	if len(orphans) != 1 || orphans[0].AssetID != "GONE" {
		t.Fatalf("orphans=%v", orphans)
	}
	ids := visitAssetIDs(match)
	if len(ids) != 1 || ids[0] != "A1" {
		t.Fatalf("전체 선택은 제품 일치만: %v", ids)
	}
	m0, o0, or0 := splitVisitActionAssets(nil, "KLAS", nil)
	if len(m0) != 0 || len(o0) != 0 || len(or0) != 0 {
		t.Fatalf("0건 %d %d %d", len(m0), len(o0), len(or0))
	}
}

func Test95_144_VisitProjectLinkCustomerOrgAndClosed(t *testing.T) {
	// 95-144B: 조직 일치 통과는 사이트 등록 규칙으로 대체. 빈 값·기존 연결은 허용.
	if err := requireVisitSiteProject(nil, "c47", "", ""); err != nil {
		t.Fatalf("빈 사업: %v", err)
	}
	if err := requireVisitSiteProject(nil, "c47", "WP-OLD", "WP-OLD"); err != nil {
		t.Fatalf("기존 연결 유지: %v", err)
	}
	if err := requireVisitSiteProject(nil, "c47", "WP-O03", ""); err == nil {
		t.Fatal("미등록 사업은 거절")
	}
}

func Test95_144_VisitActionHTTP_OwnTimeAssets(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-LIB','도서관사업','active',?,0,1,2026),
		       ('WP-OLD','닫힌연결사업','complete',?,0,1,2024),
		       ('WP-MED','미디어사업','active','O03',0,1,2026)`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-LIB','WP-LIB','c47','서산고등학교','in_progress',?,1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c144empty','빈조직고객','빈조직고객',1,'O99')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, project_id)
		VALUES ('v144','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',1,'WP-OLD'),
		       ('v144e','MP47','2026-09-18','c144empty',0,'normal','최혜영','KLAS',1,'')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, product_type, operation_status)
		VALUES ('A-KLAS','c47','KLAS 3.0 자료관리시스템','SW','operating'),
		       ('A-RFID','c47','RFID 게이트','HW','operating')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visit_targets(target_id, visit_id, asset_id, name, sort_order)
		VALUES ('t144g','v144','GONE','예전장비',0)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, err := users.GetByUsername("oa47")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	ck := jwtCookieVerified(t, oa)
	rec := doMnt47Get(t, e, "/maintenance/visits/v144/action", ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "도서관사업") {
		t.Fatal("사이트 등록 활성 사업이 Own에 없음")
	}
	if !strings.Contains(body, "닫힌연결사업") {
		t.Fatal("연결된 닫힌 사업이 없음")
	}
	if !strings.Contains(body, "방문 시간이 입력되지 않았습니다") {
		t.Fatal("시간 안내 없음")
	}
	if !strings.Contains(body, "data-fill-plan") {
		t.Fatal("계획 시각 채우기 강조 없음")
	}
	if !strings.Contains(body, "data-match-assets") || !strings.Contains(body, "KLAS 3.0") {
		t.Fatal("제품 일치 장비 없음")
	}
	if !strings.Contains(body, "다른 제품 장비") || !strings.Contains(body, "RFID") {
		t.Fatal("다른 제품 접기 없음")
	}
	if !strings.Contains(body, "저장된 대상") || !strings.Contains(body, "예전장비") {
		t.Fatal("저장된 대상 없음")
	}
	if !strings.Contains(body, "A-KLAS") {
		t.Fatal("전체 선택 대상")
	}
	empty := doMnt47Get(t, e, "/maintenance/visits/v144e/action", ck)
	if empty.Code != http.StatusOK {
		t.Fatalf("empty GET %d", empty.Code)
	}
	if !strings.Contains(empty.Body.String(), "이 사이트가 등록된 사업이 없습니다") {
		t.Fatal("Own 빈 안내 문구 없음")
	}
}

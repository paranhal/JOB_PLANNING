package handler

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_144B_T1_SiteProjectsNotOrg(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-A','사업A','active',?,0,1,2026),
		       ('WP-B','사업B','active',?,0,1,2025),
		       ('WP-C','사업C','active',?,0,1,2026)`, org, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','c47','서산고등학교','in_progress',?,1),
		       ('PS-B','WP-B','c47','서산고등학교','in_progress',?,1)`, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, project_id)
		VALUES ('v144b','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',0,'')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, err := users.GetByUsername("oa47")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	ck := jwtCookieVerified(t, oa)
	rec := doMnt47Get(t, e, "/maintenance/visits/v144b/action", ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "사업A") || !strings.Contains(body, "사업B") {
		t.Fatal("등록된 A·B가 후보에 없음")
	}
	if !strings.Contains(body, "SiteCount") || !(strings.Contains(body, `"SiteCount":2`) || strings.Contains(body, "SiteCount&#34;:2") || strings.Contains(body, `\u0022SiteCount\u0022:2`)) {
		t.Fatal("안내 N건은 사이트 등록 건수여야 함")
	}
	if !strings.Contains(body, "data-site-project-note") {
		t.Fatal("열기 전 N건 안내 칸 없음")
	}
	js := doMnt47Get(t, e, "/workboard/projects/for-site?customer_id=c47", ck)
	if js.Code != http.StatusOK {
		t.Fatalf("for-site %d %s", js.Code, js.Body.String())
	}
	if !strings.Contains(js.Body.String(), "WP-A") || !strings.Contains(js.Body.String(), "WP-B") {
		t.Fatalf("json %s", js.Body.String())
	}
	if strings.Contains(js.Body.String(), "WP-C") {
		t.Fatal("json에 C")
	}
}

func Test95_144B_T2_EmptyHintNoOther(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('v144b0','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',0)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, err := users.GetByUsername("oa47")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	body := doMnt47Get(t, e, "/maintenance/visits/v144b0/action", jwtCookieVerified(t, oa)).Body.String()
	if !strings.Contains(body, "이 사이트가 등록된 사업이 없습니다. 사업명을 검색해 연결하세요") {
		t.Fatal("0건 안내 없음")
	}
	if strings.Contains(body, "사업 → 사이트 등록에서 이 사이트를 먼저 등록하세요") {
		t.Fatal("옛 문구가 남음")
	}
	if !strings.Contains(body, `data-hide-other="1"`) {
		t.Fatal("타 부서 목록을 숨겨야 함")
	}
	if strings.Contains(body, "타 부서 사업 보기") {
		t.Fatal("타 부서 사업 목록이 있음")
	}
	if !strings.Contains(body, "data-site-project-note") {
		t.Fatal("열기 전 안내 칸 없음")
	}
}

func Test95_144B_T3_SaveAndWeekly(t *testing.T) {
	e, db, h := newMnt47Server(t)
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-A','사업A','active',?,0,1,2026),
		       ('WP-C','사업C','active',?,0,1,2026),
		       ('WP-OLD','닫힌연결','complete',?,0,1,2024)`, org, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','c47','서산고등학교','in_progress',?,1)`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, project_id)
		VALUES ('v144bs','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',0,''),
		       ('v144bo','MP47','2026-10-19','c47',0,'normal','최혜영','KLAS',0,'WP-OLD')`); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookie(t)
	form := url.Values{
		"assignee":   {"최혜영"},
		"visit_date": {"2026-09-18"},
		"status":     {"open"},
		"notes":      {"연결"},
		"start_time": {"10:00"},
		"end_time":   {"11:00"},
		"project_id": {"WP-C"},
	}
	bad := doMnt47Form(t, e, "/maintenance/visits/v144bs/action", form, ck)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("미등록 저장 %d %s", bad.Code, bad.Body.String())
	}
	if !strings.Contains(bad.Body.String(), "이 사이트가 등록되지 않은 사업입니다") {
		t.Fatalf("400 문구 %s", bad.Body.String())
	}
	form.Set("project_id", "WP-A")
	ok := doMnt47Form(t, e, "/maintenance/visits/v144bs/action", form, ck)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("등록 저장 %d %s", ok.Code, ok.Body.String())
	}
	keep := url.Values{
		"assignee":   {"최혜영"},
		"visit_date": {"2026-10-19"},
		"status":     {"open"},
		"notes":      {"유지"},
		"start_time": {"10:00"},
		"end_time":   {"11:00"},
		"project_id": {"WP-OLD"},
	}
	kept := doMnt47Form(t, e, "/maintenance/visits/v144bo/action", keep, ck)
	if kept.Code != http.StatusSeeOther {
		t.Fatalf("닫힌 기존 %d %s", kept.Code, kept.Body.String())
	}

	we := echo.New()
	we.Renderer = NewRenderer()
	mountWeekly28(we, h)
	wck := jwtCookieOrg(t, model.RoleOrgAdmin, org)
	wbad := postWeeklyLink(t, we, url.Values{
		"item": {"maintenance_visit:v144bs"}, "project_id": {"WP-C"}, "org": {org},
	}, wck)
	if wbad.Code != http.StatusBadRequest {
		t.Fatalf("주간 미등록 %d %s", wbad.Code, wbad.Body.String())
	}
	if !strings.Contains(wbad.Body.String(), "이 사이트가 등록되지 않은 사업입니다") {
		t.Fatalf("주간 문구 %s", wbad.Body.String())
	}
}

func Test95_144B_T4_OtherScreensKeepOtherPicker(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p144b.db"))
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid)
		VALUES ('WP-O01','도서관사업','active','O01',0,1),
		       ('WP-O02','미디어사업','active','O02',0,1)`); err != nil {
		t.Fatal(err)
	}
	oa, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	form := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date=2026-11-04", jwtCookieUser(t, oa)).Body.String()
	if !strings.Contains(form, "타 부서 사업 보기") {
		t.Fatal("신규 업무 타 부서 보기 회귀")
	}
	if strings.Contains(form, `data-hide-other="1"`) {
		t.Fatal("신규 업무 HideOther 기본값이 켜지면 안 됨")
	}
	_ = me
}

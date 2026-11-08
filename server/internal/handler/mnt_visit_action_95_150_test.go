package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func seed150Projects(t *testing.T, db *sql.DB) {
	t.Helper()
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-A','사업A','active',?,0,1,2026),
		       ('WP-B','사업B','active',?,0,1,2025),
		       ('WP-C','사업C','active',?,0,1,2026),
		       ('WP-OLD','닫힌사업','complete',?,0,1,2024),
		       ('WP-X','타부서사업','active','O99',0,1,2026)`, org, org, org, org); err != nil {
		t.Fatal(err)
	}
}

func Test95_150_T1_TwoSiteRowsNoAutoNoOther(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	org := model.OrgIDLibrary
	seed150Projects(t, db)
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','c47','서산고등학교','in_progress',?,1),
		       ('PS-B','WP-B','c47','서산고등학교','in_progress',?,1)`, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, project_id)
		VALUES ('v150a','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',0,'WP-A')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, err := users.GetByUsername("oa47")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	ck := jwtCookieVerified(t, oa)
	body := doMnt47Get(t, e, "/maintenance/visits/v150a/action", ck).Body.String()
	if !strings.Contains(body, "사업A") || !strings.Contains(body, "사업B") {
		t.Fatal("A·B 행 없음")
	}
	if strings.Contains(body, "사업C") {
		t.Fatal("미등록 C")
	}
	if strings.Contains(body, "타 부서 사업 보기") {
		t.Fatal("타 부서 보기")
	}
	if !strings.Contains(body, `data-hide-other="1"`) {
		t.Fatal("HideOther")
	}
	if strings.Count(extractPickerSiteJSON(body), `"project_id"`) != 2 {
		t.Fatalf("행 수 %s", extractPickerSiteJSON(body))
	}
	if !strings.Contains(body, "현재 연결") {
		t.Fatal("저장된 사업 현재 연결 없음")
	}
	oneID := "v150a1"
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES (?,'MP47','2026-09-19','c47',0,'normal','최혜영','KLAS',0)`, oneID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM project_sites WHERE customer_id='c47' AND project_id='WP-B'`); err != nil {
		t.Fatal(err)
	}
	one := doMnt47Get(t, e, "/maintenance/visits/"+oneID+"/action", ck).Body.String()
	if strings.Contains(one, `"Selected":"WP-A"`) {
		t.Fatal("1건 자동 선택")
	}
	plan := doMnt47Get(t, e, "/maintenance/MP47", ck)
	if plan.Code != http.StatusOK {
		t.Fatalf("계획 %d %s", plan.Code, plan.Body.String())
	}
	pb := plan.Body.String()
	if !strings.Contains(pb, `id="mnt-edit-project-picker"`) {
		t.Fatal("방문 수정 피커 없음")
	}
	if !strings.Contains(pb, `"HideOther":true`) && !strings.Contains(pb, "HideOther&#34;:true") {
		t.Fatal("방문 수정 HideOther")
	}
}

func Test95_150_T2_EmptySearchHint(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('v150e','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',0)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, _ := users.GetByUsername("oa47")
	body := doMnt47Get(t, e, "/maintenance/visits/v150e/action", jwtCookieVerified(t, oa)).Body.String()
	if !strings.Contains(body, "이 사이트가 등록된 사업이 없습니다. 사업명을 검색해 연결하세요") {
		t.Fatal("새 안내 없음")
	}
	if strings.Contains(body, "사업 → 사이트 등록에서 이 사이트를 먼저 등록하세요") {
		t.Fatal("옛 문구")
	}
	if !strings.Contains(body, `type="search"`) {
		t.Fatal("검색 입력 없음")
	}
	if strings.Contains(body, "타 부서 사업 보기") {
		t.Fatal("타 부서 보기")
	}
}

func Test95_150_T3_LinkConfirmSaveCompleted(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	seed150Projects(t, db)
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, completed_date)
		VALUES ('v150c','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',1,'2026-09-18')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, _ := users.GetByUsername("oa47")
	ck := jwtCookieVerified(t, oa)
	noc := doMnt47Form(t, e, "/workboard/projects/link-site", url.Values{
		"project_id": {"WP-A"}, "customer_id": {"c47"},
	}, ck)
	if noc.Code != http.StatusBadRequest {
		t.Fatalf("확인 없이 %d %s", noc.Code, noc.Body.String())
	}
	var n0 int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE customer_id='c47' AND project_id='WP-A'`).Scan(&n0)
	if n0 != 0 {
		t.Fatal("확인 전 등록됨")
	}
	ok := doMnt47Form(t, e, "/workboard/projects/link-site", url.Values{
		"confirm": {"1"}, "project_id": {"WP-A"}, "customer_id": {"c47"},
	}, ck)
	if ok.Code != http.StatusOK {
		t.Fatalf("link %d %s", ok.Code, ok.Body.String())
	}
	again := doMnt47Form(t, e, "/workboard/projects/link-site", url.Values{
		"confirm": {"1"}, "project_id": {"WP-A"}, "customer_id": {"c47"},
	}, ck)
	if again.Code != http.StatusOK {
		t.Fatalf("dup %d %s", again.Code, again.Body.String())
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE customer_id='c47' AND project_id='WP-A'`).Scan(&n)
	if n != 1 {
		t.Fatalf("sites=%d", n)
	}
	save := doMnt47Form(t, e, "/maintenance/visits/v150c/action", url.Values{
		"assignee": {"최혜영"}, "visit_date": {"2026-09-18"}, "status": {"done"},
		"notes": {"연결"}, "start_time": {"10:00"}, "end_time": {"11:00"},
		"project_id": {"WP-A"},
	}, ck)
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", save.Code, save.Body.String())
	}
	var pid string
	if err := db.QueryRow(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='v150c'`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if pid != "WP-A" {
		t.Fatalf("visit.project_id=%q", pid)
	}
	reload := doMnt47Get(t, e, "/maintenance/visits/v150c/action", ck).Body.String()
	if !strings.Contains(reload, "사업A") {
		t.Fatal("새로고침 사업명 없음")
	}
	edit := doMnt47Form(t, e, "/maintenance/visits/v150c/update", url.Values{
		"visit_date": {"2026-09-18"}, "customer_id": {"c47"}, "product_type": {"KLAS"},
		"assignee": {"최혜영"}, "project_id": {"WP-A"}, "completed_present": {"1"}, "completed": {"1"},
	}, ck)
	if edit.Code != http.StatusSeeOther {
		t.Fatalf("방문 수정 %d %s", edit.Code, edit.Body.String())
	}
}

func Test95_150_T4_ClosedAndPermission(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	seed150Projects(t, db)
	users := repository.NewUserRepo(db)
	oa, _ := users.GetByUsername("oa47")
	ck := jwtCookieVerified(t, oa)
	closed := doMnt47Get(t, e, "/workboard/projects/search?q=닫힌", ck)
	if closed.Code != http.StatusOK || strings.Contains(closed.Body.String(), "WP-OLD") {
		t.Fatalf("마감 검색 %s", closed.Body.String())
	}
	short := doMnt47Get(t, e, "/workboard/projects/search?q=사", ck)
	if strings.Contains(short.Body.String(), "WP-A") {
		t.Fatal("1글자 검색")
	}
	linkClosed := doMnt47Form(t, e, "/workboard/projects/link-site", url.Values{
		"confirm": {"1"}, "project_id": {"WP-OLD"}, "customer_id": {"c47"},
	}, ck)
	if linkClosed.Code == http.StatusOK {
		t.Fatal("마감 연결 허용")
	}
	a, err := users.GetByUsername("a47")
	if err != nil || a == nil {
		t.Fatal(err)
	}
	other := doMnt47Get(t, e, "/workboard/projects/search?q=타부서", jwtCookieVerified(t, a))
	if strings.Contains(other.Body.String(), "WP-X") {
		t.Fatal("기술 담당 타 부서 검색")
	}
	oaHit := doMnt47Get(t, e, "/workboard/projects/search?q=타부서", ck)
	if !strings.Contains(oaHit.Body.String(), "WP-X") {
		t.Fatalf("조직관리자 타 부서 검색 %s", oaHit.Body.String())
	}
}

func Test95_150_T5_MeetingProjectBlock(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	org := model.OrgIDLibrary
	seed150Projects(t, db)
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','c47','서산고등학교','in_progress',?,1)`, org); err != nil {
		t.Fatal(err)
	}
	day := "2026-09-18"
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, project_id)
		VALUES ('v150m','MP47',?,'c47',0,'normal','최혜영','KLAS',0,'WP-A')`, day); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, _ := users.GetByUsername("oa47")
	ck := jwtCookieVerified(t, oa)
	save := doMnt47Form(t, e, "/maintenance/visits/v150m/action", url.Values{
		"assignee": {"최혜영"}, "visit_date": {day}, "status": {"open"},
		"start_time": {"10:00"}, "end_time": {"11:00"}, "project_id": {"WP-A"},
	}, ck)
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", save.Code, save.Body.String())
	}
	body := doMnt47Get(t, e, "/meeting/board?from="+day+"&to="+day+"&view=project", ck).Body.String()
	if !strings.Contains(body, "사업A") {
		t.Fatal("사업별 보기 블록 없음")
	}
	if !strings.Contains(body, "서산고등학교") && !strings.Contains(body, "v150m") {
		t.Fatal("방문이 사업 블록에 없음")
	}
}

func Test95_150_T6_BrowserVisitAction(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, _ := newMnt47Server(t)
	seed150Projects(t, db)
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('v150b','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',0)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, _ := users.GetByUsername("oa47")
	ck := jwtCookieVerified(t, oa)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_150_visit_action_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"SCREENSHOT_DIR="+shot,
		"ACTION_PATH=/maintenance/visits/v150b/action",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") || (strings.Contains(s, "playwright") && strings.Contains(s, "install")) {
			t.Skip(s)
		}
		t.Fatalf("browser: %v\n%s", err, s)
	}
}

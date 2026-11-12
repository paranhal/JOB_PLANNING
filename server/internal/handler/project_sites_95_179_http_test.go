package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 1단계 업무·2단계 하부 업무 사이트 등록 화면. 95-179B.
func Test95_179B_SiteRegistrationPagesHTTP(t *testing.T) {
	e, repo, wb, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"사이트등록사업"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	db := wb.DB()
	sites := map[string]string{}
	for i, s := range []struct{ name, region string }{
		{"홍성고등학교", "충남 홍성군"}, {"홍성초등학교", "충남 홍성군"}, {"천안고등학교", "충남 천안시"}, {"예산중학교", "충남 예산군"},
	} {
		cid := fmt.Sprintf("C179B%d", i)
		if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,?)`, cid, s.name, s.name, model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
		sid := fmt.Sprintf("S179B%d", i)
		if _, err := db.Exec(`INSERT INTO project_sites(id, project_id, site_name, region, status, org_id, customer_id) VALUES (?,?,?,?, 'planned',?,?)`,
			sid, id, s.name, s.region, model.OrgIDLibrary, cid); err != nil {
			t.Fatal(err)
		}
		sites[s.name] = sid
	}
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "전처리 업무", Status: model.PhaseActive}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	phaseID := phases[0].PhaseID
	siteCnt := 0
	pkg, err := repo.CreatePackage(phaseID, "GBIC 수량 산정", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	qtyPkg, _ := repo.CreatePackage(phaseID, "수량만", model.TrackModePhase)
	_ = siteCnt

	// 수행 구조 화면에 링크: 단계, 사이트 하부 업무(수량만 제외)
	page := doGet(t, e, "/projects/"+id).Body.String()
	if !strings.Contains(page, `/phases/`+phaseID+`/sites`) || !strings.Contains(page, `/packages/`+pkg+`/sites`) || strings.Contains(page, `/packages/`+qtyPkg+`/sites`) {
		t.Fatal("사이트 등록 링크가 맞지 않다")
	}

	// 1단계 화면
	p1 := doGet(t, e, "/projects/"+id+"/phases/"+phaseID+"/sites").Body.String()
	for _, want := range []string{"data-structure-sites", "결과 전체 선택", "사업 사이트 전체 가져오기 (4곳)", "홍성고등학교", "data-sites-register-link", "등록 <b data-registered>0</b>곳 / 후보 풀 <b>4</b>곳"} {
		if !strings.Contains(p1, want) {
			t.Fatalf("1단계 화면에 %q 가 없다", want)
		}
	}
	// 검색·지역 필터가 걸리면 버튼이 「조회 결과 전체 등록」으로 바뀐다
	pf := doGet(t, e, "/projects/"+id+"/phases/"+phaseID+"/sites?q="+url.QueryEscape("홍성")).Body.String()
	if !strings.Contains(pf, "조회 결과 전체 등록 (2곳)") || strings.Contains(pf, "천안고등학교") {
		t.Fatal("검색 결과 화면")
	}
	pr := doGet(t, e, "/projects/"+id+"/phases/"+phaseID+"/sites?region="+url.QueryEscape("충남 천안시")).Body.String()
	if !strings.Contains(pr, "천안고등학교") || strings.Contains(pr, "홍성고등학교") || !strings.Contains(pr, "충남 예산군") {
		t.Fatal("지역 필터 화면(드롭다운에는 전체 지역이 남아야 한다)")
	}

	post := func(path string, v url.Values, cookies ...*http.Cookie) (int, string) {
		t.Helper()
		if len(cookies) == 0 {
			cookies = []*http.Cookie{jwtCookie(t)}
		}
		r := postPhase(t, e, "/projects/"+id+path, v, cookies...)
		loc, _ := url.QueryUnescape(r.Header().Get("Location"))
		return r.Code, loc
	}
	// 선택 등록 → 안내
	code, loc := post("/phases/"+phaseID+"/sites/add", url.Values{"site_id": {sites["홍성고등학교"], sites["천안고등학교"]}, "q": {""}})
	if code != http.StatusSeeOther || !strings.Contains(loc, "ok=add:2") {
		t.Fatalf("선택 등록 %d %s", code, loc)
	}
	after := doGet(t, e, "/projects/"+id+"/phases/"+phaseID+"/sites?ok=add:2").Body.String()
	if !strings.Contains(after, "사이트 2곳을 등록했습니다.") || strings.Count(after, "data-site-added") != 2 || !strings.Contains(after, "<b data-registered>2</b>") {
		t.Fatal("등록 뒤 화면")
	}
	// 조회 결과 전체 등록(홍성 → 홍성초만 남음)
	if _, loc = post("/phases/"+phaseID+"/sites/add", url.Values{"all": {"1"}, "q": {"홍성"}}); !strings.Contains(loc, "ok=add:1") || !strings.Contains(loc, "q=") {
		t.Fatalf("필터 전체 등록 %s", loc)
	}
	// 제외: 사유 필수 / 하부 업무에 쓰이는 사이트 거절
	if _, loc = post("/phases/"+phaseID+"/sites/remove", url.Values{"site_id": {sites["천안고등학교"]}}); !strings.Contains(loc, "err=") || !strings.Contains(loc, "제외 사유") {
		t.Fatalf("사유 없는 제외 %s", loc)
	}
	if _, loc = post("/phases/"+phaseID+"/sites/remove", url.Values{"site_id": {sites["천안고등학교"]}, "reason": {"범위 밖"}}); !strings.Contains(loc, "ok=remove:1") {
		t.Fatalf("제외 %s", loc)
	}

	// 2단계 화면: 1단계에 등록된 사이트(홍성고·홍성초)만 후보
	p2 := doGet(t, e, "/projects/"+id+"/packages/"+pkg+"/sites").Body.String()
	if !strings.Contains(p2, "홍성고등학교") || !strings.Contains(p2, "홍성초등학교") || strings.Contains(p2, "천안고등학교") || strings.Contains(p2, "예산중학교") ||
		!strings.Contains(p2, "1단계 업무 사이트 전체 가져오기 (2곳)") {
		t.Fatal("2단계 화면 후보가 1단계 등록 사이트가 아니다")
	}
	if _, loc = post("/packages/"+pkg+"/sites/add", url.Values{"site_id": {sites["예산중학교"]}}); !strings.Contains(loc, "err=") || !strings.Contains(loc, "1단계 업무에 등록된") {
		t.Fatalf("1단계 밖 사이트 %s", loc)
	}
	if _, loc = post("/packages/"+pkg+"/sites/add", url.Values{"all": {"1"}}); !strings.Contains(loc, "ok=add:2") {
		t.Fatalf("1단계 사이트 전체 가져오기 %s", loc)
	}
	if _, loc = post("/packages/"+qtyPkg+"/sites/add", url.Values{"site_id": {sites["홍성고등학교"]}}); !strings.Contains(loc, "err=") {
		t.Fatalf("수량만 %s", loc)
	}
	if r := doGet(t, e, "/projects/"+id+"/packages/"+qtyPkg+"/sites"); r.Code == http.StatusOK {
		t.Fatal("수량만 하부 업무의 사이트 화면이 열렸다")
	}

	// 권한: 임원은 조회만(체크박스·버튼 없음), POST 는 403. 다른 사업의 단계·하부 업무는 404
	clevel := jwtCookieUser(t, &model.User{UserID: "cl179b", Username: "cl179b", Role: model.RoleCLevel, OrgID: model.OrgIDLibrary})
	cv := doGetWith(t, e, "/projects/"+id+"/phases/"+phaseID+"/sites", clevel)
	if cv.Code != http.StatusOK || strings.Contains(cv.Body.String(), `name="site_id"`) || strings.Contains(cv.Body.String(), "data-add-all-form") {
		t.Fatalf("임원 조회 화면 %d", cv.Code)
	}
	if code, _ = post("/phases/"+phaseID+"/sites/add", url.Values{"all": {"1"}}, clevel); code != http.StatusForbidden {
		t.Fatalf("임원 등록 %d", code)
	}
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES ('WPO179','타사업','active',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if r := doGet(t, e, "/projects/WPO179/phases/"+phaseID+"/sites"); r.Code != http.StatusNotFound {
		t.Fatalf("다른 사업의 단계 %d", r.Code)
	}
	if r := doGet(t, e, "/projects/WPO179/packages/"+pkg+"/sites"); r.Code != http.StatusNotFound {
		t.Fatalf("다른 사업의 하부 업무 %d", r.Code)
	}
}

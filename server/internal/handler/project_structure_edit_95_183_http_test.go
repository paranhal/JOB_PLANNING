package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func cookie183(t *testing.T, uid, role, org string, ro bool, perms ...string) *http.Cookie {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": uid, "username": uid, "role": role, "name": uid, "org_id": org, "is_readonly": ro,
		"permissions": model.FormatPermissions(perms), "verified_at": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte("cs-system-jwt-secret-2026"))
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

type env183 struct {
	e                *echo.Echo
	repo             *repository.ProjectRepo
	id, phase, pkg   string
	qtyPkg, emptyPkg string
	plan, plan2      string
	sites            []string
	admin            *http.Cookie
	wb               *repository.WBRepo
}

func setup183(t *testing.T) env183 {
	t.Helper()
	e, repo, wb, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"WP-061 유사"}, "plan_year": {"2027"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2027-03-01"}, "end_date": {"2031-02-28"}, "color": {"#3B82F6"},
		"org_id": {model.OrgIDLibrary}, "is_paid": {"1"}, "contract_amount": {"1500000"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	v := env183{e: e, repo: repo, id: id, wb: wb, admin: cookie183(t, "oa", "org_admin", "O01", false)}
	for i, n := range []string{"홍성고", "홍천초", "횡성중", "청양고"} {
		sid := fmt.Sprintf("PS-E%d", i)
		v.sites = append(v.sites, sid)
		if _, err := wb.DB().Exec(`INSERT INTO project_sites (id, project_id, site_name, region, status, is_active, org_id) VALUES (?,?,?,?,'planned',1,?)`,
			sid, id, n, "충남", model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "잔처리 작업", Status: model.PhasePlanned}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	v.phase = phases[0].PhaseID
	if _, err := repo.AddPhaseSites(id, v.phase, v.sites, false, "", "", "t"); err != nil {
		t.Fatal(err)
	}
	v.pkg, _ = repo.CreatePackage(v.phase, "홍성", model.TrackModeSite)
	v.qtyPkg, _ = repo.CreatePackage(v.phase, "수량 업무", model.TrackModePhase)
	v.emptyPkg, _ = repo.CreatePackage(v.phase, "빈 업무", model.TrackModeSite)
	if _, err := repo.AddPackageSites(id, v.pkg, v.sites, false, "", "", "t"); err != nil {
		t.Fatal(err)
	}
	v.plan, _ = repo.CreatePlan(id, repository.PlanDraft{PackageID: v.pkg, Name: "1차 계획", PlanStart: "2027-04-01", PlanEnd: "2027-05-01", Basis: model.BasisSite, SiteIDs: v.sites[:2], Actor: "t"})
	v.plan2, _ = repo.CreatePlan(id, repository.PlanDraft{PackageID: v.pkg, Name: "2차 계획", Basis: model.BasisSite, SiteIDs: v.sites[2:3], Actor: "t"})
	for _, a := range [][2]string{{"pm-u", "pm"}, {"exec-u", "execution"}} {
		if _, err := wb.DB().Exec(`INSERT INTO project_assignments (project_id, user_id, duty, created_by) VALUES (?,?,?, 'x')`, id, a[0], a[1]); err != nil {
			t.Fatal(err)
		}
	}
	return v
}

func (v env183) get(t *testing.T, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return doGetWith(t, v.e, path, ck)
}

func (v env183) post(t *testing.T, path string, f url.Values, ck *http.Cookie) (int, string) {
	t.Helper()
	r := postPhase(t, v.e, path, f, ck)
	loc, _ := url.QueryUnescape(r.Header().Get("Location"))
	return r.Code, loc
}

func (v env183) batch(t *testing.T, items []map[string]interface{}, ck *http.Cookie) (int, map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(map[string]interface{}{"items": items})
	req := httptest.NewRequest(http.MethodPost, "http://localhost/projects/"+v.id+"/structure/batch", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	v.e.ServeHTTP(rec, req)
	out := map[string]interface{}{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestHTTP95_183_RenderControls(t *testing.T) {
	v := setup183(t)
	body := v.get(t, "/projects/"+v.id, v.admin).Body.String()
	// 앵커
	for _, want := range []string{
		`id="phase-` + v.phase + `"`, `id="package-` + v.pkg + `"`, `id="plan-` + v.plan + `"`,
		`data-phase-edit`, `data-struct-form="phase"`, `data-struct-form="package" data-id="` + v.pkg + `"`, `data-struct-form="plan" data-id="` + v.plan + `"`,
		`action="/projects/` + v.id + `/packages/` + v.pkg + `/move"`, `action="/projects/` + v.id + `/packages/` + v.pkg + `/status"`,
		`action="/projects/` + v.id + `/packages/` + v.pkg + `/delete"`,
		`name="track_mode"`, `name="next_note"`, `name="progress_basis"`, `name="qty_unit"`,
		`data-plan-sites-link`, `/plans/` + v.plan + `/sites`, `data-struct-bar`, `structure_batch.js`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("수행 구조 화면에 %q 가 없다", want)
		}
	}
	// 하부 업무 폼에 이름 입력이 있다(이전에는 없었다)
	re := regexp.MustCompile(`(?s)action="/projects/` + v.id + `/packages/` + v.pkg + `"[^>]*>.{0,900}?name="name"`)
	if !re.MatchString(body) {
		t.Fatal("하부 업무 수정 폼에 이름 입력이 없다")
	}
}

func TestHTTP95_183_PermissionMatrix(t *testing.T) {
	v := setup183(t)
	roles := []struct {
		name                         string
		ck                           *http.Cookie
		design, plans, barExpected   bool
		pkgPost, planPost, batchCode int
	}{
		{"비전관리자", cookie183(t, "va", "vision_admin", "O01", false), true, true, true, 303, 303, 200},
		{"조직관리자", v.admin, true, true, true, 303, 303, 200},
		{"PM", cookie183(t, "pm-u", "support", "O01", false, model.PermWorkboard), true, true, true, 303, 303, 200},
		{"수행 담당", cookie183(t, "exec-u", "support", "O01", false, model.PermWorkboard), false, true, true, 403, 303, 403},
		{"일반", cookie183(t, "gen-u", "support", "O01", false, model.PermWorkboard), false, false, false, 403, 403, 403},
		{"읽기 전용", cookie183(t, "ro-u", "org_admin", "O01", true), false, false, false, 403, 403, 403},
		{"C-Level", cookie183(t, "cl", "c_level", "O01", false), false, false, false, 403, 403, 403},
		{"옵저버", cookie183(t, "ob", "observer", "O01", false), false, false, false, 403, 403, 403},
	}
	for _, r := range roles {
		body := v.get(t, "/projects/"+v.id, r.ck).Body.String()
		has := func(s string) bool { return strings.Contains(body, s) }
		if has(`data-struct-form="package"`) != r.design || has(`/packages/`+v.pkg+`/delete`) != r.design || has(`data-phase-edit`) != r.design {
			t.Errorf("%s: 설계 권한 컨트롤 유무 want %v", r.name, r.design)
		}
		if has(`data-struct-form="plan"`) != r.plans || has(`data-add-plan`) != r.plans {
			t.Errorf("%s: 계획 컨트롤 유무 want %v", r.name, r.plans)
		}
		if has(`data-struct-bar`) != r.barExpected {
			t.Errorf("%s: 저장 막대 유무 want %v", r.name, r.barExpected)
		}
		// 서버 쪽 권한(버튼 숨김만으로 끝나지 않는다)
		code, _ := v.post(t, "/projects/"+v.id+"/packages/"+v.pkg, url.Values{"name": {"홍성"}}, r.ck)
		if code != r.pkgPost {
			t.Errorf("%s: 하부 업무 수정 POST %d want %d", r.name, code, r.pkgPost)
		}
		for _, p := range []string{"move", "status", "delete", "restore"} {
			c, _ := v.post(t, "/projects/"+v.id+"/packages/"+v.pkg+"/"+p, url.Values{"dir": {"up"}, "status": {"hold"}}, r.ck)
			if (c == 403) != (r.pkgPost == 403) {
				t.Errorf("%s: 하부 업무 %s POST %d", r.name, p, c)
			}
		}
		code, _ = v.post(t, "/projects/"+v.id+"/plans/"+v.plan, url.Values{"name": {"1차 계획"}, "plan_start": {"2027-04-01"}, "plan_end": {"2027-05-01"}}, r.ck)
		if code != r.planPost {
			t.Errorf("%s: 계획 수정 POST %d want %d", r.name, code, r.planPost)
		}
		c, _ := v.batch(t, []map[string]interface{}{{"kind": "package", "id": v.pkg, "fields": map[string]string{"name": "홍성"}}}, r.ck)
		if c != r.batchCode {
			t.Errorf("%s: 일괄 저장(하부 업무) %d want %d", r.name, c, r.batchCode)
		}
		c, _ = v.batch(t, []map[string]interface{}{{"kind": "plan", "id": v.plan, "fields": map[string]string{"name": "1차 계획"}}}, r.ck)
		wantPlan := 200
		if !r.plans {
			wantPlan = 403
		}
		if c != wantPlan {
			t.Errorf("%s: 일괄 저장(계획) %d want %d", r.name, c, wantPlan)
		}
		// 계획 사이트 화면: 보는 것은 누구나, 변경 컨트롤은 계획 권한자만
		page := v.get(t, "/projects/"+v.id+"/plans/"+v.plan+"/sites", r.ck)
		if page.Code != 200 {
			t.Errorf("%s: 계획 사이트 화면 %d", r.name, page.Code)
			continue
		}
		if strings.Contains(page.Body.String(), `data-plan-add-form`) != r.plans {
			t.Errorf("%s: 계획 사이트 화면 변경 컨트롤 want %v", r.name, r.plans)
		}
	}
	// 다른 사업의 하부 업무·계획은 이 사업 주소로 못 만진다
	if c, _ := v.post(t, "/projects/NO-SUCH/packages/"+v.pkg, url.Values{"name": {"x"}}, v.admin); c == 303 || c == 200 {
		t.Errorf("없는 사업 주소가 통과했다 %d", c)
	}
}

func TestHTTP95_183_PackageEditRedirectAndRename(t *testing.T) {
	v := setup183(t)
	base := "/projects/" + v.id
	code, loc := v.post(t, base+"/packages/"+v.pkg, url.Values{
		"name": {"홍성 교체"}, "track_mode": {"site"}, "next_note": {"검수 준비"}, "plan_start": {""}, "plan_end": {""}, "badge": {"사전"},
	}, v.admin)
	if code != 303 || !strings.Contains(loc, "ok=pkg-meta") || !strings.HasSuffix(loc, "#package-"+v.pkg) {
		t.Fatalf("저장 후 위치: %d %s", code, loc)
	}
	body := v.get(t, base+"?ok=pkg-meta", v.admin).Body.String()
	if !strings.Contains(body, "홍성 교체") || !strings.Contains(body, "하부 업무를 저장했습니다") {
		t.Fatal("이름 변경·성공 문구가 화면에 없다")
	}
	// 이름 중복 → 오류, 같은 줄로 돌아오고 수정 접힘이 열린다
	_, loc = v.post(t, base+"/packages/"+v.pkg, url.Values{"name": {"빈 업무"}}, v.admin)
	if !strings.Contains(loc, "err=") || !strings.Contains(loc, "같은 이름") || !strings.Contains(loc, "open="+v.pkg) || !strings.HasSuffix(loc, "#package-"+v.pkg) {
		t.Fatalf("중복 이름: %s", loc)
	}
	open := v.get(t, base+"?err=x&open="+v.pkg, v.admin).Body.String()
	if !regexp.MustCompile(`<details class="text-xs" data-pkg-meta-edit open>`).MatchString(open) {
		t.Fatal("오류 뒤에 수정 접힘이 열려 있지 않다")
	}
	// 예전 일자·배지 전용 폼(이름 없음)은 그대로 동작
	_, loc = v.post(t, base+"/packages/"+v.pkg, url.Values{"plan_start": {""}, "plan_end": {""}, "badge": {"신규"}}, v.admin)
	if !strings.Contains(loc, "ok=pkg-meta") {
		t.Fatalf("예전 폼: %s", loc)
	}
	// 유형 변경 거절 안내(사이트 4곳)
	_, loc = v.post(t, base+"/packages/"+v.pkg, url.Values{"name": {"홍성 교체"}, "track_mode": {"phase"}}, v.admin)
	if !strings.Contains(loc, "사이트 4곳") {
		t.Fatalf("유형 변경 안내: %s", loc)
	}
	// 1단계 저장도 단계 앵커로 돌아온다
	_, loc = v.post(t, base+"/phases/"+v.phase, url.Values{"name": {"잔처리 작업"}, "status": {"planned"}, "is_active": {"1"}}, v.admin)
	if !strings.HasSuffix(loc, "#phase-"+v.phase) {
		t.Fatalf("단계 저장 위치: %s", loc)
	}
	// 계획 저장은 계획 앵커
	_, loc = v.post(t, base+"/plans/"+v.plan, url.Values{"name": {"1차 계획"}, "plan_start": {"2027-04-01"}, "plan_end": {"2027-05-01"}}, v.admin)
	if !strings.HasSuffix(loc, "#plan-"+v.plan) {
		t.Fatalf("계획 저장 위치: %s", loc)
	}
}

func TestHTTP95_183_StatusMoveDeleteRestoreFlow(t *testing.T) {
	v := setup183(t)
	base := "/projects/" + v.id
	// 순서
	_, loc := v.post(t, base+"/packages/"+v.emptyPkg+"/move", url.Values{"dir": {"up"}}, v.admin)
	if !strings.Contains(loc, "ok=pkg-move") {
		t.Fatal(loc)
	}
	// 완료: 미완료 사이트 → 확인 패널, 확정하면 완료
	_, loc = v.post(t, base+"/packages/"+v.pkg+"/status", url.Values{"status": {"done"}}, v.admin)
	if !strings.Contains(loc, "pkg_confirm="+v.pkg) || !strings.Contains(loc, "open_n=4") {
		t.Fatalf("미완료 경고 이동: %s", loc)
	}
	page := v.get(t, base+"?pkg_confirm="+v.pkg+"&open_n=4", v.admin).Body.String()
	if !strings.Contains(page, "data-pkg-confirm") || !strings.Contains(page, "4곳 있습니다") {
		t.Fatal("완료 확인 패널이 없다")
	}
	_, loc = v.post(t, base+"/packages/"+v.pkg+"/status", url.Values{"status": {"done"}, "confirm_open": {"1"}}, v.admin)
	if !strings.Contains(loc, "ok=pkg-status") {
		t.Fatal(loc)
	}
	if !strings.Contains(v.get(t, base, v.admin).Body.String(), "홍성 · 완료") {
		t.Fatal("완료 표시가 화면에 없다")
	}
	// 보류 → 화면 표시
	v.post(t, base+"/packages/"+v.qtyPkg+"/status", url.Values{"status": {"hold"}}, v.admin)
	if !strings.Contains(v.get(t, base, v.admin).Body.String(), "수량 업무 · 보류") {
		t.Fatal("보류 표시가 화면에 없다")
	}
	// 실적 없는 하부 업무 삭제
	_, loc = v.post(t, base+"/packages/"+v.emptyPkg+"/delete", url.Values{}, v.admin)
	if !strings.Contains(loc, "ok=pkg-del") {
		t.Fatal(loc)
	}
	// 계획이 있는 하부 업무는 비활성 + 비활성 목록 + 되돌리기
	_, loc = v.post(t, base+"/packages/"+v.pkg+"/delete", url.Values{}, v.admin)
	if !strings.Contains(loc, "ok=pkg-off") {
		t.Fatalf("비활성: %s", loc)
	}
	body := v.get(t, base+"?ok=pkg-off", v.admin).Body.String()
	if !strings.Contains(body, "data-inactive-pkgs") || !strings.Contains(body, `data-inactive-pkg="`+v.pkg+`"`) || !strings.Contains(body, "비활성으로 두었습니다") {
		t.Fatal("비활성 목록·안내가 없다")
	}
	if strings.Contains(body, `data-package-id="`+v.pkg+`"`) {
		t.Fatal("비활성 하부 업무가 본 목록에 남아 있다")
	}
	// 비활성과 같은 이름의 새 하부 업무는 안내와 함께 막힌다(생성 경로)
	_, loc = v.post(t, base+"/phases/"+v.phase+"/packages", url.Values{"name": {"홍성"}, "track_mode": {"site"}}, v.admin)
	if !strings.Contains(loc, "err=") || !strings.Contains(loc, "비활성 하부 업무가 있습니다") || !strings.Contains(loc, "되돌리거나") {
		t.Fatalf("비활성 이름 중복: %s", loc)
	}
	_, loc = v.post(t, base+"/packages/"+v.pkg+"/restore", url.Values{}, v.admin)
	if !strings.Contains(loc, "ok=pkg-restore") {
		t.Fatal(loc)
	}
	if !strings.Contains(v.get(t, base, v.admin).Body.String(), `data-package-id="`+v.pkg+`"`) {
		t.Fatal("되돌린 하부 업무가 목록에 없다")
	}
	// 다른 하부 업무 번호
	if c, _ := v.post(t, base+"/packages/PK-999/move", url.Values{"dir": {"up"}}, v.admin); c != 404 {
		t.Fatalf("없는 하부 업무 %d", c)
	}
}

func TestHTTP95_183_PlanSitesPageAndQty(t *testing.T) {
	v := setup183(t)
	base := "/projects/" + v.id + "/plans/" + v.plan
	rec := v.get(t, base+"/sites", v.admin)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "홍성고") || !strings.Contains(body, `data-plan-add-form`) || !strings.Contains(body, `name="qty_`+v.sites[0]+`"`) {
		t.Fatalf("계획 사이트 화면 %d", rec.Code)
	}
	// 후보: 2차 계획 사이트(횡성중)와 미배정(청양고), 검색
	if !strings.Contains(body, `data-cand="`+v.sites[2]+`"`) || !strings.Contains(body, `data-cand="`+v.sites[3]+`"`) {
		t.Fatal("후보가 안 보인다")
	}
	if q := v.get(t, base+"/sites?q="+url.QueryEscape("청양"), v.admin).Body.String(); !strings.Contains(q, "청양고") || strings.Contains(q, `data-cand="`+v.sites[2]+`"`) {
		t.Fatal("검색이 안 걸러진다")
	}
	// 같은 수량 적용(선택 없음 → 오류), 선택하면 저장
	_, loc := v.post(t, base+"/qty", url.Values{"apply_same": {"1"}, "same_qty": {"5"}}, v.admin)
	if !strings.Contains(loc, "err=") {
		t.Fatalf("선택 없이 같은 수량: %s", loc)
	}
	_, loc = v.post(t, base+"/qty", url.Values{"apply_same": {"1"}, "same_qty": {"5"}, "site_id": {v.sites[0], v.sites[1]}}, v.admin)
	if !strings.Contains(loc, "ok=qty%3A2") && !strings.Contains(loc, "ok=qty:2") {
		t.Fatalf("같은 수량 저장: %s", loc)
	}
	_, loc = v.post(t, base+"/qty", url.Values{"qty_" + v.sites[0]: {"8"}, "qty_" + v.sites[1]: {""}}, v.admin)
	if !strings.Contains(loc, "qty") {
		t.Fatalf("개별 수량 저장: %s", loc)
	}
	view, _ := v.repo.PlanSites(v.id, v.plan)
	tq := map[string]int{}
	for _, s := range view.Sites {
		tq[s.ID] = s.Target
	}
	if tq[v.sites[0]] != 8 || tq[v.sites[1]] != 5 {
		t.Fatalf("저장된 수량 %+v", tq)
	}
	if _, loc = v.post(t, base+"/qty", url.Values{"qty_" + v.sites[0]: {"0"}}, v.admin); !strings.Contains(loc, "err=") {
		t.Fatalf("0 수량: %s", loc)
	}
	// 담기(옮기기)
	_, loc = v.post(t, base+"/sites", url.Values{"back": {"sites"}, "site_id": {v.sites[3]}}, v.admin)
	if !strings.Contains(loc, "/plans/"+v.plan+"/sites") || !strings.Contains(loc, "ok=add") {
		t.Fatalf("담기: %s", loc)
	}
	// 제외: 사유 없음 → 오류, 사유 있음 → 제외
	if _, loc = v.post(t, base+"/exclude", url.Values{"back": {"sites"}, "site_id": {v.sites[3]}}, v.admin); !strings.Contains(loc, "err=") {
		t.Fatalf("사유 없는 제외: %s", loc)
	}
	_, loc = v.post(t, base+"/exclude", url.Values{"back": {"sites"}, "site_id": {v.sites[3]}, "reason": {"철거"}}, v.admin)
	if !strings.Contains(loc, "ok=exclude") {
		t.Fatalf("제외: %s", loc)
	}
	if !strings.Contains(v.get(t, base+"/sites", v.admin).Body.String(), "제외한 사이트 1곳") {
		t.Fatal("제외한 사이트가 화면에 없다")
	}
	// 다른 사업·계획 번호
	if v.get(t, "/projects/"+v.id+"/plans/PL-999/sites", v.admin).Code != 404 {
		t.Fatal("없는 계획이 404 가 아니다")
	}
}

func TestHTTP95_183_BatchEndpoint(t *testing.T) {
	v := setup183(t)
	items := []map[string]interface{}{
		{"kind": "phase", "id": v.phase, "fields": map[string]string{"name": "잔처리 작업 B", "status": "active", "is_active": "1"}},
		{"kind": "package", "id": v.pkg, "fields": map[string]string{"name": "홍성 B", "next_note": "다음 단계", "track_mode": "site"}},
		{"kind": "plan", "id": v.plan, "fields": map[string]string{"name": "1차 계획 B", "plan_end": "2027-05-20", "extend_reason": "일정 지연"}},
	}
	code, out := v.batch(t, items, v.admin)
	if code != 200 || out["ok"] != true || out["n"].(float64) != 3 {
		t.Fatalf("일괄 저장 %d %+v", code, out)
	}
	body := v.get(t, "/projects/"+v.id, v.admin).Body.String()
	for _, want := range []string{"잔처리 작업 B", "홍성 B", "1차 계획 B"} {
		if !strings.Contains(body, want) {
			t.Fatalf("%q 가 저장 안 됐다", want)
		}
	}
	// 하나가 실패하면 전부 취소, 실패 위치를 알려 준다
	bad := []map[string]interface{}{
		{"kind": "phase", "id": v.phase, "fields": map[string]string{"name": "취소될 이름"}},
		{"kind": "package", "id": v.pkg, "fields": map[string]string{"name": "빈 업무"}},
	}
	code, out = v.batch(t, bad, v.admin)
	if code != 422 || out["ok"] != false || out["index"].(float64) != 1 || out["id"] != v.pkg || !strings.Contains(out["message"].(string), "같은 이름") {
		t.Fatalf("실패 응답 %d %+v", code, out)
	}
	if strings.Contains(v.get(t, "/projects/"+v.id, v.admin).Body.String(), "취소될 이름") {
		t.Fatal("일괄 저장이 실패했는데 앞 항목이 저장됐다")
	}
	// 잘못된 종류·빈 목록
	if c, _ := v.batch(t, []map[string]interface{}{{"kind": "x", "id": "1"}}, v.admin); c != 400 {
		t.Fatalf("알 수 없는 종류 %d", c)
	}
	if c, _ := v.batch(t, nil, v.admin); c != 422 {
		t.Fatalf("빈 목록 %d", c)
	}
}

// 구조 없는 사업의 화면은 바뀌지 않는다(저장 막대·수정 컨트롤 없음).
func TestHTTP95_183_NoStructureUnchanged(t *testing.T) {
	e, _, _, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"구조없음"}, "plan_year": {"2027"}, "status": {"active"}, "project_kind": {"other"},
		"start_date": {"2027-01-01"}, "end_date": {"2027-12-31"}, "color": {"#3B82F6"},
		"org_id": {model.OrgIDLibrary}, "is_paid": {"1"}, "contract_amount": {"1000"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	body := doGetWith(t, e, "/projects/"+id, cookie183(t, "oa", "org_admin", "O01", false)).Body.String()
	if strings.Contains(body, `data-struct-form="package"`) || strings.Contains(body, `data-pkg-actions`) {
		t.Fatal("구조 없는 사업에 하부 업무 컨트롤이 나왔다")
	}
}

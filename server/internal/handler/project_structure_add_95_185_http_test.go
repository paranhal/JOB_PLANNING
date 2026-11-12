package handler

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-185: 1단계 칸의 「+ 하부 업무」 버튼과 단계 안 추가 패널. 맨 아래 인라인 폼은 없다.

func twoPhases185(t *testing.T) (env183, string) {
	t.Helper()
	v := setup183(t)
	if err := v.repo.CreatePhase(&model.ProjectPhase{ProjectID: v.id, OrgID: model.OrgIDLibrary, Name: "구축", Status: model.PhasePlanned}); err != nil {
		t.Fatal(err)
	}
	phases, _ := v.repo.ListPhases(v.id)
	return v, phases[1].PhaseID
}

func pkgCount185(t *testing.T, v env183) int {
	t.Helper()
	list, err := v.repo.PackageChoices(v.id)
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

func TestHTTP95_185_ButtonPanelAndNoBottomForm(t *testing.T) {
	v, ph2 := twoPhases185(t)
	body := v.get(t, "/projects/"+v.id, v.admin).Body.String()
	for _, ph := range []string{v.phase, ph2} {
		if !strings.Contains(body, `data-add-package="`+ph+`"`) {
			t.Fatalf("단계 %s 에 「+ 하부 업무」 버튼이 없다", ph)
		}
		if !strings.Contains(body, `data-add-package-panel="`+ph+`"`) {
			t.Fatalf("단계 %s 에 추가 패널이 없다", ph)
		}
		// 하부 업무 추가 폼은 단계마다 패널 하나뿐(맨 아래 인라인 폼 제거)
		if n := strings.Count(body, `action="/projects/`+v.id+`/phases/`+ph+`/packages"`); n != 1 {
			t.Fatalf("단계 %s 의 하부 업무 추가 폼 %d개 want 1", ph, n)
		}
	}
	// 패널 필드는 기존 폼과 같다
	panel := regexp.MustCompile(`(?s)<form[^>]*data-add-package-panel="` + v.phase + `".*?</form>`).FindString(body)
	for _, want := range []string{`name="name"`, `maxlength="100"`, `name="track_mode"`, `name="badge"`, `maxlength="8"`, `name="plan_start"`, `name="plan_end"`, `하부 업무 추가</button>`, `취소</button>`, `required`} {
		if !strings.Contains(panel, want) {
			t.Fatalf("추가 패널에 %q 가 없다", want)
		}
	}
	// 버튼은 1단계 버튼 줄(삭제 옆)에 있다: 단계 삭제 폼 뒤, 패널 앞
	iDel := strings.Index(body, `/phases/`+v.phase+`/delete"`)
	iBtn := strings.Index(body, `data-add-package="`+v.phase+`"`)
	iPanel := strings.Index(body, `data-add-package-panel="`+v.phase+`"`)
	if !(iDel > 0 && iDel < iBtn && iBtn < iPanel) {
		t.Fatalf("버튼 위치 삭제=%d 버튼=%d 패널=%d", iDel, iBtn, iPanel)
	}
	// 하부 업무가 있는 단계는 안내 문장이 없고, 0개 단계는 안내와 버튼이 한 번 더 있다
	seg := func(ph string) string {
		re := regexp.MustCompile(`(?s)data-phase-id="` + ph + `".*?(?:data-phase-id=|$)`)
		return re.FindString(body)
	}
	if strings.Contains(seg(v.phase), "data-no-packages") {
		t.Fatal("하부 업무가 있는 단계에 없음 안내가 나왔다")
	}
	empty := seg(ph2)
	if !strings.Contains(empty, "data-no-packages") || !strings.Contains(empty, "하부 업무가 없습니다.") || strings.Count(empty, `data-add-package="`+ph2+`"`) != 2 {
		t.Fatalf("0개 단계 안내·버튼: %d", strings.Count(empty, `data-add-package="`+ph2+`"`))
	}
}

func TestHTTP95_185_PermissionHidesButton(t *testing.T) {
	type role struct {
		name string
		mk   func() *http.Cookie
		can  bool
	}
	roles := []role{
		{"비전관리자", func() *http.Cookie { return cookie183(t, "va", "vision_admin", "O01", false) }, true},
		{"조직관리자", func() *http.Cookie { return cookie183(t, "oa", "org_admin", "O01", false) }, true},
		{"PM", func() *http.Cookie { return cookie183(t, "pm-u", "support", "O01", false, model.PermWorkboard) }, true},
		{"수행 담당", func() *http.Cookie { return cookie183(t, "exec-u", "support", "O01", false, model.PermWorkboard) }, false},
		{"일반", func() *http.Cookie { return cookie183(t, "gen-u", "support", "O01", false, model.PermWorkboard) }, false},
		{"읽기 전용", func() *http.Cookie { return cookie183(t, "ro-u", "org_admin", "O01", true) }, false},
		{"C-Level", func() *http.Cookie { return cookie183(t, "cl", "c_level", "O01", false) }, false},
		{"옵저버", func() *http.Cookie { return cookie183(t, "ob", "observer", "O01", false) }, false},
	}
	for _, r := range roles {
		v, ph2 := twoPhases185(t) // 역할마다 새 환경(앞 역할이 만든 하부 업무가 0개 단계를 바꾸지 않게)
		ck := r.mk()
		body := v.get(t, "/projects/"+v.id, ck).Body.String()
		has := strings.Contains(body, "data-add-package") || strings.Contains(body, "data-add-package-panel")
		if has != r.can {
			t.Errorf("%s: 추가 버튼/패널 유무 %v want %v", r.name, has, r.can)
		}
		// 0개 단계의 안내 문장은 누구에게나 보이되 버튼은 권한자에게만
		if !strings.Contains(body, "data-no-packages") || !strings.Contains(body, "하부 업무가 없습니다.") {
			t.Errorf("%s: 0개 단계 안내가 없다", r.name)
		}
		before := pkgCount185(t, v)
		code, _ := v.post(t, "/projects/"+v.id+"/phases/"+ph2+"/packages", url.Values{"name": {"권한시험"}, "track_mode": {"site"}}, ck)
		if (code == 303) != r.can {
			t.Errorf("%s: POST %d", r.name, code)
		}
		if after := pkgCount185(t, v); r.can != (after == before+1) {
			t.Errorf("%s: 추가 건수 %d → %d", r.name, before, after)
		}
	}
}

func TestHTTP95_185_CreateRedirectsToNewRowAndRightPhase(t *testing.T) {
	v, ph2 := twoPhases185(t)
	base := "/projects/" + v.id
	idOf := func(loc string) string {
		m := regexp.MustCompile(`#package-(PK-\d+)$`).FindStringSubmatch(loc)
		if m == nil {
			t.Fatalf("새 하부 업무 앵커가 없다: %s", loc)
		}
		return m[1]
	}
	// 사이트 기준: 두 번째 단계(0개)에 추가 → 그 단계에 생기고, 새 줄로 이동
	code, loc := v.post(t, base+"/phases/"+ph2+"/packages", url.Values{"name": {"현장 조사"}, "track_mode": {"site"}, "badge": {"사전"}}, v.admin)
	if code != 303 || !strings.Contains(loc, "ok=pkg") {
		t.Fatalf("추가: %d %s", code, loc)
	}
	siteID := idOf(loc)
	// 수량만: 첫 번째 단계(이미 하부 업무 있음)
	_, loc = v.post(t, base+"/phases/"+v.phase+"/packages", url.Values{"name": {"장비 수량"}, "track_mode": {"phase"}}, v.admin)
	qtyID := idOf(loc)
	list, _ := v.repo.PackageChoices(v.id) // 사이트형만 나온다
	var phaseOfSite string
	for _, c := range list {
		if c.ID == siteID {
			phaseOfSite = c.Phase
		}
	}
	if phaseOfSite != "구축" {
		t.Fatalf("현장 조사가 %q 단계에 생겼다(구축이어야 한다)", phaseOfSite)
	}
	body := v.get(t, base+"?ok=pkg", v.admin).Body.String()
	row := func(id string) string {
		return regexp.MustCompile(`(?s)id="package-`+id+`".*?(?:id="package-|data-add-package-panel|$)`).FindString(body)
	}
	if !strings.Contains(body, "하부 업무를 추가했습니다") {
		t.Fatal("성공 문구가 없다")
	}
	sr := row(siteID)
	if !strings.Contains(sr, "현장 조사") || !strings.Contains(sr, `data-pkg-sites-link`) || !strings.Contains(sr, `사전`) || strings.Contains(sr, "data-qty-line") {
		t.Fatalf("사이트 기준 줄: 사이트 등록·관리 링크/배지가 없다")
	}
	qr := row(qtyID)
	if !strings.Contains(qr, "장비 수량") || !strings.Contains(qr, "data-qty-line") || !strings.Contains(qr, "수량만 · 목표 수량 미설정") || strings.Contains(qr, `data-pkg-sites-link`) {
		t.Fatalf("수량만 줄: 표기/링크가 다르다")
	}
	// 0개 안내가 사라진다(단계 2 에는 이제 하부 업무가 있다)
	if strings.Contains(body, "data-no-packages") {
		t.Fatal("하부 업무가 생겼는데 없음 안내가 남아 있다")
	}
}

func TestHTTP95_185_ErrorsKeepPanelOpenAndValues(t *testing.T) {
	v, ph2 := twoPhases185(t)
	base := "/projects/" + v.id
	// 비활성 하부 업무 하나 만들기(계획이 있는 홍성을 비활성)
	if c, loc := v.post(t, base+"/packages/"+v.pkg+"/delete", url.Values{}, v.admin); c != 303 || !strings.Contains(loc, "pkg-off") {
		t.Fatalf("비활성 준비: %d %s", c, loc)
	}
	cases := []struct {
		name  string
		form  url.Values
		phase string
		want  string
	}{
		{"이름 비움", url.Values{"name": {"   "}, "track_mode": {"site"}}, ph2, "하부 업무명을 입력하세요"},
		{"이름 101자", url.Values{"name": {strings.Repeat("가", 101)}, "track_mode": {"site"}}, ph2, "100자까지"},
		{"중복", url.Values{"name": {"수량 업무"}, "track_mode": {"site"}}, v.phase, "같은 이름의 하부 업무가 있습니다"},
		{"정규화 중복", url.Values{"name": {"수량(업무)"}, "track_mode": {"site"}}, v.phase, "같은 이름의 하부 업무가 있습니다"},
		{"비활성 중복", url.Values{"name": {"홍성"}, "track_mode": {"phase"}, "badge": {"가"}}, v.phase, "되돌리거나"},
		{"배지 9자", url.Values{"name": {"배지 시험"}, "track_mode": {"site"}, "badge": {"123456789"}}, ph2, "8자까지"},
		{"일자 순서", url.Values{"name": {"일자 시험"}, "track_mode": {"site"}, "plan_start": {"2027-05-02"}, "plan_end": {"2027-05-01"}}, ph2, "err="},
	}
	before := pkgCount185(t, v)
	for _, c := range cases {
		raw := postPhase(t, v.e, base+"/phases/"+c.phase+"/packages", c.form, v.admin)
		code := raw.Code
		loc, _ := url.QueryUnescape(raw.Header().Get("Location"))
		if code != 303 || !strings.Contains(loc, "err=") || !strings.Contains(loc, c.want) {
			t.Errorf("%s: %d %s", c.name, code, loc)
			continue
		}
		if !strings.Contains(loc, "add_open="+c.phase) || !strings.HasSuffix(loc, "#add-package-"+c.phase) {
			t.Errorf("%s: 패널 열림·위치 %s", c.name, loc)
		}
		// 그 주소를 열면 해당 단계 패널만 열려 있고 입력값이 채워져 있다
		page := v.get(t, strings.Split(raw.Header().Get("Location"), "#")[0], v.admin)
		if page.Code != 200 {
			t.Errorf("%s: 오류 화면 %d", c.name, page.Code)
			continue
		}
		b := page.Body.String()
		open := regexp.MustCompile(`data-phase-id="([A-Z]+-\d+)"[^>]*x-data="\{ addOpen: (true|false) \}"`).FindAllStringSubmatch(b, -1)
		for _, m := range open {
			if (m[2] == "true") != (m[1] == c.phase) {
				t.Errorf("%s: 단계 %s 패널 열림=%s", c.name, m[1], m[2])
			}
		}
		if want := strings.TrimSpace(c.form.Get("name")); want != "" && !strings.Contains(b, `value="`+want+`"`) {
			t.Errorf("%s: 입력한 이름이 되돌려 채워지지 않았다", c.name)
		}
		if bd := c.form.Get("badge"); bd != "" && !strings.Contains(b, `value="`+bd+`"`) {
			t.Errorf("%s: 배지가 되돌려 채워지지 않았다", c.name)
		}
	}
	if after := pkgCount185(t, v); after != before {
		t.Fatalf("오류인데 하부 업무가 생겼다: %d → %d", before, after)
	}
	// 수량만 유형 선택이 되돌려 채워진다
	page := v.get(t, base+"?err=x&add_open="+v.phase+"&add_name=a&add_track=phase", v.admin).Body.String()
	if !regexp.MustCompile(`<option value="phase" selected>수량만</option>`).MatchString(page) {
		t.Fatal("유형(수량만) 선택이 되돌려 채워지지 않았다")
	}
}

// 구조 없는 사업·단계가 없는 사업의 화면은 바뀌지 않는다.
func TestHTTP95_185_NoPhasesNoButton(t *testing.T) {
	e, _, _, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"구조없음"}, "plan_year": {"2027"}, "status": {"active"}, "project_kind": {"other"},
		"start_date": {"2027-01-01"}, "end_date": {"2027-12-31"}, "color": {"#3B82F6"},
		"org_id": {model.OrgIDLibrary}, "is_paid": {"1"}, "contract_amount": {"1000"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	body := doGetWith(t, e, "/projects/"+id, cookie183(t, "oa", "org_admin", "O01", false)).Body.String()
	if strings.Contains(body, "data-add-package") {
		t.Fatal("단계가 없는 사업에 하부 업무 추가 버튼이 나왔다")
	}
}

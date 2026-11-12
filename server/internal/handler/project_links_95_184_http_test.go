package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-184: 이동 링크를 버튼(<a class="btn …">)으로 바꾸고, 사이트 등록 화면의 「선택한 사이트 등록」을 안내 문구 옆으로 옮긴다.

// anchorTags 본문에서 <a …> 여는 태그만 모아 돌려준다.
func anchorTags(body string) []string {
	return regexp.MustCompile(`<a\s[^>]*>`).FindAllString(body, -1)
}

// tagWith data-속성을 가진 <a> 태그(처음 것)와 개수.
func tagWith(body, attr string) (string, int) {
	first, n := "", 0
	for _, t := range anchorTags(body) {
		if strings.Contains(t, attr) {
			if first == "" {
				first = t
			}
			n++
		}
	}
	return first, n
}

func mustBtn(t *testing.T, where, tag string, wantHref string) {
	t.Helper()
	if tag == "" {
		t.Errorf("%s: 링크가 없다", where)
		return
	}
	if !strings.Contains(tag, `class="btn `) && !strings.Contains(tag, ` btn `) {
		t.Errorf("%s: 버튼 클래스가 아니다: %s", where, tag)
	}
	if strings.Contains(tag, "text-primary") || strings.Contains(tag, "hover:underline") {
		t.Errorf("%s: 파란 글씨 링크 클래스가 남았다: %s", where, tag)
	}
	if wantHref != "" && !strings.Contains(tag, `href="`+wantHref) {
		t.Errorf("%s: href 가 바뀌었다 want %s: %s", where, wantHref, tag)
	}
}

func TestHTTP95_184_ProjectShowStructureLinksAreButtons(t *testing.T) {
	v, ph2 := twoPhases185(t)
	base := "/projects/" + v.id
	// 구조 복사 원본 후보와 템플릿 목록을 만든다(다른 사업에 구조가 있어야 복사 후보가 나온다).
	other := doForm(t, v.e, "/projects", url.Values{
		"name": {"복사원본사업"}, "plan_year": {"2027"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2027-01-01"}, "end_date": {"2027-12-31"}, "color": {"#3B82F6"},
		"org_id": {model.OrgIDLibrary}, "is_paid": {"1"}, "contract_amount": {"1000"},
	})
	otherID := strings.TrimPrefix(strings.Split(other.Header().Get("Location"), "?")[0], "/projects/")
	if err := v.repo.CreatePhase(&model.ProjectPhase{ProjectID: otherID, OrgID: model.OrgIDLibrary, Name: "착수", Status: model.PhaseActive}); err != nil {
		t.Fatal(err)
	}
	if ph, _ := v.repo.ListPhases(otherID); len(ph) > 0 {
		if _, err := v.repo.CreatePackage(ph[0].PhaseID, "현장 조사", model.TrackModeSite); err != nil {
			t.Fatal(err)
		}
	}
	if c, loc := v.post(t, base+"/structure/templates", url.Values{"name": {"기본 템플릿"}, "project_kind": {"build"}}, v.admin); c != 303 {
		t.Fatalf("템플릿 저장 %d %s", c, loc)
	}
	body := v.get(t, base+"?copy_q="+url.QueryEscape("복사원본"), v.admin).Body.String()

	// 1) 표에 있는 항목 — 주소·data-* 는 그대로, 모양만 버튼
	tag, n := tagWith(body, "data-changes-link")
	mustBtn(t, "변경내역", tag, base+"/changes")
	if n != 1 {
		t.Errorf("변경내역 %d개", n)
	}
	tag, n = tagWith(body, `data-phase-sites-link`)
	mustBtn(t, "1단계 사이트 등록·관리", tag, base+"/phases/"+v.phase+"/sites")
	if n != 2 || !strings.Contains(tag, "") { // 단계 2개
		t.Errorf("1단계 사이트 링크 %d개 want 2", n)
	}
	tag, _ = tagWith(body, `data-pkg-sites-link`)
	mustBtn(t, "하부 업무 사이트 등록·관리", tag, base+"/packages/")
	tag, _ = tagWith(body, `data-plan-sites-link`)
	mustBtn(t, "계획 사이트·수량 관리", tag, base+"/plans/"+v.plan+"/sites")
	tag, _ = tagWith(body, `data-register-shortcut`)
	mustBtn(t, "사업에 사이트 등록", tag, base+"/sites")
	// 담당자 관리 / 기본 단계로 채우기 / 구조 복사 원본 / 템플릿 이름
	// 담당자 관리: 같은 주소(/edit)를 쓰는 헤더 「수정」 버튼과 구분하려고 글자로 찾는다
	if m := regexp.MustCompile(`<a [^>]*>담당자 관리</a>`).FindString(body); m == "" {
		t.Error("담당자 관리 버튼이 없다")
	} else {
		mustBtn(t, "담당자 관리", regexp.MustCompile(`<a [^>]*>`).FindString(m), base+"/edit")
	}
	for _, c := range []struct{ name, href string }{
		{"복사 원본", base + "?copy_from=" + otherID}, {"템플릿", base + "?tpl="},
	} {
		found := ""
		for _, a := range anchorTags(body) {
			if strings.Contains(a, `href="`+c.href) {
				found = a
				break
			}
		}
		mustBtn(t, c.name, found, c.href)
	}
	// 구조 복사 원본·템플릿 목록은 줄바꿈 가능한 감싸개 안의 버튼
	if !strings.Contains(body, `data-copy-hits`) || !strings.Contains(body, `data-template-list`) {
		t.Error("복사 원본·템플릿 목록 감싸개가 없다")
	}

	// 2) 오늘 / 이번 주 / 이번 달: 선택된 것만 btn-primary + aria-current
	for _, which := range []string{"today", "week", "month"} {
		page := v.get(t, base+"?report="+which, v.admin).Body.String()
		var cur []string
		for _, a := range anchorTags(page) {
			m := regexp.MustCompile(`href="[^"]*\?report=(today|week|month)#project-phases"`).FindStringSubmatch(a)
			if m == nil {
				continue
			}
			isCur := strings.Contains(a, `aria-current="true"`)
			isPrimary := strings.Contains(a, "btn-primary")
			if isCur != (m[1] == which) || isPrimary != (m[1] == which) {
				t.Errorf("report=%s: %s 토글 상태 (current=%v primary=%v)", which, m[1], isCur, isPrimary)
			}
			if !strings.Contains(a, `class="btn btn-sm `) {
				t.Errorf("토글이 버튼이 아니다: %s", a)
			}
			cur = append(cur, m[1])
		}
		if len(cur) != 3 {
			t.Fatalf("토글 %d개 want 3", len(cur))
		}
	}
	// 3) 구역 머리: 변경내역 설명 문구는 유지
	if !strings.Contains(body, "마감 연장 이력을 봅니다") {
		t.Error("변경내역 옆 설명이 사라졌다")
	}
	_ = ph2
}

// 기본 단계로 채우기: 단계가 없는 사업(빌드형)에서 버튼으로 나온다.
func TestHTTP95_184_FillDefaultPhasesIsButton(t *testing.T) {
	e, _, _, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"단계없음"}, "plan_year": {"2027"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2027-01-01"}, "end_date": {"2027-12-31"}, "color": {"#3B82F6"},
		"org_id": {model.OrgIDLibrary}, "is_paid": {"1"}, "contract_amount": {"1000"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	body := doGetWith(t, e, "/projects/"+id, cookie183(t, "oa", "org_admin", "O01", false)).Body.String()
	var found string
	for _, a := range anchorTags(body) {
		if strings.Contains(a, `href="/projects/`+id+`/phases/preview"`) {
			found = a
		}
	}
	mustBtn(t, "기본 단계로 채우기", found, "/projects/"+id+"/phases/preview")
}

func TestHTTP95_184_ReadOnlyViewerLinks(t *testing.T) {
	v, _ := twoPhases185(t)
	viewer := cookie183(t, "gen-u", "support", "O01", false, model.PermWorkboard) // 읽기만 하는 일반 사용자
	body := v.get(t, "/projects/"+v.id, viewer).Body.String()
	tag, _ := tagWith(body, "data-phase-sites-link")
	mustBtn(t, "읽기용 사이트 보기", tag, "/projects/"+v.id+"/phases/"+v.phase+"/sites")
	tag, _ = tagWith(body, "data-plan-sites-link")
	mustBtn(t, "읽기용 계획 사이트 보기", tag, "")
	if !strings.Contains(body, `data-plan-sites-link>사이트 보기</a>`) || !strings.Contains(body, `data-phase-sites-link>사이트 보기</a>`) {
		t.Error("권한 없는 사용자는 「사이트 보기」 버튼이어야 한다")
	}
	// 수정 권한 없으면 담당자 관리·기본 단계 채우기는 없다(권한 조건 유지)
	if strings.Contains(body, "담당자 관리") {
		t.Error("권한 없는 사용자에게 담당자 관리 버튼이 보인다")
	}
}

func TestHTTP95_184_ShowHeaderButtonsAndNoLeftoverBlueLinks(t *testing.T) {
	v, _ := twoPhases185(t)
	body := v.get(t, "/projects/"+v.id, v.admin).Body.String()
	// 연차·사이트 등록은 헤더 버튼 줄(오른쪽 flex 블록)에 있다
	years, n := tagWith(body, "data-project-years-link")
	mustBtn(t, "연차", years, "/projects/"+v.id+"/years")
	if n != 1 {
		t.Errorf("연차 %d개", n)
	}
	sites, n2 := tagWith(body, "data-project-sites-link")
	mustBtn(t, "사이트 등록", sites, "/projects/"+v.id+"/sites")
	if n2 != 2 { // 헤더 + 사이트 카드 아래
		t.Errorf("사이트 등록 버튼 %d개 want 2", n2)
	}
	iYears := strings.Index(body, "data-project-years-link")
	iWork := strings.Index(body, "이 사업으로 업무 등록")
	iTitle := strings.Index(body, `<h2 class="text-xl font-bold text-ink mt-1">`)
	if !(iTitle > 0 && iTitle < iYears && iYears < iWork) {
		t.Errorf("연차 버튼이 제목 뒤 버튼 줄(업무 등록 앞)에 없다 title=%d years=%d work=%d", iTitle, iYears, iWork)
	}
	// 수행 구조 구역 안에는 「파란 글씨 링크」 클래스가 남지 않는다
	i := strings.Index(body, `id="project-phases"`)
	if i < 0 {
		t.Fatal("수행 구조 구역이 없다")
	}
	section := body[i:]
	for _, a := range anchorTags(section) {
		if strings.Contains(a, `class="text-primary`) || strings.Contains(a, "hover:underline") {
			t.Errorf("수행 구조 구역에 파란 글씨 링크가 남았다: %s", a)
		}
	}
	// 참조 링크(영업 번호·건수)는 글자 링크로 남는다 — 버튼으로 바꾸지 않는다(95-184 §2)
	if !strings.Contains(body, `class="font-semibold text-primary hover:underline"`) {
		t.Error("건수 참조 링크가 사라졌다(바꾸면 안 된다)")
	}
}

func TestHTTP95_184_SitesPanelAndBackLinks(t *testing.T) {
	v, _ := twoPhases185(t)
	body := v.get(t, "/projects/"+v.id+"/sites-panel", v.admin).Body.String()
	var a string
	for _, t := range anchorTags(body) {
		if strings.Contains(t, `/sites"`) {
			a = t
		}
	}
	mustBtn(t, "사이트 패널 사이트 등록", a, "/projects/"+v.id+"/sites")
	// 사업 사이트 등록 화면·변경내역 화면의 뒤로가기
	sites := v.get(t, "/projects/"+v.id+"/sites", v.admin).Body.String()
	tag := ""
	for _, a := range anchorTags(sites) {
		if strings.Contains(a, `href="/projects/`+v.id+`"`) {
			tag = a
			break
		}
	}
	mustBtn(t, "‹ 이전 화면", tag, "/projects/"+v.id)
	changes := v.get(t, "/projects/"+v.id+"/changes", v.admin).Body.String()
	tag = ""
	for _, a := range anchorTags(changes) {
		if strings.Contains(a, `#project-phases"`) {
			tag = a
			break
		}
	}
	mustBtn(t, "변경내역 ‹ 수행 구조로", tag, "/projects/"+v.id)
	if !strings.Contains(changes, "‹ 수행 구조로") {
		t.Error("변경내역 뒤로가기 글자")
	}
}

// 사이트 등록 화면: 「선택한 사이트 등록」이 안내 문구 옆으로 가고 form 속성으로 표 폼과 연결된다.
func TestHTTP95_184_StructureSitesAddSelectedBesideSummary(t *testing.T) {
	v, ph2 := twoPhases185(t)
	base := "/projects/" + v.id
	for _, c := range []struct{ name, path string }{
		{"1단계", base + "/phases/" + ph2 + "/sites"},
		{"2단계", base + "/packages/" + v.pkg + "/sites"},
	} {
		page := v.get(t, c.path, v.admin)
		if page.Code != 200 {
			t.Fatalf("%s: %d", c.name, page.Code)
		}
		b := page.Body.String()
		iRow := strings.Index(b, "data-site-summary-row")
		iSummary := strings.Index(b, "data-site-summary>")
		iAdd := strings.Index(b, "data-add-selected")
		iAll := strings.Index(b, "data-add-all-form")
		iTable := strings.Index(b, `id="site-form"`)
		iRemove := strings.Index(b, "data-remove-selected")
		if !(iRow > 0 && iRow < iSummary && iSummary < iAdd && iAdd < iAll && iAll < iTable && iTable < iRemove) {
			t.Fatalf("%s: 순서 row=%d summary=%d add=%d all=%d table=%d remove=%d", c.name, iRow, iSummary, iAdd, iAll, iTable, iRemove)
		}
		btn := regexp.MustCompile(`<button[^>]*data-add-selected[^>]*>`).FindString(b)
		for _, want := range []string{`type="submit"`, `form="site-form"`, `btn btn-primary btn-sm`, `:disabled="n === 0"`} {
			if !strings.Contains(btn, want) {
				t.Errorf("%s: 선택 등록 버튼에 %q 가 없다: %s", c.name, want, btn)
			}
		}
		// 표 폼 안에는 더 이상 등록 버튼이 없고(중복 방지), 제외 영역은 표 아래에 남는다
		form := regexp.MustCompile(`(?s)<form[^>]*id="site-form".*?</form>`).FindString(b)
		if strings.Contains(form, "data-add-selected") {
			t.Errorf("%s: 표 폼 안에 등록 버튼이 남아 있다", c.name)
		}
		if !strings.Contains(form, "data-remove-selected") || !strings.Contains(form, "data-remove-reason") {
			t.Errorf("%s: 제외 영역이 표 폼 안에 없다", c.name)
		}
		// 표 폼의 체크박스가 그대로 제출 대상(site_id)이다
		if !strings.Contains(form, `name="site_id"`) || !strings.Contains(form, `name="q"`) {
			t.Errorf("%s: 표 폼에 site_id/q 숨은 입력이 없다", c.name)
		}
		// 이동 링크가 버튼이다
		back, _ := tagWith(b, "")
		_ = back
		var reg, shortcut string
		for _, a := range anchorTags(b) {
			switch {
			case strings.Contains(a, "data-sites-register-link"):
				reg = a
			case strings.Contains(a, "data-register-shortcut"):
				shortcut = a
			}
		}
		mustBtn(t, c.name+" 사업 사이트 등록", reg, base+"/sites")
		if c.name == "1단계" {
			mustBtn(t, c.name+" 사업에 사이트 등록", shortcut, base+"/sites")
		} else if shortcut != "" {
			t.Errorf("2단계 화면에 1단계용 바로가기가 나왔다")
		}
		if !strings.Contains(b, "‹ 수행 구조로") {
			t.Errorf("%s: 뒤로가기 글자", c.name)
		}
	}
}

func TestHTTP95_184_StructureSitesNoResultsAndPermissions(t *testing.T) {
	v, ph2 := twoPhases185(t)
	path := "/projects/" + v.id + "/phases/" + ph2 + "/sites"
	// 조회 결과 없음: 표 폼이 없으니 「선택한 사이트 등록」은 만들지 않는다
	b := v.get(t, path+"?q="+url.QueryEscape("없는사이트"), v.admin).Body.String()
	if strings.Contains(b, "data-add-selected") || strings.Contains(b, `id="site-form"`) {
		t.Error("조회 결과가 없는데 선택 등록 버튼/표 폼이 있다")
	}
	if !strings.Contains(b, "data-add-all-form") {
		t.Error("조회 결과 없음에서도 전체 등록 폼 자리는 유지(비활성)")
	}
	// 권한 없는 사용자: 두 버튼 모두 안 보인다
	for _, c := range []struct {
		name string
		ck   *http.Cookie
	}{
		{"일반", cookie183(t, "gen-u", "support", "O01", false, model.PermWorkboard)},
		{"읽기 전용", cookie183(t, "ro-u", "org_admin", "O01", true)},
		{"수행 담당", cookie183(t, "exec-u", "support", "O01", false, model.PermWorkboard)},
	} {
		r := v.get(t, path, c.ck)
		if r.Code != 200 {
			t.Errorf("%s: 화면 %d", c.name, r.Code)
			continue
		}
		body := r.Body.String()
		if strings.Contains(body, "data-add-selected") || strings.Contains(body, "data-add-all-form") || strings.Contains(body, "data-remove-selected") {
			t.Errorf("%s: 등록·제외 버튼이 보인다", c.name)
		}
	}
}

func TestHTTP95_184_StructureSitesPagingAndPlanSitesPageButtons(t *testing.T) {
	v, ph2 := twoPhases185(t)
	// 100곳 넘게 만들어 쪽 이동 버튼을 띄운다
	for i := 0; i < 120; i++ {
		if _, err := v.wb.DB().Exec(`INSERT INTO project_sites (id, project_id, site_name, region, status, is_active, org_id) VALUES (?,?,?,?,'planned',1,?)`,
			fmt.Sprintf("PS-X%03d", i), v.id, fmt.Sprintf("대량사이트%03d", i), "충남", model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	path := "/projects/" + v.id + "/phases/" + ph2 + "/sites"
	p1 := v.get(t, path, v.admin).Body.String()
	var next string
	for _, a := range anchorTags(p1) {
		if strings.Contains(a, "다음") || strings.Contains(a, "page=2") {
			next = a
		}
	}
	mustBtn(t, "다음 ›", next, path+"?q=")
	if !strings.Contains(next, "page=2") {
		t.Errorf("다음 쪽 주소: %s", next)
	}
	p2 := v.get(t, path+"?page=2", v.admin).Body.String()
	var prev string
	for _, a := range anchorTags(p2) {
		if strings.Contains(a, "page=1") {
			prev = a
		}
	}
	mustBtn(t, "‹ 이전", prev, path+"?q=")
	if !strings.Contains(p2, "‹ 이전") {
		t.Error("이전 글자")
	}

	// 계획 사이트·수량 화면: 뒤로가기·문장 속 사이트 등록·관리·쪽 이동
	pl := v.get(t, "/projects/"+v.id+"/plans/"+v.plan+"/sites", v.admin).Body.String()
	var back, pkgLink string
	for _, a := range anchorTags(pl) {
		if strings.Contains(a, `#plan-`+v.plan) {
			back = a
		}
		if strings.Contains(a, "data-pkg-sites-link") {
			pkgLink = a
		}
	}
	mustBtn(t, "계획 화면 ‹ 수행 구조로", back, "/projects/"+v.id)
	mustBtn(t, "계획 화면 사이트 등록·관리", pkgLink, "/projects/"+v.id+"/packages/"+v.pkg+"/sites")
	if !strings.Contains(pl, "하부 업무에 사이트를 더 등록하려면") || !strings.Contains(pl, "에서 하세요.") {
		t.Error("문장은 두고 링크만 버튼으로 빼야 한다")
	}
	for _, a := range anchorTags(pl) {
		if strings.Contains(a, "text-primary") {
			t.Errorf("계획 사이트 화면에 파란 글씨 링크가 남았다: %s", a)
		}
	}
}

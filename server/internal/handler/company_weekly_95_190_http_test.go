package handler

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-190: 주간업무보고 — 작성 중이면 기본 편집, 고정 저장 막대, 저장 뒤 편집 유지, 넣을 줄 없음 안내, 엑셀·HWPX 저장.

type env190 struct {
	e   *echo.Echo
	h   *Handler
	key string
}

func setup190(t *testing.T, withRow bool) env190 {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw190.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeeklyUI(e, h)
	e.GET("/stats/company-weekly/export-hwpx", h.Stats.ExportWeeklyReportSummary, h.Auth.AuthMiddleware)
	v := env190{e: e, h: h}
	if withRow {
		k, err := h.Stats.repo.InsertWeeklyReportRow(repository.WeeklyReportRowInput{OrgID: "O03", DisplayName: "시험 사업", Kind: "manual", SortOrder: 1, Active: true})
		if err != nil {
			t.Fatal(err)
		}
		v.key = k
	}
	return v
}

func (v env190) do(t *testing.T, method, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	v.e.ServeHTTP(rec, req)
	return rec
}

const w190 = "/stats/company-weekly?week=2026-10-19&org=O03"

func TestHTTP95_190_DefaultEditWhenDraft(t *testing.T) {
	v := setup190(t, true)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	// edit 값이 없으면 작성 중(저장 전 포함)이고 줄이 있으면 편집으로 열린다
	body := v.do(t, "GET", w190, nil, oa).Body.String()
	if !strings.Contains(body, "data-weekly-form") || !strings.Contains(body, "data-weekly-savebar") {
		t.Fatal("기본 편집 + 고정 저장 막대가 없다")
	}
	if !strings.Contains(body, "편집 끝내기") {
		t.Fatal("편집 끝내기 링크가 없다")
	}
	// edit=0 은 읽기
	body = v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String()
	if strings.Contains(body, "data-weekly-form") {
		t.Fatal("edit=0 인데 편집 모드")
	}
	// 팀원·읽기 전용·C-Level 은 편집 칸이 없다
	for _, ck := range []*http.Cookie{jwtCookieOrg(t, model.RoleTech, "O03"), jwtCookieRole(t, model.RoleCLevel)} {
		if b := v.do(t, "GET", w190, nil, ck).Body.String(); strings.Contains(b, "data-weekly-form") {
			t.Fatal("권한 없는 사용자에게 편집 칸")
		}
	}
	// 확정되면 기본은 읽기, edit=1 로 열 수 있다
	if r := v.do(t, "POST", "/stats/company-weekly/ui-confirm", url.Values{"week": {"2026-10-19"}, "org": {"O03"}}, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("확정 %d", r.Code)
	}
	body = v.do(t, "GET", w190, nil, oa).Body.String()
	if strings.Contains(body, "data-weekly-form") {
		t.Fatal("확정인데 기본 편집")
	}
	if !strings.Contains(v.do(t, "GET", w190+"&edit=1", nil, oa).Body.String(), "data-weekly-form") {
		t.Fatal("edit=1 인데 편집이 안 열린다")
	}
}

func TestHTTP95_190_NavLinksAndSaveKeepEdit(t *testing.T) {
	v := setup190(t, true)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	body := v.do(t, "GET", w190, nil, oa).Body.String()
	// 기본 편집일 때 주차 이동 링크에는 edit 가 붙지 않는다(서버 기본에 맡김)
	for _, m := range regexp.MustCompile(`href="(/stats/company-weekly\?week=[^"]*)"`).FindAllStringSubmatch(body, -1) {
		if strings.Contains(m[1], "edit=") && !strings.Contains(m[1], "&edit=0") && !strings.Contains(m[1], "&edit=1") {
			t.Fatalf("이상한 edit: %s", m[1])
		}
	}
	// edit=0 로 보고 있으면 이동 링크가 edit=0 을 이어 간다
	ro := v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String()
	for _, key := range []string{"data-weekly-nav"} {
		if !strings.Contains(ro, key) {
			t.Fatalf("팀 탭 표식 %s 없음", key)
		}
	}
	if n := strings.Count(ro, "&edit=0\""); n < 3 {
		t.Fatalf("이전·다음·이번 주 링크에 edit=0 이 이어지지 않는다(%d)", n)
	}
	// 저장 폼은 edit=1 을 보내고, 저장 뒤 편집을 유지한다
	if !regexp.MustCompile(`(?s)data-weekly-form.*?name="edit" value="1"`).MatchString(body) {
		t.Fatal("저장 폼에 edit=1 이 없다")
	}
	form := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "edit": {"1"}, "touch_" + v.key: {"1"}, "done_" + v.key: {"점검 완료"}, "plan_" + v.key: {"설치 예정"}}
	r := v.do(t, "POST", "/stats/company-weekly/save", form, oa)
	if r.Code != http.StatusSeeOther || !strings.Contains(r.Header().Get("Location"), "edit=1") {
		t.Fatalf("저장 뒤 편집 유지 %d %s", r.Code, r.Header().Get("Location"))
	}
	after := v.do(t, "GET", r.Header().Get("Location"), nil, oa).Body.String()
	if !strings.Contains(after, "점검 완료") || !strings.Contains(after, "설치 예정") {
		t.Fatal("저장한 내용이 화면에 없다")
	}
	// edit 없이 저장하면 읽기로(기존 호환)
	form.Del("edit")
	r = v.do(t, "POST", "/stats/company-weekly/save", form, oa)
	if strings.Contains(r.Header().Get("Location"), "edit=1") {
		t.Fatal("edit 없이 저장했는데 편집 유지")
	}
}

func TestHTTP95_190_NoLinesNotice(t *testing.T) {
	v := setup190(t, false)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	// 줄이 없고 저장된 보고서가 없으면 기존 안내
	body := v.do(t, "GET", w190, nil, oa).Body.String()
	if strings.Contains(body, "data-weekly-savebar") {
		t.Fatal("줄이 없는데 저장 막대")
	}
	// 보고서는 있는데 줄이 없는 상태에서 edit=1 → 넣을 줄 없음 안내(저장 막대 없음, 다시 집계만)
	if r := v.do(t, "POST", "/stats/company-weekly/ui-confirm", url.Values{"week": {"2026-10-19"}, "org": {"O03"}}, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("확정 %d", r.Code)
	}
	b := v.do(t, "GET", w190+"&edit=1", nil, oa).Body.String()
	if !strings.Contains(b, "data-weekly-no-lines") {
		t.Fatalf("넣을 줄 없음 안내가 없다")
	}
	if strings.Contains(b, "data-weekly-savebar") || !strings.Contains(b, `name="reaggregate"`) {
		t.Fatal("넣을 줄이 없으면 저장 막대 없이 다시 집계만 있어야 한다")
	}
}

func TestHTTP95_190_ExportExcelAndHWPX(t *testing.T) {
	v := setup190(t, true)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	form := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "touch_" + v.key: {"1"}, "done_" + v.key: {"금주 점검 1건"}, "plan_" + v.key: {"차주 설치"}}
	if r := v.do(t, "POST", "/stats/company-weekly/save", form, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", r.Code)
	}
	x := v.do(t, "GET", "/stats/company-weekly/export?week=2026-10-19&org=O03&scope=team", nil, oa)
	if x.Code != 200 || !strings.Contains(x.Header().Get("Content-Type"), "spreadsheetml") {
		t.Fatalf("엑셀 %d %s", x.Code, x.Header().Get("Content-Type"))
	}
	h := v.do(t, "GET", "/stats/company-weekly/export-hwpx?week=2026-10-19&org=O03&scope=team", nil, oa)
	if h.Code != 200 {
		t.Fatalf("HWPX %d %s", h.Code, h.Body.String())
	}
	if cd := h.Header().Get("Content-Disposition"); !strings.Contains(cd, ".hwpx") {
		t.Fatalf("파일 이름 %s", cd)
	}
	zr, err := zip.NewReader(bytes.NewReader(h.Body.Bytes()), int64(h.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var sec string
	for _, f := range zr.File {
		if f.Name == "Contents/section0.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			sec = string(b)
		}
	}
	for _, want := range []string{"시험 사업", "금주 추진내역", "금주 점검 1건", "차주 추진계획", "차주 설치", "SI"} {
		if !strings.Contains(sec, want) {
			t.Fatalf("HWPX 에 %q 없음", want)
		}
	}
	// 타 조직 / 전사 통합 권한 / 팀원
	if r := v.do(t, "GET", "/stats/company-weekly/export-hwpx?week=2026-10-19&org=O01&scope=team", nil, oa); r.Code != http.StatusForbidden && r.Code != http.StatusNotFound {
		t.Fatalf("타 조직 HWPX %d", r.Code)
	}
	if r := v.do(t, "GET", "/stats/company-weekly/export-hwpx?week=2026-10-19&org=O03&scope=all", nil, oa); r.Code != http.StatusForbidden {
		t.Fatalf("전사 통합 HWPX 권한 %d", r.Code)
	}
	// 줄이 하나도 없으면 오류 안내로 돌아온다(빈 파일을 내려주지 않는다)
	v2 := setup190(t, false)
	e := v2.do(t, "GET", "/stats/company-weekly/export-hwpx?week=2026-10-19&org=O03&scope=team", nil, oa)
	if e.Code != http.StatusSeeOther || !strings.Contains(e.Header().Get("Location"), "err=") {
		t.Fatalf("빈 보고서 %d %s", e.Code, e.Header().Get("Location"))
	}
	// 화면 메뉴에 두 저장 링크
	body := v.do(t, "GET", w190, nil, oa).Body.String()
	if !strings.Contains(body, "data-export-hwpx") || !strings.Contains(body, "/stats/company-weekly/export?") {
		t.Fatal("엑셀·HWPX 저장 링크가 화면에 없다")
	}
}

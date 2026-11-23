package handler

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// 95-219: 고객 등록·수정 폼의 업종 다중 선택(체크박스 여러 값 → 고정 순서 쉼표 저장).
func TestCustomerIndustryMultiForm95_219(t *testing.T) {
	e, db, repo, ck := newCustomerShare2EHTTP(t)
	h2 := New(db)
	e.GET("/customers/:id/edit", h2.Customer.Edit, h2.Auth.AuthMiddleware)
	doForm2E := func(t *testing.T, e *echo.Echo, path string, f url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(f.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(ck)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	doGet2E := func(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) string {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(ck)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	post := func(path string, f url.Values) int {
		return doForm2E(t, e, path, f, ck.oa01).Code
	}
	industryOf := func(name string) string {
		var v string
		_ = db.QueryRow(`SELECT COALESCE(industry,'') FROM customers WHERE org_name=?`, name).Scan(&v)
		return v
	}
	base := func(name string, inds ...string) url.Values {
		f := url.Values{"org_name": {name}, "official_name": {name}, "confirm_dup": {"1"}}
		for _, i := range inds {
			f.Add("industry", i)
		}
		return f
	}
	// 신규: 0개(필수 검사는 현행 유지 — 하지 않는다)·1개·2개·4개
	for _, c := range []struct {
		name string
		in   []string
		want string
	}{
		{"고객영", nil, ""},
		{"고객일", []string{"학교"}, "학교"},
		{"고객이", []string{"학교", "도서관"}, "도서관,학교"}, // 고정 순서
		{"고객사", []string{"민간", "공공기관", "학교", "도서관"}, "도서관,학교,공공기관,민간"},
		{"고객별칭", []string{"school", "library"}, "도서관,학교"},
	} {
		if code := post("/customers", base(c.name, c.in...)); code != http.StatusSeeOther {
			t.Fatalf("%s 등록 %d", c.name, code)
		}
		if got := industryOf(c.name); got != c.want {
			t.Errorf("%s 저장 %q want %q", c.name, got, c.want)
		}
	}
	// 수정 화면: 저장된 다중·목록 밖 값이 선택 상태로 복원된다(수정만 눌러도 목록 밖 값 보존)
	if _, err := db.Exec(`UPDATE customers SET industry='도서관,학교,교육청' WHERE org_name='고객이'`); err != nil {
		t.Fatal(err)
	}
	var id string
	_ = db.QueryRow(`SELECT customer_id FROM customers WHERE org_name='고객이'`).Scan(&id)
	body := html.UnescapeString(doGet2E(t, e, "/customers/"+id+"/edit", ck.oa01))
	for _, want := range []string{`data-industry-multi`, `"selected":["도서관","학교","교육청"]`, `"extras":["교육청"]`, `aria-multiselectable="true"`, `data-industry-option`} {
		if !strings.Contains(body, want) {
			t.Errorf("수정 화면에 %q 없음", want)
		}
	}
	// 체크 상태 그대로 제출 → 목록 밖 값 포함해 보존
	if code := post("/customers/"+id+"/update", base("고객이", "교육청", "학교", "도서관")); code != http.StatusSeeOther {
		t.Fatalf("수정 %d", code)
	}
	if got := industryOf("고객이"); got != "도서관,학교,교육청" {
		t.Errorf("수정 뒤 업종 %q", got)
	}
	// 목록·상세 표시: 업종마다 따로 링크, 상세는 쉼표 문자열
	list := doGet2E(t, e, "/customers?industry=학교", ck.oa01)
	if !strings.Contains(list, "고객이") || !strings.Contains(list, `data-industry-note`) {
		t.Error("업종 필터(포함)로 목록에 다중 업종 고객이 나오고 안내 문구가 있어야 한다")
	}
	if strings.Count(list, `data-industry-link`) < 3 {
		t.Error("목록에 업종별 링크가 보여야 한다")
	}
	show := doGet2E(t, e, "/customers/"+id, ck.oa01)
	if !strings.Contains(show, "도서관, 학교, 교육청") {
		t.Error("상세에 업종이 쉼표로 이어 보여야 한다")
	}
	// 오류로 화면이 다시 그려질 때(같은 이름 고객 경고) 고른 업종이 그대로 남는다
	dup := base("고객이", "도서관", "민간")
	dup.Del("confirm_dup")
	rec := doForm2E(t, e, "/customers", dup, ck.oa01)
	if rec.Code != http.StatusOK {
		t.Fatalf("같은 이름 경고 화면 %d", rec.Code)
	}
	if b := html.UnescapeString(rec.Body.String()); !strings.Contains(b, `"selected":["도서관","민간"]`) {
		t.Error("오류로 다시 그려진 화면에서 선택한 업종이 유지되지 않았다")
	}
	// 외부 연동 API 형식 불변: industry 는 문자열(쉼표 연결)
	got, err := repo.GetByID(id)
	if err != nil || got == nil || got.Industry != "도서관,학교,교육청" {
		t.Errorf("API 가 읽는 고객 industry=%v", got)
	}
	_ = model.IndustryOptions
}

package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func newSalesServer(t *testing.T) *echo.Echo {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "sales_http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/sales", h.Sales.List)
	g.GET("/sales/new", h.Sales.New)
	g.POST("/sales", h.Sales.Create)
	g.GET("/sales/activities", h.Sales.Activities)
	g.GET("/sales/pipeline", h.Sales.Pipeline)
	g.GET("/sales/:id", h.Sales.Show)
	g.GET("/sales/:id/edit", h.Sales.Edit)
	g.POST("/sales/:id", h.Sales.Update)
	g.POST("/sales/:id/stage", h.Sales.ChangeStage)
	g.POST("/sales/:id/delete", h.Sales.Delete)
	g.GET("/projects/new", h.Project.New)
	return e
}

func salesIDFromRedirect(t *testing.T, loc string) string {
	t.Helper()
	path := strings.Split(loc, "?")[0]
	id := strings.TrimPrefix(path, "/sales/")
	if id == "" || strings.Contains(id, "/") {
		t.Fatalf("sales id 없음: %q", loc)
	}
	return id
}

func TestSalesHTTP_NameOnlyStageOverrideWon(t *testing.T) {
	e := newSalesServer(t)

	rec := doForm(t, e, "/sales", url.Values{
		"name":              {"세종시 도서관 RFID 증설(가칭)"},
		"is_tentative_name": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("사업명만 저장 실패 status=%d body=%s", rec.Code, rec.Body.String())
	}
	id := salesIDFromRedirect(t, rec.Header().Get("Location"))

	show := doGet(t, e, "/sales/"+id)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "정보 입수") || !strings.Contains(body, "10%") {
		t.Fatalf("기본 단계·확도 미표시: %s", body[0:min(400, len(body))])
	}
	if !strings.Contains(body, "정보 확정도 0/4") {
		t.Fatalf("확정도 미표시")
	}

	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"submit"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=stage") {
		t.Fatalf("단계 변경: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	show = doGet(t, e, "/sales/"+id)
	body = show.Body.String()
	if !strings.Contains(body, "제안서 제출") || !strings.Contains(body, "50%") {
		t.Fatalf("단계 변경 후 확도 미반영: %s", body[0:min(500, len(body))])
	}

	rec = doForm(t, e, "/sales/"+id, url.Values{
		"name":              {"세종시 도서관 RFID 증설(가칭)"},
		"is_tentative_name": {"1"},
		"probability":       {"35"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("확도 수동 저장 status=%d", rec.Code)
	}
	show = doGet(t, e, "/sales/"+id)
	if !strings.Contains(show.Body.String(), "수동 조정") {
		t.Fatal("수동 조정 표시가 없다")
	}

	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"lead"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=reason") {
		t.Fatalf("후퇴 사유 없음: loc=%q", rec.Header().Get("Location"))
	}
	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{
		"stage":  {"lead"},
		"reason": {"내년으로 이연"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=stage") {
		t.Fatalf("후퇴 실패: loc=%q", rec.Header().Get("Location"))
	}
	show = doGet(t, e, "/sales/"+id)
	if !strings.Contains(show.Body.String(), "내년으로 이연") {
		t.Fatal("후퇴 사유가 이력에 없다")
	}

	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"won"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=won") {
		t.Fatalf("미확정 수주확정: loc=%q", rec.Header().Get("Location"))
	}
	rec = doForm(t, e, "/sales/"+id, url.Values{
		"name":                      {"세종시 도서관 RFID 증설"},
		"customer_confirmed":        {"1"},
		"expected_ym_confirmed":     {"1"},
		"expected_amount_confirmed": {"1"},
		"probability":               {"10"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("확정 플래그 저장 status=%d", rec.Code)
	}
	rec = doForm(t, e, "/sales/"+id+"/stage", url.Values{"stage": {"won"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=stage") {
		t.Fatalf("수주확정 실패: loc=%q", rec.Header().Get("Location"))
	}
	show = doGet(t, e, "/sales/"+id)
	body = show.Body.String()
	if !strings.Contains(body, "수주확정") {
		t.Fatal("수주확정 단계명이 없다")
	}
	if strings.Contains(body, "수주확정 · 90%") {
		t.Fatal("수주 후인데 확도 % 가 앞에 붙었다")
	}

	list := doGet(t, e, "/sales")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "영업") {
		t.Fatalf("목록 status=%d", list.Code)
	}
	act := doGet(t, e, "/sales/activities")
	if act.Code != http.StatusOK || !strings.Contains(act.Body.String(), "2단계") {
		t.Fatalf("활동 플레이스홀더: %d", act.Code)
	}
}

func TestSalesHTTP_ProjectFormUnchanged(t *testing.T) {
	e := newSalesServer(t)
	rec := doGet(t, e, "/projects/new")
	if rec.Code != http.StatusOK {
		t.Fatalf("사업 등록 status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "유상/무상") || !strings.Contains(body, "계약방식") {
		t.Fatal("유지보수 사업 등록 화면이 바뀌었다")
	}
	if strings.Contains(body, "임시 사업명") {
		t.Fatal("사업 등록에 영업 필드가 섞였다")
	}
}

func TestSalesHTTP_TechCannotWrite(t *testing.T) {
	e := newSalesServer(t)
	rec := httptestPostAs(t, e, "/sales", url.Values{"name": {"기술은 조회만"}}, "tech")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("기술 등록 status=%d", rec.Code)
	}
}

func httptestPostAs(t *testing.T, e *echo.Echo, path string, form url.Values, role string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieRole(t, role))
	e.ServeHTTP(rec, req)
	return rec
}

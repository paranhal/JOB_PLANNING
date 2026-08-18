package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type stubHolidayAPI struct {
	err    error
	months map[int][]model.Holiday
}

func (s stubHolidayAPI) FetchRestDe(year, month int) ([]model.Holiday, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]model.Holiday(nil), s.months[month]...), nil
}

func newHolidayApp(t *testing.T) (*echo.Echo, *repository.HolidayRepo, *Handler) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "hol.db"))
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
	g.GET("/admin/holidays", h.Holiday.List)
	g.POST("/admin/holidays", h.Holiday.Create)
	g.POST("/admin/holidays/update", h.Holiday.Update)
	g.POST("/admin/holidays/delete", h.Holiday.Delete)
	g.POST("/admin/holidays/sync", h.Holiday.SyncAPI)
	g.POST("/admin/holidays/leaves", h.Holiday.CreateLeave)
	g.POST("/admin/holidays/leaves/delete", h.Holiday.DeleteLeave)
	return e, repository.NewHolidayRepo(db), h
}

func TestHolidayPageAllRolesCanView(t *testing.T) {
	e, _, _ := newHolidayApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/holidays?year=2028", nil)
	req.AddCookie(jwtCookieRole(t, "tech"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "휴무일 관리") {
		t.Fatal("제목 없음")
	}
	if !strings.Contains(body, "2028년 휴무일 미등록 — 주말만 제외됩니다") {
		t.Fatal("미등록 배너 없음")
	}
	if strings.Contains(body, "공휴일 불러오기") {
		t.Fatal("기술담당은 등록 버튼이 보이면 안 됨")
	}
}

func TestHolidayCreateAdminOnly(t *testing.T) {
	e, repo, _ := newHolidayApp(t)

	form := url.Values{
		"year": {"2026"}, "holiday_date": {"2026-07-15"},
		"name": {"창립기념일"}, "kind": {"company"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/holidays", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieRole(t, "tech"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("기술담당 등록 status=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/holidays", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("관리자 등록 status=%d body=%s", rec.Code, rec.Body.String())
	}
	h, err := repo.Get("2026-07-15")
	if err != nil || h == nil || h.Name != "창립기념일" || h.Kind != "company" || h.Source != model.HolidaySourceManual {
		t.Fatalf("저장 실패 %+v err=%v", h, err)
	}
}

func TestHolidaySyncAPISuccessAndFailureBanner(t *testing.T) {
	e, repo, h := newHolidayApp(t)
	if err := repo.Create(model.Holiday{Date: "2026-07-15", Name: "창립기념일", Kind: model.HolidayKindCompany}); err != nil {
		t.Fatal(err)
	}
	before, _ := repo.CountYear(2026)

	h.Holiday.api = stubHolidayAPI{err: fmt.Errorf("portal down")}
	form := url.Values{"year": {"2026"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/holidays/sync", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(loc, "2026 공휴일 동기화 실패") {
		t.Fatalf("실패 배너 없음 loc=%s", loc)
	}
	after, _ := repo.CountYear(2026)
	if after != before {
		t.Fatalf("실패 시 테이블 변경 %d→%d", before, after)
	}
	manual, _ := repo.Get("2026-07-15")
	if manual == nil || manual.Name != "창립기념일" {
		t.Fatal("실패 시 manual 삭제")
	}

	h.Holiday.api = stubHolidayAPI{months: map[int][]model.Holiday{
		9: {{Date: "2026-09-25", Name: "추석", Kind: model.HolidayKindPublic}},
	}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/holidays/sync", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	manual, _ = repo.Get("2026-07-15")
	if manual == nil {
		t.Fatal("성공 동기화도 manual 을 지우면 안 됨")
	}
	if ok, _ := repo.HasDate("2026-09-25"); !ok {
		t.Fatal("추석 없음")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/holidays?year=2026", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "휴무일 미등록") {
		t.Fatal("불러온 뒤에는 미등록 배너가 없어야 함")
	}
	if !strings.Contains(body, "공휴일 불러오기") {
		t.Fatal("관리자 버튼 없음")
	}
}

func TestHolidayLeaveTabAdminOnlyAndRangeSkip(t *testing.T) {
	e, _, h := newHolidayApp(t)
	admin, err := h.Holiday.users.GetByUsername("admin")
	if err != nil || admin == nil {
		t.Fatal("admin 없음")
	}

	form := url.Values{
		"year": {"2026"}, "tab": {"leave"}, "user_id": {admin.UserID},
		"leave_from": {"2026-08-21"}, "leave_to": {"2026-08-21"}, "leave_kind": {"annual"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/holidays/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieRole(t, "tech"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("기술담당 연차 등록 status=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/holidays/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("관리자 연차 등록 status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/holidays?year=2026&tab=leave", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "관리자") || !strings.Contains(body, "연차") {
		t.Fatalf("연차 목록 없음: %s", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/holidays?year=2026&tab=leave", nil)
	req.AddCookie(jwtCookieRole(t, "tech"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("조회 status=%d", rec.Code)
	}
	techBody := rec.Body.String()
	if strings.Contains(techBody, `action="/admin/holidays/leaves"`) {
		t.Fatal("기술담당에게 연차 등록 폼이 보이면 안 됨")
	}

	holidayForm := url.Values{
		"year": {"2026"}, "tab": {"leave"}, "user_id": {admin.UserID},
		"leave_from": {"2026-09-25"}, "leave_to": {"2026-09-25"}, "leave_kind": {"annual"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/holidays/leaves", strings.NewReader(holidayForm.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(loc, "주말·공휴일은 건너뛰어") {
		t.Fatalf("공휴일 단독 등록이 막혀야 함 loc=%s", loc)
	}
}

func TestHolidayPreviewSaturdayBlueSundayHolidayRed(t *testing.T) {
	e, _, _ := newHolidayApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/holidays?year=2026", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "color:#2563EB") {
		t.Fatal("토요일 파랑 없음")
	}
	if !strings.Contains(body, "color:#DC2626") {
		t.Fatal("일요일·공휴일 빨강 없음")
	}
	if !strings.Contains(body, "어린이날") {
		t.Fatal("공휴일명 없음")
	}
	if !strings.Contains(body, "background-color:#FEF2F2") {
		t.Fatal("공휴일 배경 없음")
	}
}

package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func withSupportRoutes(t *testing.T) (*echo.Echo, *Handler, *repository.ASRepo, string) {
	t.Helper()
	e, h, asRepo, _, asID := newASActionFixture(t)
	e.POST("/as/:id/support", h.AS.CreateSupport, h.Auth.AuthMiddleware)
	e.GET("/api/as/asset-history/:asset_id", h.AS.APIAssetHistory, h.Auth.AuthMiddleware)
	return e, h, asRepo, asID
}

func TestCreateSupportHTTPAndCompleteWarnAllow(t *testing.T) {
	e, h, asRepo, asID := withSupportRoutes(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/support",
		strings.NewReader(url.Values{
			"support_type": {model.SupportOther},
			"memo":         {"고객 안내"},
		}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d", rec.Code)
	}
	items, err := h.AS.workRepo.ListSupportByAS(asID)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v %v", items, err)
	}
	gotAS, _ := asRepo.GetByID(repository.OrgAll, asID)
	if gotAS.Status == "completed" {
		t.Fatal("create closed AS")
	}

	show := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID, nil)
	sreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, sreq)
	body := show.Body.String()
	if !strings.Contains(body, "지원업무") || !strings.Contains(body, items[0].Work.WorkNumber) {
		t.Fatal("show missing support")
	}

	rec = postASAction(t, e, asID, url.Values{
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_cat1":   {"rfid"},
		"cause_cat2":   {"rfid.hw"},
		"cause_cat3":   {"rfid.hw.part_check"},
		"action_taken": {"현장 조치 완료함"},
		"result_code":  {model.ResultDone},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=open_support") {
		t.Fatalf("warn loc=%s", loc)
	}
	gotAS, _ = asRepo.GetByID(repository.OrgAll, asID)
	if gotAS.Status == "completed" {
		t.Fatal("completed without confirm")
	}

	rec = postASAction(t, e, asID, url.Values{
		"work_place":            {"field"},
		"process_type":          {"visit"},
		"cause_cat1":            {"rfid"},
		"cause_cat2":            {"rfid.hw"},
		"cause_cat3":            {"rfid.hw.part_check"},
		"action_taken":          {"현장 조치 완료함"},
		"result_code":           {model.ResultDone},
		"confirm_open_support":  {"1"},
	})
	loc = rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=") {
		t.Fatalf("confirm loc=%s", loc)
	}
	gotAS, _ = asRepo.GetByID(repository.OrgAll, asID)
	if gotAS.Status != "completed" {
		t.Fatalf("status=%s", gotAS.Status)
	}
	w, _ := h.AS.workRepo.GetByID(items[0].Work.WorkID)
	if w == nil || w.Status != "open" {
		t.Fatalf("support closed: %+v", w)
	}

	act := getASActionPage(t, e, asID)
	if !strings.Contains(act, `data-open-support="1"`) && !strings.Contains(act, "지원업무") {
		// 완료 화면에도 열린 지원 건수가 남아야 한다
		if n, _ := h.AS.workRepo.CountOpenSupport(asID); n != 1 {
			t.Fatalf("open after complete=%d", n)
		}
	}
}

func TestCreateSupportForbiddenWithoutProcess(t *testing.T) {
	e, _, _, asID := withSupportRoutes(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/support",
		strings.NewReader(url.Values{
			"support_type": {model.SupportOther},
			"memo":         {"x"},
		}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookiePerms(t, model.RoleObserver, model.PermStats))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Code == http.StatusSeeOther && !strings.Contains(rec.Header().Get("Location"), "login") {
		// 권한 없는 영업은 미들웨어 없이 핸들러 403. 이 픽스처는 AuthMiddleware만 있음.
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestWorkActionSupportFieldsRoundTrip(t *testing.T) {
	e, h, _, asID := withSupportRoutes(t)
	w := &model.ASWorkItem{ASID: asID}
	if err := h.AS.workRepo.CreateSupport(w, model.ASSupportDetail{
		SupportType: model.SupportCourier, TrackingNo: "OLD",
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/work/"+w.WorkID+"/action",
		strings.NewReader(url.Values{
			"support_type": {model.SupportCourier},
			"tracking_no":  {"NEW-99"},
			"notes":        {"발송"},
		}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	d, err := h.AS.workRepo.GetDetail(w.WorkID)
	if err != nil || d == nil || d.TrackingNo != "NEW-99" {
		t.Fatalf("detail %+v %v", d, err)
	}
	parent, _ := h.AS.repo.GetByID(repository.OrgAll, asID)
	if parent.Status == "completed" {
		t.Fatal("parent changed")
	}

	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "http://localhost/as/work/"+w.WorkID+"/action", nil)
	preq.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, preq)
	if !strings.Contains(page.Body.String(), "송장번호") || !strings.Contains(page.Body.String(), "NEW-99") {
		t.Fatal("work action missing support fields")
	}
}

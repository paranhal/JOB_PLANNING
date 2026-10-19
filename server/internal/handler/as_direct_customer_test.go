package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestASNewFormHasDirectCustomer(t *testing.T) {
	e, _, _, _, _ := newReceiptPhotoFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/new", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "목록에 없음 → 직접입력") {
		t.Fatal("접수 화면에 직접입력 토글이 없다")
	}
}

func TestASCreateDirectCustomer(t *testing.T) {
	e, h, asRepo, _, _ := newReceiptPhotoFixture(t)
	form := url.Values{
		"customer_direct":  {"1"},
		"org_name_direct":  {"74B직접입력기관"},
		"contact_direct":   {"현장담당"},
		"phone_direct":     {"010-1111-2222"},
		"symptom":          {"직접입력 접수 증상"},
		"received_by":      {"테스터"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	id := strings.TrimPrefix(loc, "/as/")
	got, err := asRepo.GetByID(repository.OrgAll, id)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.OrgName != "74B직접입력기관" {
		t.Fatalf("org=%s", got.OrgName)
	}
	var prov int
	if err := h.AS.db.QueryRow(`SELECT COALESCE(is_provisional,0) FROM customers WHERE customer_id=?`, got.CustomerID).Scan(&prov); err != nil || prov != 1 {
		t.Fatalf("provisional=%d err=%v", prov, err)
	}
}

func TestASDirectInputReadonlyForbidden(t *testing.T) {
	e, _, _, _, _ := newReceiptPhotoFixture(t)
	form := url.Values{
		"customer_direct": {"1"},
		"org_name_direct": {"읽기전용직접"},
		"symptom":         {"막힘"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieReadOnly(t, model.RoleTech))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("readonly status=%d", rec.Code)
	}
}

func TestASCustomerMatchOfficial(t *testing.T) {
	e, _, _, _, _ := newReceiptPhotoFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/customer-match?name="+url.QueryEscape("가나도서관"), nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"official":true`) && !strings.Contains(rec.Body.String(), `"found":true`) {
		t.Fatalf("match body=%s", rec.Body.String())
	}
}

func TestASActionDirectCustomer(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/as/:id/customer-direct", h.AS.UpdateCustomerDirect)
	form := url.Values{
		"customer_direct": {"1"},
		"org_name_direct": {"조치화면직접기관"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/customer-direct", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	got, err := asRepo.GetByID(repository.OrgAll, asID)
	if err != nil || got == nil || got.OrgName != "조치화면직접기관" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

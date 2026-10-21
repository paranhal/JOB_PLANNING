package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestOrgImportCLevelForbiddenWrite(t *testing.T) {
	e, db, _, _, cl := newCustomer947Server(t)
	h := New(db)
	e.GET("/customers/orgs-import", h.Customer.OrgImport, h.Auth.AuthMiddleware)
	e.POST("/customers/orgs-import-preview", h.Customer.OrgImportPreview, h.Auth.AuthMiddleware)
	ck := jwtCookieViewOrg(t, cl, repository.OrgAll)
	get := doGetWith(t, e, "/customers/orgs-import", ck)
	if get.Code != http.StatusOK {
		t.Fatalf("get %d", get.Code)
	}
	if strings.Contains(get.Body.String(), "미리보기") && strings.Contains(get.Body.String(), `name="file"`) {
		t.Fatal("C-Level 쓰기 폼이 보이면 안 된다")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/customers/orgs-import-preview", strings.NewReader(""))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("preview %d", rec.Code)
	}
}

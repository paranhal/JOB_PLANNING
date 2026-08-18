package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newIntegrationApp(t *testing.T) (*echo.Echo, *repository.CustomerRepo, *repository.ContactRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "int.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := echo.New()
	h := New(db)
	v1 := e.Group("/api/v1", h.Integration.RequireAPIKey)
	v1.GET("/customers", h.Integration.ListCustomers)
	v1.GET("/customers/:id/contacts", h.Integration.ListCustomerContacts)
	v1.GET("/customers/:id", h.Integration.GetCustomer)
	v1.GET("/contacts", h.Integration.ListContacts)
	v1.GET("/contacts/:id", h.Integration.GetContact)
	v1.GET("/codes", h.Integration.ListCodes)
	return e, repository.NewCustomerRepo(db), repository.NewContactRepo(db)
}

func TestIntegrationAPIRequiresKey(t *testing.T) {
	t.Setenv("INTEGRATION_API_KEY", "")
	e, _, _ := newIntegrationApp(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/customers", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("키 없음: %d %s", rec.Code, rec.Body.String())
	}

	t.Setenv("INTEGRATION_API_KEY", "secret-key")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customers", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("틀린 키: %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/customers", nil)
	req.Header.Set("X-API-Key", "secret-key")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("정상 키: %d %s", rec.Code, rec.Body.String())
	}
}

func TestIntegrationAPICustomersAndContacts(t *testing.T) {
	t.Setenv("INTEGRATION_API_KEY", "k1")
	e, custRepo, contactRepo := newIntegrationApp(t)
	c := &model.Customer{OrgName: "테스트도서관", OfficialName: "테스트시도립도서관", MainPhone: "02-111-2222", Industry: "도서관", IsActive: true}
	if err := custRepo.Create(c); err != nil {
		t.Fatal(err)
	}
	ct := &model.Contact{CustomerID: c.CustomerID, FullName: "김사서", Affiliation: "institution", Status: "active", ContactRole: "primary", Phone: "02-111-0000"}
	if err := contactRepo.Create(ct); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/customers?q=테스트", nil)
	req.Header.Set("X-API-Key", "k1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("고객 목록: %d %s", rec.Code, rec.Body.String())
	}
	var list struct {
		OK    bool `json:"ok"`
		Total int  `json:"total"`
		Items []model.Customer
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if !list.OK || list.Total < 1 || list.Items[0].OrgName != "테스트도서관" {
		t.Fatalf("고객 목록 내용: %+v", list)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/customers/"+c.CustomerID+"/contacts", nil)
	req.Header.Set("Authorization", "Bearer k1")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("기관 담당자: %d %s", rec.Code, rec.Body.String())
	}
	var contacts struct {
		OK    bool `json:"ok"`
		Total int  `json:"total"`
		Items []model.Contact
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &contacts); err != nil {
		t.Fatal(err)
	}
	if contacts.Total != 1 || contacts.Items[0].FullName != "김사서" || !contacts.Items[0].IsPrimary {
		t.Fatalf("담당자: %+v", contacts)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/contacts/"+ct.ContactID, nil)
	req.Header.Set("X-API-Key", "k1")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("담당자 단건: %d %s", rec.Code, rec.Body.String())
	}
}

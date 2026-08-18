package handler

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

const integrationAPIMaxPage = 500

// IntegrationHandler 영업관리 시스템 연동. 읽기 전용. API 키 인증.
type IntegrationHandler struct {
	customers *repository.CustomerRepo
	contacts  *repository.ContactRepo
	codes     *repository.CodeRepo
}

func NewIntegrationHandler(customers *repository.CustomerRepo, contacts *repository.ContactRepo, codes *repository.CodeRepo) *IntegrationHandler {
	return &IntegrationHandler{customers: customers, contacts: contacts, codes: codes}
}

func integrationAPIKey() string {
	return strings.TrimSpace(os.Getenv("INTEGRATION_API_KEY"))
}

func (h *IntegrationHandler) RequireAPIKey(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		want := integrationAPIKey()
		if want == "" {
			return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
				"ok": false, "error": "INTEGRATION_API_KEY 가 설정되지 않았습니다",
			})
		}
		got := strings.TrimSpace(c.Request().Header.Get("X-API-Key"))
		if got == "" {
			auth := strings.TrimSpace(c.Request().Header.Get("Authorization"))
			if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				got = strings.TrimSpace(auth[7:])
			}
		}
		if len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			return c.JSON(http.StatusUnauthorized, map[string]interface{}{
				"ok": false, "error": "API 키가 올바르지 않습니다",
			})
		}
		return next(c)
	}
}

func (h *IntegrationHandler) ListCustomers(c echo.Context) error {
	page, size := integrationPage(c)
	var active *bool
	if raw := strings.TrimSpace(c.QueryParam("is_active")); raw != "" {
		v := raw == "1" || strings.EqualFold(raw, "true")
		active = &v
		if raw == "0" || strings.EqualFold(raw, "false") {
			f := false
			active = &f
		}
	}
	items, total, err := h.customers.ListForAPI(c.QueryParam("q"), active, size, (page-1)*size)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok": true, "total": total, "page": page, "page_size": size, "items": items,
	})
}

func (h *IntegrationHandler) GetCustomer(c echo.Context) error {
	item, err := h.customers.GetByID(c.Param("id"))
	if err != nil {
		return err
	}
	if item == nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{"ok": false, "error": "고객을 찾을 수 없습니다"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "item": item})
}

func (h *IntegrationHandler) ListCustomerContacts(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	cust, err := h.customers.GetByID(id)
	if err != nil {
		return err
	}
	if cust == nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{"ok": false, "error": "고객을 찾을 수 없습니다"})
	}
	page, size := integrationPage(c)
	items, total, err := h.contacts.ListForAPI(c.QueryParam("q"), id, c.QueryParam("status"), size, (page-1)*size)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok": true, "total": total, "page": page, "page_size": size,
		"customer_id": id, "org_name": cust.OrgName, "items": items,
	})
}

func (h *IntegrationHandler) ListContacts(c echo.Context) error {
	page, size := integrationPage(c)
	items, total, err := h.contacts.ListForAPI(
		c.QueryParam("q"), c.QueryParam("customer_id"), c.QueryParam("status"),
		size, (page-1)*size,
	)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok": true, "total": total, "page": page, "page_size": size, "items": items,
	})
}

func (h *IntegrationHandler) GetContact(c echo.Context) error {
	item, err := h.contacts.GetByID(c.Param("id"))
	if err != nil {
		return err
	}
	if item == nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{"ok": false, "error": "담당자를 찾을 수 없습니다"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "item": item})
}

func (h *IntegrationHandler) ListCodes(c echo.Context) error {
	group := strings.TrimSpace(c.QueryParam("group"))
	var items interface{}
	var err error
	if group == "" {
		items, err = h.codes.ListAll()
	} else {
		items, err = h.codes.ListByGroup(group)
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "items": items})
}

func integrationPage(c echo.Context) (page, size int) {
	page, _ = strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	size, _ = strconv.Atoi(c.QueryParam("page_size"))
	if size < 1 {
		size = 100
	}
	if size > integrationAPIMaxPage {
		size = integrationAPIMaxPage
	}
	return page, size
}

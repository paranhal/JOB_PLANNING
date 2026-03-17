package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type CustomerHandler struct {
	repo *repository.CustomerRepo
}

// List 고객 목록
func (h *CustomerHandler) List(c echo.Context) error {
	search := c.QueryParam("search")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	pageSize := 20

	items, total, err := h.repo.List(search, page, pageSize)
	if err != nil {
		return err
	}

	totalPages := (total + pageSize - 1) / pageSize

	return c.Render(http.StatusOK, "customer/list.html", map[string]interface{}{
		"Title":      "고객 관리",
		"Active":     "customers",
		"Items":      items,
		"Total":      total,
		"Page":       page,
		"PageSize":   pageSize,
		"TotalPages": totalPages,
		"Search":     search,
	})
}

// New 고객 등록 폼
func (h *CustomerHandler) New(c echo.Context) error {
	customers, _ := h.repo.ListAll()
	return c.Render(http.StatusOK, "customer/form.html", map[string]interface{}{
		"Title":     "고객 등록",
		"Active":    "customers",
		"Customer":  &model.Customer{IsActive: true},
		"Customers": customers,
		"IsNew":     true,
	})
}

// Create 고객 등록 처리
func (h *CustomerHandler) Create(c echo.Context) error {
	cust := bindCustomer(c)
	if err := h.repo.Create(cust); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/customers")
}

// Show 고객 상세
func (h *CustomerHandler) Show(c echo.Context) error {
	id := c.Param("id")
	cust, err := h.repo.GetByID(id)
	if err != nil {
		return err
	}
	if cust == nil {
		return echo.ErrNotFound
	}
	return c.Render(http.StatusOK, "customer/show.html", map[string]interface{}{
		"Title":    cust.OrgName,
		"Active":   "customers",
		"Customer": cust,
	})
}

// Edit 고객 수정 폼
func (h *CustomerHandler) Edit(c echo.Context) error {
	id := c.Param("id")
	cust, err := h.repo.GetByID(id)
	if err != nil {
		return err
	}
	if cust == nil {
		return echo.ErrNotFound
	}
	customers, _ := h.repo.ListAll()
	return c.Render(http.StatusOK, "customer/form.html", map[string]interface{}{
		"Title":     "고객 수정",
		"Active":    "customers",
		"Customer":  cust,
		"Customers": customers,
		"IsNew":     false,
	})
}

// Update 고객 수정 처리
func (h *CustomerHandler) Update(c echo.Context) error {
	cust := bindCustomer(c)
	cust.CustomerID = c.Param("id")
	if err := h.repo.Update(cust); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/customers/"+cust.CustomerID)
}

// Delete 고객 비활성화
func (h *CustomerHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/customers")
}

// bindCustomer 폼 데이터를 Customer 구조체로 변환
func bindCustomer(c echo.Context) *model.Customer {
	return &model.Customer{
		OrgName:          c.FormValue("org_name"),
		OfficialName:     c.FormValue("official_name"),
		OrgEmail:         c.FormValue("org_email"),
		MainPhone:        c.FormValue("main_phone"),
		Website:          c.FormValue("website"),
		BusinessNumber:   c.FormValue("business_number"),
		Representative:   c.FormValue("representative"),
		Industry:         c.FormValue("industry"),
		HasParent:        c.FormValue("has_parent") == "1",
		ParentCustomerID: c.FormValue("parent_customer_id"),
		Address:          c.FormValue("address"),
		AddressDetail:    c.FormValue("address_detail"),
		IsActive:         c.FormValue("is_active") != "0",
		Notes:            c.FormValue("notes"),
	}
}

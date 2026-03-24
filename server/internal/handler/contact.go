package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type ContactHandler struct {
	repo         *repository.ContactRepo
	customerRepo *repository.CustomerRepo
}

// List 담당자 목록
func (h *ContactHandler) List(c echo.Context) error {
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

	return c.Render(http.StatusOK, "contact/list.html", map[string]interface{}{
		"Title":      "담당자 관리",
		"Active":     "contacts",
		"Items":      items,
		"Total":      total,
		"Page":       page,
		"PageSize":   pageSize,
		"TotalPages": totalPages,
		"Search":     search,
	})
}

// New 담당자 등록 폼
func (h *ContactHandler) New(c echo.Context) error {
	customers, _ := h.customerRepo.ListAll()
	ct := &model.Contact{Status: "active", IsPrimary: true}
	if cid := c.QueryParam("customer_id"); cid != "" {
		ct.CustomerID = cid
	}
	return c.Render(http.StatusOK, "contact/form.html", map[string]interface{}{
		"Title":     "담당자 등록",
		"Active":    "contacts",
		"Contact":   ct,
		"Customers": customers,
		"IsNew":     true,
	})
}

// Create 담당자 등록 처리
func (h *ContactHandler) Create(c echo.Context) error {
	ct := bindContact(c)
	if err := h.repo.Create(ct); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/contacts/"+ct.ContactID)
}

// Show 담당자 상세
func (h *ContactHandler) Show(c echo.Context) error {
	id := c.Param("id")
	ct, err := h.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ct == nil {
		return echo.ErrNotFound
	}
	return c.Render(http.StatusOK, "contact/show.html", map[string]interface{}{
		"Title":   ct.FullName + " · 담당자",
		"Active":  "contacts",
		"Contact": ct,
	})
}

// Edit 담당자 수정 폼
func (h *ContactHandler) Edit(c echo.Context) error {
	id := c.Param("id")
	ct, err := h.repo.GetByID(id)
	if err != nil {
		return err
	}
	if ct == nil {
		return echo.ErrNotFound
	}
	customers, _ := h.customerRepo.ListAll()
	return c.Render(http.StatusOK, "contact/form.html", map[string]interface{}{
		"Title":     "담당자 수정",
		"Active":    "contacts",
		"Contact":   ct,
		"Customers": customers,
		"IsNew":     false,
	})
}

// Update 담당자 수정 처리
func (h *ContactHandler) Update(c echo.Context) error {
	ct := bindContact(c)
	ct.ContactID = c.Param("id")
	if err := h.repo.Update(ct); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/contacts/"+ct.ContactID)
}

func bindContact(c echo.Context) *model.Contact {
	return &model.Contact{
		CustomerID: c.FormValue("customer_id"),
		FullName:   c.FormValue("full_name"),
		JobRole:    c.FormValue("job_role"),
		Title:      c.FormValue("title"),
		Phone:      c.FormValue("phone"),
		Mobile:     c.FormValue("mobile"),
		Email:      c.FormValue("email"),
		StartDate:  c.FormValue("start_date"),
		EndDate:    c.FormValue("end_date"),
		Status:     c.FormValue("status"),
		IsPrimary:  c.FormValue("is_primary") == "1",
		Notes:      c.FormValue("notes"),
	}
}

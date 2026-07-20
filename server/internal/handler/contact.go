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
	histRepo     *repository.ContactHistoryRepo
	codeRepo     *repository.CodeRepo
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
	jobGrades, _ := h.codeRepo.ActiveByGroup("job_grade")
	ct := &model.Contact{Status: "active", ContactRole: "primary", IsPrimary: true, Affiliation: "institution"}
	if cid := c.QueryParam("customer_id"); cid != "" {
		ct.CustomerID = cid
	}
	return c.Render(http.StatusOK, "contact/form.html", map[string]interface{}{
		"Title":     "담당자 등록",
		"Active":    "contacts",
		"Contact":   ct,
		"Customers": customers,
		"JobGrades": jobGrades,
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
	jobGrades, _ := h.codeRepo.ActiveByGroup("job_grade")
	return c.Render(http.StatusOK, "contact/form.html", map[string]interface{}{
		"Title":     "담당자 수정",
		"Active":    "contacts",
		"Contact":   ct,
		"Customers": customers,
		"JobGrades": jobGrades,
		"IsNew":     false,
	})
}

// Update 담당자 수정 처리 (전보·퇴직·업무조정 시에만 이력 저장)
func (h *ContactHandler) Update(c echo.Context) error {
	id := c.Param("id")
	old, _ := h.repo.GetByID(id)

	ct := bindContact(c)
	ct.ContactID = id

	if old != nil {
		if reason := contactChangeReason(old, ct); reason != "" {
			h.histRepo.SnapshotFromContact(old, reason)
		}
	}

	if err := h.repo.Update(ct); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/contacts/"+ct.ContactID)
}

// contactChangeReason 담당자 변경 이력 사유 (없으면 빈 문자열 = 이력 미기록)
func contactChangeReason(old, neu *model.Contact) string {
	oldRole := old.ContactRole
	if oldRole == "" {
		if old.IsPrimary {
			oldRole = "primary"
		} else {
			oldRole = "regular"
		}
	}
	newRole := neu.ContactRole
	if newRole == "" {
		if neu.IsPrimary {
			newRole = "primary"
		} else {
			newRole = "regular"
		}
	}

	if old.Status != neu.Status {
		switch neu.Status {
		case "transferred":
			return "transfer"
		case "resigned":
			return "resign"
		}
	}
	if oldRole != newRole {
		return "role_adjust"
	}
	return ""
}

// APIContactsByCustomer 고객별 담당자 목록 JSON (AS 접수 요청자 콤보 등)
func (h *ContactHandler) APIContactsByCustomer(c echo.Context) error {
	items, err := h.repo.ListByCustomer(c.Param("customer_id"))
	if err != nil {
		return err
	}
	if items == nil {
		items = []model.Contact{}
	}
	return c.JSON(http.StatusOK, items)
}

func bindContact(c echo.Context) *model.Contact {
	jobGrade := c.FormValue("job_grade_code")
	if jobGrade == "custom" {
		jobGrade = c.FormValue("job_grade_custom")
	}

	role := c.FormValue("contact_role")
	if role == "" {
		if c.FormValue("is_primary") == "1" {
			role = "primary"
		} else {
			role = "regular"
		}
	}

	return &model.Contact{
		CustomerID:  c.FormValue("customer_id"),
		FullName:    c.FormValue("full_name"),
		Affiliation: c.FormValue("affiliation"),
		JobRole:     c.FormValue("job_role"),
		Title:       c.FormValue("title"),
		JobGrade:    jobGrade,
		Phone:       c.FormValue("phone"),
		Mobile:      c.FormValue("mobile"),
		Email:       c.FormValue("email"),
		StartDate:   c.FormValue("start_date"),
		EndDate:     c.FormValue("end_date"),
		Status:      c.FormValue("status"),
		ContactRole: role,
		IsPrimary:   role == "primary",
		Notes:       c.FormValue("notes"),
	}
}

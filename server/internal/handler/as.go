package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type ASHandler struct {
	repo         *repository.ASRepo
	customerRepo *repository.CustomerRepo
}

// List AS 목록
func (h *ASHandler) List(c echo.Context) error {
	status := c.QueryParam("status")
	search := c.QueryParam("search")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	items, total, err := h.repo.List(status, search, page, 20)
	if err != nil {
		return err
	}

	stats, _ := h.repo.Stats()
	totalPages := (total + 19) / 20

	return c.Render(http.StatusOK, "as/list.html", map[string]interface{}{
		"Title":      "AS 관리",
		"Active":     "as",
		"Items":      items,
		"Total":      total,
		"Page":       page,
		"TotalPages": totalPages,
		"Status":     status,
		"Search":     search,
		"Stats":      stats,
	})
}

// New AS 접수 폼
func (h *ASHandler) New(c echo.Context) error {
	customers, _ := h.customerRepo.ListAll()
	return c.Render(http.StatusOK, "as/form.html", map[string]interface{}{
		"Title":     "AS 접수",
		"Active":    "as",
		"AS":        &model.ASReceipt{Urgency: "normal", Status: "received"},
		"Customers": customers,
		"IsNew":     true,
	})
}

// Create AS 접수 등록
func (h *ASHandler) Create(c echo.Context) error {
	as := &model.ASReceipt{
		CustomerID:    c.FormValue("customer_id"),
		AssetID:       c.FormValue("asset_id"),
		ReceiptChannel: c.FormValue("receipt_channel"),
		Requester:     c.FormValue("requester"),
		Symptom:       c.FormValue("symptom"),
		Urgency:       c.FormValue("urgency"),
		RequesterType: c.FormValue("requester_type"),
		RequesterName: c.FormValue("requester_name"),
		AssignedTo:    c.FormValue("assigned_to"),
	}
	if as.Urgency == "" {
		as.Urgency = "normal"
	}
	if err := h.repo.Create(as); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as")
}

// Show AS 상세
func (h *ASHandler) Show(c echo.Context) error {
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil {
		return err
	}
	if as == nil {
		return echo.ErrNotFound
	}
	return c.Render(http.StatusOK, "as/show.html", map[string]interface{}{
		"Title":  as.ASNumber,
		"Active": "as",
		"AS":     as,
	})
}

// Update AS 처리 내용 수정
func (h *ASHandler) Update(c echo.Context) error {
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	as.Status = c.FormValue("status")
	as.AssignedTo = c.FormValue("assigned_to")
	as.ProcessType = c.FormValue("process_type")
	as.CauseType = c.FormValue("cause_type")
	as.ActionTaken = c.FormValue("action_taken")
	as.PartsUsed = c.FormValue("parts_used")
	as.ResultCode = c.FormValue("result_code")
	as.FollowupAction = c.FormValue("followup_action")

	if err := h.repo.Update(as); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

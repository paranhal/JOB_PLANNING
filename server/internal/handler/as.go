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
	processRepo  *repository.ASProcessRepo
	customerRepo *repository.CustomerRepo
	assetRepo    *repository.AssetRepo
	codeRepo     *repository.CodeRepo
}

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
		"Title": "AS 관리", "Active": "as",
		"Items": items, "Total": total,
		"Page": page, "TotalPages": totalPages,
		"Status": status, "Search": search, "Stats": stats,
	})
}

func (h *ASHandler) New(c echo.Context) error {
	customers, _ := h.customerRepo.ListAll()
	channels, _ := h.codeRepo.ActiveByGroup("receipt_channel")
	urgencies, _ := h.codeRepo.ActiveByGroup("urgency")
	reqTypes, _ := h.codeRepo.ActiveByGroup("requester_type")

	as := &model.ASReceipt{Urgency: "normal", Priority: "normal", Status: "received"}
	if cid := c.QueryParam("customer_id"); cid != "" {
		as.CustomerID = cid
	}

	return c.Render(http.StatusOK, "as/form.html", map[string]interface{}{
		"Title": "AS 접수", "Active": "as", "IsNew": true,
		"AS": as, "Customers": customers,
		"Channels": channels, "Urgencies": urgencies, "ReqTypes": reqTypes,
	})
}

func (h *ASHandler) Create(c echo.Context) error {
	as := &model.ASReceipt{
		CustomerID:     c.FormValue("customer_id"),
		AssetID:        c.FormValue("asset_id"),
		ReceiptChannel: c.FormValue("receipt_channel"),
		Requester:      c.FormValue("requester"),
		Symptom:        c.FormValue("symptom"),
		Urgency:        c.FormValue("urgency"),
		Priority:       c.FormValue("priority"),
		RequesterType:  c.FormValue("requester_type"),
		RequesterName:  c.FormValue("requester_name"),
		AssignedTo:     c.FormValue("assigned_to"),
	}
	if as.Urgency == "" {
		as.Urgency = "normal"
	}
	if as.Priority == "" {
		as.Priority = "normal"
	}
	if err := h.repo.Create(as); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+as.ASID)
}

func (h *ASHandler) Show(c echo.Context) error {
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}

	processes, _ := h.processRepo.ListByAS(id)
	procTypes, _ := h.codeRepo.ActiveByGroup("process_type")
	causeTypes, _ := h.codeRepo.ActiveByGroup("cause_type")
	resultCodes, _ := h.codeRepo.ActiveByGroup("result_code")

	return c.Render(http.StatusOK, "as/show.html", map[string]interface{}{
		"Title": as.ASNumber, "Active": "as", "AS": as,
		"Processes": processes,
		"ProcTypes": procTypes, "CauseTypes": causeTypes, "ResultCodes": resultCodes,
	})
}

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
	as.CustomerConfirmer = c.FormValue("customer_confirmer")
	as.IsRecurrence = c.FormValue("is_recurrence") == "1"
	as.IsReopen = c.FormValue("is_reopen") == "1"
	as.ReplaceReview = c.FormValue("replace_review") == "1"

	if err := h.repo.Update(as); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id)
}

// AddProcess AS 처리 이력 추가
func (h *ASHandler) AddProcess(c echo.Context) error {
	asID := c.Param("id")
	timeSpent, _ := strconv.Atoi(c.FormValue("time_spent"))
	p := &model.ASProcess{
		ASID:        asID,
		Worker:      c.FormValue("worker"),
		WorkType:    c.FormValue("work_type"),
		WorkContent: c.FormValue("work_content"),
		PartsUsed:   c.FormValue("parts_used"),
		TimeSpent:   timeSpent,
		Notes:       c.FormValue("notes"),
	}
	h.processRepo.Create(p)
	return c.Redirect(http.StatusSeeOther, "/as/"+asID)
}

// Stats AS 현황 전용 화면
func (h *ASHandler) StatsDashboard(c echo.Context) error {
	stats, _ := h.repo.Stats()

	byCustomer, _ := h.repo.StatsByCustomer()
	byStatus, _ := h.repo.StatsByStatus()

	overdue, _, _ := h.repo.ListOverdue(1, 50)

	return c.Render(http.StatusOK, "as/stats.html", map[string]interface{}{
		"Title": "AS 현황", "Active": "as_stats",
		"Stats": stats, "ByCustomer": byCustomer, "ByStatus": byStatus,
		"Overdue": overdue,
	})
}

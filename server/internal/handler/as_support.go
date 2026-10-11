package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func (h *ASHandler) CreateSupport(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(currentOrg(c), id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	d := supportDetailFromForm(c)
	if d.AssetID == "" && as.AssetID != "" {
		d.AssetID = as.AssetID
	}
	w := &model.ASWorkItem{
		ASID:              as.ASID,
		WorkKind:          model.WorkKindSupport,
		ScheduledDate:     normalizeVisitDate(firstNonEmpty(d.DueDate, c.FormValue("scheduled_date"))),
		ScheduleConfirmed: c.FormValue("schedule_confirmed") == "1",
		Notes:             strings.TrimSpace(firstNonEmpty(d.Memo, c.FormValue("notes"))),
	}
	w.AssignedTo, w.AssignedUserID = resolveAssigneeFormFields(c, h.userRepo,
		"assigned_to_code", "assigned_to_custom", "assigned_to")
	if err := h.workRepo.CreateSupport(w, d); err != nil {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id+"?ok="+url.QueryEscape("지원업무를 등록했습니다"))
}

func supportDetailFromForm(c echo.Context) model.ASSupportDetail {
	return model.ASSupportDetail{
		SupportType: strings.TrimSpace(c.FormValue("support_type")),
		VendorName:  strings.TrimSpace(c.FormValue("vendor_name")),
		TrackingNo:  strings.TrimSpace(c.FormValue("tracking_no")),
		ActionDate:  normalizeVisitDate(c.FormValue("action_date")),
		DueDate:     normalizeVisitDate(c.FormValue("due_date")),
		AssetID:     strings.TrimSpace(c.FormValue("asset_id")),
		Memo:        strings.TrimSpace(c.FormValue("memo")),
	}
}

func (h *ASHandler) mergeSupportPageData(c echo.Context, as *model.ASReceipt, data map[string]interface{}) {
	if h.workRepo == nil || as == nil {
		return
	}
	items, _ := h.workRepo.ListSupportByAS(as.ASID)
	if items == nil {
		items = []model.ASSupportItem{}
	}
	data["SupportItems"] = items
	data["SupportStepper"] = model.DeriveSupportStepper(items)
	data["SupportTypes"] = model.SupportTypeOptions()
	n, _ := h.workRepo.CountOpenSupport(as.ASID)
	data["OpenSupportN"] = n
	today := time.Now().Format("2006-01-02")
	data["FactoryWait"] = h.workRepo.FactoryWaitForAS(as.ASID, today)
	if as.AssetID != "" {
		data["SupportDefaultAsset"] = as.AssetID
	}
}

func parseOpenSupportN(c echo.Context) int {
	n, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam("n")))
	return n
}

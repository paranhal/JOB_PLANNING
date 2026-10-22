package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func (h *MeetingHandler) markWeeklyCanEdit(c echo.Context, items []model.WorkListItem) {
	for i := range items {
		items[i].CanEdit = h.weeklyItemCanEdit(c, items[i])
	}
}

func (h *MeetingHandler) weeklyItemCanEdit(c echo.Context, it model.WorkListItem) bool {
	if h == nil || isReadOnly(c) || viewAsActive(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	switch it.Prefix {
	case model.WorkPrefixAS:
		if h.asRepo == nil {
			return false
		}
		as, err := h.asRepo.GetByID(currentOrg(c), it.RefID)
		if err != nil || as == nil {
			return false
		}
		canMod := false
		if h.asH != nil {
			canMod = h.asH.canModifyAS(c, as)
		}
		return canSaveASReceipt(c, as, canMod)
	case model.WorkPrefixSales:
		return canEditSalesActivity(c, it.Assignee)
	default:
		return canEditTask(c, it.Assignee, "")
	}
}

// WeeklyItemForm GET /meeting/weekly/item 간단 수정 조각. D-95
func (h *MeetingHandler) WeeklyItemForm(c echo.Context) error {
	it, as, errCode := h.loadWeeklyEditTarget(c)
	if errCode != 0 {
		return echo.NewHTTPError(errCode)
	}
	if !h.weeklyItemCanEdit(c, it) {
		return echo.ErrForbidden
	}
	var users []model.User
	if h.userRepo != nil {
		users = listAssignableForPicker(h.userRepo, c)
	}
	date := strings.TrimSpace(it.ScheduledDate)
	if date == "" {
		date = strings.TrimSpace(it.WorkDate)
	}
	memo := strings.TrimSpace(it.Title)
	if as != nil {
		date = as.VisitScheduledDate
		memo = as.Symptom
	}
	return RenderPartial(c, "meeting/_item_edit.html", map[string]interface{}{
		"Prefix":   it.Prefix,
		"RefID":    it.RefID,
		"Title":    it.Title,
		"Date":     date,
		"Assignee": it.Assignee,
		"Status":   it.Status,
		"Memo":     memo,
		"Users":    users,
		"Return":   strings.TrimSpace(c.QueryParam("return")),
		"Detail":   it.Href,
		"Err":      strings.TrimSpace(c.QueryParam("err")),
	})
}

// WeeklyItemSave POST /meeting/weekly/item. 기존 AS 접수 수정 경로를 탄다.
func (h *MeetingHandler) WeeklyItemSave(c echo.Context) error {
	back := strings.TrimSpace(c.FormValue("return"))
	if back == "" {
		back = "/meeting/weekly"
	}
	it, as, errCode := h.loadWeeklyEditTarget(c)
	if errCode == http.StatusUnauthorized {
		return c.Redirect(http.StatusFound, "/login")
	}
	if errCode != 0 {
		return echo.NewHTTPError(errCode)
	}
	if !h.weeklyItemCanEdit(c, it) {
		return echo.ErrForbidden
	}
	switch it.Prefix {
	case model.WorkPrefixAS:
		if as == nil || h.asRepo == nil {
			return echo.ErrNotFound
		}
		if raw := strings.TrimSpace(c.FormValue("work_date")); raw != "" {
			date, err := model.ParseAppDate(raw)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "날짜를 확인하세요")
			}
			if canEditVisitDate(c, as) {
				if err := h.asRepo.UpdateVisitScheduledDate(as.ASID, date, model.SyncScheduleConfirmed(date)); err != nil {
					return err
				}
			}
		}
		as2, err := h.asRepo.GetByID(currentOrg(c), as.ASID)
		if err != nil || as2 == nil {
			return echo.ErrNotFound
		}
		if memo := strings.TrimSpace(c.FormValue("memo")); memo != "" {
			as2.Symptom = memo
		}
		if name := strings.TrimSpace(c.FormValue("assignee")); name != "" {
			if err := checkAssigneeInOrg(h.userRepo, as2.OrgID, name, "", as2.AssignedTo); err != nil {
				return err
			}
			as2.AssignedTo = name
			as2.AssignedUserID = weeklyUserIDByName(h, c, name)
		}
		if err := h.asRepo.UpdateReceipt(as2); err != nil {
			return err
		}
		if h.asH != nil {
			h.asH.syncASPlannedDailyTask(as2.ASID)
		}
	default:
		return c.Redirect(http.StatusSeeOther, it.Href)
	}
	return c.Redirect(http.StatusSeeOther, back)
}

func (h *MeetingHandler) loadWeeklyEditTarget(c echo.Context) (model.WorkListItem, *model.ASReceipt, int) {
	if currentUserID(c) == "" {
		return model.WorkListItem{}, nil, http.StatusUnauthorized
	}
	prefix := strings.TrimSpace(c.QueryParam("prefix"))
	ref := strings.TrimSpace(c.QueryParam("ref"))
	if prefix == "" {
		prefix = strings.TrimSpace(c.FormValue("prefix"))
	}
	if ref == "" {
		ref = strings.TrimSpace(c.FormValue("ref"))
	}
	it := model.WorkListItem{Prefix: prefix, RefID: ref, Href: weeklyItemHref(prefix, ref)}
	if prefix == model.WorkPrefixAS {
		if h.asRepo == nil {
			return it, nil, http.StatusNotFound
		}
		as, err := h.asRepo.GetByID(currentOrg(c), ref)
		if err != nil || as == nil {
			return it, nil, http.StatusNotFound
		}
		it.Title = as.Symptom
		it.Assignee = as.AssignedTo
		it.Status = as.Status
		it.ScheduledDate = as.VisitScheduledDate
		it.Href = "/as/" + as.ASID
		return it, as, 0
	}
	if prefix == "" || ref == "" {
		return it, nil, http.StatusBadRequest
	}
	return it, nil, 0
}

func weeklyItemHref(prefix, ref string) string {
	switch prefix {
	case model.WorkPrefixAS:
		return "/as/" + ref
	case model.WorkPrefixMaintenance:
		return "/maintenance/visits/" + ref
	case model.WorkPrefixProjectWork:
		return model.WorkTaskOpenHref(ref, model.WBWorkProjectWork)
	default:
		if ref == "" {
			return ""
		}
		return "/workboard/tasks/" + ref
	}
}

func weeklyUserIDByName(h *MeetingHandler, c echo.Context, name string) string {
	if h == nil || h.userRepo == nil || strings.TrimSpace(name) == "" {
		return ""
	}
	if u := h.userRepo.FindAssignable(assignableOrgID(c), name); u != nil {
		return u.UserID
	}
	return ""
}

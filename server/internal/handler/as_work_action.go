package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

// WorkAction 하부업무 조치 화면 — 원 접수는 참고용이며 저장 시 원 접수를 변경하지 않는다.
func (h *ASHandler) WorkAction(c echo.Context) error {
	workID := c.Param("work_id")
	w, err := h.workRepo.GetByID(workID)
	if err != nil || w == nil {
		return echo.ErrNotFound
	}
	as, err := h.repo.GetByID(w.ASID)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	assignees, _ := h.userRepo.ListAssignable()
	return c.Render(http.StatusOK, "as/work_action.html", map[string]interface{}{
		"Title":        "하부업무 조치 · " + w.WorkNumber,
		"Active":       NavAS,
		"Work":         w,
		"AS":           as,
		"Assignees":    assignees,
		"CanWrite":     canProcessAS(c) || canReceiveAS(c) || canWriteWorkboard(c),
		"FlashOK":      c.QueryParam("ok"),
		"FlashErr":     c.QueryParam("err"),
		"ParentHref":   "/as/" + as.ASID,
		"ParentAction": "/as/" + as.ASID + "/action",
	})
}

// UpdateWorkAction 하부업무만 저장·완료. 원 접수 담당자·상태는 건드리지 않는다.
func (h *ASHandler) UpdateWorkAction(c echo.Context) error {
	if !(canProcessAS(c) || canReceiveAS(c) || canWriteWorkboard(c)) {
		return echo.ErrForbidden
	}
	workID := c.Param("work_id")
	w, err := h.workRepo.GetByID(workID)
	if err != nil || w == nil {
		return echo.ErrNotFound
	}
	w.ScheduledDate = normalizeVisitDate(c.FormValue("scheduled_date"))
	w.ScheduleConfirmed = c.FormValue("schedule_confirmed") == "1"
	w.ConfirmTarget = strings.TrimSpace(c.FormValue("confirm_target"))
	w.ConfirmContact = strings.TrimSpace(c.FormValue("confirm_contact"))
	w.Notes = strings.TrimSpace(c.FormValue("notes"))
	w.AssignedTo, w.AssignedUserID = resolveAssigneeFormFields(c, h.userRepo,
		"assigned_to_code", "assigned_to_custom", "assigned_to")

	if c.FormValue("mark_done") == "1" {
		w.Status = "done"
	}

	if err := h.workRepo.Update(w); err != nil {
		return err
	}
	msg := "하부업무를 저장했습니다"
	if w.Status == "done" {
		msg = "하부업무를 완료 처리했습니다(원 접수는 변경되지 않습니다)"
	}
	return c.Redirect(http.StatusSeeOther,
		"/as/work/"+workID+"/action?ok="+url.QueryEscape(msg))
}

func resolveAssigneeFormFields(c echo.Context, userRepo *repository.UserRepo, codeField, customField, legacyField string) (name, userID string) {
	if userRepo == nil {
		return strings.TrimSpace(c.FormValue(customField)), ""
	}
	assignedCode := c.FormValue(codeField)
	if assignedCode == "custom" {
		return strings.TrimSpace(c.FormValue(customField)), ""
	}
	if assignedCode != "" {
		if u, _ := userRepo.GetByID(assignedCode); u != nil {
			return u.FullName, u.UserID
		}
		return assignedCode, ""
	}
	if legacyField != "" {
		if v := strings.TrimSpace(c.FormValue(legacyField)); v != "" {
			if users, _ := userRepo.ListAssignable(); users != nil {
				for _, u := range users {
					if strings.TrimSpace(u.FullName) == v || u.Username == v {
						return u.FullName, u.UserID
					}
				}
			}
			return v, ""
		}
	}
	return "", ""
}

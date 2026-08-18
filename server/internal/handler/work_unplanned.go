package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// UnplannedList GET /plan/unplanned — §8 미계획 업무함
func (h *WorkHandler) UnplannedList(c echo.Context) error {
	role := currentRole(c)
	uid := currentUserID(c)
	keys := assigneeKeys(c)
	kind := strings.TrimSpace(c.QueryParam("kind"))

	mineParam := c.QueryParam("mine")
	mineUID, mineKeys := "", []string(nil)
	scopeAll := false
	if role == model.RoleTech {
		if mineParam == "0" {
			scopeAll = true
		} else {
			mineUID, mineKeys = uid, keys
		}
	} else if mineParam == "1" {
		mineUID, mineKeys = uid, keys
	} else {
		scopeAll = true
	}

	items, counts, err := h.repo.ListUnplanned(mineUID, mineKeys, kind)
	if err != nil {
		return err
	}

	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}

	showAssignee := role == model.RoleAdmin || role == model.RoleOffice || scopeAll
	canWrite := canWriteUnplanned(c)
	return c.Render(http.StatusOK, "plan/unplanned.html", map[string]interface{}{
		"Title":        "미계획 업무함",
		"Active":       "plan_unplanned",
		"Items":        items,
		"Total":        len(items),
		"Counts":       counts,
		"Kind":         kind,
		"ShowAssignee": showAssignee,
		"Role":         role,
		"Mine":         mineUID != "",
		"ScopeAll":     scopeAll,
		"ScopeNote":    unplannedScopeNote(role, scopeAll),
		"CanWrite":     canWrite,
		"Users":        users,
		"Err":          c.QueryParam("err"),
		"Ok":           c.QueryParam("ok"),
		"MineQ":        unplannedMineQuery(role, mineUID != ""),
	})
}

func unplannedScopeNote(role string, scopeAll bool) string {
	if role == model.RoleTech && !scopeAll {
		return "내 배정 업무 기준 · 담당자 미배정·정기점검 배정안됨은 팀 공통"
	}
	return "전체 업무 기준"
}

func unplannedMineQuery(role string, mine bool) string {
	if role == model.RoleTech {
		if mine {
			return "mine=1"
		}
		return "mine=0"
	}
	if mine {
		return "mine=1"
	}
	return ""
}

func canWriteUnplanned(c echo.Context) bool {
	if isObserverRole(c) {
		return false
	}
	role := currentRole(c)
	return role == model.RoleAdmin || role == model.RoleOffice || role == model.RoleTech
}

func unplannedBack(c echo.Context) string {
	kind := strings.TrimSpace(c.FormValue("kind"))
	if kind == "" {
		kind = strings.TrimSpace(c.QueryParam("kind"))
	}
	role := currentRole(c)
	mine := c.FormValue("mine")
	if mine == "" {
		mine = c.QueryParam("mine")
	}
	return planUnplannedURL(mine == "1" || (role == model.RoleTech && mine != "0"), role, kind)
}

// UnplannedAssign POST /plan/unplanned/assign — 단건·일괄 날짜 배정
func (h *WorkHandler) UnplannedAssign(c echo.Context) error {
	if !canWriteUnplanned(c) {
		return echo.ErrForbidden
	}
	date := normalizeVisitDate(c.FormValue("visit_date"))
	if date == "" {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=date"))
	}
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	assigneeUID := strings.TrimSpace(c.FormValue("assignee_user_id"))
	if assigneeUID == "" && h.userRepo != nil && assignee != "" {
		if users, err := h.userRepo.ListAssignable(); err == nil {
			for _, u := range users {
				if u.FullName == assignee || u.Username == assignee {
					assigneeUID = u.UserID
					if assignee == u.Username {
						assignee = u.FullName
					}
					break
				}
			}
		}
	}

	keys := c.Request().Form["item_key"]
	if len(keys) == 0 {
		if params, err := c.FormParams(); err == nil {
			keys = params["item_key"]
		}
	}
	if len(keys) == 0 {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=select"))
	}

	n := 0
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if err := h.repo.AssignUnplannedDate(key, date, assignee, assigneeUID); err != nil {
			continue
		}
		n++
		if strings.HasPrefix(key, "as:") {
			h.syncASPlannedFromUnplanned(strings.TrimPrefix(key, "as:"))
		}
		if strings.HasPrefix(key, "slot:") && h.wbRepo != nil {
			_, _ = h.wbRepo.EnsureMaintenanceTasks()
		}
	}
	back := unplannedBack(c)
	if n == 0 {
		return c.Redirect(http.StatusSeeOther, back+qjoin(back, "err=assign"))
	}
	return c.Redirect(http.StatusSeeOther, back+qjoin(back, "ok=assign"))
}

// UnplannedNoDate POST /plan/unplanned/no-date — 미정+사유
func (h *WorkHandler) UnplannedNoDate(c echo.Context) error {
	if !canWriteUnplanned(c) {
		return echo.ErrForbidden
	}
	if h.asRepo == nil {
		return echo.ErrForbidden
	}
	key := strings.TrimSpace(c.FormValue("item_key"))
	if !strings.HasPrefix(key, "as:") {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=nodate"))
	}
	reason := model.FormatNoDateReason(c.FormValue("no_date_reason"), c.FormValue("no_date_detail"))
	if strings.TrimSpace(reason) == "" {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=reason"))
	}
	asID := strings.TrimPrefix(key, "as:")
	if err := h.asRepo.SetScheduleNoDate(asID, reason); err != nil {
		return c.Redirect(http.StatusSeeOther, unplannedBack(c)+qjoin(unplannedBack(c), "err=nodate"))
	}
	back := unplannedBack(c)
	return c.Redirect(http.StatusSeeOther, back+qjoin(back, "ok=nodate"))
}

func (h *WorkHandler) syncASPlannedFromUnplanned(asID string) {
	if h.wbRepo == nil || h.asRepo == nil || strings.TrimSpace(asID) == "" {
		return
	}
	as, err := h.asRepo.GetByID(asID)
	if err != nil || as == nil {
		return
	}
	visit := strings.TrimSpace(as.VisitScheduledDate)
	if visit == "" {
		return
	}
	existing, _ := h.wbRepo.GetTaskBySource(model.WBSourceAS, as.ASID)
	title := model.FormatASWorkTitle(as.OrgName, as.ASNumber)
	if existing != nil {
		existing.DueDate = visit
		existing.Title = title
		if strings.TrimSpace(existing.Description) == "" {
			existing.Description = as.Symptom
		}
		if strings.TrimSpace(existing.Assignee) == "" {
			existing.Assignee = as.AssignedTo
		}
		if strings.TrimSpace(existing.StartTime) == "" {
			existing.WorkDate = visit
		}
		_ = h.wbRepo.UpdateTask(existing)
		return
	}
	t := &model.WorkTask{
		WorkType:    model.WBWorkAS,
		Title:       title,
		Description: as.Symptom,
		DueDate:     visit,
		WorkDate:    visit,
		DurationMin: 30,
		Status:      model.WBTaskWaiting,
		Priority:    model.WBPriorityNormal,
		Assignee:    strings.TrimSpace(as.AssignedTo),
		SourceType:  model.WBSourceAS,
		SourceID:    as.ASID,
	}
	_ = h.wbRepo.CreateTask(t)
}

func qjoin(url, extra string) string {
	if strings.Contains(url, "?") {
		return "&" + extra
	}
	return "?" + extra
}

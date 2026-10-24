package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const weeklyTaskLinkLimit = 200

func (h *StatsHandler) LinkWeeklyTasksProject(c echo.Context) error {
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return echo.ErrForbidden
	}
	orgID, err := resolveInsertOrg(c)
	if err != nil {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	ids := c.Request().Form["task_id"]
	if len(ids) == 0 {
		if one := strings.TrimSpace(c.FormValue("task_id")); one != "" {
			ids = []string{one}
		}
	}
	if len(ids) > weeklyTaskLinkLimit {
		return echo.NewHTTPError(http.StatusBadRequest, "한 번에 200건까지 연결할 수 있습니다")
	}
	pid := strings.TrimSpace(c.FormValue("project_id"))
	if pid == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 선택하세요")
	}
	proj, ok, err := h.weeklyLinkProject(orgID, pid)
	if err != nil {
		return err
	}
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "목록에 없는 사업입니다")
	}
	if proj.OrgID != "" && proj.OrgID != orgID && c.FormValue("confirm_foreign") != "1" {
		return echo.NewHTTPError(http.StatusBadRequest, "타 조직 사업 확인이 필요합니다")
	}

	var okN int
	var warns []string
	var linked []string
	unlock := repository.NewASUnlockRepo(h.repo.DB())
	for _, raw := range ids {
		tid := strings.TrimSpace(raw)
		if tid == "" {
			continue
		}
		t, err := h.wbRepo.GetTask(tid)
		if err != nil || t == nil {
			warns = append(warns, tid+": 업무가 없습니다")
			continue
		}
		if err := denyOtherOrgTask(c, t); err != nil {
			return echo.ErrNotFound
		}
		if t.OrgID != orgID {
			return echo.ErrNotFound
		}
		if !h.canLinkWeeklyTask(c, t) {
			warns = append(warns, t.Title+": 권한이 없습니다")
			continue
		}
		if isProjectWorkClosed(t) || t.Status == model.WBTaskComplete || t.Status == model.WBTaskCancelled {
			unlocked, _, _ := unlock.HasActive(t.TaskID, currentUserID(c))
			if !unlocked {
				warns = append(warns, t.Title+": 완료·취소 건은 수정 비밀번호로 잠금 해제한 뒤 연결하세요")
				continue
			}
		}
		before, after, err := h.wbRepo.SetTaskProject(t.OrgID, t.TaskID, pid)
		if err != nil {
			warns = append(warns, t.Title+": "+err.Error())
			continue
		}
		line := "사업 연결: "
		if before == "" {
			line += "없음"
		} else {
			line += before
		}
		line += " → "
		if after == "" {
			line += "없음"
		} else {
			line += after
		}
		_, _ = h.wbRepo.InsertWorkTaskProcess(model.WorkTaskProcess{
			TaskID: t.TaskID, OrgID: t.OrgID, Worker: ctxString(c, "user_name"),
			WorkerUserID: currentUserID(c), WorkContent: line,
		})
		okN++
		linked = append(linked, t.TaskID)
	}
	if okN > 0 {
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionUpdate, TargetTable: "work_tasks", TargetID: strings.Join(linked, ","),
			Reason: fmt.Sprintf("주간보고 사업 연결 %s → %s", strings.Join(linked, ","), pid), Result: auditlog.ResultOK,
		})
	}
	q := url.Values{}
	if okN > 0 {
		q.Set("ok", fmt.Sprintf("업무 %d건을 사업에 연결했습니다", okN))
	}
	if len(warns) > 0 {
		q.Set("err", strings.Join(warns, "; "))
	}
	if has, _ := h.repo.HasWeeklyReportRowForProject(orgID, pid); !has {
		q.Set("need_row", pid)
	}
	return c.Redirect(http.StatusSeeOther, weeklyAppendQuery(weeklySafeNext(c, orgID), q))
}

func (h *StatsHandler) weeklyLinkProject(orgID, projectID string) (model.WorkProject, bool, error) {
	list, err := h.repo.ListWeeklyRowProjects(orgID)
	if err != nil {
		return model.WorkProject{}, false, err
	}
	for _, p := range list {
		if p.ProjectID == projectID {
			return p, true, nil
		}
	}
	return model.WorkProject{}, false, nil
}

func (h *StatsHandler) canLinkWeeklyTask(c echo.Context, t *model.WorkTask) bool {
	if t == nil || isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	if currentRole(c) == model.RoleVisionAdmin {
		org := strings.TrimSpace(nz(c.FormValue("org"), currentOrg(c)))
		return org != "" && org != repository.OrgAll && org == t.OrgID
	}
	if currentRole(c) == model.RoleOrgAdmin && loginHomeOrg(c) == t.OrgID {
		return true
	}
	if canEditTask(c, t.Assignee, t.AssigneeUserID) {
		return true
	}
	if h.wbRepo == nil {
		return false
	}
	members, _ := h.wbRepo.ListMembers(t.TaskID)
	for _, m := range members {
		if assigneeIsMine(c, m.Assignee, m.UserID) {
			return true
		}
	}
	return false
}

package handler

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *WorkboardHandler) createProjectWork(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	if currentRole(c) == model.RoleCLevel || isReadOnly(c) {
		return echo.ErrForbidden
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		return echo.NewHTTPError(http.StatusBadRequest, "조직을 선택하세요")
	}
	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" || utf8.RuneCountInString(title) > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "작업명을 입력하세요")
	}
	sub := model.NormalizeWorkSubtype(c.FormValue("work_subtype"))
	if sub == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "작업 구분을 선택하세요")
	}
	workDay := strings.TrimSpace(c.FormValue("work_date"))
	if workDay == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "작업일을 입력하세요")
	}
	if d, err := model.ParseAppDate(workDay); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "작업일을 확인하세요")
	} else {
		workDay = d
	}
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	if assignee == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "담당자를 선택하세요")
	}
	projectID := strings.TrimSpace(c.FormValue("project_id"))
	_ = c.Request().ParseForm()
	siteIDs := c.Request().Form["site_id"]
	if projectID != "" {
		pOrg, err := h.repo.ProjectOrgID(projectID)
		if err != nil || pOrg != org {
			return echo.NewHTTPError(http.StatusBadRequest, "다른 조직 사업입니다")
		}
		if err := h.repo.AssertSitesForProject(org, projectID, siteIDs); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	} else if len(nonEmpty(siteIDs)) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 선택하면 대상 사이트를 고를 수 있습니다")
	}

	start := strings.TrimSpace(c.FormValue("start_time"))
	end := strings.TrimSpace(c.FormValue("end_time"))
	dur := 30
	workDate := ""
	if start != "" || end != "" {
		if start == "" || end == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "시작·종료 시간을 함께 입력하세요")
		}
		end, dur = parseDurationForm(start, end, c.FormValue("duration_min"), dur)
		workDate = workDay
		if !h.canEditRegisterDate(c, workDate) {
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=past"))
		}
	}
	supports := parseSupportMembers(c, assignee)
	if workDate != "" {
		if msg := h.firstParticipantOverlap(workDate, start, end, "", occupancyNames(assignee, supports)); msg != "" {
			return overlapRedirect(c, msg)
		}
	}
	if err := checkAssigneeInOrg(h.userRepo, org, assignee, "", ""); err != nil {
		return err
	}
	st, pr := createTaskStatusPriority(c)
	t := &model.WorkTask{
		WorkType:    model.WBWorkProjectWork,
		WorkSubtype: sub,
		ProjectID:   projectID,
		Title:       title,
		Description: strings.TrimSpace(c.FormValue("description")),
		DueDate:     workDay,
		WorkDate:    workDate,
		StartTime:   start,
		EndTime:     end,
		DurationMin: dur,
		Status:      st,
		Priority:    pr,
		Assignee:    assignee,
		OrgID:       org,
	}
	if err := h.repo.CreateTask(t); err != nil {
		return err
	}
	recordTaskNotice(h.notices, c, t, "")
	if err := h.saveSupportMembers(c, t.TaskID, t.Assignee); err != nil {
		return err
	}
	if projectID != "" {
		if err := h.repo.ReplaceTaskSites(t.TaskID, siteIDs); err != nil {
			return err
		}
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=task"))
}

func nonEmpty(ids []string) []string {
	var out []string
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			out = append(out, strings.TrimSpace(id))
		}
	}
	return out
}

func (h *WorkboardHandler) SearchProjectWorkSites(c echo.Context) error {
	if !hasPerm(c, model.PermWorkboard) {
		return echo.ErrForbidden
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		return c.JSON(http.StatusOK, []repository.WorkTaskSite{})
	}
	pid := strings.TrimSpace(c.QueryParam("project_id"))
	if pid == "" {
		return c.JSON(http.StatusOK, []repository.WorkTaskSite{})
	}
	pOrg, err := h.repo.ProjectOrgID(pid)
	if err != nil || pOrg != org {
		return echo.NewHTTPError(http.StatusBadRequest, "다른 조직 사업입니다")
	}
	list, err := h.repo.SearchProjectSites(org, pid, c.QueryParam("q"), 20)
	if err != nil {
		return err
	}
	if list == nil {
		list = []repository.WorkTaskSite{}
	}
	return c.JSON(http.StatusOK, list)
}

func (h *WorkboardHandler) attachProjectWorkSites(tasks []model.WorkTask) {
	for i := range tasks {
		if tasks[i].WorkType != model.WBWorkProjectWork {
			continue
		}
		line, err := h.repo.TaskSiteLine(tasks[i].TaskID)
		if err == nil {
			tasks[i].SiteLine = line
		}
	}
}

func (h *WorkboardHandler) paletteWorkCards(orgID string) ([]model.WBCard, error) {
	tasks, err := h.repo.ListUnplacedTasks(orgID, []string{model.WBWorkProjectWork})
	if err != nil {
		return nil, err
	}
	var out []model.WBCard
	for _, t := range tasks {
		out = append(out, paletteTaskCard(t, false))
	}
	return out, nil
}

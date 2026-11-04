package handler

import (
	"net/http"
	"net/url"
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
	org, orgErr := resolveInsertOrg(c)
	if orgErr != nil {
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
	dueDay := strings.TrimSpace(c.FormValue("due_date"))
	if dueDay != "" {
		if d, err := model.ParseAppDate(dueDay); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "종료일을 확인하세요")
		} else {
			dueDay = d
		}
	}
	if dueDay == "" {
		dueDay = workDay
	}
	if dueBeforeWork(workDay, dueDay) {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=date_order"))
	}
	sel, selErr := h.taskSelectionFromForm(c, true)
	if selErr != nil {
		return rejectPickerErr(c, selErr, redirectBack(c, ""))
	}
	assignee := strings.TrimSpace(sel.PrimaryName)
	if assignee == "" {
		assignee = strings.TrimSpace(c.FormValue("assignee"))
	}
	if assignee == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "담당자를 선택하세요")
	}
	projectID := strings.TrimSpace(c.FormValue("project_id"))
	_ = c.Request().ParseForm()
	siteIDs := c.Request().Form["site_id"]
	if err := h.assertProjectLinkable(c, projectID); err != nil {
		return err
	}
	if err := h.repo.AssertCustomersActive(siteIDs); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
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
		planDates := collectPlanDatesFromForm(c, workDay, dueDay, h.holidayYearMissing())
		if err := h.rejectIfOverlapDates(c, planDates, start, end, "", occupancyNames(assignee, supports)); err != nil {
			return overlapDone(err)
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
		DueDate:     dueDay,
		WorkDate:    firstNonEmpty(workDate, workDay),
		StartTime:   start,
		EndTime:     end,
		DurationMin: dur,
		Status:      st,
		Priority:    pr,
		Assignee:    assignee,
		OrgID:       org,
	}
	if err := h.applyWorkGroupForm(c, t); err != nil {
		return err
	}
	if err := h.repo.CreateTask(t); err != nil {
		return err
	}
	if err := h.applyPendingInterrupt(c, t.TaskID); err != nil {
		return overlapDone(err)
	}
	h.applyPlanWorkPlace(c, t.TaskID)
	recordTaskNotice(h.notices, c, t, "")
	if err := h.saveSupportMembers(c, t.TaskID, t.Assignee); err != nil {
		return err
	}
	if err := h.saveWorkItemsFromForm(c, t.TaskID); err != nil {
		setFlashErrCookie(c, err.Error())
		return c.Redirect(http.StatusSeeOther, taskPagePath(t.TaskID, false)+"?err=item")
	}
	if !sel.Empty() {
		applySelectionToTask(t, sel)
		_ = h.repo.UpdateTask(t)
		_ = h.saveSelectionOnTask(t.TaskID, sel)
	}
	if err := h.repo.ReplaceTaskCustomers(t.TaskID, t.ProjectID, siteIDs); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=task"))
}

func (h *WorkboardHandler) SearchProjectWorkSites(c echo.Context) error {
	return h.SearchWorkTaskSites(c)
}

func (h *WorkboardHandler) SaveSiteCheck(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if !canCheckTaskSiteProgress(c, t) {
		return echo.ErrForbidden
	}
	cid := strings.TrimSpace(c.FormValue("customer_id"))
	day := strings.TrimSpace(c.FormValue("work_date"))
	st := strings.TrimSpace(c.FormValue("status"))
	if err := h.repo.UpsertSiteCheck(t.TaskID, cid, day, st, currentUserID(c)); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	back := strings.TrimSpace(c.FormValue("back"))
	loc := "/workboard/tasks/" + t.TaskID + "?ok=saved"
	if back != "" {
		loc += "&back=" + url.QueryEscape(back)
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

func (h *WorkboardHandler) SearchWorkTaskSites(c echo.Context) error {
	if !hasPerm(c, model.PermWorkboard) {
		return echo.ErrForbidden
	}
	if currentRole(c) == model.RoleCLevel || isReadOnly(c) {
		return echo.ErrForbidden
	}
	pid := strings.TrimSpace(c.QueryParam("project_id"))
	if pid != "" {
		if _, err := h.repo.ProjectOrgID(pid); err != nil {
			pid = ""
		}
	}
	var selected []string
	for _, id := range strings.Split(c.QueryParam("selected"), ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			selected = append(selected, id)
		}
	}
	list, err := h.repo.SearchWorkTaskCustomers(c.QueryParam("q"), pid, selected, 20)
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

func (h *WorkboardHandler) assertProjectExists(projectID string) error {
	return h.assertProjectLinkable(nil, projectID)
}

func (h *WorkboardHandler) assertProjectLinkable(c echo.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	p, err := h.repo.GetProject(projectID)
	if err != nil || p == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 찾을 수 없습니다")
	}
	return requireProjectLink(c, func(id string) (*model.WorkProject, error) {
		return p, nil
	}, projectID, false)
}

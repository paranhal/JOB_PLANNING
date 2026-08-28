package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type AdminWorkHandler struct {
	repo         *repository.WBRepo
	userRepo     *repository.UserRepo
	customerRepo *repository.CustomerRepo
}

func NewAdminWorkHandler(repo *repository.WBRepo, userRepo *repository.UserRepo, customerRepo *repository.CustomerRepo) *AdminWorkHandler {
	return &AdminWorkHandler{repo: repo, userRepo: userRepo, customerRepo: customerRepo}
}

func (h *AdminWorkHandler) List(c echo.Context) error {
	status := strings.TrimSpace(c.QueryParam("status"))
	search := strings.TrimSpace(c.QueryParam("search"))
	view := strings.TrimSpace(c.QueryParam("view"))
	if view != "kanban" {
		view = "list"
	}
	sort, dir := model.NormalizeAdminWorkSort(c.QueryParam("sort"), c.QueryParam("dir"))
	items, err := h.repo.ListAdminWorkSorted(status, search, sort, dir)
	if err != nil {
		return err
	}
	fillWaitingActionCounts(h.repo, items)
	h.repo.FillTaskDepth(items)
	projects, _ := h.repo.ListProjects(true)
	assignees, _ := h.userRepo.ListAssignable()
	customers, _ := h.customerRepo.ListAll()
	flashErr := c.QueryParam("err")
	byStatus := map[string][]model.WorkTask{
		model.WBTaskWaiting:    {},
		model.WBTaskInProgress: {},
		model.WBTaskComplete:   {},
	}
	for _, t := range items {
		b := model.WBKanbanBucket(t.Status)
		if b == "" {
			continue
		}
		byStatus[b] = append(byStatus[b], t)
	}
	return c.Render(http.StatusOK, "admin_work/list.html", map[string]interface{}{
		"Title":        "행정관련업무등록/처리",
		"Active":       NavAdminWork,
		"Items":        items,
		"ByStatus":     byStatus,
		"Status":       status,
		"Search":       search,
		"View":         view,
		"Sort":         sort,
		"Dir":          dir,
		"SortHref":     adminWorkSortHrefs("/admin-work", status, search, view, sort, dir),
		"QuerySuffix":  adminWorkQuerySuffix(status, search, view, sort, dir),
		"Total":        len(items),
		"Today":        time.Now().Format("2006-01-02"),
		"CanWrite":     canWriteWorkboard(c),
		"Projects":     projects,
		"Assignees":    assignees,
		"Customers":    customers,
		"FlashOK":      c.QueryParam("ok"),
		"FlashErr":     flashErr,
		"FlashErrText": gtdFlashMsg(flashErr, c.QueryParam("n")),
	})
}

func (h *AdminWorkHandler) Stats(c echo.Context) error {
	st, err := h.repo.CountAdminWorkStats()
	if err != nil {
		return err
	}
	status := strings.TrimSpace(c.QueryParam("status"))
	if status == "" {
		status = "all"
	}
	sort, dir := model.NormalizeAdminWorkSort(c.QueryParam("sort"), c.QueryParam("dir"))
	items, err := h.repo.ListAdminWorkSorted(status, "", sort, dir)
	if err != nil {
		return err
	}
	fillWaitingActionCounts(h.repo, items)
	h.repo.FillTaskDepth(items)
	projects, _ := h.repo.ListProjects(true)
	assignees, _ := h.userRepo.ListAssignable()
	customers, _ := h.customerRepo.ListAll()
	return c.Render(http.StatusOK, "admin_work/stats.html", map[string]interface{}{
		"Title":      "행정관련업무현황",
		"Active":     NavAdminWorkStats,
		"Stats":      st,
		"Items":      items,
		"ListStatus":  status,
		"Sort":        sort,
		"Dir":         dir,
		"SortHref":    adminWorkSortHrefs("/admin-work/stats", status, "", "list", sort, dir),
		"QuerySuffix": adminWorkQuerySuffix(status, "", "list", sort, dir),
		"Total":       len(items),
		"Today":      time.Now().Format("2006-01-02"),
		"FromStats":  true,
		"CanWrite":   canWriteWorkboard(c),
		"Projects":   projects,
		"Assignees":  assignees,
		"Customers":  customers,
	})
}

func (h *AdminWorkHandler) New(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	projects, _ := h.repo.ListProjects(true)
	assignees, _ := h.userRepo.ListAssignable()
	customers, _ := h.customerRepo.ListAll()
	data := map[string]interface{}{
		"Title":     "행정관련업무 등록",
		"Active":    NavAdminWork,
		"Projects":  projects,
		"Assignees": assignees,
		"Customers": customers,
		"Today":     time.Now().Format("2006-01-02"),
		"FlashErr":  c.QueryParam("err"),
	}
	parentID := strings.TrimSpace(c.QueryParam("parent"))
	if parentID != "" {
		parent, err := h.repo.GetTask(parentID)
		if err != nil || parent == nil {
			data["FlashErr"] = "sub_parent"
		} else if err := h.repo.CanAttachSubtask(parentID); err != nil {
			data["FlashErr"] = subtaskErrQuery(err)
		} else {
			data["Parent"] = parent
		}
	} else {
		choices, _ := h.repo.ListSubtaskParentCandidates("")
		data["ParentChoices"] = choices
	}
	return c.Render(http.StatusOK, "admin_work/form.html", data)
}

func (h *AdminWorkHandler) Create(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	workType := strings.TrimSpace(c.FormValue("work_type"))
	if workType != model.WBWorkSupport {
		workType = model.WBWorkAdmin
	}
	dueDate := strings.TrimSpace(c.FormValue("due_date"))
	workDate := strings.TrimSpace(c.FormValue("work_date"))
	if dueDate != "" {
		d, err := model.ParseAppDate(dueDate)
		if err != nil {
			return c.Redirect(http.StatusSeeOther, "/admin-work/new?err=task")
		}
		dueDate = d
	}
	if workDate != "" {
		d, err := model.ParseAppDate(workDate)
		if err != nil {
			return c.Redirect(http.StatusSeeOther, "/admin-work/new?err=task")
		}
		workDate = d
	}
	title := strings.TrimSpace(c.FormValue("title"))
	status, priority := createTaskStatusPriority(c)
	parentID := strings.TrimSpace(c.FormValue("parent_task_id"))
	fail := func(code string) error {
		loc := "/admin-work/new?err=" + code
		if parentID != "" {
			loc += "&parent=" + url.QueryEscape(parentID)
		}
		return c.Redirect(http.StatusSeeOther, loc)
	}
	if title == "" {
		return fail("task")
	}
	if dueUndeterminedFromForm(c) {
		if model.FormatNoDateReason(c.FormValue("no_date_reason"), c.FormValue("no_date_detail")) == "" {
			return fail("task")
		}
		dueDate = ""
	} else if status != model.WBTaskInbox && dueDate == "" {
		return fail("task")
	}
	if workType == model.WBWorkSupport && strings.TrimSpace(c.FormValue("project_id")) == "" {
		return fail("project_required")
	}
	progress := 0
	if p := strings.TrimSpace(c.FormValue("progress")); p != "" {
		fmtScanInt(p, &progress)
	}
	t := &model.WorkTask{
		WorkType:    workType,
		ProjectID:   strings.TrimSpace(c.FormValue("project_id")),
		Title:       title,
		Description: strings.TrimSpace(c.FormValue("description")),
		DueDate:     dueDate,
		WorkDate:    workDate,
		StartTime:   strings.TrimSpace(c.FormValue("start_time")),
		EndTime:     strings.TrimSpace(c.FormValue("end_time")),
		DurationMin: 30,
		Status:      status,
		Priority:    priority,
		Assignee:     strings.TrimSpace(c.FormValue("assignee")),
		Progress:     progress,
		ParentTaskID: parentID,
	}
	if d := strings.TrimSpace(c.FormValue("duration_min")); d != "" {
		fmtScanInt(d, &t.DurationMin)
	}
	applyCustomerForm(t, c)
	if model.IsAdminGTDTask(*t) {
		applyAdminGTDForm(t, c)
		if code := model.AdminGTDErr(t.Status, t.HoldReason, t.ReviewDate, t.CancelReason,
			t.WaitParty, t.WaitRequest, t.ReplyDueDate, t.NextCheckDate, t.CompleteNote,
			0, 0, isAdminRole(c), strings.TrimSpace(c.FormValue("force_complete")) == "1",
			strings.TrimSpace(c.FormValue("force_reason"))); code != "" {
			return fail(code)
		}
	}
	if err := h.repo.CreateTask(t); err != nil {
		if q := subtaskErrQuery(err); q != "" {
			return fail(q)
		}
		return err
	}
	if applied, code := applyRecurrenceFromForm(c, h.repo, t, nil); code != "" {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+url.PathEscape(t.TaskID)+"?err="+code)
	} else if applied {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+url.PathEscape(t.TaskID)+"?ok=rec_gen")
	}
	if t.Status == model.WBTaskComplete && t.CompleteNote != "" {
		_ = h.repo.CreateActivity(&model.WorkActivity{
			TaskID: t.TaskID, ActivityType: model.WBActivityDone,
			Content: t.CompleteNote, Actor: ctxString(c, "user_name"),
			SpentMinutes: model.WBActivityDefaultSpent,
		})
	}
	if t.Status == model.WBTaskWaitingFor {
		title := t.WaitRequest
		if title == "" {
			title = "회신 대기"
		}
		_ = h.repo.CreateAction(&model.WorkAction{
			TaskID: t.TaskID, Title: title, Status: model.WBActionWaiting, Required: true,
			WaitPartyKind: t.WaitPartyKind, WaitParty: t.WaitParty, WaitRequest: t.WaitRequest,
			ReplyDueDate: t.ReplyDueDate, NextCheckDate: t.NextCheckDate,
		})
	}
	if t.ParentTaskID != "" {
		back := strings.TrimSpace(c.FormValue("redirect"))
		if back == "" {
			back = "/workboard/tasks/" + url.PathEscape(t.ParentTaskID) + "?ok=sub"
		}
		return c.Redirect(http.StatusSeeOther, back)
	}
	return c.Redirect(http.StatusSeeOther, "/admin-work?ok=task")
}

func (h *AdminWorkHandler) Show(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	back := "/admin-work"
	if ref := strings.TrimSpace(c.QueryParam("from")); ref == "stats" {
		back = "/admin-work/stats"
	}
	return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+url.PathEscape(id)+"?back="+url.QueryEscape(back))
}

func subtaskErrQuery(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, model.ErrSubtaskDepth):
		return "sub_depth"
	case errors.Is(err, model.ErrSubtaskCycle):
		return "sub_cycle"
	case errors.Is(err, model.ErrSubtaskOccur):
		return "sub_occur"
	case errors.Is(err, model.ErrSubtaskParent):
		return "sub_parent"
	}
	return ""
}

func fillWaitingActionCounts(repo *repository.WBRepo, items []model.WorkTask) {
	if repo == nil || len(items) == 0 {
		return
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.TaskID)
	}
	counts, err := repo.CountWaitingActionsByTasks(ids)
	if err != nil {
		return
	}
	for i := range items {
		items[i].WaitingActionCount = counts[items[i].TaskID]
	}
}

// createTaskStatusPriority 등록 폼에 상태·우선순위가 없으면 기본값. 컬럼은 유지한다(§33.2).
func createTaskStatusPriority(c echo.Context) (status, priority string) {
	status = strings.TrimSpace(c.FormValue("status"))
	if status == "" {
		status = model.WBTaskWaiting
	}
	priority = strings.TrimSpace(c.FormValue("priority"))
	if priority == "" {
		priority = model.WBPriorityNormal
	}
	return status, priority
}

func dueUndeterminedFromForm(c echo.Context) bool {
	v := strings.TrimSpace(c.FormValue("due_undetermined"))
	return v == "1" || v == "on" || v == "true"
}

func adminWorkQuerySuffix(status, search, view, sort, dir string) string {
	q := url.Values{}
	if status != "" && status != "all" {
		q.Set("status", status)
	}
	if search != "" {
		q.Set("search", search)
	}
	_ = view
	sort, dir = model.NormalizeAdminWorkSort(sort, dir)
	if sort != "due_date" || dir != "asc" {
		q.Set("sort", sort)
		q.Set("dir", dir)
	}
	enc := q.Encode()
	if enc == "" {
		return ""
	}
	return "&" + enc
}

func adminWorkSortHrefs(path, status, search, view, curSort, curDir string) map[string]string {
	if path == "" {
		path = "/admin-work"
	}
	curSort, curDir = model.NormalizeAdminWorkSort(curSort, curDir)
	out := map[string]string{}
	for _, col := range []string{"task_id", "title", "customer", "assignee", "work_date", "due_date", "status", "duration", "created_at"} {
		dir := "asc"
		if curSort == col && curDir == "asc" {
			dir = "desc"
		}
		q := url.Values{}
		if status != "" && status != "all" {
			q.Set("status", status)
		}
		if search != "" {
			q.Set("search", search)
		}
		if view == "kanban" {
			q.Set("view", "kanban")
		}
		q.Set("sort", col)
		q.Set("dir", dir)
		out[col] = path + "?" + q.Encode()
	}
	return out
}

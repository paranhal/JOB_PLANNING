package handler

import (
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
	items, err := h.repo.ListAdminWork(status, search)
	if err != nil {
		return err
	}
	fillWaitingActionCounts(h.repo, items)
	flashErr := c.QueryParam("err")
	return c.Render(http.StatusOK, "admin_work/list.html", map[string]interface{}{
		"Title":        "행정관련업무등록/처리",
		"Active":       "admin_work",
		"Items":        items,
		"Status":       status,
		"Search":       search,
		"Total":        len(items),
		"Today":        time.Now().Format("2006-01-02"),
		"CanWrite":     canWriteWorkboard(c),
		"FlashOK":      c.QueryParam("ok"),
		"FlashErr":     flashErr,
		"FlashErrText": gtdFlash(flashErr),
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
	items, err := h.repo.ListAdminWork(status, "")
	if err != nil {
		return err
	}
	fillWaitingActionCounts(h.repo, items)
	return c.Render(http.StatusOK, "admin_work/stats.html", map[string]interface{}{
		"Title":      "행정관련업무현황",
		"Active":     "admin_work_stats",
		"Stats":      st,
		"Items":      items,
		"ListStatus": status,
		"Total":      len(items),
		"Today":      time.Now().Format("2006-01-02"),
		"FromStats":  true,
		"CanWrite":   canWriteWorkboard(c),
	})
}

func (h *AdminWorkHandler) New(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	projects, _ := h.repo.ListProjects(true)
	assignees, _ := h.userRepo.ListAssignable()
	customers, _ := h.customerRepo.ListAll()
	return c.Render(http.StatusOK, "admin_work/form.html", map[string]interface{}{
		"Title":     "행정관련업무 등록",
		"Active":    "admin_work",
		"Projects":  projects,
		"Assignees": assignees,
		"Customers": customers,
		"Today":     time.Now().Format("2006-01-02"),
		"FlashErr":  c.QueryParam("err"),
	})
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
	title := strings.TrimSpace(c.FormValue("title"))
	status := strings.TrimSpace(c.FormValue("status"))
	if title == "" {
		return c.Redirect(http.StatusSeeOther, "/admin-work/new?err=task")
	}
	if status != model.WBTaskInbox && dueDate == "" {
		return c.Redirect(http.StatusSeeOther, "/admin-work/new?err=task")
	}
	if workType == model.WBWorkSupport && strings.TrimSpace(c.FormValue("project_id")) == "" {
		return c.Redirect(http.StatusSeeOther, "/admin-work/new?err=project_required")
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
		Priority:    strings.TrimSpace(c.FormValue("priority")),
		Assignee:    strings.TrimSpace(c.FormValue("assignee")),
		Progress:    progress,
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
			return c.Redirect(http.StatusSeeOther, "/admin-work/new?err="+code)
		}
	}
	if err := h.repo.CreateTask(t); err != nil {
		return err
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

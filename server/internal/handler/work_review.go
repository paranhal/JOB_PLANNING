package handler

import (
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func workReviewBack(c echo.Context) string {
	back := strings.TrimSpace(c.FormValue("back"))
	if strings.HasPrefix(back, "/work") {
		return back
	}
	return "/work?review=1"
}

func reviewFormItem(c echo.Context) (prefix, refID string) {
	return strings.TrimSpace(c.FormValue("prefix")), strings.TrimSpace(c.FormValue("ref_id"))
}

func reviewUnplannedKey(prefix, refID string) string {
	refID = strings.TrimSpace(refID)
	if refID == "" {
		return ""
	}
	switch strings.TrimSpace(prefix) {
	case model.WorkPrefixAS:
		return "as:" + refID
	case model.WorkPrefixMaintenance:
		return "mnt:" + refID
	case model.WorkPrefixConfirm:
		return "wi:" + refID
	default:
		return "task:" + refID
	}
}

func reviewNoticeSource(prefix, refID, taskID string) (sourceType, sourceID string) {
	switch strings.TrimSpace(prefix) {
	case model.WorkPrefixAS:
		return model.AssignNoticeSourceAS, strings.TrimSpace(refID)
	case model.WorkPrefixMaintenance:
		return model.AssignNoticeSourceMaintenance, strings.TrimSpace(refID)
	default:
		sid := strings.TrimSpace(taskID)
		if sid == "" {
			sid = strings.TrimSpace(refID)
		}
		return model.AssignNoticeSourceTask, sid
	}
}

func (h *WorkHandler) ensureReviewTask(prefix, refID string) (*model.WorkTask, error) {
	if h == nil || h.wbRepo == nil {
		return nil, nil
	}
	prefix, refID = strings.TrimSpace(prefix), strings.TrimSpace(refID)
	if refID == "" {
		return nil, nil
	}
	switch prefix {
	case model.WorkPrefixAS:
		t, err := h.wbRepo.GetTaskBySource(model.WBSourceAS, refID)
		if err != nil || t != nil {
			return t, err
		}
		if h.asRepo == nil {
			return nil, nil
		}
		as, err := h.asRepo.GetByID(refID)
		if err != nil || as == nil {
			return nil, err
		}
		if err := h.wbRepo.SyncASDailyTask(as); err != nil {
			return nil, err
		}
		return h.wbRepo.GetTaskBySource(model.WBSourceAS, refID)
	case model.WorkPrefixMaintenance:
		t, err := h.wbRepo.GetTaskBySource(model.WBSourceMaintenance, refID)
		if err != nil || t != nil {
			return t, err
		}
		if _, err := h.wbRepo.SyncMaintenanceTaskFromVisit(refID); err != nil {
			return nil, err
		}
		return h.wbRepo.GetTaskBySource(model.WBSourceMaintenance, refID)
	default:
		return h.wbRepo.GetTask(refID)
	}
}

// ReviewReschedule 지연 건의 날짜를 다시 잡는다. 오늘·내일·직접. §42.7
func (h *WorkHandler) ReviewReschedule(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return c.Redirect(http.StatusSeeOther, workReviewBack(c))
	}
	prefix, refID := reviewFormItem(c)
	date, err := model.ParseAppDate(c.FormValue("work_date"))
	if err != nil || date == "" {
		return c.Redirect(http.StatusSeeOther, qjoin(workReviewBack(c), "err=date"))
	}
	key := reviewUnplannedKey(prefix, refID)
	if h.repo != nil && key != "" {
		_ = h.repo.AssignUnplannedDate(key, date, "", "")
	}
	if t, err := h.ensureReviewTask(prefix, refID); err == nil && t != nil {
		_ = h.wbRepo.SetTaskDueDate(t.TaskID, date)
		_ = h.wbRepo.SetTaskBlocked(t.TaskID, "")
		if prefix == model.WorkPrefixAS {
			h.syncASPlannedFromUnplanned(refID)
		}
	}
	return c.Redirect(http.StatusSeeOther, workReviewBack(c))
}

// ReviewTransfer 지연 건을 다른 담당자에게 넘긴다. 38-C 재사용. §42.7
func (h *WorkHandler) ReviewTransfer(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return c.Redirect(http.StatusSeeOther, workReviewBack(c))
	}
	if h.notices == nil {
		return c.Redirect(http.StatusSeeOther, workReviewBack(c))
	}
	to := h.notices.resolveUser(c.FormValue("assignee_user_id"), c.FormValue("assignee"))
	if to == nil {
		return c.Redirect(http.StatusSeeOther, qjoin(workReviewBack(c), "err=assignee"))
	}
	prefix, refID := reviewFormItem(c)
	t, _ := h.ensureReviewTask(prefix, refID)
	taskID := ""
	if t != nil {
		taskID = t.TaskID
	}
	st, sid := reviewNoticeSource(prefix, refID, taskID)
	n := &model.AssignNotice{SourceType: st, SourceID: sid, UserID: currentUserID(c)}
	_ = h.applyNoticeTransfer(c, n, to)
	if t != nil {
		_ = h.wbRepo.SetTaskBlocked(t.TaskID, "")
	}
	return c.Redirect(http.StatusSeeOther, workReviewBack(c))
}

// ReviewBlock 막힌 이유를 적어 기다리는 것으로 가른다. §42.7
func (h *WorkHandler) ReviewBlock(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return c.Redirect(http.StatusSeeOther, workReviewBack(c))
	}
	reason := model.NormalizeBlockedReason(c.FormValue("blocked_reason"))
	if reason == "" {
		reason = model.NormalizeBlockedReason(c.FormValue("reason"))
	}
	if reason == "" {
		return c.Redirect(http.StatusSeeOther, qjoin(workReviewBack(c), "err=reason"))
	}
	prefix, refID := reviewFormItem(c)
	t, err := h.ensureReviewTask(prefix, refID)
	if err != nil || t == nil {
		return c.Redirect(http.StatusSeeOther, qjoin(workReviewBack(c), "err=task"))
	}
	if err := h.wbRepo.SetTaskBlocked(t.TaskID, reason); err != nil {
		return c.Redirect(http.StatusSeeOther, qjoin(workReviewBack(c), "err=save"))
	}
	return c.Redirect(http.StatusSeeOther, "/work")
}

const weekReviewCookie = "week_review_dismissed"

func weekReviewCookieValue(c echo.Context) string {
	ck, err := c.Cookie(weekReviewCookie)
	if err != nil || ck == nil {
		return ""
	}
	return strings.TrimSpace(ck.Value)
}

func weekReviewShouldShow(now time.Time, cookie string) bool {
	if !model.IsMonday(now) {
		return false
	}
	return strings.TrimSpace(cookie) != model.CalendarMonday(now).Format("2006-01-02")
}

func setWeekReviewDismissedCookie(c echo.Context, now time.Time) {
	mon := model.CalendarMonday(now)
	next := mon.AddDate(0, 0, 7)
	c.SetCookie(&http.Cookie{
		Name:     weekReviewCookie,
		Value:    mon.Format("2006-01-02"),
		Path:     "/",
		Expires:  next,
		MaxAge:   int(next.Sub(now).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// DismissWeekReview 이번 주 월요일 한 줄을 닫는다. DB는 건드리지 않는다. §42.7.3
func (h *Handler) DismissWeekReview(c echo.Context) error {
	setWeekReviewDismissedCookie(c, time.Now())
	back := strings.TrimSpace(c.FormValue("back"))
	if back == "" {
		back = "/"
	}
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		back = "/"
	}
	return c.Redirect(http.StatusSeeOther, back)
}

func weekReviewFromParts(rows []repository.WeekCompleteRow, carried int) model.WeekReview {
	var out model.WeekReview
	out.Carried = carried
	var leadSum, leadN int
	for _, row := range rows {
		out.Completed++
		if row.Due == "" || row.Complete <= row.Due {
			out.OnTime++
		}
		if n, ok := service.LeadBusinessDays(row.Receipt, row.Complete); ok {
			leadSum += n
			leadN++
		}
	}
	if leadN > 0 {
		out.AvgOK = true
		out.AvgLead = math.Round(float64(leadSum)/float64(leadN)*10) / 10
	}
	return out
}

func loadWeekReview(board *repository.WorkBoardRepo, uid string, keys []string, now time.Time) model.WeekReview {
	if board == nil {
		return model.WeekReview{}
	}
	rows, carried, err := board.WeekReviewParts(uid, keys, now)
	if err != nil {
		return model.WeekReview{}
	}
	return weekReviewFromParts(rows, carried)
}

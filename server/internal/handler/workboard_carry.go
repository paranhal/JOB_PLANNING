package handler

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func (h *WorkboardHandler) workdayFinder() service.WorkdayFinder {
	f := service.WorkdayFinder{Cal: service.DefaultCalendar(), Leaves: service.DefaultLeaves()}
	if h != nil && h.mntRepo != nil {
		if lv := h.mntRepo.Leaves(); lv != nil {
			f.Leaves = lv
		}
		if hr := h.mntRepo.Holidays(); hr != nil {
			f.YearMissing = hr.YearMissing
		}
	}
	return f
}

func nextWorkdayFor(h *WorkboardHandler, from time.Time, who []string, orgID string, includeFrom bool) (string, []service.SkipInfo) {
	return h.workdayFinder().NextWorkday(from, who, orgID, includeFrom)
}

func taskWhoNames(t *model.WorkTask, mems []model.WorkTaskMember) []string {
	var who []string
	if t != nil {
		who = append(who, t.Assignee, t.AssigneeUserID)
	}
	for _, m := range mems {
		who = append(who, m.Assignee, m.UserID)
	}
	return who
}

func carryDefaultDates() (today, tomorrow, minDate, nextOpen string, holiday bool) {
	now := time.Now()
	today = now.Format(dateLayout)
	next, skipped := service.NextWorkday(now, nil, "", false)
	if next == "" {
		next = now.AddDate(0, 0, 1).Format(dateLayout)
	}
	tomorrow = next
	minDate = today
	nextOpen = next
	holiday = len(skipped) > 0
	return
}

func mergeCarryModal(data map[string]interface{}) {
	mergeCarryModalWho(data, nil, "")
}

func mergeCarryModalWho(data map[string]interface{}, who []string, orgID string) {
	now := time.Now()
	today := now.Format(dateLayout)
	next, skipped := service.NextWorkday(now, who, orgID, false)
	if next == "" {
		next = now.AddDate(0, 0, 1).Format(dateLayout)
	}
	data["CarryDefaultDate"] = next
	data["CarryMinDate"] = today
	data["CarryNextWorkday"] = next
	data["CarryHoliday"] = len(skipped) > 0
	data["CarrySkipNotice"] = service.FormatSkipNotice(next, skipped)
	_, _, _, some := service.WorkdayFinder{Cal: service.DefaultCalendar(), Leaves: service.DefaultLeaves()}.InspectWorkday(next, who, orgID)
	data["CarryPartialLeave"] = service.FormatPartialLeaveNotice(some)
}

func (h *WorkboardHandler) CarryTask(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		id = strings.TrimSpace(c.FormValue("task_id"))
	}
	if i := strings.Index(id, ":"); i >= 0 {
		id = id[i+1:]
	}
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanPlanEdit(c, t); err != nil {
		return err
	}
	today := time.Now().Format(dateLayout)
	minDate := today
	reason := strings.TrimSpace(c.FormValue("delay_reason"))
	ds := model.TaskDelayState(*t, today)
	newWork, newDue, errMsg := parseCarryDates(t, c, minDate)
	if errMsg != "" {
		return carryFail(c, errMsg, "date")
	}
	if ds.Delayed && reason == "" {
		return carryFail(c, "지연 사유를 입력하세요", "delay_reason")
	}
	if t.StartTime != "" {
		moveDay := newWork
		if moveDay == "" {
			moveDay = newDue
		}
		if err := h.rejectIfOverlap(c, moveDay, t.StartTime, t.EndTime, t.TaskID, occupancyNames(t.Assignee, nil)); err != nil {
			return overlapDone(err)
		}
	}
	if err := applyDateMoveToSegments(c, h.repo, t, t.WorkDate, newWork); err != nil {
		if err == errSegDate {
			return carryFail(c, "사람별 구간이 있어 날짜 이동 확인이 필요합니다", "seg_date")
		}
		return err
	}
	if err := h.repo.CarryTask(t, newWork, newDue, reason, currentUserID(c), currentUserDisplayName(c), ds.Days, repository.DelayKindCarry); err != nil {
		return carryFail(c, err.Error(), "carry")
	}
	if wantsJSONError(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "task_id": t.TaskID})
	}
	back := strings.TrimSpace(c.FormValue("redirect"))
	if back == "" {
		back = strings.TrimSpace(c.FormValue("back"))
	}
	if back == "" {
		back = "/workboard/board"
	}
	return c.Redirect(http.StatusSeeOther, back)
}

func (h *WorkboardHandler) SaveDelayReason(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanPlanEdit(c, t); err != nil {
		return err
	}
	reason := strings.TrimSpace(c.FormValue("delay_reason"))
	if reason == "" {
		return carryFail(c, "지연 사유를 입력하세요", "delay_reason")
	}
	today, _, _, _, _ := carryDefaultDates()
	ds := model.TaskDelayState(*t, today)
	if err := h.repo.CarryTask(t, t.WorkDate, t.DueDate, reason, currentUserID(c), currentUserDisplayName(c), ds.Days, repository.DelayKindReason); err != nil {
		return carryFail(c, err.Error(), "carry")
	}
	if wantsJSONError(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true})
	}
	back := strings.TrimSpace(c.FormValue("redirect"))
	if back == "" {
		back = "/workboard/tasks/" + t.TaskID
	}
	return c.Redirect(http.StatusSeeOther, back)
}

func parseCarryDates(t *model.WorkTask, c echo.Context, minDate string) (newWork, newDue, errMsg string) {
	single := strings.TrimSpace(c.FormValue("new_date"))
	nw := strings.TrimSpace(c.FormValue("new_work_date"))
	nd := strings.TrimSpace(c.FormValue("new_due_date"))
	span := model.TaskIsSpan(t.WorkDate, t.DueDate)
	if !span {
		if single == "" {
			single = nw
		}
		if single == "" {
			return "", "", "새 날짜를 선택하세요"
		}
		if err := model.RequireAppDateYear(single); err != nil {
			return "", "", err.Error()
		}
		if single < minDate {
			return "", "", "오늘 이후 날짜만 선택할 수 있습니다"
		}
		return single, single, ""
	}
	if nd == "" {
		nd = single
	}
	if nd == "" {
		return "", "", "새 종료일을 선택하세요"
	}
	if err := model.RequireAppDateYear(nd); err != nil {
		return "", "", err.Error()
	}
	if nd < minDate {
		return "", "", "오늘 이후 날짜만 선택할 수 있습니다"
	}
	work := strings.TrimSpace(t.WorkDate)
	if nw != "" {
		work = nw
	}
	if work != "" && nd < work {
		return "", "", "종료일이 시작일보다 앞입니다"
	}
	return work, nd, ""
}

func carryFail(c echo.Context, msg, field string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "이월하지 못했습니다"
	}
	if wantsJSONError(c) {
		if err := c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": field, "message": msg, "field": field,
		}); err != nil {
			return err
		}
		return errOverlapResponse
	}
	setFlashErrCookie(c, msg)
	back := strings.TrimSpace(c.FormValue("redirect"))
	if back == "" {
		back = strings.TrimSpace(c.FormValue("back"))
	}
	if back == "" {
		back = redirectBack(c, "err="+url.QueryEscape(msg))
		return c.Redirect(http.StatusSeeOther, back)
	}
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, back+sep+"err="+url.QueryEscape(msg))
}

func (h *WorkboardHandler) requireDelayReasonOnDateMove(c echo.Context, old *model.WorkTask, newWork, newDue string) error {
	if old == nil {
		return nil
	}
	today := time.Now().Format(dateLayout)
	if !model.TaskDelayState(*old, today).Delayed {
		return nil
	}
	if strings.TrimSpace(old.WorkDate) == strings.TrimSpace(newWork) && strings.TrimSpace(old.DueDate) == strings.TrimSpace(newDue) {
		return nil
	}
	if strings.TrimSpace(c.FormValue("delay_reason")) == "" {
		return carryFail(c, "지연 중인 업무는 날짜를 바꿀 때 사유가 필요합니다", "delay_reason")
	}
	return nil
}

func (h *WorkboardHandler) recordDelayIfDateMoved(c echo.Context, old *model.WorkTask, newWork, newDue string) {
	if old == nil || h.repo == nil {
		return
	}
	today := time.Now().Format(dateLayout)
	if !model.TaskDelayState(*old, today).Delayed {
		return
	}
	if strings.TrimSpace(old.WorkDate) == strings.TrimSpace(newWork) && strings.TrimSpace(old.DueDate) == strings.TrimSpace(newDue) {
		return
	}
	reason := strings.TrimSpace(c.FormValue("delay_reason"))
	ds := model.TaskDelayState(*old, today)
	_ = h.repo.RecordDateMoveDelay(old, newWork, newDue, reason, currentUserID(c), currentUserDisplayName(c), ds.Days)
}

func stampBoardDelayReasons(h *WorkboardHandler, items []boardItem) {
	if h == nil || h.repo == nil {
		return
	}
	var ids []string
	for _, it := range items {
		if id := strings.TrimSpace(it.Task.TaskID); id != "" {
			ids = append(ids, id)
		}
	}
	m, err := h.repo.LatestDelayReasons(ids)
	if err != nil || len(m) == 0 {
		return
	}
	for i := range items {
		id := strings.TrimSpace(items[i].Task.TaskID)
		if s := m[id]; s != "" {
			items[i].DelayReasonFull = s
			items[i].DelayReason = model.TruncRunes(s, 40)
		}
	}
}

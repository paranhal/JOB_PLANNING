package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func mountLiveWorkRoutes(g *echo.Group, h *WorkboardHandler) {
	if g == nil || h == nil {
		return
	}
	g.POST("/workboard/tasks/:id/live/start", h.LiveStart)
	g.POST("/workboard/tasks/:id/live/interrupt", h.LiveInterrupt)
	g.POST("/workboard/tasks/:id/live/end-day", h.LiveEndDay)
	g.POST("/workboard/tasks/:id/live/postpone", h.LivePostpone)
	g.POST("/workboard/departures/:id/resume", h.LiveResume)
	g.POST("/workboard/departures/:id/later", h.LiveLater)
}

func liveWantsJSON(c echo.Context) bool {
	if c.FormValue("live_json") == "1" || c.QueryParam("live_json") == "1" {
		return true
	}
	acc := c.Request().Header.Get("Accept")
	return strings.Contains(acc, "application/json")
}

func liveReply(c echo.Context, code int, payload map[string]any, loc string) error {
	if liveWantsJSON(c) {
		if payload == nil {
			payload = map[string]any{}
		}
		payload["ok"] = code < 400
		return c.JSON(code, payload)
	}
	if code >= 400 {
		msg, _ := payload["error"].(string)
		if msg == "" {
			msg = http.StatusText(code)
		}
		return echo.NewHTTPError(code, msg)
	}
	if loc == "" {
		loc = redirectBack(c, "ok=live")
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

func (h *WorkboardHandler) LiveStart(c echo.Context) error {
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	_ = h.repo.LoadTaskActionFields(t)
	if err := h.liveStartTask(c, t); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			msg, _ := he.Message.(string)
			return liveReply(c, he.Code, map[string]any{"error": msg}, "")
		}
		return liveReply(c, 400, map[string]any{"error": err.Error()}, "")
	}
	return liveReply(c, 200, map[string]any{"task_id": t.TaskID, "started": true}, "/workboard/tasks/"+t.TaskID+"?ok=live")
}

func (h *WorkboardHandler) LiveInterrupt(c echo.Context) error {
	host, err := h.repo.GetTask(c.Param("id"))
	if err != nil || host == nil {
		return echo.ErrNotFound
	}
	if err := denyOtherOrgTask(c, host); err != nil {
		return err
	}
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return echo.ErrForbidden
	}
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	_ = h.repo.LoadTaskActionFields(host)
	existing := strings.TrimSpace(c.FormValue("existing_task_id"))
	workType := strings.TrimSpace(firstNonEmpty(c.FormValue("work_type"), model.WBWorkAdmin))
	field := c.FormValue("field_work") == "1" || c.FormValue("field_work") == "field"
	newPlace := model.NormalizeWorkPlace(c.FormValue("work_place"))
	tmp := &model.WorkTask{WorkType: workType, WorkPlace: newPlace}
	ad := adapterFor(workType)
	needTravel := travelRequiredForInterrupt(ad, tmp, field)
	tb := atoiSafe(c.FormValue("travel_before_min"))
	unk := strings.TrimSpace(c.FormValue("travel_unknown_reason"))
	if needTravel && tb <= 0 && unk == "" {
		return liveReply(c, 422, map[string]any{"error": "외근 끼워넣기는 이동시간이 필요합니다"}, "")
	}
	day, hhmm, now := parseLiveClock(c, firstNonEmpty(c.FormValue("work_date"), host.WorkDate))
	dur := atoiSafe(c.FormValue("duration_min"))
	if dur <= 0 {
		dur = 60
	}
	ta := atoiSafe(c.FormValue("travel_after_min"))
	var nextFixed *time.Time
	nextTitle := strings.TrimSpace(c.FormValue("next_fixed_title"))
	if raw := strings.TrimSpace(c.FormValue("next_fixed_at")); raw != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04", raw, time.Local); err == nil {
			nextFixed = &t
		} else if t, err := time.ParseInLocation("15:04", raw, time.Local); err == nil {
			x := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
			nextFixed = &x
		}
	}
	fit := FitCheck(currentUserID(c), now, dur, tb, ta, nextFixed, nextTitle)
	force := c.FormValue("force") == "1"
	forceReason := strings.TrimSpace(c.FormValue("force_reason"))
	if !fit.OK {
		if !force {
			return liveReply(c, 409, map[string]any{
				"error": "이동 포함 일정이 부족합니다", "fit": fit, "choices": fit.Choices,
			}, "")
		}
		if forceReason == "" {
			return liveReply(c, 422, map[string]any{"error": "강제 배치 사유가 필요합니다", "fit": fit}, "")
		}
	}
	start := firstNonEmpty(c.FormValue("start_time"), hhmm)
	endMin := model.ParseHHMMMinutes(start) + dur
	end := model.FormatHHMMMinutes(endMin)
	if existing == "" {
		title := strings.TrimSpace(c.FormValue("title"))
		if title == "" {
			title = "끼어든 업무"
		}
		org, oerr := resolveInsertOrg(c)
		if oerr != nil {
			org = host.OrgID
		}
		nt := &model.WorkTask{
			WorkType: workType, Title: title, WorkDate: day, DueDate: day,
			StartTime: start, EndTime: end, DurationMin: dur,
			Status: model.WBTaskInProgress, Priority: model.WBPriorityNormal,
			Assignee: firstNonEmpty(c.FormValue("assignee"), currentUserDisplayName(c)),
			AssigneeUserID: firstNonEmpty(c.FormValue("assignee_user_id"), currentUserID(c)),
			OrgID: org, WorkPlace: newPlace,
		}
		if err := h.repo.CreateTask(nt); err != nil {
			return err
		}
		existing = nt.TaskID
		if remaining := atoiSafe(c.FormValue("est_minutes")); remaining > 0 {
			_ = h.repo.SetTaskLiveFields(existing, remaining, "", "")
		}
	}
	c.Set("pending_interrupt", &pendingInterrupt{
		Hits:   []overlapHit{{Task: *host, Date: day}},
		Travel: tb, Date: day, Start: start, End: end,
		Names: occupancyNames(host.Assignee, nil),
	})
	if forceReason != "" {
		c.Set("live_force_reason", forceReason)
	}
	if err := h.applyLiveInterrupt(c, existing); err != nil {
		_ = h.repo.DeleteTaskLogged(existing, "끼워넣기 실패 롤백")
		if he, ok := err.(*echo.HTTPError); ok {
			msg, _ := he.Message.(string)
			return liveReply(c, he.Code, map[string]any{"error": msg}, "")
		}
		return liveReply(c, 400, map[string]any{"error": err.Error()}, "")
	}
	if forceReason != "" {
		if deps, _ := h.repo.ListDepartures(host.TaskID); len(deps) > 0 {
			d := deps[len(deps)-1]
			d.ForceReason = forceReason
			_ = h.repo.UpdateDepartureLive(d)
		}
	}
	hint, _ := c.Get("live_interrupt_hint").(string)
	return liveReply(c, 200, map[string]any{
		"interrupt_task_id": existing, "hint": hint, "fit": fit, "force": force,
	}, "/workboard/tasks/"+existing+"?ok=live")
}

func (h *WorkboardHandler) LiveEndDay(c echo.Context) error {
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := h.liveEndDay(c, t); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			msg, _ := he.Message.(string)
			return liveReply(c, he.Code, map[string]any{"error": msg}, "")
		}
		return liveReply(c, 400, map[string]any{"error": err.Error()}, "")
	}
	deps, _ := h.repo.ListPendingReturns()
	var depID string
	for _, d := range deps {
		if d.InterruptTaskID == t.TaskID {
			depID = d.DepartureID
			break
		}
	}
	return liveReply(c, 200, map[string]any{"ended": true, "departure_id": depID, "end_href": adapterFor(t.WorkType).EndHref(t)},
		"/workboard/tasks/"+t.TaskID+"?ok=live")
}

func (h *WorkboardHandler) LivePostpone(c echo.Context) error {
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	ad := adapterFor(t.WorkType)
	if !ad.Enabled() {
		return liveReply(c, 403, map[string]any{"error": "live_disabled_for_type"}, "")
	}
	if err := h.liveEndDay(c, t); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			msg, _ := he.Message.(string)
			return liveReply(c, he.Code, map[string]any{"error": msg}, "")
		}
	}
	reason := strings.TrimSpace(c.FormValue("reason"))
	next := strings.TrimSpace(c.FormValue("next_action_date"))
	if err := h.ExtendToNextDay(t, reason, next, atoiSafe(c.FormValue("remaining_min")), currentUserID(c), currentUserDisplayName(c)); err != nil {
		return liveReply(c, 422, map[string]any{"error": err.Error()}, "")
	}
	got, _ := h.repo.GetTask(t.TaskID)
	badge := ""
	if got != nil {
		actuals, _ := h.repo.ListActualSegments(got.TaskID)
		var dates []string
		dates = append(dates, got.OrigWorkDate, got.WorkDate, got.NextActionDate)
		for _, s := range actuals {
			dates = append(dates, s.WorkDate)
		}
		badge = model.PlanVsActualDaysBadge(1, model.UniqueWorkDates(dates))
	}
	return liveReply(c, 200, map[string]any{"postponed": true, "orig_work_date": t.OrigWorkDate, "badge": badge, "task_id": t.TaskID},
		"/workboard/tasks/"+t.TaskID+"?ok=live")
}

func (h *WorkboardHandler) LiveResume(c echo.Context) error {
	d, err := h.repo.GetDeparture(c.Param("id"))
	if err != nil || d == nil {
		return echo.ErrNotFound
	}
	if err := h.liveResumeNow(c, d); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			msg, _ := he.Message.(string)
			return liveReply(c, he.Code, map[string]any{"error": msg}, "")
		}
		return liveReply(c, 400, map[string]any{"error": err.Error()}, "")
	}
	return liveReply(c, 200, map[string]any{"resumed": true, "task_id": d.TaskID}, "/workboard/tasks/"+d.TaskID+"?ok=live")
}

func (h *WorkboardHandler) LiveLater(c echo.Context) error {
	d, err := h.repo.GetDeparture(c.Param("id"))
	if err != nil || d == nil {
		return echo.ErrNotFound
	}
	if err := h.liveResumeLater(c, d); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			msg, _ := he.Message.(string)
			return liveReply(c, he.Code, map[string]any{"error": msg}, "")
		}
		return liveReply(c, 400, map[string]any{"error": err.Error()}, "")
	}
	return liveReply(c, 200, map[string]any{"later": true, "task_id": d.TaskID}, "/workboard/tasks/"+d.TaskID+"?ok=live")
}

func attachLiveWorkData(data map[string]interface{}, h *WorkboardHandler, c echo.Context, t *model.WorkTask) {
	if data == nil {
		return
	}
	seed := defaultAssigneeSeed(c)
	data["AssigneeSeed"] = seed
	data["LiveFitChoices"] = model.FitCheckChoices()
	if data["Date"] == nil || data["Date"] == "" {
		data["Date"] = time.Now().Format(dateLayout)
	}
	if data["BoardHref"] == nil || data["BoardHref"] == "" {
		data["BoardHref"] = "/workboard/board"
	}
	if data["CurrentUserName"] == nil {
		data["CurrentUserName"] = currentUserDisplayName(c)
	}
	if data["UserID"] == nil {
		data["UserID"] = currentUserID(c)
	}
	_, _ = h.repo.CloseStaleOpenSegments(time.Now().Format(dateLayout))
	urgent := PendingUrgentForWorker(h, currentUserID(c), currentUserDisplayName(c))
	data["UrgentBanner"] = urgent
	data["HasUrgentBanner"] = len(urgent) > 0
	if t == nil {
		data["LiveEnabled"] = false
		data["ShowLiveStart"] = false
		return
	}
	_ = h.repo.LoadTaskLiveFields(t)
	_ = h.repo.LoadTaskActionFields(t)
	ad := adapterFor(t.WorkType)
	data["LiveEnabled"] = ad.Enabled() && canWriteWorkboard(c) && !isReadOnly(c) && currentRole(c) != model.RoleCLevel
	data["LiveEndHref"] = ad.EndHref(t)
	data["LiveFieldWork"] = ad.IsFieldWork(t)
	data["LiveStartHint"] = liveTypeStartHint(t.WorkType)
	opens, _ := h.repo.ListOpenActualForTask(t.TaskID, currentUserID(c))
	if len(opens) == 0 {
		opens, _ = h.repo.ListOpenActualForTask(t.TaskID, "")
	}
	data["LiveOpen"] = opens
	data["LiveWorking"] = len(opens) > 0
	data["ShowLiveStart"] = ad.Enabled() && data["LiveEnabled"] == true && len(opens) == 0 && t.Status != model.WBTaskComplete
	sum, _ := TaskTimeSummary(h.repo, t.TaskID)
	data["TimeSummary"] = sum
	data["OccupyLine"] = model.OccupancyLine(sum.ActualMin, sum.TravelMin)
	if t.RecurrenceRole == model.RecurrenceRoleParent || t.ParentTaskID == "" {
		if n, _ := h.repo.CountOccurrences(t.TaskID); n > 0 {
			pp, _ := ParentProgress(h.repo, t.TaskID)
			data["ParentProgress"] = pp
			data["ProgressCount"] = progressCountLine(pp)
			data["ProgressTime"] = progressTimeLine(pp)
			data["ProgressRemain"] = progressRemainLine(pp)
			data["ProgressTimePct"] = progressPctDash(pp.ByTimePct)
			data["ProgressCountPct"] = progressPctDash(pp.ByCountPct)
		}
	}
	actuals, _ := h.repo.ListActualSegments(t.TaskID)
	data["LiveTimeline"] = actuals
	deps, _ := h.repo.ListDepartures(t.TaskID)
	data["LiveDepartures"] = deps
	var pending *model.WorkTaskDeparture
	for i := range deps {
		if deps[i].ReturnStatus == model.ReturnPending {
			pending = &deps[i]
		}
	}
	data["LivePendingReturn"] = pending
	if t.OrigWorkDate != "" {
		var dates []string
		dates = append(dates, t.OrigWorkDate, t.WorkDate, t.NextActionDate)
		for _, s := range actuals {
			dates = append(dates, s.WorkDate)
		}
		data["PlanActualBadge"] = model.PlanVsActualDaysBadge(1, model.UniqueWorkDates(dates))
	}
}

func progressCountLine(p model.ParentProgress) string {
	if p.Total == 0 {
		return "—"
	}
	return strconv.Itoa(p.Done) + " / " + strconv.Itoa(p.Total) + "회"
}

func progressTimeLine(p model.ParentProgress) string {
	if p.EstMin == 0 {
		return "—"
	}
	return hourLine(p.ActualMin) + " / " + hourLine(p.EstMin)
}

func progressRemainLine(p model.ParentProgress) string {
	if p.EstMin == 0 && p.RemainingMin == 0 {
		return "—"
	}
	return hourLine(p.RemainingMin)
}

func hourLine(min int) string {
	if min%60 == 0 {
		return strconv.Itoa(min/60) + "h"
	}
	return strconv.Itoa(min/60) + "h " + strconv.Itoa(min%60) + "분"
}

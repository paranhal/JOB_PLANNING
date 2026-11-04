package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func parseLiveClock(c echo.Context, day string) (date, hhmm string, at time.Time) {
	day = strings.TrimSpace(day)
	raw := strings.TrimSpace(firstNonEmpty(c.FormValue("live_at"), c.FormValue("clock")))
	now := time.Now()
	if raw != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04", raw, time.Local); err == nil {
			now = t
		} else if t, err := time.ParseInLocation("15:04", raw, time.Local); err == nil {
			base := now
			if day != "" {
				if d, err := time.ParseInLocation(dateLayout, day, time.Local); err == nil {
					base = d
				}
			}
			now = time.Date(base.Year(), base.Month(), base.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
		}
	}
	if day == "" {
		day = now.Format(dateLayout)
	}
	return day, now.Format("15:04"), now
}

func (h *WorkboardHandler) applyLiveInterrupt(c echo.Context, newID string) error {
	v, _ := c.Get("pending_interrupt").(*pendingInterrupt)
	if v == nil || strings.TrimSpace(newID) == "" {
		return h.finishLiveInterruptLink(c, newID)
	}
	day, hhmm, _ := parseLiveClock(c, v.Date)
	for _, hit := range v.Hits {
		t, err := h.repo.GetTask(hit.Task.TaskID)
		if err != nil || t == nil {
			continue
		}
		splitDay := firstNonEmpty(hit.Date, v.Date, day)
		_ = h.repo.ApplyInterruptSplit(t, newID, splitDay, v.Start, v.End, v.Travel, v.Names)
	}
	uid := currentUserID(c)
	hostIDs := map[string]bool{}
	for _, hit := range v.Hits {
		hostIDs[hit.Task.TaskID] = true
	}
	opensW, _ := h.repo.ListOpenActualForWorker(uid)
	for _, s := range opensW {
		_ = h.repo.CloseSegment(s, hhmm, model.EndedByInterrupt)
		if hostIDs[s.TaskID] {
			_ = h.repo.SetSegmentInterrupt(s.SegmentID, newID)
		}
	}
	for _, hit := range v.Hits {
		id := hit.Task.TaskID
		if uid == "" {
			opensAll, _ := h.repo.ListOpenActualForTask(id, "")
			for _, s := range opensAll {
				_ = h.repo.CloseSegment(s, hhmm, model.EndedByInterrupt)
				_ = h.repo.SetSegmentInterrupt(s.SegmentID, newID)
			}
		}
		h.ensureLiveDeparture(id, newID, uid, day, firstNonEmpty(v.Start, hhmm), firstNonEmpty(v.End, hhmm), c)
	}
	nt, _ := h.repo.GetTask(newID)
	if nt == nil {
		return nil
	}
	_ = h.repo.LoadTaskActionFields(nt)
	ad := adapterFor(nt.WorkType)
	if !ad.Enabled() {
		c.Set("live_interrupt_hint", liveTypeStartHint(nt.WorkType))
		return nil
	}
	travelBefore, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("travel_before_min")))
	_, err := h.repo.InsertLiveActual(model.WorkTaskSegment{
		TaskID:              newID,
		Kind:                model.SegKindActual,
		Scope:               model.SegScopeCommon,
		WorkDate:            day,
		StartTime:           firstNonEmpty(v.Start, hhmm),
		EndTime:             "",
		Source:              model.SegSourceLive,
		UserID:              uid,
		Assignee:            firstNonEmpty(nt.Assignee, currentUserDisplayName(c)),
		WorkerUserID:        uid,
		WorkerName:          currentUserDisplayName(c),
		TravelBeforeMin:     model.ClampTravelMin(travelBefore),
		TravelAfterMin:      model.ClampTravelMin(atoiSafe(c.FormValue("travel_after_min"))),
		TravelOrigin:        strings.TrimSpace(c.FormValue("travel_origin")),
		TravelDest:          strings.TrimSpace(c.FormValue("travel_dest")),
		TravelUnknownReason: strings.TrimSpace(c.FormValue("travel_unknown_reason")),
	})
	return err
}

func (h *WorkboardHandler) finishLiveInterruptLink(c echo.Context, newID string) error {
	hostID := strings.TrimSpace(firstNonEmpty(c.FormValue("live_host_id"), c.FormValue("interrupt_host_id")))
	if hostID == "" || newID == "" {
		return nil
	}
	uid := currentUserID(c)
	day, hhmm, _ := parseLiveClock(c, c.FormValue("work_date"))
	opens, _ := h.repo.ListOpenActualForTask(hostID, uid)
	if len(opens) == 0 {
		opens, _ = h.repo.ListOpenActualForTask(hostID, "")
	}
	for _, s := range opens {
		_ = h.repo.CloseSegment(s, hhmm, model.EndedByInterrupt)
		_ = h.repo.SetSegmentInterrupt(s.SegmentID, newID)
	}
	host, _ := h.repo.GetTask(hostID)
	if host != nil {
		_, hhmm2 := hhmm, hhmm
		_ = hhmm2
		end := firstNonEmpty(c.FormValue("end_time"), hhmm)
		start := firstNonEmpty(c.FormValue("start_time"), hhmm)
		_, _ = h.repo.InsertDeparture(model.WorkTaskDeparture{
			TaskID: hostID, UserID: uid, Assignee: firstNonEmpty(host.Assignee, currentUserDisplayName(c)),
			WorkDate: day, StartTime: start, EndTime: end, InterruptTaskID: newID,
			ReturnStatus: model.ReturnPending, ForceReason: strings.TrimSpace(c.FormValue("force_reason")),
		})
		_ = h.repo.ApplyInterruptSplit(host, newID, day, start, end, atoiSafe(c.FormValue("split_travel_min")), occupancyNames(host.Assignee, nil))
	}
	nt, _ := h.repo.GetTask(newID)
	if nt != nil && adapterFor(nt.WorkType).Enabled() {
		_, _ = h.repo.InsertLiveActual(model.WorkTaskSegment{
			TaskID: newID, Kind: model.SegKindActual, Scope: model.SegScopeCommon, WorkDate: day,
			StartTime: firstNonEmpty(c.FormValue("start_time"), hhmm), Source: model.SegSourceLive,
			UserID: uid, WorkerUserID: uid, WorkerName: currentUserDisplayName(c), Assignee: nt.Assignee,
			TravelBeforeMin: model.ClampTravelMin(atoiSafe(c.FormValue("travel_before_min"))),
			TravelUnknownReason: strings.TrimSpace(c.FormValue("travel_unknown_reason")),
		})
	} else if nt != nil {
		c.Set("live_interrupt_hint", liveTypeStartHint(nt.WorkType))
	}
	return nil
}

func (h *WorkboardHandler) ensureLiveDeparture(hostID, newID, uid, day, start, end string, c echo.Context) {
	deps, _ := h.repo.ListDepartures(hostID)
	for _, d := range deps {
		if d.InterruptTaskID == newID && (d.ReturnStatus == model.ReturnPending || d.ReturnStatus == "") {
			return
		}
	}
	host, _ := h.repo.GetTask(hostID)
	asg := currentUserDisplayName(c)
	if host != nil {
		asg = firstNonEmpty(host.Assignee, asg)
		if uid == "" {
			uid = host.AssigneeUserID
		}
	}
	force, _ := c.Get("live_force_reason").(string)
	_, _ = h.repo.InsertDeparture(model.WorkTaskDeparture{
		TaskID: hostID, UserID: uid, Assignee: asg, WorkDate: day, StartTime: start, EndTime: end,
		InterruptTaskID: newID, ReturnStatus: model.ReturnPending, ForceReason: force,
	})
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func (h *WorkboardHandler) liveStartTask(c echo.Context, t *model.WorkTask) error {
	ad := adapterFor(t.WorkType)
	if !ad.Enabled() {
		return echo.NewHTTPError(403, "live_disabled_for_type")
	}
	if err := ad.CanStart(c, t); err != nil {
		return err
	}
	_, _ = h.repo.CloseStaleOpenSegments(time.Now().Format(dateLayout))
	uid := currentUserID(c)
	opens, _ := h.repo.ListOpenActualForWorker(uid)
	for _, s := range opens {
		if s.TaskID != t.TaskID {
			return echo.NewHTTPError(409, "이미 작업 중인 업무가 있습니다:/workboard/tasks/"+s.TaskID)
		}
		return nil // 멱등
	}
	mine, _ := h.repo.ListOpenActualForTask(t.TaskID, uid)
	if len(mine) > 0 {
		return nil
	}
	day, hhmm, _ := parseLiveClock(c, firstNonEmpty(t.WorkDate, t.DueDate))
	scope := model.SegScopeCommon
	if t.WorkType == model.WBWorkProjectWork {
		scope = model.SegScopePerson
	}
	_, err := h.repo.InsertLiveActual(model.WorkTaskSegment{
		TaskID: t.TaskID, Kind: model.SegKindActual, Scope: scope, WorkDate: day, StartTime: hhmm,
		Source: model.SegSourceLive, UserID: uid, Assignee: t.Assignee, WorkerUserID: uid, WorkerName: currentUserDisplayName(c),
	})
	if err != nil {
		return err
	}
	if t.RecurrenceRole == model.RecurrenceRoleOccurrence && model.NormalizeOccurrenceStatus(t.OccurrenceStatus) == model.OccurrenceScheduled {
		_ = h.repo.UpdateOccurrenceFields(t.TaskID, model.OccurrenceInProgress, t.NotDoneReason, t.NextCheckDate)
	} else if t.Status == model.WBTaskWaiting {
		t.Status = model.WBTaskInProgress
		_ = h.repo.UpdateTask(t)
	}
	return ad.AfterSegmentsChanged(nil, t)
}

func (h *WorkboardHandler) liveEndDay(c echo.Context, t *model.WorkTask) error {
	ad := adapterFor(t.WorkType)
	if !ad.Enabled() {
		return echo.NewHTTPError(403, "live_disabled_for_type")
	}
	uid := currentUserID(c)
	_, hhmm, _ := parseLiveClock(c, t.WorkDate)
	opens, _ := h.repo.ListOpenActualForTask(t.TaskID, uid)
	if len(opens) == 0 {
		opens, _ = h.repo.ListOpenActualForTask(t.TaskID, "")
	}
	for _, s := range opens {
		if err := h.repo.CloseSegment(s, hhmm, model.EndedByDayEnd); err != nil {
			return err
		}
	}
	return nil
}

func (h *WorkboardHandler) ExtendToNextDay(t *model.WorkTask, reason, nextDate string, remainingMin int, byID, byName string) error {
	if t == nil {
		return fmt.Errorf("업무가 없습니다")
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("미완료 사유를 입력하세요")
	}
	if strings.TrimSpace(nextDate) == "" {
		return fmt.Errorf("다음 조치일을 입력하세요")
	}
	actuals, _ := h.repo.ListActualSegments(t.TaskID)
	keep := map[string]string{}
	for _, s := range actuals {
		keep[s.SegmentID] = s.WorkDate
	}
	due := t.DueDate
	if due == "" {
		due = t.WorkDate
	}
	if err := h.repo.CarryTask(t, nextDate, due, reason, byID, byName, 0, ""); err != nil {
		return err
	}
	_ = h.repo.RestoreActualSegmentDates(t.TaskID, keep)
	_ = h.repo.SetTaskNextActionDate(t.TaskID, nextDate)
	if remainingMin > 0 {
		est := remainingMin
		_ = h.repo.SetTaskLiveFields(t.TaskID, est, nextDate, t.CompletionCondition)
	}
	if t.RecurrenceRole == model.RecurrenceRoleOccurrence {
		_ = h.repo.UpdateOccurrenceFields(t.TaskID, model.OccurrenceDeferred, reason, nextDate)
	}
	got, _ := h.repo.GetTask(t.TaskID)
	if got != nil {
		*t = *got
	}
	return nil
}

func (h *WorkboardHandler) liveResumeNow(c echo.Context, d *model.WorkTaskDeparture) error {
	host, err := h.repo.GetTask(d.TaskID)
	if err != nil || host == nil {
		return echo.ErrNotFound
	}
	ad := adapterFor(host.WorkType)
	if !ad.Enabled() {
		return echo.NewHTTPError(403, "live_disabled_for_type")
	}
	day, hhmm, _ := parseLiveClock(c, firstNonEmpty(host.WorkDate, d.WorkDate))
	d.ReturnStatus = model.ReturnResumedNow
	d.EndTime = hhmm
	if err := h.repo.UpdateDepartureLive(*d); err != nil {
		return err
	}
	uid := currentUserID(c)
	scope := model.SegScopeCommon
	if host.WorkType == model.WBWorkProjectWork {
		scope = model.SegScopePerson
	}
	_, err = h.repo.InsertLiveActual(model.WorkTaskSegment{
		TaskID: host.TaskID, Kind: model.SegKindActual, Scope: scope, WorkDate: day, StartTime: hhmm,
		Source: model.SegSourceLive, UserID: uid, WorkerUserID: uid, WorkerName: currentUserDisplayName(c),
		Assignee: host.Assignee, TravelBeforeMin: model.ClampTravelMin(atoiSafe(c.FormValue("travel_before_min"))),
	})
	return err
}

func (h *WorkboardHandler) liveResumeLater(c echo.Context, d *model.WorkTaskDeparture) error {
	reason := strings.TrimSpace(c.FormValue("reason"))
	if reason == "" {
		return echo.NewHTTPError(422, "중단 사유를 입력하세요")
	}
	next := strings.TrimSpace(c.FormValue("next_action_date"))
	var rem *int
	if n := atoiSafe(c.FormValue("remaining_min")); n > 0 {
		rem = &n
	}
	d.Reason = reason
	d.NextActionDate = next
	d.RemainingMin = rem
	d.ReturnStatus = model.ReturnResumeLater
	if err := h.repo.UpdateDepartureLive(*d); err != nil {
		return err
	}
	host, _ := h.repo.GetTask(d.TaskID)
	if host == nil {
		return nil
	}
	due := host.DueDate
	_ = h.repo.SetTaskNextActionDate(host.TaskID, next)
	if rem != nil {
		_ = h.repo.SetTaskLiveFields(host.TaskID, *rem, next, host.CompletionCondition)
	}
	if host.RecurrenceRole == model.RecurrenceRoleOccurrence {
		st := model.OccurrenceHold
		if next != "" {
			st = model.OccurrenceDeferred
		}
		_ = h.repo.UpdateOccurrenceFields(host.TaskID, st, reason, next)
	} else if next == "" {
		host.Status = model.WBTaskHold
		host.HoldReason = reason
		_ = h.repo.UpdateTask(host)
	}
	_ = due
	return nil
}

func PendingUrgentForWorker(h *WorkboardHandler, workerID, name string) []model.WorkTask {
	if h == nil || h.repo == nil {
		return nil
	}
	items, _ := h.repo.ListPendingUrgentForWorker(workerID, name)
	var out []model.WorkTask
	for i := range items {
		if adapterFor(items[i].WorkType).IsUrgent(&items[i]) {
			out = append(out, items[i])
		}
	}
	return out
}

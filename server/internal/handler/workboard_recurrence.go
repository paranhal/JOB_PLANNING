package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func parseRecurrenceForm(c echo.Context) model.WorkRecurrence {
	_ = c.Request().ParseForm()
	n := 1
	fmtScanInt(c.FormValue("interval_n"), &n)
	if n < 1 {
		n = 1
	}
	monthDay, monthN := 0, 0
	fmtScanInt(c.FormValue("month_day"), &monthDay)
	fmtScanInt(c.FormValue("month_n"), &monthN)
	start := strings.TrimSpace(c.FormValue("start_date"))
	if start == "" {
		start = strings.TrimSpace(c.FormValue("work_date"))
	}
	end := strings.TrimSpace(c.FormValue("end_date"))
	if end == "" {
		end = strings.TrimSpace(c.FormValue("due_date"))
	}
	w := model.WorkRecurrence{
		StartDate:             start,
		EndDate:               end,
		RuleType:              c.FormValue("rule_type"),
		IntervalN:             n,
		Weekdays:              model.JoinWeekdays(c.Request().PostForm["weekdays"]),
		HolidayPolicy:         c.FormValue("holiday_policy"),
		CompletePolicy:        c.FormValue("complete_policy"),
		ProgressIncludeFuture: c.FormValue("progress_include_future") == "1",
		MonthDay:              monthDay,
		MonthN:                monthN,
		LastWorkday:           c.FormValue("monthly_mode") == "last" || c.FormValue("last_workday") == "1",
		ManualDates:           model.ParseDateList(c.FormValue("manual_dates")),
	}
	if model.NormalizeRecurrenceRuleType(w.RuleType) == model.RecurrenceManual {
		w.HolidayPolicy = model.HolidayPolicyAsIs
	}
	return w
}

func (h *WorkboardHandler) holidayYearMissing() func(int) bool {
	var hr interface{ YearMissing(int) bool }
	if h != nil && h.mntRepo != nil {
		if r := h.mntRepo.Holidays(); r != nil {
			hr = r
		}
	}
	return func(y int) bool {
		return hr != nil && hr.YearMissing(y)
	}
}

func defaultRecurrenceForm(t *model.WorkTask, today string) model.WorkRecurrence {
	start := ""
	end := ""
	if t != nil {
		start = strings.TrimSpace(t.WorkDate)
		if start == "" {
			start = strings.TrimSpace(t.DueDate)
		}
		end = strings.TrimSpace(t.DueDate)
	}
	if start == "" {
		start = today
	}
	if end == "" {
		end = start
	}
	return model.WorkRecurrence{
		StartDate:      start,
		EndDate:        end,
		RuleType:       model.RecurrenceNone,
		IntervalN:      1,
		HolidayPolicy:  model.HolidayPolicyAsIs,
		CompletePolicy: model.CompletePolicyManual,
	}
}

func recurrenceYears(from, to string) []int {
	seen := map[int]bool{}
	var years []int
	for _, s := range []string{from, to} {
		t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
		if err != nil {
			continue
		}
		y := t.Year()
		if seen[y] {
			continue
		}
		seen[y] = true
		years = append(years, y)
	}
	return years
}

func (h *WorkboardHandler) PreviewRecurrence(c echo.Context) error {
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanEditTask(c, t); err != nil {
		return err
	}
	if t.ParentTaskID != "" || t.SourceType != "" {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID+"/edit?err=rec_parent")
	}
	rule := parseRecurrenceForm(c)
	rule.TaskID = t.TaskID
	prev := service.ExpandOccurrencePreview(rule, service.DefaultCalendar(), h.holidayYearMissing())
	c.Set("recurrence_form", &rule)
	c.Set("recurrence_preview", &prev)
	c.Set("recurrence_change_mode", model.NormalizeRecurrenceChangeMode(c.FormValue("change_mode")))
	return h.EditTask(c)
}

func (h *WorkboardHandler) GenerateRecurrence(c echo.Context) error {
	return h.runOccurrenceWrite(c, false)
}

func (h *WorkboardHandler) RegenerateRecurrence(c echo.Context) error {
	return h.runOccurrenceWrite(c, true)
}

func (h *WorkboardHandler) runOccurrenceWrite(c echo.Context, regenerate bool) error {
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanEditTask(c, t); err != nil {
		return err
	}
	loc := "/workboard/tasks/" + t.TaskID + "/edit"
	if t.ParentTaskID != "" {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_parent")
	}
	if t.SourceType != "" {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_source")
	}
	rule := parseRecurrenceForm(c)
	rule.TaskID = t.TaskID
	prev := service.ExpandOccurrencePreview(rule, service.DefaultCalendar(), h.holidayYearMissing())
	if err := h.rejectIfOverlapDates(c, prev.Dates, t.StartTime, t.EndTime, t.TaskID, occupancyNames(t.Assignee, nil)); err != nil {
		return overlapDone(err)
	}
	prev.Dates = filterSkippedDates(prev.Dates, overlapSkipDateSet(c))
	if prev.RejectYear {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_year")
	}
	if prev.Reject500 {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_limit")
	}
	if prev.Count == 0 || prev.Error != "" && !prev.Warn200 {
		code := "rec_rule"
		if strings.Contains(prev.Error, "500") {
			code = "rec_limit"
		}
		return c.Redirect(http.StatusSeeOther, loc+"?err="+code)
	}
	if prev.Warn200 && c.FormValue("confirm_over_200") != "1" {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_warn")
	}
	mode := ""
	if regenerate {
		mode = model.NormalizeRecurrenceChangeMode(c.FormValue("change_mode"))
		if mode == model.RecurrenceChangeReplace && c.FormValue("confirm_replace") != "1" {
			return c.Redirect(http.StatusSeeOther, loc+"?err=rec_replace")
		}
	}
	var res model.OccurrenceGenerateResult
	if regenerate {
		today := time.Now().Format("2006-01-02")
		res, err = h.repo.ApplyOccurrenceChange(t, rule, prev.Dates, mode, today)
	} else {
		res, err = h.repo.GenerateOccurrences(t, rule, prev.Dates)
	}
	if err != nil {
		if strings.Contains(err.Error(), "이미 실행 작업") {
			return c.Redirect(http.StatusSeeOther, loc+"?err=rec_exists")
		}
		return err
	}
	ok := "rec_gen"
	if regenerate {
		ok = "rec_regen"
	}
	return c.Redirect(http.StatusSeeOther, loc+"?ok="+ok+
		"&n="+fmt.Sprintf("%d", res.Created)+
		"&report="+url.QueryEscape(res.Report()))
}

func (h *WorkboardHandler) recurrenceCompleteErr(existing, t *model.WorkTask, c echo.Context) string {
	if existing == nil || t == nil {
		return ""
	}
	if t.Status != model.WBTaskComplete || existing.Status == model.WBTaskComplete {
		return ""
	}
	if existing.ParentTaskID != "" {
		return ""
	}
	occN, err := h.repo.CountOccurrences(existing.TaskID)
	if err != nil || occN == 0 {
		return ""
	}
	open, _ := h.repo.CountOpenOccurrences(existing.TaskID)
	if open > 0 && c.FormValue("confirm_incomplete_occ") != "1" {
		return "rec_open"
	}
	rule, _ := h.repo.GetRecurrence(existing.TaskID)
	if rule != nil && model.NormalizeCompletePolicy(rule.CompletePolicy) == model.CompletePolicyRequireResult {
		result := strings.TrimSpace(rule.FinalResult)
		if result == "" {
			result = strings.TrimSpace(c.FormValue("final_result"))
		}
		if result == "" {
			return "rec_result"
		}
	}
	if fr := strings.TrimSpace(c.FormValue("final_result")); fr != "" {
		_ = h.repo.SetRecurrenceFinalResult(existing.TaskID, fr)
	}
	return ""
}

func (h *WorkboardHandler) saveOccurrenceStatus(existing, t *model.WorkTask, c echo.Context) error {
	if existing == nil || existing.RecurrenceRole != model.RecurrenceRoleOccurrence {
		return nil
	}
	occ := strings.TrimSpace(c.FormValue("occurrence_status"))
	if occ == "" && t != nil && t.Status == model.WBTaskComplete {
		occ = model.OccurrenceComplete
	}
	if occ == "" {
		return nil
	}
	err := h.repo.UpdateOccurrenceFields(existing.TaskID, occ,
		c.FormValue("not_done_reason"), c.FormValue("next_check_date"))
	if err != nil {
		return err
	}
	return h.repo.MaybeAutoCompleteParent(existing.ParentTaskID)
}

func (h *WorkboardHandler) UpdateOccurrence(c echo.Context) error {
	parentID := c.Param("id")
	oid := c.Param("oid")
	occ, err := h.repo.GetTask(oid)
	if err != nil || occ == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanEditTask(c, occ); err != nil {
		return err
	}
	loc := "/workboard/tasks/" + parentID
	if occ.ParentTaskID != parentID || occ.RecurrenceRole != model.RecurrenceRoleOccurrence {
		return echo.ErrNotFound
	}
	status := strings.TrimSpace(c.FormValue("occurrence_status"))
	if status == "" {
		status = model.OccurrenceComplete
	}
	if err := h.repo.UpdateOccurrenceFields(oid, status, c.FormValue("not_done_reason"), c.FormValue("next_check_date")); err != nil {
		code := "rec_rule"
		if strings.Contains(err.Error(), "제외 사유") {
			code = "rec_skip_reason"
		} else if strings.Contains(err.Error(), "다음 조치일") {
			code = "rec_defer_date"
		}
		return c.Redirect(http.StatusSeeOther, loc+"?err="+code)
	}
	_ = h.repo.MaybeAutoCompleteParent(parentID)
	return c.Redirect(http.StatusSeeOther, loc+"?ok=saved")
}

func (h *WorkboardHandler) SaveRecurrenceSettings(c echo.Context) error {
	id := c.Param("id")
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanEditTask(c, t); err != nil {
		return err
	}
	loc := "/workboard/tasks/" + id + "/edit"
	if t.ParentTaskID != "" {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_parent")
	}
	if err := h.repo.UpdateRecurrenceSettings(id,
		c.FormValue("complete_policy"),
		c.FormValue("progress_include_future") == "1",
		c.FormValue("final_result")); err != nil {
		return err
	}
	_ = h.repo.MaybeAutoCompleteParent(id)
	return c.Redirect(http.StatusSeeOther, loc+"?ok=saved")
}

func (h *WorkboardHandler) ArchiveRecurrence(c echo.Context) error {
	id := c.Param("id")
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanEditTask(c, t); err != nil {
		return err
	}
	loc := "/workboard/tasks/" + id
	if t.ParentTaskID != "" {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_parent")
	}
	on := c.FormValue("archived") != "0"
	if err := h.repo.SetRecurrenceArchived(id, on); err != nil {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_rule")
	}
	ok := "rec_archived"
	if !on {
		ok = "rec_restored"
	}
	return c.Redirect(http.StatusSeeOther, loc+"?ok="+ok)
}

func (h *WorkboardHandler) DeleteRecurrenceParent(c echo.Context) error {
	id := c.Param("id")
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	loc := "/workboard/tasks/" + id
	key, why := deleteKeyForTask(*t)
	if key == "" {
		msg := why
		if msg == "" {
			msg = msgBulkNoKey
		}
		return c.Redirect(http.StatusSeeOther, loc+"?err="+url.QueryEscape(msg))
	}
	if reason := h.listDeleteBlockReason(c, t); reason != "" {
		return c.Redirect(http.StatusSeeOther, loc+"?err="+url.QueryEscape(reason))
	}
	ok, reason, _ := h.deleteTaskForList(c, t)
	if !ok {
		if reason == "" {
			reason = "삭제할 수 없습니다"
		}
		return c.Redirect(http.StatusSeeOther, loc+"?err="+url.QueryEscape(reason))
	}
	back := strings.TrimSpace(c.FormValue("back"))
	if back == "" {
		back = workboardHomeURL("")
	}
	return c.Redirect(http.StatusSeeOther, back+"?ok=rec_deleted")
}

// applyRecurrenceFromForm 등록 폼에서 「기간 내 반복 실행」이 켜져 있으면 실행 작업을 만든다. §33.4
func applyRecurrenceFromForm(c echo.Context, repo *repository.WBRepo, t *model.WorkTask, missing func(int) bool) (applied bool, errCode string) {
	if c == nil || repo == nil || t == nil || strings.TrimSpace(c.FormValue("recurrence_enabled")) != "1" {
		return false, ""
	}
	rule := parseRecurrenceForm(c)
	if model.NormalizeRecurrenceRuleType(rule.RuleType) == model.RecurrenceNone {
		return false, ""
	}
	rule.TaskID = t.TaskID
	prev := service.ExpandOccurrencePreview(rule, service.DefaultCalendar(), missing)
	if prev.RejectYear {
		return false, "rec_year"
	}
	if prev.Reject500 {
		return false, "rec_limit"
	}
	if prev.Count == 0 || (prev.Error != "" && !prev.Warn200) {
		return false, "rec_rule"
	}
	if prev.Warn200 && c.FormValue("confirm_over_200") != "1" {
		return false, "rec_warn"
	}
	dates := filterSkippedDates(prev.Dates, overlapSkipDateSet(c))
	if len(dates) == 0 {
		return false, ""
	}
	if _, err := repo.GenerateOccurrences(t, rule, dates); err != nil {
		if strings.Contains(err.Error(), "이미 실행 작업") {
			return false, "rec_exists"
		}
		return false, "rec_rule"
	}
	return true, ""
}

// closeAllTargets 95-200: 요청의 상위·회차를 확인하고 편집 권한을 검사한다.
func (h *WorkboardHandler) closeAllTargets(c echo.Context) (parent, occ *model.WorkTask, err error) {
	parentID := c.Param("id")
	oid := c.Param("oid")
	occ, e := h.repo.GetTask(oid)
	if e != nil || occ == nil {
		return nil, nil, echo.ErrNotFound
	}
	if occ.ParentTaskID != parentID || occ.RecurrenceRole != model.RecurrenceRoleOccurrence {
		return nil, nil, echo.ErrNotFound
	}
	parent, e = h.repo.GetTask(parentID)
	if e != nil || parent == nil || parent.RecurrenceRole != model.RecurrenceRoleParent {
		return nil, nil, echo.ErrNotFound
	}
	for _, t := range []*model.WorkTask{occ, parent} {
		if denyOtherOrgTask(c, t) != nil {
			return nil, nil, echo.ErrForbidden
		}
		if e := denyUnlessCanEditTask(c, t); e != nil {
			return nil, nil, e
		}
	}
	return parent, occ, nil
}

// CloseAllOccurrencesPreview GET …/close-all/preview — 모달의 건수 요약(JSON).
func (h *WorkboardHandler) CloseAllOccurrencesPreview(c echo.Context) error {
	parent, occ, err := h.closeAllTargets(c)
	if err != nil {
		return err
	}
	pv, err := h.repo.PreviewCloseOccurrences(parent.TaskID, occ.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrOccCloseAlready) {
			return c.JSON(http.StatusConflict, map[string]interface{}{"error": "rec_closed_already"})
		}
		return echo.ErrNotFound
	}
	return c.JSON(http.StatusOK, pv)
}

// CloseAllOccurrences POST …/close-all — 이 회차 완료 + 이후 일정 제외 + 상위 업무 완료.
func (h *WorkboardHandler) CloseAllOccurrences(c echo.Context) error {
	parent, occ, err := h.closeAllTargets(c)
	if err != nil {
		return err
	}
	parentLoc := "/workboard/tasks/" + parent.TaskID
	occLoc := "/workboard/tasks/" + occ.TaskID
	include := true
	if c.FormValue("earlier_sent") == "1" {
		include = c.FormValue("include_earlier") == "1"
	}
	res, err := h.repo.CloseOccurrencesEarly(parent.TaskID, occ.TaskID, c.FormValue("close_reason"),
		c.FormValue("final_result"), include, currentUserDisplayName(c))
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrOccCloseAlready):
			return c.Redirect(http.StatusSeeOther, parentLoc+"?err=rec_closed_already")
		case errors.Is(err, repository.ErrOccCloseReason):
			return c.Redirect(http.StatusSeeOther, occLoc+"?err=rec_close_reason")
		case errors.Is(err, repository.ErrOccCloseResult):
			return c.Redirect(http.StatusSeeOther, occLoc+"?err=rec_result")
		case errors.Is(err, repository.ErrOccCloseNotFound):
			return echo.ErrNotFound
		}
		return c.Redirect(http.StatusSeeOther, occLoc+"?err=rec_rule")
	}
	_, _ = h.repo.InsertWorkTaskProcess(model.WorkTaskProcess{
		TaskID: occ.TaskID, OrgID: occ.OrgID, Worker: ctxString(c, "user_name"), WorkerUserID: currentUserID(c),
		WorkContent: fmt.Sprintf("업무 모두 종료: %s (제외 %d건%s)", strings.TrimSpace(c.FormValue("close_reason")), res.Skipped,
			map[bool]string{true: ", 상위 업무 완료", false: ""}[res.ParentCompleted]),
	})
	if res.EarlierKept > 0 {
		return c.Redirect(http.StatusSeeOther, fmt.Sprintf("%s?ok=rec_closed_partial&n=%d&keep=%d", parentLoc, res.Skipped, res.EarlierKept))
	}
	return c.Redirect(http.StatusSeeOther, fmt.Sprintf("%s?ok=rec_closed&n=%d", parentLoc, res.Skipped))
}

// attachOccCloseAll 95-200: 「업무 모두 종료」 버튼 노출 조건.
//   - 하위(실행) 업무 상세: 쓰기 권한 + 상위가 열려 있고 보관 아님 + 이 회차 외에 열린 회차가 1건 이상
//   - 상위 업무 실행 이력 표: 쓰기 권한 + 상위가 열려 있고 보관 아님 + 열린 회차가 2건 이상(행마다 「이 회차 외 1건 이상」)
func (h *WorkboardHandler) attachOccCloseAll(c echo.Context, data map[string]interface{}, t *model.WorkTask, archived bool, openN int) {
	if t == nil || !canWriteBoolFromData(data) {
		return
	}
	switch t.RecurrenceRole {
	case model.RecurrenceRoleParent:
		if t.Status == model.WBTaskComplete || t.Status == model.WBTaskCancelled || archived {
			return
		}
		data["OccCloseRows"] = openN >= 2
	case model.RecurrenceRoleOccurrence:
		p, err := h.repo.GetTask(t.ParentTaskID)
		if err != nil || p == nil || p.RecurrenceRole != model.RecurrenceRoleParent ||
			p.Status == model.WBTaskComplete || p.Status == model.WBTaskCancelled {
			return
		}
		if rule, _ := h.repo.GetRecurrence(p.TaskID); rule != nil && rule.Archived {
			return
		}
		open, err := h.repo.CountOpenOccurrences(p.TaskID)
		if err != nil {
			return
		}
		if model.OccurrenceOpen(*t) {
			open-- // 이 회차를 뺀 나머지
		}
		if open >= 1 {
			data["OccCloseHeader"] = true
			data["OccCloseParentID"] = p.TaskID
		}
	}
}

func canWriteBoolFromData(data map[string]interface{}) bool {
	v, _ := data["CanWrite"].(bool)
	return v
}

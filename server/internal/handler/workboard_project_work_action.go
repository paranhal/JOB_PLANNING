package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func projectWorkActionPath(id string) string {
	return "/workboard/tasks/" + id + "/action"
}

func isProjectWorkClosed(t *model.WorkTask) bool {
	if t == nil {
		return false
	}
	switch t.Status {
	case model.WBTaskComplete, model.WBTaskCancelled:
		return true
	}
	return t.ResultCode == model.WorkResultDone || t.ResultCode == model.WorkResultCancel
}

func canWriteProjectWorkAction(c echo.Context, t *model.WorkTask, members []model.WorkTaskMember) bool {
	if t == nil || isReadOnly(c) || !canWriteWorkboard(c) {
		return false
	}
	if canEditTask(c, t.Assignee, t.AssigneeUserID) {
		return true
	}
	for _, m := range members {
		if assigneeIsMine(c, m.Assignee, m.UserID) {
			return true
		}
	}
	return false
}

func (h *WorkboardHandler) loadProjectWorkForAction(c echo.Context) (*model.WorkTask, error) {
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return nil, echo.ErrNotFound
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return nil, err
	}
	if t.WorkType != model.WBWorkProjectWork {
		return nil, echo.NewHTTPError(http.StatusNotFound)
	}
	_ = h.repo.LoadTaskActionFields(t)
	return t, nil
}

func (h *WorkboardHandler) ShowProjectWorkAction(c echo.Context) error {
	t, err := h.loadProjectWorkForAction(c)
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok && he.Code == http.StatusNotFound && c.Param("id") != "" {
			if t2, e2 := h.repo.GetTask(c.Param("id")); e2 == nil && t2 != nil && t2.WorkType != model.WBWorkProjectWork {
				return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t2.TaskID)
			}
		}
		return err
	}
	return h.renderProjectWorkAction(c, t)
}

func (h *WorkboardHandler) renderProjectWorkAction(c echo.Context, t *model.WorkTask) error {
	members, _ := h.repo.ListMembers(t.TaskID)
	sites, _ := h.repo.ListTaskSites(t.TaskID)
	procs, _ := h.repo.ListWorkTaskProcesses(t.OrgID, t.TaskID)
	parts, _ := h.repo.ListWorkTaskParts(t.OrgID, t.TaskID)
	var atts []model.Attachment
	if h.attach != nil && h.attach.repo != nil {
		atts, _ = h.attach.repo.ListByRef(model.RefTypeWorkTask, t.TaskID)
	}
	closed := isProjectWorkClosed(t)
	unlocked, unlockExp := false, time.Time{}
	if h.unlockRepo != nil {
		unlocked, unlockExp, _ = h.unlockRepo.HasActive(t.TaskID, currentUserID(c))
	}
	canWrite := canWriteProjectWorkAction(c, t, members)
	canMod := canWrite && (!closed || unlocked)
	readOnly := !canMod
	now := time.Now()
	completeLocal := strings.TrimSpace(t.CompleteDatetime)
	if v, ok := c.Get("pw_complete_local").(string); ok {
		completeLocal = v
	} else if completeLocal == "" {
		completeLocal = now.Format("2006-01-02T15:04")
	}
	unlockExpLocal := ""
	if unlocked && !unlockExp.IsZero() {
		unlockExpLocal = unlockExp.Format("15:04")
	}
	if draft, ok := c.Get("pw_parts_rows").([]model.ASPartUsed); ok {
		parts = draft
	}
	enabled := len(parts) > 0
	if v, ok := c.Get("pw_parts_enabled").(bool); ok {
		enabled = v
	}
	projectName := strings.TrimSpace(t.ProjectName)
	if projectName == "" {
		projectName = "사업 없음"
	}
	back := strings.TrimSpace(c.QueryParam("back"))
	if back == "" {
		back = strings.TrimSpace(c.FormValue("back"))
	}
	if back == "" {
		back = workboardHomeURL("")
	}
	pick, _ := h.repo.ListWorkPickProjects(t.OrgID)
	cats, _ := h.repo.ListWorkTargetCategories(t.OrgID)
	var siteIDs []string
	for _, s := range sites {
		if id := strings.TrimSpace(s.SiteID); id != "" {
			siteIDs = append(siteIDs, id)
		}
	}
	hist, _ := h.repo.ListPastWorkHistory(t.OrgID, t.TaskID, t.ProjectID, siteIDs, 3)
	var projectSites []repository.WorkTaskSite
	if strings.TrimSpace(t.ProjectID) != "" && len(sites) == 0 {
		projectSites, _ = h.repo.ListProjectSitesForPick(t.ProjectID)
	}
	var photos, docs []model.Attachment
	for _, a := range atts {
		if a.IsImage() {
			photos = append(photos, a)
		} else {
			docs = append(docs, a)
		}
	}
	errCode := strings.TrimSpace(c.QueryParam("err"))
	if v, ok := c.Get("pw_action_err").(string); ok && v != "" {
		errCode = v
	}
	errMsg := projectWorkActionErr(errCode)
	if v, ok := c.Get("pw_action_err_msg").(string); ok && v != "" {
		errMsg = v
	}
	cancelLocal := now.Format("2006-01-02")
	if v := strings.TrimSpace(c.FormValue("cancel_date")); v != "" {
		cancelLocal = v
	}
	ver, _, _, _ := currentBuild()
	data := map[string]interface{}{
		"Title": "작업 조치 · " + t.Title, "Active": workboardNavActive(back),
		"Task": t, "Sites": sites, "Members": members, "Processes": procs,
		"PartRows": parts, "PartsEnabled": enabled, "Attachments": docs,
		"ActionPhotos":    photos,
		"Assignees":       ensurePickerUsersNamed(h.userRepo, listAssignableForPicker(h.userRepo, c), storedPickerNamesFromTask(t, members), t.AssigneeUserID),
		"DisplayAssignee": userDisplayName(h.userRepo, t.AssigneeUserID, t.Assignee),
		"CanProcess":      canMod, "ReadOnly": readOnly, "CanWrite": canWrite,
		"CanPlanEdit":   canPlanEditTask(c, t),
		"CanCarry":      canPlanEditTask(c, t),
		"TaskDelayed":   model.TaskDelayState(*t, now.Format(dateLayout)).Delayed,
		"TaskDelayDays": model.TaskDelayState(*t, now.Format(dateLayout)).Days,
		"PlanEditHref":  planEditHref(t.TaskID),
		"IsClosed":      closed, "EditUnlocked": unlocked, "UnlockExpires": unlockExpLocal,
		"CanUnlockEdit":   closed && isAdminRole(c) && !unlocked,
		"ShowWorkflow":    canWrite,
		"ProjectName":     projectName,
		"Projects":        pick,
		"WorkProjects":    pick,
		"TargetCats":      cats,
		"ShowTargetCat":   len(cats) > 0,
		"PastHistory":     hist,
		"ProjectSites":    projectSites,
		"ShowImportSites": len(projectSites) > 0,
		"CompleteLocal":   completeLocal,
		"TodayLocal":      now.Format("2006-01-02"),
		"CancelLocal":     cancelLocal,
		"WorkerDefault":   ctxString(c, "user_name"),
		"ActionErr":       errCode,
		"ActionErrMsg":    errMsg,
		"ActionErrField":  projectWorkActionErrField(errCode),
		"ActionOK":        c.QueryParam("ok"),
		"BackURL":         back,
		"ListURL":         workboardHomeURL(""),
		"CancelURL":       projectWorkCancelURL(c, t.TaskID, back),
		"SubtypeLabel":    model.WorkSubtypeLabel(t.WorkSubtype),
		"StatusLabel":     model.WorkTaskActionStatusLabel(t.Status, t.ResultCode),
		"ResultDone":      model.WorkResultDone,
		"ResultPartial":   model.WorkResultPartial,
		"ResultTransfer":  model.WorkResultTransfer,
		"AttachRefID":     t.TaskID,
		"AttachRedirect":  projectWorkActionPath(t.TaskID),
		"AttachPhotoType": model.RefTypeWorkTask,
		"AttachDocType":   model.RefTypeWorkTask,
		"AppVer":          ver,
	}
	if msg, ok := c.Get("pw_parts_msg").(string); ok && msg != "" {
		data["PartsFormErr"] = msg
		data["ActionErrMsg"] = msg
	}
	if v, ok := c.Get("pw_action_na").(bool); ok {
		data["ActionNA"] = v
	}
	if v, ok := c.Get("pw_na_reason").(string); ok {
		data["ActionNAReason"] = v
	}
	if v, ok := c.Get("pw_followup_note").(string); ok {
		data["FollowupNote"] = v
	}
	if v, ok := c.Get("pw_next_date").(string); ok {
		data["NextDate"] = v
	}
	if v, ok := c.Get("pw_create_followup").(bool); ok {
		data["CreateFollowup"] = v
	}
	if nd, _ := data["NextDate"].(string); strings.TrimSpace(nd) == "" {
		who := []string{t.Assignee, t.AssigneeUserID}
		if mems, err := h.repo.ListMembers(t.TaskID); err == nil {
			who = taskWhoNames(t, mems)
		}
		next, skipped := nextWorkdayFor(h, now, who, t.OrgID, false)
		if next == "" {
			next = now.AddDate(0, 0, 1).Format(dateLayout)
		}
		data["NextDate"] = next
		data["CarrySkipNotice"] = service.FormatSkipNotice(next, skipped)
	}
	mergeCarryModal(data)
	if hist, err := h.repo.ListDelaysByTask(t.TaskID); err == nil {
		data["DelayHistory"] = hist
	}
	if v, ok := c.Get("pw_transfer_reason").(string); ok {
		data["TransferReason"] = v
	}
	if v, ok := c.Get("pw_hold_reason").(string); ok {
		data["HoldReason"] = v
	}
	if v, ok := c.Get("pw_hold_next").(string); ok {
		data["HoldNextAction"] = v
	}
	if v, ok := c.Get("pw_cancel_reason").(string); ok {
		data["CancelReason"] = v
	}
	data["ConfirmStartDiffersPlan"] = errCode == "start_differs_plan"
	data["ConfirmSegShift"] = errCode == "seg_shift"
	items, _ := h.repo.ListItems(t.TaskID)
	if items == nil {
		items = []model.WorkTaskItem{}
	}
	data["WorkItems"] = items
	attachWorkItemView(data, h, t, readOnly)
	attachWorkSegData(data, h.repo, t, errCode, projectWorkActionPath(t.TaskID), readOnly)
	attachLiveWorkData(data, h, c, t)
	if v, ok := c.Get("pw_action_err").(string); ok && v != "" {
		overlayProjectWorkActionDraft(c, t)
		overlaySupportDraft(c, listAssignableForPicker(h.userRepo, c))
		if draft, ok := c.Get("pw_parts_rows").([]model.ASPartUsed); ok {
			data["PartRows"] = draft
		}
		if en, ok := c.Get("pw_parts_enabled").(bool); ok {
			data["PartsEnabled"] = en
		}
		if cl, ok := c.Get("pw_complete_local").(string); ok {
			data["CompleteLocal"] = cl
		}
	}
	deps, _ := h.repo.ListDepartures(t.TaskID)
	data["Departures"] = deps
	startLocal := defaultActionStartLocal(t, now)
	if v, ok := c.Get("pw_start_local").(string); ok && v != "" {
		startLocal = v
	}
	data["StartLocal"] = startLocal
	dur := defaultActionDuration(t)
	if v, ok := c.Get("pw_duration_min").(int); ok && v > 0 {
		dur = v
	}
	data["ActionDurationMin"] = dur
	data["DurationOptions"] = model.ActionDurationMins
	data["DurationCustom"] = !model.IsActionDurationOption(dur)
	if model.IsActionDurationOption(dur) {
		data["ActionDurationSel"] = strconv.Itoa(dur)
	} else {
		data["ActionDurationSel"] = "custom"
	}
	attachPlanLink(data, h.repo, t.ProjectID, t.TaskID)
	supports := supportMembersOnly(members)
	if v, ok := c.Get("pw_supports").([]model.WorkTaskMember); ok {
		supports = v
	}
	data["SupportMembers"] = supports
	if v, ok := c.Get("pw_action_remark").(string); ok {
		t.ActionRemark = v
	}
	if v, ok := c.Get("pw_confirmed_at").(string); ok {
		t.CustomerConfirmedAt = v
	}
	if v, ok := c.Get("pw_recurrence").(bool); ok {
		t.RecurrencePossible = v
	}
	if v, ok := c.Get("pw_title").(string); ok && v != "" {
		t.Title = v
	}
	formResult := t.ResultCode
	if v, ok := c.Get("pw_form_result").(string); ok && v != "" {
		formResult = v
	}
	data["FormResult"] = formResult
	attachProjectPicker(c, h, data, ProjectPickerOpts{
		Selected: t.ProjectID, SelectedName: t.ProjectName, EmptyLabel: "연결 해제",
	})
	return c.Render(http.StatusOK, "workboard/project_work_action.html", data)
}

func projectWorkActionErr(code string) string {
	switch code {
	case "content":
		return "조치 내용을 입력하세요"
	case "na_reason":
		return "해당 없음 사유를 입력하세요"
	case "work_place":
		return "근무 구분을 선택하세요"
	case "work_subtype":
		return "작업 구분을 선택하세요"
	case "result":
		return "조치 결과를 선택하세요"
	case "project":
		return "사업을 찾을 수 없습니다"
	case "project_match":
		return projectMatchNeededMsg
	case "assignee":
		return "담당자를 선택하세요"
	case "work_date":
		return "작업일을 입력하세요"
	case "action_start":
		return "시작 일시를 입력하세요"
	case "action_duration":
		return "작업 시간을 입력하세요"
	case "title":
		return "작업명을 입력하세요"
	case "complete_dt":
		return "완료 일시를 입력하세요"
	case "complete_before_start":
		return "완료 일시가 시작 일시보다 이전입니다"
	case "start_differs_plan":
		return "작업 일정과 다른 날짜입니다. 그대로 저장할까요?"
	case "seg_shift":
		return "끼워 넣은 업무의 실제 시간이 다음 구간과 겹칩니다. 후속 구간을 조정할까요?"
	case "supporter_owner":
		return "처리 담당자는 지원 인력에 넣을 수 없습니다"
	case "supporters_max":
		return "지원 인력은 최대 20명입니다"
	case "item":
		return "작업 항목을 확인하세요"
	case "partial_note":
		return "추가조치 사유를 입력하세요"
	case "hold_reason":
		return "대기 사유를 입력하세요"
	case "hold_next":
		return "대기 후속을 선택하세요"
	case "transfer_to":
		return "이관처를 입력하세요"
	case "transfer_reason":
		return "이관 사유를 입력하세요"
	case "cancel_reason":
		return "취소 사유를 입력하세요"
	case "cancel_date":
		return "취소일을 입력하세요"
	case "locked":
		return "완료·취소 건은 읽기 전용입니다. 관리자는 수정 잠금 해제 후 이용하세요"
	case "forbidden":
		return "권한이 없습니다"
	default:
		return ""
	}
}

func projectWorkActionErrField(code string) string {
	switch code {
	case "content":
		return "action_taken"
	case "na_reason":
		return "action_na_reason"
	case "work_place":
		return "work_place"
	case "work_subtype":
		return "work_subtype"
	case "result":
		return "result_code"
	case "assignee":
		return "assigned_to_code"
	case "action_start", "start_differs_plan":
		return "action_start_at"
	case "action_duration":
		return "action_duration_min"
	case "title":
		return "title"
	case "complete_dt", "complete_before_start":
		return "complete_datetime"
	case "partial_note":
		return "followup_note"
	case "transfer_to":
		return "transfer_to"
	case "transfer_reason":
		return "transfer_reason"
	case "hold_reason":
		return "hold_reason"
	case "hold_next":
		return "hold_next_action"
	case "cancel_reason":
		return "cancel_reason"
	case "cancel_date":
		return "cancel_date"
	default:
		return ""
	}
}

func resolveProjectWorkDate(formDay, taskDay, completeDT string) string {
	if d := strings.TrimSpace(formDay); d != "" {
		return d
	}
	if d := strings.TrimSpace(taskDay); d != "" {
		return d
	}
	c := strings.TrimSpace(completeDT)
	if len(c) >= 10 {
		return c[:10]
	}
	return ""
}

func completeDatetimeBeforeWorkDate(completeDT, workDay string) bool {
	c := strings.TrimSpace(completeDT)
	w := strings.TrimSpace(workDay)
	if len(c) < 10 || w == "" {
		return false
	}
	return c[:10] < w
}

func overlayProjectWorkActionDraft(c echo.Context, t *model.WorkTask) {
	if t == nil {
		return
	}
	_ = c.Request().ParseForm()
	if v := strings.TrimSpace(c.FormValue("work_place")); v != "" {
		t.WorkPlace = v
	}
	if v := model.NormalizeWorkSubtype(c.FormValue("work_subtype")); v != "" {
		t.WorkSubtype = v
	}
	if _, ok := c.Request().PostForm["action_taken"]; ok {
		t.ActionTaken = strings.TrimSpace(c.FormValue("action_taken"))
	}
	if v := strings.TrimSpace(c.FormValue("result_code")); v != "" {
		c.Set("pw_form_result", v)
	}
	if v := strings.TrimSpace(c.FormValue("work_reason")); v != "" || c.Request().PostForm.Has("work_reason") {
		t.WorkReason = strings.TrimSpace(c.FormValue("work_reason"))
	}
	if v := strings.TrimSpace(c.FormValue("target_category_id")); v != "" || c.Request().PostForm.Has("target_category_id") {
		t.TargetCategoryID = strings.TrimSpace(c.FormValue("target_category_id"))
	}
	if v := strings.TrimSpace(c.FormValue("complete_datetime")); v != "" {
		c.Set("pw_complete_local", v)
		t.CompleteDatetime = v
	} else if _, ok := c.Request().PostForm["complete_datetime"]; ok {
		c.Set("pw_complete_local", "")
	}
	if v := strings.TrimSpace(c.FormValue("action_start_at")); v != "" {
		c.Set("pw_start_local", v)
		t.ActionStartAt = v
	}
	if n, code := parseActionDurationMin(c); code == "" {
		c.Set("pw_duration_min", n)
		t.ActionDurationMin = n
	}
	if v := strings.TrimSpace(c.FormValue("title")); v != "" {
		c.Set("pw_title", v)
		t.Title = v
	}
	if _, ok := c.Request().PostForm["description"]; ok {
		t.Description = strings.TrimSpace(c.FormValue("description"))
	}
	if _, ok := c.Request().PostForm["action_remark"]; ok {
		c.Set("pw_action_remark", strings.TrimSpace(c.FormValue("action_remark")))
		t.ActionRemark = strings.TrimSpace(c.FormValue("action_remark"))
	}
	if _, ok := c.Request().PostForm["customer_confirmed_at"]; ok {
		c.Set("pw_confirmed_at", strings.TrimSpace(c.FormValue("customer_confirmed_at")))
		t.CustomerConfirmedAt = strings.TrimSpace(c.FormValue("customer_confirmed_at"))
	}
	c.Set("pw_recurrence", c.FormValue("recurrence_possible") == "1")
	t.RecurrencePossible = c.FormValue("recurrence_possible") == "1"
	confirmer := strings.TrimSpace(c.FormValue("customer_confirmer"))
	if custom := strings.TrimSpace(c.FormValue("customer_confirmer_custom")); custom != "" {
		confirmer = custom
	}
	if confirmer != "" {
		t.CustomerConfirmer = confirmer
	}
	code := strings.TrimSpace(c.FormValue("assigned_to_code"))
	if code == "custom" {
		t.Assignee = strings.TrimSpace(c.FormValue("assigned_to_custom"))
		t.AssigneeUserID = ""
	} else if code != "" {
		t.AssigneeUserID = code
	}
	if v := strings.TrimSpace(c.FormValue("transfer_to")); v != "" {
		t.TransferTo = v
	}
	c.Set("pw_action_na", c.FormValue("action_na") == "1")
	c.Set("pw_na_reason", strings.TrimSpace(c.FormValue("action_na_reason")))
	c.Set("pw_followup_note", strings.TrimSpace(c.FormValue("followup_note")))
	c.Set("pw_next_date", strings.TrimSpace(c.FormValue("next_date")))
	c.Set("pw_create_followup", c.FormValue("create_followup") == "1")
	c.Set("pw_transfer_reason", strings.TrimSpace(c.FormValue("transfer_reason")))
	c.Set("pw_hold_reason", strings.TrimSpace(c.FormValue("hold_reason")))
	c.Set("pw_hold_next", strings.TrimSpace(c.FormValue("hold_next_action")))
	c.Set("pw_cancel_reason", strings.TrimSpace(c.FormValue("cancel_reason")))
}

func (h *WorkboardHandler) failPWAction(c echo.Context, t *model.WorkTask, code string) error {
	overlayProjectWorkActionDraft(c, t)
	overlaySupportDraft(c, listAssignableForPicker(h.userRepo, c))
	c.Set("pw_action_err", code)
	if code == "start_differs_plan" {
		c.Set("pw_action_err_msg", "작업 일정("+strings.TrimSpace(t.WorkDate)+")과 다른 날짜입니다. 그대로 저장할까요?")
	}
	return h.renderProjectWorkAction(c, t)
}

func (h *WorkboardHandler) redirectPWAction(c echo.Context, id, errCode string) error {
	q := url.Values{}
	if errCode != "" {
		q.Set("err", errCode)
	}
	if b := strings.TrimSpace(c.FormValue("back")); b != "" {
		q.Set("back", b)
	}
	u := projectWorkActionPath(id)
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	return c.Redirect(http.StatusSeeOther, u)
}

func (h *WorkboardHandler) SaveProjectWorkAction(c echo.Context) error {
	t, err := h.loadProjectWorkForAction(c)
	if err != nil {
		return err
	}
	members, _ := h.repo.ListMembers(t.TaskID)
	if !canWriteProjectWorkAction(c, t, members) {
		return echo.ErrForbidden
	}
	if err := denyIfProjectMatchPending(t); err != nil {
		return h.failPWAction(c, t, "project_match")
	}
	closed := isProjectWorkClosed(t)
	unlocked := false
	if h.unlockRepo != nil {
		unlocked, _, _ = h.unlockRepo.HasActive(t.TaskID, currentUserID(c))
	}
	if closed && !unlocked {
		return h.failPWAction(c, t, "locked")
	}
	if err := h.applyProjectWorkActionForm(c, t, ""); err != nil {
		if _, ok := err.(pwRendered); ok {
			return nil
		}
		if code, ok := err.(*pwActionErr); ok {
			return h.failPWAction(c, t, code.code)
		}
		return err
	}
	return c.Redirect(http.StatusSeeOther, projectWorkActionPath(t.TaskID)+queryBack(c, "ok=saved"))
}

func queryBack(c echo.Context, extra string) string {
	q := url.Values{}
	if b := strings.TrimSpace(c.FormValue("back")); b != "" {
		q.Set("back", b)
	}
	for _, p := range strings.Split(extra, "&") {
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			q.Set(kv[0], kv[1])
		}
	}
	if enc := q.Encode(); enc != "" {
		return "?" + enc
	}
	return ""
}

type pwActionErr struct{ code string }

func (e *pwActionErr) Error() string { return e.code }

type pwRendered struct{}

func (pwRendered) Error() string { return "rendered" }

func (h *WorkboardHandler) applyProjectWorkActionForm(c echo.Context, t *model.WorkTask, defaultResult string) error {
	result := strings.TrimSpace(c.FormValue("result_code"))
	if result == "" {
		result = defaultResult
	}
	if result == "" {
		return &pwActionErr{"result"}
	}
	place := strings.TrimSpace(c.FormValue("work_place"))
	if place != "office" && place != "field" {
		return &pwActionErr{"work_place"}
	}
	sub := model.NormalizeWorkSubtype(c.FormValue("work_subtype"))
	if sub == "" {
		sub = model.NormalizeWorkSubtype(t.WorkSubtype)
	}
	if sub == "" {
		return &pwActionErr{"work_subtype"}
	}
	assignee, assigneeUID, aerr := h.resolveProjectWorkAssignee(c, t)
	if aerr != nil {
		return aerr
	}
	completeDT := strings.TrimSpace(c.FormValue("complete_datetime"))
	startLocal, startDay, startErr := parseActionStartAt(c.FormValue("action_start_at"))
	if startErr != "" {
		return &pwActionErr{startErr}
	}
	durMin, durErr := parseActionDurationMin(c)
	if durErr != "" {
		return &pwActionErr{durErr}
	}
	if title := strings.TrimSpace(c.FormValue("title")); title != "" {
		t.Title = title
	}
	if strings.TrimSpace(t.Title) == "" {
		return &pwActionErr{"title"}
	}
	if resolveProcessWorkDate(startDay, c.FormValue("work_date"), t.WorkDate, completeDT) == "" {
		return &pwActionErr{"action_start"}
	}
	na := c.FormValue("action_na") == "1"
	content := strings.TrimSpace(c.FormValue("action_taken"))
	if na {
		reason := strings.TrimSpace(c.FormValue("action_na_reason"))
		if reason == "" {
			return &pwActionErr{"na_reason"}
		}
		content = "해당 없음: " + reason
	} else if content == "" {
		return &pwActionErr{"content"}
	}
	on, parts, perr := collectWorkTaskPartsForm(c)
	if perr != nil {
		c.Set("pw_parts_enabled", true)
		c.Set("pw_parts_rows", parts)
		c.Set("pw_parts_msg", perr.Error())
		if err := h.renderProjectWorkAction(c, t); err != nil {
			return err
		}
		return pwRendered{}
	}

	note := strings.TrimSpace(c.FormValue("followup_note"))
	nextDate := strings.TrimSpace(c.FormValue("next_date"))
	confirmer := strings.TrimSpace(c.FormValue("customer_confirmer"))
	if custom := strings.TrimSpace(c.FormValue("customer_confirmer_custom")); custom != "" {
		confirmer = custom
	}

	if completeDT == "" && (result == model.WorkResultDone || result == model.WorkResultPartial) {
		completeDT = time.Now().Format("2006-01-02T15:04")
	}
	if (result == model.WorkResultDone || result == model.WorkResultPartial) &&
		completeDatetimeBeforeStart(completeDT, startLocal) {
		return &pwActionErr{"complete_before_start"}
	}
	if startDayDiffersPlan(startDay, t.WorkDate) &&
		c.FormValue("confirm_start_differs_plan") != "1" {
		return &pwActionErr{"start_differs_plan"}
	}
	supports, suppErr := collectProjectWorkSupports(c, assigneeUID, assignee, listAssignableForPicker(h.userRepo, c))
	if suppErr != "" {
		return &pwActionErr{suppErr}
	}
	t.WorkPlace = place
	t.Assignee = assignee
	t.AssigneeUserID = assigneeUID
	t.WorkSubtype = sub
	t.ActionTaken = content
	t.CustomerConfirmer = confirmer
	t.ResultCode = result
	t.TargetCategoryID = strings.TrimSpace(c.FormValue("target_category_id"))
	t.WorkReason = strings.TrimSpace(c.FormValue("work_reason"))
	t.ActionStartAt = startLocal
	t.ActionDurationMin = durMin
	t.ActionRemark = strings.TrimSpace(c.FormValue("action_remark"))
	t.RecurrencePossible = c.FormValue("recurrence_possible") == "1"
	t.CustomerConfirmedAt = strings.TrimSpace(c.FormValue("customer_confirmed_at"))
	if _, ok := c.Request().PostForm["description"]; ok {
		t.Description = strings.TrimSpace(c.FormValue("description"))
	}

	var items []model.WorkTaskItem
	if _, postedItems := c.Request().PostForm["item_title"]; postedItems {
		allowed := itemAssigneeAllowed(c, h, t.TaskID)
		parsed, excludedIDs, err := parseWorkItemsForm(c, allowed)
		if err != nil {
			c.Set("pw_action_err_msg", err.Error())
			return h.failPWAction(c, t, "item")
		}
		old, _ := h.repo.ListItems(t.TaskID)
		oldByID := map[string]model.WorkTaskItem{}
		for _, o := range old {
			oldByID[o.ItemID] = o
		}
		for _, id := range excludedIDs {
			if o, ok := oldByID[id]; ok && itemHistoryLocked(o) {
				c.Set("pw_action_err_msg", "완료·연결 항목은 제외할 수 없습니다")
				return h.failPWAction(c, t, "item")
			}
		}
		if err := h.repo.ReplaceItems(t.TaskID, parsed); err != nil {
			return err
		}
		items = parsed
	} else {
		items, _ = h.repo.ListItems(t.TaskID)
	}
	hasOpen := false
	for _, it := range items {
		if model.ItemIncomplete(it.Status) {
			hasOpen = true
			break
		}
	}
	if result == model.WorkResultDone && hasOpen {
		result = model.WorkResultPartial
		t.ResultCode = result
		if note == "" {
			note = "미완료 작업 항목이 있어 부분 완료로 저장했습니다"
		}
		c.Set("pw_force_followup_items", true)
	}

	if err := savePostedActionSegments(c, h.repo, t); err != nil {
		if errors.Is(err, errSegShift) {
			return &pwActionErr{"seg_shift"}
		}
		return err
	}

	switch result {
	case model.WorkResultDone:
		t.CompleteDatetime = completeDT
		t.CompleteDate = completeDT[:min(10, len(completeDT))]
		t.Status = model.WBTaskComplete
		t.Progress = 100
	case model.WorkResultPartial:
		if note == "" {
			return &pwActionErr{"partial_note"}
		}
		t.Status = model.StatusPartialComplete
		t.CompleteDatetime = completeDT
		if t.CompleteDate == "" && len(completeDT) >= 10 {
			t.CompleteDate = completeDT[:10]
		}
	case model.WorkResultTransfer:
		to := strings.TrimSpace(c.FormValue("transfer_to"))
		reason := strings.TrimSpace(c.FormValue("transfer_reason"))
		if to == "" {
			return &pwActionErr{"transfer_to"}
		}
		if reason == "" {
			return &pwActionErr{"transfer_reason"}
		}
		t.TransferTo = to
		t.Status = model.WBTaskTransfer
		note = reason
	default:
		return &pwActionErr{"result"}
	}
	if line := repository.FirstLine(content, 200); line != "" {
		t.CompleteNote = line
	}
	if err := h.repo.SaveTaskActionFields(t); err != nil {
		return err
	}
	if err := h.repo.ReplaceSupportMembers(t.TaskID, supports); err != nil {
		return err
	}
	processAt := startLocal
	if len(processAt) == 16 {
		processAt = processAt[:10] + " " + processAt[11:] + ":00"
	}
	pid, err := h.repo.InsertWorkTaskProcess(model.WorkTaskProcess{
		TaskID: t.TaskID, OrgID: t.OrgID, Worker: ctxString(c, "user_name"),
		WorkerUserID: currentUserID(c), WorkPlace: place, ResultCode: result,
		WorkContent: content, Note: note, NextDate: nextDate, ProcessAt: processAt,
	})
	if err != nil {
		return err
	}
	if c.FormValue("parts_form") == "1" {
		if !on {
			_ = h.repo.ReplaceWorkTaskParts(t.OrgID, t.TaskID, pid, nil)
		} else {
			_ = h.repo.ReplaceWorkTaskParts(t.OrgID, t.TaskID, pid, parts)
		}
	}
	if result == model.WorkResultDone {
		_ = h.repo.SyncSitesOnProjectWorkComplete(t, currentUserID(c))
	}
	if result == model.WorkResultPartial && (c.FormValue("create_followup") == "1" || c.Get("pw_force_followup_items") == true) {
		if err := h.createPartialFollowup(c, t, nextDate); err != nil {
			return err
		}
	}
	return nil
}

func collectWorkTaskPartsForm(c echo.Context) (bool, []model.ASPartUsed, error) {
	if c.FormValue("parts_form") != "1" {
		return false, nil, nil
	}
	on := c.FormValue("parts_enabled") == "1"
	if !on {
		return false, nil, nil
	}
	_ = c.Request().ParseForm()
	form := c.Request().PostForm
	rows, err := model.ParseASPartsFromForm(form["part_name"], form["part_code"], form["part_qty"], form["part_item_id"])
	return true, rows, err
}

func (h *WorkboardHandler) createPartialFollowup(c echo.Context, src *model.WorkTask, nextDate string) error {
	if nextDate == "" {
		who := taskWhoNames(src, nil)
		nextDate, _ = nextWorkdayFor(h, time.Now(), who, src.OrgID, false)
		if nextDate == "" {
			nextDate = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
		}
	}
	n, err := h.repo.CountPartialFollowups(src.OrgID, src.TaskID, nextDate)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	child := *src
	child.TaskID = ""
	child.ParentTaskID = src.TaskID
	child.SourceType = model.WBWorkProjectWork
	child.SourceID = src.TaskID
	child.SourceRole = model.WorkSourcePartial
	child.Status = model.WBTaskWaiting
	child.Progress = 0
	child.WorkDate = nextDate
	child.DueDate = nextDate
	child.CompleteDate = ""
	child.CompleteDatetime = ""
	child.CompleteNote = ""
	child.ActionTaken = ""
	child.ResultCode = ""
	child.HoldReason = ""
	child.HoldNextAction = ""
	child.TransferTo = ""
	child.CancelReason = ""
	child.Title = strings.TrimSpace(src.Title)
	if child.Title != "" && !strings.Contains(child.Title, "후속") {
		child.Title = child.Title + " 후속"
	}
	if err := h.repo.CreateTask(&child); err != nil {
		return err
	}
	sites, _ := h.repo.ListTaskSites(src.TaskID)
	var ids []string
	for _, s := range sites {
		if id := strings.TrimSpace(s.SiteID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		_ = h.repo.ReplaceTaskCustomers(child.TaskID, src.ProjectID, ids)
	}
	_ = h.repo.CopyIncompleteItems(src.TaskID, child.TaskID)
	return nil
}

func (h *WorkboardHandler) HoldProjectWorkAction(c echo.Context) error {
	return h.saveProjectWorkSideResult(c, model.WorkResultHold)
}

func (h *WorkboardHandler) TransferProjectWorkAction(c echo.Context) error {
	return h.saveProjectWorkSideResult(c, model.WorkResultTransfer)
}

func (h *WorkboardHandler) CancelProjectWorkAction(c echo.Context) error {
	return h.saveProjectWorkSideResult(c, model.WorkResultCancel)
}

func (h *WorkboardHandler) saveProjectWorkSideResult(c echo.Context, result string) error {
	t, err := h.loadProjectWorkForAction(c)
	if err != nil {
		return err
	}
	members, _ := h.repo.ListMembers(t.TaskID)
	if !canWriteProjectWorkAction(c, t, members) {
		return echo.ErrForbidden
	}
	closed := isProjectWorkClosed(t)
	unlocked := false
	if h.unlockRepo != nil {
		unlocked, _, _ = h.unlockRepo.HasActive(t.TaskID, currentUserID(c))
	}
	if closed && !unlocked {
		return h.failPWAction(c, t, "locked")
	}
	place := strings.TrimSpace(c.FormValue("work_place"))
	if place == "" {
		place = t.WorkPlace
	}
	content := strings.TrimSpace(c.FormValue("action_taken"))
	if content == "" {
		content = t.ActionTaken
	}
	note := strings.TrimSpace(c.FormValue("hold_reason"))
	if result == model.WorkResultTransfer {
		note = strings.TrimSpace(c.FormValue("transfer_reason"))
		to := strings.TrimSpace(c.FormValue("transfer_to"))
		if to == "" {
			return h.failPWAction(c, t, "transfer_to")
		}
		if note == "" {
			return h.failPWAction(c, t, "transfer_reason")
		}
		t.TransferTo = to
		t.Status = model.WBTaskTransfer
	}
	if result == model.WorkResultHold {
		next := strings.TrimSpace(c.FormValue("hold_next_action"))
		if next != "action" && next != "transfer" && next != "cancel" {
			return h.failPWAction(c, t, "hold_next")
		}
		if note == "" {
			return h.failPWAction(c, t, "hold_reason")
		}
		t.HoldReason = note
		t.HoldNextAction = next
		t.Status = model.WBTaskHold
	}
	if result == model.WorkResultCancel {
		note = strings.TrimSpace(c.FormValue("cancel_reason"))
		day := strings.TrimSpace(c.FormValue("cancel_date"))
		if day == "" {
			return h.failPWAction(c, t, "cancel_date")
		}
		if note == "" {
			return h.failPWAction(c, t, "cancel_reason")
		}
		t.CancelReason = note
		t.CompleteDate = day
		t.Status = model.WBTaskCancelled
	}
	t.WorkPlace = place
	t.ActionTaken = content
	t.ResultCode = result
	if line := repository.FirstLine(content, 200); line != "" && t.CompleteNote == "" {
		t.CompleteNote = line
	}
	if err := h.repo.SaveTaskActionFields(t); err != nil {
		return err
	}
	_, err = h.repo.InsertWorkTaskProcess(model.WorkTaskProcess{
		TaskID: t.TaskID, OrgID: t.OrgID, Worker: ctxString(c, "user_name"),
		WorkerUserID: currentUserID(c), WorkPlace: place, ResultCode: result,
		WorkContent: content, Note: note,
	})
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, projectWorkActionPath(t.TaskID)+queryBack(c, "ok=saved"))
}

func (h *WorkboardHandler) ReleaseProjectWorkHold(c echo.Context) error {
	t, err := h.loadProjectWorkForAction(c)
	if err != nil {
		return err
	}
	members, _ := h.repo.ListMembers(t.TaskID)
	if !canWriteProjectWorkAction(c, t, members) {
		return echo.ErrForbidden
	}
	if t.Status != model.WBTaskHold {
		return c.Redirect(http.StatusSeeOther, projectWorkActionPath(t.TaskID))
	}
	t.Status = model.WBTaskInProgress
	t.ResultCode = ""
	t.HoldReason = ""
	t.HoldNextAction = ""
	if err := h.repo.SaveTaskActionFields(t); err != nil {
		return err
	}
	_, _ = h.repo.InsertWorkTaskProcess(model.WorkTaskProcess{
		TaskID: t.TaskID, OrgID: t.OrgID, Worker: ctxString(c, "user_name"),
		WorkerUserID: currentUserID(c), ResultCode: "resume", WorkContent: "대기 해제",
	})
	return c.Redirect(http.StatusSeeOther, projectWorkActionPath(t.TaskID)+queryBack(c, "ok=saved"))
}

func (h *WorkboardHandler) UnlockProjectWorkAction(c echo.Context) error {
	if !isVisionRole(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	t, err := h.loadProjectWorkForAction(c)
	if err != nil {
		return err
	}
	if !isProjectWorkClosed(t) {
		return c.Redirect(http.StatusSeeOther, projectWorkActionPath(t.TaskID))
	}
	pw := strings.TrimSpace(c.FormValue("unlock_password"))
	ok := verifyAndUpgradeSetting(h.settingsRepo, repository.SettingASCompletedEditPassword, pw)
	if h.unlockRepo != nil {
		_ = h.unlockRepo.LogAttempt(t.TaskID, currentUserID(c), ctxString(c, "username"), c.RealIP(), ok)
	}
	if !ok {
		return h.redirectPWAction(c, t.TaskID, "forbidden")
	}
	if h.unlockRepo == nil {
		return h.redirectPWAction(c, t.TaskID, "forbidden")
	}
	if _, err := h.unlockRepo.Grant(t.TaskID, currentUserID(c)); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, projectWorkActionPath(t.TaskID)+"?ok="+url.QueryEscape("수정 잠금이 해제되었습니다(30분)"))
}

func (h *WorkboardHandler) resolveProjectWorkAssignee(c echo.Context, t *model.WorkTask) (string, string, error) {
	code := strings.TrimSpace(c.FormValue("assigned_to_code"))
	custom := strings.TrimSpace(c.FormValue("assigned_to_custom"))
	if code == "custom" {
		if custom == "" {
			return "", "", &pwActionErr{"assignee"}
		}
		return custom, "", nil
	}
	if code != "" {
		for _, u := range listAssignableForPicker(h.userRepo, c) {
			if u.UserID == code {
				if err := checkAssigneeInOrg(h.userRepo, t.OrgID, u.FullName, u.UserID, t.Assignee); err != nil {
					return "", "", &pwActionErr{"assignee"}
				}
				return u.FullName, u.UserID, nil
			}
		}
	}
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	if assignee == "" {
		assignee = strings.TrimSpace(t.Assignee)
	}
	if assignee == "" {
		return "", "", &pwActionErr{"assignee"}
	}
	uid := strings.TrimSpace(c.FormValue("assignee_user_id"))
	if err := checkAssigneeInOrg(h.userRepo, t.OrgID, assignee, uid, t.Assignee); err != nil {
		return "", "", &pwActionErr{"assignee"}
	}
	return assignee, uid, nil
}

func (h *WorkboardHandler) canMutateProjectWorkAction(c echo.Context, t *model.WorkTask) error {
	members, _ := h.repo.ListMembers(t.TaskID)
	if !canWriteProjectWorkAction(c, t, members) {
		return echo.ErrForbidden
	}
	closed := isProjectWorkClosed(t)
	unlocked := false
	if h.unlockRepo != nil {
		unlocked, _, _ = h.unlockRepo.HasActive(t.TaskID, currentUserID(c))
	}
	if closed && !unlocked {
		return &pwActionErr{"locked"}
	}
	return nil
}

func (h *WorkboardHandler) LinkProjectWorkAction(c echo.Context) error {
	t, err := h.loadProjectWorkForAction(c)
	if err != nil {
		return err
	}
	if err := h.canMutateProjectWorkAction(c, t); err != nil {
		if code, ok := err.(*pwActionErr); ok {
			return h.redirectPWAction(c, t.TaskID, code.code)
		}
		return err
	}
	pid := strings.TrimSpace(c.FormValue("project_id"))
	if err := h.assertProjectLinkable(c, pid); err != nil {
		return err
	}
	before, after, err := h.repo.SetTaskProject(t.OrgID, t.TaskID, pid)
	if err != nil {
		return h.redirectPWAction(c, t.TaskID, "project")
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
	_, _ = h.repo.InsertWorkTaskProcess(model.WorkTaskProcess{
		TaskID: t.TaskID, OrgID: t.OrgID, Worker: ctxString(c, "user_name"),
		WorkerUserID: currentUserID(c), WorkContent: line,
	})
	return c.Redirect(http.StatusSeeOther, projectWorkActionPath(t.TaskID)+queryBack(c, "ok=saved"))
}

func (h *WorkboardHandler) ImportProjectWorkSites(c echo.Context) error {
	t, err := h.loadProjectWorkForAction(c)
	if err != nil {
		return err
	}
	if err := h.canMutateProjectWorkAction(c, t); err != nil {
		if code, ok := err.(*pwActionErr); ok {
			return h.redirectPWAction(c, t.TaskID, code.code)
		}
		return err
	}
	if strings.TrimSpace(t.ProjectID) == "" {
		return h.redirectPWAction(c, t.TaskID, "project")
	}
	_ = c.Request().ParseForm()
	if err := h.repo.AddTaskCustomers(t.TaskID, t.ProjectID, c.Request().Form["site_id"]); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, projectWorkActionPath(t.TaskID)+queryBack(c, "ok=saved"))
}

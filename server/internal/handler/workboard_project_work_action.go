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
	if completeLocal == "" {
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
	data := map[string]interface{}{
		"Title": "작업 조치 · " + t.Title, "Active": NavWorkRegister,
		"Task": t, "Sites": sites, "Members": members, "Processes": procs,
		"PartRows": parts, "PartsEnabled": enabled, "Attachments": atts,
		"Assignees":       listAssignableForPicker(h.userRepo, c),
		"CanProcess":      canMod, "ReadOnly": readOnly, "CanWrite": canWrite,
		"IsClosed":        closed, "EditUnlocked": unlocked, "UnlockExpires": unlockExpLocal,
		"CanUnlockEdit":   closed && isVisionRole(c) && !unlocked,
		"ProjectName":     projectName,
		"CompleteLocal":   completeLocal,
		"TodayLocal":      now.Format("2006-01-02"),
		"CancelLocal":     now.Format("2006-01-02"),
		"WorkerDefault":   ctxString(c, "user_name"),
		"ActionErr":       c.QueryParam("err"),
		"ActionErrMsg":    projectWorkActionErr(c.QueryParam("err")),
		"ActionOK":        c.QueryParam("ok"),
		"BackURL":         back,
		"SubtypeLabel":    model.WorkSubtypeLabel(t.WorkSubtype),
		"StatusLabel":     model.WorkTaskActionStatusLabel(t.Status, t.ResultCode),
		"ResultDone":      model.WorkResultDone,
		"ResultPartial":   model.WorkResultPartial,
	}
	if msg, ok := c.Get("pw_parts_msg").(string); ok && msg != "" {
		data["PartsFormErr"] = msg
		data["ActionErrMsg"] = msg
	}
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
	case "assignee":
		return "담당자를 선택하세요"
	case "work_date":
		return "작업일을 입력하세요"
	case "complete_dt":
		return "완료 일시를 입력하세요"
	case "partial_note":
		return "일부완료 사유를 입력하세요"
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
	closed := isProjectWorkClosed(t)
	unlocked := false
	if h.unlockRepo != nil {
		unlocked, _, _ = h.unlockRepo.HasActive(t.TaskID, currentUserID(c))
	}
	if closed && !unlocked {
		return h.redirectPWAction(c, t.TaskID, "locked")
	}
	if err := h.applyProjectWorkActionForm(c, t, model.WorkResultDone); err != nil {
		if _, ok := err.(pwRendered); ok {
			return nil
		}
		if code, ok := err.(*pwActionErr); ok {
			return h.redirectPWAction(c, t.TaskID, code.code)
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
	place := strings.TrimSpace(c.FormValue("work_place"))
	if place != "office" && place != "field" {
		return &pwActionErr{"work_place"}
	}
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	if assignee == "" {
		return &pwActionErr{"assignee"}
	}
	if err := checkAssigneeInOrg(h.userRepo, t.OrgID, assignee, c.FormValue("assignee_user_id"), t.Assignee); err != nil {
		return &pwActionErr{"assignee"}
	}
	workDay := strings.TrimSpace(c.FormValue("work_date"))
	if workDay == "" {
		return &pwActionErr{"work_date"}
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
	completeDT := strings.TrimSpace(c.FormValue("complete_datetime"))
	confirmer := strings.TrimSpace(c.FormValue("customer_confirmer"))
	if custom := strings.TrimSpace(c.FormValue("customer_confirmer_custom")); custom != "" {
		confirmer = custom
	}

	t.WorkPlace = place
	t.Assignee = assignee
	if uid := strings.TrimSpace(c.FormValue("assignee_user_id")); uid != "" {
		t.AssigneeUserID = uid
	}
	t.WorkDate = workDay
	t.ActionTaken = content
	t.CustomerConfirmer = confirmer
	t.ResultCode = result

	switch result {
	case model.WorkResultDone:
		if completeDT == "" {
			return &pwActionErr{"complete_dt"}
		}
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
	default:
		return &pwActionErr{"content"}
	}
	if line := repository.FirstLine(content, 200); line != "" {
		t.CompleteNote = line
	}
	if err := h.repo.SaveTaskActionFields(t); err != nil {
		return err
	}
	pid, err := h.repo.InsertWorkTaskProcess(model.WorkTaskProcess{
		TaskID: t.TaskID, OrgID: t.OrgID, Worker: ctxString(c, "user_name"),
		WorkerUserID: currentUserID(c), WorkPlace: place, ResultCode: result,
		WorkContent: content, Note: note, NextDate: nextDate,
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
	if result == model.WorkResultPartial && c.FormValue("create_followup") == "1" {
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
		nextDate = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
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
		return h.redirectPWAction(c, t.TaskID, "locked")
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
			return h.redirectPWAction(c, t.TaskID, "transfer_to")
		}
		if note == "" {
			return h.redirectPWAction(c, t.TaskID, "transfer_reason")
		}
		t.TransferTo = to
		t.Status = model.WBTaskTransfer
	}
	if result == model.WorkResultHold {
		next := strings.TrimSpace(c.FormValue("hold_next_action"))
		if next != "action" && next != "transfer" && next != "cancel" {
			return h.redirectPWAction(c, t.TaskID, "hold_next")
		}
		if note == "" {
			return h.redirectPWAction(c, t.TaskID, "hold_reason")
		}
		t.HoldReason = note
		t.HoldNextAction = next
		t.Status = model.WBTaskHold
	}
	if result == model.WorkResultCancel {
		note = strings.TrimSpace(c.FormValue("cancel_reason"))
		day := strings.TrimSpace(c.FormValue("cancel_date"))
		if day == "" {
			return h.redirectPWAction(c, t.TaskID, "cancel_date")
		}
		if note == "" {
			return h.redirectPWAction(c, t.TaskID, "cancel_reason")
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

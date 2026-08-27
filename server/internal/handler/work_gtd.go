package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func applyAdminGTDForm(t *model.WorkTask, c echo.Context) {
	t.HoldReason = strings.TrimSpace(c.FormValue("hold_reason"))
	t.ReviewDate = strings.TrimSpace(c.FormValue("review_date"))
	t.CancelReason = strings.TrimSpace(c.FormValue("cancel_reason"))
	t.WaitPartyKind = strings.TrimSpace(c.FormValue("wait_party_kind"))
	t.WaitParty = strings.TrimSpace(c.FormValue("wait_party"))
	t.WaitRequest = strings.TrimSpace(c.FormValue("wait_request"))
	t.ReplyDueDate = strings.TrimSpace(c.FormValue("reply_due_date"))
	t.NextCheckDate = strings.TrimSpace(c.FormValue("next_check_date"))
	t.CompleteNote = strings.TrimSpace(c.FormValue("complete_note"))
	if t.CompleteNote == "" {
		t.CompleteNote = strings.TrimSpace(c.FormValue("activity_content"))
	}
}

func copyAdminGTDFields(dst, src *model.WorkTask) {
	if dst == nil || src == nil {
		return
	}
	dst.HoldReason = src.HoldReason
	dst.ReviewDate = src.ReviewDate
	dst.CancelReason = src.CancelReason
	dst.WaitPartyKind = src.WaitPartyKind
	dst.WaitParty = src.WaitParty
	dst.WaitRequest = src.WaitRequest
	dst.ReplyDueDate = src.ReplyDueDate
	dst.NextCheckDate = src.NextCheckDate
	dst.CompleteNote = src.CompleteNote
}

func (h *WorkboardHandler) adminGTDErr(taskID, status string, t *model.WorkTask, c echo.Context) string {
	openReq, unconf, err := h.repo.AdminCompleteBlockers(taskID)
	if err != nil {
		return ""
	}
	force := strings.TrimSpace(c.FormValue("force_complete")) == "1"
	return model.AdminGTDErr(status, t.HoldReason, t.ReviewDate, t.CancelReason,
		t.WaitParty, t.WaitRequest, t.ReplyDueDate, t.NextCheckDate, t.CompleteNote,
		openReq, unconf, isAdminRole(c), force, strings.TrimSpace(c.FormValue("force_reason")))
}

func (h *WorkboardHandler) addCompleteActivity(taskID, note, actor string) {
	note = strings.TrimSpace(note)
	if note == "" {
		return
	}
	_ = h.repo.CreateActivity(&model.WorkActivity{
		TaskID:       taskID,
		ActivityType: model.WBActivityDone,
		Content:      note,
		Actor:        actor,
		SpentMinutes: model.WBActivityDefaultSpent,
	})
}

func persistWaitingAction(repo *repository.WBRepo, t *model.WorkTask) {
	if repo == nil || t == nil || t.Status != model.WBTaskWaitingFor {
		return
	}
	actions, _ := repo.ListActions(t.TaskID)
	for _, a := range actions {
		if a.Status == model.WBActionWaiting && !a.Confirmed {
			return
		}
	}
	title := t.WaitRequest
	if title == "" {
		title = "회신 대기"
	}
	_ = repo.CreateAction(&model.WorkAction{
		TaskID:        t.TaskID,
		Title:         title,
		Status:        model.WBActionWaiting,
		Required:      true,
		WaitPartyKind: t.WaitPartyKind,
		WaitParty:     t.WaitParty,
		WaitRequest:   t.WaitRequest,
		ReplyDueDate:  t.ReplyDueDate,
		NextCheckDate: t.NextCheckDate,
	})
}

func (h *WorkboardHandler) ensureWaitingAction(t *model.WorkTask) {
	persistWaitingAction(h.repo, t)
}

func (h *AdminWorkHandler) CreateInbox(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	title := strings.TrimSpace(c.FormValue("title"))
	if title == "" {
		return c.Redirect(http.StatusSeeOther, "/admin-work?err=inbox")
	}
	t := &model.WorkTask{
		WorkType:    model.WBWorkAdmin,
		Title:       title,
		Description: strings.TrimSpace(c.FormValue("description")),
		Status:      model.WBTaskInbox,
		Priority:    model.WBPriorityNormal,
		DurationMin: 30,
	}
	applyCustomerForm(t, c)
	if err := h.repo.CreateTask(t); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/admin-work?ok=inbox")
}

// Classify 수집함 건을 할 일 등으로 분류한다(§13.5·§13.11). 삭제하지 않는다.
func (h *AdminWorkHandler) Classify(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	existing, err := h.repo.GetTask(id)
	if err != nil || existing == nil || !model.IsAdminGTDTask(*existing) {
		return echo.ErrNotFound
	}
	if existing.Status != model.WBTaskInbox {
		return c.Redirect(http.StatusSeeOther, "/admin-work?err=classify")
	}
	status := strings.TrimSpace(c.FormValue("status"))
	if status == "" {
		status = model.WBTaskWaiting
	}
	dueDate := strings.TrimSpace(c.FormValue("due_date"))
	if dueDate != "" {
		parsed, err := model.ParseAppDate(dueDate)
		if err != nil {
			return c.Redirect(http.StatusSeeOther, "/admin-work?err=classify")
		}
		dueDate = parsed
	}
	if dueDate == "" {
		dueDate = time.Now().Format("2006-01-02")
	}
	t := *existing
	t.Status = status
	t.DueDate = dueDate
	if strings.TrimSpace(c.FormValue("assignee")) != "" {
		t.Assignee = strings.TrimSpace(c.FormValue("assignee"))
	}
	applyAdminGTDForm(&t, c)
	if code := model.AdminGTDErr(t.Status, t.HoldReason, t.ReviewDate, t.CancelReason,
		t.WaitParty, t.WaitRequest, t.ReplyDueDate, t.NextCheckDate, t.CompleteNote,
		0, 0, isAdminRole(c), strings.TrimSpace(c.FormValue("force_complete")) == "1",
		strings.TrimSpace(c.FormValue("force_reason"))); code != "" {
		return c.Redirect(http.StatusSeeOther, "/admin-work?err="+code)
	}
	if err := h.repo.UpdateTask(&t); err != nil {
		return err
	}
	persistWaitingAction(h.repo, &t)
	if t.Status == model.WBTaskComplete && t.CompleteNote != "" {
		_ = h.repo.CreateActivity(&model.WorkActivity{
			TaskID: t.TaskID, ActivityType: model.WBActivityDone,
			Content: t.CompleteNote, Actor: ctxString(c, "user_name"),
			SpentMinutes: model.WBActivityDefaultSpent,
		})
	}
	return c.Redirect(http.StatusSeeOther, "/admin-work?ok=classify")
}

func (h *WorkboardHandler) CreateAction(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil || !model.IsAdminGTDTask(*t) {
		return echo.ErrNotFound
	}
	back := actionBack(c, id)
	title := strings.TrimSpace(c.FormValue("action_title"))
	if title == "" {
		return c.Redirect(http.StatusSeeOther, back+"err=action")
	}
	st := strings.TrimSpace(c.FormValue("action_status"))
	if st == "" {
		st = model.WBActionTodo
	}
	a := &model.WorkAction{
		TaskID:        id,
		Title:         title,
		Status:        st,
		Required:      c.FormValue("action_required") != "0",
		ScheduledDate: strings.TrimSpace(c.FormValue("action_scheduled_date")),
		DueDate:       strings.TrimSpace(c.FormValue("action_due_date")),
		Assignee:      strings.TrimSpace(c.FormValue("action_assignee")),
		WaitPartyKind: strings.TrimSpace(c.FormValue("wait_party_kind")),
		WaitParty:     strings.TrimSpace(c.FormValue("wait_party")),
		WaitRequest:   strings.TrimSpace(c.FormValue("wait_request")),
		ReplyDueDate:  strings.TrimSpace(c.FormValue("reply_due_date")),
		NextCheckDate: strings.TrimSpace(c.FormValue("next_check_date")),
	}
	if err := waitingActionFieldsErr(a); err != "" {
		return c.Redirect(http.StatusSeeOther, back+"err="+err)
	}
	if err := h.repo.CreateAction(a); err != nil {
		return err
	}
	_ = h.repo.SyncAdminActionProgress(id)
	return c.Redirect(http.StatusSeeOther, back+"ok=action")
}

func (h *WorkboardHandler) UpdateAction(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	aid := strings.TrimSpace(c.Param("aid"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil || !model.IsAdminGTDTask(*t) {
		return echo.ErrNotFound
	}
	a, err := h.repo.GetAction(aid)
	if err != nil || a == nil || a.TaskID != id {
		return echo.ErrNotFound
	}
	back := actionBack(c, id)
	if title := strings.TrimSpace(c.FormValue("action_title")); title != "" {
		a.Title = title
	}
	if st := strings.TrimSpace(c.FormValue("action_status")); st != "" {
		a.Status = st
	}
	if v := c.FormValue("action_required"); v != "" {
		a.Required = v != "0"
	}
	if v := strings.TrimSpace(c.FormValue("action_scheduled_date")); v != "" || c.FormValue("clear_scheduled") == "1" {
		a.ScheduledDate = v
	}
	if v := c.FormValue("confirmed"); v != "" {
		a.Confirmed = v == "1"
	}
	if v := strings.TrimSpace(c.FormValue("wait_party_kind")); v != "" {
		a.WaitPartyKind = v
	}
	if v := strings.TrimSpace(c.FormValue("wait_party")); v != "" {
		a.WaitParty = v
	}
	if v := strings.TrimSpace(c.FormValue("wait_request")); v != "" {
		a.WaitRequest = v
	}
	if v := strings.TrimSpace(c.FormValue("reply_due_date")); v != "" {
		a.ReplyDueDate = v
	}
	if v := strings.TrimSpace(c.FormValue("next_check_date")); v != "" {
		a.NextCheckDate = v
	}
	if err := waitingActionFieldsErr(a); err != "" {
		return c.Redirect(http.StatusSeeOther, back+"err="+err)
	}
	if a.Status == model.WBActionComplete || a.Status == model.WBActionCancelled {
		a.Confirmed = true
	}
	if err := h.repo.UpdateAction(a); err != nil {
		return err
	}
	_ = h.repo.SyncAdminActionProgress(id)
	_ = h.repo.ResumeInProgressIfReady(id)
	return c.Redirect(http.StatusSeeOther, back+"ok=action")
}

func (h *WorkboardHandler) CreateActivity(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil || !model.IsAdminGTDTask(*t) {
		return echo.ErrNotFound
	}
	back := actionBack(c, id)
	content := strings.TrimSpace(c.FormValue("activity_content"))
	if content == "" {
		return c.Redirect(http.StatusSeeOther, back+"err=activity")
	}
	typ := strings.TrimSpace(c.FormValue("activity_type"))
	if typ == "" {
		typ = model.WBActivityOther
	}
	spent := 0
	if p := strings.TrimSpace(c.FormValue("spent_minutes")); p != "" {
		fmtScanInt(p, &spent)
	}
	if spent <= 0 {
		return c.Redirect(http.StatusSeeOther, back+"err=spent")
	}
	act := &model.WorkActivity{
		TaskID:       id,
		ActionID:     strings.TrimSpace(c.FormValue("action_id")),
		ActivityType: typ,
		Content:      content,
		Actor:        ctxString(c, "user_name"),
		SpentMinutes: spent,
	}
	if err := h.repo.CreateActivity(act); err != nil {
		return err
	}
	if h.attach != nil {
		if fh, err := c.FormFile("file"); err == nil && fh != nil {
			_ = h.attach.saveGenericFile(model.RefTypeWorkActivity, act.ActivityID, "", fh, 0)
		}
	}
	return c.Redirect(http.StatusSeeOther, back+"ok=activity")
}

func waitingActionFieldsErr(a *model.WorkAction) string {
	if a == nil || a.Status != model.WBActionWaiting {
		return ""
	}
	if a.WaitPartyKind == "" || a.WaitParty == "" || a.WaitRequest == "" || (a.ReplyDueDate == "" && a.NextCheckDate == "") {
		return "waiting_for"
	}
	return ""
}

func actionBack(c echo.Context, taskID string) string {
	back := strings.TrimSpace(c.FormValue("back"))
	loc := "/workboard/tasks/" + url.PathEscape(taskID) + "?"
	if back != "" {
		loc += "back=" + url.QueryEscape(back) + "&"
	}
	return loc
}

func gtdFlash(err string) string {
	return gtdFlashMsg(err, "")
}

func gtdFlashMsg(err, n string) string {
	switch err {
	case "hold_required":
		return "보류는 사유와 재검토일이 필요합니다."
	case "waiting_for":
		return "회신 대기는 대상 구분·대상·요청 내용·회신 예정일 또는 다음 확인일이 필요합니다."
	case "complete_note":
		return "완료 시 최종 조치 내용을 입력하세요."
	case "complete_block":
		return "미완료 필수 다음 행동 또는 미확인 회신 대기가 있어 완료할 수 없습니다."
	case "force_reason":
		return "관리자 강제 완료 사유를 입력하세요."
	case "cancel_reason":
		return "취소 사유를 입력하세요."
	case "inbox":
		return "수집함은 제목이 필요합니다."
	case "classify":
		return "수집함 건만 분류할 수 있습니다."
	case "action":
		return "다음 행동 제목을 입력하세요."
	case "activity":
		return "조치 내용을 입력하세요."
	case "spent":
		return "소요 시간(분)은 0보다 커야 합니다. 기본 30분입니다."
	case "rec_exists":
		return "이미 실행 작업이 있습니다. 재생성 버튼을 쓰세요."
	case "rec_year":
		return "매일 × 1년은 만들 수 없습니다. 기간이나 주기를 줄이세요."
	case "rec_limit":
		return "실행 예정일이 500건을 넘으면 만들 수 없습니다."
	case "rec_warn":
		return "200건을 넘습니다. 확인란을 선택한 뒤 다시 생성하세요."
	case "rec_rule":
		return "반복 규칙·기간을 확인하세요."
	case "rec_source":
		return "AS·점검 원본 업무에는 실행 작업을 만들 수 없습니다."
	case "rec_parent":
		return "상위 업무에서만 실행 작업을 만들 수 있습니다."
	case "rec_open":
		if strings.TrimSpace(n) == "" {
			n = "0"
		}
		return fmt.Sprintf("완료되지 않은 실행 작업이 %s건 있습니다. 전체 업무를 완료 처리하시겠습니까?", n)
	case "rec_result":
		return "최종 결과를 입력해야 완료됩니다."
	case "rec_skip_reason":
		return "제외 사유를 입력하세요."
	case "rec_defer_date":
		return "다음 조치일을 입력하세요."
	case "rec_replace":
		return "미완료 일정을 모두 지우고 다시 만듭니다. 확인란을 선택한 뒤 다시 실행하세요."
	case "rec_has_complete":
		return "완료된 실행 작업이 있어 삭제할 수 없습니다. 보관으로 내리세요."
	default:
		return ""
	}
}

func (h *WorkboardHandler) renderTaskGTD(c echo.Context, data map[string]interface{}, t *model.WorkTask) {
	if t == nil || !model.IsAdminGTDTask(*t) {
		data["IsAdminGTD"] = false
		return
	}
	actions, _ := h.repo.ListActions(t.TaskID)
	acts, _ := h.repo.ListActivities(t.TaskID)
	openReq, unconf, _ := h.repo.AdminCompleteBlockers(t.TaskID)
	doneReq, reqTotal, _ := h.repo.CountActionProgress(t.TaskID)
	pct, pctOK := model.ActionProgressPct(doneReq, reqTotal)
	today := time.Now().Format("2006-01-02")
	lead, leadOK := model.AdminLeadTimeDays(t.ReceiptDate, t.CompleteDate)
	files := map[string][]model.Attachment{}
	titles := map[string]string{}
	for _, a := range actions {
		titles[a.ActionID] = a.Title
	}
	if h.attach != nil && h.attach.repo != nil {
		for _, a := range acts {
			if atts, err := h.attach.repo.ListByRef(model.RefTypeWorkActivity, a.ActivityID); err == nil && len(atts) > 0 {
				files[a.ActivityID] = atts
			}
		}
	}
	data["IsAdminGTD"] = true
	data["Actions"] = actions
	data["ActionTitles"] = titles
	data["Activities"] = acts
	data["ActivityFiles"] = files
	data["OpenRequired"] = openReq
	data["UnconfirmedWait"] = unconf
	data["CompleteBlocked"] = openReq > 0 || unconf > 0
	data["MissingNextAction"] = t.Status == model.WBTaskInProgress && len(actions) == 0
	data["ProgressDone"] = doneReq
	data["ProgressRequired"] = reqTotal
	data["ProgressPct"] = int(pct + 0.5)
	data["ProgressShow"] = pctOK
	data["LeadDays"] = lead
	data["LeadShow"] = leadOK
	data["Today"] = today
	data["IsAdminUser"] = isAdminRole(c)
}

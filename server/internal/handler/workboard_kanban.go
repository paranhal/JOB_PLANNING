package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type RegisterKanbanColumn = model.KanbanColumn
type RegisterKanbanCard = model.KanbanCard

var registerPlanKanbanOrder = []string{
	model.WorkBucketUnplanned,
	model.WorkBucketToday,
	model.WorkBucketInProgress,
	model.WorkBucketDelayed,
	model.WorkBucketCompletedToday,
}

func (h *WorkboardHandler) loadRegisterKanban(date, assigneeFilter string) ([]RegisterKanbanColumn, int, error) {
	today := time.Now().Format(dateLayout)
	empty := emptyRegisterKanban(date, today)
	if h.workBoard == nil {
		return empty, 0, nil
	}
	var mineKeys []string
	if strings.TrimSpace(assigneeFilter) != "" {
		mineKeys = []string{assigneeFilter}
	}

	completed, err := h.workBoard.ListBucketOn(model.WorkBucketCompletedToday, date, "", mineKeys, 0)
	if err != nil {
		return nil, 0, err
	}
	progress, err := h.workBoard.ListBucketOn(model.WorkBucketInProgress, date, "", mineKeys, 0)
	if err != nil {
		return nil, 0, err
	}
	delayed, err := h.workBoard.ListBucketOn(model.WorkBucketDelayed, date, "", mineKeys, 0)
	if err != nil {
		return nil, 0, err
	}
	todayItems, err := h.workBoard.ListBucketOn(model.WorkBucketToday, date, "", mineKeys, 0)
	if err != nil {
		return nil, 0, err
	}
	unplanned, _, err := h.workBoard.ListUnplanned("", mineKeys, "")
	if err != nil {
		return nil, 0, err
	}

	seen := map[string]bool{}
	var all []model.WorkListItem
	all = appendUniqueKanban(all, completed, seen)
	for _, u := range unplanned {
		if !u.NeedPlanKind() {
			continue
		}
		it := u.WorkListItem
		it.NeedPlan = true
		all = appendUniqueKanban(all, []model.WorkListItem{it}, seen)
	}
	all = appendUniqueKanban(all, progress, seen)
	all = appendUniqueKanban(all, delayed, seen)
	all = appendUniqueKanban(all, todayItems, seen)

	by := map[string][]model.KanbanCard{}
	for _, key := range registerPlanKanbanOrder {
		by[key] = nil
	}
	total := 0
	for i := range all {
		it := all[i]
		if !it.ApplyPlanKanbanMapping(date) {
			continue
		}
		if assigneeFilter != "" && strings.TrimSpace(it.Assignee) != assigneeFilter {
			continue
		}
		by[it.Bucket] = append(by[it.Bucket], h.toRegisterKanbanCard(it))
		total++
	}

	cols := make([]model.KanbanColumn, 0, len(registerPlanKanbanOrder))
	for _, key := range registerPlanKanbanOrder {
		items := by[key]
		if items == nil {
			items = []model.KanbanCard{}
		}
		cols = append(cols, model.KanbanColumn{
			Key:    key,
			Title:  model.PlanKanbanColumnTitle(key, date, today),
			Count:  len(items),
			Items:  items,
			Border: "border-slate-200",
		})
	}
	return cols, total, nil
}

func emptyRegisterKanban(date, today string) []RegisterKanbanColumn {
	cols := make([]RegisterKanbanColumn, 0, len(registerPlanKanbanOrder))
	for _, key := range registerPlanKanbanOrder {
		cols = append(cols, model.KanbanColumn{
			Key:    key,
			Title:  model.PlanKanbanColumnTitle(key, date, today),
			Items:  []model.KanbanCard{},
			Border: "border-slate-200",
		})
	}
	return cols
}

func appendUniqueKanban(dst []model.WorkListItem, src []model.WorkListItem, seen map[string]bool) []model.WorkListItem {
	for _, it := range src {
		k := model.WorkListItemKey(it)
		if k == ":" || seen[k] {
			continue
		}
		seen[k] = true
		dst = append(dst, it)
	}
	return dst
}

func (h *WorkboardHandler) toRegisterKanbanCard(it model.WorkListItem) model.KanbanCard {
	card := model.KanbanCardFromWork(it)
	card.Kind = planKanbanKind(it)
	card.ItemKey = planKanbanItemKey(it)
	card.Urgent = model.WorkItemUrgent(it.Urgency)
	card.Grouped = strings.TrimSpace(it.ReceiptGroupID) != ""
	card.ActionHref, card.EditHref = planKanbanHrefs(it)
	if t := h.planKanbanWorkTask(it); t != nil {
		wb := taskCardFromWork(*t)
		if h != nil {
			wb = h.taskCard(*t)
		}
		if href := wb.DetailHref(); href != "" {
			card.ActionHref = href
		}
		if href := wb.EditHref(); href != "" {
			card.EditHref = href
		}
		if card.LeftStyle == "" || strings.TrimSpace(it.ProductType) == "" {
			if pt := wb.ProductType; pt != "" {
				card.LeftStyle = model.WorkCardColorStyle(it.Assignee, pt, it.Prefix)
			}
		}
	}
	return card
}

func planKanbanKind(it model.WorkListItem) string {
	switch {
	case strings.HasPrefix(it.Href, "/as/work/"):
		return "wi"
	case it.Prefix == model.WorkPrefixAS || strings.HasPrefix(it.Href, "/as/"):
		return model.WBSourceAS
	case it.Prefix == model.WorkPrefixMaintenance:
		return model.WBSourceMaintenance
	case strings.HasPrefix(it.Href, "/workboard/tasks/"):
		return "task"
	default:
		return "task"
	}
}

func planKanbanItemKey(it model.WorkListItem) string {
	switch {
	case strings.Contains(it.Href, "/as/work/"):
		return "wi:" + it.RefID
	case strings.HasPrefix(it.Href, "/as/"):
		return "as:" + it.RefID
	case strings.HasPrefix(it.Href, "/workboard/tasks/"):
		return "task:" + it.RefID
	case it.Prefix == model.WorkPrefixMaintenance:
		return "mnt:" + it.RefID
	default:
		return "gen:" + it.RefID
	}
}

func planKanbanHrefs(it model.WorkListItem) (action, edit string) {
	switch {
	case strings.HasPrefix(it.Href, "/as/") && !strings.Contains(it.Href, "/work/"):
		return "/as/" + it.RefID + "/action", ""
	case it.Prefix == model.WorkPrefixMaintenance:
		return "/maintenance/visits/" + it.RefID + "/action", ""
	case strings.HasPrefix(it.Href, "/workboard/tasks/"):
		return it.Href, it.Href + "/edit"
	default:
		return it.Href, ""
	}
}

func (h *WorkboardHandler) planKanbanWorkTask(it model.WorkListItem) *model.WorkTask {
	if h == nil || h.repo == nil {
		return nil
	}
	if strings.HasPrefix(it.Href, "/workboard/tasks/") {
		t, _ := h.repo.GetTask(it.RefID)
		return t
	}
	switch it.Prefix {
	case model.WorkPrefixAS:
		t, _ := h.repo.GetTaskBySource(model.WBSourceAS, it.RefID)
		return t
	case model.WorkPrefixMaintenance:
		t, _ := h.repo.GetTaskBySource(model.WBSourceMaintenance, it.RefID)
		return t
	}
	return nil
}

// RegisterKanbanMove 일일 업무 등록 칸반 드래그. §7.8.5
func (h *WorkboardHandler) RegisterKanbanMove(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	date := model.NormalizeAppDate(c.FormValue("date"))
	if date == "" {
		date = time.Now().Format(dateLayout)
	}
	if !h.canEditRegisterDate(c, date) {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=past"))
	}
	from := strings.TrimSpace(c.FormValue("from"))
	to := strings.TrimSpace(c.FormValue("to"))
	key := strings.TrimSpace(c.FormValue("item_key"))
	if key == "" || to == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=place"))
	}
	if to == model.WorkBucketUnplanned {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=kanban_unplanned"))
	}
	if from == to {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, ""))
	}
	if from == model.WorkBucketCompletedToday && strings.TrimSpace(c.FormValue("confirm_reopen")) != "1" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=kanban_reopen"))
	}

	assignee := strings.TrimSpace(c.FormValue("assignee"))
	note := strings.TrimSpace(c.FormValue("complete_note"))
	if err := h.applyRegisterKanbanMove(key, from, to, date, assignee, note); err != nil {
		if km, ok := err.(*kanbanMoveError); ok {
			code := km.code
			if code == "complete_note" {
				code = "kanban_complete"
			}
			if code == "assignee" {
				code = "kanban_assignee"
			}
			if code == "place" {
				code = "place"
			}
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+code))
		}
		return err
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=kanban"))
}

func (h *WorkboardHandler) applyRegisterKanbanMove(key, from, to, date, assignee, note string) error {
	needAssignee := (to == model.WorkBucketToday || to == model.WorkBucketInProgress) &&
		(from == model.WorkBucketUnplanned)
	if needAssignee {
		cur := h.kanbanCurrentAssignee(key)
		if strings.TrimSpace(cur) == "" && assignee == "" {
			return errKanban("assignee")
		}
		if assignee == "" {
			assignee = cur
		}
	}

	switch {
	case to == model.WorkBucketCompletedToday:
		if note == "" {
			return errKanban("complete_note")
		}
		return h.kanbanComplete(key, date, note)
	case from == model.WorkBucketCompletedToday:
		if err := h.kanbanReopen(key, to, date, assignee); err != nil {
			return err
		}
		if to == model.WorkBucketToday || to == model.WorkBucketInProgress {
			return h.kanbanApplyOpenMove(key, model.WorkBucketToday, to, date, assignee)
		}
		return nil
	default:
		return h.kanbanApplyOpenMove(key, from, to, date, assignee)
	}
}

func errKanban(code string) error {
	return &kanbanMoveError{code: code}
}

type kanbanMoveError struct{ code string }

func (e *kanbanMoveError) Error() string { return e.code }

func (h *WorkboardHandler) kanbanApplyOpenMove(key, from, to, date, assignee string) error {
	switch {
	case from == model.WorkBucketUnplanned && to == model.WorkBucketToday:
		return h.kanbanAssignDate(key, date, assignee, false)
	case from == model.WorkBucketUnplanned && to == model.WorkBucketInProgress:
		return h.kanbanAssignDate(key, date, assignee, true)
	case from == model.WorkBucketDelayed && to == model.WorkBucketToday:
		return h.kanbanAssignDate(key, date, assignee, false)
	case from == model.WorkBucketToday && to == model.WorkBucketInProgress:
		return h.kanbanSetProgress(key, true)
	case from == model.WorkBucketDelayed && to == model.WorkBucketInProgress:
		return h.kanbanSetProgress(key, true)
	case from == model.WorkBucketInProgress && to == model.WorkBucketToday:
		return h.kanbanSetProgress(key, false)
	default:
		// 같은 열 계열의 날짜·상태만 맞춘다
		if to == model.WorkBucketToday {
			return h.kanbanAssignDate(key, date, assignee, false)
		}
		if to == model.WorkBucketInProgress {
			if err := h.kanbanAssignDate(key, date, assignee, false); err != nil {
				return err
			}
			return h.kanbanSetProgress(key, true)
		}
		return errKanban("place")
	}
}

func (h *WorkboardHandler) kanbanAssignDate(key, date, assignee string, start bool) error {
	if h.workBoard != nil {
		_ = h.workBoard.AssignUnplannedDate(key, date, assignee, h.assigneeUserID(assignee))
	}
	if err := h.kanbanForceDate(key, date, assignee); err != nil {
		return err
	}
	if start {
		return h.kanbanSetProgress(key, true)
	}
	return nil
}

func (h *WorkboardHandler) kanbanForceDate(key, date, assignee string) error {
	kind, id := splitKanbanKey(key)
	switch kind {
	case "as":
		if h.asRepo == nil {
			return errKanban("place")
		}
		if assignee != "" {
			if as, err := h.asRepo.GetByID(id); err == nil && as != nil && strings.TrimSpace(as.AssignedTo) == "" {
				as.AssignedTo = assignee
				as.AssignedUserID = h.assigneeUserID(assignee)
				_ = h.asRepo.Update(as)
			}
		}
		if err := h.asRepo.UpdateVisitScheduledDate(id, date, true); err != nil {
			return err
		}
		h.syncASPlannedFromKanban(id)
		return nil
	case "mnt":
		if h.mntRepo != nil {
			if err := h.mntRepo.SetVisitDate(id, date); err != nil {
				return err
			}
		}
		h.syncVisitDateFromTask(model.WBSourceMaintenance, id, date)
		if t, _ := h.repo.GetTaskBySource(model.WBSourceMaintenance, id); t != nil {
			t.DueDate = date
			t.WorkDate = date
			if assignee != "" && strings.TrimSpace(t.Assignee) == "" {
				t.Assignee = assignee
			}
			return h.updateTaskLogged(t)
		}
		return nil
	case "task":
		t, err := h.repo.GetTask(id)
		if err != nil || t == nil {
			return echo.ErrNotFound
		}
		t.DueDate = date
		t.WorkDate = date
		if assignee != "" && strings.TrimSpace(t.Assignee) == "" {
			t.Assignee = assignee
		}
		return h.updateTaskLogged(t)
	case "wi":
		if h.workBoard != nil {
			return h.workBoard.AssignUnplannedDate(key, date, assignee, h.assigneeUserID(assignee))
		}
		return nil
	default:
		return nil
	}
}

func (h *WorkboardHandler) syncASPlannedFromKanban(asID string) {
	if h.repo == nil || h.asRepo == nil || strings.TrimSpace(asID) == "" {
		return
	}
	as, err := h.asRepo.GetByID(asID)
	if err != nil || as == nil {
		return
	}
	_ = h.repo.SyncASDailyTask(as)
}

func (h *WorkboardHandler) kanbanSetProgress(key string, start bool) error {
	kind, id := splitKanbanKey(key)
	status := model.WBTaskWaiting
	if start {
		status = model.WBTaskInProgress
	}
	switch kind {
	case "as":
		if h.asRepo == nil {
			return errKanban("place")
		}
		as, err := h.asRepo.GetByID(id)
		if err != nil || as == nil {
			return echo.ErrNotFound
		}
		if start {
			as.Status = model.WBTaskInProgress
			if as.StartDatetime == nil || as.StartDatetime.IsZero() {
				now := time.Now()
				as.StartDatetime = &now
			}
		} else if as.Status == model.WBTaskInProgress {
			as.Status = "assigned"
		}
		return h.asRepo.Update(as)
	case "task":
		t, err := h.repo.GetTask(id)
		if err != nil || t == nil {
			return echo.ErrNotFound
		}
		t.Status = status
		return h.updateTaskLogged(t)
	case "mnt":
		if t, _ := h.repo.GetTaskBySource(model.WBSourceMaintenance, id); t != nil {
			t.Status = status
			return h.updateTaskLogged(t)
		}
		return nil
	default:
		if t, _ := h.repo.GetTask(id); t != nil {
			t.Status = status
			return h.updateTaskLogged(t)
		}
		return nil
	}
}

func (h *WorkboardHandler) kanbanComplete(key, date, note string) error {
	kind, id := splitKanbanKey(key)
	switch kind {
	case "as":
		if h.asRepo == nil {
			return errKanban("place")
		}
		as, err := h.asRepo.GetByID(id)
		if err != nil || as == nil {
			return echo.ErrNotFound
		}
		as.Status = "completed"
		as.ActionTaken = note
		now := time.Now()
		as.CompleteDatetime = &now
		return h.asRepo.Update(as)
	case "mnt":
		if h.mntRepo == nil {
			return errKanban("place")
		}
		if err := h.mntRepo.SetVisitCompleted(id, true, date); err != nil {
			return err
		}
		if t, _ := h.repo.GetTaskBySource(model.WBSourceMaintenance, id); t != nil {
			t.Status = model.WBTaskComplete
			t.CompleteNote = note
			t.CompleteDate = date
			t.Progress = 100
			return h.updateTaskLogged(t)
		}
		return nil
	case "task":
		t, err := h.repo.GetTask(id)
		if err != nil || t == nil {
			return echo.ErrNotFound
		}
		t.Status = model.WBTaskComplete
		t.CompleteNote = note
		t.CompleteDate = date
		t.Progress = 100
		if model.IsAdminGTDTask(*t) {
			openReq, unconf, _ := h.repo.AdminCompleteBlockers(t.TaskID)
			if code := model.AdminGTDErr(t.Status, t.HoldReason, t.ReviewDate, t.CancelReason,
				t.WaitParty, t.WaitRequest, t.ReplyDueDate, t.NextCheckDate, t.CompleteNote,
				openReq, unconf, false, false, ""); code != "" {
				return errKanban("complete_note")
			}
		}
		return h.updateTaskLogged(t)
	default:
		return errKanban("place")
	}
}

func (h *WorkboardHandler) kanbanReopen(key, to, date, assignee string) error {
	kind, id := splitKanbanKey(key)
	switch kind {
	case "as":
		as, err := h.asRepo.GetByID(id)
		if err != nil || as == nil {
			return echo.ErrNotFound
		}
		as.Status = model.WBTaskInProgress
		if to == model.WorkBucketToday {
			as.Status = "assigned"
		}
		as.CompleteDatetime = nil
		if date != "" {
			as.VisitScheduledDate = date
			as.ScheduleConfirmed = true
		}
		if assignee != "" {
			as.AssignedTo = assignee
		}
		return h.asRepo.Update(as)
	case "mnt":
		if h.mntRepo != nil {
			_ = h.mntRepo.SetVisitCompleted(id, false, "")
			if date != "" {
				_ = h.mntRepo.SetVisitDate(id, date)
			}
		}
		if t, _ := h.repo.GetTaskBySource(model.WBSourceMaintenance, id); t != nil {
			t.Status = model.WBTaskWaiting
			if to == model.WorkBucketInProgress {
				t.Status = model.WBTaskInProgress
			}
			t.CompleteNote = ""
			t.CompleteDate = ""
			t.Progress = 0
			if date != "" {
				t.DueDate = date
				if strings.TrimSpace(t.StartTime) == "" {
					t.WorkDate = date
				}
			}
			return h.updateTaskLogged(t)
		}
		return nil
	case "task":
		t, err := h.repo.GetTask(id)
		if err != nil || t == nil {
			return echo.ErrNotFound
		}
		t.Status = model.WBTaskWaiting
		if to == model.WorkBucketInProgress {
			t.Status = model.WBTaskInProgress
		}
		t.CompleteNote = ""
		t.CompleteDate = ""
		t.Progress = 0
		if date != "" {
			t.DueDate = date
			if strings.TrimSpace(t.StartTime) == "" {
				t.WorkDate = date
			}
		}
		if assignee != "" {
			t.Assignee = assignee
		}
		return h.updateTaskLogged(t)
	default:
		return errKanban("place")
	}
}

func (h *WorkboardHandler) kanbanCurrentAssignee(key string) string {
	kind, id := splitKanbanKey(key)
	switch kind {
	case "as":
		if h.asRepo != nil {
			if as, err := h.asRepo.GetByID(id); err == nil && as != nil {
				return strings.TrimSpace(as.AssignedTo)
			}
		}
	case "mnt":
		if h.mntRepo != nil {
			if v, err := h.mntRepo.GetVisit(id); err == nil && v != nil {
				return strings.TrimSpace(v.Assignee)
			}
		}
	case "task":
		if t, _ := h.repo.GetTask(id); t != nil {
			return strings.TrimSpace(t.Assignee)
		}
	}
	return ""
}

func (h *WorkboardHandler) assigneeUserID(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || h.userRepo == nil {
		return ""
	}
	users, err := h.userRepo.ListAssignable()
	if err != nil {
		return ""
	}
	for _, u := range users {
		if u.FullName == name || u.Username == name {
			return u.UserID
		}
	}
	return ""
}

func (h *WorkboardHandler) updateTaskLogged(t *model.WorkTask) error {
	if t == nil {
		return nil
	}
	return repository.TouchWorkTaskUpdate(h.repo, t)
}

func splitKanbanKey(key string) (kind, id string) {
	key = strings.TrimSpace(key)
	if i := strings.Index(key, ":"); i > 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}

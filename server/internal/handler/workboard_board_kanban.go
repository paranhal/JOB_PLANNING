package handler

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/service"
)

const (
	kanbanColDelay    = service.BoardKanbanDelay
	kanbanColOff      = service.BoardKanbanOff
	kanbanColPlan     = service.BoardKanbanPlan
	kanbanColProgress = service.BoardKanbanProgress
	kanbanColPartial  = service.BoardKanbanPartial
	kanbanColDone     = service.BoardKanbanDone
)

func boardKanbanErrMsg(code string) string {
	switch strings.TrimSpace(code) {
	case service.KanbanErrForbidden:
		return "오늘의 업무에서 지연 업무·진행중(일정 외)으로 옮길 수 없습니다"
	case service.KanbanErrCarryConfirm:
		return "오늘로 이월하려면 확인이 필요합니다"
	case service.KanbanErrPartialItems:
		return "미완료 작업 항목이 있습니다. 부분 완료로 저장할까요?"
	case service.KanbanErrPartialEmpty:
		return "모든 항목이 끝났습니다. 완료로 옮겨 주세요"
	case service.KanbanErrRemainItems:
		return "남은 항목이 있습니다"
	case service.KanbanErrActionScreen:
		return "조치 화면에서 완료하세요"
	case "kanban_complete":
		return "완료 내용을 입력하세요"
	case "kanban_reopen":
		return "완료를 되돌리려면 확인이 필요합니다"
	case "kanban_assignee":
		return "담당자를 지정하세요"
	default:
		return ""
	}
}

func kanbanItemKey(t model.WorkTask) string {
	st, id := strings.TrimSpace(t.SourceType), strings.TrimSpace(t.SourceID)
	if st == model.WBSourceAS && id != "" {
		return "as:" + id
	}
	if st == model.WBSourceMaintenance && id != "" {
		return "mnt:" + id
	}
	if strings.TrimSpace(t.TaskID) != "" {
		return "task:" + t.TaskID
	}
	return ""
}

func hasTodaySchedule(t model.WorkTask, today string) bool {
	today = model.NormalizeAppDate(today)
	if today == "" {
		return false
	}
	if model.NormalizeAppDate(t.WorkDate) == today {
		return true
	}
	if model.NormalizeAppDate(t.DueDate) == today {
		return true
	}
	if model.NormalizeAppDate(t.CompleteDate) == today {
		return true
	}
	return false
}

func kanbanInProgressStatus(st string) bool {
	switch strings.TrimSpace(st) {
	case model.WBTaskInProgress, model.WBTaskHold, model.WBTaskTransfer, model.WBTaskReview, model.WBTaskWaitingFor:
		return true
	default:
		return false
	}
}

func classifyKanban(items []boardItem, today string) map[string][]boardItem {
	out := map[string][]boardItem{
		kanbanColDelay:    nil,
		kanbanColOff:      nil,
		kanbanColPlan:     nil,
		kanbanColProgress: nil,
		kanbanColPartial:  nil,
		kanbanColDone:     nil,
	}
	seen := map[string]bool{}
	today = model.NormalizeAppDate(today)
	for _, it := range items {
		st := strings.TrimSpace(it.Task.Status)
		if st == model.WBTaskCancelled {
			continue
		}
		key := it.DedupKey
		if key == "" {
			key = taskDedupKey(it.Task)
		}
		if seen[key] {
			continue
		}
		it.KanbanKey = kanbanItemKey(it.Task)
		it.TodaySchedule = hasTodaySchedule(it.Task, today)
		it.KanbanBadgeToday = it.TodaySchedule
		if boardItemDone(it) {
			if model.NormalizeAppDate(it.Task.CompleteDate) == today {
				out[kanbanColDone] = append(out[kanbanColDone], it)
				seen[key] = true
			}
			continue
		}
		if st == model.StatusPartialComplete {
			if it.TodaySchedule {
				out[kanbanColPartial] = append(out[kanbanColPartial], it)
				seen[key] = true
			}
			continue
		}
		ds := model.TaskDelayState(it.Task, today)
		if ds.Delayed {
			it.Overdue = true
			it.OverdueDays = ds.Days
			out[kanbanColDelay] = append(out[kanbanColDelay], it)
			seen[key] = true
			continue
		}
		if it.TodaySchedule {
			if kanbanInProgressStatus(st) {
				out[kanbanColProgress] = append(out[kanbanColProgress], it)
			} else {
				out[kanbanColPlan] = append(out[kanbanColPlan], it)
			}
			seen[key] = true
			continue
		}
		if kanbanInProgressStatus(st) {
			out[kanbanColOff] = append(out[kanbanColOff], it)
			seen[key] = true
		}
	}
	sort.SliceStable(out[kanbanColDelay], func(i, j int) bool {
		return out[kanbanColDelay][i].OverdueDays > out[kanbanColDelay][j].OverdueDays
	})
	sort.SliceStable(out[kanbanColOff], func(i, j int) bool {
		a, b := out[kanbanColOff][i].Task.DueDate, out[kanbanColOff][j].Task.DueDate
		if a == b {
			return out[kanbanColOff][i].Task.Title < out[kanbanColOff][j].Task.Title
		}
		if a == "" {
			return false
		}
		if b == "" {
			return true
		}
		return a < b
	})
	for _, col := range []string{kanbanColPlan, kanbanColProgress, kanbanColPartial, kanbanColDone} {
		sort.SliceStable(out[col], func(i, j int) bool {
			ai, aj := out[col][i], out[col][j]
			if ai.HasTime != aj.HasTime {
				return ai.HasTime
			}
			if ai.StartMin != aj.StartMin {
				return ai.StartMin < aj.StartMin
			}
			return ai.Task.Title < aj.Task.Title
		})
	}
	return out
}

func stampKanbanExtras(h *WorkboardHandler, cols map[string][]boardItem) {
	if h == nil || h.repo == nil {
		return
	}
	var ids []string
	for _, list := range cols {
		for _, it := range list {
			if id := strings.TrimSpace(it.Task.TaskID); id != "" {
				ids = append(ids, id)
			}
		}
	}
	follows, _ := h.repo.ListPartialFollowupsByParents(ids)
	for col, list := range cols {
		for i := range list {
			id := strings.TrimSpace(list[i].Task.TaskID)
			chs := follows[id]
			if len(chs) > 0 {
				ch := chs[0]
				list[i].FollowHref = "/workboard/tasks/" + ch.TaskID
				d := model.NormalizeAppDate(ch.WorkDate)
				if d == "" {
					d = model.NormalizeAppDate(ch.DueDate)
				}
				if d != "" {
					if t, err := time.ParseInLocation(dateLayout, d, time.Local); err == nil {
						list[i].FollowLabel = "후속 " + t.Format("01/02") + " ↗"
					} else {
						list[i].FollowLabel = "후속 " + d + " ↗"
					}
				} else {
					list[i].FollowLabel = "후속 ↗"
				}
			} else if col == kanbanColPartial {
				list[i].FollowMissing = true
			}
		}
		cols[col] = list
	}
}

func mergeKanbanOpen(h *WorkboardHandler, c echo.Context, entries *boardEntries, org string, scope boardScope, today string) {
	if h == nil || h.repo == nil || entries == nil {
		return
	}
	open, err := h.repo.ListOpenInProgressForOrg(org)
	if err != nil || len(open) == 0 {
		return
	}
	seen := entries.keySet()
	ids := taskIDsOf(open)
	memberMap := map[string][]model.WorkTaskMember{}
	if len(ids) > 0 {
		if m, err := h.repo.ListMembersByTaskIDs(ids); err == nil {
			memberMap = m
		}
	}
	open = applyBoardPeopleFilter(open, memberMap, scope)
	write := boardCanWrite(c)
	_ = write
	for _, t := range open {
		if t.Status == model.WBTaskCancelled {
			continue
		}
		it := buildBoardItem(h.userRepo, t, memberMap[t.TaskID], "", today)
		it.DedupKey = taskDedupKey(t)
		it.Href = t.DetailHref()
		if seen[it.DedupKey] {
			continue
		}
		seen[it.DedupKey] = true
		entries.Placed = append(entries.Placed, it)
	}
}

func (h *WorkboardHandler) BoardKanbanMove(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	from := strings.TrimSpace(c.FormValue("from"))
	to := strings.TrimSpace(c.FormValue("to"))
	key := strings.TrimSpace(c.FormValue("item_key"))
	if key == "" {
		key = strings.TrimSpace(c.FormValue("task_id"))
		if key != "" && !strings.Contains(key, ":") {
			key = "task:" + key
		}
	}
	today := time.Now().Format(dateLayout)
	date := model.NormalizeAppDate(c.FormValue("date"))
	if date == "" {
		date = today
	}
	note := strings.TrimSpace(c.FormValue("complete_note"))
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	if err := h.denyKanbanKeyOtherOrg(c, key); err != nil {
		return err
	}
	kind, id := splitKanbanKey(key)
	if t, _ := h.kanbanMoveTask(kind, id); t != nil {
		if err := denyUnlessCanEditTask(c, t); err != nil {
			if to == kanbanColPlan || from == kanbanColDelay {
				if err2 := denyUnlessCanPlanEdit(c, t); err2 != nil {
					return err2
				}
			} else {
				return err
			}
		}
	}
	if service.BoardKanbanForbidden(from, to) {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+service.KanbanErrForbidden))
	}
	if (from == kanbanColDelay || from == kanbanColOff) && to == kanbanColPlan {
		if strings.TrimSpace(c.FormValue("confirm_carry")) != "1" {
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+service.KanbanErrCarryConfirm))
		}
		if strings.TrimSpace(c.FormValue("task_id")) == "" {
			if _, id := splitKanbanKey(key); id != "" {
				_ = c.Request().ParseForm()
				if c.Request().PostForm == nil {
					c.Request().PostForm = url.Values{}
				}
				c.Request().PostForm.Set("task_id", id)
			}
		}
		return h.CarryTask(c)
	}
	if from == kanbanColPlan && to == kanbanColPartial {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+service.KanbanErrPartialEmpty))
	}
	if to == kanbanColPartial {
		return h.boardKanbanPartial(c, key, note)
	}
	if to == kanbanColDone {
		return h.boardKanbanComplete(c, key, from, date, note)
	}
	if from == kanbanColPartial && to == kanbanColProgress {
		if strings.TrimSpace(c.FormValue("confirm_reopen")) != "1" {
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=kanban_reopen"))
		}
		return h.kanbanSetProgress(key, true)
	}
	if from == kanbanColDone && to == kanbanColProgress {
		err := service.ApplyKanbanMove(handlerKanbanOps{h: h, c: c}, service.KanbanMove{
			Key: key, From: model.WorkBucketCompletedToday, To: model.WorkBucketInProgress,
			Date: date, Assignee: assignee, Note: note, Org: assignableOrgID(c),
			ConfirmReopen: strings.TrimSpace(c.FormValue("confirm_reopen")) == "1",
		})
		return h.redirectKanbanErr(c, err)
	}
	fromB, toB := service.BoardKanbanToBucket(from), service.BoardKanbanToBucket(to)
	err := service.ApplyKanbanMove(handlerKanbanOps{h: h, c: c}, service.KanbanMove{
		Key: key, From: fromB, To: toB, Date: date, Assignee: assignee, Note: note,
		Org: assignableOrgID(c), ConfirmReopen: strings.TrimSpace(c.FormValue("confirm_reopen")) == "1",
	})
	return h.redirectKanbanErr(c, err)
}

func (h *WorkboardHandler) kanbanMoveTask(kind, id string) (*model.WorkTask, error) {
	switch kind {
	case "task":
		return h.repo.GetTask(id)
	case "as":
		return h.repo.GetTaskBySource(model.WBSourceAS, id)
	case "mnt":
		return h.repo.GetTaskBySource(model.WBSourceMaintenance, id)
	default:
		return h.repo.GetTask(id)
	}
}

func (h *WorkboardHandler) boardKanbanComplete(c echo.Context, key, from, date, note string) error {
	kind, id := splitKanbanKey(key)
	t, _ := h.kanbanMoveTask(kind, id)
	if t != nil {
		if err := denyIfProjectMatchPending(t); err != nil {
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=project_match"))
		}
	}
	if t != nil && model.SourceLinkedASOrMnt(*t) {
		href := t.DetailHref()
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+service.KanbanErrActionScreen+"&action="+url.QueryEscape(href)))
	}
	if t != nil {
		has, _ := h.repo.HasIncompleteItems(t.TaskID)
		if has && strings.TrimSpace(c.FormValue("confirm_partial")) != "1" {
			return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+service.KanbanErrPartialItems))
		}
		if has && strings.TrimSpace(c.FormValue("confirm_partial")) == "1" {
			return h.boardKanbanPartial(c, key, note)
		}
		if from == kanbanColPartial {
			has, _ = h.repo.HasIncompleteItems(t.TaskID)
			if has {
				return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+service.KanbanErrRemainItems))
			}
		}
	}
	if note == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=kanban_complete"))
	}
	err := service.ApplyKanbanMove(handlerKanbanOps{h: h, c: c}, service.KanbanMove{
		Key: key, From: service.BoardKanbanToBucket(from), To: model.WorkBucketCompletedToday,
		Date: date, Note: note, Org: assignableOrgID(c),
	})
	return h.redirectKanbanErr(c, err)
}

func (h *WorkboardHandler) boardKanbanPartial(c echo.Context, key, note string) error {
	kind, id := splitKanbanKey(key)
	t, err := h.kanbanMoveTask(kind, id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanEditTask(c, t); err != nil {
		return err
	}
	has, _ := h.repo.HasIncompleteItems(t.TaskID)
	if !has {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+service.KanbanErrPartialEmpty))
	}
	if strings.TrimSpace(note) == "" {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=kanban_complete"))
	}
	t.Status = model.StatusPartialComplete
	t.CompleteNote = note
	t.ResultCode = model.WorkResultPartial
	if err := h.updateTaskLogged(t); err != nil {
		return err
	}
	if c.FormValue("create_followup") != "0" {
		next := strings.TrimSpace(c.FormValue("next_date"))
		if next == "" {
			who := taskWhoNames(t, nil)
			next, _ = nextWorkdayFor(h, time.Now(), who, t.OrgID, false)
		}
		_ = h.createPartialFollowup(c, t, next)
	}
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=kanban"))
}

func (h *WorkboardHandler) redirectKanbanErr(c echo.Context, err error) error {
	if err == nil {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "ok=kanban"))
	}
	if km, ok := err.(*service.KanbanMoveError); ok {
		code := km.Code
		if code == service.KanbanErrCompleteNote {
			code = "kanban_complete"
		}
		if code == service.KanbanErrAssignee {
			code = "kanban_assignee"
		}
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+code))
	}
	if km, ok := err.(*kanbanMoveError); ok {
		return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+km.code))
	}
	return err
}

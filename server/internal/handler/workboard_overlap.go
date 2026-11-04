package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

var errOverlapResponse = errors.New("overlap response written")

func overlapDone(err error) error {
	if errors.Is(err, errOverlapResponse) {
		return nil
	}
	return err
}

type overlapConflictJSON struct {
	TaskID           string   `json:"task_id"`
	Title            string   `json:"title"`
	Assignee         string   `json:"assignee"`
	Date             string   `json:"date,omitempty"`
	Start            string   `json:"start"`
	End              string   `json:"end"`
	WorkPlace        string   `json:"work_place"`
	WorkPlaceLabel   string   `json:"work_place_label"`
	Href             string   `json:"href"`
	CanSplit         bool     `json:"can_split"`
	SplitBlockReason string   `json:"split_block_reason,omitempty"`
	SplitBlockLabel  string   `json:"split_block_label,omitempty"`
	Preview          []string `json:"preview,omitempty"`
}

type overlapHit struct {
	Task     model.WorkTask
	Assignee string
	Date     string
}

type overlapDayJSON struct {
	Date      string                `json:"date"`
	Conflicts []overlapConflictJSON `json:"conflicts"`
}

func workPlaceLabel(p string) string {
	switch model.NormalizeWorkPlace(p) {
	case model.WorkPlaceField:
		return "외근"
	case model.WorkPlaceOffice:
		return "내근"
	default:
		return ""
	}
}

func (h *WorkboardHandler) listParticipantOverlaps(date, start, end, excludeID string, names []string) []overlapHit {
	if h == nil || h.repo == nil || strings.TrimSpace(date) == "" || strings.TrimSpace(start) == "" {
		return nil
	}
	seen := map[string]bool{}
	var hits []overlapHit
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		list, err := h.repo.ListAssigneeTimeOverlaps(date, name, start, end, excludeID)
		if err != nil {
			continue
		}
		for _, t := range list {
			if t.TaskID == "" || seen[t.TaskID] {
				continue
			}
			seen[t.TaskID] = true
			_ = h.repo.LoadTaskActionFields(&t)
			hits = append(hits, overlapHit{Task: t, Assignee: name, Date: date})
		}
	}
	return hits
}

func (h *WorkboardHandler) firstParticipantOverlap(date, start, end, excludeID string, names []string) string {
	hits := h.listParticipantOverlaps(date, start, end, excludeID, names)
	if len(hits) == 0 {
		return ""
	}
	t := hits[0].Task
	return assigneeOverlapMessage(hits[0].Assignee, t.StartTime, t.EndTime)
}

func overlapConflictsJSON(c echo.Context, h *WorkboardHandler, hits []overlapHit, newStart, newEnd string, travel int) []overlapConflictJSON {
	out := make([]overlapConflictJSON, 0, len(hits))
	for _, hit := range hits {
		t := hit.Task
		place := model.NormalizeWorkPlace(t.WorkPlace)
		mems, _ := h.repo.ListMembers(t.TaskID)
		can, reason, label := canSplitPlanReason(c, &t, mems)
		var preview []string
		if can {
			plan, _ := h.repo.ListPlanSegmentsOrVirtual(t)
			if len(plan) == 0 {
				can = false
				reason, label = "no_segments", splitBlockLabel("no_segments")
			}
			splitOK := false
			for _, seg := range plan {
				if !model.RangesOverlapHHMM(seg.StartTime, seg.EndTime, newStart, newEnd) {
					continue
				}
				sp, err := model.SplitPlanAround(seg.StartTime, seg.EndTime, newStart, newEnd, travel)
				if err != nil {
					if reason == "" {
						reason, label = "too_short", splitBlockLabel("too_short")
					}
					continue
				}
				splitOK = true
				if sp.HasFront {
					preview = append(preview, t.Title+" "+sp.FrontStart+"~"+sp.FrontEnd)
				}
				preview = append(preview, "끼워넣기 "+newStart+"~"+newEnd)
				if sp.HasBack {
					preview = append(preview, t.Title+" "+sp.BackStart+"~"+sp.BackEnd)
				}
			}
			if can && !splitOK {
				can = false
				if reason == "" {
					reason, label = "too_short", splitBlockLabel("too_short")
				}
			}
		}
		out = append(out, overlapConflictJSON{
			TaskID:           t.TaskID,
			Title:            t.Title,
			Assignee:         hit.Assignee,
			Date:             hit.Date,
			Start:            trimHHMM(t.StartTime),
			End:              trimHHMM(t.EndTime),
			WorkPlace:        place,
			WorkPlaceLabel:   workPlaceLabel(place),
			Href:             "/workboard/tasks/" + t.TaskID + "/edit",
			CanSplit:         can,
			SplitBlockReason: reason,
			SplitBlockLabel:  label,
			Preview:          preview,
		})
	}
	return out
}

func overlapMessageFromHits(hits []overlapHit, pendingPlace string) string {
	if len(hits) == 0 {
		return "같은 담당자의 시간이 겹칩니다"
	}
	msg := assigneeOverlapMessage(hits[0].Assignee, hits[0].Task.StartTime, hits[0].Task.EndTime)
	pendingPlace = model.NormalizeWorkPlace(pendingPlace)
	if pendingPlace == model.WorkPlaceField {
		for _, h := range hits {
			if model.NormalizeWorkPlace(h.Task.WorkPlace) == model.WorkPlaceField {
				return msg + " (외근이 겹칩니다)"
			}
		}
	}
	return msg
}

func postedOverlapIDs(c echo.Context) []string {
	if c == nil || c.Request() == nil {
		return nil
	}
	_ = c.Request().ParseForm()
	raw := append([]string{}, c.Request().PostForm["conflict_task_id"]...)
	raw = append(raw, c.Request().PostForm["conflict_task_ids"]...)
	raw = append(raw, c.Request().PostForm["conflict_task_ids[]"]...)
	seen := map[string]bool{}
	var out []string
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func overlapHitIDs(hits []overlapHit) []string {
	out := make([]string, 0, len(hits))
	seen := map[string]bool{}
	for _, h := range hits {
		id := strings.TrimSpace(h.Task.TaskID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func idSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		if m[x] == 0 {
			return false
		}
		m[x]--
	}
	return true
}

func idSetCovers(posted, current []string) bool {
	m := map[string]bool{}
	for _, x := range posted {
		m[x] = true
	}
	for _, x := range current {
		if !m[x] {
			return false
		}
	}
	return true
}

func overlapActionErr(c echo.Context, msg string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "겹침을 처리하지 못했습니다"
	}
	if wantsJSONError(c) {
		if err := c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":   "overlap_action",
			"message": msg,
		}); err != nil {
			return err
		}
		return errOverlapResponse
	}
	setFlashErrCookie(c, msg)
	if err := c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+url.QueryEscape(msg))); err != nil {
		return err
	}
	return errOverlapResponse
}

func overlapDeleteBlockedReason(t *model.WorkTask) string {
	if t == nil {
		return "겹친 업무를 찾을 수 없습니다"
	}
	st := strings.TrimSpace(t.Status)
	if st == model.WBTaskComplete || st == model.WBTaskCancelled {
		return "완료된 업무는 삭제할 수 없습니다"
	}
	if strings.TrimSpace(t.SourceType) != "" && strings.TrimSpace(t.SourceID) != "" {
		return "원본에 연결된 업무는 삭제할 수 없습니다"
	}
	return ""
}

func (h *WorkboardHandler) applyOverlapResolutionToHits(c echo.Context, action string, hits []overlapHit, names []string) error {
	switch action {
	case "edit":
		pick := strings.TrimSpace(c.FormValue("overlap_edit_task_id"))
		if pick == "" {
			pick = strings.TrimSpace(c.FormValue("conflict_task_id"))
		}
		id := hits[0].Task.TaskID
		for _, hit := range hits {
			if strings.TrimSpace(hit.Task.TaskID) == pick {
				id = pick
				break
			}
		}
		t, err := h.repo.GetTask(id)
		if err != nil || t == nil {
			return overlapActionErr(c, "겹친 업무를 찾을 수 없습니다")
		}
		if err := denyOtherOrgTask(c, t); err != nil {
			return overlapActionErr(c, "다른 조직의 업무입니다")
		}
		return overlapEditReply(c, t.TaskID)
	case "delete":
		var targets []*model.WorkTask
		for _, hit := range hits {
			t, err := h.repo.GetTask(hit.Task.TaskID)
			if err != nil || t == nil {
				return overlapActionErr(c, "겹친 업무를 찾을 수 없습니다")
			}
			if err := denyOtherOrgTask(c, t); err != nil {
				return overlapActionErr(c, "다른 조직의 업무입니다")
			}
			if why := overlapDeleteBlockedReason(t); why != "" {
				return overlapActionErr(c, why)
			}
			if !canPlanEditTask(c, t) {
				return overlapActionErr(c, "기존 계획을 삭제할 수 없습니다")
			}
			targets = append(targets, t)
		}
		for _, t := range targets {
			if err := h.repo.DeleteTaskLogged(t.TaskID, "겹침: 기존 계획 삭제"); err != nil {
				return overlapActionErr(c, "기존 계획을 삭제하지 못했습니다")
			}
		}
		return nil
	case "skip":
		var days []string
		for _, hit := range hits {
			if d := strings.TrimSpace(hit.Date); d != "" {
				days = append(days, d)
			}
		}
		if len(days) == 0 {
			if d := strings.TrimSpace(c.FormValue("work_date")); d != "" {
				days = append(days, d)
			}
		}
		mergeOverlapSkipDates(c, days)
		return nil
	case "split":
		travel := parseSplitTravel(c)
		date := strings.TrimSpace(c.FormValue("work_date"))
		start := strings.TrimSpace(c.FormValue("start_time"))
		end := strings.TrimSpace(c.FormValue("end_time"))
		skip := overlapSkipDateSet(c)
		var keep []overlapHit
		for _, hit := range hits {
			day := strings.TrimSpace(hit.Date)
			if day == "" {
				day = date
			}
			if skip[day] {
				continue
			}
			t, err := h.repo.GetTask(hit.Task.TaskID)
			if err != nil || t == nil {
				return overlapActionErr(c, "겹친 업무를 찾을 수 없습니다")
			}
			if err := denyOtherOrgTask(c, t); err != nil {
				return overlapActionErr(c, "다른 조직의 업무입니다")
			}
			mems, _ := h.repo.ListMembers(t.TaskID)
			ok, _, label := canSplitPlanReason(c, t, mems)
			if !ok {
				if label == "" {
					label = "이 계획을 나눌 권한이 없습니다"
				}
				return overlapActionErr(c, label)
			}
			plan, _ := h.repo.ListPlanSegmentsOrVirtual(*t)
			okSplit := false
			for _, seg := range plan {
				if model.RangesOverlapHHMM(seg.StartTime, seg.EndTime, start, end) {
					if _, err := model.SplitPlanAround(seg.StartTime, seg.EndTime, start, end, travel); err == nil {
						okSplit = true
					}
				}
			}
			if !okSplit {
				return overlapActionErr(c, "남는 구간이 15분 미만이라 끼워 넣을 수 없습니다")
			}
			hit.Date = day
			keep = append(keep, hit)
		}
		if len(keep) == 0 {
			return nil
		}
		c.Set("pending_interrupt", &pendingInterrupt{
			Hits: keep, Travel: travel, Date: date, Start: start, End: end, Names: names,
		})
		return nil
	case "leave":
		for _, hit := range hits {
			t, err := h.repo.GetTask(hit.Task.TaskID)
			if err != nil || t == nil {
				return overlapActionErr(c, "겹친 업무를 찾을 수 없습니다")
			}
			if err := denyOtherOrgTask(c, t); err != nil {
				return overlapActionErr(c, "다른 조직의 업무입니다")
			}
			if !canPlanEditTask(c, t) {
				return overlapActionErr(c, "완료된 업무에서는 담당자를 뺄 수 없습니다")
			}
			if err := h.dropNamesFromTask(t, names); err != nil {
				return overlapActionErr(c, "담당자를 빼지 못했습니다")
			}
		}
		return nil
	default:
		return overlapActionErr(c, "알 수 없는 겹침 처리입니다")
	}
}

func overlapEditReply(c echo.Context, taskID string) error {
	loc := "/workboard/tasks/" + taskID + "/edit?notice=overlap_edit"
	if wantsJSONError(c) {
		if err := c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "redirect": loc}); err != nil {
			return err
		}
		return errOverlapResponse
	}
	if err := c.Redirect(http.StatusSeeOther, loc); err != nil {
		return err
	}
	return errOverlapResponse
}

func overlapCancelled(c echo.Context) error {
	if wantsJSONError(c) {
		if err := c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "cancelled": true}); err != nil {
			return err
		}
		return errOverlapResponse
	}
	if err := c.Redirect(http.StatusSeeOther, redirectBack(c, "")); err != nil {
		return err
	}
	return errOverlapResponse
}

func (h *WorkboardHandler) dropNamesFromTask(t *model.WorkTask, names []string) error {
	if t == nil {
		return nil
	}
	drop := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			drop[n] = true
		}
	}
	if len(drop) == 0 {
		return nil
	}
	shouldDrop := func(name, uid string) bool {
		return drop[strings.TrimSpace(name)] || (strings.TrimSpace(uid) != "" && drop[strings.TrimSpace(uid)])
	}
	return h.repo.UpdateTaskLogged(t.TaskID, "겹침: 담당자만 제외", func() error {
		members, _ := h.repo.ListMembers(t.TaskID)
		owner := strings.TrimSpace(t.Assignee)
		var supports []model.WorkTaskMember
		for _, m := range members {
			if !m.IsSupport() {
				continue
			}
			if shouldDrop(m.Assignee, m.UserID) {
				continue
			}
			supports = append(supports, m)
		}
		if shouldDrop(owner, t.AssigneeUserID) {
			newOwner := ""
			var rest []model.WorkTaskMember
			for _, m := range supports {
				if newOwner == "" {
					newOwner = m.Assignee
					continue
				}
				rest = append(rest, m)
			}
			if err := h.repo.SetTaskAssignee(t.TaskID, newOwner); err != nil {
				return err
			}
			return h.repo.ReplaceSupportMembers(t.TaskID, rest)
		}
		return h.repo.ReplaceSupportMembers(t.TaskID, supports)
	})
}

func (h *WorkboardHandler) rejectIfOverlap(c echo.Context, date, start, end, excludeID string, names []string) error {
	return h.rejectIfOverlapDates(c, []string{date}, start, end, excludeID, names)
}

func (h *WorkboardHandler) rejectIfOverlapDates(c echo.Context, dates []string, start, end, excludeID string, names []string) error {
	action := strings.TrimSpace(c.FormValue("overlap_action"))
	if action == "cancel" {
		return overlapCancelled(c)
	}
	dates = filterSkippedDates(dates, overlapSkipDateSet(c))
	hits := h.PlanOverlapsForDates(dates, start, end, excludeID, names)
	place := model.NormalizeWorkPlace(c.FormValue("work_place"))
	if action == "" {
		if len(hits) == 0 {
			return nil
		}
		return respondOverlap(c, hits, place, start, end, parseSplitTravel(c), h)
	}
	posted := postedOverlapIDs(c)
	if len(posted) > 0 {
		curIDs := overlapHitIDs(hits)
		okIDs := idSetEqual(posted, curIDs)
		if len(overlapSkipDateSet(c)) > 0 {
			okIDs = idSetCovers(posted, curIDs)
		}
		if !okIDs {
			if len(hits) == 0 {
				return nil
			}
			return respondOverlap(c, hits, place, start, end, parseSplitTravel(c), h)
		}
	}
	if len(hits) == 0 {
		return nil
	}
	if err := h.applyOverlapResolutionToHits(c, action, hits, names); err != nil {
		return err
	}
	if action == "edit" {
		return errOverlapResponse
	}
	if action == "split" || action == "skip" {
		return nil
	}
	remain := filterSkippedDates(dates, overlapSkipDateSet(c))
	hits = h.PlanOverlapsForDates(remain, start, end, excludeID, names)
	if len(hits) == 0 {
		return nil
	}
	return respondOverlap(c, hits, place, start, end, parseSplitTravel(c), h)
}

func respondOverlap(c echo.Context, hits []overlapHit, pendingPlace, newStart, newEnd string, travel int, h *WorkboardHandler) error {
	msg := overlapMessageFromHits(hits, pendingPlace)
	conflicts := overlapConflictsJSON(c, h, hits, newStart, newEnd, travel)
	days := groupOverlapDays(conflicts)
	if wantsJSONError(c) {
		if err := c.JSON(http.StatusConflict, map[string]interface{}{
			"error":      "overlap",
			"message":    msg,
			"conflicts":  conflicts,
			"dates":      days,
			"travel_min": travel,
		}); err != nil {
			return err
		}
		return errOverlapResponse
	}
	setFlashErrCookie(c, msg)
	if err := c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+url.QueryEscape(msg))); err != nil {
		return err
	}
	return errOverlapResponse
}

func overlapRedirect(c echo.Context, msg string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "같은 담당자의 시간이 겹칩니다"
	}
	if wantsJSONError(c) {
		if err := c.JSON(http.StatusConflict, map[string]interface{}{
			"error":   "overlap",
			"message": msg,
		}); err != nil {
			return err
		}
		return errOverlapResponse
	}
	setFlashErrCookie(c, msg)
	if err := c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+url.QueryEscape(msg))); err != nil {
		return err
	}
	return errOverlapResponse
}

func visibleQueryErr(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if un, err := url.QueryUnescape(raw); err == nil {
		raw = un
	}
	if strings.Contains(raw, "다른 업무가 있습니다") || strings.Contains(raw, "겹칩") {
		return raw
	}
	return ""
}

func (h *WorkboardHandler) applyPlanWorkPlace(c echo.Context, taskID string) {
	if h == nil || h.repo == nil {
		return
	}
	place := model.NormalizeWorkPlace(c.FormValue("work_place"))
	if place == "" {
		return
	}
	_ = h.repo.SetTaskWorkPlace(taskID, place)
}

func planEditHref(taskID string) string {
	id := strings.TrimSpace(taskID)
	if id == "" || strings.HasPrefix(id, "ext:") {
		return ""
	}
	return "/workboard/tasks/" + id + "/edit"
}

func planEditHrefFromTask(t *model.WorkTask) string {
	if t == nil {
		return ""
	}
	return planEditHref(t.TaskID)
}

func canPlanEditTask(c echo.Context, t *model.WorkTask) bool {
	if t == nil {
		return false
	}
	if isReadOnly(c) || !canWriteWorkboard(c) {
		return false
	}
	st := strings.TrimSpace(t.Status)
	if st == model.WBTaskComplete || st == model.WBTaskCancelled {
		return false
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return false
	}
	return canEditTask(c, t.Assignee, t.AssigneeUserID, t.WorkGroupID)
}

func denyUnlessCanPlanEdit(c echo.Context, t *model.WorkTask) error {
	if t == nil {
		return echo.ErrNotFound
	}
	if !canPlanEditTask(c, t) {
		return echo.ErrForbidden
	}
	return nil
}

func stampBoardPlanEdit(c echo.Context, items []boardItem) {
	for i := range items {
		if canPlanEditTask(c, &items[i].Task) {
			items[i].PlanEditHref = planEditHref(items[i].Task.TaskID)
			if !items[i].Fragment {
				items[i].CanCarry = true
			}
		}
	}
}

func workListPlanEditHref(c echo.Context, it model.WorkListItem) string {
	if it.EditLocked {
		return ""
	}
	st := strings.TrimSpace(it.Status)
	if st == model.WBTaskComplete || st == model.WBTaskCancelled {
		return ""
	}
	id := strings.TrimSpace(it.RefID)
	href := strings.TrimSpace(it.Href)
	if strings.Contains(href, "/workboard/tasks/") {
		rest := href[strings.Index(href, "/workboard/tasks/")+len("/workboard/tasks/"):]
		if i := strings.IndexAny(rest, "/?"); i >= 0 {
			rest = rest[:i]
		}
		id = rest
	} else if !strings.HasPrefix(id, "WT") {
		return ""
	}
	if id == "" {
		return ""
	}
	org := currentOrg(c)
	if org != "" && org != repository.OrgAll && strings.TrimSpace(it.OrgID) != "" && strings.TrimSpace(it.OrgID) != org {
		return ""
	}
	return planEditHref(id)
}

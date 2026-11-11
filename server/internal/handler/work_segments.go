package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

var (
	errSegShift       = errors.New("seg_shift")
	errSegMerge       = errors.New("seg_merge")
	errSegDate        = errors.New("seg_date_move")
	errScheduleClosed = errors.New("schedule_closed")
)

type workSegRow struct {
	Index         int
	StartLocal    string
	EndLocal      string
	Worker        string
	Label         string
	CanDelete     bool
	CanMerge      bool
	DeleteHint    string
	InterruptNote string
}

func planClockOf(repo *repository.WBRepo, t *model.WorkTask) (start, end string, n int) {
	if t == nil {
		return "", "", 0
	}
	if repo != nil {
		if stored, err := repo.CountStoredPlanSegments(t.TaskID); err == nil {
			n = stored
		}
		segs, _ := repo.ListPlanSegmentsOrVirtual(*t)
		st, en, _ := model.EnvelopeFromPlanSegments(segs)
		return model.ClockHHMM(st), model.ClockHHMM(en), n
	}
	return model.ClockHHMM(t.StartTime), model.ClockHHMM(t.EndTime), 0
}

func buildWorkSegRows(repo *repository.WBRepo, t *model.WorkTask) ([]workSegRow, int) {
	if t == nil || repo == nil {
		return nil, 0
	}
	planSegs, _ := repo.ListPlanSegmentsOrVirtual(*t)
	actualSegs, _ := repo.ListActualSegments(t.TaskID)
	src := actualSegs
	if len(src) == 0 {
		src = planSegs
	}
	day := strings.TrimSpace(t.WorkDate)
	open := false
	for _, s := range src {
		if segmentStillOpen(s) {
			open = true
			break
		}
	}
	var rows []workSegRow
	total := 0
	for i, s := range src {
		label := fmt.Sprintf("%d/%d", i+1, len(src))
		note := ""
		if strings.TrimSpace(s.InterruptTaskID) != "" {
			note = "끼워 넣은 업무 연결이 이 구간에서 빠질 수 있습니다"
		}
		hint := "마지막 구간은 삭제할 수 없습니다"
		if open {
			hint = "진행 중인 구간은 삭제하거나 합칠 수 없습니다"
		}
		stored := !strings.HasPrefix(s.SegmentID, "virtual:")
		rows = append(rows, workSegRow{
			Index:         i,
			StartLocal:    datetimeLocalFromSeg(firstNonEmpty(s.WorkDate, day), s.StartTime),
			EndLocal:      datetimeLocalFromSeg(firstNonEmpty(s.WorkDate, day), s.EndTime),
			Worker:        firstNonEmpty(s.WorkerName, s.Assignee, t.Assignee),
			Label:         label,
			CanDelete:     len(src) > 1 && !open && stored,
			CanMerge:      i > 0 && !open && stored,
			DeleteHint:    hint,
			InterruptNote: note,
		})
		total += s.DurationMin
		if s.DurationMin <= 0 {
			total += model.DurationFromTimes(s.StartTime, s.EndTime)
		}
	}
	if len(rows) == 0 && strings.TrimSpace(t.StartTime) != "" {
		rows = append(rows, workSegRow{
			StartLocal: datetimeLocalFromSeg(day, t.StartTime),
			EndLocal:   datetimeLocalFromSeg(day, t.EndTime),
			Worker:     t.Assignee,
			Label:      "1/1",
		})
		total = t.DurationMin
	}
	return rows, total
}

func attachWorkSegData(data map[string]interface{}, repo *repository.WBRepo, t *model.WorkTask, errCode, cancelHref string, readOnly bool) {
	if data == nil {
		return
	}
	rows, total := buildWorkSegRows(repo, t)
	data["WorkSegRows"] = rows
	data["SegTotalMin"] = total
	data["SegsOpen"] = errCode == "seg_shift" || errCode == "seg_merge"
	data["SegsReadOnly"] = readOnly
	data["SegsCancelHref"] = cancelHref
	if t != nil {
		data["SegTaskID"] = t.TaskID
	}
	if t != nil && strings.TrimSpace(t.WorkDate) != "" {
		data["SegAddHref"] = "/workboard/board?view=day&date=" + t.WorkDate
	}
	if errCode == "seg_shift" {
		data["ActionErr"] = "seg_shift"
		if data["ActionErrMsg"] == nil || data["ActionErrMsg"] == "" {
			data["ActionErrMsg"] = projectWorkActionErr("seg_shift")
		}
	}
	if errCode == "seg_merge" {
		data["ActionErr"] = "seg_merge"
		if data["ActionErrMsg"] == nil || data["ActionErrMsg"] == "" {
			data["ActionErrMsg"] = "이 업무는 여러 구간으로 나뉘어 있습니다. 방문 시간을 바꾸면 구간이 하나로 합쳐집니다"
		}
	}
	if errCode == "seg_date" {
		data["ActionErr"] = "seg_date"
		if data["ActionErrMsg"] == nil || data["ActionErrMsg"] == "" {
			data["ActionErrMsg"] = "사람별 구간이 있는 업무의 날짜를 옮기면 공통 구간 1개로 합칩니다"
		}
	}
}

func confirmShiftFollowingSegments(c echo.Context, repo *repository.WBRepo, t *model.WorkTask, actualSegs []model.WorkTaskSegment) error {
	if repo == nil || t == nil || len(actualSegs) == 0 {
		return nil
	}
	plan, _ := repo.ListPlanSegmentsOrVirtual(*t)
	overrun := actualOverrunsNextPlan(actualSegs, plan)
	if hosts, err := repo.TasksInterruptedBy(t.TaskID); err == nil {
		last := actualSegs[len(actualSegs)-1].EndTime
		ae := model.ParseHHMMMinutes(last)
		for _, hid := range hosts {
			ht, _ := repo.GetTask(hid)
			if ht == nil {
				continue
			}
			hp, _ := repo.ListPlanSegmentsOrVirtual(*ht)
			for _, p := range hp {
				if strings.TrimSpace(p.InterruptTaskID) != t.TaskID {
					continue
				}
				ps := model.ParseHHMMMinutes(p.StartTime)
				if ae >= 0 && ps >= 0 && ae > ps {
					overrun = true
				}
			}
		}
	}
	confirm := strings.TrimSpace(c.FormValue("confirm_seg_shift"))
	if overrun && confirm != "1" && confirm != "0" {
		return errSegShift
	}
	if err := repo.ReplaceActualSegments(t.TaskID, actualSegs); err != nil {
		return err
	}
	if confirm == "1" {
		last := actualSegs[len(actualSegs)-1].EndTime
		if hosts, err := repo.TasksInterruptedBy(t.TaskID); err == nil {
			for _, hid := range hosts {
				ht, _ := repo.GetTask(hid)
				if ht == nil {
					continue
				}
				_ = repo.ShiftPlanAfter(ht, last, t.TaskID)
			}
		}
	}
	return nil
}

func savePostedActionSegments(c echo.Context, repo *repository.WBRepo, t *model.WorkTask) error {
	if t == nil || repo == nil {
		return nil
	}
	if _, ok := c.Request().PostForm["seg_start"]; !ok {
		_ = c.Request().ParseForm()
		if _, ok := c.Request().PostForm["seg_start"]; !ok {
			return nil
		}
	}
	actual := parseActualSegmentsForm(c, t)
	if len(actual) == 0 {
		return nil
	}
	return confirmShiftFollowingSegments(c, repo, t, actual)
}

func envelopeTimes(segs []model.WorkTaskSegment) (start, end string, dur int) {
	return model.EnvelopeFromPlanSegments(segs)
}

func placeTaskMovingSegments(c echo.Context, repo *repository.WBRepo, taskID, date, start, end string) error {
	if repo == nil {
		return nil
	}
	if t, _ := repo.GetTask(taskID); t != nil {
		if err := applyDateMoveToSegments(c, repo, t, t.WorkDate, date); err != nil {
			return err
		}
	}
	return repo.PlaceTask(taskID, date, start, end)
}

func applyDateMoveToSegments(c echo.Context, repo *repository.WBRepo, t *model.WorkTask, oldDate, newDate string) error {
	if repo == nil || t == nil {
		return nil
	}
	oldDate, newDate = strings.TrimSpace(oldDate), strings.TrimSpace(newDate)
	if oldDate == "" || newDate == "" || oldDate == newDate {
		return nil
	}
	nPerson, _ := repo.CountPersonScopedPlan(t.TaskID)
	if nPerson > 0 {
		conf := strings.TrimSpace(c.FormValue("confirm_collapse_segments"))
		if conf != "1" && conf != "0" {
			return errSegDate
		}
		if conf == "1" {
			st, en := strings.TrimSpace(t.StartTime), strings.TrimSpace(t.EndTime)
			_ = repo.CollapsePlanToSingle(t, newDate, st, en)
		}
	}
	return nil
}

func (h *WorkboardHandler) DeleteWorkSegment(c echo.Context) error {
	return h.mutateVisibleSegments(c, false)
}

func (h *WorkboardHandler) MergeWorkSegment(c echo.Context) error {
	return h.mutateVisibleSegments(c, true)
}

func (h *WorkboardHandler) mutateVisibleSegments(c echo.Context, merge bool) error {
	if h == nil || h.repo == nil {
		return echo.ErrNotFound
	}
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return err
	}
	if !canEditTask(c, t.Assignee, t.AssigneeUserID, t.WorkGroupID) {
		return echo.ErrForbidden
	}
	idx, err := strconv.Atoi(strings.TrimSpace(c.FormValue("seg_index")))
	if err != nil || idx < 0 {
		return h.redirectSeg(c, t.TaskID, "err=seg_empty")
	}
	kind, segs, err := storedVisibleSegments(h.repo, t)
	if err != nil {
		return err
	}
	if len(segs) == 0 {
		return h.redirectSeg(c, t.TaskID, "err=seg_empty")
	}
	if segmentsHaveOpen(segs) {
		return h.redirectSeg(c, t.TaskID, "err=seg_live")
	}
	if idx >= len(segs) {
		return h.redirectSeg(c, t.TaskID, "err=seg_empty")
	}
	var next []model.WorkTaskSegment
	var note string
	if merge {
		if idx < 1 {
			return h.redirectSeg(c, t.TaskID, "err=seg_date_mix")
		}
		prev, cur := segs[idx-1], segs[idx]
		if strings.TrimSpace(prev.WorkDate) != strings.TrimSpace(cur.WorkDate) {
			return h.redirectSeg(c, t.TaskID, "err=seg_date_mix")
		}
		merged := prev
		merged.EndTime = cur.EndTime
		merged.DurationMin = model.DurationFromTimes(merged.StartTime, merged.EndTime)
		dropped := ""
		if strings.TrimSpace(merged.InterruptTaskID) == "" {
			merged.InterruptTaskID = cur.InterruptTaskID
		} else if id := strings.TrimSpace(cur.InterruptTaskID); id != "" && id != strings.TrimSpace(merged.InterruptTaskID) {
			dropped = id
		}
		next = append(append([]model.WorkTaskSegment{}, segs[:idx-1]...), merged)
		next = append(next, segs[idx+1:]...)
		note = fmt.Sprintf("구간 합치기: %s + %s → %s", segSpan(prev), segSpan(cur), segSpan(merged))
		if dropped != "" {
			note += " (끼워 넣은 업무 " + dropped + " 연결은 앞 구간에 남지 않음)"
		}
	} else {
		if len(segs) <= 1 {
			return h.redirectSeg(c, t.TaskID, "err=seg_last")
		}
		gone := segs[idx]
		next = append(append([]model.WorkTaskSegment{}, segs[:idx]...), segs[idx+1:]...)
		note = fmt.Sprintf("구간 삭제: %s 제거", segSpan(gone))
		if id := strings.TrimSpace(gone.InterruptTaskID); id != "" {
			note += " (끼워 넣은 업무 " + id + " 연결이 이 구간에서 빠짐)"
		}
	}
	if err := replaceVisibleSegments(h.repo, t.TaskID, kind, next); err != nil {
		return err
	}
	_ = h.repo.InsertWorkTaskDelay(repository.WorkTaskDelay{
		TaskID: t.TaskID, OrgID: t.OrgID, Kind: repository.DelayKindSegment,
		Reason: note, CreatedByUserID: currentUserID(c), CreatedByName: ctxString(c, "user_name"),
	})
	return h.redirectSeg(c, t.TaskID, "ok=seg")
}

func storedVisibleSegments(repo *repository.WBRepo, t *model.WorkTask) (string, []model.WorkTaskSegment, error) {
	if repo == nil || t == nil {
		return "", nil, nil
	}
	actual, err := repo.ListActualSegments(t.TaskID)
	if err != nil {
		return "", nil, err
	}
	if len(actual) > 0 {
		return model.SegKindActual, actual, nil
	}
	all, err := repo.ListSegments(t.TaskID)
	if err != nil {
		return "", nil, err
	}
	var plan []model.WorkTaskSegment
	for _, s := range all {
		if model.NormalizeSegmentKind(s.Kind) == model.SegKindPlan && !strings.HasPrefix(s.SegmentID, "virtual:") {
			plan = append(plan, s)
		}
	}
	return model.SegKindPlan, plan, nil
}

func replaceVisibleSegments(repo *repository.WBRepo, taskID, kind string, segs []model.WorkTaskSegment) error {
	if model.NormalizeSegmentKind(kind) == model.SegKindActual {
		return repo.ReplaceActualSegments(taskID, segs)
	}
	return repo.ReplacePlanSegments(taskID, segs)
}

func segmentsHaveOpen(segs []model.WorkTaskSegment) bool {
	for _, s := range segs {
		if segmentStillOpen(s) {
			return true
		}
	}
	return false
}

func segmentStillOpen(s model.WorkTaskSegment) bool {
	return strings.TrimSpace(s.StartTime) != "" && strings.TrimSpace(s.EndTime) == ""
}

func segSpan(s model.WorkTaskSegment) string {
	return strings.TrimSpace(s.WorkDate) + " " + strings.TrimSpace(s.StartTime) + "~" + strings.TrimSpace(s.EndTime)
}

func (h *WorkboardHandler) redirectSeg(c echo.Context, taskID, query string) error {
	return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+taskID+"?"+query)
}

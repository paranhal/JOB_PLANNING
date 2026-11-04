package handler

import (
	"errors"
	"fmt"
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
	StartLocal string
	EndLocal   string
	Worker     string
	Label      string
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
	var rows []workSegRow
	total := 0
	for i, s := range src {
		label := fmt.Sprintf("%d/%d", i+1, len(src))
		rows = append(rows, workSegRow{
			StartLocal: datetimeLocalFromSeg(firstNonEmpty(s.WorkDate, day), s.StartTime),
			EndLocal:   datetimeLocalFromSeg(firstNonEmpty(s.WorkDate, day), s.EndTime),
			Worker:     firstNonEmpty(s.WorkerName, s.Assignee, t.Assignee),
			Label:      label,
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

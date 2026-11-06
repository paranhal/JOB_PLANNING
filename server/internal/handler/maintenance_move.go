package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

// moveVisitDate 방문 날짜를 바꾼다. UpdateVisit 와 POST /move 가 같이 쓴다.
// 담당자·회차·메모·제품·사이트·그룹은 v 에 이미 있는 값을 유지한다.
func (h *MaintenanceHandler) moveVisitDate(c echo.Context, v *model.MaintenanceVisit, newDate string) error {
	if v == nil {
		return echo.NewHTTPError(http.StatusNotFound, "방문을 찾을 수 없습니다")
	}
	date, err := model.ParseAppDate(newDate)
	if err != nil {
		return err
	}
	if date == "" {
		return errors.New("날짜 형식이 아닙니다")
	}
	v.VisitDate = date
	if err := h.repo.UpdateVisit(*v); err != nil {
		return err
	}
	if c != nil {
		uid, name, on := stampOnBehalf(c, "", v.Assignee)
		_ = h.repo.StampVisitActedBy(v.VisitID, uid, name, on)
		h.recordVisitAssign(c, v.VisitID, v.Assignee, "", "", "", "")
	}
	h.syncVisitTask(v.VisitID, v.Completed)
	if v.PlanID != "" {
		h.repo.TouchPlanUpdated(v.PlanID)
	}
	return nil
}

func (h *MaintenanceHandler) visitHasOpenLive(visitID string) bool {
	if h == nil || h.wbRepo == nil || strings.TrimSpace(visitID) == "" {
		return false
	}
	t, err := h.wbRepo.GetTaskBySource(model.WBSourceMaintenance, visitID)
	if err != nil || t == nil {
		return false
	}
	type openLister interface {
		ListOpenActualForTask(taskID, workerID string) ([]model.WorkTaskSegment, error)
	}
	lister, ok := any(h.wbRepo).(openLister)
	if !ok {
		return false
	}
	opens, err := lister.ListOpenActualForTask(t.TaskID, "")
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return false
		}
		return false
	}
	return len(opens) > 0
}

func (h *MaintenanceHandler) openLiveVisitIDs(visits []model.MaintenanceVisit) map[string]bool {
	out := map[string]bool{}
	if h == nil || h.wbRepo == nil {
		return out
	}
	var ids []string
	for _, v := range visits {
		if !v.Completed {
			ids = append(ids, v.VisitID)
		}
	}
	if len(ids) == 0 {
		return out
	}
	bySrc, err := h.wbRepo.ListTasksBySourceIDs(model.WBSourceMaintenance, ids)
	if err != nil {
		return out
	}
	taskIDs := make([]string, 0, len(bySrc))
	visitOf := map[string]string{}
	for vid, t := range bySrc {
		if t == nil || strings.TrimSpace(t.TaskID) == "" {
			continue
		}
		taskIDs = append(taskIDs, t.TaskID)
		visitOf[t.TaskID] = vid
	}
	open, err := h.wbRepo.ListOpenActualTaskIDs(taskIDs)
	if err != nil {
		return out
	}
	for taskID := range open {
		if vid := visitOf[taskID]; vid != "" {
			out[vid] = true
		}
	}
	return out
}

func (h *MaintenanceHandler) visitMoveWarnings(v model.MaintenanceVisit, date string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	if t, err := time.Parse("2006-01-02", date); err == nil && t.Weekday() == time.Sunday {
		add("휴일/휴가일입니다")
	}
	if ok, _ := service.IsHoliday(date); ok {
		add("휴일/휴가일입니다")
	}
	if h != nil && h.holidayRepo != nil {
		if holi := loadHolidays(h.holidayRepo, date, date); strings.TrimSpace(holi[date].Name) != "" {
			add("휴일/휴가일입니다")
		}
	}
	if h != nil && h.repo != nil && h.repo.Leaves() != nil {
		items, _ := h.repo.Leaves().ListInRange(date, date)
		names := map[string]bool{}
		for _, n := range visitAssigneeNames(v) {
			names[strings.ToLower(strings.TrimSpace(n))] = true
		}
		for _, it := range items {
			if names[strings.ToLower(strings.TrimSpace(it.UserName))] {
				add("휴일/휴가일입니다")
				break
			}
		}
	}
	if h != nil && h.repo != nil && v.PlanID != "" {
		if plan, err := h.repo.GetPlan(v.PlanID); err == nil && plan != nil && plan.PlanYear > 0 {
			if y, err := strconv.Atoi(date[:4]); err == nil && y != plan.PlanYear {
				add("계획 연도 밖입니다")
			}
		}
	}
	return out
}

func jsonMoveErr(c echo.Context, status int, code, msg string) error {
	return c.JSON(status, map[string]any{"error": msg, "code": code})
}

// MoveVisit 캘린더에서 방문 칩을 다른 날짜로 옮긴다.
func (h *MaintenanceHandler) MoveVisit(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return jsonMoveErr(c, http.StatusForbidden, "forbidden", "권한이 없습니다")
	}
	vid := strings.TrimSpace(c.Param("visit_id"))
	existing, err := h.repo.GetVisit(vid)
	if err != nil || existing == nil {
		return jsonMoveErr(c, http.StatusNotFound, "not_found", "방문을 찾을 수 없습니다")
	}
	plan, err := h.requireVisiblePlan(c, existing.PlanID)
	if err != nil || plan == nil {
		return jsonMoveErr(c, http.StatusNotFound, "not_found", "방문을 찾을 수 없습니다")
	}
	date, err := model.ParseAppDate(c.FormValue("date"))
	if err != nil {
		code := "bad_date"
		if errors.Is(err, model.ErrAppDateYear) {
			code = "bad_year"
		}
		return jsonMoveErr(c, http.StatusBadRequest, code, err.Error())
	}
	if date == "" {
		return jsonMoveErr(c, http.StatusBadRequest, "bad_date", "날짜 형식이 아닙니다")
	}
	from := strings.TrimSpace(existing.VisitDate)
	if from == date {
		return c.JSON(http.StatusOK, map[string]any{
			"ok": true, "visit_id": existing.VisitID, "date": date, "from": from, "warnings": []string{},
		})
	}
	if existing.Completed {
		return jsonMoveErr(c, http.StatusConflict, "visit_completed", "완료된 방문은 옮길 수 없습니다")
	}
	if h.visitHasOpenLive(existing.VisitID) {
		return jsonMoveErr(c, http.StatusConflict, "visit_in_progress", "작업 중인 방문은 옮길 수 없습니다")
	}
	v := *existing
	if err := h.moveVisitDate(c, &v, date); err != nil {
		var dup repository.MonthDupError
		if errors.As(err, &dup) {
			return jsonMoveErr(c, http.StatusBadRequest, "dup", err.Error())
		}
		return jsonMoveErr(c, http.StatusBadRequest, "update", err.Error())
	}
	got, _ := h.repo.GetVisit(vid)
	if got == nil {
		got = &v
	}
	return c.JSON(http.StatusOK, map[string]any{
		"ok":       true,
		"visit_id": got.VisitID,
		"date":     got.VisitDate,
		"from":     from,
		"warnings": h.visitMoveWarnings(*got, got.VisitDate),
	})
}

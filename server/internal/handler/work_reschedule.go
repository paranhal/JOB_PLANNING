package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func RescheduleOccurrences(repo *repository.WBRepo, parent *model.WorkTask, rule model.WorkRecurrence, dates []string, mode, today string) (model.OccurrenceGenerateResult, error) {
	var empty model.OccurrenceGenerateResult
	if repo == nil || parent == nil {
		return empty, fmt.Errorf("상위 업무가 없습니다")
	}
	mode = model.NormalizeRecurrenceChangeMode(mode)
	return repo.ApplyOccurrenceChange(parent, rule, dates, mode, today)
}

func (h *WorkboardHandler) Reschedule(c echo.Context) error {
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanEditTask(c, t); err != nil {
		return err
	}
	loc := "/workboard/tasks/" + t.TaskID + "/edit"
	rule := parseRecurrenceForm(c)
	dates := rule.ManualDates
	if len(dates) == 0 {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_dates")
	}
	mode := model.NormalizeRecurrenceChangeMode(c.FormValue("change_mode"))
	today := strings.TrimSpace(c.FormValue("today"))
	if today == "" {
		today = c.QueryParam("date")
	}
	res, err := RescheduleOccurrences(h.repo, t, rule, dates, mode, today)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_rule")
	}
	_ = res
	return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID+"?ok=rec_gen")
}

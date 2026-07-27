package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

type WorkStatusHandler struct {
	repo *repository.WorkStatusRepo
}

func NewWorkStatusHandler(repo *repository.WorkStatusRepo) *WorkStatusHandler {
	return &WorkStatusHandler{repo: repo}
}

// Calendar 업무처리현황 월간 캘린더
func (h *WorkStatusHandler) Calendar(c echo.Context) error {
	now := time.Now()
	year, _ := strconv.Atoi(c.QueryParam("year"))
	month, _ := strconv.Atoi(c.QueryParam("month"))
	if year < 2000 || year > 2100 {
		year = now.Year()
	}
	if month < 1 || month > 12 {
		month = int(now.Month())
	}

	category := c.QueryParam("category")
	switch category {
	case "as", "maintenance", "other":
	default:
		category = "as"
	}
	phase := c.QueryParam("phase")
	switch phase {
	case "receipt", "visit", "complete":
	default:
		phase = "receipt"
	}

	summary, err := h.repo.MonthSummary(year, month, category)
	if err != nil {
		return err
	}
	items, err := h.repo.ListMonthItems(year, month, category, phase)
	if err != nil {
		return err
	}
	days := repository.BuildCalendar(year, month, items)

	prevY, prevM := year, month-1
	if prevM < 1 {
		prevY, prevM = year-1, 12
	}
	nextY, nextM := year, month+1
	if nextM > 12 {
		nextY, nextM = year+1, 1
	}

	return c.Render(http.StatusOK, "work_status/calendar.html", map[string]interface{}{
		"Title":      "업무처리현황",
		"Active":     "work_status",
		"Year":       year,
		"Month":      month,
		"Category":   category,
		"Phase":      phase,
		"Summary":    summary,
		"Days":       days,
		"PrevYear":   prevY,
		"PrevMonth":  prevM,
		"NextYear":   nextY,
		"NextMonth":  nextM,
		"MonthLabel": fmt.Sprintf("%d년 %02d월", year, month),
	})
}

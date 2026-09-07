package handler

import (
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func parseLookback(c echo.Context, baseDate string, now time.Time) model.StatsLookback {
	rangeQ := strings.TrimSpace(c.QueryParam("range"))
	unit := strings.TrimSpace(c.QueryParam("range_unit"))
	n := strings.TrimSpace(c.QueryParam("range_n"))
	if rangeQ == "" && n != "" && unit != "" && unit != model.LookbackUnitCustom {
		if unit == "day" {
			unit = model.LookbackUnitDay
		}
		if unit == "week" {
			unit = model.LookbackUnitWeek
		}
		if unit == "month" {
			unit = model.LookbackUnitMonth
		}
		rangeQ = n + unit
	}
	return model.ParseStatsLookback(
		rangeQ,
		c.QueryParam("from"),
		c.QueryParam("to"),
		c.QueryParam("view"),
		unit,
		baseDate,
		now,
	)
}

func lookbackTimes(lb model.StatsLookback, now time.Time) (fromIncl, toIncl time.Time) {
	loc := now.Location()
	fromIncl, toIncl = now, now
	if t, err := time.ParseInLocation("2006-01-02", lb.From, loc); err == nil {
		fromIncl = t
	}
	if t, err := time.ParseInLocation("2006-01-02", lb.To, loc); err == nil {
		toIncl = t
	}
	return fromIncl, toIncl
}

func lookbackRequest(c echo.Context) bool {
	if strings.TrimSpace(c.QueryParam("range")) != "" {
		return true
	}
	if strings.TrimSpace(c.QueryParam("range_unit")) == model.LookbackUnitCustom {
		return true
	}
	if strings.TrimSpace(c.QueryParam("range_n")) != "" && strings.TrimSpace(c.QueryParam("range_unit")) != "" {
		return true
	}
	from := strings.TrimSpace(c.QueryParam("from"))
	to := strings.TrimSpace(c.QueryParam("to"))
	if from == "" || to == "" {
		return false
	}
	p := strings.TrimSpace(c.QueryParam("period"))
	return p != model.StatsPeriodWeek && p != model.StatsPeriodMonth
}

func parseLookbackReportPeriod(c echo.Context, now time.Time, baseDate string) reportPeriod {
	if !lookbackRequest(c) {
		return parseReportPeriod(c, now)
	}
	lb := parseLookback(c, baseDate, now)
	fromIncl, toIncl := lookbackTimes(lb, now)
	toEx := toIncl.AddDate(0, 0, 1)
	return reportPeriod{
		Period:  model.StatsPeriodRange,
		From:    fromIncl,
		ToEx:    toEx,
		FromStr: fromIncl.Format("2006-01-02"),
		ToStr:   toIncl.Format("2006-01-02"),
		ToExStr: toEx.Format("2006-01-02"),
		FileDay: toIncl.Format("20060102"),
	}
}

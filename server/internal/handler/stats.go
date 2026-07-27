package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type StatsHandler struct {
	repo *repository.StatsRepo
}

func NewStatsHandler(repo *repository.StatsRepo) *StatsHandler {
	return &StatsHandler{repo: repo}
}

var statsPeriodOptions = []struct {
	Value string
	Label string
}{
	{model.StatsPeriodAll, "전체"},
	{model.StatsPeriodDay, "일별"},
	{model.StatsPeriodWeek, "주간별"},
	{model.StatsPeriodMonth, "월별"},
	{model.StatsPeriodQuarter, "분기별"},
}

func normalizeStatsPeriod(p string) string {
	switch p {
	case model.StatsPeriodDay, model.StatsPeriodWeek, model.StatsPeriodMonth, model.StatsPeriodQuarter:
		return p
	default:
		return model.StatsPeriodAll
	}
}

func statsPeriodLabel(p string) string {
	for _, o := range statsPeriodOptions {
		if o.Value == p {
			return o.Label
		}
	}
	return "전체"
}

func statsOffsetOptions(period string) []struct {
	Value int
	Label string
} {
	switch period {
	case model.StatsPeriodDay:
		return []struct {
			Value int
			Label string
		}{
			{0, "현재 기준일"},
			{1, "전일"},
			{2, "전전일"},
		}
	case model.StatsPeriodWeek:
		return []struct {
			Value int
			Label string
		}{
			{0, "현재 주"},
			{1, "전주"},
			{2, "전전주"},
		}
	case model.StatsPeriodMonth:
		return []struct {
			Value int
			Label string
		}{
			{0, "현재 월"},
			{1, "전월"},
			{2, "전전월"},
		}
	case model.StatsPeriodQuarter:
		return []struct {
			Value int
			Label string
		}{
			{0, "현재 분기"},
			{1, "전분기"},
			{2, "전전분기"},
		}
	default:
		return nil
	}
}

func statsOffsetLabel(period string, offset int) string {
	for _, o := range statsOffsetOptions(period) {
		if o.Value == offset {
			return o.Label
		}
	}
	return ""
}

func (h *StatsHandler) List(c echo.Context) error {
	period := normalizeStatsPeriod(c.QueryParam("period"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if period == model.StatsPeriodAll {
		offset = 0
	} else {
		offset = repository.NormalizeStatsOffset(offset)
	}

	rows, err := h.repo.ListDetail(period, offset)
	if err != nil {
		return err
	}
	assignees, err := h.repo.ListByAssignee(period, offset)
	if err != nil {
		return err
	}
	total := 0
	for _, a := range assignees {
		total += a.Count
	}

	now := time.Now()
	rangeLabels := repository.PeriodDisplayLabels(period, offset, now)

	return c.Render(http.StatusOK, "stats/list.html", map[string]interface{}{
		"Title":          "통계",
		"Active":         "stats",
		"Period":         period,
		"PeriodLabel":    statsPeriodLabel(period),
		"PeriodOptions":  statsPeriodOptions,
		"Offset":         offset,
		"OffsetLabel":    statsOffsetLabel(period, offset),
		"OffsetOptions":  statsOffsetOptions(period),
		"RangeLabels":    rangeLabels,
		"ShowOffset":     period != model.StatsPeriodAll,
		"Rows":           rows,
		"Assignees":      assignees,
		"AssigneeTotal":  total,
		"RowCount":       len(rows),
	})
}

func (h *StatsHandler) ExportExcel(c echo.Context) error {
	f := excelize.NewFile()
	defer f.Close()

	periods := []string{
		model.StatsPeriodDay,
		model.StatsPeriodWeek,
		model.StatsPeriodMonth,
		model.StatsPeriodQuarter,
		model.StatsPeriodAll,
	}
	first := true
	for _, p := range periods {
		sheet := statsPeriodLabel(p)
		if first {
			f.SetSheetName("Sheet1", sheet)
			first = false
		} else {
			f.NewSheet(sheet)
		}
		rows, err := h.repo.ListDetail(p, 0)
		if err != nil {
			return err
		}
		assignees, err := h.repo.ListByAssignee(p, 0)
		if err != nil {
			return err
		}
		if err := writeStatsSheet(f, sheet, rows, assignees); err != nil {
			return err
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	return writeExcelDownload(c, buf.Bytes(), "stats")
}

func writeStatsSheet(f *excelize.File, sheet string, rows []model.StatsRow, assignees []model.StatsAssigneeRow) error {
	headers := []string{
		"제품분류", "제품모델명", "업무형태", "접수형태", "거래처명",
		"수행담당자", "접수내용", "처리내용", "처리상태",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for r, row := range rows {
		vals := []interface{}{
			row.ProductCategory, row.ProductModel, row.WorkForm, row.ReceiptForm, row.CustomerName,
			row.Assignee, row.Symptom, row.ActionTaken, row.StatusLabel,
		}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	start := len(rows) + 4
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", start), "담당자별")
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", start+1), "수행담당자")
	_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", start+1), "건수")
	total := 0
	for i, a := range assignees {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", start+2+i), a.Assignee)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", start+2+i), a.Count)
		total += a.Count
	}
	sumRow := start + 2 + len(assignees)
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", sumRow), "종합")
	_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", sumRow), total)
	return nil
}

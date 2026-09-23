package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/repository"
)

func (h *SalesHandler) SalesLogs(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	f := parseSalesLogFilter(c)
	rows, err := h.repo.ListSalesLogs(f)
	if err != nil {
		return err
	}
	if strings.HasSuffix(c.Request().URL.Path, ".xlsx") || strings.TrimSpace(c.QueryParam("format")) == "xlsx" {
		return writeSalesLogsXLSX(c, f, rows)
	}
	return c.Render(http.StatusOK, "admin/sales_logs.html", map[string]interface{}{
		"Title": "영업 로그 관리", "Active": NavSalesLogs,
		"Filter": f, "Rows": rows,
		"Kind": f.Kind, "Search": f.Search, "User": f.User, "From": f.From, "To": f.To,
	})
}

func parseSalesLogFilter(c echo.Context) repository.SalesLogFilter {
	from := strings.TrimSpace(c.QueryParam("from"))
	to := strings.TrimSpace(c.QueryParam("to"))
	if from == "" {
		from = time.Now().AddDate(0, -3, 0).Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	return repository.SalesLogFilter{
		From: from, To: to,
		Search: strings.TrimSpace(c.QueryParam("q")),
		User:   strings.TrimSpace(c.QueryParam("user")),
		Kind:   strings.TrimSpace(c.QueryParam("kind")),
		Limit:  500,
	}
}

func writeSalesLogsXLSX(c echo.Context, f repository.SalesLogFilter, rows []repository.SalesLogRow) error {
	xf := excelize.NewFile()
	sheet := "로그"
	xf.SetSheetName("Sheet1", sheet)
	_ = xf.SetCellValue(sheet, "A1", fmt.Sprintf("영업 로그 · %s~%s · %s", f.From, f.To, time.Now().Format("2006-01-02 15:04")))
	heads := []string{"언제", "누가", "종류", "번호", "이름", "동작", "항목", "전", "후", "사유"}
	for i, h := range heads {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = xf.SetCellValue(sheet, cell, h)
	}
	r := 3
	for _, row := range rows {
		if len(row.Diffs) == 0 {
			cell := func(c int) string { s, _ := excelize.CoordinatesToCellName(c, r); return s }
			_ = xf.SetCellValue(sheet, cell(1), row.OccurredAt)
			_ = xf.SetCellValue(sheet, cell(2), row.UserName)
			_ = xf.SetCellValue(sheet, cell(3), row.KindLabel)
			_ = xf.SetCellValue(sheet, cell(4), row.WhatNo)
			_ = xf.SetCellValue(sheet, cell(5), row.WhatName)
			_ = xf.SetCellValue(sheet, cell(6), row.ActionLabel)
			_ = xf.SetCellValue(sheet, cell(10), row.Reason)
			r++
			continue
		}
		for _, d := range row.Diffs {
			cell := func(c int) string { s, _ := excelize.CoordinatesToCellName(c, r); return s }
			_ = xf.SetCellValue(sheet, cell(1), row.OccurredAt)
			_ = xf.SetCellValue(sheet, cell(2), row.UserName)
			_ = xf.SetCellValue(sheet, cell(3), row.KindLabel)
			_ = xf.SetCellValue(sheet, cell(4), row.WhatNo)
			_ = xf.SetCellValue(sheet, cell(5), row.WhatName)
			_ = xf.SetCellValue(sheet, cell(6), row.ActionLabel)
			_ = xf.SetCellValue(sheet, cell(7), d.Field)
			_ = xf.SetCellValue(sheet, cell(8), d.From)
			_ = xf.SetCellValue(sheet, cell(9), d.To)
			_ = xf.SetCellValue(sheet, cell(10), row.Reason)
			r++
		}
	}
	_ = xf.AutoFilter(sheet, "A2:J2", nil)
	_ = xf.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: true, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft"})
	c.Response().Header().Set(echo.HeaderContentType, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="영업로그.xlsx"`)
	_ = xf.Write(c.Response())
	return nil
}

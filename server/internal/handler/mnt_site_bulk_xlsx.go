package handler

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func (h *MaintenanceHandler) SiteBulkSkipExcel(c echo.Context) error {
	if viewAsActive(c) {
		return bulkForbidden(c)
	}
	skips := readSiteBulkSkipCookie(c)
	f, err := BuildSiteBulkSkipWorkbook(skips)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	day := time.Now().Format("20060102")
	return writeExcelFile(c, buf.Bytes(), day+"_mnt_site_bulk_skips.xlsx", day+"_점검사이트_일괄등록_건너뜀.xlsx")
}

func BuildSiteBulkSkipWorkbook(skips []model.SiteBulkSkip) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := "건너뜀"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	head, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Family: "맑은 고딕", Color: xlColor("FFFFFFFF")},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{xlColor("FF1E3A5F")}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	warn, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 10, Family: "맑은 고딕", Color: xlColor("FFB91C1C")},
	})
	if err != nil {
		return nil, err
	}
	headers := []string{"고객ID", "이름", "사유"}
	for i, hname := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, hname)
		_ = f.SetCellStyle(sheet, cell, cell, head)
	}
	for r, s := range skips {
		row := r + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), s.CustomerID)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), s.Name)
		cell := fmt.Sprintf("C%d", row)
		_ = f.SetCellValue(sheet, cell, s.Reason)
		_ = f.SetCellStyle(sheet, cell, cell, warn)
	}
	_ = f.SetColWidth(sheet, "A", "A", 18)
	_ = f.SetColWidth(sheet, "B", "B", 24)
	_ = f.SetColWidth(sheet, "C", "C", 22)
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")
	return f, nil
}

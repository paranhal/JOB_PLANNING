package handler

import (
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/repository"
)

func (h *SpaceHandler) ExportExcel(c echo.Context) error {
	orgID := currentOrg(c)
	q := strings.TrimSpace(c.QueryParam("q"))
	customerID := strings.TrimSpace(c.QueryParam("customer_id"))
	sort := strings.TrimSpace(c.QueryParam("sort"))
	dir := strings.TrimSpace(c.QueryParam("dir"))
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}
	switch sort {
	case "org", "building", "floor", "room":
	default:
		sort = ""
	}
	includeInactive := c.QueryParam("include_inactive") == "1"

	rows, _, _, err := h.repo.ListSpaceRows(orgID, customerID, q, sort, dir, includeInactive, 0, 0)
	if err != nil {
		return err
	}
	stats, err := h.repo.SpaceStats(orgID, customerID, q, includeInactive)
	if err != nil {
		return err
	}
	f, err := buildSpaceExcelWorkbook(rows, stats)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	return writeExcelDownload(c, buf.Bytes(), "spaces")
}

func buildSpaceExcelWorkbook(rows []repository.SpaceRow, stats []repository.SpaceStatRow) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}
	if err := f.SetSheetName(sheet, "공간"); err != nil {
		return nil, err
	}
	hdrStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Family: "맑은 고딕"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFDBEAFE"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}

	spaceHeaders := []string{"기관명", "건물명", "층", "호/실명"}
	for i, h := range spaceHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("공간", cell, h)
	}
	_ = f.SetCellStyle("공간", "A1", "D1", hdrStyle)
	_ = f.SetRowHeight("공간", 1, 22)
	for i, r := range rows {
		row := i + 2
		vals := []interface{}{r.OrgName, r.BuildingName, r.FloorName, r.RoomName}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			_ = f.SetCellValue("공간", cell, v)
		}
	}
	_ = f.SetColWidth("공간", "A", "D", 22)
	_ = f.SetPanes("공간", &excelize.Panes{
		Freeze: true, Split: false, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})

	if _, err := f.NewSheet("기관별 집계"); err != nil {
		return nil, err
	}
	statHeaders := []string{"기관명", "건물수", "층수", "호/실 개수"}
	for i, h := range statHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("기관별 집계", cell, h)
	}
	_ = f.SetCellStyle("기관별 집계", "A1", "D1", hdrStyle)
	_ = f.SetRowHeight("기관별 집계", 1, 22)
	for i, s := range stats {
		row := i + 2
		a, _ := excelize.CoordinatesToCellName(1, row)
		b, _ := excelize.CoordinatesToCellName(2, row)
		c, _ := excelize.CoordinatesToCellName(3, row)
		d, _ := excelize.CoordinatesToCellName(4, row)
		_ = f.SetCellValue("기관별 집계", a, s.OrgName)
		_ = f.SetCellValue("기관별 집계", b, s.Buildings)
		_ = f.SetCellValue("기관별 집계", c, s.Floors)
		_ = f.SetCellValue("기관별 집계", d, s.Rooms)
	}
	_ = f.SetColWidth("기관별 집계", "A", "A", 22)
	_ = f.SetColWidth("기관별 집계", "B", "D", 12)
	_ = f.SetPanes("기관별 집계", &excelize.Panes{
		Freeze: true, Split: false, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})
	return f, nil
}

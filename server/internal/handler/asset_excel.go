package handler

import (
	"fmt"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

// ExportExcel 설치자산 목록 엑셀 (현재 검색·기관·정렬 반영)
func (h *AssetHandler) ExportExcel(c echo.Context) error {
	search := strings.TrimSpace(c.QueryParam("search"))
	customerID := strings.TrimSpace(c.QueryParam("customer_id"))
	projectID := strings.TrimSpace(c.QueryParam("project_id"))
	category := strings.TrimSpace(c.QueryParam("category"))
	sort := strings.TrimSpace(c.QueryParam("sort"))
	dir := strings.TrimSpace(c.QueryParam("dir"))
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}
	items, err := h.repo.ListExport(customerID, search, projectID, category, sort, dir)
	if err != nil {
		return err
	}
	f, err := buildAssetExcelWorkbook(items)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	return writeExcelDownload(c, buf.Bytes(), "assets")
}

func buildAssetExcelWorkbook(items []model.Asset) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}
	if err := f.SetSheetName(sheet, "설치자산"); err != nil {
		return nil, err
	}
	sheet = "설치자산"

	headers := []string{
		"자산번호", "기관", "제품명", "제품분류", "구분", "모델명", "제조사", "S/N",
		"위치", "설치위치", "계약구분", "점검주기", "상태", "설치일", "설치연수", "AS건수", "사업(프로젝트)",
	}
	hdrStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Family: "맑은 고딕"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFDBEAFE"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	lastCol, _ := excelize.ColumnNumberToName(len(headers))
	_ = f.SetCellStyle(sheet, "A1", lastCol+"1", hdrStyle)
	_ = f.SetRowHeight(sheet, 1, 22)

	for i, a := range items {
		row := i + 2
		loc := strings.TrimSpace(strings.Join([]string{a.BuildingName, a.FloorName, a.RoomName}, " "))
		yrs := ""
		if a.InstallYears > 0 {
			yrs = fmt.Sprintf("%d", a.InstallYears)
		}
		vals := []interface{}{
			a.AssetID, a.OrgName, a.ProductName,
			excelProductCategoryLabel(a.ProductCategory),
			strings.ToUpper(a.ProductType), a.ModelName, a.Manufacturer, a.SerialNumber,
			loc, a.InstallLocation,
			excelMaintContractLabel(a.MaintContractType),
			excelMaintCycleLabel(a.MaintCycle),
			excelOpStatusLabel(a.OperationStatus),
			a.InstallDate, yrs, a.AsCount, a.ProjectName,
		}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	widths := []float64{18, 22, 22, 12, 10, 14, 12, 16, 20, 14, 10, 10, 10, 12, 10, 8, 22}
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, w)
	}
	return f, nil
}

func excelProductCategoryLabel(s string) string {
	m := map[string]string{
		"homepage": "홈페이지", "elibrary": "전자도서관", "mobile": "모바일",
		"rfid": "RFID자동화", "materials": "자료관리", "other": "기타",
	}
	if l, ok := m[s]; ok {
		return l
	}
	return s
}

func excelOpStatusLabel(s string) string {
	m := map[string]string{
		"operating": "운영중", "maintenance": "점검중", "fault": "장애",
		"retired": "철수", "disposed": "폐기",
	}
	if l, ok := m[s]; ok {
		return l
	}
	return s
}

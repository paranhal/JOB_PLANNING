package handler

import (
	"bytes"
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func (h *ProjectHandler) EquipTemplateXLSX(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	cats, _ := h.pe.ListCategories(org, false)
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName(f.GetSheetName(0), "사이트")
	_, _ = f.NewSheet("장비")
	hdrFill, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Family: "맑은 고딕", Color: xlColor("FFFFFF")},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{xlColor("1E3A5F")}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	siteHdr := []string{"사이트명", "지역", "주소", "상태", "비고"}
	equipHdr := []string{"사이트명", "분류코드", "제품명", "모델", "제조사", "수량", "시리얼", "설치위치", "설치일", "상태", "연차번호", "비고"}
	for _, c := range cats {
		for _, fld := range c.Fields {
			equipHdr = append(equipHdr, fld.FieldKey)
		}
	}
	writeHdr := func(sheet string, cols []string) {
		for i, htxt := range cols {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			_ = f.SetCellValue(sheet, cell, htxt)
		}
		last, _ := excelize.ColumnNumberToName(len(cols))
		_ = f.SetCellStyle(sheet, "A1", last+"1", hdrFill)
		_ = f.SetRowHeight(sheet, 1, 22)
	}
	writeHdr("사이트", siteHdr)
	writeHdr("장비", equipHdr)
	_ = f.SetCellValue("사이트", "A2", "예시사이트")
	_ = f.SetCellValue("사이트", "D2", "예정")
	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	_ = p
	ascii := fmt.Sprintf("sites_equip_%s.xlsx", p.ProjectID)
	return writeExcelFile(c, buf.Bytes(), ascii, "사이트장비_양식.xlsx")
}

func buildEquipErrorExcel(prev *model.EquipImportPreview) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName(f.GetSheetName(0), "오류")
	hdrFill, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 11, Family: "맑은 고딕", Color: xlColor("FFFFFF")},
		Fill: excelize.Fill{Type: "pattern", Color: []string{xlColor("B91C1C")}, Pattern: 1},
	})
	for i, h := range []string{"시트", "행", "구분", "키", "사유"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue("오류", cell, h)
	}
	_ = f.SetCellStyle("오류", "A1", "E1", hdrFill)
	r := 2
	if prev != nil {
		for _, row := range prev.Rows {
			if row.Action != "err" {
				continue
			}
			_ = f.SetCellValue("오류", fmt.Sprintf("A%d", r), row.Sheet)
			_ = f.SetCellValue("오류", fmt.Sprintf("B%d", r), row.Line)
			_ = f.SetCellValue("오류", fmt.Sprintf("C%d", r), row.Kind)
			_ = f.SetCellValue("오류", fmt.Sprintf("D%d", r), row.Key)
			_ = f.SetCellValue("오류", fmt.Sprintf("E%d", r), row.Msg)
			r++
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildEquipWorkbookBytes(sites, equips [][]string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	_ = f.SetSheetName(f.GetSheetName(0), "사이트")
	_, _ = f.NewSheet("장비")
	put := func(sheet string, rows [][]string) {
		for i, row := range rows {
			for j, v := range row {
				cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
				_ = f.SetCellValue(sheet, cell, v)
			}
		}
	}
	put("사이트", sites)
	put("장비", equips)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

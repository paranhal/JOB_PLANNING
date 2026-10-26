package repository

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

var productGroupExcelHeaders = []string{"고유번호", "제품분류", "제품구분", "세부제품군"}

func ProductGroupTemplateXLSX() ([]byte, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for i, h := range productGroupExcelHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	sample := [][]string{
		{"", "HW", "네트워크장비", "L2스위치"},
	}
	for r, row := range sample {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ProductGroupExportXLSX(items []model.ProductGroup) ([]byte, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for i, h := range productGroupExcelHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for r, g := range items {
		vals := []string{g.GroupID, g.ClassName, g.TypeName, g.DetailLabel()}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type ProductGroupImportRow struct {
	Line       int
	GroupID    string
	ClassName  string
	TypeName   string
	DetailName string
	Msg        string
	Err        bool
}

func (r *ProductGroupRepo) PreviewExcel(raw []byte) ([]ProductGroupImportRow, error) {
	return r.walkExcel(raw, false)
}

func (r *ProductGroupRepo) ImportExcel(raw []byte) (ok int, errs []string, err error) {
	rows, err := r.walkExcel(raw, true)
	if err != nil {
		return 0, nil, err
	}
	for _, row := range rows {
		if row.Err {
			errs = append(errs, strconv.Itoa(row.Line)+"행: "+row.Msg)
			continue
		}
		ok++
	}
	return ok, errs, nil
}

func (r *ProductGroupRepo) walkExcel(raw []byte, apply bool) ([]ProductGroupImportRow, error) {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("엑셀을 열 수 없습니다")
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("데이터 행이 없습니다")
	}
	idx := map[string]int{}
	for i, h := range rows[0] {
		idx[strings.TrimSpace(h)] = i
	}
	col := func(row []string, name string) string {
		i, ok := idx[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	var out []ProductGroupImportRow
	for n, row := range rows[1:] {
		line := n + 2
		className := col(row, "제품분류")
		typeName := col(row, "제품구분")
		detail := col(row, "세부제품군")
		gid := col(row, "고유번호")
		if className == "" && typeName == "" {
			continue
		}
		ir := ProductGroupImportRow{Line: line, GroupID: gid, ClassName: className, TypeName: typeName, DetailName: detail}
		if r.FindClass(className) == "" || typeName == "" {
			ir.Err = true
			ir.Msg = "제품분류·제품구분이 필요합니다"
			out = append(out, ir)
			continue
		}
		if apply {
			id, e := r.ImportRow(gid, className, typeName, detail)
			if e != nil {
				ir.Err = true
				ir.Msg = e.Error()
			} else {
				ir.GroupID = id
				ir.Msg = "반영"
			}
		} else {
			ir.Msg = "등록 예정"
		}
		out = append(out, ir)
	}
	return out, nil
}

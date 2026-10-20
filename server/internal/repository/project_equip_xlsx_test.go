package repository

import (
	"bytes"

	"github.com/xuri/excelize/v2"
)

func buildTestEquipXLSX(sites, equips [][]string) ([]byte, error) {
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

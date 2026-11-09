package leaveimport

import "github.com/xuri/excelize/v2"

// BuildFixture makes a synthetic workbook. Do not put real staff names here.
func BuildFixture(year int) ([]byte, error) {
	f := excelize.NewFile()
	idx, err := f.NewSheet(SheetName)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")
	_, _ = f.NewSheet("다른시트")
	headers := []string{"이름", "입사일", "", "", "", "부여", "예비군보전", "", "", "남은연차", "1월", "3월"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(SheetName, cell, h)
	}
	data := [][]interface{}{
		{"가상갑", "2020-03-01", "", "", "", 15, 1, "", "", 14.5, "3/5", "3/12(반차)"},
		{"가상을", "2019-01-10", "", "", "", 10, 0, "", "", 10, "4/1 예비군", ""},
		{"동명이", "2018-05-01", "", "", "", 8, "", "", "", 8, "", ""},
		{"없는사람", "2021-01-01", "", "", "", 5, "", "", "", 5, "", ""},
		{"미등록자", "2022-02-02", "", "", "", "", "", "", "", "", "", ""},
		{"입사충돌", "2010-01-01", "", "", "", 7, "", "", "", 7, "", ""},
		{"표기오류", "2020-01-01", "", "", "", 6, "", "", "", 6, "오전반차", ""},
	}
	for r, line := range data {
		for c, v := range line {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(SheetName, cell, v)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TemplateXLSX() ([]byte, error) {
	f := excelize.NewFile()
	idx, err := f.NewSheet(SheetName)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")
	headers := []string{"이름", "입사일", "", "", "", "부여", "예비군보전", "", "", "남은연차", "1월", "2월", "3월", "4월", "5월", "6월", "7월", "8월", "9월", "10월", "11월", "12월"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(SheetName, cell, h)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

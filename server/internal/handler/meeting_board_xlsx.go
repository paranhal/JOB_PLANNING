package handler

import (
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func (h *MeetingHandler) BoardXLSX(c echo.Context) error {
	p, err := h.loadBoard(c)
	if err != nil {
		return err
	}
	if p == nil {
		return echo.ErrNotFound
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := writeBoardSummarySheet(f, p); err != nil {
		return err
	}
	if err := writeBoardDetailSheet(f, p); err != nil {
		return err
	}
	_ = f.DeleteSheet("Sheet1")
	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	day := strings.ReplaceAll(p.To, "-", "")
	return writeExcelFile(c, buf.Bytes(), day+"_meeting_board.xlsx", day+"_업무회의.xlsx")
}

func writeBoardSummarySheet(f *excelize.File, p *boardPage) error {
	name := "요약"
	if _, err := f.NewSheet(name); err != nil {
		return err
	}
	headers := []string{"구분", "이름", "건수", "담당자"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(name, cell, h)
	}
	row := 2
	write := func(kind, label string, n int, extra string) {
		_ = f.SetCellValue(name, cellName(1, row), kind)
		_ = f.SetCellValue(name, cellName(2, row), label)
		_ = f.SetCellValue(name, cellName(3, row), n)
		_ = f.SetCellValue(name, cellName(4, row), extra)
		row++
	}
	for _, g := range p.PeopleGroups {
		write("담당자별", g.Label, g.Count, "")
	}
	write("담당자별", "합계", p.PeopleN, "")
	for _, g := range p.ProjectGroups {
		write("사업별", g.Label, g.Count, g.AssigneeLine)
	}
	write("사업별", "합계", p.ProjectN, "")
	write("정기점검", "대상", p.Mnt.Target, "")
	write("정기점검", "완료", p.Mnt.Done, "")
	write("정기점검", "미완료", p.Mnt.Incomplete, "")
	write("정기점검", "지연", p.Mnt.Overdue, "")
	write("정기점검", "미배정", p.Mnt.Unassigned, "")
	write("정기점검", "집행률", 0, p.Mnt.RateLabel)
	return nil
}

func writeBoardDetailSheet(f *excelize.File, p *boardPage) error {
	name := "상세"
	if _, err := f.NewSheet(name); err != nil {
		return err
	}
	headers := []string{"담당자", "사업", "종류", "번호", "제목", "접수", "조치", "상태"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(name, cell, h)
	}
	proj := map[string]string{}
	for _, g := range p.ProjectGroups {
		for _, it := range g.Items {
			proj[model.WorkListItemKey(it)] = g.Label
		}
	}
	for i, it := range p.Items {
		r := i + 2
		_ = f.SetCellValue(name, cellName(1, r), strings.TrimSpace(it.Assignee))
		_ = f.SetCellValue(name, cellName(2, r), proj[model.WorkListItemKey(it)])
		_ = f.SetCellValue(name, cellName(3, r), model.WorkPrefixLabel(it.Prefix))
		_ = f.SetCellValue(name, cellName(4, r), it.RefNumber)
		_ = f.SetCellValue(name, cellName(5, r), it.Title)
		_ = f.SetCellValue(name, cellName(6, r), boardReceiptText(it))
		_ = f.SetCellValue(name, cellName(7, r), boardActionText(it))
		_ = f.SetCellValue(name, cellName(8, r), it.StatusLabel)
	}
	return nil
}

func cellName(col, row int) string {
	n, _ := excelize.CoordinatesToCellName(col, row)
	return n
}

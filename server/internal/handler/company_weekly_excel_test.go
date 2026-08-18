package handler

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestCompanyWeeklyPreserveFormulasAndHiddenCol(t *testing.T) {
	src := mustCompanyWeeklyFixture(t)
	period := repository.CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	in := companyWeeklyBuildInput{
		Source:    src,
		SheetName: "26.8월3주(업무)",
		Period:    period,
		Rows:      sampleCompanyWeeklyRows(),
	}
	out, err := buildCompanyWeeklyWorkbook(in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Mode != companyWeeklyModeSheetAdd {
		t.Fatalf("원본 보존 실패로 후퇴함: %s", out.FallbackReason)
	}
	if out.OrigSheets != 7 || out.KeptSheets != 7 {
		t.Fatalf("원본 시트=%d 유지=%d want 7", out.OrigSheets, out.KeptSheets)
	}
	if out.KeptFormulas < 3 {
		t.Fatalf("수식 유지=%d", out.KeptFormulas)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	names := f.GetSheetList()
	if len(names) != 8 {
		t.Fatalf("시트 수=%d %v want 8", len(names), names)
	}
	if !containsSheetName(names, "26.8월3주(업무)") {
		t.Fatalf("새 시트 없음: %v", names)
	}
	assertCompanyWeeklyBizPreserved(t, f)

	workIdx := indexOfSheet(names, "26.8월3주(업무)")
	oldIdx := indexOfSheet(names, "26.8월2주(업무)")
	if workIdx < 0 || oldIdx < 0 || workIdx != oldIdx-1 {
		t.Fatalf("삽입 위치 잘못: %v", names)
	}
}

func TestCompanyWeeklyOnlyFillsRows20to25(t *testing.T) {
	src := mustCompanyWeeklyFixture(t)
	period := repository.CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	out, err := buildCompanyWeeklyWorkbook(companyWeeklyBuildInput{
		Source: src, SheetName: "26.8월2주(업무)-새", Period: period, Rows: sampleCompanyWeeklyRows(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Mode != companyWeeklyModeSheetAdd {
		t.Fatalf("후퇴: %s", out.FallbackReason)
	}
	f, err := excelize.OpenReader(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := "26.8월2주(업무)-새"
	e20, _ := f.GetCellValue(sheet, "E20")
	if !strings.Contains(e20, "충청남도") {
		t.Fatalf("E20=%q", e20)
	}
	e3, _ := f.GetCellValue(sheet, "E3")
	a10, _ := f.GetCellValue(sheet, "A10")
	if e3 != "" || a10 != "" {
		t.Fatalf("다른 팀 행을 만들면 안 됨 E3=%q A10=%q", e3, a10)
	}
	e25, _ := f.GetCellValue(sheet, "E25")
	if e25 != "##자산관리활동" {
		t.Fatalf("E25=%q", e25)
	}
	w, err := f.GetColWidth(sheet, "A")
	if err != nil || w < 12 || w > 14 {
		t.Fatalf("A열 너비=%v err=%v", w, err)
	}
	panes, err := f.GetPanes(sheet)
	if err != nil || panes.TopLeftCell != "F20" {
		t.Fatalf("틀고정=%+v err=%v", panes, err)
	}
	props, err := f.GetSheetProps(sheet)
	if err != nil || props.TabColorRGB == nil || !strings.Contains(*props.TabColorRGB, "0000FF") {
		t.Fatalf("탭 색=%v err=%v", props.TabColorRGB, err)
	}
}

func TestCompanyWeeklyFallbackSingleSheet(t *testing.T) {
	period := repository.CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	data, err := buildCompanyWeeklySingleSheet(companyWeeklyBuildInput{
		SheetName: period.SheetNameDefault,
		Period:    period,
		Rows:      sampleCompanyWeeklyRows(),
	})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if len(f.GetSheetList()) != 1 {
		t.Fatalf("한 장짜리가 아님: %v", f.GetSheetList())
	}
	v, _ := f.GetCellValue(period.SheetNameDefault, "F20")
	if !strings.Contains(v, "정기점검") {
		t.Fatalf("F20=%q", v)
	}
}

func TestCompanyWeeklyEditedFGHIWritten(t *testing.T) {
	src := mustCompanyWeeklyFixture(t)
	period := repository.CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	rows := sampleCompanyWeeklyRows()
	rows[0].PrevText = "·전주 수정문장"
	rows[0].PlanText = "·금주 수정문장"
	rows[0].HelpText = "지원 요청"
	rows[0].DecisionText = "결정 사항"
	out, err := buildCompanyWeeklyWorkbook(companyWeeklyBuildInput{
		Source: src, SheetName: "편집검증(업무)", Period: period, Rows: rows,
	})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := "편집검증(업무)"
	f20, _ := f.GetCellValue(sheet, "F20")
	g20, _ := f.GetCellValue(sheet, "G20")
	h20, _ := f.GetCellValue(sheet, "H20")
	i20, _ := f.GetCellValue(sheet, "I20")
	if f20 != "·전주 수정문장" || g20 != "·금주 수정문장" || h20 != "지원 요청" || i20 != "결정 사항" {
		t.Fatalf("F=%q G=%q H=%q I=%q", f20, g20, h20, i20)
	}
}

func TestCompanyWeeklyDuplicateSheetNameRejected(t *testing.T) {
	src := mustCompanyWeeklyFixture(t)
	period := repository.CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	_, err := buildCompanyWeeklyWorkbook(companyWeeklyBuildInput{
		Source: src, SheetName: "26.8월2주(업무)", Period: period, Rows: sampleCompanyWeeklyRows(),
	})
	if err == nil {
		t.Fatal("같은 시트 이름을 받아야 한다")
	}
}

func assertCompanyWeeklyBizPreserved(t *testing.T, f *excelize.File) {
	t.Helper()
	for _, cell := range []string{"N3", "W3", "Y3"} {
		if !companyWeeklyCellIsFormula(f, companyWeeklyBizSheet, cell) {
			fm, _ := f.GetCellFormula(companyWeeklyBizSheet, cell)
			val, _ := f.GetCellValue(companyWeeklyBizSheet, cell, excelize.Options{RawCellValue: true})
			t.Fatalf("%s 수식 아님 formula=%q value=%q", cell, fm, val)
		}
		fm, _ := f.GetCellFormula(companyWeeklyBizSheet, cell)
		if strings.TrimSpace(fm) == "" {
			t.Fatalf("%s GetCellFormula 비어 있음", cell)
		}
	}
	vis, err := f.GetColVisible(companyWeeklyBizSheet, "AB")
	if err != nil || vis {
		t.Fatalf("AB 숨김 아님 visible=%v err=%v", vis, err)
	}
}

func mustCompanyWeeklyFixture(t *testing.T) []byte {
	t.Helper()
	b, err := buildCompanyWeeklyPreserveFixture()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func buildCompanyWeeklyPreserveFixture() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", companyWeeklyBizSheet); err != nil {
		return nil, err
	}
	_ = f.SetCellValue(companyWeeklyBizSheet, "A3", "x")
	_ = f.SetCellValue(companyWeeklyBizSheet, "B3", "y")
	if err := f.SetCellFormula(companyWeeklyBizSheet, "N3", "VLOOKUP(A3,Ref!A:B,2,FALSE)"); err != nil {
		return nil, err
	}
	if err := f.SetCellFormula(companyWeeklyBizSheet, "W3", "NETWORKDAYS(A3,B3)"); err != nil {
		return nil, err
	}
	if err := f.SetCellFormula(companyWeeklyBizSheet, "Y3", "CHOOSE(1,1,2)"); err != nil {
		return nil, err
	}
	if err := f.SetColVisible(companyWeeklyBizSheet, "AB", false); err != nil {
		return nil, err
	}
	if _, err := f.NewSheet("Ref"); err != nil {
		return nil, err
	}
	_ = f.SetCellValue("Ref", "A1", "x")
	_ = f.SetCellValue("Ref", "B1", "1")
	for _, name := range []string{"26.8월2주(업무)", "26.7월5주(업무)", "기타1", "기타2", "기타3"} {
		if _, err := f.NewSheet(name); err != nil {
			return nil, err
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sampleCompanyWeeklyRows() []model.CompanyWeeklyRow {
	return []model.CompanyWeeklyRow{
		{RowKey: "401", SheetRow: 20, Division: "경영전략기획", Team: "도서관사업부", NoLabel: "401",
			DisplayName: "2026년 충청남도교육청 도서관 통합정보시스템 SW 유지관리",
			PrevText:    "·정기점검 5사이트, AS 5건 처리완료", PlanText: "·정기점검 4사이트"},
		{RowKey: "402", SheetRow: 21, Team: "도서관사업부", NoLabel: "402", DisplayName: "제천"},
		{RowKey: "403", SheetRow: 22, Team: "도서관사업부", NoLabel: "403", DisplayName: "세종"},
		{RowKey: "404", SheetRow: 23, Team: "도서관사업부", NoLabel: "404", DisplayName: "전자책", MissingProject: true},
		{RowKey: "X01", SheetRow: 24, Team: "도서관사업부", DisplayName: "앤로보틱스"},
		{RowKey: "X02", SheetRow: 25, Team: "도서관사업부", DisplayName: "##자산관리활동", Highlight: true},
	}
}

func indexOfSheet(names []string, want string) int {
	for i, n := range names {
		if n == want {
			return i
		}
	}
	return -1
}


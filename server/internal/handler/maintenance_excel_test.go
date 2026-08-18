package handler

import (
	"archive/zip"
	"bytes"
	"regexp"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestBuildMaintenanceExcelWorkbookGraysWeekdayHoliday(t *testing.T) {
	f, err := BuildMaintenanceExcelWorkbook(2026, nil, map[string]bool{"2026-05-05": true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	holidayStyle, err := f.GetCellStyle("5월", "C4") // 2026-05-05 화
	if err != nil {
		t.Fatal(err)
	}
	weekendStyle, err := f.GetCellStyle("5월", "G3") // 2026-05-02 토
	if err != nil {
		t.Fatal(err)
	}
	weekdayStyle, err := f.GetCellStyle("5월", "D4") // 2026-05-06 수
	if err != nil {
		t.Fatal(err)
	}
	if holidayStyle != weekendStyle {
		t.Fatalf("평일 공휴일 스타일=%d 주말=%d", holidayStyle, weekendStyle)
	}
	if weekdayStyle == holidayStyle {
		t.Fatal("일반 평일까지 회색이면 안 됨")
	}
}

func TestBuildMaintenanceExcelWorkbookHolidayName(t *testing.T) {
	f, err := BuildMaintenanceExcelWorkbook(2026, nil, map[string]bool{"2026-05-05": true}, map[string]string{"2026-05-05": "어린이날"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	text, err := f.GetCellValue("5월", "C4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "어린이날") {
		t.Fatalf("공휴일명 없음: %q", text)
	}
}

func TestBuildMaintenanceExcelWorkbook(t *testing.T) {
	visits := []model.MaintenanceVisit{
		{VisitDate: "2026-03-15", ShortName: "테스트관", EntryCategory: "normal"},
		{VisitDate: "2026-03-15", ShortName: "고정관", EntryCategory: "fixed"},
		{VisitDate: "2026-03-20", ShortName: "사무소", EntryCategory: "office"},
	}
	f, err := BuildMaintenanceExcelWorkbook(2026, visits, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) != 12 {
		t.Fatalf("expected 12 sheets, got %d: %v", len(sheets), sheets)
	}
	if sheets[0] != "1월" {
		t.Fatalf("first sheet: %q", sheets[0])
	}
}

func TestMaintenanceExcelColorsAreSixDigitRGB(t *testing.T) {
	for _, c := range []string{
		excelEntryFontColor("fixed"),
		excelEntryFontColor("office"),
		excelEntryFontColor("normal"),
		excelProductFontColor("앤로보틱스"),
		excelProductFontColor("KLAS"),
		excelProductFontColor(""),
	} {
		if len(c) != 6 {
			t.Fatalf("색상 %q 는 6자리 RGB여야 한다", c)
		}
	}

	visits := []model.MaintenanceVisit{
		{VisitDate: "2026-03-15", ShortName: "고정관", EntryCategory: "fixed"},
		{VisitDate: "2026-03-15", ShortName: "KLAS관", ProductType: "KLAS"},
	}
	f, err := BuildMaintenanceExcelWorkbook(2026, visits, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	ten := regexp.MustCompile(`rgb="[A-Fa-f0-9]{10}"`)
	for _, zf := range zr.File {
		if !strings.HasSuffix(zf.Name, ".xml") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if _, err := b.ReadFrom(rc); err != nil {
			_ = rc.Close()
			t.Fatal(err)
		}
		_ = rc.Close()
		if loc := ten.Find(b.Bytes()); loc != nil {
			t.Fatalf("%s 에 10자리 색상(ARGB 이중 접두): %s", zf.Name, loc)
		}
	}
}

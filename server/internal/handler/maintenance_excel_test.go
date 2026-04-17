package handler

import (
	"testing"

	"customer-support/internal/model"
)

func TestBuildMaintenanceExcelWorkbook(t *testing.T) {
	visits := []model.MaintenanceVisit{
		{VisitDate: "2026-03-15", ShortName: "테스트관", EntryCategory: "normal"},
		{VisitDate: "2026-03-15", ShortName: "고정관", EntryCategory: "fixed"},
		{VisitDate: "2026-03-20", ShortName: "사무소", EntryCategory: "office"},
	}
	f, err := BuildMaintenanceExcelWorkbook(2026, visits)
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

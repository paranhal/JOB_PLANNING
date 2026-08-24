package model

import (
	"testing"
	"time"
)

func TestFormatExpectedPeriod(t *testing.T) {
	if got := FormatExpectedPeriod("2027-03", ExpectedPrecisionMonth); got != "2027-03" {
		t.Fatalf("month=%q", got)
	}
	if got := FormatExpectedPeriod("2027-03", ExpectedPrecisionQuarter); got != "2027년 1분기" {
		t.Fatalf("quarter=%q", got)
	}
	if got := FormatExpectedPeriod("2027-08", ExpectedPrecisionHalf); got != "2027년 하반기" {
		t.Fatalf("half=%q", got)
	}
	if got := FormatExpectedPeriod("2027-01", ExpectedPrecisionYear); got != "2027년" {
		t.Fatalf("year=%q", got)
	}
}

func TestValidateWorkProjectSales(t *testing.T) {
	mnt := &WorkProject{Name: "유지", ProjectKind: ProjectKindMaintenance}
	if err := ValidateWorkProject(mnt); err != nil {
		t.Fatalf("유지보수: %v", err)
	}
	sales := &WorkProject{
		Name: "영업", ProjectKind: ProjectKindBuild, SalesStage: SalesStageLead,
		ExpectedYM: "2027-03", SalesOwner: "김영업", ProspectName: "충남교육청",
	}
	if err := ValidateWorkProject(sales); err != nil {
		t.Fatalf("영업: %v", err)
	}
	sales.ExpectedYM = ""
	if err := ValidateWorkProject(sales); err == nil {
		t.Fatal("예정 시기 없이 통과")
	}
	sales.ExpectedUndatedReason = NoDateReasonCustomer
	if err := ValidateWorkProject(sales); err != nil {
		t.Fatalf("미정+사유: %v", err)
	}
	sales.SalesStage = SalesStageWon
	if err := ValidateWorkProject(sales); err == nil {
		t.Fatal("won 에 고객 없이 통과")
	}
	sales.CustomerID = "C1"
	sales.StartDate, sales.EndDate = "2027-01-01", "2027-12-31"
	sales.ContractType = "private"
	if err := ValidateWorkProject(sales); err != nil {
		t.Fatalf("won: %v", err)
	}
}

func TestNormalizeExpectedYM(t *testing.T) {
	if got := NormalizeExpectedYM("2027-03-01"); got != "2027-03" {
		t.Fatalf("date=%q", got)
	}
	if got := NormalizeExpectedYM("2027-3"); got != "2027-03" {
		t.Fatalf("short=%q", got)
	}
}

func TestFormatKRW(t *testing.T) {
	if got := FormatKRW(1234567); got != "1,234,567원" {
		t.Fatalf("krw=%q", got)
	}
	if FormatKRW(0) != "" {
		t.Fatal("0은 비운다")
	}
}

func TestSalesTimelineMonthsIncludesThisAndNext(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	items := []WorkProject{{ExpectedYM: "2027-03"}}
	got := SalesTimelineMonths(items, now)
	want := map[string]bool{"2026-08": true, "2026-09": true, "2027-03": true}
	if len(got) != 3 {
		t.Fatalf("months=%v", got)
	}
	for _, ym := range got {
		if !want[ym] {
			t.Fatalf("unexpected %s in %v", ym, got)
		}
	}
}

package handler

import (
	"testing"

	"customer-support/internal/model"
)

func TestBuildCustomerExcelWorkbookIncludesMaintContract(t *testing.T) {
	items := []model.CustomerListItem{
		{CustomerID: "C1", OrgName: "테스트도서관", IsActive: true, AssetCount: 1},
	}
	maint := []model.Asset{
		{
			AssetID:           "A1",
			CustomerID:        "C1",
			OrgName:           "테스트도서관",
			ProductName:       "자가대출반납기",
			ModelName:         "2510HSC",
			MaintContractType: "paid",
			MaintCycle:        "monthly",
			MaintBillingParty: "나이콤",
			MaintBillingCycle: "quarterly",
			InstallLocation:   "1층 로비",
		},
	}
	f, err := buildCustomerExcelWorkbook(items, maint)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	got, err := f.GetCellValue("Customers", "J2")
	if err != nil {
		t.Fatal(err)
	}
	if got != "유상" {
		t.Fatalf("Customers 유지보수계약 = %q, want 유상", got)
	}
	contract, err := f.GetCellValue("유지보수계약", "I2")
	if err != nil {
		t.Fatal(err)
	}
	if contract != "유상" {
		t.Fatalf("유지보수계약!I2 = %q, want 유상", contract)
	}
	cycle, err := f.GetCellValue("유지보수계약", "J2")
	if err != nil {
		t.Fatal(err)
	}
	if cycle != "월" {
		t.Fatalf("유지보수계약!J2 = %q, want 월", cycle)
	}
}

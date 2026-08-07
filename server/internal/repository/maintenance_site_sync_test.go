package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func insertSyncCustomer(t *testing.T, db *sql.DB, id, name, sido string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, addr_sido, is_active)
		VALUES (?,?,?,?,1)`, id, name, name, sido)
	if err != nil {
		t.Fatal(err)
	}
}

func insertSyncAsset(t *testing.T, db *sql.DB, assetID, customerID, productName, category, contract, cycle string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, product_category,
		maint_contract_type, maint_cycle) VALUES (?,?,?,?,?,?)`,
		assetID, customerID, productName, category, contract, cycle)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncSiteConfigsFromAssets(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	insertSyncCustomer(t, db, "c_rfid", "가나도서관", "충청남도")
	insertSyncCustomer(t, db, "c_mixed", "신성대학교", "충청남도")
	insertSyncCustomer(t, db, "c_klas", "다라평생교육원", "세종특별자치시")
	insertSyncCustomer(t, db, "c_free", "무상도서관", "충청남도")
	insertSyncCustomer(t, db, "c_call", "수시도서관", "충청남도")
	insertSyncCustomer(t, db, "c_exists", "기존도서관", "대전광역시")

	insertSyncAsset(t, db, "AA-001", "c_rfid", "RFID 게이트", "rfid", "paid", "monthly")
	insertSyncAsset(t, db, "AA-002", "c_mixed", "RFID 게이트", "rfid", "paid", "quarterly")
	insertSyncAsset(t, db, "AA-003", "c_mixed", "자동대출반납기", "rfid", "paid", "monthly")
	insertSyncAsset(t, db, "AA-004", "c_klas", "KLAS 3.0 자료관리시스템", "materials", "paid", "odd_bimonthly")
	insertSyncAsset(t, db, "AA-005", "c_free", "RFID 게이트", "rfid", "free", "monthly")
	insertSyncAsset(t, db, "AA-006", "c_call", "RFID 게이트", "rfid", "paid", "수시")
	insertSyncAsset(t, db, "AA-007", "c_exists", "RFID 게이트", "rfid", "paid", "monthly")

	repo := NewMaintenanceRepo(db)
	existing := &model.MaintenanceSiteConfig{
		CustomerID: "c_exists", ShortName: "기존", Region: "대전",
		InspectionCycle: "semi", EntryCategory: "fixed", FixedRule: "LAST_MONDAY_OF_MONTH",
	}
	if err := repo.UpsertSiteConfig(existing); err != nil {
		t.Fatal(err)
	}

	res, err := repo.SyncSiteConfigsFromAssets()
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 3 {
		t.Fatalf("added=%d names=%v (기대: 3)", res.Added, res.Names)
	}
	if res.Skipped != 1 {
		t.Fatalf("skipped=%d (기대: 1)", res.Skipped)
	}

	get := func(id string) *model.MaintenanceSiteConfig {
		t.Helper()
		cfg, err := repo.GetSiteConfig(id)
		if err != nil || cfg == nil {
			t.Fatalf("GetSiteConfig(%s): cfg=%v err=%v", id, cfg, err)
		}
		return cfg
	}

	rfid := get("c_rfid")
	if rfid.ShortName != "가나도서관" || rfid.Region != "충남" {
		t.Fatalf("표시명/지역: %q %q", rfid.ShortName, rfid.Region)
	}
	if !rfid.HasRfid || rfid.HasKlas {
		t.Fatalf("RFID/KLAS 판정: rfid=%v klas=%v", rfid.HasRfid, rfid.HasKlas)
	}
	if rfid.InspectionCycle != "monthly" || rfid.EntryCategory != "normal" {
		t.Fatalf("주기/유형: %q %q", rfid.InspectionCycle, rfid.EntryCategory)
	}

	if c := get("c_mixed").InspectionCycle; c != "monthly" {
		t.Fatalf("주기가 섞인 기관은 가장 짧은 주기여야 함: %q", c)
	}

	klas := get("c_klas")
	if !klas.HasKlas || klas.HasRfid {
		t.Fatalf("KLAS 기관 판정: klas=%v rfid=%v", klas.HasKlas, klas.HasRfid)
	}
	if klas.Region != "세종" || klas.InspectionCycle != "odd_bimonthly" {
		t.Fatalf("KLAS 기관 지역/주기: %q %q", klas.Region, klas.InspectionCycle)
	}

	if cfg, _ := repo.GetSiteConfig("c_free"); cfg != nil {
		t.Fatal("무상 계약 기관은 추가되면 안 됨")
	}
	if cfg, _ := repo.GetSiteConfig("c_call"); cfg != nil {
		t.Fatal("정기 주기가 아닌(수시) 기관은 추가되면 안 됨")
	}

	kept := get("c_exists")
	if kept.ShortName != "기존" || kept.InspectionCycle != "semi" || kept.FixedRule != "LAST_MONDAY_OF_MONTH" {
		t.Fatalf("기존 설정이 덮어써짐: %+v", kept)
	}

	// 다시 실행해도 중복 추가가 없어야 한다.
	res2, err := repo.SyncSiteConfigsFromAssets()
	if err != nil {
		t.Fatal(err)
	}
	if res2.Added != 0 || res2.Skipped != 4 {
		t.Fatalf("재실행: added=%d skipped=%d", res2.Added, res2.Skipped)
	}
}

func TestShortestCycleAndRegion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"quarterly,monthly", "monthly"},
		{"semi,quarterly", "quarterly"},
		{"even_bimonthly", "even_bimonthly"},
		{"", "monthly"},
		{"수시", "monthly"},
	}
	for _, c := range cases {
		if got := shortestCycle(c.in); got != c.want {
			t.Errorf("shortestCycle(%q)=%q, want %q", c.in, got, c.want)
		}
	}

	regions := []struct{ in, want string }{
		{"충청남도", "충남"},
		{"세종특별자치시", "세종"},
		{"대전광역시", "대전"},
		{"충북", "충북"},
		{"", ""},
	}
	for _, r := range regions {
		if got := shortRegionName(r.in); got != r.want {
			t.Errorf("shortRegionName(%q)=%q, want %q", r.in, got, r.want)
		}
	}
}

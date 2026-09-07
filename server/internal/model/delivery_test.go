package model

import "testing"

func TestLineCreatesInstalledAssetGoodsOnly(t *testing.T) {
	if !LineCreatesInstalledAsset(SalesItemKindGoods, SalesItemCatRFID) {
		t.Fatal("RFID 물품은 자산이 되어야 한다")
	}
	if LineCreatesInstalledAsset(SalesItemKindGoods, SalesItemCatConsumable) {
		t.Fatal("소모품은 자산이 되면 안 된다")
	}
	if LineCreatesInstalledAsset(SalesItemKindLabor, "") {
		t.Fatal("용역은 자산이 되면 안 된다")
	}
	if LineCreatesInstalledAsset(SalesItemKindService, "") {
		t.Fatal("AS 작업은 자산이 되면 안 된다")
	}
}

func TestDeliveryAssetDraftsSplitsSerialsAndLeavesMaintEmpty(t *testing.T) {
	o := &SalesOrder{OrderID: "SO-1", CustomerID: "c1"}
	line := SalesOrderLine{Name: "RFID 게이트", Spec: "EZ-1", Qty: 3, ItemID: "SI-1"}
	item := &SalesItem{ItemKind: SalesItemKindGoods, Category: SalesItemCatRFID, Model: "EZ-5120", Manufacturer: "앤로보틱스"}
	d := SalesDelivery{DeliveryDate: "2026-09-04", Qty: 3, InstalledBy: "양기헌"}
	got, err := DeliveryAssetDrafts(o, line, item, d, []string{"SN-A", "SN-B", "SN-C"}, "1층 로비")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("행=%d", len(got))
	}
	for i, a := range got {
		if a.CustomerID != "c1" || a.InstallDate != "2026-09-04" || a.InstallerType != InstallerTypeSelf {
			t.Fatalf("필드 %+v", a)
		}
		if a.InstallLocation != "1층 로비" || a.SalesOrderID != "SO-1" {
			t.Fatalf("위치/수주 %+v", a)
		}
		if !a.MaintFieldsEmpty() {
			t.Fatalf("maint 가 채워졌다 %+v", a)
		}
		if a.IsManaged {
			t.Fatal("계약 전에 is_managed 가 켜졌다")
		}
		want := []string{"SN-A", "SN-B", "SN-C"}[i]
		if a.SerialNumber != want {
			t.Fatalf("시리얼=%s", a.SerialNumber)
		}
	}
}

func TestDeliveryAssetDraftsSkipNonGoods(t *testing.T) {
	o := &SalesOrder{OrderID: "SO-1", CustomerID: "c1"}
	d := SalesDelivery{Qty: 1, DeliveryDate: "2026-09-04"}
	paper := &SalesItem{ItemKind: SalesItemKindGoods, Category: SalesItemCatConsumable}
	got, err := DeliveryAssetDrafts(o, SalesOrderLine{Name: "감열지"}, paper, d, []string{"X"}, "")
	if err != nil || len(got) != 0 {
		t.Fatalf("소모품 assets=%d err=%v", len(got), err)
	}
	labor := &SalesItem{ItemKind: SalesItemKindLabor}
	got, err = DeliveryAssetDrafts(o, SalesOrderLine{Name: "개발"}, labor, d, nil, "")
	if err != nil || len(got) != 0 {
		t.Fatalf("용역 assets=%d err=%v", len(got), err)
	}
}

func TestSerialSlots(t *testing.T) {
	if SerialSlots(3) != 3 || SerialSlots(2.2) != 3 || SerialSlots(0) != 0 {
		t.Fatal("슬롯 계산")
	}
}

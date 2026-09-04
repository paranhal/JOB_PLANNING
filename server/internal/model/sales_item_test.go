package model

import "testing"

func TestFillSalesItemKanbanFourKinds(t *testing.T) {
	items := []SalesItem{
		{ItemID: "SI-001", Name: "감열지", ItemKind: SalesItemKindGoods, NeedsReview: true, Unit: "EA"},
		{ItemID: "SI-002", Name: "출장 AS", ItemKind: SalesItemKindService, LastPrice: 80000},
		{ItemID: "SI-003", Name: "개발", ItemKind: SalesItemKindLabor, Unit: "M/M"},
		{ItemID: "SI-001", Name: "중복", ItemKind: SalesItemKindEtc},
	}
	view := FillSalesItemKanban(items)
	if len(view.Columns) != 4 {
		t.Fatalf("열=%d", len(view.Columns))
	}
	if view.Total != 3 {
		t.Fatalf("건수=%d", view.Total)
	}
	if view.Columns[0].Key != SalesItemKindGoods || view.Columns[0].Items[0].DelayBadge != "확인 필요" {
		t.Fatalf("물품 열: %+v", view.Columns[0])
	}
	if view.Columns[1].Key != SalesItemKindService || view.Columns[2].Key != SalesItemKindLabor || view.Columns[3].Key != SalesItemKindEtc {
		t.Fatalf("열 순서: %+v", view.Columns)
	}
	if view.Columns[3].Count != 0 {
		t.Fatal("기타 열에 카드가 들어갔다")
	}
}

func TestMapAssetProductToSalesItem(t *testing.T) {
	kind, cat := MapAssetProductToSalesItem("materials")
	if kind != SalesItemKindGoods || cat != SalesItemCatConsumable {
		t.Fatalf("materials → %s %s", kind, cat)
	}
	kind, cat = MapAssetProductToSalesItem("homepage")
	if cat != SalesItemCatSW {
		t.Fatalf("homepage → %s", cat)
	}
}

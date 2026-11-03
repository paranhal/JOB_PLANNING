package model

import "testing"

func TestItemKindSummary_95_82(t *testing.T) {
	got := SummarizeItemKinds([]WorkTaskItem{
		{Kind: ItemKindInstall, Qty: 5, Status: ItemDone},
		{Kind: ItemKindInstall, Qty: 10, Status: ItemTodo},
		{Kind: ItemKindWork, Qty: 1, Status: ItemNA},
		{Kind: ItemKindAS, Qty: 1, Status: ItemDoing},
		{Kind: ItemKindEtc, Qty: 1, Status: ItemHold},
	})
	if got.Line != "장착 2건 · 공사 1건 · AS 1건 · 기타 1건" {
		t.Fatalf("line %q", got.Line)
	}
	if got.LineQty != "장착 2건(15대) · 공사 1건 · AS 1건 · 기타 1건" {
		t.Fatalf("qty %q", got.LineQty)
	}
	if got.Progress != "항목 완료 2/5" {
		t.Fatalf("progress %q", got.Progress)
	}
	empty := SummarizeItemKinds(nil)
	if empty.Line != "" || empty.Progress != "" {
		t.Fatalf("0건은 생략이어야 함 %+v", empty)
	}
}

func TestItemStatusFromAS_95_82(t *testing.T) {
	if ItemStatusFromAS("received") != ItemTodo || ItemStatusFromAS("") != ItemTodo {
		t.Fatal("미착수")
	}
	if ItemStatusFromAS("in_progress") != ItemDoing {
		t.Fatal("진행")
	}
	if ItemStatusFromAS("complete") != ItemDone {
		t.Fatal("완료")
	}
	if ItemStatusFromAS("cancelled") != ItemNA {
		t.Fatal("취소")
	}
	if ItemStatusFromAS("hold") != ItemDoing {
		t.Fatal("보류는 진행 표시")
	}
}

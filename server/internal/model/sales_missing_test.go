package model

import "testing"

func TestSalesMissingInfoItemsAndList(t *testing.T) {
	empty := &SalesProject{Stage: SalesStage4Discover, Status: SalesStatusActive}
	got := SalesMissingInfo(empty)
	if got.HideBox || !got.HasEmpty() || !got.ListNeeded() {
		t.Fatalf("빈 발굴 건 HideBox=%v empty=%v list=%v", got.HideBox, got.HasEmpty(), got.ListNeeded())
	}
	byKey := map[string]SalesMissingItem{}
	for _, it := range got.Items {
		byKey[it.Key] = it
	}
	if byKey["amount"].Filled || !byKey["amount"].ListRelevant {
		t.Fatal("금액")
	}
	if byKey["period"].Filled || byKey["period"].When != "제안 단계 전 권장" {
		t.Fatal("시기")
	}
	if byKey["biz_type"].Filled {
		t.Fatal("유형")
	}
	if byKey["customer"].Filled || byKey["customer"].When != "계약 전환 때 필요" {
		t.Fatal("고객 둘 다 비움")
	}
	if byKey["contract"].Filled || byKey["contract"].ListRelevant {
		t.Fatal("계약은 목록 제외")
	}

	prospect := &SalesProject{Stage: SalesStage4Discover, ProspectName: "미등록기관"}
	c := SalesMissingInfo(prospect)
	for _, it := range c.Items {
		if it.Key == "customer" {
			if it.Filled || it.When != "계약 전환 때 고객 등록" {
				t.Fatalf("미등록만: %+v", it)
			}
		}
	}
	if !c.ListNeeded() {
		t.Fatal("미등록 고객명만 있어도 보완 필요")
	}

	fullList := &SalesProject{
		Stage: SalesStage4Discover, ExpectedAmount: 100, ExpectedYM: "2026-11",
		BizType: SalesBizBuild, CustomerID: "C1",
	}
	f := SalesMissingInfo(fullList)
	if f.ListNeeded() {
		t.Fatal("목록 4항목이 있으면 보완 필요 아님")
	}
	if !f.HasEmpty() {
		t.Fatal("계약은 아직 비어 있음")
	}

	closed := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseContracted}
	if !SalesMissingInfo(closed).HideBox || SalesMissingInfo(closed).ListNeeded() {
		t.Fatal("종료 단계는 박스·목록에서 뺀다")
	}
	lost := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseLost}
	if !SalesMissingInfo(lost).HideBox {
		t.Fatal("실주")
	}
	drop := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseDropped}
	if !SalesMissingInfo(drop).HideBox {
		t.Fatal("중단")
	}
	dorm := &SalesProject{Stage: SalesStage4Discover, Status: SalesStatusDormant}
	if !SalesMissingInfo(dorm).HideBox || SalesMissingInfo(dorm).ListNeeded() {
		t.Fatal("휴면")
	}
}

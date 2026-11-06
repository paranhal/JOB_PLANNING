package model

import "testing"

func TestContractRowPromoteChipSkipsSupply(t *testing.T) {
	build := ContractRow{Source: ContractSourceSales, DealType: SalesDealBuild, PromotePossible: true}
	if !build.PromotePossible || build.StatusChip() != "영업 계약 완료 · 계약 전환 필요" {
		t.Fatalf("구축 칩=%q possible=%v", build.StatusChip(), build.PromotePossible)
	}
	supply := ContractRow{Source: ContractSourceSales, DealType: SalesDealSupply, PromotePossible: false}
	if supply.PromotePossible || supply.StatusChip() != "단품 납품 · 영업에서 종료" {
		t.Fatalf("단품 칩=%q possible=%v", supply.StatusChip(), supply.PromotePossible)
	}
	promo := ContractRow{Source: ContractSourceSales, Promoted: true, PromotePossible: false}
	if promo.PromotePossible || promo.StatusChip() != "전환 기록 있음, 연결 없음" {
		t.Fatalf("승격 칩=%q", promo.StatusChip())
	}
	kpi := BuildContractUnionKPI([]ContractRow{build, supply, promo}, "2026-11-06")
	if kpi.Unpromoted != 1 {
		t.Fatalf("미전환=%d", kpi.Unpromoted)
	}
	if ContractRowPromotePossible(&SalesProject{DealType: SalesDealSupply, Stage: SalesStage4Closed, CloseReason: SalesCloseContracted, Status: SalesStatusContracted}) {
		t.Fatal("단품 PromotePossible")
	}
}

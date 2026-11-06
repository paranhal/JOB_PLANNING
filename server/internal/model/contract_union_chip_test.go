package model

import "testing"

func TestContractRowPromoteChipSkipsSupply(t *testing.T) {
	build := ContractRow{Source: ContractSourceSales, DealType: SalesDealBuild}
	if !build.PromoteEligible() || build.StatusChip() != "영업 계약 완료 · 계약 전환 필요" {
		t.Fatalf("구축 칩=%q eligible=%v", build.StatusChip(), build.PromoteEligible())
	}
	supply := ContractRow{Source: ContractSourceSales, DealType: SalesDealSupply}
	if supply.PromoteEligible() || supply.StatusChip() != "" {
		t.Fatalf("단품 칩=%q eligible=%v", supply.StatusChip(), supply.PromoteEligible())
	}
	promo := ContractRow{Source: ContractSourceSales, Promoted: true}
	if promo.PromoteEligible() || promo.StatusChip() != "전환 기록 있음, 연결 없음" {
		t.Fatalf("승격 칩=%q", promo.StatusChip())
	}
	kpi := BuildContractUnionKPI([]ContractRow{build, supply}, "2026-11-06")
	if kpi.Unpromoted != 1 {
		t.Fatalf("미전환=%d", kpi.Unpromoted)
	}
}

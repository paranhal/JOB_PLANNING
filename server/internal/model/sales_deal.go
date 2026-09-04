package model

import "strings"

const (
	SalesDealBuild  = "build"
	SalesDealSupply = "supply"
	SalesDealAll    = "all" // 목록 필터 전용. DB 값이 아니다.
)

const (
	SalesStageInquiry   = "inquiry"
	SalesStageQuoted    = "quoted"
	SalesStageOrdered   = "ordered"
	SalesStageDelivered = "delivered"
	SalesStageDropped   = "dropped"
)

const SalesCodeGroupSupplyStage = "sales_supply_stage"

func NormalizeSalesDealType(s string) string {
	if strings.TrimSpace(s) == SalesDealSupply {
		return SalesDealSupply
	}
	return SalesDealBuild
}

func (p *SalesProject) IsSupply() bool {
	return p != nil && NormalizeSalesDealType(p.DealType) == SalesDealSupply
}

func DefaultSalesStageCodeFor(dealType string) string {
	if NormalizeSalesDealType(dealType) == SalesDealSupply {
		return SalesStageInquiry
	}
	return DefaultSalesStageCode()
}

func IsSalesBuildStage(code string) bool {
	switch CurrentSalesStage(code) {
	case SalesStageContact, SalesStageLead, SalesStageProposal, SalesStageNegotiation, SalesStageWon, SalesStageLost:
		return true
	default:
		return false
	}
}

func IsSalesSupplyStage(code string) bool {
	switch strings.TrimSpace(code) {
	case SalesStageInquiry, SalesStageQuoted, SalesStageOrdered, SalesStageDelivered, SalesStageDropped:
		return true
	default:
		return false
	}
}

func SalesStageAllowedForDeal(dealType, stage string) bool {
	if NormalizeSalesDealType(dealType) == SalesDealSupply {
		return IsSalesSupplyStage(stage)
	}
	return IsSalesBuildStage(stage)
}

func fallbackSupplyStages() []SalesStageDef {
	return []SalesStageDef{
		{Code: SalesStageInquiry, Label: "문의 접수", SortOrder: 1},
		{Code: SalesStageQuoted, Label: "견적 제출", SortOrder: 2},
		{Code: SalesStageOrdered, Label: "수주", SortOrder: 3},
		{Code: SalesStageDelivered, Label: "납품 완료", SortOrder: 4},
		{Code: SalesStageDropped, Label: "취소·실주", SortOrder: 5},
	}
}

func LoadSalesSupplyStages(stageCodes []Code) []SalesStageDef {
	out := LoadSalesStages(stageCodes, nil)
	if len(out) == 0 {
		return fallbackSupplyStages()
	}
	for i := range out {
		out[i].Probability = 0
	}
	return out
}

// SupplyAutoStage 견적 금액·발주번호·납품일로 단품 단계를 앞으로만 올린다 (§36.5).
func SupplyAutoStage(p *SalesProject) string {
	if p == nil || !p.IsSupply() {
		return ""
	}
	cur := strings.TrimSpace(p.Stage)
	if cur == SalesStageDropped {
		return cur
	}
	next := SalesStageInquiry
	if p.ExpectedAmount > 0 {
		next = SalesStageQuoted
	}
	if strings.TrimSpace(p.PONo) != "" {
		next = SalesStageOrdered
	}
	if strings.TrimSpace(p.DeliveredAt) != "" {
		next = SalesStageDelivered
	}
	stages := fallbackSupplyStages()
	from := FindSalesStage(stages, cur)
	to := FindSalesStage(stages, next)
	if from != nil && to != nil && IsSalesStageBackward(from, to) {
		return cur
	}
	if cur == "" {
		return next
	}
	return next
}

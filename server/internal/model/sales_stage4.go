package model

import (
	"strings"
	"time"
)

const (
	SalesStage4Discover = "discover"
	SalesStage4Propose  = "propose"
	SalesStage4Bid      = "bid"
	SalesStage4Closed   = "closed"

	SalesBidPending     = "pending"
	SalesBidWon         = "won"
	SalesBidNegotiating = "negotiating"

	SalesCloseContracted = "contracted"
	SalesCloseLost       = "lost"
	SalesCloseDropped    = "dropped"

	SalesStatusContracted = "contracted"
	SalesStatusDropped    = "dropped"

	SalesCodeGroupStage4      = "sales_stage4"
	SalesCodeGroupStage4Prob  = "sales_stage4_prob"
	SalesCodeGroupBidStatus   = "sales_bid_status"
	SalesCodeGroupCloseReason = "sales_close_reason"
	SalesCodeGroupDropReason  = "sales_drop_reason"
	SalesDirectWin            = "direct_won"
)

// SalesProbability §47.4.2. probability 열은 이 값만 쓴다.
func SalesProbability(p *SalesProject) int {
	if p == nil {
		return 0
	}
	switch strings.TrimSpace(p.Stage) {
	case SalesStage4Discover:
		return 10
	case SalesStage4Propose:
		if strings.TrimSpace(p.RFPReceivedAt) == "" {
			return 20
		}
		if p.WinProb == nil {
			return 50
		}
		return *p.WinProb
	case SalesStage4Bid:
		switch strings.TrimSpace(p.BidStatus) {
		case SalesBidPending:
			if p.ProbabilityFinal == nil {
				return 0
			}
			return *p.ProbabilityFinal
		case SalesBidWon, SalesBidNegotiating:
			return 100
		}
	case SalesStage4Closed:
		switch strings.TrimSpace(p.CloseReason) {
		case SalesCloseContracted:
			return 100
		case SalesCloseLost, SalesCloseDropped:
			return 0
		}
	}
	return 0
}

func IntPtr(n int) *int { return &n }

func ValidWinProb(n int) bool {
	if n < 10 || n > 90 {
		return false
	}
	return n%10 == 0
}

func CanEnterBidWithoutEval(p *SalesProject) bool {
	if p == nil {
		return false
	}
	route := strings.TrimSpace(p.ProcurementRoute)
	method := strings.TrimSpace(p.ContractMethod)
	return route == "private" || method == "private_contract"
}

func CanPromoteSales(p *SalesProject) bool {
	if p == nil {
		return false
	}
	return strings.TrimSpace(p.Stage) == SalesStage4Closed &&
		strings.TrimSpace(p.CloseReason) == SalesCloseContracted &&
		strings.TrimSpace(p.Status) != SalesStatusPromoted
}

func SalesContractUnsigned(p *SalesProject, today time.Time) bool {
	if p == nil {
		return false
	}
	st := strings.TrimSpace(p.BidStatus)
	if st != SalesBidWon && st != SalesBidNegotiating {
		return false
	}
	if strings.TrimSpace(p.Stage) == SalesStage4Closed {
		return false
	}
	start := parseSalesAt(p.WonAt)
	if start.IsZero() {
		return false
	}
	a := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	days := int(b.Sub(a).Hours() / 24)
	return days > SalesContractUnsignedDays
}

// SalesWinSample 이긴 건: won_at != ” · 진 건: close_reason=lost AND won_at=”. 포기는 제외. §47.15.5
func SalesWinSample(projects []SalesProject) (won, lost int) {
	for i := range projects {
		p := projects[i]
		if strings.TrimSpace(p.Status) == SalesStatusDormant {
			continue
		}
		if strings.TrimSpace(p.WonAt) != "" {
			won++
		}
		if strings.TrimSpace(p.CloseReason) == SalesCloseLost && strings.TrimSpace(p.WonAt) == "" {
			lost++
		}
	}
	return won, lost
}

func SalesWinSampleLabel() string {
	return "입찰 이긴 건 ÷ (이긴 건 + 진 건) · 포기 제외"
}

func IsSalesStage4(code string) bool {
	switch strings.TrimSpace(code) {
	case SalesStage4Discover, SalesStage4Propose, SalesStage4Bid, SalesStage4Closed:
		return true
	default:
		return false
	}
}

func knownSalesStageLabel(code string) string {
	switch strings.TrimSpace(code) {
	case SalesStage4Discover, SalesStageContact:
		return "발굴"
	case SalesStage4Propose:
		return "제안"
	case SalesStage4Bid:
		return "입찰"
	case SalesStage4Closed:
		return "사업 종료"
	case SalesStageLead:
		return "검토"
	case SalesStageProposal:
		return "견적"
	case SalesStageNegotiation:
		return "협상"
	case SalesStageWon, SalesStageOrdered:
		return "수주"
	case SalesStageLost:
		return "실주"
	case SalesStageInquiry:
		return "문의 접수"
	case SalesStageQuoted:
		return "견적 제출"
	case SalesStageDelivered:
		return "납품 완료"
	case SalesStageDropped:
		return "포기"
	case SalesBidPending:
		return "결과 대기"
	case SalesBidNegotiating:
		return "협상 중"
	case SalesCloseContracted:
		return "계약"
	default:
		return SalesLegacyStageLabel(code)
	}
}

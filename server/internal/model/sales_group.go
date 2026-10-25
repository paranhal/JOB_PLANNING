package model

import (
	"encoding/json"
	"strings"
)

const (
	SalesGroupManual = "manual"
	SalesGroupFilter = "filter"
	SalesGroupActive = "active"
	SalesGroupClosed = "closed"

	SalesMemberManual  = "manual"
	SalesMemberInclude = "include"
	SalesMemberExclude = "exclude"
)

type SalesGroup struct {
	GroupID    string
	GroupNo    string
	Name       string
	Year       int
	OwnerID    string
	OwnerName  string
	GroupKind  string
	FilterJSON string
	Pinned     bool
	Status     string
	Notes      string
	CreatedAt  string
	UpdatedAt  string
	ClosedAt           string
	ChildNameTemplate  string
	ChildDefaultsJSON  string
	AutoClose          bool
}

type SalesGroupMember struct {
	GroupID    string
	SalesID    string
	SortOrder  int
	AddedAt    string
	AddedBy    string
	MemberKind string
}

func (g *SalesGroup) Filter() SalesListFilterJSON {
	var f SalesListFilterJSON
	raw := ""
	if g != nil {
		raw = strings.TrimSpace(g.FilterJSON)
	}
	if raw == "" || raw == "{}" || raw == "null" {
		return f
	}
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return SalesListFilterJSON{}
	}
	return f
}

// SalesListFilterJSON 조건형 대분류 조건. 목록 필터와 같은 칸.
type SalesListFilterJSON struct {
	BizType          string `json:"biz_type"`
	BudgetYear       int    `json:"budget_year"`
	BudgetStatus     string `json:"budget_status"`
	Owner            string `json:"owner"`
	Stage            string `json:"stage"`
	ContractTarget   string `json:"contract_target"`
	ProcurementRoute string `json:"procurement_route"`
	ContractMethod   string `json:"contract_method"`
	BidEvalMethod    string `json:"bid_eval_method"`
	MallContractType string `json:"mall_contract_type"`
	ExpectedFrom     string `json:"expected_from"`
	ExpectedTo       string `json:"expected_to"`
}

func SalesGroupTotals(items []SalesProject, quotes map[string][]SalesQuote) (count, amount, weighted int) {
	seen := map[string]bool{}
	for i := range items {
		p := items[i]
		if p.Status == SalesStatusDormant {
			continue
		}
		if p.CloseReason == SalesCloseLost || p.CloseReason == SalesCloseDropped ||
			p.Status == SalesStatusLost || p.Status == SalesStatusDropped {
			continue
		}
		if seen[p.SalesID] {
			continue
		}
		seen[p.SalesID] = true
		ApplySalesPipelineAmount(&p, quotes[p.SalesID])
		count++
		amount += p.PipeAmount
		weighted += p.PipeAmount * SalesProbability(&p) / 100
	}
	return
}

func SalesGroupTotalsAllowDup(items []SalesProject, quotes map[string][]SalesQuote) (count, amount, weighted int) {
	for i := range items {
		p := items[i]
		if p.Status == SalesStatusDormant {
			continue
		}
		if p.CloseReason == SalesCloseLost || p.CloseReason == SalesCloseDropped ||
			p.Status == SalesStatusLost || p.Status == SalesStatusDropped {
			continue
		}
		ApplySalesPipelineAmount(&p, quotes[p.SalesID])
		count++
		amount += p.PipeAmount
		weighted += p.PipeAmount * SalesProbability(&p) / 100
	}
	return
}

const (
	SalesChildProgress  = "progress"
	SalesChildWon       = "won"
	SalesChildContract  = "contract"
	SalesChildLost      = "lost"
	SalesChildDropped   = "dropped"
)

func SalesChildBucket(p SalesProject) string {
	if p.Stage == SalesStage4Closed {
		switch strings.TrimSpace(p.CloseReason) {
		case SalesCloseContracted:
			return SalesChildContract
		case SalesCloseLost:
			return SalesChildLost
		case SalesCloseDropped:
			return SalesChildDropped
		}
		if p.Status == SalesStatusContracted {
			return SalesChildContract
		}
		if p.Status == SalesStatusDropped {
			return SalesChildDropped
		}
		return SalesChildLost
	}
	if p.Stage == SalesStage4Bid {
		switch strings.TrimSpace(p.BidStatus) {
		case SalesBidWon, SalesBidNegotiating:
			return SalesChildWon
		}
	}
	return SalesChildProgress
}

func SalesChildDecided(p SalesProject) bool {
	if strings.TrimSpace(p.Status) == SalesStatusDormant {
		return false
	}
	b := SalesChildBucket(p)
	return b == SalesChildWon || b == SalesChildContract || b == SalesChildLost || b == SalesChildDropped
}

func SalesChildAmount(p SalesProject) int {
	switch SalesChildBucket(p) {
	case SalesChildContract:
		if p.ContractAmount > 0 {
			return p.ContractAmount
		}
		if p.AwardedAmount > 0 {
			return p.AwardedAmount
		}
	case SalesChildWon:
		if p.AwardedAmount > 0 {
			return p.AwardedAmount
		}
	}
	return p.ExpectedAmount
}

type SalesChildSummary struct {
	ProgressN, ProgressAmt int
	WonN, WonAmt           int
	ContractN, ContractAmt int
	LostN, LostAmt         int
	DroppedN, DroppedAmt   int
}

func (s SalesChildSummary) Add(p SalesProject) SalesChildSummary {
	if strings.TrimSpace(p.Status) == SalesStatusDormant {
		return s
	}
	n, amt := 1, SalesChildAmount(p)
	switch SalesChildBucket(p) {
	case SalesChildWon:
		s.WonN += n
		s.WonAmt += amt
	case SalesChildContract:
		s.ContractN += n
		s.ContractAmt += amt
	case SalesChildLost:
		s.LostN += n
		s.LostAmt += amt
	case SalesChildDropped:
		s.DroppedN += n
		s.DroppedAmt += amt
	default:
		s.ProgressN += n
		s.ProgressAmt += amt
	}
	return s
}

func SalesChildSummaryOf(items []SalesProject) SalesChildSummary {
	var s SalesChildSummary
	for _, p := range items {
		s = s.Add(p)
	}
	return s
}

func SalesAvgProbLabel(amount, weighted int) string {
	if amount <= 0 {
		return "—"
	}
	return FormatSalesRate(weighted, amount)
}

package model

import "encoding/json"

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
	ClosedAt   string
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
	if g == nil || g.FilterJSON == "" {
		return f
	}
	_ = json.Unmarshal([]byte(g.FilterJSON), &f)
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

func SalesAvgProbLabel(amount, weighted int) string {
	if amount <= 0 {
		return "—"
	}
	return FormatSalesRate(weighted, amount)
}

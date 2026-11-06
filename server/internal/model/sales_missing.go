package model

import "strings"

type SalesMissingItem struct {
	Key          string
	Label        string
	When         string
	Filled       bool
	ListRelevant bool
}

type SalesMissingInfoData struct {
	HideBox bool
	Items   []SalesMissingItem
}

func (m SalesMissingInfoData) HasEmpty() bool {
	for _, it := range m.Items {
		if !it.Filled {
			return true
		}
	}
	return false
}

func (m SalesMissingInfoData) ListNeeded() bool {
	if m.HideBox {
		return false
	}
	for _, it := range m.Items {
		if it.ListRelevant && !it.Filled {
			return true
		}
	}
	return false
}

func SalesMissingInfoHideBox(p *SalesProject) bool {
	if p == nil {
		return true
	}
	if strings.TrimSpace(p.Stage) == SalesStage4Closed {
		return true
	}
	if p.IsDormant() {
		return true
	}
	switch strings.TrimSpace(p.CloseReason) {
	case SalesCloseContracted, SalesCloseLost, SalesCloseDropped:
		return true
	default:
		return false
	}
}

func SalesMissingInfo(p *SalesProject) SalesMissingInfoData {
	out := SalesMissingInfoData{HideBox: SalesMissingInfoHideBox(p)}
	if p == nil {
		return out
	}
	custID := strings.TrimSpace(p.CustomerID)
	prospect := strings.TrimSpace(p.ProspectName)
	custWhen := "계약 전환 때 필요"
	if custID == "" && prospect != "" {
		custWhen = "계약 전환 때 고객 등록"
	}
	out.Items = []SalesMissingItem{
		{Key: "amount", Label: "예상 금액", When: "제안·협상 때 권장", Filled: p.ExpectedAmount > 0, ListRelevant: true},
		{Key: "period", Label: "시기", When: "제안 단계 전 권장", Filled: strings.TrimSpace(p.ExpectedYM) != "", ListRelevant: true},
		{Key: "biz_type", Label: "사업 유형", When: "진행 중", Filled: strings.TrimSpace(p.BizType) != "", ListRelevant: true},
		{Key: "customer", Label: "고객 등록", When: custWhen, Filled: custID != "", ListRelevant: true},
		{Key: "contract", Label: "계약일·계약금액", When: "계약 마감 때 필수", Filled: strings.TrimSpace(p.ContractedAt) != "" && p.ContractAmount > 0, ListRelevant: false},
	}
	return out
}

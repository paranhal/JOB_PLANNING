package model

import (
	"strings"
)

const (
	SalesItemKindGoods   = "goods"
	SalesItemKindService = "service"
	SalesItemKindLabor   = "labor"
	SalesItemKindEtc     = "etc"
)

const (
	SalesItemCatRFID         = "RFID장비"
	SalesItemCatConsumable   = "소모품"
	SalesItemCatSW           = "SW"
	SalesItemCatConstruction = "공사"
)

const SalesCodeGroupItemKind = "sales_item_kind"
const SalesCodeGroupItemCat = "sales_item_category"
const SalesCodeGroupItemUnit = "sales_item_unit"

// SalesItem 판매 품목 카탈로그. assets(시리얼)와 표를 나눈다 (§36.6).
type SalesItem struct {
	ItemID          string
	ItemKind        string
	Category        string
	Name            string
	Spec            string
	Model           string
	Manufacturer    string
	Unit            string
	GovItemNo       string
	ListPrice       int
	LastPrice       int
	LastQuotedAt    string
	LastCustomer    string
	DefaultSupplier string
	IsActive        bool
	NeedsReview     bool
	Notes           string
	CreatedAt       string
	UpdatedAt       string
}

func NormalizeSalesItemKind(s string) string {
	switch strings.TrimSpace(s) {
	case SalesItemKindService, SalesItemKindLabor, SalesItemKindEtc:
		return strings.TrimSpace(s)
	default:
		return SalesItemKindGoods
	}
}

func SalesItemKindLabel(kind string) string {
	switch NormalizeSalesItemKind(kind) {
	case SalesItemKindService:
		return "AS·작업"
	case SalesItemKindLabor:
		return "개발 용역"
	case SalesItemKindEtc:
		return "기타"
	default:
		return "물품"
	}
}

func SalesItemKindDefs() []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: SalesItemKindGoods, Title: "물품", Border: "border-slate-200"},
		{Key: SalesItemKindService, Title: "AS·작업", Border: "border-sky-200"},
		{Key: SalesItemKindLabor, Title: "개발 용역", Border: "border-violet-200"},
		{Key: SalesItemKindEtc, Title: "기타", Border: "border-gray-300"},
	}
}

func SalesItemUnits() []string {
	return []string{"EA", "식", "대", "Box", "인", "M/M"}
}

func SalesItemCategories() []string {
	return []string{SalesItemCatRFID, SalesItemCatConsumable, SalesItemCatSW, SalesItemCatConstruction}
}

func MapAssetProductToSalesItem(productCategory string) (kind, category string) {
	switch strings.ToLower(strings.TrimSpace(productCategory)) {
	case "rfid":
		return SalesItemKindGoods, SalesItemCatRFID
	case "materials":
		return SalesItemKindGoods, SalesItemCatConsumable
	case "homepage", "elibrary", "mobile":
		return SalesItemKindGoods, SalesItemCatSW
	default:
		return SalesItemKindGoods, SalesItemCatRFID
	}
}

func (p *SalesItem) KindLabel() string {
	if p == nil {
		return SalesItemKindLabel("")
	}
	return SalesItemKindLabel(p.ItemKind)
}

func (p *SalesItem) LastPriceLabel() string {
	if p == nil || p.LastPrice <= 0 {
		return ""
	}
	return formatSalesAmount(p.LastPrice) + "원"
}

func (p *SalesItem) ListPriceLabel() string {
	if p == nil || p.ListPrice <= 0 {
		return ""
	}
	return formatSalesAmount(p.ListPrice) + "원"
}

func FillSalesItemKanban(items []SalesItem) KanbanView {
	cols := emptyKanbanColumns(SalesItemKindDefs())
	idx := indexKanbanColumns(cols)
	seen := map[string]bool{}
	for i := range items {
		it := items[i]
		id := strings.TrimSpace(it.ItemID)
		if id == "" || seen[id] {
			continue
		}
		b := NormalizeSalesItemKind(it.ItemKind)
		j, ok := idx[b]
		if !ok {
			continue
		}
		seen[id] = true
		extra := it.LastPriceLabel()
		if extra == "" {
			extra = it.Unit
		}
		card := KanbanCard{
			ID:         id,
			RefID:      id,
			RefNumber:  id,
			Title:      it.Name,
			Href:       "/items/" + id + "/edit",
			EditHref:   "/items/" + id + "/edit",
			OrgName:    it.Manufacturer,
			Extra:      extra,
			Bucket:     b,
			DelayBadge: "",
			DelayClass: "",
			AmountDesc: it.LastPrice,
		}
		if it.NeedsReview {
			card.DelayBadge = "확인 필요"
			card.DelayClass = "bg-amber-100 text-amber-800"
		}
		if !it.IsActive && !it.NeedsReview {
			card.DelayBadge = "미사용"
			card.DelayClass = "bg-gray-100 text-gray-600"
		}
		cols[j].Items = append(cols[j].Items, card)
	}
	v := finishKanbanView(cols)
	for i := range v.Columns {
		v.Columns[i].CountUnit = "건"
	}
	return v
}

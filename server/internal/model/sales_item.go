package model

import (
	"strconv"
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
	ManufacturerID  string
	SupplierID      string
	MfrPartyName    string
	SupPartyName    string
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
	SpecCount       int
	PriceMin        int
	PriceMax        int
	Specs           []SalesItemSpec
	Suppliers       []SalesItemSupplierBlock
}

type SalesItemSpec struct {
	SpecID    string
	ItemID    string
	Spec      string
	Unit      string
	Price     int
	SortOrder int
	IsActive  bool
}

type SalesItemSupplierPrice struct {
	SPID       string
	ItemID     string
	SupplierID string
	SpecID     string
	Unit       string
	Price      int
	SortOrder  int
}

type SalesItemSupplierBlock struct {
	SupplierID   string
	SupplierName string
	Rows         []SalesItemSupplierPrice
}

type SalesItemSpecMargin struct {
	Spec    SalesItemSpec
	BuyMin  int
	Profit  int
	Percent string
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

// LineCreatesInstalledAsset 물품(goods)만 설치자산을 만든다.
// 소모품·용역·AS 작업은 만들지 않는다 (§36.10.1).
func LineCreatesInstalledAsset(kind, category string) bool {
	if NormalizeSalesItemKind(kind) != SalesItemKindGoods {
		return false
	}
	return strings.TrimSpace(category) != SalesItemCatConsumable
}

func (p *SalesItem) KindLabel() string {
	if p == nil {
		return SalesItemKindLabel("")
	}
	return SalesItemKindLabel(p.ItemKind)
}

func (p *SalesItem) ManufacturerLabel() string {
	if p == nil {
		return ""
	}
	if s := strings.TrimSpace(p.MfrPartyName); s != "" {
		return s
	}
	return strings.TrimSpace(p.Manufacturer)
}

func (p *SalesItem) SupplierLabel() string {
	if p == nil {
		return ""
	}
	if s := strings.TrimSpace(p.SupPartyName); s != "" {
		return s
	}
	return strings.TrimSpace(p.DefaultSupplier)
}

func (p *SalesItem) NeedsPartyLink() bool {
	if p == nil {
		return false
	}
	if strings.TrimSpace(p.Manufacturer) != "" && strings.TrimSpace(p.ManufacturerID) == "" {
		return true
	}
	if strings.TrimSpace(p.DefaultSupplier) != "" && strings.TrimSpace(p.SupplierID) == "" {
		return true
	}
	return false
}

// ItemQuoteUse 품목이 들어간 견적 한 줄. 품목 표에 복사하지 않는다. §47.20.3
type ItemQuoteUse struct {
	QuoteID   string
	QuoteNo   string
	QuoteDate string
	Customer  string
	LineName  string
	UnitPrice int
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

func (p *SalesItem) SpecRangeLabel() string {
	if p == nil || p.SpecCount <= 0 {
		return ""
	}
	head := formatSalesAmount(p.SpecCount) + "개 규격"
	if p.SpecCount < 1000 {
		head = strconv.Itoa(p.SpecCount) + "개 규격"
	}
	if p.PriceMin <= 0 && p.PriceMax <= 0 {
		return head
	}
	if p.PriceMin == p.PriceMax {
		return head + " · " + formatSalesAmount(p.PriceMin) + "원"
	}
	return head + " · " + formatSalesAmount(p.PriceMin) + "~" + formatSalesAmount(p.PriceMax) + "원"
}

func (sp SalesItemSpec) EffectiveUnit() string {
	u := strings.TrimSpace(sp.Unit)
	if u == "" {
		return "EA"
	}
	return u
}

func SupplierRowUnit(row SalesItemSupplierPrice, spec SalesItemSpec) string {
	if u := strings.TrimSpace(row.Unit); u != "" {
		return u
	}
	return spec.EffectiveUnit()
}

func SpecMargins(specs []SalesItemSpec, prices []SalesItemSupplierPrice) []SalesItemSpecMargin {
	buy := map[string]int{}
	for _, p := range prices {
		id := strings.TrimSpace(p.SpecID)
		if id == "" || p.Price <= 0 {
			continue
		}
		if cur, ok := buy[id]; !ok || p.Price < cur {
			buy[id] = p.Price
		}
	}
	var out []SalesItemSpecMargin
	for _, sp := range specs {
		if !sp.IsActive {
			continue
		}
		m := SalesItemSpecMargin{Spec: sp, BuyMin: buy[sp.SpecID]}
		if sp.Price > 0 && m.BuyMin > 0 {
			m.Profit = sp.Price - m.BuyMin
			if sp.Price > 0 {
				pct := m.Profit * 100 / sp.Price
				m.Percent = strconv.Itoa(pct) + "%"
			}
		}
		out = append(out, m)
	}
	return out
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
		extra := it.SpecRangeLabel()
		if extra == "" {
			extra = it.ListPriceLabel()
		}
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
			OrgName:    it.ManufacturerLabel(),
			Extra:      extra,
			Bucket:     b,
			DelayBadge: "",
			DelayClass: "",
			AmountDesc: it.ListPrice,
		}
		if it.NeedsPartyLink() {
			card.DelayBadge = "연결 필요"
			card.DelayClass = "bg-rose-100 text-rose-800"
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

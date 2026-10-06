package handler

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type ItemsHandler struct {
	repo      *repository.SalesItemRepo
	customers *repository.CustomerRepo
	quotes    *repository.QuoteRepo
}

func NewItemsHandler(repo *repository.SalesItemRepo, customers *repository.CustomerRepo, quotes *repository.QuoteRepo) *ItemsHandler {
	return &ItemsHandler{repo: repo, customers: customers, quotes: quotes}
}

func (h *ItemsHandler) List(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	f := repository.SalesItemFilter{
		Search:   strings.TrimSpace(c.QueryParam("search")),
		Kind:     strings.TrimSpace(c.QueryParam("kind")),
		Category: strings.TrimSpace(c.QueryParam("category")),
		Review:   strings.TrimSpace(c.QueryParam("review")),
		Active:   strings.TrimSpace(c.QueryParam("active")),
	}
	display := model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view"))
	items, err := h.repo.List(f)
	if err != nil {
		return err
	}
	items = h.repo.AttachSpecSummaries(items)
	kanban := model.FillSalesItemKanban(items)
	q := itemsListQuery(f, "")
	listHref := "/items"
	if s := q.Encode(); s != "" {
		listHref += "?" + s
	}
	kq := cloneURLValues(q)
	kq.Set("display", "kanban")
	return c.Render(http.StatusOK, "items/list.html", map[string]interface{}{
		"Title": "품목 마스터", "Active": NavSalesItems,
		"Items": items, "Total": len(items),
		"Filter": f, "Search": f.Search, "Kind": f.Kind,
		"Category": f.Category, "Review": f.Review, "FilterActive": f.Active,
		"Kinds": model.SalesItemKindDefs(), "Categories": model.SalesItemCategories(),
		"Display": display, "ListHref": listHref, "KanbanHref": "/items?" + kq.Encode(),
		"FilterQ":       q.Encode(),
		"KanbanColumns": kanban.Columns, "KanbanTotal": kanban.Total,
		"KanbanDrag": canWriteSales(c), "KanbanDrop": "key",
		"KanbanHint":    "열 = 품목 성격. 끌어 옮기면 물품·AS·용역·기타가 바뀝니다.",
		"CanWrite":      canWriteSales(c),
		"QuoteCustomer": "",
		"Today":         time.Now().Format("2006-01-02"),
		"FlashOK":       c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
	})
}

func (h *ItemsHandler) New(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	return h.renderForm(c, &model.SalesItem{ItemKind: model.SalesItemKindGoods, Unit: "EA", IsActive: true}, true, "")
}

func (h *ItemsHandler) Create(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p := parseSalesItemForm(c)
	specs, prices := parseItemCatalog(c)
	applyFirstSpecToItem(p, specs)
	p.NeedsReview = false
	if err := h.repo.Create(p); err != nil {
		p.Specs, p.Suppliers = specs, groupPricesForForm(prices)
		return h.renderForm(c, p, true, err.Error())
	}
	if _, err := h.repo.ReplaceSpecsAndPrices(p.ItemID, specs, prices); err != nil {
		p.Specs, p.Suppliers = specs, groupPricesForForm(prices)
		return h.renderForm(c, p, false, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/items?ok=created")
}

func (h *ItemsHandler) Edit(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	p, err := h.repo.Get(c.Param("id"))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Redirect(http.StatusSeeOther, "/items?err=notfound")
		}
		return err
	}
	_ = h.repo.LoadCatalog(p)
	return h.renderForm(c, p, false, "")
}

func (h *ItemsHandler) Update(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	cur, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/items?err=notfound")
	}
	p := parseSalesItemForm(c)
	p.ItemID = cur.ItemID
	p.Spec, p.Unit, p.ListPrice = cur.Spec, cur.Unit, cur.ListPrice
	specs, prices := parseItemCatalog(c)
	applyFirstSpecToItem(p, specs)
	if c.FormValue("confirm_review") == "1" {
		p.NeedsReview = false
		p.IsActive = true
	} else {
		p.NeedsReview = cur.NeedsReview
	}
	if err := h.repo.Update(p); err != nil {
		p.Specs, p.Suppliers = specs, groupPricesForForm(prices)
		return h.renderForm(c, p, false, err.Error())
	}
	off, err := h.repo.ReplaceSpecsAndPrices(p.ItemID, specs, prices)
	if err != nil {
		p.Specs, p.Suppliers = specs, groupPricesForForm(prices)
		return h.renderForm(c, p, false, err.Error())
	}
	if len(off) > 0 {
		return c.Redirect(http.StatusSeeOther, "/items/"+p.ItemID+"/edit?ok=spec_off")
	}
	return c.Redirect(http.StatusSeeOther, "/items/"+p.ItemID+"/edit?ok=updated")
}

func (h *ItemsHandler) SetKind(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	kind := model.NormalizeSalesItemKind(c.FormValue("kind"))
	if err := h.repo.SetKind(id, kind); err != nil {
		if wantsJSON(c) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "저장에 실패했습니다"})
		}
		return err
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true})
	}
	back := strings.TrimSpace(c.FormValue("return"))
	if back == "" {
		back = "/items?display=kanban"
	}
	return c.Redirect(http.StatusSeeOther, back)
}

func (h *ItemsHandler) Confirm(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	if err := h.repo.Confirm(c.Param("id")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/items?err=notfound")
	}
	return c.Redirect(http.StatusSeeOther, "/items?ok=confirmed")
}

func (h *ItemsHandler) Suggest(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	items, err := h.repo.Suggest(c.QueryParam("q"), 20)
	if err != nil {
		return err
	}
	out := make([]map[string]interface{}, 0, len(items))
	for i := range items {
		_ = h.repo.LoadCatalog(&items[i])
		out = append(out, salesItemAPI(&items[i]))
	}
	return c.JSON(http.StatusOK, out)
}

func (h *ItemsHandler) QuoteLine(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	itemID := strings.TrimSpace(c.FormValue("item_id"))
	name := strings.TrimSpace(c.FormValue("name"))
	spec := strings.TrimSpace(c.FormValue("spec"))
	unit := strings.TrimSpace(c.FormValue("unit"))
	if unit == "" {
		unit = "EA"
	}
	price := parseItemMoney(c.FormValue("unit_price"))
	register := c.FormValue("register") == "1"

	var it *model.SalesItem
	if itemID != "" {
		got, err := h.repo.Get(itemID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "품목을 찾을 수 없습니다"})
		}
		it = got
	} else if name != "" {
		if got, err := h.repo.FindByName(name); err == nil {
			it = got
		}
	}

	if it != nil {
		_ = h.repo.LoadCatalog(it)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"ok": true, "ask_register": false, "item": salesItemAPI(it),
		})
	}

	if name == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "품명을 입력하세요"})
	}
	if register {
		it = &model.SalesItem{
			ItemKind:    model.SalesItemKindGoods,
			Name:        name,
			Spec:        spec,
			Unit:        unit,
			ListPrice:   price,
			IsActive:    true,
			NeedsReview: false,
		}
		if err := h.repo.Create(it); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		}
		_ = h.repo.LoadCatalog(it)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"ok": true, "ask_register": false, "registered": true, "item": salesItemAPI(it),
			"prompt": "",
		})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok":           true,
		"ask_register": true,
		"prompt":       "품목으로 등록할까요?",
		"name":         name,
		"spec":         spec,
		"unit":         unit,
		"unit_price":   price,
	})
}

func (h *ItemsHandler) CreatePartner(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	if h.customers == nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "고객 저장소를 쓸 수 없습니다"})
	}
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		name = strings.TrimSpace(c.QueryParam("name"))
	}
	confirm := c.FormValue("confirm") == "1"
	exist, err := h.customers.FindByExactOrgName(name)
	if err != nil {
		return err
	}
	if exist != nil && !confirm {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"ok": true, "existed": true, "ask_confirm": true,
			"prompt":   "같은 이름의 거래처가 있습니다 — 이것을 쓰시겠습니까?",
			"customer": customerBrief(exist),
		})
	}
	if exist != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"ok": true, "existed": true, "customer": customerBrief(exist),
		})
	}
	created, _, err := h.customers.CreatePartnerNameOnly(name)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok": true, "existed": false, "created": true, "customer": customerBrief(created),
	})
}

func customerBrief(c *model.Customer) map[string]interface{} {
	if c == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"customer_id": c.CustomerID, "org_name": c.OrgName, "party_kind": c.PartyKind,
	}
}

func (h *ItemsHandler) renderForm(c echo.Context, p *model.SalesItem, isNew bool, formErr string) error {
	title := "품목 등록"
	if !isNew {
		title = "품목 수정"
	}
	var uses []model.ItemQuoteUse
	if !isNew && h.quotes != nil && p != nil {
		uses, _ = h.quotes.ListByItemID(p.ItemID)
	}
	var margins []model.SalesItemSpecMargin
	var inactive []model.SalesItemSpec
	if p != nil {
		if len(p.Specs) == 0 && !isNew {
			_ = h.repo.LoadCatalog(p)
		}
		var prices []model.SalesItemSupplierPrice
		for _, b := range p.Suppliers {
			prices = append(prices, b.Rows...)
		}
		margins = model.SpecMargins(p.Specs, prices)
		for _, sp := range p.Specs {
			if !sp.IsActive {
				inactive = append(inactive, sp)
			}
		}
	}
	return c.Render(http.StatusOK, "items/form.html", map[string]interface{}{
		"Title": title, "Active": NavSalesItems, "IsNew": isNew,
		"Item": p, "FormError": formErr,
		"Kinds": model.SalesItemKindDefs(), "Categories": model.SalesItemCategories(),
		"Units":         model.SalesItemUnits(),
		"CanWrite":      canWriteSales(c),
		"FlashOK":       c.QueryParam("ok"),
		"Quotes":        uses,
		"Margins":       margins,
		"InactiveSpecs": inactive,
		"CatalogJSON":   itemCatalogJSON(p),
	})
}

func parseSalesItemForm(c echo.Context) *model.SalesItem {
	return &model.SalesItem{
		ItemKind:        model.NormalizeSalesItemKind(c.FormValue("item_kind")),
		Category:        strings.TrimSpace(c.FormValue("category")),
		Name:            strings.TrimSpace(c.FormValue("name")),
		Model:           strings.TrimSpace(c.FormValue("model")),
		Manufacturer:    strings.TrimSpace(c.FormValue("manufacturer")),
		ManufacturerID:  strings.TrimSpace(c.FormValue("manufacturer_id")),
		SupplierID:      strings.TrimSpace(c.FormValue("supplier_id")),
		GovItemNo:       strings.TrimSpace(c.FormValue("gov_item_no")),
		DefaultSupplier: strings.TrimSpace(c.FormValue("default_supplier")),
		IsActive:        c.FormValue("is_active") == "1",
		Notes:           strings.TrimSpace(c.FormValue("notes")),
	}
}

func parseItemCatalog(c echo.Context) ([]model.SalesItemSpec, []model.SalesItemSupplierPrice) {
	if c.Request().Form == nil {
		_ = c.Request().ParseForm()
	}
	form := c.Request().Form
	n := len(form["spec"])
	if len(form["spec_id"]) > n {
		n = len(form["spec_id"])
	}
	get := func(key string, i int) string {
		vals := form[key]
		if i < len(vals) {
			return vals[i]
		}
		return ""
	}
	var specs []model.SalesItemSpec
	for i := 0; i < n; i++ {
		specs = append(specs, model.SalesItemSpec{
			SpecID: strings.TrimSpace(get("spec_id", i)),
			Spec:   strings.TrimSpace(get("spec", i)),
			Unit:   strings.TrimSpace(get("spec_unit", i)),
			Price:  parseItemMoney(get("spec_price", i)),
		})
	}
	supIDs := form["sup_id"]
	var prices []model.SalesItemSupplierPrice
	for i := range form["sp_spec_id"] {
		gi := atoiQuiet(get("sp_group", i))
		sid := ""
		if gi >= 0 && gi < len(supIDs) {
			sid = strings.TrimSpace(supIDs[gi])
		}
		if sid == "" {
			sid = strings.TrimSpace(get("sp_supplier_id", i))
		}
		prices = append(prices, model.SalesItemSupplierPrice{
			SupplierID: sid,
			SpecID:     strings.TrimSpace(get("sp_spec_id", i)),
			Unit:       strings.TrimSpace(get("sp_unit", i)),
			Price:      parseItemMoney(get("sp_price", i)),
		})
	}
	return specs, prices
}

func applyFirstSpecToItem(p *model.SalesItem, specs []model.SalesItemSpec) {
	if p == nil || len(specs) == 0 {
		return
	}
	sp := specs[0]
	if strings.TrimSpace(p.Spec) == "" {
		p.Spec = strings.TrimSpace(sp.Spec)
	}
	if strings.TrimSpace(p.Unit) == "" {
		p.Unit = sp.EffectiveUnit()
	}
	if p.ListPrice == 0 {
		p.ListPrice = sp.Price
	}
}

func groupPricesForForm(prices []model.SalesItemSupplierPrice) []model.SalesItemSupplierBlock {
	idx := map[string]int{}
	var blocks []model.SalesItemSupplierBlock
	for _, p := range prices {
		sid := strings.TrimSpace(p.SupplierID)
		if sid == "" {
			continue
		}
		i, ok := idx[sid]
		if !ok {
			idx[sid] = len(blocks)
			blocks = append(blocks, model.SalesItemSupplierBlock{SupplierID: sid})
			i = len(blocks) - 1
		}
		blocks[i].Rows = append(blocks[i].Rows, p)
	}
	return blocks
}

type itemCatalogInitSpec struct {
	ID     string `json:"id"`
	Spec   string `json:"spec"`
	Unit   string `json:"unit"`
	Price  int    `json:"price"`
	Active bool   `json:"active"`
}

type itemCatalogInitRow struct {
	SpecID string `json:"specId"`
	Unit   string `json:"unit"`
	Price  int    `json:"price"`
}

type itemCatalogInitSup struct {
	SupplierID   string               `json:"supplierId"`
	SupplierName string               `json:"supplierName"`
	Rows         []itemCatalogInitRow `json:"rows"`
}

func itemCatalogJSON(p *model.SalesItem) template.JS {
	init := struct {
		Specs     []itemCatalogInitSpec `json:"specs"`
		Suppliers []itemCatalogInitSup  `json:"suppliers"`
	}{Specs: []itemCatalogInitSpec{}, Suppliers: []itemCatalogInitSup{}}
	if p != nil {
		for _, sp := range p.Specs {
			if !sp.IsActive {
				continue
			}
			init.Specs = append(init.Specs, itemCatalogInitSpec{
				ID: sp.SpecID, Spec: sp.Spec, Unit: sp.EffectiveUnit(), Price: sp.Price, Active: true,
			})
		}
		for _, b := range p.Suppliers {
			rows := make([]itemCatalogInitRow, 0, len(b.Rows))
			for _, r := range b.Rows {
				rows = append(rows, itemCatalogInitRow{SpecID: r.SpecID, Unit: r.Unit, Price: r.Price})
			}
			init.Suppliers = append(init.Suppliers, itemCatalogInitSup{
				SupplierID: b.SupplierID, SupplierName: b.SupplierName, Rows: rows,
			})
		}
	}
	b, err := json.Marshal(init)
	if err != nil {
		return template.JS(`{"specs":[],"suppliers":[]}`)
	}
	return template.JS(b)
}

func parseItemMoney(s string) int {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	if n < 0 {
		return 0
	}
	return n
}

func salesItemAPI(p *model.SalesItem) map[string]interface{} {
	if p == nil {
		return map[string]interface{}{}
	}
	fill := p.ListPrice
	if fill <= 0 {
		fill = p.LastPrice
	}
	var specs []map[string]interface{}
	for _, sp := range p.Specs {
		if !sp.IsActive {
			continue
		}
		specs = append(specs, map[string]interface{}{
			"spec_id": sp.SpecID, "spec": sp.Spec, "unit": sp.EffectiveUnit(), "price": sp.Price,
		})
		if fill <= 0 && sp.Price > 0 {
			fill = sp.Price
		}
	}
	if fill <= 0 && len(specs) > 0 {
		if pr, ok := specs[0]["price"].(int); ok {
			fill = pr
		}
	}
	return map[string]interface{}{
		"item_id":         p.ItemID,
		"item_kind":       p.ItemKind,
		"kind_label":      p.KindLabel(),
		"category":        p.Category,
		"name":            p.Name,
		"spec":            p.Spec,
		"model":           p.Model,
		"manufacturer":    p.ManufacturerLabel(),
		"manufacturer_id": p.ManufacturerID,
		"supplier_id":     p.SupplierID,
		"unit":            p.Unit,
		"gov_item_no":     p.GovItemNo,
		"list_price":      p.ListPrice,
		"last_price":      p.LastPrice,
		"fill_price":      fill,
		"last_quoted_at":  p.LastQuotedAt,
		"last_customer":   p.LastCustomer,
		"is_active":       p.IsActive,
		"needs_review":    p.NeedsReview,
		"specs":           specs,
	}
}

func itemsListQuery(f repository.SalesItemFilter, display string) url.Values {
	v := url.Values{}
	if f.Search != "" {
		v.Set("search", f.Search)
	}
	if f.Kind != "" {
		v.Set("kind", f.Kind)
	}
	if f.Category != "" {
		v.Set("category", f.Category)
	}
	if f.Review != "" {
		v.Set("review", f.Review)
	}
	if f.Active != "" {
		v.Set("active", f.Active)
	}
	if display != "" {
		v.Set("display", display)
	}
	return v
}

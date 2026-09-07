package handler

import (
	"database/sql"
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
	repo *repository.SalesItemRepo
}

func NewItemsHandler(repo *repository.SalesItemRepo) *ItemsHandler {
	return &ItemsHandler{repo: repo}
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
	p.NeedsReview = false
	if err := h.repo.Create(p); err != nil {
		return h.renderForm(c, p, true, err.Error())
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
	if c.FormValue("confirm_review") == "1" {
		p.NeedsReview = false
		p.IsActive = true
	} else {
		p.NeedsReview = cur.NeedsReview
	}
	if err := h.repo.Update(p); err != nil {
		return h.renderForm(c, p, false, err.Error())
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
	customer := strings.TrimSpace(c.FormValue("customer"))
	quotedAt := strings.TrimSpace(c.FormValue("quoted_at"))
	if quotedAt == "" {
		quotedAt = time.Now().Format("2006-01-02")
	}
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
		if err := h.repo.TouchLastQuoted(it.ItemID, price, customer, quotedAt); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		}
		fresh, _ := h.repo.Get(it.ItemID)
		if fresh == nil {
			fresh = it
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"ok": true, "ask_register": false, "item": salesItemAPI(fresh),
		})
	}

	if name == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "품명을 입력하세요"})
	}
	if register {
		it = &model.SalesItem{
			ItemKind:     model.SalesItemKindGoods,
			Name:         name,
			Spec:         spec,
			Unit:         unit,
			LastPrice:    price,
			LastCustomer: customer,
			LastQuotedAt: quotedAt,
			IsActive:     true,
			NeedsReview:  false,
		}
		if err := h.repo.Create(it); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		}
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

func (h *ItemsHandler) renderForm(c echo.Context, p *model.SalesItem, isNew bool, formErr string) error {
	title := "품목 등록"
	if !isNew {
		title = "품목 수정"
	}
	return c.Render(http.StatusOK, "items/form.html", map[string]interface{}{
		"Title": title, "Active": NavSalesItems, "IsNew": isNew,
		"Item": p, "FormError": formErr,
		"Kinds": model.SalesItemKindDefs(), "Categories": model.SalesItemCategories(),
		"Units":    model.SalesItemUnits(),
		"CanWrite": canWriteSales(c),
		"FlashOK":  c.QueryParam("ok"),
	})
}

func parseSalesItemForm(c echo.Context) *model.SalesItem {
	return &model.SalesItem{
		ItemKind:        model.NormalizeSalesItemKind(c.FormValue("item_kind")),
		Category:        strings.TrimSpace(c.FormValue("category")),
		Name:            strings.TrimSpace(c.FormValue("name")),
		Spec:            strings.TrimSpace(c.FormValue("spec")),
		Model:           strings.TrimSpace(c.FormValue("model")),
		Manufacturer:    strings.TrimSpace(c.FormValue("manufacturer")),
		Unit:            strings.TrimSpace(c.FormValue("unit")),
		GovItemNo:       strings.TrimSpace(c.FormValue("gov_item_no")),
		ListPrice:       parseItemMoney(c.FormValue("list_price")),
		LastPrice:       parseItemMoney(c.FormValue("last_price")),
		LastQuotedAt:    strings.TrimSpace(c.FormValue("last_quoted_at")),
		LastCustomer:    strings.TrimSpace(c.FormValue("last_customer")),
		DefaultSupplier: strings.TrimSpace(c.FormValue("default_supplier")),
		IsActive:        c.FormValue("is_active") == "1",
		Notes:           strings.TrimSpace(c.FormValue("notes")),
	}
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
	fill := p.LastPrice
	if fill <= 0 {
		fill = p.ListPrice
	}
	return map[string]interface{}{
		"item_id":        p.ItemID,
		"item_kind":      p.ItemKind,
		"kind_label":     p.KindLabel(),
		"category":       p.Category,
		"name":           p.Name,
		"spec":           p.Spec,
		"model":          p.Model,
		"manufacturer":   p.Manufacturer,
		"unit":           p.Unit,
		"gov_item_no":    p.GovItemNo,
		"list_price":     p.ListPrice,
		"last_price":     p.LastPrice,
		"fill_price":     fill,
		"last_quoted_at": p.LastQuotedAt,
		"last_customer":  p.LastCustomer,
		"is_active":      p.IsActive,
		"needs_review":   p.NeedsReview,
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

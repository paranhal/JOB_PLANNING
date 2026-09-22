package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type OrdersHandler struct {
	repo   *repository.OrderRepo
	quotes *repository.QuoteRepo
	sales  *repository.SalesRepo
	attach *repository.AttachmentRepo
}

func NewOrdersHandler(
	repo *repository.OrderRepo,
	quotes *repository.QuoteRepo,
	sales *repository.SalesRepo,
	attach *repository.AttachmentRepo,
) *OrdersHandler {
	return &OrdersHandler{repo: repo, quotes: quotes, sales: sales, attach: attach}
}

func (h *OrdersHandler) List(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	f := repository.OrderFilter{
		Search: strings.TrimSpace(c.QueryParam("search")),
		Status: strings.TrimSpace(c.QueryParam("status")),
	}
	display := model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view"))
	items, err := h.repo.List(f)
	if err != nil {
		return err
	}
	items = exposeOrders(c, items)
	kanban := model.FillOrderKanban(items)
	qv := url.Values{}
	if f.Search != "" {
		qv.Set("search", f.Search)
	}
	if f.Status != "" {
		qv.Set("status", f.Status)
	}
	listHref := "/orders"
	if s := qv.Encode(); s != "" {
		listHref += "?" + s
	}
	kq := cloneURLValues(qv)
	kq.Set("display", "kanban")
	see := canSeeMargin(c)
	today := time.Now().Format("2006-01-02")
	var confirmed int64
	for i := range items {
		confirmed += int64(items[i].Total)
	}
	return c.Render(http.StatusOK, "orders/list.html", map[string]interface{}{
		"Title": "수주", "Active": NavOrders,
		"Items": items, "Total": len(items),
		"Search": f.Search, "Status": f.Status,
		"Display": display, "ListHref": listHref, "KanbanHref": "/orders?" + kq.Encode(),
		"KanbanColumns": kanban.Columns, "KanbanTotal": kanban.Total,
		"KanbanDrag": canWriteSales(c), "KanbanDrop": "key",
		"KanbanHint":   "열 = 수주 → 발주 → 입고 → 납품 완료. 끌어 옮기면 상태가 바뀝니다.",
		"CanWrite":     canWriteSales(c),
		"CanSeeMargin": see,
		"Statuses":     model.OrderStatusDefs(),
		"Today":        today,
		"ConfirmedAmt": model.FormatSalesMoney(confirmed),
		"FlashOK":      c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
	})
}

func (h *OrdersHandler) FromQuote(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	qid := strings.TrimSpace(c.Param("id"))
	if qid == "" {
		qid = strings.TrimSpace(c.FormValue("quote_id"))
	}
	if h.quotes == nil {
		return echo.ErrNotFound
	}
	q, err := h.quotes.Get(qid)
	if err != nil || q == nil {
		return c.Redirect(http.StatusSeeOther, "/quotes?err=notfound")
	}
	o, err := h.repo.CreateFromQuote(q)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/quotes/"+q.QuoteID+"?err="+url.QueryEscape(err.Error()))
	}
	_ = h.quotes.SetStatus(q.QuoteID, model.QuoteStatusWon)
	h.afterOrder(c, o)
	return c.Redirect(http.StatusSeeOther, "/orders/"+o.OrderID+"?ok=created")
}

func (h *OrdersHandler) Show(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	o, err := h.repo.Get(c.Param("id"))
	if err != nil {
		if wantsJSON(c) {
			return c.JSON(http.StatusNotFound, map[string]interface{}{"ok": false, "error": "notfound"})
		}
		return c.Redirect(http.StatusSeeOther, "/orders?err=notfound")
	}
	see := canSeeMargin(c)
	exposeOrder(c, o)
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, publicOrderMap(o, see))
	}
	var atts []model.Attachment
	if h.attach != nil {
		atts, _ = h.attach.ListByRef(model.RefTypeSalesOrder, o.OrderID)
	}
	deliveryFiles := map[string][]model.Attachment{}
	if h.attach != nil {
		for _, d := range o.Deliveries {
			deliveryFiles[d.DeliveryID], _ = h.attach.ListByRef(model.RefTypeSalesDelivery, d.DeliveryID)
		}
	}
	return c.Render(http.StatusOK, "orders/show.html", map[string]interface{}{
		"Title": o.DisplayNo(), "Active": NavOrders, "Order": o,
		"CanWrite":      canWriteSales(c),
		"CanSeeMargin":  see,
		"Differs":       o.DiffersFromQuote(),
		"Attachments":   atts,
		"DeliveryFiles": deliveryFiles,
		"FlashOK":       c.QueryParam("ok"),
		"FlashErr":      c.QueryParam("err"),
	})
}

func (h *OrdersHandler) Edit(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	o, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/orders?err=notfound")
	}
	exposeOrder(c, o)
	return c.Render(http.StatusOK, "orders/form.html", map[string]interface{}{
		"Title": o.DisplayNo() + " 수정", "Active": NavOrders, "Order": o,
		"CanSeeMargin": canSeeMargin(c),
		"FormErr":      "",
	})
}

func (h *OrdersHandler) Update(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	cur, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/orders?err=notfound")
	}
	o := h.parseOrderHead(c, cur)
	if err := h.repo.Update(o); err != nil {
		exposeOrder(c, o)
		return c.Render(http.StatusOK, "orders/form.html", map[string]interface{}{
			"Title": cur.DisplayNo() + " 수정", "Active": NavOrders, "Order": o,
			"CanSeeMargin": canSeeMargin(c),
			"FormErr":      err.Error(),
		})
	}
	return c.Redirect(http.StatusSeeOther, "/orders/"+o.OrderID+"?ok=updated")
}

func (h *OrdersHandler) SetStatus(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	if err := h.repo.SetStatus(c.Param("id"), c.FormValue("status")); err != nil {
		if wantsJSON(c) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "저장에 실패했습니다"})
		}
		return err
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true})
	}
	return c.Redirect(http.StatusSeeOther, "/orders?display=kanban")
}

func (h *OrdersHandler) AddPurchase(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	if _, err := h.repo.Get(id); err != nil {
		return c.Redirect(http.StatusSeeOther, "/orders?err=notfound")
	}
	p := &model.SalesPurchase{
		OrderID:      id,
		LineID:       strings.TrimSpace(c.FormValue("line_id")),
		SupplierName: strings.TrimSpace(c.FormValue("supplier_name")),
		SupplierID:   strings.TrimSpace(c.FormValue("supplier_id")),
		PurchaseDate: strings.TrimSpace(c.FormValue("purchase_date")),
		Qty:          parseItemQty(c.FormValue("qty")),
		EtaDate:      strings.TrimSpace(c.FormValue("eta_date")),
		ArrivedDate:  strings.TrimSpace(c.FormValue("arrived_date")),
		Status:       c.FormValue("status"),
		Note:         strings.TrimSpace(c.FormValue("note")),
	}
	if canSeeMargin(c) {
		p.CostPrice = parseItemMoney(c.FormValue("cost_price"))
	}
	if err := h.repo.AddPurchase(p); err != nil {
		return c.Redirect(http.StatusSeeOther, "/orders/"+id+"?err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/orders/"+id+"?ok=purchased")
}

func (h *OrdersHandler) AddDelivery(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	if c.Request().Form == nil {
		_ = c.Request().ParseForm()
	}
	d := &model.SalesDelivery{
		OrderID:      id,
		LineID:       strings.TrimSpace(c.FormValue("line_id")),
		DeliveryDate: strings.TrimSpace(c.FormValue("delivery_date")),
		Qty:          parseItemQty(c.FormValue("qty")),
		Place:        strings.TrimSpace(c.FormValue("place")),
		ReceiverName: strings.TrimSpace(c.FormValue("receiver_name")),
		InstalledBy:  strings.TrimSpace(c.FormValue("installed_by")),
		Note:         strings.TrimSpace(c.FormValue("note")),
	}
	serials := c.Request().Form["serial"]
	_, err := h.repo.AddDelivery(d, serials, c.FormValue("install_location"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/orders/"+id+"?err="+url.QueryEscape(err.Error()))
	}
	if o, e := h.repo.Get(id); e == nil {
		h.afterDelivery(c, o)
	}
	return c.Redirect(http.StatusSeeOther, "/orders/"+id+"?ok=delivered")
}

func (h *OrdersHandler) parseOrderHead(c echo.Context, cur *model.SalesOrder) *model.SalesOrder {
	o := *cur
	o.PONo = strings.TrimSpace(c.FormValue("po_no"))
	o.PODate = strings.TrimSpace(c.FormValue("po_date"))
	if v := strings.TrimSpace(c.FormValue("recipient_name")); v != "" {
		o.RecipientName = v
	}
	if v := strings.TrimSpace(c.FormValue("title")); v != "" {
		o.Title = v
	}
	o.DueDate = strings.TrimSpace(c.FormValue("due_date"))
	o.Remarks = strings.TrimSpace(c.FormValue("remarks"))
	if v := strings.TrimSpace(c.FormValue("vat_mode")); v != "" {
		o.VATMode = model.NormalizeVATMode(v)
	}
	o.Lines = overlayOrderLines(cur.Lines, c)
	o.Purchases = nil
	return &o
}

func overlayOrderLines(cur []model.SalesOrderLine, c echo.Context) []model.SalesOrderLine {
	if c.Request().Form == nil {
		_ = c.Request().ParseForm()
	}
	ids := c.Request().Form["line_id"]
	if len(ids) == 0 {
		return cur
	}
	get := func(key string, i int) string {
		vals := c.Request().Form[key]
		if i < len(vals) {
			return vals[i]
		}
		return ""
	}
	byID := map[string]model.SalesOrderLine{}
	for _, ln := range cur {
		byID[ln.LineID] = ln
	}
	var out []model.SalesOrderLine
	for i, id := range ids {
		id = strings.TrimSpace(id)
		ln, ok := byID[id]
		if !ok {
			continue
		}
		if q := strings.TrimSpace(get("line_qty", i)); q != "" {
			ln.Qty = parseItemQty(q)
		}
		if p := strings.TrimSpace(get("line_price", i)); p != "" {
			ln.UnitPrice = parseItemMoney(p)
		}
		if u := strings.TrimSpace(get("line_unit", i)); u != "" {
			ln.Unit = u
		}
		out = append(out, ln)
	}
	if len(out) == 0 {
		return cur
	}
	return out
}

func (h *OrdersHandler) afterOrder(c echo.Context, o *model.SalesOrder) {
	if o == nil || o.SalesID == "" || h.sales == nil {
		return
	}
	p, err := h.sales.Get(o.SalesID)
	if err != nil || p == nil {
		return
	}
	uid, uname := ctxString(c, "user_id"), ctxString(c, "user_name")
	switch {
	case p.Stage == model.SalesStage4Bid && p.BidStatus == model.SalesBidPending:
		_ = h.sales.SetBidResult(p.SalesID, model.SalesBidWon, "", "", uid, uname)
	case p.Stage == model.SalesStage4Discover || p.Stage == model.SalesStage4Propose:
		_ = h.sales.ChangeStage(p.SalesID, model.SalesDirectWin, "", uid, uname, false)
	}
}

func (h *OrdersHandler) afterDelivery(c echo.Context, o *model.SalesOrder) {
	_ = c
	_ = o
}

func exposeOrder(c echo.Context, o *model.SalesOrder) *model.SalesOrder {
	if o == nil {
		return nil
	}
	if !canSeeMargin(c) {
		o.StripCost()
	}
	return o
}

func exposeOrders(c echo.Context, items []model.SalesOrder) []model.SalesOrder {
	if canSeeMargin(c) {
		return items
	}
	for i := range items {
		items[i].StripCost()
	}
	return items
}

func publicOrderMap(o *model.SalesOrder, seeMargin bool) map[string]interface{} {
	if o == nil {
		return map[string]interface{}{}
	}
	lines := make([]map[string]interface{}, 0, len(o.Lines))
	for _, ln := range o.Lines {
		row := map[string]interface{}{
			"line_id":     ln.LineID,
			"name":        ln.Name,
			"spec":        ln.Spec,
			"qty":         ln.Qty,
			"unit":        ln.Unit,
			"unit_price":  ln.UnitPrice,
			"amount":      ln.Amount,
			"quote_qty":   ln.QuoteQty,
			"quote_price": ln.QuoteUnitPrice,
			"differs":     ln.DiffersFromQuote(),
		}
		if seeMargin {
			row["cost"] = ln.Cost
			row["margin"] = ln.Margin
		}
		lines = append(lines, row)
	}
	purchases := make([]map[string]interface{}, 0, len(o.Purchases))
	for _, p := range o.Purchases {
		row := map[string]interface{}{
			"purchase_id":   p.PurchaseID,
			"line_id":       p.LineID,
			"supplier_name": p.SupplierName,
			"purchase_date": p.PurchaseDate,
			"qty":           p.Qty,
			"eta_date":      p.EtaDate,
			"arrived_date":  p.ArrivedDate,
			"status":        p.Status,
			"note":          p.Note,
		}
		if seeMargin {
			row["cost_price"] = p.CostPrice
		}
		purchases = append(purchases, row)
	}
	out := map[string]interface{}{
		"order_id":       o.OrderID,
		"quote_id":       o.QuoteID,
		"order_no":       o.OrderNo,
		"order_date":     o.OrderDate,
		"po_no":          o.PONo,
		"po_date":        o.PODate,
		"recipient_name": o.RecipientName,
		"title":          o.Title,
		"due_date":       o.DueDate,
		"vat_mode":       o.VATMode,
		"amount":         o.Amount,
		"vat":            o.VAT,
		"total":          o.Total,
		"status":         o.Status,
		"differs":        o.DiffersFromQuote(),
		"lines":          lines,
		"purchases":      purchases,
	}
	if seeMargin {
		out["cost_total"] = o.CostTotal
		out["margin"] = o.Margin
	}
	return out
}

func parseItemQty(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	n, _ := strconv.ParseFloat(s, 64)
	if n < 0 {
		return 0
	}
	return n
}

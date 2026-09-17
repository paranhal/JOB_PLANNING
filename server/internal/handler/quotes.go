package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type QuotesHandler struct {
	repo     *repository.QuoteRepo
	items    *repository.SalesItemRepo
	sales    *repository.SalesRepo
	users    *repository.UserRepo
	settings *repository.SettingsRepo
	orders   *repository.OrderRepo
}

func NewQuotesHandler(
	repo *repository.QuoteRepo,
	items *repository.SalesItemRepo,
	sales *repository.SalesRepo,
	users *repository.UserRepo,
	settings *repository.SettingsRepo,
	orders *repository.OrderRepo,
) *QuotesHandler {
	return &QuotesHandler{repo: repo, items: items, sales: sales, users: users, settings: settings, orders: orders}
}

func sortQuotes(items []model.SalesQuote, sortKey, dir string) {
	if len(items) == 0 || sortKey == "" {
		return
	}
	desc := dir == "desc"
	val := func(q model.SalesQuote) string {
		switch sortKey {
		case "quote_no":
			return strings.ToLower(q.DisplayNo())
		case "recipient":
			return strings.ToLower(q.RecipientName)
		case "title":
			return strings.ToLower(q.Title)
		case "total":
			return fmt.Sprintf("%020d", q.Total)
		case "status":
			return q.Status
		default:
			return q.QuoteDate
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := val(items[i]), val(items[j])
		if a != b {
			if desc {
				return a > b
			}
			return a < b
		}
		return false
	})
}

func (h *QuotesHandler) List(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	f := repository.QuoteFilter{
		Search:  strings.TrimSpace(c.QueryParam("search")),
		Status:  strings.TrimSpace(c.QueryParam("status")),
		SalesID: strings.TrimSpace(c.QueryParam("sales_id")),
		Purpose: strings.TrimSpace(c.QueryParam("purpose")),
	}
	display := model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view"))
	items, err := h.repo.List(f)
	if err != nil {
		return err
	}
	sortKey, dir := parseOptionalSort(c.QueryParam("sort"), c.QueryParam("dir"), "quote_no,quote_date,recipient,title,total,status")
	if display != "kanban" && sortKey != "" {
		sortQuotes(items, sortKey, dir)
	}
	kanban := model.FillQuoteKanban(items)
	qv := url.Values{}
	if f.Search != "" {
		qv.Set("search", f.Search)
	}
	if f.Status != "" {
		qv.Set("status", f.Status)
	}
	if f.SalesID != "" {
		qv.Set("sales_id", f.SalesID)
	}
	if f.Purpose != "" {
		qv.Set("purpose", f.Purpose)
	}
	if sortKey != "" {
		qv.Set("sort", sortKey)
		qv.Set("dir", dir)
	}
	listHref := "/quotes"
	if s := qv.Encode(); s != "" {
		listHref += "?" + s
	}
	kq := cloneURLValues(qv)
	kq.Set("display", "kanban")
	today := time.Now().Format("2006-01-02")
	expiredN := 0
	for i := range items {
		if items[i].IsExpiredSent(today) {
			expiredN++
		}
	}
	filter := cloneURLValues(qv)
	filter.Del("sort")
	filter.Del("dir")
	if display == "kanban" {
		filter.Set("display", "kanban")
	}
	hrefs := sortLinkHrefs("/quotes", filter, []string{"quote_no", "quote_date", "recipient", "title", "total", "status"}, sortKey, dir)
	return c.Render(http.StatusOK, "quotes/list.html", map[string]interface{}{
		"Title": "견적", "Active": NavQuotes,
		"Items": items, "Total": len(items),
		"Search": f.Search, "Status": f.Status, "SalesID": f.SalesID, "Purpose": f.Purpose,
		"Display": display, "ListHref": listHref, "KanbanHref": "/quotes?" + kq.Encode(),
		"KanbanColumns": kanban.Columns, "KanbanTotal": kanban.Total,
		"KanbanDrag": canWriteSales(c), "KanbanDrop": "key",
		"KanbanHint": "열 = 상태. 끌어 옮기면 견적 상태가 바뀝니다.",
		"CanWrite":   canWriteSales(c),
		"Statuses":   model.QuoteStatusDefs(),
		"FlashOK":    c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"NewHref":      "/quotes/new",
		"ExpiredCount": expiredN,
		"Sort":         sortKey,
		"Dir":          dir,
		"SortHref":     hrefs,
		"SortSelect":   sortSelectOptions(quoteListSortCols(), hrefs, sortKey, dir),
	})
}

func (h *QuotesHandler) New(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	form := model.NormalizeQuoteForm(c.QueryParam("form"))
	q := &model.SalesQuote{
		FormType:       form,
		VATMode:        model.DefaultVATMode(form),
		RoundRule:      h.repo.LastRoundRule(),
		QuoteDate:      time.Now().Format("2006-01-02"),
		RecipientKind:  model.QuoteRecipientCustomer,
		Purpose:        model.QuotePurposeDeal,
		Status:         model.QuoteStatusDraft,
		SalesID:        strings.TrimSpace(c.QueryParam("sales_id")),
		OwnerName:      ctxString(c, "user_name"),
		OwnerUserID:    ctxString(c, "user_id"),
		ValidUntilText: h.setting(repository.SettingQuoteValid),
		DueText:        h.setting(repository.SettingQuoteDue),
		PlaceText:      h.setting(repository.SettingQuotePlace),
		PaymentText:    h.setting(repository.SettingQuotePayment),
		Lines:          []model.SalesQuoteLine{{Unit: "EA", Qty: 1}, {Unit: "EA"}, {Unit: "EA"}},
	}
	oh, tech := h.repo.StandardRates()
	q.OverheadRate, q.TechFeeRate = oh, tech
	if q.SalesID != "" && h.sales != nil {
		if p, err := h.sales.Get(q.SalesID); err == nil && p != nil {
			q.RecipientName = p.CustomerValue()
			q.Title = p.Name
		}
	}
	return h.renderForm(c, q, true, "")
}

func (h *QuotesHandler) Create(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	q := h.parseQuoteForm(c)
	q.QuoteDate = time.Now().Format("2006-01-02")
	q.QuoteNo = ""
	q.IsLegacy = false
	if err := h.repo.Create(q); err != nil {
		return h.renderForm(c, q, true, err.Error())
	}
	h.afterSave(c, q)
	loc := "/quotes/" + q.QuoteID + "?ok=created"
	if ask := unmatchedQuoteNames(q); ask != "" {
		loc += "&ask=" + url.QueryEscape(ask)
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

func (h *QuotesHandler) Show(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	q, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/quotes?err=notfound")
	}
	tot := q.Totals()
	var ask []string
	for _, s := range strings.Split(c.QueryParam("ask"), "|") {
		if t := strings.TrimSpace(s); t != "" {
			ask = append(ask, t)
		}
	}
	latest, _ := h.repo.IsLatest(q)
	revs, _ := h.repo.ListByQuoteNo(q.QuoteNo)
	stdOH, stdTech := h.repo.StandardRates()
	var groups []model.QuoteGroupSubtotal
	if model.QuoteFormIsGrouped(q.FormType) {
		groups = model.GroupSubtotals(q.Lines)
	}
	var linked []model.SalesOrder
	if h.orders != nil {
		linked, _ = h.orders.ListByQuote(q.QuoteID)
	}
	return c.Render(http.StatusOK, "quotes/show.html", map[string]interface{}{
		"Title": q.DisplayNo(), "Active": NavQuotes, "Quote": q, "Totals": tot,
		"CanWrite":     canWriteSales(c),
		"FlashOK":      c.QueryParam("ok"),
		"AskNames":     ask,
		"RoundLabel":   model.RoundRuleLabel(q.RoundRule),
		"VATLabel":     model.VATModeLabel(q.VATMode),
		"IsLatest":     latest,
		"Revs":         revs,
		"Groups":       groups,
		"OverheadDiff": model.RateDiffLabel(q.OverheadRate, stdOH),
		"TechDiff":     model.RateDiffLabel(q.TechFeeRate, stdTech),
		"QuoteYear":    q.QuoteYear(),
		"Orders":       linked,
	})
}

func (h *QuotesHandler) Edit(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	q, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/quotes?err=notfound")
	}
	if ok, _ := h.repo.IsLatest(q); !ok {
		return c.Redirect(http.StatusSeeOther, "/quotes/"+q.QuoteID+"?err=readonly")
	}
	return h.renderForm(c, q, false, "")
}

func (h *QuotesHandler) Update(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	cur, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/quotes?err=notfound")
	}
	q := h.parseQuoteForm(c)
	q.QuoteID = cur.QuoteID
	if err := h.repo.Update(q); err != nil {
		q.QuoteNo, q.QuoteDate = cur.QuoteNo, cur.QuoteDate
		return h.renderForm(c, q, false, err.Error())
	}
	h.afterSave(c, q)
	return c.Redirect(http.StatusSeeOther, "/quotes/"+q.QuoteID+"?ok=updated")
}

func (h *QuotesHandler) Copy(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	today := time.Now().Format("2006-01-02")
	dst, err := h.repo.CopyAsNew(c.Param("id"), today)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/quotes?err=notfound")
	}
	return c.Redirect(http.StatusSeeOther, "/quotes/"+dst.QuoteID+"/edit?ok=copied")
}

func (h *QuotesHandler) Revise(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	dst, err := h.repo.Revise(c.Param("id"), c.FormValue("rev_reason"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/quotes/"+c.Param("id")+"?err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/quotes/"+dst.QuoteID+"/edit?ok=revised")
}

func (h *QuotesHandler) SetStatus(c echo.Context) error {
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
	return c.Redirect(http.StatusSeeOther, "/quotes?display=kanban")
}

func (h *QuotesHandler) Download(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	q, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return echo.ErrNotFound
	}
	raw, err := FillQuoteWorkbook(q, h.repo.Company())
	if err != nil {
		return err
	}
	name := strings.ReplaceAll(q.DisplayNo(), "/", "-") + ".xlsx"
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	return c.Blob(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", raw)
}

func (h *QuotesHandler) Preview(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	if c.Request().Form == nil {
		_ = c.Request().ParseForm()
	}
	q := h.parseQuoteForm(c)
	_ = model.ApplyQuoteTotals(q, q.Lines)
	tot := q.Totals()
	stdOH, stdTech := h.repo.StandardRates()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok":                 true,
		"subtotal":           tot.LineSum,
		"supply":             tot.Supply,
		"vat":                tot.VAT,
		"total":              tot.Total,
		"total_before_round": tot.TotalBeforeRound,
		"direct":             tot.Direct,
		"overhead":           tot.Overhead,
		"tech_fee":           tot.TechFee,
		"round_label":        model.RoundRuleLabel(q.RoundRule),
		"vat_label":          model.VATModeLabel(q.VATMode),
		"overhead_diff":      model.RateDiffLabel(q.OverheadRate, stdOH),
		"tech_diff":          model.RateDiffLabel(q.TechFeeRate, stdTech),
		"lines":              len(q.Lines),
	})
}

func (h *QuotesHandler) renderForm(c echo.Context, q *model.SalesQuote, isNew bool, formErr string) error {
	users, _ := h.users.ListAssignable()
	title := "견적 작성"
	if !isNew {
		title = "견적 수정"
	}
	tot := q.Totals()
	oh, tech := h.repo.StandardRates()
	year := q.QuoteYear()
	if year <= 0 {
		year = time.Now().Year()
	}
	rates, _ := h.repo.ListLaborRates(year)
	return c.Render(http.StatusOK, "quotes/form.html", map[string]interface{}{
		"Title": title, "Active": NavQuotes, "IsNew": isNew, "Quote": q,
		"Totals": tot, "FormError": formErr, "Users": users,
		"CanWrite":     canWriteSales(c),
		"RoundOptions": [][]string{{"none", "절사 없음"}, {"hundred", "100원 단위 절사"}, {"thousand", "1,000원 단위 절사"}},
		"Units":        model.SalesItemUnits(),
		"FlashOK":      c.QueryParam("ok"),
		"LaborRates":   rates,
		"StandardOH":   oh,
		"StandardTech": tech,
		"OverheadDiff": model.RateDiffLabel(q.OverheadRate, oh),
		"TechDiff":     model.RateDiffLabel(q.TechFeeRate, tech),
		"QuoteYear":    year,
	})
}

func (h *QuotesHandler) parseQuoteForm(c echo.Context) *model.SalesQuote {
	if c.Request().Form == nil {
		_ = c.Request().ParseForm()
	}
	form := model.NormalizeQuoteForm(c.FormValue("form_type"))
	vat := strings.TrimSpace(c.FormValue("vat_mode"))
	if vat == "" {
		vat = model.DefaultVATMode(form)
	}
	q := &model.SalesQuote{
		SalesID:        strings.TrimSpace(c.FormValue("sales_id")),
		FormType:       form,
		RecipientKind:  strings.TrimSpace(c.FormValue("recipient_kind")),
		CustomerID:     strings.TrimSpace(c.FormValue("customer_id")),
		RecipientName:  strings.TrimSpace(c.FormValue("recipient_name")),
		AttnName:       strings.TrimSpace(c.FormValue("attn_name")),
		AttnTitle:      strings.TrimSpace(c.FormValue("attn_title")),
		Title:          strings.TrimSpace(c.FormValue("title")),
		ValidUntilText: strings.TrimSpace(c.FormValue("valid_until_text")),
		DueText:        strings.TrimSpace(c.FormValue("due_text")),
		PlaceText:      strings.TrimSpace(c.FormValue("place_text")),
		PaymentText:    strings.TrimSpace(c.FormValue("payment_text")),
		VATMode:        model.NormalizeVATMode(vat),
		RoundRule:      model.NormalizeRoundRule(c.FormValue("round_rule")),
		OwnerUserID:    strings.TrimSpace(c.FormValue("owner_user_id")),
		OwnerName:      strings.TrimSpace(c.FormValue("owner_name")),
		OwnerPhone:     strings.TrimSpace(c.FormValue("owner_phone")),
		Remarks:        strings.TrimSpace(c.FormValue("remarks")),
		Purpose:        model.NormalizeQuotePurpose(c.FormValue("purpose")),
		Status:         model.NormalizeQuoteStatus(c.FormValue("status")),
		Lines:          parseQuoteLines(c),
		OverheadRate:   parseQuoteRate(c.FormValue("overhead_rate")),
		TechFeeRate:    parseQuoteRate(c.FormValue("tech_fee_rate")),
		TargetTotal:    parseItemMoney(c.FormValue("target_total")),
		MaintBlock:     c.FormValue("maint_block") == "1" || c.FormValue("maint_block") == "on",
		IsReverseCalc:  c.FormValue("is_reverse_calc") == "1",
	}
	if y, err := strconv.Atoi(strings.TrimSpace(c.FormValue("budget_year"))); err == nil {
		q.BudgetYear = y
	}
	if q.TargetTotal > 0 {
		q.IsReverseCalc = true
		q.Lines = model.ReverseQuoteLines(q.Lines, q.TargetTotal, q.VATMode, q.RoundRule)
	}
	if q.Purpose == "" {
		q.Purpose = model.QuotePurposeDeal
	}
	if q.OwnerName == "" && q.OwnerUserID != "" && h.users != nil {
		if u, err := h.users.GetByID(q.OwnerUserID); err == nil && u != nil {
			q.OwnerName = strings.TrimSpace(u.FullName)
		}
	}
	return q
}

func parseQuoteLines(c echo.Context) []model.SalesQuoteLine {
	if c.Request().Form == nil {
		_ = c.Request().ParseForm()
	}
	names := c.Request().Form["line_name"]
	n := len(names)
	get := func(key string, i int) string {
		vals := c.Request().Form[key]
		if i < len(vals) {
			return vals[i]
		}
		return ""
	}
	var lines []model.SalesQuoteLine
	for i := 0; i < n; i++ {
		qty, _ := strconv.ParseFloat(strings.ReplaceAll(get("line_qty", i), ",", ""), 64)
		mm, _ := strconv.ParseFloat(strings.ReplaceAll(get("line_mm", i), ",", ""), 64)
		disc, _ := strconv.ParseFloat(strings.ReplaceAll(get("line_disc", i), ",", ""), 64)
		if disc > 1 {
			disc = disc / 100
		}
		if mm > 1 {
			mm = mm / 100
		}
		lines = append(lines, model.SalesQuoteLine{
			ItemID:          strings.TrimSpace(get("line_item_id", i)),
			GroupLabel:      strings.TrimSpace(get("line_group", i)),
			Name:            strings.TrimSpace(get("line_name", i)),
			Spec:            strings.TrimSpace(get("line_spec", i)),
			Qty:             qty,
			Unit:            strings.TrimSpace(get("line_unit", i)),
			UnitPrice:       parseItemMoney(get("line_price", i)),
			GovPrice:        parseItemMoney(get("line_gov", i)),
			MMRate:          mm,
			DiscountRate:    disc,
			Note:            strings.TrimSpace(get("line_note", i)),
			RateID:          strings.TrimSpace(get("line_rate_id", i)),
			LaborYear:       atoiQuiet(get("line_labor_year", i)),
			PriceOverridden: get("line_price_overridden", i) == "1",
		})
	}
	return lines
}

func (h *QuotesHandler) afterSave(c echo.Context, q *model.SalesQuote) {
	if q == nil {
		return
	}
	cust := q.RecipientName
	for _, ln := range q.Lines {
		if strings.TrimSpace(ln.ItemID) == "" || h.items == nil {
			continue
		}
		_ = h.items.TouchLastQuoted(ln.ItemID, ln.UnitPrice, cust, q.QuoteDate)
	}
	if q.SalesID == "" || h.sales == nil {
		return
	}
	if model.NormalizeQuotePurpose(q.Purpose) != model.QuotePurposeDeal {
		return
	}
	p, err := h.sales.Get(q.SalesID)
	if err != nil || p == nil || !p.IsSupply() {
		return
	}
	if p.Stage == model.SalesStageInquiry || p.Stage == "" {
		_ = h.sales.ChangeStage(p.SalesID, model.SalesStageQuoted, "", ctxString(c, "user_id"), ctxString(c, "user_name"), false)
	}
}

func unmatchedQuoteNames(q *model.SalesQuote) string {
	if q == nil {
		return ""
	}
	var names []string
	seen := map[string]bool{}
	for _, ln := range q.Lines {
		if strings.TrimSpace(ln.ItemID) != "" || ln.Name == "" {
			continue
		}
		if seen[ln.Name] {
			continue
		}
		seen[ln.Name] = true
		names = append(names, ln.Name)
	}
	return strings.Join(names, "|")
}

func (h *QuotesHandler) setting(key string) string {
	if h.settings == nil {
		return ""
	}
	v, _ := h.settings.Get(key)
	return strings.TrimSpace(v)
}

func parseQuoteRate(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	s = strings.TrimSuffix(s, "%")
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

func atoiQuiet(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func (h *QuotesHandler) LaborRates(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	year := atoiQuiet(c.QueryParam("year"))
	if year <= 0 {
		year = time.Now().Year()
	}
	items, err := h.repo.ListLaborRates(year)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		items, _ = h.repo.ListLaborRates(2026)
		if year != 2026 && len(items) > 0 {
			year = 2026
		}
	}
	oh, tech := h.repo.StandardRates()
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, items)
	}
	return c.Render(http.StatusOK, "quotes/labor_rates.html", map[string]interface{}{
		"Title": "노임단가", "Active": NavQuotes, "Year": year, "Items": items,
		"Overhead": oh, "TechFee": tech, "CanWrite": canWriteSales(c),
		"FlashOK": c.QueryParam("ok"),
	})
}

func (h *QuotesHandler) SaveStandardRates(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	if err := h.repo.SetStandardRates(parseQuoteRate(c.FormValue("overhead_rate")), parseQuoteRate(c.FormValue("tech_fee_rate"))); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/labor-rates?ok=saved")
}

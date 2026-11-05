package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/appclock"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *SalesHandler) quoteSetting(key string) string {
	if h.settings == nil {
		return ""
	}
	v, _ := h.settings.Get(key)
	return strings.TrimSpace(v)
}

func (h *SalesHandler) QuickQuoteNew(c echo.Context) error {
	if !canWriteSalesDeal(c) {
		return echo.ErrForbidden
	}
	u := h.loginUser(c)
	ownerPhone := model.OwnerPhoneFromUser(u)
	ownerName := ctxString(c, "user_name")
	if u != nil {
		if n := model.PersonDisplayName(u.FullName, u.Username, u.UserID); n != "" {
			ownerName = n
		}
	}
	q := &model.SalesQuote{
		FormType:       model.QuoteFormA,
		QuoteKind:      model.QuoteKindSolution,
		VATMode:        model.QuoteVATExcluded,
		RoundRule:      model.QuoteRoundNone,
		CutScope:       model.QuoteCutTotal,
		QuoteDate:      appclock.Now().Format("2006-01-02"),
		Status:         model.QuoteStatusDraft,
		Purpose:        model.QuotePurposeDeal,
		RecipientKind:  model.QuoteRecipientCustomer,
		OwnerName:      ownerName,
		OwnerUserID:    ctxString(c, "user_id"),
		OwnerPhone:     ownerPhone,
		ValidUntilText: h.quoteSetting(repository.SettingQuoteValid),
		DueText:        h.quoteSetting(repository.SettingQuoteDue),
		PlaceText:      h.quoteSetting(repository.SettingQuotePlace),
		PaymentText:    h.quoteSetting(repository.SettingQuotePayment),
		Lines:          []model.SalesQuoteLine{{Unit: "EA", Qty: 1}, {Unit: "EA"}, {Unit: "EA"}},
	}
	return h.renderQuickQuote(c, q, "")
}

func (h *SalesHandler) renderQuickQuote(c echo.Context, q *model.SalesQuote, formErr string) error {
	if q == nil {
		q = &model.SalesQuote{}
	}
	return c.Render(http.StatusOK, "sales/quick_quote.html", map[string]interface{}{
		"Title":             "빠른 견적",
		"Active":            NavSalesPipeline,
		"Quote":             q,
		"FormError":         formErr,
		"RoundOptions":      model.QuoteRoundOptions(),
		"Units":             model.SalesItemUnits(),
		"OwnerPhoneMissing": strings.TrimSpace(q.OwnerPhone) == "",
		"CanWrite":          true,
	})
}

func (h *SalesHandler) QuickQuoteCreate(c echo.Context) error {
	if !canWriteSalesDeal(c) {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	u := h.loginUser(c)
	ownerPhone := model.OwnerPhoneFromUser(u)
	if ownerPhone == "" {
		return quickQuoteErr(c, "프로필에 연락처를 등록하세요", http.StatusBadRequest)
	}
	org, err := resolveInsertOrg(c)
	if err != nil {
		return quickQuoteErr(c, err.Error(), http.StatusBadRequest)
	}

	newCust := c.FormValue("new_customer") == "1"
	customerID := strings.TrimSpace(c.FormValue("customer_id"))
	customerName := model.CollapseNewlines(c.FormValue("customer_name"))
	attn := model.CollapseNewlines(c.FormValue("attn_name"))
	contactID := strings.TrimSpace(c.FormValue("contact_id"))
	contactLine := model.CollapseNewlines(c.FormValue("contact_line"))
	titleManual := c.FormValue("title_manual") == "1"
	title := model.CollapseNewlines(c.FormValue("title"))
	lines := parseQuoteLines(c)
	var filled []model.SalesQuoteLine
	for _, ln := range lines {
		ln.Name = model.CollapseNewlines(ln.Name)
		ln.Spec = model.CollapseNewlines(ln.Spec)
		if ln.Name == "" {
			continue
		}
		if ln.Qty <= 0 || ln.UnitPrice <= 0 {
			return quickQuoteErr(c, "품목의 품명·수량·단가를 입력하세요", http.StatusBadRequest)
		}
		if ln.Unit == "" {
			ln.Unit = "EA"
		}
		filled = append(filled, ln)
	}

	var errs []string
	if newCust {
		if customerName == "" {
			errs = append(errs, "거래처명")
		}
		if attn == "" {
			errs = append(errs, "담당자명")
		}
		if err := model.ValidateContactLine(contactLine); err != nil {
			errs = append(errs, err.Error())
		}
	} else {
		if customerID == "" {
			errs = append(errs, "고객")
		}
		if attn == "" {
			errs = append(errs, "수신인")
		}
		if err := model.ValidateContactLine(contactLine); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(filled) == 0 {
		errs = append(errs, "품목(품명·수량·단가)")
	}
	if len(errs) > 0 {
		return quickQuoteErr(c, strings.Join(errs, ", "), http.StatusBadRequest)
	}
	if !titleManual || title == "" {
		title = model.AutoQuoteTitle(filled)
	}

	ownerName := ctxString(c, "user_name")
	ownerID := ctxString(c, "user_id")
	if u != nil {
		if n := model.PersonDisplayName(u.FullName, u.Username, u.UserID); n != "" {
			ownerName = n
		}
	}

	q := &model.SalesQuote{
		FormType:       model.QuoteFormA,
		QuoteKind:      model.QuoteKindSolution,
		VATMode:        model.NormalizeVATMode(c.FormValue("vat_mode")),
		RoundRule:      model.NormalizeRoundRule(c.FormValue("round_rule")),
		CutScope:       model.NormalizeCutScope(c.FormValue("cut_scope")),
		QuoteDate:      appclock.Now().Format("2006-01-02"),
		Title:          title,
		RecipientKind:  model.QuoteRecipientCustomer,
		RecipientName:  customerName,
		AttnName:       attn,
		OwnerName:      ownerName,
		OwnerUserID:    ownerID,
		OwnerPhone:     ownerPhone,
		Remarks:        model.CollapseNewlines(c.FormValue("remarks")),
		ValidUntilText: qqFirstNonEmpty(c.FormValue("valid_until_text"), h.quoteSetting(repository.SettingQuoteValid)),
		DueText:        qqFirstNonEmpty(c.FormValue("due_text"), h.quoteSetting(repository.SettingQuoteDue)),
		PlaceText:      qqFirstNonEmpty(c.FormValue("place_text"), h.quoteSetting(repository.SettingQuotePlace)),
		PaymentText:    qqFirstNonEmpty(c.FormValue("payment_text"), h.quoteSetting(repository.SettingQuotePayment)),
		Lines:          filled,
		Status:         model.QuoteStatusDraft,
		Purpose:        model.QuotePurposeDeal,
		OrgID:          org,
	}
	if err := model.ApplyQuoteTotals(q, filled); err != nil {
		return quickQuoteErr(c, err.Error(), http.StatusBadRequest)
	}

	p := &model.SalesProject{
		Name:         title,
		OrgID:        org,
		SalesOwnerID: ownerID,
		SalesOwner:   ownerName,
		Notes:        q.Remarks,
	}
	if u != nil {
		p.SalesOwnerIDs = []string{u.UserID}
	}

	addContact := contactID == ""
	updateContact := false
	if !newCust && contactID != "" && h.contactRepo != nil {
		if ct, err := h.contactRepo.GetByID(contactID); err == nil && ct != nil {
			updateContact = model.ContactLineChanged(ct.Mobile, ct.Phone, ct.Email, contactLine)
			if cust, err := h.customerRepo.GetByID(customerID); err == nil && cust != nil && customerName == "" {
				customerName = cust.OrgName
				q.RecipientName = cust.OrgName
			}
		} else {
			addContact = true
		}
	}
	if !newCust && q.RecipientName == "" && h.customerRepo != nil {
		if cust, err := h.customerRepo.GetByID(customerID); err == nil && cust != nil {
			q.RecipientName = cust.OrgName
			customerName = cust.OrgName
		}
	}

	res, err := repository.SaveQuickQuote(h.db, repository.QuickQuoteInput{
		OrgID:         org,
		NewCustomer:   newCust,
		CustomerID:    customerID,
		CustomerName:  customerName,
		ContactID:     contactID,
		AttnName:      attn,
		ContactLine:   contactLine,
		AddContact:    addContact,
		UpdateContact: updateContact,
		Project:       p,
		Quote:         q,
	})
	if err != nil {
		msg := err.Error()
		if errors.Is(err, repository.ErrCustomerNameTaken) {
			msg = "이미 등록된 거래처입니다. 체크를 해제하고 검색해 선택하세요."
		}
		return quickQuoteErr(c, msg, http.StatusBadRequest)
	}
	tot := res.Quote.Totals()
	cut := model.QuoteCutDisplay(res.Quote.RoundRule, res.Quote.CutScope, tot.CutAmount)
	out := map[string]interface{}{
		"ok":           true,
		"sales_id":     res.Project.SalesID,
		"quote_id":     res.Quote.QuoteID,
		"quote_no":     res.Quote.DisplayNo(),
		"total":        res.Quote.Total,
		"cut_label":    cut,
		"master_note":  res.MasterNote,
		"project_note": "사업이 함께 만들어졌습니다",
		"xlsx":         "/quotes/" + res.Quote.QuoteID + "/xlsx",
		"detail":       "/quotes/" + res.Quote.QuoteID,
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, out)
	}
	return c.Redirect(http.StatusSeeOther, "/quotes/"+res.Quote.QuoteID+"?ok=quick")
}

func quickQuoteErr(c echo.Context, msg string, code int) error {
	if wantsJSON(c) {
		return c.JSON(code, map[string]interface{}{"ok": false, "error": msg})
	}
	return c.JSON(code, map[string]interface{}{"ok": false, "error": msg})
}

func qqFirstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

func (h *SalesHandler) loginUser(c echo.Context) *model.User {
	if h == nil || h.userRepo == nil {
		return nil
	}
	id := currentUserID(c)
	if id != "" {
		if u, err := h.userRepo.GetByID(id); err == nil && u != nil {
			return u
		}
	}
	name := strings.TrimSpace(ctxString(c, "username"))
	if name == "" {
		name = strings.TrimSpace(ctxString(c, "user_name"))
	}
	if name != "" {
		if u, err := h.userRepo.GetByUsername(name); err == nil {
			return u
		}
	}
	return nil
}

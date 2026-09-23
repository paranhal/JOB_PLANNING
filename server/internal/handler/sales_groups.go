package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *SalesHandler) Groups(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	closed := c.QueryParam("closed") == "1"
	items, err := h.repo.ListGroups(closed)
	if err != nil {
		return err
	}
	rows := make([]map[string]interface{}, 0, len(items))
	allIDs := []string{}
	byGroup := map[string][]model.SalesProject{}
	for i := range items {
		ps, _ := h.repo.ResolveGroupProjects(&items[i])
		byGroup[items[i].GroupID] = ps
		for _, p := range ps {
			allIDs = append(allIDs, p.SalesID)
		}
	}
	quotes, _ := h.repo.QuotesBySalesIDs(allIDs)
	qm := model.GroupQuotesBySales(quotes)
	for i := range items {
		g := items[i]
		ps := byGroup[g.GroupID]
		cnt, amt, w := model.SalesGroupTotalsAllowDup(ps, qm)
		stageN := map[string]int{}
		for _, p := range ps {
			if p.Status == model.SalesStatusDormant {
				continue
			}
			if p.CloseReason == model.SalesCloseLost || p.CloseReason == model.SalesCloseDropped {
				continue
			}
			stageN[p.Stage]++
		}
		rows = append(rows, map[string]interface{}{
			"Group": g, "Count": cnt, "Amount": model.FormatSalesMoney(int64(amt)), "Weighted": model.FormatSalesMoney(int64(w)),
			"Avg": model.SalesAvgProbLabel(amt, w), "StageCounts": stageN,
		})
	}
	return c.Render(http.StatusOK, "sales/groups.html", map[string]interface{}{
		"Title": "사업 대분류", "Active": NavSalesGroups, "Rows": rows, "IncludeClosed": closed, "CanWrite": canWriteSales(c),
	})
}

func (h *SalesHandler) GroupNew(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	return h.renderGroupForm(c, &model.SalesGroup{GroupKind: model.SalesGroupManual, Year: 0}, true, "")
}

func (h *SalesHandler) GroupCreate(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	g := h.parseGroupForm(c)
	if err := h.repo.CreateGroup(g); err != nil {
		return h.renderGroupForm(c, g, true, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales/groups/"+g.GroupID)
}

func (h *SalesHandler) GroupShow(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	g, err := h.repo.GetGroup(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales/groups?err=notfound")
	}
	ps, _ := h.repo.ResolveGroupProjects(g)
	ids := make([]string, 0, len(ps))
	for i := range ps {
		ids = append(ids, ps[i].SalesID)
	}
	quotes, _ := h.repo.QuotesBySalesIDs(ids)
	qm := model.GroupQuotesBySales(quotes)
	model.ApplySalesPipelineAmounts(ps, quotes)
	cnt, amt, w := model.SalesGroupTotalsAllowDup(ps, qm)
	members, _ := h.repo.ListGroupMemberRows(g.GroupID)
	ex, inc := 0, 0
	for _, m := range members {
		if m.MemberKind == model.SalesMemberExclude {
			ex++
		}
		if m.MemberKind == model.SalesMemberInclude {
			inc++
		}
	}
	filterN := 0
	if g.GroupKind == model.SalesGroupFilter {
		got, _ := h.repo.ListFilter(repository.FilterJSONToList(g.Filter()))
		filterN = len(got)
	}
	others := map[string]string{}
	for i := range ps {
		gs, _ := h.repo.GroupsForSales(ps[i].SalesID)
		var names []string
		for _, og := range gs {
			if og.GroupID != g.GroupID {
				names = append(names, og.GroupNo+" "+og.Name)
			}
		}
		if len(names) > 0 {
			others[ps[i].SalesID] = strings.Join(names, ", ")
		}
	}
	return c.Render(http.StatusOK, "sales/group_show.html", map[string]interface{}{
		"Title": g.GroupNo + " · " + g.Name, "Active": NavSalesGroups, "Group": g, "Projects": ps,
		"Count": cnt, "Amount": model.FormatSalesMoney(int64(amt)), "Weighted": model.FormatSalesMoney(int64(w)),
		"Avg": model.SalesAvgProbLabel(amt, w), "FilterN": filterN, "ExcludeN": ex, "IncludeN": inc,
		"Others": others, "CanWrite": canWriteSales(c),
	})
}

func (h *SalesHandler) GroupEdit(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	g, err := h.repo.GetGroup(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales/groups?err=notfound")
	}
	return h.renderGroupForm(c, g, false, "")
}

func (h *SalesHandler) GroupUpdate(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	g := h.parseGroupForm(c)
	g.GroupID = c.Param("id")
	if err := h.repo.UpdateGroup(g); err != nil {
		return h.renderGroupForm(c, g, false, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales/groups/"+g.GroupID)
}

func (h *SalesHandler) GroupAddMembers(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	kind := c.FormValue("member_kind")
	ids := c.Request().Form["sales_id"]
	if err := h.repo.AddGroupMembers(c.Param("id"), ids, kind, ctxString(c, "user_name")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales/groups/"+c.Param("id")+"?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales/groups/"+c.Param("id"))
}

func (h *SalesHandler) GroupRemoveMember(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	if err := h.repo.RemoveGroupMember(c.Param("id"), c.Param("sid")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales/groups/"+c.Param("id")+"?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales/groups/"+c.Param("id"))
}

func (h *SalesHandler) GroupClose(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	if err := h.repo.CloseGroup(c.Param("id")); err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales/groups/"+c.Param("id")+"?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales/groups")
}

func (h *SalesHandler) GroupSearchProjects(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	items, err := h.repo.ListFilter(repository.SalesListFilter{Search: q})
	if err != nil {
		return c.JSON(http.StatusOK, []interface{}{})
	}
	out := make([]map[string]string, 0, len(items))
	for i := range items {
		if i >= 30 {
			break
		}
		out = append(out, map[string]string{
			"sales_id": items[i].SalesID, "sales_no": items[i].DisplayNo(), "name": items[i].Name,
		})
	}
	return c.JSON(http.StatusOK, out)
}

func (h *SalesHandler) GroupPreview(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	f := parseSalesListFilterQ(c, repository.SalesListFilter{})
	items, err := h.repo.ListFilter(f)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]int{"count": 0})
	}
	return c.JSON(http.StatusOK, map[string]int{"count": len(items)})
}

func (h *SalesHandler) salesGroupSections(items []model.SalesProject, rows []map[string]interface{}, quotes []model.SalesQuote, groups []model.SalesGroup) []map[string]interface{} {
	byID := map[string]model.SalesProject{}
	rowBy := map[string]map[string]interface{}{}
	for i := range items {
		byID[items[i].SalesID] = items[i]
	}
	for _, r := range rows {
		if id, _ := r["SalesID"].(string); id != "" {
			rowBy[id] = r
		}
	}
	qm := model.GroupQuotesBySales(quotes)
	linked, _ := h.repo.MemberSalesIDSet()
	var sections []map[string]interface{}
	for i := range groups {
		g := groups[i]
		ps, _ := h.repo.ResolveGroupProjects(&g)
		var bucket []map[string]interface{}
		var bucketP []model.SalesProject
		for _, p := range ps {
			if _, ok := byID[p.SalesID]; !ok {
				continue
			}
			if r := rowBy[p.SalesID]; r != nil {
				bucket = append(bucket, r)
			}
			bucketP = append(bucketP, p)
		}
		cnt, amt, w := model.SalesGroupTotalsAllowDup(bucketP, qm)
		title := g.Name + " (" + g.GroupNo + ")"
		if g.Year > 0 {
			title = strconv.Itoa(g.Year) + "년 " + title
		}
		sections = append(sections, map[string]interface{}{
			"Title": title, "Count": cnt, "Amount": model.FormatSalesMoney(int64(amt)),
			"Weighted": model.FormatSalesMoney(int64(w)), "Avg": model.SalesAvgProbLabel(amt, w),
			"Rows": bucket, "Unclass": false,
		})
	}
	var unc []map[string]interface{}
	var uncP []model.SalesProject
	for i := range items {
		if linked[items[i].SalesID] {
			continue
		}
		unc = append(unc, rowBy[items[i].SalesID])
		uncP = append(uncP, items[i])
	}
	cnt, amt, w := model.SalesGroupTotalsAllowDup(uncP, qm)
	sections = append(sections, map[string]interface{}{
		"Title": "미분류", "Count": cnt, "Amount": model.FormatSalesMoney(int64(amt)),
		"Weighted": model.FormatSalesMoney(int64(w)), "Avg": model.SalesAvgProbLabel(amt, w),
		"Rows": unc, "Unclass": true,
	})
	gCnt, gAmt, gW := model.SalesGroupTotals(items, qm)
	sections = append(sections, map[string]interface{}{
		"Title": "전체 합계 (중복 제외)", "Count": gCnt, "Amount": model.FormatSalesMoney(int64(gAmt)),
		"Weighted": model.FormatSalesMoney(int64(gW)), "Avg": model.SalesAvgProbLabel(gAmt, gW),
		"Rows": nil, "Grand": true, "DupNote": "중복 제외",
	})
	return sections
}

func (h *SalesHandler) parseGroupForm(c echo.Context) *model.SalesGroup {
	y, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("year")))
	by, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("budget_year")))
	f := model.SalesListFilterJSON{
		BizType: c.FormValue("biz_type"), BudgetYear: by, BudgetStatus: c.FormValue("budget_status"),
		Owner: c.FormValue("owner"), Stage: c.FormValue("stage"),
		ContractTarget: c.FormValue("contract_target"), ProcurementRoute: c.FormValue("procurement_route"),
		ContractMethod: c.FormValue("contract_method"), BidEvalMethod: c.FormValue("bid_eval_method"),
		MallContractType: c.FormValue("mall_contract_type"),
		ExpectedFrom:     c.FormValue("expected_from"), ExpectedTo: c.FormValue("expected_to"),
	}
	return &model.SalesGroup{
		Name: strings.TrimSpace(c.FormValue("name")), Year: y,
		OwnerID: c.FormValue("owner_id"), OwnerName: c.FormValue("owner_name"),
		GroupKind: c.FormValue("group_kind"), FilterJSON: repository.EncodeGroupFilter(f),
		Pinned: c.FormValue("pinned") == "1" || c.FormValue("pinned") == "on",
		Notes:  c.FormValue("notes"),
	}
}

func (h *SalesHandler) renderGroupForm(c echo.Context, g *model.SalesGroup, isNew bool, formErr string) error {
	users, _ := h.userRepo.ListAssignable()
	preview := 0
	if g.GroupKind == model.SalesGroupFilter {
		ps, _ := h.repo.ListFilter(repository.FilterJSONToList(g.Filter()))
		preview = len(ps)
	}
	title := "대분류 등록"
	if !isNew {
		title = "대분류 수정"
	}
	return c.Render(http.StatusOK, "sales/group_form.html", map[string]interface{}{
		"Title": title, "Active": NavSalesGroups, "Group": g, "IsNew": isNew, "FormError": formErr,
		"Users": users, "PreviewN": preview, "Filter": g.Filter(),
	})
}

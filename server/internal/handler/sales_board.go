package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func salesListView(display, view string) string {
	d, v := strings.TrimSpace(display), strings.TrimSpace(view)
	if d == "kanban" || v == "kanban" {
		return "kanban"
	}
	if d == "timeline" || v == "timeline" {
		return "timeline"
	}
	return "list"
}

func salesActView(v string) string {
	if strings.TrimSpace(v) == "kanban" {
		return "kanban"
	}
	return "list"
}

func salesActGroup(v string) string {
	if strings.TrimSpace(v) == "stage" {
		return "stage"
	}
	return "type"
}

func salesListFilterValues(f repository.SalesListFilter, view, fromTask string) url.Values {
	s := salesFilterEncode(f, "")
	v, _ := url.ParseQuery(s)
	if view != "" && view != "list" {
		v.Set("display", view)
	}
	if strings.TrimSpace(fromTask) != "" {
		v.Set("from_task", fromTask)
	}
	return v
}

func parseSalesListFilter(c echo.Context) repository.SalesListFilter {
	deal := strings.TrimSpace(c.QueryParam("deal"))
	if deal != model.SalesDealSupply {
		deal = ""
	}
	return repository.SalesListFilter{
		Search:          strings.TrimSpace(c.QueryParam("search")),
		Status:          strings.TrimSpace(c.QueryParam("status")),
		Stage:           strings.TrimSpace(c.QueryParam("stage")),
		Owner:           strings.TrimSpace(c.QueryParam("owner")),
		Period:          strings.TrimSpace(c.QueryParam("period")),
		Customer:        strings.TrimSpace(c.QueryParam("customer")),
		AmountConfirmed: strings.TrimSpace(c.QueryParam("amount_confirmed")),
		DealType:        deal,
	}
}

func salesFilterEncode(f repository.SalesListFilter, extra string) string {
	q := url.Values{}
	if f.Search != "" {
		q.Set("search", f.Search)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.Stage != "" {
		q.Set("stage", f.Stage)
	}
	if f.Owner != "" {
		q.Set("owner", f.Owner)
	}
	if f.Period != "" {
		q.Set("period", f.Period)
	}
	if f.Customer != "" {
		q.Set("customer", f.Customer)
	}
	if f.AmountConfirmed != "" {
		q.Set("amount_confirmed", f.AmountConfirmed)
	}
	if strings.TrimSpace(f.DealType) == model.SalesDealSupply {
		q.Set("deal", model.SalesDealSupply)
	}
	s := q.Encode()
	if extra == "" {
		return s
	}
	if s == "" {
		return extra
	}
	return s + "&" + extra
}

func salesDealTypeOf(f repository.SalesListFilter) string {
	if strings.TrimSpace(f.DealType) == model.SalesDealSupply {
		return model.SalesDealSupply
	}
	return model.SalesDealBuild
}

func salesPath(action, encoded string) string {
	if strings.TrimSpace(encoded) == "" {
		return action
	}
	return action + "?" + encoded
}

func salesDealBoardLinks(action, view string, f repository.SalesListFilter, fromTask string) (dealType, buildHref, supplyHref, newHref, resetHref, listQ string) {
	dealType = salesDealTypeOf(f)
	extra := ""
	if action == "/sales" && view != "" && view != "list" {
		extra = "display=" + url.QueryEscape(view)
	}
	fBuild := f
	fBuild.DealType = ""
	fSupply := f
	fSupply.DealType = model.SalesDealSupply
	buildHref = salesPath(action, salesFilterEncode(fBuild, extra))
	supplyHref = salesPath(action, salesFilterEncode(fSupply, extra))
	resetExtra := extra
	resetF := repository.SalesListFilter{}
	if dealType == model.SalesDealSupply {
		resetF.DealType = model.SalesDealSupply
	}
	resetHref = salesPath(action, salesFilterEncode(resetF, resetExtra))
	nq := url.Values{}
	if dealType == model.SalesDealSupply {
		nq.Set("deal", model.SalesDealSupply)
	}
	if strings.TrimSpace(fromTask) != "" {
		nq.Set("from_task", fromTask)
	}
	newHref = salesPath("/sales/new", nq.Encode())
	listQ = salesFilterEncode(f, "")
	return
}

func salesPeriodOptions(now time.Time) []model.Code {
	if now.IsZero() {
		now = time.Now()
	}
	y := now.Year()
	var out []model.Code
	for _, yr := range []int{y - 1, y, y + 1} {
		ys := fmt.Sprintf("%d", yr)
		out = append(out, model.Code{CodeValue: ys, CodeName: ys + "년"})
		for q := 1; q <= 4; q++ {
			v := fmt.Sprintf("%s-Q%d", ys, q)
			out = append(out, model.Code{CodeValue: v, CodeName: fmt.Sprintf("%s %d분기", ys, q)})
		}
	}
	return out
}

func salesReturnPath(c echo.Context, fallback string) string {
	ret := strings.TrimSpace(c.FormValue("return"))
	if ret == "" {
		ret = strings.TrimSpace(c.QueryParam("return"))
	}
	if strings.HasPrefix(ret, "/sales") && !strings.Contains(ret, "://") && !strings.ContainsAny(ret, " \n\r") {
		return ret
	}
	return fallback
}

func wantsJSON(c echo.Context) bool {
	accept := c.Request().Header.Get("Accept")
	return strings.Contains(accept, "application/json") ||
		c.Request().Header.Get("X-Requested-With") == "XMLHttpRequest"
}

func (h *SalesHandler) replyStage(c echo.Context, err error, id, okPath string) error {
	if err != nil {
		code := salesErrCode(err)
		msg := querySalesErr(code)
		if msg == "" {
			msg = err.Error()
		}
		if wantsJSON(c) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"ok": false, "code": code, "error": msg,
			})
		}
		back := salesReturnPath(c, "/sales/"+id)
		sep := "?"
		if strings.Contains(back, "?") {
			sep = "&"
		}
		if strings.HasPrefix(back, "/sales/"+id) || back == "/sales/"+id {
			return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?err="+code)
		}
		return c.Redirect(http.StatusSeeOther, back+sep+"err="+code)
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true})
	}
	back := salesReturnPath(c, okPath)
	if back == okPath {
		return c.Redirect(http.StatusSeeOther, okPath)
	}
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	if !strings.Contains(back, "ok=") {
		back += sep + "ok=stage"
	}
	return c.Redirect(http.StatusSeeOther, back)
}

type salesKanbanCol = model.KanbanColumn

func salesProjectKanban(items []model.SalesProject, stages []model.SalesStageDef, fromTask string, lastAct map[string]string, showAmount bool) []model.KanbanColumn {
	proc, lost := model.SalesKanbanStages(stages)
	cols := make([]model.KanbanColumn, 0, len(proc)+1)
	index := map[string]int{}
	for _, st := range proc {
		index[st.Code] = len(cols)
		cols = append(cols, model.KanbanColumn{Key: st.Code, Title: st.Label, Sort: st.SortOrder, Border: "border-slate-200", CountUnit: "건", Items: []model.KanbanCard{}})
	}
	if lost != nil {
		index[lost.Code] = len(cols)
		cols = append(cols, model.KanbanColumn{Key: lost.Code, Title: lost.Label, Sort: lost.SortOrder, IsLost: true, Border: "border-slate-200", CountUnit: "건", Items: []model.KanbanCard{}})
	}
	if lastAct == nil {
		lastAct = map[string]string{}
	}
	seen := map[string]bool{}
	amounts := make([]int64, len(cols))
	for i := range items {
		p := items[i]
		if p.SalesID == "" || seen[p.SalesID] {
			continue
		}
		key := p.Stage
		idx, ok := index[key]
		if !ok {
			continue
		}
		seen[p.SalesID] = true
		title := p.Name
		if p.IsTentativeName {
			title = p.Name + " (가칭)"
		}
		href := "/sales/" + p.SalesID
		if fromTask != "" {
			href += "?tab=timeline&from_task=" + url.QueryEscape(fromTask)
		}
		amt := p.AmountLabel()
		if amt == "" {
			amt = "0원"
		}
		last := lastAct[p.SalesID]
		if last == "" {
			last = "—"
		}
		amounts[idx] += int64(p.ExpectedAmount)
		cols[idx].Items = append(cols[idx].Items, model.KanbanCard{
			ID: p.SalesID, RefID: p.SalesID, Title: title, Href: href,
			OrgName: p.CustomerValue(), Extra: amt,
			Assignee: p.SalesOwner, Bucket: key, Stage: key,
			LeftStyle: model.WorkCardColorStyle(p.SalesOwner, "", "sales"),
			SortDate:  model.NormalizeSalesYM(p.ExpectedYM), DueDate: p.PeriodLabel(),
			AmountUnconfirmed: !p.ExpectedAmountConfirmed, LastActivity: last,
			AmountDesc: p.ExpectedAmount,
		})
	}
	for i := range cols {
		model.SortKanbanCards(cols[i].Items)
		cols[i].Count = len(cols[i].Items)
		if showAmount {
			cols[i].Subtitle = model.FormatSalesMoneyShort(amounts[i])
		}
	}
	return cols
}

type salesActCol struct {
	Key   string
	Label string
	Sort  int
	Items []model.SalesActivity
}

func salesActivityKanban(acts []model.SalesActivity, group string, types []model.Code, stages []model.SalesStageDef) []salesActCol {
	group = salesActGroup(group)
	var cols []salesActCol
	index := map[string]int{}
	if group == "stage" {
		proc, lost := model.SalesKanbanStages(stages)
		for _, st := range proc {
			index[st.Code] = len(cols)
			cols = append(cols, salesActCol{Key: st.Code, Label: st.Label, Sort: st.SortOrder})
		}
		if lost != nil {
			index[lost.Code] = len(cols)
			cols = append(cols, salesActCol{Key: lost.Code, Label: lost.Label, Sort: lost.SortOrder})
		}
		index["_"] = len(cols)
		cols = append(cols, salesActCol{Key: "_", Label: "단계 없음", Sort: 99})
	} else {
		for i, t := range types {
			index[t.CodeValue] = len(cols)
			cols = append(cols, salesActCol{Key: t.CodeValue, Label: t.CodeName, Sort: t.SortOrder})
			if t.SortOrder == 0 {
				cols[len(cols)-1].Sort = i + 1
			}
		}
		if _, ok := index["other"]; !ok {
			index["other"] = len(cols)
			cols = append(cols, salesActCol{Key: "other", Label: "기타", Sort: 99})
		}
	}
	for _, a := range acts {
		key := model.SalesActivityColumnKey(a, group)
		i, ok := index[key]
		if !ok {
			i = len(cols) - 1
		}
		if i < 0 || i >= len(cols) {
			continue
		}
		cols[i].Items = append(cols[i].Items, a)
	}
	return cols
}

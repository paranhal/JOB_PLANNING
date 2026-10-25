package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type salesChildRow struct {
	CustomerID string `json:"customer_id"`
	Name       string `json:"name"`
	Amount     int    `json:"amount"`
	ExpectedYM string `json:"expected_ym"`
	Force      bool   `json:"force"`
	Skip       bool   `json:"skip"`
	Reason     string `json:"reason,omitempty"`
	Label      string `json:"label,omitempty"`
}

type salesChildBatchResult struct {
	Created []salesChildRow `json:"created"`
	Skipped []salesChildRow `json:"skipped"`
	Failed  []salesChildRow `json:"failed"`
	Message string          `json:"message"`
	Reopened bool           `json:"reopened,omitempty"`
	Closed   bool           `json:"closed,omitempty"`
}

func expandChildName(tmpl, customer, group string, year int) string {
	s := strings.TrimSpace(tmpl)
	s = strings.ReplaceAll(s, "{고객명}", customer)
	s = strings.ReplaceAll(s, "{대분류명}", group)
	s = strings.ReplaceAll(s, "{연도}", strconv.Itoa(year))
	return strings.TrimSpace(s)
}

func (h *SalesHandler) GroupChildrenPreview(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	g, err := h.repo.GetGroup(c.Param("id"))
	if err != nil || g == nil {
		return echo.ErrNotFound
	}
	rows, err := h.parseChildRows(c, g)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "rows": rows})
}

func (h *SalesHandler) GroupChildrenCreate(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	g, err := h.repo.GetGroup(c.Param("id"))
	if err != nil || g == nil {
		return echo.ErrNotFound
	}
	if g.GroupKind == model.SalesGroupFilter {
		return echo.NewHTTPError(http.StatusBadRequest, "조건형 대분류에는 하위 사업을 일괄 만들 수 없습니다")
	}
	tmpl := strings.TrimSpace(c.FormValue("name_template"))
	if tmpl == "" {
		tmpl = strings.TrimSpace(g.ChildNameTemplate)
	}
	if !strings.Contains(tmpl, "{고객명}") {
		return echo.NewHTTPError(http.StatusBadRequest, "이름 규칙에 {고객명} 이 필요합니다")
	}
	_ = h.repo.SaveGroupChildDefaults(g.GroupID, tmpl, g.ChildDefaultsJSON, g.AutoClose)
	g.ChildNameTemplate = tmpl
	rows, err := h.parseChildRows(c, g)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(rows) > 200 {
		return echo.NewHTTPError(http.StatusBadRequest, "한 번에 200곳까지입니다")
	}
	out := &salesChildBatchResult{}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		org = model.OrgIDLibrary
	}
	reopened := false
	for _, row := range rows {
		if row.Skip && !row.Force {
			out.Skipped = append(out.Skipped, row)
			continue
		}
		cust, err := h.customerRepo.GetByID(row.CustomerID)
		if err != nil || cust == nil {
			row.Reason = "고객을 찾을 수 없습니다"
			out.Failed = append(out.Failed, row)
			continue
		}
		p := &model.SalesProject{
			Name:           row.Name,
			CustomerID:     row.CustomerID,
			OrgID:          org,
			ExpectedAmount: row.Amount,
			ExpectedYM:     row.ExpectedYM,
			Stage:          model.SalesStage4Discover,
			SalesOwnerID: g.OwnerID,
			SalesOwner:   g.OwnerName,
		}
		if err := h.repo.Create(p); err != nil {
			row.Reason = err.Error()
			out.Failed = append(out.Failed, row)
			continue
		}
		_ = h.repo.AddGroupMembers(g.GroupID, []string{p.SalesID}, model.SalesMemberManual, currentUserID(c))
		row.Label = p.DisplayNo()
		out.Created = append(out.Created, row)
	}
	if len(out.Created) > 0 && g.Status == model.SalesGroupClosed {
		if err := h.repo.ReopenGroup(g.GroupID); err == nil {
			reopened = true
			out.Reopened = true
		}
	}
	out.Message = fmt.Sprintf("사업 %d건 등록", len(out.Created))
	if len(out.Skipped) > 0 {
		out.Message += fmt.Sprintf(" / %d건 건너뜀", len(out.Skipped))
	}
	if len(out.Failed) > 0 {
		out.Message += fmt.Sprintf(" / %d건 실패", len(out.Failed))
	}
	if reopened {
		out.Message += " · 대분류를 다시 열었습니다"
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, out)
	}
	return c.Redirect(http.StatusSeeOther, "/sales/groups/"+g.GroupID+"?ok=children")
}

func (h *SalesHandler) parseChildRows(c echo.Context, g *model.SalesGroup) ([]salesChildRow, error) {
	tmpl := strings.TrimSpace(c.FormValue("name_template"))
	if tmpl == "" {
		tmpl = strings.TrimSpace(g.ChildNameTemplate)
	}
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["customer_id"]
	names := c.Request().PostForm["child_name"]
	amts := c.Request().PostForm["child_amount"]
	yms := c.Request().PostForm["child_ym"]
	forces := c.Request().PostForm["child_force"]
	paste := strings.TrimSpace(c.FormValue("paste_names"))
	if paste != "" {
		for _, line := range strings.Split(paste, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			hit, err := h.customerRepo.FindByExactOrgName(line)
			if err != nil || hit == nil {
				ms, _ := h.customerRepo.ListActiveNameMatches(line, "")
				if len(ms) == 1 {
					hit = &ms[0]
				} else {
					ids = append(ids, "")
					names = append(names, line)
					continue
				}
			}
			ids = append(ids, hit.CustomerID)
		}
	}
	existing, _ := h.repo.ResolveGroupProjects(g)
	haveCust := map[string]bool{}
	for _, p := range existing {
		if cid := strings.TrimSpace(p.CustomerID); cid != "" {
			haveCust[cid] = true
		}
	}
	year := g.Year
	var rows []salesChildRow
	seen := map[string]bool{}
	for i, id := range ids {
		id = strings.TrimSpace(id)
		row := salesChildRow{CustomerID: id}
		if i < len(names) {
			row.Name = strings.TrimSpace(names[i])
		}
		if i < len(amts) {
			n, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(amts[i]), ",", ""))
			row.Amount = n
		}
		if i < len(yms) {
			row.ExpectedYM = strings.TrimSpace(yms[i])
		}
		if i < len(forces) && strings.TrimSpace(forces[i]) == "1" {
			row.Force = true
		}
		if id == "" {
			row.Skip = true
			row.Reason = "찾지 못함"
			row.Label = row.Name
			rows = append(rows, row)
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		cust, err := h.customerRepo.GetByID(id)
		if err != nil || cust == nil {
			row.Skip = true
			row.Reason = "고객을 찾을 수 없습니다"
			rows = append(rows, row)
			continue
		}
		row.Label = cust.OrgName
		if row.Name == "" {
			row.Name = expandChildName(tmpl, cust.OrgName, g.Name, year)
		}
		if haveCust[id] && !row.Force {
			row.Skip = true
			row.Reason = "이미 있음"
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (h *SalesHandler) afterSalesChildChanged(salesID string) (closedMsg string) {
	if h == nil || h.repo == nil || strings.TrimSpace(salesID) == "" {
		return ""
	}
	gs, err := h.repo.GroupsForSales(salesID)
	if err != nil {
		return ""
	}
	for _, g := range gs {
		if g.GroupKind == model.SalesGroupFilter || !g.AutoClose {
			continue
		}
		if g.Status == model.SalesGroupClosed {
			continue
		}
		ps, err := h.repo.ResolveGroupProjects(&g)
		if err != nil || len(ps) == 0 {
			continue
		}
		all := true
		sum := model.SalesChildSummary{}
		for _, p := range ps {
			sum = sum.Add(p)
			if !model.SalesChildDecided(p) {
				all = false
			}
		}
		if !all {
			continue
		}
		if err := h.repo.CloseGroup(g.GroupID); err != nil {
			continue
		}
		closedMsg = fmt.Sprintf("모든 하위 사업이 정리되어 대분류를 종료했습니다(수주 %d·실주 %d·포기 %d)",
			sum.WonN+sum.ContractN, sum.LostN, sum.DroppedN)
	}
	return closedMsg
}

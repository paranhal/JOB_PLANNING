package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *SalesHandler) renderSalesPipeline(c echo.Context, f repository.SalesListFilter, items []model.SalesProject) error {
	now := time.Now()
	opts := model.DefaultForecastOpts(now)
	if b := strings.TrimSpace(c.QueryParam("basis")); b != "" {
		opts.Basis = b
	}
	if from := strings.TrimSpace(c.QueryParam("from")); from != "" {
		opts.From = model.NormalizeSalesYM(from)
	}
	if to := strings.TrimSpace(c.QueryParam("to")); to != "" {
		opts.To = model.NormalizeSalesYM(to)
	}
	if g := strings.TrimSpace(c.QueryParam("group")); g != "" {
		opts.Group = g
	}
	if cell := strings.TrimSpace(c.QueryParam("cell")); cell != "" {
		opts.Cell = cell
	}
	opts.Spread = c.QueryParam("spread") == "1"
	showAll := c.QueryParam("all_ym") == "1"
	h.attachForecastGroups(items)
	fc := model.BuildSalesForecast(items, opts)
	stages, _ := h.repo.StagesFor(f.DealType)
	rows := make([]map[string]interface{}, 0, len(fc.Rows))
	for i := range fc.Rows {
		r := fc.Rows[i]
		def := model.FindSalesStage(stages, r.Project.Stage)
		vp := h.viewProject(&r.Project, def)
		vp["BasisYM"] = r.BasisYM
		vp["Unsched"] = r.Unsched
		vp["Weighted"] = model.FormatSalesMoney(int64(r.Weighted))
		vp["VATLabel"] = r.VATLabel
		vp["BidYM"] = r.Project.BidYM
		vp["RevenueYM"] = r.Project.RevenueYM
		vp["WonAt"] = r.Project.WonAt
		vp["ContractedAt"] = r.Project.ContractedAt
		vp["RevenueFrom"] = r.Project.RevenueFrom
		vp["RevenueTo"] = r.Project.RevenueTo
		vp["BillingCycle"] = model.BillingCycleLabel(r.Project.BillingCycle)
		vp["GroupNames"] = strings.Join(r.Project.ForecastGroups, ", ")
		rows = append(rows, vp)
	}
	if strings.HasSuffix(c.Request().URL.Path, ".xlsx") || c.QueryParam("format") == "xlsx" {
		return writePipelineXLSX(c, f, opts, fc, rows)
	}
	fromTask := strings.TrimSpace(c.QueryParam("from_task"))
	_, buildHref, supplyHref, newHref, resetHref, filterQ := salesDealBoardLinks("/sales", "pipeline", f, fromTask)
	users, _ := h.userRepo.ListAssignable()
	targets, _ := h.codeRepo.ActiveByGroup("sales_contract_target")
	bizTypes, _ := h.codeRepo.ActiveByGroup(model.SalesCodeGroupBizType)
	budgetSt, _ := h.codeRepo.ActiveByGroup(model.SalesCodeGroupBudgetStatus)
	groups, _ := h.repo.ListGroups(false)
	hidden, _ := h.repo.CountHiddenClosed(f)
	hiddenDormant, _ := h.repo.CountHiddenDormant(f)
	return c.Render(http.StatusOK, "sales/pipeline.html", map[string]interface{}{
		"Title": "영업 파이프라인", "Active": NavSales,
		"View": "pipeline", "Filter": f, "FilterQ": filterQ,
		"DealType": f.DealType, "DealBuildHref": buildHref, "DealSupplyHref": supplyHref,
		"NewHref": newHref, "FilterReset": resetHref, "FilterAction": "/sales",
		"Search": f.Search, "Status": f.Status, "Stage": f.Stage,
		"Owner": f.Owner, "Period": f.Period, "Customer": f.Customer, "AmountConfirmed": f.AmountConfirmed,
		"IncludeClosed": f.IncludeClosed, "IncludeDormant": f.IncludeDormant, "CloseReason": f.CloseReason,
		"HiddenClosed": hidden, "HiddenDormant": hiddenDormant,
		"ContractTarget": f.ContractTarget, "ContractTargets": targets,
		"BizType": f.BizType, "BizTypes": bizTypes, "BudgetYear": f.BudgetYear, "BudgetStatus": f.BudgetStatus, "BudgetStatuses": budgetSt,
		"GroupID": f.GroupID, "Groups": groups, "Users": users, "PeriodOptions": salesPeriodOptions(now),
		"CanWrite": canWriteSales(c), "Forecast": fc, "Rows": rows,
		"Basis": opts.Basis, "From": opts.From, "To": opts.To, "GroupBy": opts.Group, "Cell": opts.Cell,
		"Spread": opts.Spread, "ShowAllYM": showAll, "BasisLabel": model.ForecastBasisLabel(opts.Basis),
		"Summary": fc.Summary, "Matrix": fc.Matrix,
	})
}

func (h *SalesHandler) PipelineXLSX(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	f := salesListFilterFromRequest(c)
	items, err := h.repo.ListFilter(f)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(items))
	for i := range items {
		ids = append(ids, items[i].SalesID)
	}
	quotes, _ := h.repo.QuotesBySalesIDs(ids)
	model.ApplySalesPipelineAmounts(items, quotes)
	return h.renderSalesPipeline(c, f, items)
}

func (h *SalesHandler) attachForecastGroups(items []model.SalesProject) {
	groups, err := h.repo.ListGroups(false)
	if err != nil {
		return
	}
	by := map[string][]string{}
	for i := range groups {
		g := groups[i]
		ps, err := h.repo.ResolveGroupProjects(&g)
		if err != nil {
			continue
		}
		lab := strings.TrimSpace(g.GroupNo + " " + g.Name)
		for _, p := range ps {
			by[p.SalesID] = append(by[p.SalesID], lab)
		}
	}
	for i := range items {
		items[i].ForecastGroups = by[items[i].SalesID]
	}
}

func writePipelineXLSX(c echo.Context, f repository.SalesListFilter, opts model.SalesForecastOpts, fc model.SalesForecast, _ []map[string]interface{}) error {
	xf := excelize.NewFile()
	list := "사업 목록"
	xf.SetSheetName("Sheet1", list)
	cond := fmt.Sprintf("기준: %s · 기간 %s~%s · %s", model.ForecastBasisLabel(opts.Basis), opts.From, opts.To, time.Now().Format("2006-01-02 15:04"))
	if !f.IncludeClosed {
		cond += " · 실주·포기 제외"
	}
	_ = xf.SetCellValue(list, "A1", cond)
	heads := []string{"사업번호", "사업명", "고객", "담당", "단계", "입찰 상태", "확도", "사업 시기", "입찰 시기", "수주일", "계약일", "매출 시기", "대금 시작", "대금 종료", "주기", "금액", "VAT 포함", "금액 출처", "가중금액", "결과", "대분류"}
	for i, h := range heads {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = xf.SetCellValue(list, cell, h)
	}
	for i, r := range fc.Rows {
		row := i + 3
		p := r.Project
		vals := []interface{}{p.DisplayNo(), p.Name, p.CustomerValue(), p.SalesOwner, p.Stage, p.BidStatus, p.EffectiveProbability(),
			p.ExpectedYM, p.BidYM, p.WonAt, p.ContractedAt, p.RevenueYM, p.RevenueFrom, p.RevenueTo, model.BillingCycleLabel(p.BillingCycle),
			r.Amount, r.VATLabel, r.Source, r.Weighted, p.CloseReason, strings.Join(p.ForecastGroups, ", ")}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			_ = xf.SetCellValue(list, cell, v)
		}
		amtCell, _ := excelize.CoordinatesToCellName(16, row)
		_ = xf.SetCellStyle(list, amtCell, amtCell, pipelineNumStyle(xf))
		wCell, _ := excelize.CoordinatesToCellName(19, row)
		_ = xf.SetCellStyle(list, wCell, wCell, pipelineNumStyle(xf))
	}
	_ = xf.AutoFilter(list, "A2:U2", nil)
	_ = xf.SetPanes(list, &excelize.Panes{Freeze: true, Split: true, YSplit: 2, TopLeftCell: "A3", ActivePane: "bottomLeft"})

	ms := "월별 합계"
	_, _ = xf.NewSheet(ms)
	_ = xf.SetCellValue(ms, "A1", cond)
	_ = xf.SetCellValue(ms, "A2", "구분")
	for i, m := range fc.Matrix.Months {
		cell, _ := excelize.CoordinatesToCellName(i+2, 2)
		_ = xf.SetCellValue(ms, cell, m)
	}
	uCell, _ := excelize.CoordinatesToCellName(len(fc.Matrix.Months)+2, 2)
	_ = xf.SetCellValue(ms, uCell, "미정")
	for i, row := range fc.Matrix.Rows {
		r := i + 3
		_ = xf.SetCellValue(ms, fmt.Sprintf("A%d", r), row.Label)
		src := row.Cells
		if opts.Cell == "weighted" {
			src = row.WCells
		}
		for j, n := range src {
			cell, _ := excelize.CoordinatesToCellName(j+2, r)
			_ = xf.SetCellValue(ms, cell, n)
			_ = xf.SetCellStyle(ms, cell, cell, pipelineNumStyle(xf))
		}
		uc, _ := excelize.CoordinatesToCellName(len(fc.Matrix.Months)+2, r)
		uv := row.Unsched
		if opts.Cell == "weighted" {
			uv = row.WUnsched
		}
		_ = xf.SetCellValue(ms, uc, uv)
		_ = xf.SetCellStyle(ms, uc, uc, pipelineNumStyle(xf))
	}
	tr := len(fc.Matrix.Rows) + 3
	_ = xf.SetCellValue(ms, fmt.Sprintf("A%d", tr), "합계")
	for j, n := range fc.Matrix.Totals {
		cell, _ := excelize.CoordinatesToCellName(j+2, tr)
		_ = xf.SetCellValue(ms, cell, n)
		_ = xf.SetCellStyle(ms, cell, cell, pipelineNumStyle(xf))
	}
	gs := "대분류 합계"
	_, _ = xf.NewSheet(gs)
	_ = xf.SetCellValue(gs, "A1", "대분류 줄은 중복 포함, 합계는 중복 제외")
	gopts := opts
	gopts.Group = "group"
	gfc := model.BuildSalesForecast(projectsFromForecast(fc), gopts)
	_ = xf.SetCellValue(gs, "A2", "대분류")
	for i, m := range gfc.Matrix.Months {
		cell, _ := excelize.CoordinatesToCellName(i+2, 2)
		_ = xf.SetCellValue(gs, cell, m)
	}
	for i, row := range gfc.Matrix.Rows {
		r := i + 3
		_ = xf.SetCellValue(gs, fmt.Sprintf("A%d", r), row.Label)
		for j, n := range row.Cells {
			cell, _ := excelize.CoordinatesToCellName(j+2, r)
			_ = xf.SetCellValue(gs, cell, n)
			_ = xf.SetCellStyle(gs, cell, cell, pipelineNumStyle(xf))
		}
	}

	c.Response().Header().Set(echo.HeaderContentType, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="영업파이프라인_%s.xlsx"`, time.Now().Format("20060102")))
	_ = xf.Write(c.Response())
	return nil
}

func projectsFromForecast(fc model.SalesForecast) []model.SalesProject {
	out := make([]model.SalesProject, 0, len(fc.Rows))
	for _, r := range fc.Rows {
		out = append(out, r.Project)
	}
	return out
}

func pipelineNumStyle(f *excelize.File) int {
	s, err := f.NewStyle(&excelize.Style{CustomNumFmt: strPtr("#,##0")})
	if err != nil {
		return 0
	}
	return s
}

func strPtr(s string) *string { return &s }

package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type SalesHandler struct {
	repo         *repository.SalesRepo
	customerRepo *repository.CustomerRepo
	userRepo     *repository.UserRepo
	codeRepo     *repository.CodeRepo
}

func NewSalesHandler(
	repo *repository.SalesRepo,
	customerRepo *repository.CustomerRepo,
	userRepo *repository.UserRepo,
	codeRepo *repository.CodeRepo,
) *SalesHandler {
	return &SalesHandler{
		repo: repo, customerRepo: customerRepo, userRepo: userRepo, codeRepo: codeRepo,
	}
}

func (h *SalesHandler) List(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	search := strings.TrimSpace(c.QueryParam("search"))
	status := strings.TrimSpace(c.QueryParam("status"))
	stage := strings.TrimSpace(c.QueryParam("stage"))
	items, err := h.repo.List(search, status, stage)
	if err != nil {
		return err
	}
	stages, _ := h.repo.Stages()
	rows := make([]map[string]interface{}, 0, len(items))
	for i := range items {
		def := model.FindSalesStage(stages, items[i].Stage)
		rows = append(rows, h.viewProject(&items[i], def))
	}
	return c.Render(http.StatusOK, "sales/list.html", map[string]interface{}{
		"Title": "영업 사업", "Active": "sales",
		"Projects": rows, "Stages": stages,
		"Search": search, "Status": status, "Stage": stage,
		"CanWrite": canWriteSales(c),
		"FlashOK":  c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
	})
}

func (h *SalesHandler) New(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p := &model.SalesProject{
		IsTentativeName: true,
		Stage:           model.DefaultSalesStageCode(),
		Status:          model.SalesStatusActive,
	}
	return h.renderForm(c, p, false, "")
}

func (h *SalesHandler) Create(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p := h.parseForm(c)
	if err := h.repo.Create(p); err != nil {
		return h.renderForm(c, p, false, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+p.SalesID+"?ok=created")
}

func (h *SalesHandler) Show(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	p, err := h.repo.Get(c.Param("id"))
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
		}
		return err
	}
	stages, _ := h.repo.Stages()
	def := model.FindSalesStage(stages, p.Stage)
	hist, _ := h.repo.ListHistory(p.SalesID)
	return c.Render(http.StatusOK, "sales/show.html", map[string]interface{}{
		"Title": p.Name, "Active": "sales",
		"Project":  h.viewProject(p, def),
		"Raw":      p,
		"Stages":   stages,
		"History":  hist,
		"CanWrite": canWriteSales(c), "CanDelete": canDeleteSales(c),
		"FlashOK": c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"FormError": querySalesErr(c.QueryParam("err")),
	})
}

func (h *SalesHandler) Edit(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
	}
	return h.renderForm(c, p, true, "")
}

func (h *SalesHandler) Update(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	cur, err := h.repo.Get(id)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
	}
	p := h.parseForm(c)
	p.SalesID = cur.SalesID
	p.Stage = cur.Stage
	p.Status = cur.Status
	if p.LostReason == "" {
		p.LostReason = cur.LostReason
	}
	if err := h.repo.Update(p); err != nil {
		p.SalesID = cur.SalesID
		return h.renderForm(c, p, true, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+p.SalesID+"?ok=updated")
}

func (h *SalesHandler) ChangeStage(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	to := strings.TrimSpace(c.FormValue("stage"))
	reason := strings.TrimSpace(c.FormValue("reason"))
	keep := c.FormValue("keep_override") == "1"
	err := h.repo.ChangeStage(id, to, reason, currentUserID(c), ctxString(c, "user_name"), keep)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?err="+salesErrCode(err))
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+id+"?ok=stage")
}

func (h *SalesHandler) Delete(c echo.Context) error {
	if !canDeleteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		if err == sql.ErrNoRows {
			return c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
		}
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/sales?ok=deleted")
}

func (h *SalesHandler) Activities(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	return c.Render(http.StatusOK, "sales/placeholder.html", map[string]interface{}{
		"Title": "영업 활동", "Active": "sales_activities",
		"Heading": "영업 활동",
		"Message": "2단계에서 일일 단위 활동 로그를 채웁니다.",
	})
}

func (h *SalesHandler) Pipeline(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	return c.Render(http.StatusOK, "sales/placeholder.html", map[string]interface{}{
		"Title": "영업 파이프라인", "Active": "sales_pipeline",
		"Heading": "영업 파이프라인",
		"Message": "4단계에서 단계별 건수·금액 지표를 채웁니다.",
	})
}

func (h *SalesHandler) renderForm(c echo.Context, p *model.SalesProject, isEdit bool, formErr string) error {
	stages, _ := h.repo.Stages()
	customers, _ := h.customerRepo.ListAll()
	users, _ := h.userRepo.ListAssignable()
	sources, _ := h.codeRepo.ActiveByGroup(model.SalesCodeGroupLeadSource)
	title := "영업 사업 등록"
	if isEdit {
		title = "영업 사업 수정"
	}
	def := model.FindSalesStage(stages, p.Stage)
	if def != nil && !p.HasOverride {
		p.Probability = def.Probability
	}
	return c.Render(http.StatusOK, "sales/form.html", map[string]interface{}{
		"Title": title, "Active": "sales",
		"Project": p, "IsEdit": isEdit, "FormError": formErr,
		"Stages": stages, "Customers": customers, "Users": users, "LeadSources": sources,
		"DisplayStage": p.DisplayStage(def),
	})
}

func (h *SalesHandler) parseForm(c echo.Context) *model.SalesProject {
	p := &model.SalesProject{
		Name:                    strings.TrimSpace(c.FormValue("name")),
		IsTentativeName:         c.FormValue("is_tentative_name") == "1",
		Stage:                   strings.TrimSpace(c.FormValue("stage")),
		CustomerID:              strings.TrimSpace(c.FormValue("customer_id")),
		ProspectName:            strings.TrimSpace(c.FormValue("prospect_name")),
		ProspectRegion:          strings.TrimSpace(c.FormValue("prospect_region")),
		CustomerConfirmed:       c.FormValue("customer_confirmed") == "1",
		ExpectedYM:              strings.TrimSpace(c.FormValue("expected_ym")),
		ExpectedPrecision:       strings.TrimSpace(c.FormValue("expected_precision")),
		ExpectedYMConfirmed:     c.FormValue("expected_ym_confirmed") == "1",
		ExpectedAmount:          parseSalesAmount(c.FormValue("expected_amount")),
		ExpectedAmountConfirmed: c.FormValue("expected_amount_confirmed") == "1",
		SalesOwnerID:            strings.TrimSpace(c.FormValue("sales_owner_id")),
		SalesOwner:              strings.TrimSpace(c.FormValue("sales_owner")),
		Competitor:              strings.TrimSpace(c.FormValue("competitor")),
		LeadSource:              strings.TrimSpace(c.FormValue("lead_source")),
		LostReason:              strings.TrimSpace(c.FormValue("lost_reason")),
		Notes:                   strings.TrimSpace(c.FormValue("notes")),
		Status:                  model.SalesStatusActive,
	}
	if p.SalesOwnerID != "" && p.SalesOwner == "" && h.userRepo != nil {
		if u, err := h.userRepo.GetByID(p.SalesOwnerID); err == nil && u != nil {
			p.SalesOwner = u.FullName
		}
	}
	stages, _ := h.repo.Stages()
	if p.Stage == "" {
		p.Stage = model.DefaultSalesStageCode()
	}
	def := model.FindSalesStage(stages, p.Stage)
	stageProb := 0
	if def != nil {
		stageProb = def.Probability
	}
	rawProb := strings.TrimSpace(c.FormValue("probability"))
	if rawProb != "" {
		if n, err := strconv.Atoi(rawProb); err == nil {
			if n != stageProb {
				p.HasOverride = true
				p.OverrideValue = n
			}
		}
	}
	p.Probability = stageProb
	return p
}

func (h *SalesHandler) viewProject(p *model.SalesProject, def *model.SalesStageDef) map[string]interface{} {
	n, total := p.ConfirmedCount()
	customer := p.CustomerValue()
	period := p.PeriodLabel()
	amount := p.AmountLabel()
	return map[string]interface{}{
		"SalesID":            p.SalesID,
		"Name":               p.Name,
		"IsTentativeName":    p.IsTentativeName,
		"NameConfirmed":      p.NameConfirmed(),
		"Stage":              p.Stage,
		"StageLabel":         stageLabel(def, p.Stage),
		"DisplayStage":       p.DisplayStage(def),
		"HasOverride":        p.HasOverride,
		"EffectiveProb":      p.EffectiveProbability(),
		"Customer":           customer,
		"CustomerConfirmed":  p.CustomerConfirmed,
		"Period":             period,
		"ExpectedYMConfirmed": p.ExpectedYMConfirmed,
		"Amount":             amount,
		"ExpectedAmountConfirmed": p.ExpectedAmountConfirmed,
		"Owner":              p.SalesOwner,
		"Status":             p.Status,
		"LostReason":         p.LostReason,
		"Notes":              p.Notes,
		"Competitor":         p.Competitor,
		"LeadSource":         p.LeadSource,
		"ConfirmedCount":     n,
		"ConfirmedTotal":     total,
		"ConfirmationLabel":  p.ConfirmationLabel(),
		"Unconfirmed":        p.UnconfirmedItems(),
	}
}

func stageLabel(def *model.SalesStageDef, code string) string {
	if def != nil && def.Label != "" {
		return def.Label
	}
	return code
}

func parseSalesAmount(s string) int {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	s = strings.TrimSuffix(s, "원")
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	if n < 0 {
		return 0
	}
	return n
}

func salesErrCode(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "되돌릴") {
		return "reason"
	}
	if strings.Contains(msg, "수주확정") {
		return "won"
	}
	return "stage"
}

func querySalesErr(code string) string {
	switch code {
	case "reason":
		return "단계를 되돌릴 때는 사유가 필요합니다."
	case "won":
		return "수주확정으로 넘기려면 사업명·고객·시기·금액을 모두 확정해야 합니다."
	case "stage":
		return "단계를 바꿀 수 없습니다."
	case "notfound":
		return "영업 사업을 찾을 수 없습니다."
	default:
		return ""
	}
}

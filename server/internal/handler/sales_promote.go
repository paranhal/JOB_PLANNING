package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func (h *SalesHandler) PromoteForm(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p, wp, err := h.promoteState(c)
	if err != nil {
		return err
	}
	if wp != nil {
		if p.Status != model.SalesStatusPromoted {
			_ = h.repo.MarkPromoted(p.SalesID)
		}
		return c.Redirect(http.StatusSeeOther, "/projects/"+wp.ProjectID)
	}
	if !model.CanPromoteSales(p) {
		return c.Redirect(http.StatusSeeOther, "/sales/"+p.SalesID+"?err=not_contracted")
	}
	if strings.TrimSpace(p.CustomerID) == "" {
		return h.renderPromoteCustomer(c, p, "")
	}
	return h.renderPromoteProjectForm(c, p, model.WorkProjectFromSales(p), nil, "")
}

func (h *SalesHandler) PromoteCustomer(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p, wp, err := h.promoteState(c)
	if err != nil {
		return err
	}
	if wp != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/"+wp.ProjectID)
	}
	if !model.CanPromoteSales(p) {
		return c.Redirect(http.StatusSeeOther, "/sales/"+p.SalesID+"?err=not_contracted")
	}
	if strings.TrimSpace(p.CustomerID) != "" {
		return c.Redirect(http.StatusSeeOther, "/sales/"+p.SalesID+"/promote")
	}
	org := strings.TrimSpace(c.FormValue("org_name"))
	if org == "" {
		org = strings.TrimSpace(p.ProspectName)
	}
	if org == "" {
		return h.renderPromoteCustomer(c, p, "기관명을 입력하세요. 자산·AS가 붙으려면 고객 마스터가 필요합니다.")
	}
	official := strings.TrimSpace(c.FormValue("official_name"))
	if official == "" {
		official = org
	}
	industry := strings.TrimSpace(c.FormValue("industry"))
	if industry == "" {
		industry = "공공기관"
	}
	cust := &model.Customer{
		OrgName:      org,
		OfficialName: official,
		MainPhone:    strings.TrimSpace(c.FormValue("main_phone")),
		Industry:     industry,
		IsActive:     true,
		Notes:        "영업 승격 시 등록",
	}
	if err := h.customerRepo.Create(cust); err != nil {
		return h.renderPromoteCustomer(c, p, err.Error())
	}
	p.CustomerID = cust.CustomerID
	p.CustomerConfirmed = true
	if strings.TrimSpace(p.ProspectName) == "" {
		p.ProspectName = org
	}
	if err := h.repo.Update(p, ctxString(c, "user_name")); err != nil {
		return h.renderPromoteCustomer(c, p, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales/"+p.SalesID+"/promote")
}

func (h *SalesHandler) PromoteSave(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p, wp, err := h.promoteState(c)
	if err != nil {
		return err
	}
	if wp != nil {
		if p.Status != model.SalesStatusPromoted {
			_ = h.repo.MarkPromoted(p.SalesID)
		}
		return c.Redirect(http.StatusSeeOther, "/projects/"+wp.ProjectID)
	}
	if !model.CanPromoteSales(p) {
		return c.Redirect(http.StatusSeeOther, "/sales/"+p.SalesID+"?err=not_contracted")
	}
	if strings.TrimSpace(p.CustomerID) == "" {
		return c.Redirect(http.StatusSeeOther, "/sales/"+p.SalesID+"/promote")
	}
	proj := ParseWorkProjectForm(c)
	rules := parseScopeRules(c)
	if strings.TrimSpace(proj.Name) == "" {
		draft := model.WorkProjectFromSales(p)
		draft.ContractType, draft.BillingType = proj.ContractType, proj.BillingType
		draft.StartDate, draft.EndDate = proj.StartDate, proj.EndDate
		draft.IsPaid, draft.Notes = proj.IsPaid, proj.Notes
		return h.renderPromoteProjectForm(c, p, draft, rules, "사업명을 입력하세요.")
	}
	if strings.TrimSpace(proj.CustomerID) == "" {
		proj.CustomerID = p.CustomerID
	}
	if strings.TrimSpace(proj.OrderingPartyID) == "" && strings.TrimSpace(proj.OrderingParty) == "" {
		proj.OrderingPartyID = p.CustomerID
	}
	proj.SalesProjectID = p.SalesID
	if err := h.projectRepo.Create(proj); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			if existing, e2 := h.projectRepo.GetBySalesID(p.SalesID); e2 == nil && existing != nil {
				_ = h.repo.MarkPromoted(p.SalesID)
				return c.Redirect(http.StatusSeeOther, "/projects/"+existing.ProjectID)
			}
		}
		return h.renderPromoteProjectForm(c, p, proj, rules, err.Error())
	}
	if err := h.projectRepo.ReplaceRules(proj.ProjectID, rules); err != nil {
		return err
	}
	if err := h.repo.MarkPromoted(p.SalesID); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+proj.ProjectID+"?ok=promoted")
}

func (h *SalesHandler) promoteState(c echo.Context) (*model.SalesProject, *model.WorkProject, error) {
	p, err := h.repo.Get(c.Param("id"))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
		}
		return nil, nil, err
	}
	if h.projectRepo == nil {
		return p, nil, nil
	}
	wp, err := h.projectRepo.GetBySalesID(p.SalesID)
	if err == sql.ErrNoRows {
		return p, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return p, wp, nil
}

func (h *SalesHandler) renderPromoteCustomer(c echo.Context, p *model.SalesProject, errMsg string) error {
	org := strings.TrimSpace(p.ProspectName)
	if org == "" {
		org = p.CustomerValue()
	}
	return c.Render(http.StatusOK, "sales/promote_customer.html", map[string]interface{}{
		"Title": "고객 마스터에 등록", "Active": NavSales,
		"SalesID": p.SalesID, "SalesName": p.Name,
		"Customer": &model.Customer{
			OrgName:      org,
			OfficialName: org,
			Industry:     "공공기관",
		},
		"FormError": errMsg,
	})
}

func (h *SalesHandler) renderPromoteProjectForm(c echo.Context, s *model.SalesProject, p *model.WorkProject, rules []model.ProjectScopeRule, errMsg string) error {
	customers, _ := h.customerRepo.ListAll()
	parents, _, _ := h.customerRepo.ListCategories()
	contractTypes, _ := h.codeRepo.ActiveByGroup("project_contract_type")
	billingTypes, _ := h.codeRepo.ActiveByGroup("project_billing_type")
	type ruleJS struct {
		ID       string   `json:"id"`
		Parent   string   `json:"parent"`
		Products []string `json:"products"`
		Kinds    []string `json:"kinds"`
		Notes    string   `json:"notes"`
	}
	jsRules := make([]ruleJS, 0, len(rules))
	for _, rule := range rules {
		jsRules = append(jsRules, ruleJS{
			ID: rule.RuleID, Parent: rule.ParentCustomerID,
			Products: rule.ProductKeySlice(),
			Kinds:    splitCSVLocal(rule.WorkKinds),
			Notes:    rule.Notes,
		})
	}
	rulesJSON, _ := json.Marshal(jsRules)
	return c.Render(http.StatusOK, "project/form.html", map[string]interface{}{
		"Title": "사업관리로 등록", "Active": NavSales,
		"Project": p, "Rules": rules, "RulesJSON": string(rulesJSON), "IsEdit": false,
		"FromSales": true, "SalesID": s.SalesID, "SalesAmount": s.AmountLabel(),
		"Customers": customers, "Parents": parents,
		"ContractTypes": contractTypes, "BillingTypes": billingTypes,
		"ProductKeys": []map[string]string{
			{"Key": model.ProductKeyKLAS, "Label": model.ProductKeyLabel(model.ProductKeyKLAS)},
			{"Key": model.ProductKeySejongKLAS, "Label": model.ProductKeyLabel(model.ProductKeySejongKLAS)},
			{"Key": model.ProductKeyAnrobotics, "Label": model.ProductKeyLabel(model.ProductKeyAnrobotics)},
		},
		"WorkKinds": []map[string]string{
			{"Key": model.ScopeWorkAS, "Label": model.ScopeWorkKindLabel(model.ScopeWorkAS)},
			{"Key": model.ScopeWorkMaintenance, "Label": model.ScopeWorkKindLabel(model.ScopeWorkMaintenance)},
			{"Key": model.ScopeWorkAdmin, "Label": model.ScopeWorkKindLabel(model.ScopeWorkAdmin)},
		},
		"FormError": errMsg,
		"CanWrite":  true,
	})
}

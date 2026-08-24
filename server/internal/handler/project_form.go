package handler

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// ParseWorkProjectForm 일일업무 모달·사업관리 화면 공통 폼 파싱.
// 확장 필드(연도·유상·정렬·상태 등)는 값이 있을 때만 채운다.
func ParseWorkProjectForm(c echo.Context) *model.WorkProject {
	contactID := strings.TrimSpace(c.FormValue("contact_id"))
	if contactID == "__new__" {
		contactID = ""
	}
	orderingPartyID := strings.TrimSpace(c.FormValue("ordering_party_id"))
	orderingParty := ""
	if orderingPartyID == "custom" {
		orderingPartyID = ""
		orderingParty = strings.TrimSpace(c.FormValue("ordering_party_custom"))
	}
	contractType := strings.TrimSpace(c.FormValue("contract_type"))
	if contractType == "custom" {
		contractType = strings.TrimSpace(c.FormValue("contract_type_custom"))
	}
	billingType := strings.TrimSpace(c.FormValue("billing_type"))
	if billingType == "custom" {
		billingType = strings.TrimSpace(c.FormValue("billing_type_custom"))
	}
	year, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("plan_year")))
	sortOrder, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("sort_order")))
	isPaid := true
	if v := c.FormValue("is_paid"); v == "0" {
		isPaid = false
	}
	p := &model.WorkProject{
		Name:            strings.TrimSpace(c.FormValue("name")),
		ShortName:       strings.TrimSpace(c.FormValue("short_name")),
		PlanYear:        year,
		IsPaid:          isPaid,
		SortOrder:       sortOrder,
		OrderingPartyID: orderingPartyID,
		OrderingParty:   orderingParty,
		CustomerID:      strings.TrimSpace(c.FormValue("customer_id")),
		ContractType:    contractType,
		BillingType:     billingType,
		StartDate:       strings.TrimSpace(c.FormValue("start_date")),
		EndDate:         strings.TrimSpace(c.FormValue("end_date")),
		Notes:           strings.TrimSpace(c.FormValue("notes")),
		ContactID:       contactID,
		Color:           strings.TrimSpace(c.FormValue("color")),
		Status:          strings.TrimSpace(c.FormValue("status")),
		ProjectKind:     model.NormalizeProjectKind(c.FormValue("project_kind")),
	}
	applySalesForm(c, p)
	if p.Color == "" {
		p.Color = "#3B82F6"
	}
	if p.Status == "" {
		p.Status = model.WBProjectActive
	}
	return p
}

func applySalesForm(c echo.Context, p *model.WorkProject) {
	if p == nil || !p.IsSales() {
		return
	}
	p.SalesStage = model.NormalizeSalesStage(c.FormValue("sales_stage"))
	if p.SalesStage == "" {
		p.SalesStage = model.SalesStageLead
	}
	p.ExpectedPrecision = model.NormalizeExpectedPrecision(c.FormValue("expected_precision"))
	undated := c.FormValue("expected_undated") == "1"
	if undated {
		p.ExpectedYM = ""
		reason := strings.TrimSpace(c.FormValue("expected_undated_reason"))
		detail := strings.TrimSpace(c.FormValue("expected_undated_detail"))
		if reason == model.NoDateReasonOther || reason == "기타" {
			p.ExpectedUndatedReason = model.FormatNoDateReason(reason, detail)
		} else {
			p.ExpectedUndatedReason = reason
		}
	} else {
		p.ExpectedYM = model.NormalizeExpectedYM(c.FormValue("expected_ym"))
		p.ExpectedUndatedReason = ""
	}
	p.ExpectedNote = strings.TrimSpace(c.FormValue("expected_note"))
	p.ProspectName = strings.TrimSpace(c.FormValue("prospect_name"))
	p.ProspectRegion = strings.TrimSpace(c.FormValue("prospect_region"))
	p.ProspectContactName = strings.TrimSpace(c.FormValue("prospect_contact_name"))
	p.ProspectContactTitle = strings.TrimSpace(c.FormValue("prospect_contact_title"))
	p.ProspectContactPhone = strings.TrimSpace(c.FormValue("prospect_contact_phone"))
	p.ProspectContactEmail = strings.TrimSpace(c.FormValue("prospect_contact_email"))
	p.SalesOwnerID = strings.TrimSpace(c.FormValue("sales_owner_id"))
	p.SalesOwner = strings.TrimSpace(c.FormValue("sales_owner"))
	p.ExpectedAmount = model.ParseExpectedAmount(c.FormValue("expected_amount"))
	win := model.ParseWinProbability(c.FormValue("win_probability"))
	if win < 0 {
		p.WinProbability = model.DefaultWinProbability(p.SalesStage)
	} else {
		p.WinProbability = win
	}
	p.Competitor = strings.TrimSpace(c.FormValue("competitor"))
	p.LeadSource = strings.TrimSpace(c.FormValue("lead_source"))
}

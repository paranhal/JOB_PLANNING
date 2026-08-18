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
	}
	if p.Color == "" {
		p.Color = "#3B82F6"
	}
	if p.Status == "" {
		p.Status = model.WBProjectActive
	}
	return p
}

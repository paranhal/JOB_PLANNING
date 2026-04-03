package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type RelationHandler struct {
	repo         *repository.RelationRepo
	customerRepo *repository.CustomerRepo
	codeRepo     *repository.CodeRepo
}

func (h *RelationHandler) List(c echo.Context) error {
	customerID := c.QueryParam("customer_id")
	assetID := c.QueryParam("asset_id")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	items, total, err := h.repo.List(customerID, assetID, page, 20)
	if err != nil {
		return err
	}
	totalPages := (total + 19) / 20
	customers, _ := h.customerRepo.ListAll()
	relTypes, _ := h.codeRepo.ActiveByGroup("relation_type")
	compTypes, _ := h.codeRepo.ActiveByGroup("company_type")

	return c.Render(http.StatusOK, "relation/list.html", map[string]interface{}{
		"Title": "수행관계 관리", "Active": "relations",
		"Items": items, "Total": total, "Page": page, "TotalPages": totalPages,
		"CustomerID": customerID, "Customers": customers,
		"RelTypes": relTypes, "CompTypes": compTypes,
	})
}

func (h *RelationHandler) New(c echo.Context) error {
	customers, _ := h.customerRepo.ListAll()
	relTypes, _ := h.codeRepo.ActiveByGroup("relation_type")
	compTypes, _ := h.codeRepo.ActiveByGroup("company_type")
	rel := &model.PerformanceRelation{IsActive: true}
	if cid := c.QueryParam("customer_id"); cid != "" {
		rel.CustomerID = cid
	}
	return c.Render(http.StatusOK, "relation/form.html", map[string]interface{}{
		"Title": "수행관계 등록", "Active": "relations", "IsNew": true,
		"Relation": rel, "Customers": customers,
		"RelTypes": relTypes, "CompTypes": compTypes,
	})
}

func (h *RelationHandler) Create(c echo.Context) error {
	p := bindRelation(c)
	if err := h.repo.Create(p); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/relations")
}

func (h *RelationHandler) Edit(c echo.Context) error {
	p, err := h.repo.GetByID(c.Param("id"))
	if err != nil || p == nil {
		return echo.ErrNotFound
	}
	customers, _ := h.customerRepo.ListAll()
	relTypes, _ := h.codeRepo.ActiveByGroup("relation_type")
	compTypes, _ := h.codeRepo.ActiveByGroup("company_type")
	return c.Render(http.StatusOK, "relation/form.html", map[string]interface{}{
		"Title": "수행관계 수정", "Active": "relations", "IsNew": false,
		"Relation": p, "Customers": customers,
		"RelTypes": relTypes, "CompTypes": compTypes,
	})
}

func (h *RelationHandler) Update(c echo.Context) error {
	p := bindRelation(c)
	p.RelationID = c.Param("id")
	if err := h.repo.Update(p); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/relations")
}

func (h *RelationHandler) Delete(c echo.Context) error {
	h.repo.Delete(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, "/relations")
}

func bindRelation(c echo.Context) *model.PerformanceRelation {
	return &model.PerformanceRelation{
		CustomerID:   c.FormValue("customer_id"),
		AssetID:      c.FormValue("asset_id"),
		RelationType: c.FormValue("relation_type"),
		CompanyType:  c.FormValue("company_type"),
		CompanyName:  c.FormValue("company_name"),
		ContactName:  c.FormValue("contact_name"),
		ContactPhone: c.FormValue("contact_phone"),
		ContactEmail: c.FormValue("contact_email"),
		StartDate:    c.FormValue("start_date"),
		EndDate:      c.FormValue("end_date"),
		IsActive:     c.FormValue("is_active") != "0",
		Notes:        c.FormValue("notes"),
	}
}

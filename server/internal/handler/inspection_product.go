package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func canViewInspectionProducts(c echo.Context) bool {
	if currentRole(c) == model.RoleCLevel {
		return true
	}
	return isAdminRole(c) || isReadOnly(c) || hasPerm(c, model.PermCodesEdit) || hasPerm(c, model.PermCodesUsers)
}

func canWriteInspectionProducts(c echo.Context) bool {
	if isReadOnly(c) || viewAsActive(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	return isAdminRole(c) || hasPerm(c, model.PermCodesEdit) || hasPerm(c, model.PermCodesUsers)
}

func (h *MaintenanceHandler) requireProductView(c echo.Context) error {
	if !canViewInspectionProducts(c) {
		return echo.ErrForbidden
	}
	return nil
}

func (h *MaintenanceHandler) requireProductWrite(c echo.Context) error {
	if err := h.requireProductView(c); err != nil {
		return err
	}
	if !canWriteInspectionProducts(c) {
		return echo.ErrForbidden
	}
	return nil
}

func (h *MaintenanceHandler) inspectionProductOrg(c echo.Context) (string, error) {
	org := currentOrg(c)
	if org == repository.OrgAll {
		pick := strings.TrimSpace(c.QueryParam("view_org_id"))
		if pick == "" {
			pick = strings.TrimSpace(c.FormValue("org_id"))
		}
		if pick == "" || pick == repository.OrgAll {
			return "", nil
		}
		return pick, nil
	}
	return org, nil
}

func (h *MaintenanceHandler) orgProductList(orgID string) []model.InspectionProduct {
	if h == nil || h.repo == nil || orgID == "" || orgID == repository.OrgAll {
		return nil
	}
	list, _ := h.repo.ListInspectionProducts(orgID, false)
	if list == nil && !model.UseLegacyInspectionSlots(orgID, h.repo.CountInspectionProducts(orgID)) {
		return []model.InspectionProduct{}
	}
	return list
}

func (h *MaintenanceHandler) useOrgProductLegend(orgID string) bool {
	if h == nil || h.repo == nil {
		return false
	}
	return !model.UseLegacyInspectionSlots(orgID, h.repo.CountInspectionProducts(orgID))
}

func (h *MaintenanceHandler) ListInspectionProducts(c echo.Context) error {
	if err := h.requireProductView(c); err != nil {
		return err
	}
	org, _ := h.inspectionProductOrg(c)
	var items []model.InspectionProduct
	var err error
	if org != "" {
		items, err = h.repo.ListInspectionProducts(org, true)
		if err != nil {
			return err
		}
	}
	return c.Render(http.StatusOK, "maintenance/products.html", map[string]interface{}{
		"Title":          "점검 제품",
		"Active":         NavMaintenanceProducts,
		"Items":          items,
		"OrgID":          org,
		"NeedOrg":        org == "",
		"CanWrite":       canWriteInspectionProducts(c),
		"ShowImport":     org == model.OrgIDLibrary && canWriteInspectionProducts(c),
		"Legacy":         org == model.OrgIDLibrary && h.repo.CountInspectionProducts(org) == 0,
		"FlashOK":    c.QueryParam("ok"),
		"FlashErr":   c.QueryParam("err"),
	})
}

func (h *MaintenanceHandler) SaveInspectionProduct(c echo.Context) error {
	if err := h.requireProductWrite(c); err != nil {
		return err
	}
	org, _ := h.inspectionProductOrg(c)
	if org == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "조직을 선택하세요")
	}
	code := strings.TrimSpace(c.FormValue("code"))
	name := strings.TrimSpace(c.FormValue("name"))
	if code == "" {
		code = model.SuggestInspectionProductCode(name)
	}
	sortN, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("sort_order")))
	p := &model.InspectionProduct{OrgID: org, Code: code, Name: name, SortOrder: sortN, IsActive: true}
	if err := h.repo.CreateInspectionProduct(p); err != nil {
		return c.Redirect(http.StatusSeeOther, "/maintenance/products?err="+url.QueryEscape(err.Error())+orgQuery(org))
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/products?ok=1"+orgQuery(org))
}

func (h *MaintenanceHandler) UpdateInspectionProduct(c echo.Context) error {
	if err := h.requireProductWrite(c); err != nil {
		return err
	}
	org, _ := h.inspectionProductOrg(c)
	id := c.Param("id")
	p, err := h.repo.GetInspectionProduct(id)
	if err != nil || p == nil {
		return echo.ErrNotFound
	}
	if org != "" && p.OrgID != org {
		return echo.ErrNotFound
	}
	if currentOrg(c) != repository.OrgAll && p.OrgID != currentOrg(c) {
		return echo.ErrNotFound
	}
	p.Name = strings.TrimSpace(c.FormValue("name"))
	p.Code = strings.TrimSpace(c.FormValue("code"))
	p.SortOrder, _ = strconv.Atoi(strings.TrimSpace(c.FormValue("sort_order")))
	if err := h.repo.UpdateInspectionProduct(p); err != nil {
		return c.Redirect(http.StatusSeeOther, "/maintenance/products?err="+url.QueryEscape(err.Error())+orgQuery(p.OrgID))
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/products?ok=1"+orgQuery(p.OrgID))
}

func (h *MaintenanceHandler) ToggleInspectionProduct(c echo.Context) error {
	if err := h.requireProductWrite(c); err != nil {
		return err
	}
	p, err := h.repo.GetInspectionProduct(c.Param("id"))
	if err != nil || p == nil {
		return echo.ErrNotFound
	}
	if currentOrg(c) != repository.OrgAll && p.OrgID != currentOrg(c) {
		return echo.ErrNotFound
	}
	if err := h.repo.SetInspectionProductActive(p.ProductID, !p.IsActive); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/products?ok=1"+orgQuery(p.OrgID))
}

func (h *MaintenanceHandler) ImportLegacyInspectionProducts(c echo.Context) error {
	if err := h.requireProductWrite(c); err != nil {
		return err
	}
	org, _ := h.inspectionProductOrg(c)
	if org != model.OrgIDLibrary {
		return echo.NewHTTPError(http.StatusForbidden, "도서관 조직에서만 가져올 수 있습니다")
	}
	if _, err := h.repo.ImportLegacyLibraryProducts(); err != nil {
		return c.Redirect(http.StatusSeeOther, "/maintenance/products?err="+url.QueryEscape(err.Error())+orgQuery(org))
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/products?ok=imported"+orgQuery(org))
}

func orgQuery(org string) string {
	if org == "" || org == repository.OrgAll {
		return ""
	}
	if org == model.OrgIDLibrary {
		return ""
	}
	return "&view_org_id=" + org
}

package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func canViewProjectEquip(c echo.Context) bool {
	if ctxBool(c, "deny_project_equip") || ctxBool(c, "is_test") {
		return false
	}
	r := currentRole(c)
	if r == model.RoleObserver || r == model.RoleTester || r == "partner" {
		return false
	}
	if r == model.RoleCLevel {
		return true
	}
	return canViewProjects(c)
}

func canWriteProjectEquip(c echo.Context, h *ProjectHandler, p *model.WorkProject, orgID string) bool {
	if !canViewProjectEquip(c) || isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	role := currentRole(c)
	if role == model.RoleVisionAdmin {
		return true
	}
	if model.IsAdminGrade(role) || role == model.RoleSales {
		home := loginHomeOrg(c)
		view := currentOrg(c)
		if home != "" && home != orgID && view != orgID && view != repository.OrgAll {
			return false
		}
		return true
	}
	if h != nil && h.pe != nil && currentUserID(c) != "" && p != nil {
		return h.pe.AssignedUser(p.ProjectID, currentUserID(c))
	}
	return false
}

func canWriteEquipCategory(c echo.Context) bool {
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	r := currentRole(c)
	return r == model.RoleVisionAdmin || model.IsAdminGrade(r)
}

func canDownloadEquipCategory(c echo.Context) bool {
	if currentRole(c) == model.RoleCLevel {
		return canViewProjectEquip(c)
	}
	return canWriteEquipCategory(c)
}

func (h *ProjectHandler) loadEquipProject(c echo.Context) (*model.WorkProject, string, error) {
	if !canViewProjectEquip(c) {
		return nil, "", echo.ErrForbidden
	}
	p, err := h.repo.Get(c.Param("id"))
	if err != nil || p == nil {
		return nil, "", echo.ErrNotFound
	}
	org, _ := h.pe.ProjectOrg(p.ProjectID)
	if org == "" || org == repository.OrgAll {
		org = loginHomeOrg(c)
	}
	if org == "" || org == repository.OrgAll {
		org = model.OrgIDLibrary
	}
	view := currentOrg(c)
	if view != "" && view != repository.OrgAll && org != "" && org != view {
		if currentRole(c) != model.RoleVisionAdmin && currentRole(c) != model.RoleCLevel {
			return nil, "", echo.ErrNotFound
		}
	}
	return p, org, nil
}

func redirectEquipErr(c echo.Context, path, msg string) error {
	return c.Redirect(http.StatusSeeOther, path+"?err="+url.QueryEscape(msg))
}

func (h *ProjectHandler) Years(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	years, _ := h.pe.ListYears(p.ProjectID)
	sum, warn := h.pe.YearSumWarn(p.ProjectID, p.ContractAmount)
	return c.Render(http.StatusOK, "project/years.html", map[string]interface{}{
		"Title": p.DisplayName() + " · 연차", "Active": NavProjects, "Project": p,
		"Years": years, "Sum": sum, "Warn": warn, "OrgID": org,
		"CanWrite": canWriteProjectEquip(c, h, p, org),
		"FlashOK":  c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
	})
}

func (h *ProjectHandler) YearSave(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	back := "/projects/" + p.ProjectID + "/years"
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	no, _ := strconv.Atoi(c.FormValue("year_no"))
	amt, _ := strconv.Atoi(c.FormValue("amount"))
	y := &model.ContractYear{
		ID: c.FormValue("year_id"), ProjectID: p.ProjectID, YearNo: no,
		Label: strings.TrimSpace(c.FormValue("label")), StartDate: c.FormValue("start_date"),
		EndDate: c.FormValue("end_date"), Amount: amt, Status: c.FormValue("status"),
		Note: c.FormValue("note"), OrgID: org,
	}
	if err := h.pe.SaveYear(y, currentUserID(c)); err != nil {
		return redirectEquipErr(c, back, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, back+"?ok=1")
}

func (h *ProjectHandler) Sites(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	epage, _ := strconv.Atoi(c.QueryParam("epage"))
	size, _ := strconv.Atoi(c.QueryParam("size"))
	q, region, st := c.QueryParam("q"), c.QueryParam("region"), c.QueryParam("status")
	sites, total, _ := h.pe.ListSites(p.ProjectID, q, region, st, page, size)
	equips, etotal, _ := h.pe.ListEquipment(p.ProjectID, c.QueryParam("site_id"), c.QueryParam("equip_status"), c.QueryParam("eq"), epage, size)
	years, _ := h.pe.ListYears(p.ProjectID)
	cats, _ := h.pe.ListCategories(org, false)
	smap, emap, pending := h.pe.Summary(p.ProjectID)
	prog, _ := h.pe.SiteProgress(p.ProjectID)
	return c.Render(http.StatusOK, "project/sites.html", map[string]interface{}{
		"Title": p.DisplayName() + " · 사이트·장비", "Active": NavProjects, "Project": p,
		"Sites": sites, "SiteTotal": total, "Equips": equips, "EquipTotal": etotal,
		"Years": years, "Cats": cats, "SiteCounts": smap, "EquipCounts": emap, "Pending": pending,
		"Progress": prog,
		"Q": q, "Region": region, "Status": st, "CanWrite": canWriteProjectEquip(c, h, p, org),
		"FlashOK": c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"Preview": nil, "Apply": false,
	})
}

func (h *ProjectHandler) SitesAddSchools(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	ids := c.Request().Form["customer_id"]
	if _, err := h.pe.AddSitesFromCustomers(p.ProjectID, org, currentUserID(c), ids); err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"/sites?ok=1")
}

func (h *ProjectHandler) SitesBulkStatus(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	ids := c.Request().Form["site_id"]
	if err := h.pe.BulkSiteStatus(p.ProjectID, currentUserID(c), c.FormValue("status"), c.FormValue("event_date"), c.FormValue("reason"), ids); err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"/sites?ok=1")
}

func (h *ProjectHandler) SitesPick(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	parentID := strings.TrimSpace(c.QueryParam("parent_id"))
	q, region := c.QueryParam("q"), c.QueryParam("region")
	parents, _ := h.pe.ListParentOrgs(org)
	if parentID == "" && len(parents) > 0 {
		for _, par := range parents {
			if par.Name == model.SchoolParentOfficialName {
				parentID = par.CustomerID
				break
			}
		}
		if parentID == "" {
			parentID = parents[0].CustomerID
		}
	}
	picks, total, _ := h.pe.ListSchoolPick(p.ProjectID, org, parentID, q, region, page, 100)
	return c.Render(http.StatusOK, "project/sites_pick.html", map[string]interface{}{
		"Title": p.DisplayName() + " · 대상 학교 추가", "Active": NavProjects, "Project": p,
		"Parents": parents, "ParentID": parentID, "Picks": picks, "Total": total,
		"Q": q, "Region": region, "Page": page, "CanWrite": canWriteProjectEquip(c, h, p, org),
		"FlashOK": c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
	})
}

func (h *ProjectHandler) SiteSchoolTemplateXLSX(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	_ = org
	_ = p
	if !canViewProjectEquip(c) {
		return echo.ErrForbidden
	}
	raw, err := repository.BuildSiteSchoolWorkbook()
	if err != nil {
		return err
	}
	return writeExcelFile(c, raw, "sites_schools_template.xlsx", "사업사이트_업로드양식.xlsx")
}

func (h *ProjectHandler) SiteSchoolImportPreview(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readEquipUpload(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", "파일을 선택하세요")
	}
	prev, err := h.pe.PreviewSiteSchool(p.ProjectID, org, raw, c.FormValue("mode"), c.FormValue("create_missing") == "1")
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", err.Error())
	}
	prev.FileName = name
	return h.renderSitesPreview(c, p, org, prev, false)
}

func (h *ProjectHandler) SiteSchoolImportApply(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readEquipUpload(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", "파일을 선택하세요")
	}
	prev, err := h.pe.ApplySiteSchool(p.ProjectID, org, currentUserID(c), name, c.FormValue("mode"), raw, c.FormValue("create_missing") == "1")
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", err.Error())
	}
	return h.renderSitesPreview(c, p, org, prev, true)
}

func (h *ProjectHandler) SiteSave(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	back := "/projects/" + p.ProjectID + "/sites"
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	so, _ := strconv.Atoi(c.FormValue("sort_order"))
	s := &model.ProjectSite{
		ID: c.FormValue("site_id"), ProjectID: p.ProjectID, CustomerID: c.FormValue("customer_id"),
		SiteName: c.FormValue("site_name"), Region: c.FormValue("region"), Address: c.FormValue("address"),
		ContactNote: c.FormValue("contact_note"), Status: c.FormValue("status"), SortOrder: so,
		Note: c.FormValue("note"), OrgID: org,
	}
	if err := h.pe.SaveSite(s); err != nil {
		return redirectEquipErr(c, back, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, back+"?ok=1")
}

func (h *ProjectHandler) SiteOff(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	_ = h.pe.DeactivateSite(c.FormValue("site_id"), p.ProjectID)
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"/sites?ok=1")
}

func (h *ProjectHandler) EquipSave(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	back := "/projects/" + p.ProjectID + "/sites"
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	qty, _ := strconv.Atoi(c.FormValue("qty"))
	if err := c.Request().ParseForm(); err != nil {
		return redirectEquipErr(c, back, "입력을 확인하세요")
	}
	e := &model.ProjectEquipment{
		ID: c.FormValue("equip_id"), ProjectID: p.ProjectID, SiteID: c.FormValue("site_id"),
		YearID: c.FormValue("year_id"), CategoryID: c.FormValue("category_id"),
		ProductName: c.FormValue("product_name"), Model: c.FormValue("model"),
		Manufacturer: c.FormValue("manufacturer"), Qty: qty, SerialNo: c.FormValue("serial_no"),
		InstallLocation: c.FormValue("install_location"), InstallDate: c.FormValue("install_date"),
		Status: c.FormValue("status"), AssetID: c.FormValue("asset_id"), Note: c.FormValue("note"), OrgID: org,
		Values: map[string]string{},
	}
	for k, vs := range c.Request().Form {
		if strings.HasPrefix(k, "f_") && len(vs) > 0 {
			e.Values[strings.TrimPrefix(k, "f_")] = vs[0]
		}
	}
	if err := h.pe.SaveEquipment(e); err != nil {
		return redirectEquipErr(c, back, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, back+"?ok=1")
}

func (h *ProjectHandler) EquipOff(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	_ = h.pe.DeactivateEquipment(c.FormValue("equip_id"), p.ProjectID)
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"/sites?ok=1")
}

func (h *ProjectHandler) EquipLinkAsset(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	_ = h.pe.LinkAsset(c.FormValue("equip_id"), p.ProjectID, c.FormValue("asset_id"))
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"/sites?ok=linked")
}

func (h *ProjectHandler) EquipCats(c echo.Context) error {
	if !canViewProjectEquip(c) {
		return echo.ErrForbidden
	}
	org, needOrg, cats := h.equipCatsList(c)
	return h.renderEquipCats(c, org, needOrg, cats, nil, false)
}

func (h *ProjectHandler) renderEquipCats(c echo.Context, org string, needOrg bool, cats []model.EquipmentCategory, prev *model.EquipCatImportPreview, applied bool) error {
	return c.Render(http.StatusOK, "project/equip_cats.html", map[string]interface{}{
		"Title": "장비 분류", "Active": NavEquipCategories, "Cats": cats, "OrgID": org,
		"CanWrite": canWriteEquipCategory(c), "CanDownload": canDownloadEquipCategory(c),
		"NeedOrg": needOrg, "FlashOK": c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"Preview": prev, "Apply": applied,
	})
}

func (h *ProjectHandler) catImportScope(c echo.Context) (repository.CatImportScope, error) {
	allowed := map[string]bool{}
	role := currentRole(c)
	if role == model.RoleVisionAdmin {
		ids, err := h.pe.ListActiveOrgIDs()
		if err != nil {
			return repository.CatImportScope{}, err
		}
		for _, id := range ids {
			if id != "" && id != repository.OrgAll {
				allowed[id] = true
			}
		}
		return repository.CatImportScope{Allowed: allowed}, nil
	}
	home := loginHomeOrg(c)
	if home == "" || home == repository.OrgAll {
		home = model.OrgIDLibrary
	}
	allowed[home] = true
	return repository.CatImportScope{Allowed: allowed}, nil
}

func (h *ProjectHandler) catExportOrgIDs(c echo.Context) []string {
	if currentRole(c) == model.RoleVisionAdmin {
		if q := strings.TrimSpace(c.QueryParam("org_id")); q != "" && q != repository.OrgAll {
			return []string{q}
		}
		if view := currentOrg(c); view != "" && view != repository.OrgAll {
			return []string{view}
		}
		ids, _ := h.pe.ListActiveOrgIDs()
		var out []string
		for _, id := range ids {
			if id != "" && id != repository.OrgAll {
				out = append(out, id)
			}
		}
		return out
	}
	home := loginHomeOrg(c)
	if home == "" || home == repository.OrgAll {
		home = model.OrgIDLibrary
	}
	return []string{home}
}

func (h *ProjectHandler) readCatUpload(c echo.Context) ([]byte, string, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, "", err
	}
	src, err := fh.Open()
	if err != nil {
		return nil, "", err
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, 2<<20+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > 2<<20 {
		return nil, "", fmt.Errorf("파일은 2MB 이하여야 합니다")
	}
	return raw, fh.Filename, nil
}

func (h *ProjectHandler) EquipCatTemplateXLSX(c echo.Context) error {
	if !canDownloadEquipCategory(c) {
		return echo.ErrForbidden
	}
	raw, err := buildEquipCatTemplateBytes()
	if err != nil {
		return err
	}
	return writeExcelFile(c, raw, "equip_cat_template.xlsx", "장비분류표_업로드양식.xlsx")
}

func (h *ProjectHandler) EquipCatExportXLSX(c echo.Context) error {
	if !canDownloadEquipCategory(c) {
		return echo.ErrForbidden
	}
	cats, fields, err := h.pe.CurrentCategoryExport(h.catExportOrgIDs(c))
	if err != nil {
		return err
	}
	raw, err := repository.BuildEquipCatWorkbook(repository.EquipCatGuideLines(), cats, fields, repository.EquipCatSampleRows())
	if err != nil {
		return err
	}
	return writeExcelFile(c, raw, "equip_cat_current.xlsx", "장비분류표_현재.xlsx")
}

func (h *ProjectHandler) EquipCatImportPreview(c echo.Context) error {
	if !canWriteEquipCategory(c) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readCatUpload(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	scope, err := h.catImportScope(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	prev, err := h.pe.PreviewCategoryImport(raw, name, scope)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	org, needOrg, cats := h.equipCatsList(c)
	return h.renderEquipCats(c, org, needOrg, cats, prev, false)
}

func (h *ProjectHandler) EquipCatImportApply(c echo.Context) error {
	if !canWriteEquipCategory(c) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readCatUpload(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	scope, err := h.catImportScope(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	prev, err := h.pe.ApplyCategoryImport(currentUserID(c), name, raw, scope)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	org, needOrg, cats := h.equipCatsList(c)
	return h.renderEquipCats(c, org, needOrg, cats, prev, true)
}

func (h *ProjectHandler) EquipCatImportErrors(c echo.Context) error {
	if !canWriteEquipCategory(c) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readCatUpload(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	scope, err := h.catImportScope(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	prev, err := h.pe.PreviewCategoryImport(raw, name, scope)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	buf, err := buildEquipCatErrorExcel(prev)
	if err != nil {
		return err
	}
	return writeExcelFile(c, buf, "equip_cat_errors.xlsx", "장비분류표_오류목록.xlsx")
}

func (h *ProjectHandler) equipCatsList(c echo.Context) (string, bool, []model.EquipmentCategory) {
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		org = loginHomeOrg(c)
	}
	if currentRole(c) == model.RoleVisionAdmin && strings.TrimSpace(c.QueryParam("org_id")) != "" {
		org = c.QueryParam("org_id")
	}
	needOrg := org == "" || org == repository.OrgAll
	if needOrg {
		org = ""
	}
	var cats []model.EquipmentCategory
	if org != "" {
		cats, _ = h.pe.ListCategories(org, true)
	}
	return org, needOrg, cats
}

func (h *ProjectHandler) EquipCatSave(c echo.Context) error {
	if !canWriteEquipCategory(c) {
		return echo.ErrForbidden
	}
	org, err := resolveInsertOrg(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	active := c.FormValue("is_active") != "0"
	cat := &model.EquipmentCategory{
		ID: c.FormValue("id"), OrgID: org, ParentID: c.FormValue("parent_id"),
		Code: c.FormValue("code"), Name: c.FormValue("name"), IsActive: active,
	}
	cat.SortOrder, _ = strconv.Atoi(c.FormValue("sort_order"))
	if err := h.pe.SaveCategory(cat); err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/projects/equipment-categories?ok=1")
}

func (h *ProjectHandler) EquipCatOff(c echo.Context) error {
	if !canWriteEquipCategory(c) {
		return echo.ErrForbidden
	}
	org, err := resolveInsertOrg(c)
	if err != nil {
		return echo.ErrForbidden
	}
	if err := h.pe.DeleteCategory(c.FormValue("id"), org); err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/projects/equipment-categories?ok=1")
}

func (h *ProjectHandler) EquipFieldSave(c echo.Context) error {
	if !canWriteEquipCategory(c) {
		return echo.ErrForbidden
	}
	f := &model.EquipmentField{
		ID: c.FormValue("id"), CategoryID: c.FormValue("category_id"), FieldKey: c.FormValue("field_key"),
		Label: c.FormValue("label"), FieldType: c.FormValue("field_type"), Options: c.FormValue("options"),
		Required: c.FormValue("required") == "1", IsActive: c.FormValue("is_active") != "0",
	}
	f.SortOrder, _ = strconv.Atoi(c.FormValue("sort_order"))
	if err := h.pe.SaveField(f); err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/projects/equipment-categories?ok=1")
}

func (h *ProjectHandler) EquipFieldOff(c echo.Context) error {
	if !canWriteEquipCategory(c) {
		return echo.ErrForbidden
	}
	if err := h.pe.DeleteField(c.FormValue("id")); err != nil {
		return redirectEquipErr(c, "/projects/equipment-categories", err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/projects/equipment-categories?ok=1")
}

func (h *ProjectHandler) readEquipUpload(c echo.Context) ([]byte, string, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, "", err
	}
	src, err := fh.Open()
	if err != nil {
		return nil, "", err
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, 10<<20+1))
	if err != nil {
		return nil, "", err
	}
	return raw, fh.Filename, nil
}

func (h *ProjectHandler) renderSitesPreview(c echo.Context, p *model.WorkProject, org string, prev *model.EquipImportPreview, applied bool) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("size"))
	sites, total, _ := h.pe.ListSites(p.ProjectID, "", "", "", page, size)
	equips, etotal, _ := h.pe.ListEquipment(p.ProjectID, "", "", "", 1, size)
	years, _ := h.pe.ListYears(p.ProjectID)
	cats, _ := h.pe.ListCategories(org, false)
	smap, emap, pending := h.pe.Summary(p.ProjectID)
	prog, _ := h.pe.SiteProgress(p.ProjectID)
	return c.Render(http.StatusOK, "project/sites.html", map[string]interface{}{
		"Title": p.DisplayName() + " · 사이트·장비", "Active": NavProjects, "Project": p,
		"Sites": sites, "SiteTotal": total, "Equips": equips, "EquipTotal": etotal,
		"Years": years, "Cats": cats, "SiteCounts": smap, "EquipCounts": emap, "Pending": pending,
		"Progress": prog,
		"CanWrite": canWriteProjectEquip(c, h, p, org),
		"Preview":  prev, "Apply": applied,
	})
}

func (h *ProjectHandler) EquipImportPreview(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readEquipUpload(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", "파일을 선택하세요")
	}
	prev, err := h.pe.PreviewEquipImport(p.ProjectID, raw, name)
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", err.Error())
	}
	return h.renderSitesPreview(c, p, org, prev, false)
}

func (h *ProjectHandler) EquipImportApply(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readEquipUpload(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", "파일을 선택하세요")
	}
	prev, err := h.pe.ApplyEquipImport(p.ProjectID, currentUserID(c), name, raw)
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", err.Error())
	}
	return h.renderSitesPreview(c, p, org, prev, true)
}

func (h *ProjectHandler) EquipImportErrors(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	if !canWriteProjectEquip(c, h, p, org) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readEquipUpload(c)
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", "파일을 선택하세요")
	}
	prev, err := h.pe.PreviewEquipImport(p.ProjectID, raw, name)
	if err != nil {
		return redirectEquipErr(c, "/projects/"+p.ProjectID+"/sites", err.Error())
	}
	buf, err := buildEquipErrorExcel(prev)
	if err != nil {
		return err
	}
	return writeExcelFile(c, buf, "equip_import_errors.xlsx", "사이트장비_오류목록.xlsx")
}

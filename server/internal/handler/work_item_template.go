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

type WorkItemTemplateHandler struct {
	repo     *repository.ItemTemplateRepo
	codeRepo *repository.CodeRepo
	orgs     *repository.OrgRepo
}

func canManageItemTemplates(c echo.Context) bool {
	return isAdminRole(c) && !isReadOnly(c)
}

func canEditItemTemplate(c echo.Context, orgID string) bool {
	if !canManageItemTemplates(c) {
		return false
	}
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return isVisionRole(c)
	}
	if isVisionRole(c) {
		return true
	}
	return currentOrg(c) == orgID
}

func witRedirect(ok, errMsg string) string {
	u := "/work-item-templates"
	v := url.Values{}
	if ok != "" {
		v.Set("ok", ok)
	}
	if errMsg != "" {
		v.Set("err", errMsg)
	}
	if s := v.Encode(); s != "" {
		u += "?" + s
	}
	return u
}

func (h *WorkItemTemplateHandler) Page(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	kind := strings.TrimSpace(c.QueryParam("kind"))
	q := strings.TrimSpace(c.QueryParam("q"))
	inactive := c.QueryParam("inactive") == "1"
	vision := isVisionRole(c)
	scope := currentOrg(c)
	if vision && (scope == repository.OrgAll || scope == "") {
		scope = ""
	}
	list, err := h.repo.List(scope, kind, q, inactive, vision)
	if err != nil {
		return err
	}
	for i := range list {
		list[i].CanEdit = canEditItemTemplate(c, list[i].OrgID)
		if list[i].OrgID == "" {
			list[i].OrgLabel = "전사 공통"
		}
	}
	orgs, _ := h.orgs.ListActive()
	orgNames := map[string]string{}
	for _, o := range orgs {
		orgNames[o.OrgID] = o.OrgName
		if strings.TrimSpace(o.ShortName) != "" {
			orgNames[o.OrgID] = o.ShortName
		}
	}
	for i := range list {
		if list[i].OrgID != "" {
			if n := orgNames[list[i].OrgID]; n != "" {
				list[i].OrgLabel = n
			}
		}
	}
	return c.Render(http.StatusOK, "work_item_template/list.html", map[string]interface{}{
		"Title": "작업 항목 관리", "Active": NavWorkItemTemplates,
		"Items": list, "ItemKinds": defaultItemKinds(h.codeRepo), "ItemUnits": defaultItemUnits(h.codeRepo),
		"Kind": kind, "Q": q, "Inactive": inactive,
		"FlashErr": c.QueryParam("err"), "FlashOK": c.QueryParam("ok"),
		"CanWrite": canManageItemTemplates(c), "IsVision": vision,
		"Orgs": orgs, "Candidates": list, "CurrentOrg": currentOrg(c),
	})
}

func (h *WorkItemTemplateHandler) ListJSON(c echo.Context) error {
	org := currentOrg(c)
	if tOrg := strings.TrimSpace(c.QueryParam("org_id")); tOrg != "" && isVisionRole(c) {
		org = tOrg
	}
	list, err := h.repo.ListActiveForOrg(org)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"templates": repository.ItemTemplatePickJSON(list)})
}

func (h *WorkItemTemplateHandler) bind(c echo.Context) (model.ItemTemplate, []model.ItemTemplateLink, error) {
	t := model.ItemTemplate{
		Name:      strings.TrimSpace(c.FormValue("name")),
		Kind:      strings.TrimSpace(c.FormValue("kind")),
		Unit:      strings.TrimSpace(c.FormValue("unit")),
		Note:      strings.TrimSpace(c.FormValue("note")),
		CreatedBy: currentUserID(c),
		IsActive:  true,
	}
	qty, err := strconv.Atoi(strings.TrimSpace(c.FormValue("default_qty")))
	if err != nil {
		qty = 0
	}
	t.DefaultQty = qty
	ord, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("sort_order")))
	t.SortOrder = ord
	if isVisionRole(c) {
		t.OrgID = strings.TrimSpace(c.FormValue("org_id"))
	} else {
		t.OrgID = currentOrg(c)
		if t.OrgID == repository.OrgAll {
			t.OrgID = ""
		}
	}
	if !canEditItemTemplate(c, t.OrgID) {
		return t, nil, echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	checked := map[string]bool{}
	for _, id := range c.Request().PostForm["link_checked"] {
		checked[strings.TrimSpace(id)] = true
	}
	var links []model.ItemTemplateLink
	for i, id := range c.Request().PostForm["link_child"] {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		links = append(links, model.ItemTemplateLink{
			ChildTemplateID: id,
			DefaultChecked:  checked[id],
			SortOrder:       i + 1,
		})
	}
	return t, links, nil
}

func (h *WorkItemTemplateHandler) Create(c echo.Context) error {
	if !canManageItemTemplates(c) {
		return echo.ErrForbidden
	}
	t, links, err := h.bind(c)
	if err != nil {
		if err == echo.ErrForbidden {
			return err
		}
		return c.Redirect(http.StatusSeeOther, witRedirect("", err.Error()))
	}
	if err := h.repo.Create(&t, links); err != nil {
		return c.Redirect(http.StatusSeeOther, witRedirect("", err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, witRedirect(t.Name+" 등록", ""))
}

func (h *WorkItemTemplateHandler) Update(c echo.Context) error {
	if !canManageItemTemplates(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	cur, err := h.repo.Get(id)
	if err != nil || cur == nil {
		return c.Redirect(http.StatusSeeOther, witRedirect("", "항목을 찾을 수 없습니다"))
	}
	if !canEditItemTemplate(c, cur.OrgID) {
		return echo.ErrForbidden
	}
	t, links, err := h.bind(c)
	if err != nil {
		if err == echo.ErrForbidden {
			return err
		}
		return c.Redirect(http.StatusSeeOther, witRedirect("", err.Error()))
	}
	t.TemplateID = id
	t.OrgID = cur.OrgID
	if isVisionRole(c) {
		t.OrgID = strings.TrimSpace(c.FormValue("org_id"))
		if !canEditItemTemplate(c, t.OrgID) {
			return echo.ErrForbidden
		}
	}
	if err := h.repo.Update(&t, links); err != nil {
		return c.Redirect(http.StatusSeeOther, witRedirect("", err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, witRedirect("저장했습니다", ""))
}

func (h *WorkItemTemplateHandler) Deactivate(c echo.Context) error {
	if !canManageItemTemplates(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	cur, err := h.repo.Get(id)
	if err != nil || cur == nil {
		return c.Redirect(http.StatusSeeOther, witRedirect("", "항목을 찾을 수 없습니다"))
	}
	if !canEditItemTemplate(c, cur.OrgID) {
		return echo.ErrForbidden
	}
	if err := h.repo.Deactivate(id); err != nil {
		return c.Redirect(http.StatusSeeOther, witRedirect("", err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, witRedirect("비활성했습니다", ""))
}

func (h *WorkItemTemplateHandler) Delete(c echo.Context) error {
	if !canManageItemTemplates(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	cur, err := h.repo.Get(id)
	if err != nil || cur == nil {
		return c.Redirect(http.StatusSeeOther, witRedirect("", "항목을 찾을 수 없습니다"))
	}
	if !canEditItemTemplate(c, cur.OrgID) {
		return echo.ErrForbidden
	}
	if err := h.repo.Delete(id); err != nil {
		return c.Redirect(http.StatusSeeOther, witRedirect("", err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, witRedirect("삭제했습니다", ""))
}

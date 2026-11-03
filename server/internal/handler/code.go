package handler

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type CodeHandler struct{ repo *repository.CodeRepo }

func (h *CodeHandler) List(c echo.Context) error {
	group := c.QueryParam("group")
	groups, _ := h.repo.Groups()

	var codes []model.Code
	var err error
	if group != "" {
		codes, err = h.repo.ListByGroup(group)
	} else {
		codes, err = h.repo.ListAll()
	}
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "code/list.html", map[string]interface{}{
		"Title": "코드 관리", "Active": NavCodes,
		"Codes": codes, "Groups": groups, "SelectedGroup": group,
		"ProductCatGroup": repository.ProductCategoryGroup,
		"FlashErr":        c.QueryParam("err"),
	})
}

func codesRedirect(group, errMsg string) string {
	q := "/codes?group=" + url.QueryEscape(group)
	if errMsg != "" {
		q += "&err=" + url.QueryEscape(errMsg)
	}
	return q
}

func (h *CodeHandler) Create(c echo.Context) error {
	sortOrder, _ := strconv.Atoi(c.FormValue("sort_order"))
	code := &model.Code{
		CodeGroup: c.FormValue("code_group"),
		CodeValue: c.FormValue("code_value"),
		CodeName:  c.FormValue("code_name"),
		SortOrder: sortOrder,
		IsActive:  c.FormValue("is_active") != "0",
	}
	if err := h.repo.Create(code); err != nil {
		return c.Redirect(http.StatusSeeOther, codesRedirect(code.CodeGroup, err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, codesRedirect(code.CodeGroup, ""))
}

func (h *CodeHandler) Update(c echo.Context) error {
	sortOrder, _ := strconv.Atoi(c.FormValue("sort_order"))
	code := &model.Code{
		CodeID:    c.Param("id"),
		CodeGroup: c.FormValue("code_group"),
		CodeValue: c.FormValue("code_value"),
		CodeName:  c.FormValue("code_name"),
		SortOrder: sortOrder,
		IsActive:  c.FormValue("is_active") != "0",
	}
	if err := h.repo.Update(code); err != nil {
		return c.Redirect(http.StatusSeeOther, codesRedirect(code.CodeGroup, err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, codesRedirect(code.CodeGroup, ""))
}

func (h *CodeHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	group := c.QueryParam("group")
	if err := h.repo.Delete(id); err != nil {
		if group == "" {
			if cur, e := h.repo.Get(id); e == nil && cur != nil {
				group = cur.CodeGroup
			}
		}
		return c.Redirect(http.StatusSeeOther, codesRedirect(group, err.Error()))
	}
	if group == "" {
		group = c.FormValue("code_group")
	}
	return c.Redirect(http.StatusSeeOther, codesRedirect(group, ""))
}

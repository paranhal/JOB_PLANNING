package handler

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type ProductGroupHandler struct {
	repo *repository.ProductGroupRepo
	hold sync.Map
}

func canManageProductGroups(c echo.Context) bool {
	return isVisionRole(c)
}

func (h *ProductGroupHandler) Page(c echo.Context) error {
	classID := strings.TrimSpace(c.QueryParam("class"))
	q := strings.TrimSpace(c.QueryParam("q"))
	all, err := h.repo.List("", "")
	if err != nil {
		return err
	}
	items, err := h.repo.List(classID, "")
	if err != nil {
		return err
	}
	classes, _ := h.repo.Classes()
	return c.Render(http.StatusOK, "product_group/list.html", map[string]interface{}{
		"Title": "제품군 관리", "Active": NavProductGroups,
		"Items": items, "Classes": classes, "KPI": h.repo.KPI(all),
		"ClassID": classID, "Q": q,
		"FlashErr": c.QueryParam("err"), "FlashOK": c.QueryParam("ok"),
		"CanWrite": canManageProductGroups(c),
		"Unmapped": h.repo.CountUnmappedAssets(),
	})
}

func pgRedirect(ok, errMsg string) string {
	u := "/product-groups"
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

func (h *ProductGroupHandler) Create(c echo.Context) error {
	if !canManageProductGroups(c) {
		return echo.ErrForbidden
	}
	g := bindProductGroup(c)
	g.IsActive = true
	if err := h.repo.Create(g); err != nil {
		return c.Redirect(http.StatusSeeOther, pgRedirect("", err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, pgRedirect(g.GroupID+" 등록", ""))
}

func (h *ProductGroupHandler) Update(c echo.Context) error {
	if !canManageProductGroups(c) {
		return echo.ErrForbidden
	}
	g := bindProductGroup(c)
	g.GroupID = strings.TrimSpace(c.Param("id"))
	cur, err := h.repo.Get(g.GroupID)
	if err != nil || cur == nil {
		return c.Redirect(http.StatusSeeOther, pgRedirect("", repository.ErrProductGroupMissing.Error()))
	}
	g.IsActive = cur.IsActive
	g.SortOrder = cur.SortOrder
	if g.SyncType == "" {
		g.SyncType = cur.SyncType
	}
	if g.SyncCategory == "" {
		g.SyncCategory = cur.SyncCategory
	}
	if err := h.repo.Update(g); err != nil {
		return c.Redirect(http.StatusSeeOther, pgRedirect("", err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, pgRedirect("저장했습니다", ""))
}

func (h *ProductGroupHandler) Delete(c echo.Context) error {
	if !canManageProductGroups(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	if err := h.repo.Delete(id); err != nil {
		return c.Redirect(http.StatusSeeOther, pgRedirect("", err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, pgRedirect("삭제했습니다", ""))
}

func (h *ProductGroupHandler) BulkDelete(c echo.Context) error {
	if !canManageProductGroups(c) {
		return echo.ErrForbidden
	}
	ids := c.Request().Form["ids"]
	if len(ids) == 0 {
		ids = c.Request().PostForm["ids"]
	}
	_ = c.Request().ParseForm()
	ids = c.Request().Form["ids"]
	del, skip, err := h.repo.BulkDelete(ids)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, pgRedirect("", err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, pgRedirect(strconv.Itoa(del)+"건 삭제 · 사용 중 "+strconv.Itoa(skip)+"건 건너뜀", ""))
}

func (h *ProductGroupHandler) Deactivate(c echo.Context) error {
	if !canManageProductGroups(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("id"))
	if err := h.repo.Deactivate(id); err != nil {
		return c.Redirect(http.StatusSeeOther, pgRedirect("", err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, pgRedirect("사용중지했습니다", ""))
}

func (h *ProductGroupHandler) ExportXLSX(c echo.Context) error {
	items, err := h.repo.List(c.QueryParam("class"), c.QueryParam("q"))
	if err != nil {
		return err
	}
	body, err := repository.ProductGroupExportXLSX(items)
	if err != nil {
		return err
	}
	return writeDownloadBytes(c, body, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"product_groups.xlsx", "제품군_목록.xlsx")
}

func (h *ProductGroupHandler) TemplateXLSX(c echo.Context) error {
	body, err := repository.ProductGroupTemplateXLSX()
	if err != nil {
		return err
	}
	return writeDownloadBytes(c, body, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"product_groups_template.xlsx", "제품군_업로드양식.xlsx")
}

func (h *ProductGroupHandler) ImportForm(c echo.Context) error {
	return c.Render(http.StatusOK, "product_group/import.html", map[string]interface{}{
		"Title": "제품군 엑셀 업로드", "Active": NavProductGroups,
		"CanWrite": canManageProductGroups(c), "FlashErr": c.QueryParam("err"),
	})
}

func (h *ProductGroupHandler) ImportPreview(c echo.Context) error {
	if !canManageProductGroups(c) {
		return echo.ErrForbidden
	}
	raw, err := readUpload(c, "file")
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/product-groups/import?err="+url.QueryEscape(err.Error()))
	}
	rows, err := h.repo.PreviewExcel(raw)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/product-groups/import?err="+url.QueryEscape(err.Error()))
	}
	tok := strconv.FormatInt(int64(len(raw)+len(rows)+1), 10) + "-" + strconv.Itoa(len(raw))
	h.hold.Store(tok, raw)
	okn, errn := 0, 0
	for _, r := range rows {
		if r.Err {
			errn++
		} else {
			okn++
		}
	}
	return c.Render(http.StatusOK, "product_group/import.html", map[string]interface{}{
		"Title": "제품군 엑셀 업로드", "Active": NavProductGroups,
		"CanWrite": true, "Preview": true, "Rows": rows, "OKN": okn, "ErrN": errn,
		"PreviewToken": tok,
	})
}

func (h *ProductGroupHandler) ImportApply(c echo.Context) error {
	if !canManageProductGroups(c) {
		return echo.ErrForbidden
	}
	tok := strings.TrimSpace(c.FormValue("preview_token"))
	v, ok := h.hold.Load(tok)
	if !ok {
		return c.Redirect(http.StatusSeeOther, "/product-groups/import?err="+url.QueryEscape("미리보기가 만료되었습니다"))
	}
	raw := v.([]byte)
	h.hold.Delete(tok)
	n, errs, err := h.repo.ImportExcel(raw)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/product-groups/import?err="+url.QueryEscape(err.Error()))
	}
	msg := strconv.Itoa(n) + "건 반영"
	if len(errs) > 0 {
		return c.Redirect(http.StatusSeeOther, pgRedirect("", msg+" · "+strings.Join(errs, " · ")))
	}
	return c.Redirect(http.StatusSeeOther, pgRedirect(msg, ""))
}

func readUpload(c echo.Context, name string) ([]byte, error) {
	fh, err := c.FormFile(name)
	if err != nil {
		return nil, err
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func bindProductGroup(c echo.Context) *model.ProductGroup {
	return &model.ProductGroup{
		ClassID:      strings.TrimSpace(c.FormValue("class_id")),
		TypeName:     strings.TrimSpace(c.FormValue("type_name")),
		DetailName:   strings.TrimSpace(c.FormValue("detail_name")),
		SyncType:     strings.TrimSpace(c.FormValue("sync_type")),
		SyncCategory: strings.TrimSpace(c.FormValue("sync_category")),
	}
}

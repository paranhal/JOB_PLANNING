package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func canManageCauseCategories(c echo.Context) bool {
	if isReadOnly(c) {
		return false
	}
	r := currentRole(c)
	return r == model.RoleOrgAdmin || r == model.RoleAdmin
}

func (h *ASHandler) causeCatOrgID(c echo.Context) string {
	org := loginHomeOrg(c)
	if org == "" || org == repository.OrgAll {
		org = strings.TrimSpace(ctxString(c, "org_id"))
	}
	return org
}

func (h *ASHandler) requireCauseCatAdmin(c echo.Context) (string, error) {
	if !canManageCauseCategories(c) {
		return "", echo.ErrForbidden
	}
	org := h.causeCatOrgID(c)
	if org == "" || org == repository.OrgAll {
		return "", echo.ErrForbidden
	}
	return org, nil
}

func (h *ASHandler) CauseCats(c echo.Context) error {
	org, err := h.requireCauseCatAdmin(c)
	if err != nil {
		return err
	}
	return h.renderCauseCats(c, org, nil, false)
}

func (h *ASHandler) renderCauseCats(c echo.Context, org string, prev *model.CauseCatImportPreview, applied bool) error {
	cats, _ := h.repo.ListCauseCategoriesForOrgAll(org)
	return c.Render(http.StatusOK, "as/cause_cats.html", map[string]interface{}{
		"Title": "원인 분류표", "Active": NavCauseCategories, "Cats": cats, "OrgID": org,
		"FlashOK": c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"Preview": prev, "Apply": applied,
	})
}

func (h *ASHandler) readCauseCatUpload(c echo.Context) ([]byte, string, error) {
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

func (h *ASHandler) CauseCatTemplateXLSX(c echo.Context) error {
	if _, err := h.requireCauseCatAdmin(c); err != nil {
		return err
	}
	raw, err := repository.BuildCauseCatTemplateBytes()
	if err != nil {
		return err
	}
	return writeExcelFile(c, raw, "cause_cat_template.xlsx", "원인분류_양식.xlsx")
}

func (h *ASHandler) CauseCatExportXLSX(c echo.Context) error {
	org, err := h.requireCauseCatAdmin(c)
	if err != nil {
		return err
	}
	rows, err := h.repo.CurrentCauseCatExport(org)
	if err != nil {
		return err
	}
	raw, err := repository.BuildCauseCatWorkbook(repository.CauseCatGuideLines(), rows, repository.CauseTypeSheetRows(), nil)
	if err != nil {
		return err
	}
	return writeExcelFile(c, raw, "cause_cat_current.xlsx", "원인분류_현재.xlsx")
}

func (h *ASHandler) CauseCatImportPreview(c echo.Context) error {
	org, err := h.requireCauseCatAdmin(c)
	if err != nil {
		return err
	}
	raw, name, err := h.readCauseCatUpload(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/as/cause-categories?err="+err.Error())
	}
	prev, err := h.repo.PreviewCauseCatImport(raw, name, org)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/as/cause-categories?err="+err.Error())
	}
	return h.renderCauseCats(c, org, prev, false)
}

func (h *ASHandler) CauseCatImportApply(c echo.Context) error {
	org, err := h.requireCauseCatAdmin(c)
	if err != nil {
		return err
	}
	raw, name, err := h.readCauseCatUpload(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/as/cause-categories?err="+err.Error())
	}
	prev, err := h.repo.ApplyCauseCatImport(currentUserID(c), name, org, raw)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/as/cause-categories?err="+err.Error())
	}
	return h.renderCauseCats(c, org, prev, true)
}

func (h *ASHandler) CauseCatImportErrors(c echo.Context) error {
	org, err := h.requireCauseCatAdmin(c)
	if err != nil {
		return err
	}
	raw, name, err := h.readCauseCatUpload(c)
	if err != nil {
		return err
	}
	prev, err := h.repo.PreviewCauseCatImport(raw, name, org)
	if err != nil {
		return err
	}
	buf, err := repository.BuildCauseCatErrorExcel(prev)
	if err != nil {
		return err
	}
	return writeExcelFile(c, buf, "cause_cat_errors.xlsx", "원인분류_오류목록.xlsx")
}

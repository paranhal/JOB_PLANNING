package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func canDownloadContractImport(c echo.Context) bool {
	return canViewProjects(c)
}

func canWriteContractImport(c echo.Context) bool {
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	r := currentRole(c)
	return r == model.RoleVisionAdmin || model.IsAdminGrade(r) || r == model.RoleTech
}

func (h *ProjectHandler) ContractImport(c echo.Context) error {
	if !canDownloadContractImport(c) {
		return echo.ErrForbidden
	}
	return h.renderContractImport(c, nil, false)
}

func (h *ProjectHandler) renderContractImport(c echo.Context, prev *model.ContractImportPreview, applied bool) error {
	return c.Render(http.StatusOK, "project/import.html", map[string]interface{}{
		"Title":       "사업 엑셀 등록",
		"Active":      NavProjectsImport,
		"CanWrite":    canWriteContractImport(c),
		"CanDownload": canDownloadContractImport(c),
		"Preview":     prev,
		"Apply":       applied,
		"FlashErr":    c.QueryParam("err"),
	})
}

func (h *ProjectHandler) ContractImportTemplateXLSX(c echo.Context) error {
	if !canDownloadContractImport(c) {
		return echo.ErrForbidden
	}
	raw, err := repository.BuildContractWorkbook(repository.ContractGuideLines(), nil)
	if err != nil {
		return err
	}
	return writeExcelFile(c, raw, "contract_template.xlsx", "사업등록_업로드양식.xlsx")
}

func (h *ProjectHandler) readContractUpload(c echo.Context) ([]byte, string, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, "", err
	}
	src, err := fh.Open()
	if err != nil {
		return nil, "", err
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, model.ContractImportMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > model.ContractImportMaxBytes {
		return nil, "", fmt.Errorf("파일은 2MB 이하여야 합니다")
	}
	return raw, fh.Filename, nil
}

func (h *ProjectHandler) contractImportScope(c echo.Context) (repository.CatImportScope, error) {
	return h.catImportScope(c)
}

func (h *ProjectHandler) ContractImportPreview(c echo.Context) error {
	if !canWriteContractImport(c) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readContractUpload(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/import?err="+url.QueryEscape(err.Error()))
	}
	scope, err := h.contractImportScope(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/import?err="+url.QueryEscape(err.Error()))
	}
	prev, err := h.repo.PreviewContractImport(raw, name, scope, time.Now())
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/import?err="+url.QueryEscape(err.Error()))
	}
	return h.renderContractImport(c, prev, false)
}

func (h *ProjectHandler) ContractImportApply(c echo.Context) error {
	if !canWriteContractImport(c) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readContractUpload(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/import?err="+url.QueryEscape(err.Error()))
	}
	scope, err := h.contractImportScope(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/import?err="+url.QueryEscape(err.Error()))
	}
	prev, err := h.repo.ApplyContractImport(currentUserID(c), name, raw, scope, time.Now())
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/import?err="+url.QueryEscape(err.Error()))
	}
	return h.renderContractImport(c, prev, true)
}

func (h *ProjectHandler) ContractImportErrors(c echo.Context) error {
	if !canWriteContractImport(c) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readContractUpload(c)
	if err != nil {
		return err
	}
	scope, err := h.contractImportScope(c)
	if err != nil {
		return err
	}
	prev, err := h.repo.PreviewContractImport(raw, name, scope, time.Now())
	if err != nil {
		return err
	}
	buf, err := repository.BuildContractErrorWorkbook(prev)
	if err != nil {
		return err
	}
	return writeExcelFile(c, buf, "contract_import_errors.xlsx", "사업등록_오류목록.xlsx")
}

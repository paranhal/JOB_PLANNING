package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	return h.renderContractImport(c, nil, "", false, false)
}

func (h *ProjectHandler) renderContractImport(c echo.Context, prev *model.ContractImportPreview, token string, applied, needPipe bool) error {
	return c.Render(http.StatusOK, "project/import.html", map[string]interface{}{
		"Title":               "사업 엑셀 등록",
		"Active":              NavProjectsImport,
		"CanWrite":            canWriteContractImport(c),
		"CanDownload":         canDownloadContractImport(c),
		"Preview":             prev,
		"PreviewToken":        token,
		"Apply":               applied,
		"NeedPipelineConfirm": needPipe,
		"FlashErr":            c.QueryParam("err"),
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

func (h *ProjectHandler) contractUpload(c echo.Context, consumeApply bool) ([]byte, string, error) {
	tok := strings.TrimSpace(c.FormValue("preview_token"))
	if tok != "" {
		raw, name, err := h.importHold.Get(currentUserID(c), currentOrg(c), tok, consumeApply)
		if err != nil {
			return nil, "", err
		}
		return raw, name, nil
	}
	return h.readContractUpload(c)
}

func (h *ProjectHandler) rememberContractUpload(c echo.Context, raw []byte, name string) string {
	if h.importHold == nil {
		h.importHold = newContractImportHold()
	}
	return h.importHold.Put(currentUserID(c), currentOrg(c), name, raw)
}

func (h *ProjectHandler) holdErrStatus(err error) error {
	if errors.Is(err, errImportHoldForbidden) || errors.Is(err, errImportHoldUsed) {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}
	return nil
}

func (h *ProjectHandler) contractImportScope(c echo.Context) (repository.CatImportScope, error) {
	return h.catImportScope(c)
}

func (h *ProjectHandler) ContractImportPreview(c echo.Context) error {
	if !canWriteContractImport(c) {
		return echo.ErrForbidden
	}
	raw, name, err := h.contractUpload(c, false)
	if err != nil {
		if he := h.holdErrStatus(err); he != nil {
			return he
		}
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
	tok := h.rememberContractUpload(c, raw, name)
	return h.renderContractImport(c, prev, tok, false, false)
}

func parseContractImportApplyOpt(c echo.Context) model.ContractImportApplyOpt {
	opt := model.ContractImportApplyOpt{
		Pipeline: strings.TrimSpace(c.FormValue("pipeline")),
		Stage:    strings.TrimSpace(c.FormValue("pipeline_stage")),
	}
	if strings.TrimSpace(c.FormValue("pipeline_lines_present")) == "1" {
		opt.Lines = map[int]bool{}
		_ = c.Request().ParseForm()
		for _, v := range c.Request().Form["pipeline_line"] {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err == nil && n > 0 {
				opt.Lines[n] = true
			}
		}
	}
	return opt
}

func (h *ProjectHandler) ContractImportApply(c echo.Context) error {
	if !canWriteContractImport(c) {
		return echo.ErrForbidden
	}
	opt := parseContractImportApplyOpt(c)
	consume := opt.Pipeline == "yes" || opt.Pipeline == "no"
	raw, name, err := h.contractUpload(c, consume)
	if err != nil {
		if he := h.holdErrStatus(err); he != nil {
			return he
		}
		return c.Redirect(http.StatusSeeOther, "/projects/import?err="+url.QueryEscape(err.Error()))
	}
	scope, err := h.contractImportScope(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/import?err="+url.QueryEscape(err.Error()))
	}
	tok := strings.TrimSpace(c.FormValue("preview_token"))
	if tok == "" {
		tok = h.rememberContractUpload(c, raw, name)
	}
	prev, err := h.repo.ApplyContractImport(currentUserID(c), name, raw, scope, time.Now(), opt)
	if errors.Is(err, repository.ErrPipelineNeedsConfirm) {
		return h.renderContractImport(c, prev, tok, false, true)
	}
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/import?err="+url.QueryEscape(err.Error()))
	}
	return h.renderContractImport(c, prev, tok, true, false)
}

func (h *ProjectHandler) ContractImportErrors(c echo.Context) error {
	if !canWriteContractImport(c) {
		return echo.ErrForbidden
	}
	raw, name, err := h.contractUpload(c, false)
	if err != nil {
		if he := h.holdErrStatus(err); he != nil {
			return he
		}
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

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

func (h *CustomerHandler) OrgImport(c echo.Context) error {
	if !canViewMaster(c) {
		return echo.ErrForbidden
	}
	return h.renderOrgImport(c, nil, false, "")
}

func (h *CustomerHandler) renderOrgImport(c echo.Context, prev *model.SchoolImportPreview, applied bool, errMsg string) error {
	return c.Render(http.StatusOK, "customer/org_import.html", map[string]interface{}{
		"Title":       "하위 기관 일괄 등록",
		"Active":      NavCustomers,
		"CanWrite":    canWriteMaster(c) && currentRole(c) != model.RoleCLevel,
		"CanDownload": canViewMaster(c),
		"Preview":     prev,
		"Apply":       applied,
		"FlashErr":    errMsg,
		"ParentHint":  model.SchoolParentOfficialName,
		"WorkOrg":     currentOrg(c),
	})
}

func (h *CustomerHandler) OrgImportTemplateXLSX(c echo.Context) error {
	if !canViewMaster(c) {
		return echo.ErrForbidden
	}
	raw, err := repository.BuildSchoolMasterWorkbook()
	if err != nil {
		return err
	}
	return writeExcelFile(c, raw, "orgs_template.xlsx", "기관목록_업로드양식.xlsx")
}

func (h *CustomerHandler) readSchoolUpload(c echo.Context) ([]byte, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, err
	}
	src, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, model.SchoolImportMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > model.SchoolImportMaxBytes {
		return nil, fmt.Errorf("파일은 10MB 이하여야 합니다")
	}
	return raw, nil
}

func (h *CustomerHandler) schoolImportOrg(c echo.Context) (string, error) {
	org := currentOrg(c)
	if currentRole(c) == model.RoleVisionAdmin {
		if v := strings.TrimSpace(c.FormValue("org_id")); v != "" {
			org = v
		}
	}
	if org == "" || org == repository.OrgAll {
		return "", fmt.Errorf("조직을 선택하세요")
	}
	return org, nil
}

func (h *CustomerHandler) OrgImportPreview(c echo.Context) error {
	if !canWriteMaster(c) || currentRole(c) == model.RoleCLevel {
		return echo.ErrForbidden
	}
	raw, err := h.readSchoolUpload(c)
	if err != nil {
		return h.renderOrgImport(c, nil, false, err.Error())
	}
	org, err := h.schoolImportOrg(c)
	if err != nil {
		return h.renderOrgImport(c, nil, false, err.Error())
	}
	create := c.FormValue("create_parent") == "1"
	prev, err := h.repo.PreviewSchoolMaster(raw, org, create)
	if err != nil {
		return h.renderOrgImport(c, nil, false, err.Error())
	}
	return h.renderOrgImport(c, prev, false, "")
}

func (h *CustomerHandler) OrgImportApply(c echo.Context) error {
	if !canWriteMaster(c) || currentRole(c) == model.RoleCLevel {
		return echo.ErrForbidden
	}
	raw, err := h.readSchoolUpload(c)
	if err != nil {
		return h.renderOrgImport(c, nil, false, err.Error())
	}
	org, err := h.schoolImportOrg(c)
	if err != nil {
		return h.renderOrgImport(c, nil, false, err.Error())
	}
	create := c.FormValue("create_parent") == "1"
	prev, err := h.repo.ApplySchoolMaster(currentUserID(c), org, raw, create)
	if err != nil {
		return h.renderOrgImport(c, nil, false, err.Error())
	}
	return h.renderOrgImport(c, prev, true, "")
}

package handler

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *AssetHandler) ImportForm(c echo.Context) error {
	if !canViewMaster(c) {
		return echo.ErrForbidden
	}
	return h.renderAssetImport(c, nil, "", false)
}

func (h *AssetHandler) renderAssetImport(c echo.Context, prev *model.AssetImportPreview, token string, applied bool) error {
	return c.Render(http.StatusOK, "asset/import.html", map[string]interface{}{
		"Title":        "설치자산 엑셀 업로드",
		"Active":       NavAssets,
		"CanWrite":     canWriteMaster(c),
		"Preview":      prev,
		"PreviewToken": token,
		"Apply":        applied,
		"FlashErr":     c.QueryParam("err"),
	})
}

func (h *AssetHandler) ImportTemplateXLSX(c echo.Context) error {
	if !canViewMaster(c) {
		return echo.ErrForbidden
	}
	raw, err := repository.BuildAssetImportTemplate()
	if err != nil {
		return err
	}
	return writeExcelFile(c, raw, "asset_template.xlsx", "설치자산_업로드양식.xlsx")
}

func (h *AssetHandler) readAssetUpload(c echo.Context) ([]byte, string, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, "", err
	}
	src, err := fh.Open()
	if err != nil {
		return nil, "", err
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, model.AssetImportMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > model.AssetImportMaxBytes {
		return nil, "", fmt.Errorf("파일은 2MB 이하여야 합니다")
	}
	return raw, fh.Filename, nil
}

func (h *AssetHandler) holdAssetUpload(c echo.Context) *contractImportHold {
	if h.importHold == nil {
		h.importHold = newContractImportHold()
	}
	return h.importHold
}

func (h *AssetHandler) ImportPreview(c echo.Context) error {
	if !canWriteMaster(c) {
		return echo.ErrForbidden
	}
	raw, name, err := h.readAssetUpload(c)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/assets/import?err="+url.QueryEscape(err.Error()))
	}
	create := c.FormValue("create_spaces") == "1"
	prev, err := h.repo.PreviewAssetImport(h.customerRepo, currentOrg(c), name, raw, create)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/assets/import?err="+url.QueryEscape(err.Error()))
	}
	tok := h.holdAssetUpload(c).Put(currentUserID(c), currentOrg(c), name, raw)
	return h.renderAssetImport(c, prev, tok, false)
}

func (h *AssetHandler) ImportApply(c echo.Context) error {
	if !canWriteMaster(c) {
		return echo.ErrForbidden
	}
	tok := strings.TrimSpace(c.FormValue("preview_token"))
	raw, name, err := h.holdAssetUpload(c).Get(currentUserID(c), currentOrg(c), tok, true)
	if err != nil {
		if he := h.holdErr(err); he != nil {
			return he
		}
		return c.Redirect(http.StatusSeeOther, "/assets/import?err="+url.QueryEscape(err.Error()))
	}
	create := c.FormValue("create_spaces") == "1"
	prev, err := h.applyAssetImport(currentOrg(c), name, raw, create)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/assets/import?err="+url.QueryEscape(err.Error()))
	}
	if csvBody, csvErr := assetImportCSV(prev); csvErr == nil {
		if h.resultCSV == nil {
			h.resultCSV = map[string][]byte{}
		}
		h.resultCSV[tok] = csvBody
	}
	return h.renderAssetImport(c, prev, tok, true)
}

func (h *AssetHandler) holdErr(err error) error {
	if err == errImportHoldForbidden || err == errImportHoldUsed {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}
	return nil
}

func (h *AssetHandler) applyAssetImport(orgID, name string, raw []byte, createSpaces bool) (*model.AssetImportPreview, error) {
	prev, err := h.repo.PreviewAssetImport(h.customerRepo, orgID, name, raw, createSpaces)
	if err != nil {
		return nil, err
	}
	for i := range prev.Rows {
		row := &prev.Rows[i]
		if row.Action != model.AssetImportRegister {
			continue
		}
		a := row.Asset
		h.applyGroup(&a)
		applyAssetBusinessRules(&a)
		if createSpaces && h.spaceRepo != nil {
			_ = h.spaceRepo.EnsureNamedSpace(a.CustomerID, a.LocBuildingName, a.LocFloorName, a.LocRoomName)
		}
		if err := h.repo.Create(&a); err != nil {
			row.Action = model.AssetImportError
			row.Msgs = append(row.Msgs, "저장에 실패했습니다")
			continue
		}
		row.Asset = a
	}
	model.TallyAssetImport(prev)
	return prev, nil
}

func (h *AssetHandler) ImportResultCSV(c echo.Context) error {
	if !canWriteMaster(c) {
		return echo.ErrForbidden
	}
	tok := strings.TrimSpace(c.QueryParam("token"))
	if body, ok := h.resultCSV[tok]; ok {
		return writeDownloadBytes(c, body, "text/csv; charset=utf-8", "asset_import_result.csv", "설치자산_업로드_결과.csv")
	}
	raw, name, err := h.holdAssetUpload(c).Get(currentUserID(c), currentOrg(c), tok, false)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	create := c.QueryParam("create_spaces") == "1"
	prev, err := h.repo.PreviewAssetImport(h.customerRepo, currentOrg(c), name, raw, create)
	if err != nil {
		return err
	}
	body, err := assetImportCSV(prev)
	if err != nil {
		return err
	}
	return writeDownloadBytes(c, body, "text/csv; charset=utf-8", "asset_import_result.csv", "설치자산_업로드_결과.csv")
}

func assetImportCSV(prev *model.AssetImportPreview) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"행", "처리", "기관명", "제품명", "시리얼", "설치위치", "건물명", "층", "호실", "관리유형", "설치주체", "제품구분", "메시지"})
	if prev != nil {
		for _, r := range prev.Rows {
			_ = w.Write([]string{
				fmt.Sprintf("%d", r.Line), r.Action, r.SiteName, r.Asset.ProductName, r.Asset.SerialNumber,
				r.Asset.InstallLocation, r.Asset.LocBuildingName, r.Asset.LocFloorName, r.Asset.LocRoomName,
				r.Asset.ManagementType, r.Asset.InstallerType, r.Asset.ProductType, strings.Join(r.Msgs, "; "),
			})
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}


package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type SWDetailHandler struct {
	repo *repository.SWDetailRepo
}

func (h *SWDetailHandler) ListByAsset(c echo.Context) error {
	assetID := c.Param("asset_id")
	items, _ := h.repo.ListByAsset(assetID)
	return c.Render(http.StatusOK, "sw_detail/list.html", map[string]interface{}{
		"Title": "SW 상세 관리", "Active": "assets",
		"Items": items, "AssetID": assetID,
	})
}

func (h *SWDetailHandler) Create(c echo.Context) error {
	s := bindSWDetail(c)
	h.repo.Create(s)
	return c.Redirect(http.StatusSeeOther, "/assets/"+s.AssetID+"/sw")
}

func (h *SWDetailHandler) Edit(c echo.Context) error {
	s, err := h.repo.GetByID(c.Param("id"))
	if err != nil || s == nil {
		return echo.ErrNotFound
	}
	return c.Render(http.StatusOK, "sw_detail/form.html", map[string]interface{}{
		"Title": "SW 상세 수정", "Active": "assets",
		"SW": s, "IsNew": false,
	})
}

func (h *SWDetailHandler) Update(c echo.Context) error {
	s := bindSWDetail(c)
	s.SWDetailID = c.Param("id")
	h.repo.Update(s)
	return c.Redirect(http.StatusSeeOther, "/assets/"+s.AssetID+"/sw")
}

func (h *SWDetailHandler) Delete(c echo.Context) error {
	s, _ := h.repo.GetByID(c.Param("id"))
	assetID := ""
	if s != nil {
		assetID = s.AssetID
	}
	h.repo.Delete(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, "/assets/"+assetID+"/sw")
}

func bindSWDetail(c echo.Context) *model.AssetSWDetail {
	return &model.AssetSWDetail{
		AssetID:      c.FormValue("asset_id"),
		SoftwareName: c.FormValue("software_name"),
		Version:      c.FormValue("version"),
		InstallType:  c.FormValue("install_type"),
		HWInfo:       c.FormValue("hw_info"),
		OS:           c.FormValue("os"),
		OSVersion:    c.FormValue("os_version"),
		DBMS:         c.FormValue("dbms"),
		DBVersion:    c.FormValue("db_version"),
		AccessMethod: c.FormValue("access_method"),
		AccessURL:    c.FormValue("access_url"),
		InstallPath:  c.FormValue("install_path"),
		BackupPath:   c.FormValue("backup_path"),
		ConfigPath:   c.FormValue("config_path"),
	}
}

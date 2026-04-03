package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type AssetHandler struct {
	repo         *repository.AssetRepo
	customerRepo *repository.CustomerRepo
	codeRepo     *repository.CodeRepo
}

func (h *AssetHandler) List(c echo.Context) error {
	search := c.QueryParam("search")
	customerID := c.QueryParam("customer_id")
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	items, total, err := h.repo.List(customerID, search, page, 20)
	if err != nil {
		return err
	}
	totalPages := (total + 19) / 20
	customers, _ := h.customerRepo.ListAll()

	return c.Render(http.StatusOK, "asset/list.html", map[string]interface{}{
		"Title": "설치자산 관리", "Active": "assets",
		"Items": items, "Total": total,
		"Page": page, "TotalPages": totalPages,
		"Search": search, "CustomerID": customerID,
		"Customers": customers,
	})
}

func (h *AssetHandler) New(c echo.Context) error {
	customers, _ := h.customerRepo.ListAll()
	productTypes, _ := h.codeRepo.ActiveByGroup("product_type")
	installerTypes, _ := h.codeRepo.ActiveByGroup("installer_type")
	managementTypes, _ := h.codeRepo.ActiveByGroup("management_type")
	requesterTypes, _ := h.codeRepo.ActiveByGroup("requester_type")
	opStatuses, _ := h.codeRepo.ActiveByGroup("operation_status")

	asset := &model.Asset{IsManaged: true, OperationStatus: "operating"}
	if cid := c.QueryParam("customer_id"); cid != "" {
		asset.CustomerID = cid
	}

	return c.Render(http.StatusOK, "asset/form.html", map[string]interface{}{
		"Title": "자산 등록", "Active": "assets", "IsNew": true,
		"Asset": asset, "Customers": customers,
		"ProductTypes": productTypes, "InstallerTypes": installerTypes,
		"ManagementTypes": managementTypes, "RequesterTypes": requesterTypes,
		"OpStatuses": opStatuses,
	})
}

func (h *AssetHandler) Create(c echo.Context) error {
	a := bindAsset(c)
	if err := h.repo.Create(a); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/assets/"+a.AssetID)
}

func (h *AssetHandler) Show(c echo.Context) error {
	id := c.Param("id")
	a, err := h.repo.GetByID(id)
	if err != nil {
		return err
	}
	if a == nil {
		return echo.ErrNotFound
	}
	return c.Render(http.StatusOK, "asset/show.html", map[string]interface{}{
		"Title": a.ProductName, "Active": "assets", "Asset": a,
	})
}

func (h *AssetHandler) Edit(c echo.Context) error {
	id := c.Param("id")
	a, err := h.repo.GetByID(id)
	if err != nil || a == nil {
		return echo.ErrNotFound
	}
	customers, _ := h.customerRepo.ListAll()
	productTypes, _ := h.codeRepo.ActiveByGroup("product_type")
	installerTypes, _ := h.codeRepo.ActiveByGroup("installer_type")
	managementTypes, _ := h.codeRepo.ActiveByGroup("management_type")
	requesterTypes, _ := h.codeRepo.ActiveByGroup("requester_type")
	opStatuses, _ := h.codeRepo.ActiveByGroup("operation_status")

	return c.Render(http.StatusOK, "asset/form.html", map[string]interface{}{
		"Title": "자산 수정", "Active": "assets", "IsNew": false,
		"Asset": a, "Customers": customers,
		"ProductTypes": productTypes, "InstallerTypes": installerTypes,
		"ManagementTypes": managementTypes, "RequesterTypes": requesterTypes,
		"OpStatuses": opStatuses,
	})
}

func (h *AssetHandler) Update(c echo.Context) error {
	a := bindAsset(c)
	a.AssetID = c.Param("id")
	if err := h.repo.Update(a); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/assets/"+a.AssetID)
}

func (h *AssetHandler) Delete(c echo.Context) error {
	h.repo.Delete(c.Param("id"))
	return c.Redirect(http.StatusSeeOther, "/assets")
}

// APIAssetsByCustomer HTMX용: 고객별 자산 목록 JSON
func (h *AssetHandler) APIAssetsByCustomer(c echo.Context) error {
	items, _ := h.repo.ListByCustomer(c.Param("customer_id"))
	return c.JSON(http.StatusOK, items)
}

func bindAsset(c echo.Context) *model.Asset {
	return &model.Asset{
		CustomerID:        c.FormValue("customer_id"),
		ProductName:       c.FormValue("product_name"),
		ProductType:       c.FormValue("product_type"),
		ModelName:         c.FormValue("model_name"),
		Manufacturer:      c.FormValue("manufacturer"),
		SerialNumber:      c.FormValue("serial_number"),
		InstallDate:       c.FormValue("install_date"),
		RetireDate:        c.FormValue("retire_date"),
		InstallerType:     c.FormValue("installer_type"),
		OriginalInstaller: c.FormValue("original_installer"),
		OperationStatus:   c.FormValue("operation_status"),
		ManagementType:    c.FormValue("management_type"),
		IsManaged:         c.FormValue("is_managed") != "0",
		RequesterType:     c.FormValue("requester_type"),
		RequesterName:     c.FormValue("requester_name"),
		CustomerContactID: c.FormValue("customer_contact_id"),
		OurContact:        c.FormValue("our_contact"),
		BuildingID:        c.FormValue("building_id"),
		FloorID:           c.FormValue("floor_id"),
		RoomID:            c.FormValue("room_id"),
		LocationDetail:    c.FormValue("location_detail"),
		Notes:             c.FormValue("notes"),
	}
}

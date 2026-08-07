package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type AssetHandler struct {
	repo         *repository.AssetRepo
	customerRepo *repository.CustomerRepo
	codeRepo     *repository.CodeRepo
	attachRepo   *repository.AttachmentRepo
	wbRepo       *repository.WBRepo
}

func (h *AssetHandler) List(c echo.Context) error {
	search := c.QueryParam("search")
	customerID := c.QueryParam("customer_id")
	projectID := c.QueryParam("project_id")
	category := c.QueryParam("category")
	sort := c.QueryParam("sort")
	dir := c.QueryParam("dir")
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	items, total, err := h.repo.List(customerID, search, projectID, category, sort, dir, page, 20)
	if err != nil {
		return err
	}
	totalPages := (total + 19) / 20
	customers, _ := h.customerRepo.ListAll()
	projects, _ := h.wbRepo.ListProjects(false)
	productCategories, _ := h.codeRepo.ActiveByGroup("stats_product_category")

	nextDir := func(col string) string {
		if sort == col && dir == "asc" {
			return "desc"
		}
		return "asc"
	}

	return c.Render(http.StatusOK, "asset/list.html", map[string]interface{}{
		"Title": "설치자산 관리", "Active": "assets",
		"Items": items, "Total": total,
		"Page": page, "TotalPages": totalPages,
		"Search": search, "CustomerID": customerID,
		"ProjectID": projectID, "Category": category,
		"Customers": customers, "Projects": projects,
		"ProductCategories": productCategories,
		"Sort": sort, "Dir": dir,
		"SortDirAsset": nextDir("asset_id"),
		"SortDirOrg":   nextDir("org_name"),
		"SortDirName":  nextDir("product_name"),
		"SortDirCat":   nextDir("product_category"),
		"SortDirType":  nextDir("product_type"),
		"SortDirSN":    nextDir("serial_number"),
		"SortDirLoc":   nextDir("location"),
		"SortDirInst":  nextDir("install_location"),
		"SortDirMaint": nextDir("maint_contract"),
		"SortDirStat":  nextDir("status"),
		"SortDirYears": nextDir("install_years"),
		"SortDirAS":    nextDir("as_count"),
		"SortDirProj":  nextDir("project"),
		"CanWrite":     canWriteMaster(c),
	})
}

func (h *AssetHandler) New(c echo.Context) error {
	customers, _ := h.customerRepo.ListAll()
	productTypes, _ := h.codeRepo.ActiveByGroup("product_type")
	productCategories, _ := h.codeRepo.ActiveByGroup("stats_product_category")
	installerTypes, _ := h.codeRepo.ActiveByGroup("installer_type")
	managementTypes, _ := h.codeRepo.ActiveByGroup("management_type")
	requesterTypes, _ := h.codeRepo.ActiveByGroup("requester_type")
	opStatuses, _ := h.codeRepo.ActiveByGroup("operation_status")
	maintContractTypes, _ := h.codeRepo.ActiveByGroup("maint_contract_type")
	maintCycles, _ := h.codeRepo.ActiveByGroup("maint_cycle")
	maintBillingCycles, _ := h.codeRepo.ActiveByGroup("maint_billing_cycle")

	asset := &model.Asset{
		IsManaged: true, OperationStatus: "operating",
		ProductCategory: "rfid", ProjectID: repository.ProjectIDAnroboticsRFID,
	}
	if cid := c.QueryParam("customer_id"); cid != "" {
		asset.CustomerID = cid
	}
	projects, _ := h.wbRepo.ListProjects(false)

	return c.Render(http.StatusOK, "asset/form.html", map[string]interface{}{
		"Title": "자산 등록", "Active": "assets", "IsNew": true,
		"Asset": asset, "Customers": customers, "Projects": projects,
		"ProductTypes": productTypes, "ProductCategories": productCategories,
		"InstallerTypes": installerTypes,
		"ManagementTypes": managementTypes, "RequesterTypes": requesterTypes,
		"OpStatuses": opStatuses,
		"MaintContractTypes": maintContractTypes, "MaintCycles": maintCycles,
		"MaintBillingCycles": maintBillingCycles,
	})
}

func (h *AssetHandler) Create(c echo.Context) error {
	a := bindAsset(c)
	// 타사 장비는 RFID자동화가 아닌 경우가 많아, 미선택 시 기타로 둔다.
	if a.ProductCategory == "" {
		a.ProductCategory = "other"
	}
	applyRFIDProjectDefault(a)
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
	slots, filled := buildAssetImageSlots(h.attachRepo, id)
	return c.Render(http.StatusOK, "asset/show.html", map[string]interface{}{
		"Title": a.ProductName, "Active": "assets", "Asset": a,
		"Images": slots, "ImageCount": filled, "CanUpload": filled < 3,
		"CanWrite": canWriteMaster(c), "CanReceive": canReceiveAS(c),
	})
}

func buildAssetImageSlots(repo *repository.AttachmentRepo, assetID string) ([]model.AssetImageSlot, int) {
	atts, _ := repo.ListByRef("asset", assetID)
	bySlot := map[int]model.Attachment{}
	unslotted := []model.Attachment{}
	for _, att := range atts {
		if att.SlotNo >= 1 && att.SlotNo <= 3 {
			bySlot[att.SlotNo] = att
		} else {
			unslotted = append(unslotted, att)
		}
	}
	// 슬롯 없는 구 데이터는 빈 슬롯에 순서대로 배치(표시용)
	next := 1
	for _, att := range unslotted {
		for next <= 3 {
			if _, ok := bySlot[next]; !ok {
				bySlot[next] = att
				next++
				break
			}
			next++
		}
	}
	slots := make([]model.AssetImageSlot, 0, 3)
	filled := 0
	for i := 1; i <= 3; i++ {
		s := model.AssetImageSlot{Slot: i}
		if att, ok := bySlot[i]; ok {
			filled++
			s.ID = att.AttachmentID
			s.Name = att.FileName
			s.URL = attachmentPublicURL(att.FilePath)
			s.Keywords = att.Keywords
		}
		slots = append(slots, s)
	}
	return slots, filled
}

func attachmentPublicURL(filePath string) string {
	p := strings.ReplaceAll(filePath, "\\", "/")
	const prefix = "data/uploads/"
	if i := strings.Index(p, prefix); i >= 0 {
		return "/uploads/" + p[i+len(prefix):]
	}
	if strings.HasPrefix(p, "uploads/") {
		return "/" + p
	}
	return ""
}

func (h *AssetHandler) Edit(c echo.Context) error {
	id := c.Param("id")
	a, err := h.repo.GetByID(id)
	if err != nil || a == nil {
		return echo.ErrNotFound
	}
	customers, _ := h.customerRepo.ListAll()
	productTypes, _ := h.codeRepo.ActiveByGroup("product_type")
	productCategories, _ := h.codeRepo.ActiveByGroup("stats_product_category")
	installerTypes, _ := h.codeRepo.ActiveByGroup("installer_type")
	managementTypes, _ := h.codeRepo.ActiveByGroup("management_type")
	requesterTypes, _ := h.codeRepo.ActiveByGroup("requester_type")
	opStatuses, _ := h.codeRepo.ActiveByGroup("operation_status")
	maintContractTypes, _ := h.codeRepo.ActiveByGroup("maint_contract_type")
	maintCycles, _ := h.codeRepo.ActiveByGroup("maint_cycle")
	maintBillingCycles, _ := h.codeRepo.ActiveByGroup("maint_billing_cycle")
	projects, _ := h.wbRepo.ListProjects(false)
	slots, filled := buildAssetImageSlots(h.attachRepo, id)

	return c.Render(http.StatusOK, "asset/form.html", map[string]interface{}{
		"Title": "자산 수정", "Active": "assets", "IsNew": false,
		"Asset": a, "Customers": customers, "Projects": projects,
		"ProductTypes": productTypes, "ProductCategories": productCategories,
		"InstallerTypes": installerTypes,
		"ManagementTypes": managementTypes, "RequesterTypes": requesterTypes,
		"OpStatuses": opStatuses,
		"MaintContractTypes": maintContractTypes, "MaintCycles": maintCycles,
		"MaintBillingCycles": maintBillingCycles,
		"Images": slots, "ImageCount": filled, "CanUpload": filled < 3,
		"CanWrite": canWriteMaster(c),
	})
}

func (h *AssetHandler) Update(c echo.Context) error {
	a := bindAsset(c)
	a.AssetID = c.Param("id")
	applyRFIDProjectDefault(a)
	if err := h.repo.Update(a); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/assets/"+a.AssetID)
}

// applyRFIDProjectDefault RFID자동화 분류이고 사업 미선택이면 앤로보틱스 RFID 사업으로 둔다.
func applyRFIDProjectDefault(a *model.Asset) {
	if a == nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(a.ProductCategory), "rfid") && strings.TrimSpace(a.ProjectID) == "" {
		a.ProjectID = repository.ProjectIDAnroboticsRFID
	}
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
	billingCycle := c.FormValue("maint_billing_cycle_code")
	if billingCycle == "custom" {
		billingCycle = c.FormValue("maint_billing_cycle_custom")
	}
	maintCycle := c.FormValue("maint_cycle_code")
	if maintCycle == "custom" {
		maintCycle = c.FormValue("maint_cycle_custom")
	}

	productType := strings.TrimSpace(c.FormValue("product_type"))
	if productType != "" {
		productType = strings.ToUpper(productType)
	}
	return &model.Asset{
		CustomerID:        c.FormValue("customer_id"),
		ProductName:       c.FormValue("product_name"),
		ProductType:       productType,
		ProductCategory:   c.FormValue("product_category"),
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
		MaintContractType: c.FormValue("maint_contract_type"),
		MaintCycle:        maintCycle,
		MaintStartDate:    c.FormValue("maint_start_date"),
		MaintEndDate:      c.FormValue("maint_end_date"),
		MaintBillingParty: c.FormValue("maint_billing_party"),
		MaintBillingCycle: billingCycle,
		RequesterType:     c.FormValue("requester_type"),
		RequesterName:     c.FormValue("requester_name"),
		CustomerContactID: c.FormValue("customer_contact_id"),
		OurContact:        c.FormValue("our_contact"),
		BuildingID:        c.FormValue("building_id"),
		FloorID:           c.FormValue("floor_id"),
		RoomID:            c.FormValue("room_id"),
		LocBuildingName:   c.FormValue("loc_building_name"),
		LocFloorName:      c.FormValue("loc_floor_name"),
		LocRoomName:       c.FormValue("loc_room_name"),
		InstallLocation:   c.FormValue("install_location"),
		LocationDetail:    c.FormValue("location_detail"),
		Notes:             c.FormValue("notes"),
		ProjectID:         strings.TrimSpace(c.FormValue("project_id")),
	}
}

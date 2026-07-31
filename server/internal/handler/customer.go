package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type CustomerHandler struct {
	repo        *repository.CustomerRepo
	assetRepo   *repository.AssetRepo
	contactRepo *repository.ContactRepo
	asRepo      *repository.ASRepo
}

// TabAssets HTMX: 고객별 설치자산 탭 부분 렌더링
func (h *CustomerHandler) TabAssets(c echo.Context) error {
	customerID := c.Param("id")
	assets, err := h.assetRepo.ListForTab(customerID)
	if err != nil {
		return err
	}
	return RenderPartial(c, "customer/tab_assets.html", map[string]interface{}{
		"Assets":     assets,
		"CustomerID": customerID,
		"CanWrite":   canWriteMaster(c),
	})
}

// TabContacts HTMX: 고객별 담당자 탭 부분 렌더링
func (h *CustomerHandler) TabContacts(c echo.Context) error {
	customerID := c.Param("id")
	contacts, err := h.contactRepo.ListByCustomer(customerID)
	if err != nil {
		return err
	}
	return RenderPartial(c, "customer/tab_contacts.html", map[string]interface{}{
		"Contacts":   contacts,
		"CustomerID": customerID,
		"CanWrite":   canWriteMaster(c),
	})
}

// TabAS HTMX: 고객별 AS 이력 탭 부분 렌더링
func (h *CustomerHandler) TabAS(c echo.Context) error {
	customerID := c.Param("id")
	items, err := h.asRepo.ListByCustomer(customerID)
	if err != nil {
		return err
	}
	return RenderPartial(c, "customer/tab_as.html", map[string]interface{}{
		"Items":      items,
		"CustomerID": customerID,
	})
}

// List 고객 목록
func (h *CustomerHandler) List(c echo.Context) error {
	search := c.QueryParam("search")
	category := c.QueryParam("category")
	industry := c.QueryParam("industry")
	sort := c.QueryParam("sort")
	dir := c.QueryParam("dir")
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	pageSize := 20

	items, total, err := h.repo.List(search, category, industry, sort, dir, page, pageSize)
	if err != nil {
		return err
	}

	cats, noneCount, err := h.repo.ListCategories()
	if err != nil {
		return err
	}
	industries, err := h.repo.ListIndustries()
	if err != nil {
		return err
	}
	exportRegions, _ := h.repo.ListExportRegions()
	exportSites, _ := h.repo.ListAll()

	totalPages := (total + pageSize - 1) / pageSize

	// 컬럼 정렬 토글 링크용
	nextDir := func(col string) string {
		if sort == col && dir == "asc" {
			return "desc"
		}
		return "asc"
	}

	return c.Render(http.StatusOK, "customer/list.html", map[string]interface{}{
		"Title":         "고객현황",
		"Active":        "customers",
		"Items":         items,
		"Total":         total,
		"Page":          page,
		"PageSize":      pageSize,
		"TotalPages":    totalPages,
		"Search":        search,
		"Category":      category,
		"Categories":    cats,
		"NoneCount":     noneCount,
		"Industry":      industry,
		"Industries":    industries,
		"Sort":          sort,
		"Dir":           dir,
		"SortDirOrg":    nextDir("org_name"),
		"SortDirInd":    nextDir("industry"),
		"SortDirAst":    nextDir("assets"),
		"SortDirAS":     nextDir("as"),
		"CanWrite":      canWriteMaster(c),
		"ExportRegions": exportRegions,
		"ExportSites":   exportSites,
		"ExportRegion":  "",
		"ExportSite":    "",
	})
}

// ExportExcel 고객현황 엑셀 (지역·사이트명 콤보 + 목록 필터 반영)
func (h *CustomerHandler) ExportExcel(c echo.Context) error {
	search := c.QueryParam("search")
	category := c.QueryParam("category")
	industry := c.QueryParam("industry")
	sort := c.QueryParam("sort")
	dir := c.QueryParam("dir")
	region := strings.TrimSpace(c.QueryParam("region"))
	siteID := strings.TrimSpace(c.QueryParam("site"))
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}
	items, err := h.repo.ListExport(search, category, industry, sort, dir, region, siteID)
	if err != nil {
		return err
	}
	f, err := buildCustomerExcelWorkbook(items)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	return writeExcelDownload(c, buf.Bytes(), "customers")
}

func buildCustomerExcelWorkbook(items []model.CustomerListItem) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}
	if err := f.SetSheetName(sheet, "Customers"); err != nil {
		return nil, err
	}
	sheet = "Customers"

	headers := []interface{}{
		"고객ID", "기관명", "공식명칭", "상위기관", "지역", "업종",
		"대표전화", "자산수", "AS수", "상태",
	}
	hdrStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true, Family: "맑은 고딕", Size: 11,
			Color: "FFFFFF",
		},
		Fill: excelize.Fill{
			Type: "pattern", Color: []string{"2F5496"}, Pattern: 1,
		},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	if err := f.SetSheetRow(sheet, "A1", &headers); err != nil {
		return nil, err
	}
	if err := f.SetCellStyle(sheet, "A1", "J1", hdrStyle); err != nil {
		return nil, err
	}
	if err := f.SetRowHeight(sheet, 1, 30); err != nil {
		return nil, err
	}

	for r, it := range items {
		status := "비활성"
		if it.IsActive {
			status = "활성"
		}
		region := repository.ExtractKoreaRegion(it.AddrSido)
		if region == "" {
			region = repository.ExtractKoreaRegion(it.Address)
		}
		if region == "" {
			region = repository.ExtractKoreaRegion(it.SiteRegion)
		}
		if region == "" {
			region = strings.TrimSpace(it.SiteRegion)
		}
		row := []interface{}{
			it.CustomerID, it.OrgName, it.OfficialName, it.ParentOrgName, region, it.Industry,
			it.MainPhone, it.AssetCount, it.AsCount, status,
		}
		cell := fmt.Sprintf("A%d", r+2)
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			return nil, err
		}
	}
	widths := []float64{22, 28, 28, 22, 10, 12, 14, 8, 8, 8}
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, w)
	}
	return f, nil
}

// New 고객 등록 폼
func (h *CustomerHandler) New(c echo.Context) error {
	customers, _ := h.repo.ListAll()
	return c.Render(http.StatusOK, "customer/form.html", map[string]interface{}{
		"Title":       "고객 등록",
		"Active":      "customers",
		"Customer":    &model.Customer{IsActive: true},
		"Customers":   customers,
		"IsNew":       true,
		"SidoOptions": model.KoreaSidoOptions,
	})
}

// Create 고객 등록 처리
func (h *CustomerHandler) Create(c echo.Context) error {
	cust := bindCustomer(c)
	if err := h.repo.Create(cust); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/customers")
}

// Show 고객 상세
func (h *CustomerHandler) Show(c echo.Context) error {
	id := c.Param("id")
	cust, err := h.repo.GetByID(id)
	if err != nil {
		return err
	}
	if cust == nil {
		return echo.ErrNotFound
	}
	return c.Render(http.StatusOK, "customer/show.html", map[string]interface{}{
		"Title":      cust.OrgName,
		"Active":     "customers",
		"Customer":   cust,
		"CanWrite":   canWriteMaster(c),
		"CanReceive": canReceiveAS(c),
	})
}

// Edit 고객 수정 폼
func (h *CustomerHandler) Edit(c echo.Context) error {
	id := c.Param("id")
	cust, err := h.repo.GetByID(id)
	if err != nil {
		return err
	}
	if cust == nil {
		return echo.ErrNotFound
	}
	customers, _ := h.repo.ListAll()
	return c.Render(http.StatusOK, "customer/form.html", map[string]interface{}{
		"Title":       "고객 수정",
		"Active":      "customers",
		"Customer":    cust,
		"Customers":   customers,
		"IsNew":       false,
		"SidoOptions": model.KoreaSidoOptions,
	})
}

// Update 고객 수정 처리
func (h *CustomerHandler) Update(c echo.Context) error {
	cust := bindCustomer(c)
	cust.CustomerID = c.Param("id")
	if err := h.repo.Update(cust); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/customers/"+cust.CustomerID)
}

// Delete 고객 비활성화
func (h *CustomerHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		return err
	}
	q := url.Values{}
	for _, k := range []string{"search", "category", "industry", "sort", "dir", "page"} {
		if v := strings.TrimSpace(c.FormValue(k)); v != "" {
			q.Set(k, v)
		}
	}
	redir := "/customers"
	if enc := q.Encode(); enc != "" {
		redir += "?" + enc
	}
	return c.Redirect(http.StatusSeeOther, redir)
}

// bindCustomer 폼 데이터를 Customer 구조체로 변환
func bindCustomer(c echo.Context) *model.Customer {
	return &model.Customer{
		OrgName:          c.FormValue("org_name"),
		OfficialName:     c.FormValue("official_name"),
		OrgEmail:         c.FormValue("org_email"),
		MainPhone:        c.FormValue("main_phone"),
		Website:          c.FormValue("website"),
		BusinessNumber:   c.FormValue("business_number"),
		Representative:   c.FormValue("representative"),
		Industry:         c.FormValue("industry"),
		HasParent:        c.FormValue("has_parent") == "1",
		ParentCustomerID: c.FormValue("parent_customer_id"),
		PostalCode:       strings.TrimSpace(c.FormValue("postal_code")),
		AddrSido:         strings.TrimSpace(c.FormValue("addr_sido")),
		AddrSigungu:      strings.TrimSpace(c.FormValue("addr_sigungu")),
		AddrDong:         strings.TrimSpace(c.FormValue("addr_dong")),
		AddressDetail:    strings.TrimSpace(c.FormValue("address_detail")),
		IsActive:         c.FormValue("is_active") != "0",
		Notes:            c.FormValue("notes"),
	}
}

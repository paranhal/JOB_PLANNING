package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// Search 접수·조치 본문 검색. §4.5 기준일을 적용하지 않는다. §12.11.4
func (h *ASHandler) Search(c echo.Context) error {
	q := strings.TrimSpace(c.QueryParam("q"))
	if q == "" {
		q = strings.TrimSpace(c.QueryParam("search"))
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	f := model.ASSearchFilter{
		Query:       q,
		DateFrom:    strings.TrimSpace(c.QueryParam("from")),
		DateTo:      strings.TrimSpace(c.QueryParam("to")),
		CustomerID:  strings.TrimSpace(c.QueryParam("customer_id")),
		Product:     strings.TrimSpace(c.QueryParam("product")),
		CauseType:   strings.TrimSpace(c.QueryParam("cause_type")),
		ProcessType: strings.TrimSpace(c.QueryParam("process_type")),
		Assigned:    strings.TrimSpace(c.QueryParam("assigned")),
		KeywordID:   strings.TrimSpace(c.QueryParam("keyword_id")),
		Sort:        strings.TrimSpace(c.QueryParam("sort")),
		Page:        page,
		PageSize:    20,
	}
	if f.Sort != "newest" {
		f.Sort = "relevance"
	}

	items, total, err := h.repo.SearchAS(f)
	if err != nil {
		return err
	}

	customers, _ := h.customerRepo.ListAll()
	products, _ := h.repo.SearchProductNames()
	causeTypes, _ := h.codeRepo.ActiveByGroup("cause_type")
	procTypes, _ := h.codeRepo.ActiveByGroup("process_type")
	assignees, _ := h.userRepo.ListAssignable()
	var keywords []model.ASKeyword
	var selectedKW *model.ASKeyword
	if h.kwRepo != nil {
		keywords, _ = h.kwRepo.List(true)
		if f.KeywordID != "" {
			selectedKW, _ = h.kwRepo.Get(f.KeywordID)
		}
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + 19) / 20
	}
	if page < 1 {
		page = 1
	}

	base := cloneURLValues(c.QueryParams())
	base.Del("page")

	return c.Render(http.StatusOK, "as/search.html", map[string]interface{}{
		"Title": "AS 본문 검색", "Active": NavAS,
		"Q": q, "Filter": f, "Items": items, "Total": total,
		"Page": page, "TotalPages": totalPages,
		"Customers": customers, "Products": products,
		"CauseTypes": causeTypes, "ProcTypes": procTypes, "Assignees": assignees,
		"Keywords": keywords, "SelectedKeyword": selectedKW,
		"UsesFTS":     h.repo.SearchUsesFTS(),
		"CanReceive":  canReceiveAS(c),
		"PrevURL":     asPageURL("/as/search", base, page-1),
		"NextURL":     asPageURL("/as/search", base, page+1),
		"DisplayName": ctxString(c, "user_name"),
	})
}

// Similar 접수·조치 화면의 비슷한 사례 JSON. §12.11.5
func (h *ASHandler) Similar(c echo.Context) error {
	items, err := h.repo.SimilarCases(model.ASSimilarFilter{
		Query:      strings.TrimSpace(c.QueryParam("q")),
		CustomerID: strings.TrimSpace(c.QueryParam("customer_id")),
		AssetID:    strings.TrimSpace(c.QueryParam("asset_id")),
		ExcludeID:  strings.TrimSpace(c.QueryParam("exclude")),
		Limit:      5,
	})
	if err != nil {
		return err
	}
	if items == nil {
		items = []model.ASSimilarCase{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"items": items})
}

func (h *ASHandler) loadSimilarCases(as *model.ASReceipt) []model.ASSimilarCase {
	if as == nil || strings.TrimSpace(as.Symptom) == "" {
		return nil
	}
	items, err := h.repo.SimilarCases(model.ASSimilarFilter{
		Query:      as.Symptom,
		CustomerID: as.CustomerID,
		AssetID:    as.AssetID,
		ExcludeID:  as.ASID,
		Limit:      5,
	})
	if err != nil || len(items) == 0 {
		return nil
	}
	return items
}

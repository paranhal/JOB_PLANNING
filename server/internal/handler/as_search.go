package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/service"
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

// Similar 접수·조치 화면의 비슷한 사례 JSON. §12.11.5 · §41.2
func (h *ASHandler) Similar(c echo.Context) error {
	items, err := h.repo.SimilarCases(model.ASSimilarFilter{
		Query:      h.similarQuery(strings.TrimSpace(c.QueryParam("q"))),
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
	attachSimilarLead(items)
	return c.JSON(http.StatusOK, map[string]interface{}{"items": items})
}

func (h *ASHandler) similarQuery(symptom string) string {
	q := strings.TrimSpace(symptom)
	if q == "" {
		return ""
	}
	if h.kwRepo != nil {
		if extracted := h.kwRepo.QueryFromSymptom(q); extracted != "" {
			return extracted
		}
	}
	return q
}

func attachSimilarLead(items []model.ASSimilarCase) {
	for i := range items {
		end := strings.TrimSpace(items[i].CompleteDate)
		if end == "" {
			continue
		}
		days, ok := service.LeadBusinessDays(items[i].ReceiptDate, end)
		if !ok {
			continue
		}
		items[i].LeadDays = days
		items[i].LeadLabel = fmt.Sprintf("%d영업일", days)
	}
}

func (h *ASHandler) loadSimilarCases(as *model.ASReceipt) []model.ASSimilarCase {
	if as == nil || strings.TrimSpace(as.Symptom) == "" {
		return nil
	}
	items, err := h.repo.SimilarCases(model.ASSimilarFilter{
		Query:      h.similarQuery(as.Symptom),
		CustomerID: as.CustomerID,
		AssetID:    as.AssetID,
		ExcludeID:  as.ASID,
		Limit:      5,
	})
	if err != nil || len(items) == 0 {
		return nil
	}
	attachSimilarLead(items)
	return items
}

// SimilarPanel 조치 화면의 비슷한 사례 HTML. 첫 페인트 뒤에 채운다. §39.2
func (h *ASHandler) SimilarPanel(c echo.Context) error {
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	c.Request().Header.Set("HX-Request", "true")
	return c.Render(http.StatusOK, "as/similar_panel.html", map[string]interface{}{
		"SimilarCases": h.loadSimilarCases(as),
		"CanReceive":   canReceiveAS(c),
	})
}

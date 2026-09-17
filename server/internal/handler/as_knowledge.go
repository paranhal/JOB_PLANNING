package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func knowledgeFilter(c echo.Context) model.ASSearchFilter {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	all := strings.TrimSpace(c.QueryParam("all")) == "1"
	f := model.ASSearchFilter{
		Query:         strings.TrimSpace(c.QueryParam("q")),
		DateFrom:      strings.TrimSpace(c.QueryParam("from")),
		DateTo:        strings.TrimSpace(c.QueryParam("to")),
		CustomerID:    strings.TrimSpace(c.QueryParam("customer_id")),
		Product:       strings.TrimSpace(c.QueryParam("product")),
		CauseCat:      strings.TrimSpace(c.QueryParam("cause_cat")),
		Sort:          strings.TrimSpace(c.QueryParam("sort")),
		RequireAction: !all,
		Page:          page,
		PageSize:      20,
	}
	if f.Sort != "newest" {
		f.Sort = "relevance"
	}
	return f
}

func attachKnowledgeLead(items []model.ASSearchHit) {
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

func knowledgeAvgLead(items []model.ASSearchHit) string {
	sum, n := 0, 0
	for _, it := range items {
		if strings.TrimSpace(it.LeadLabel) == "" {
			continue
		}
		sum += it.LeadDays
		n++
	}
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f영업일", float64(sum)/float64(n))
}

// Knowledge AS 사례 검색. §41.3
func (h *ASHandler) Knowledge(c echo.Context) error {
	tab := strings.TrimSpace(c.QueryParam("tab"))
	if tab != "site" {
		tab = "symptom"
	}
	f := knowledgeFilter(c)
	asRef := strings.TrimSpace(c.QueryParam("as_id"))
	withAction, totalAll, classified, err := h.repo.KnowledgeCorpus()
	if err != nil {
		return err
	}
	customers, _ := h.customerRepo.ListAll()
	products, _ := h.repo.SearchProductNames()
	cats, _ := h.repo.ListCauseCategories()
	var catL1 []model.CauseCategory
	for _, cat := range cats {
		if cat.Level == 1 {
			catL1 = append(catL1, cat)
		}
	}
	sites, _ := h.repo.KnowledgeSites()

	data := map[string]interface{}{
		"Title": "AS 사례 검색", "Active": NavASKnowledge,
		"Tab": tab, "Filter": f, "Q": f.Query, "All": !f.RequireAction,
		"WithAction": withAction, "TotalAll": totalAll, "Classified": classified,
		"Customers": customers, "Products": products, "CauseCats": catL1,
		"Sites": sites, "CanReceive": canReceiveAS(c),
		"CanProcess": canProcessAS(c),
		"DisplayName": ctxString(c, "user_name"),
		"AttachErrMsg": attachErrMessage(c.QueryParam("err")),
		"KBRedirect": c.Request().URL.RequestURI(),
	}

	if tab == "site" {
		var cases []model.ASSearchHit
		var summary *model.ASKnowledgeSummary
		var freqs []model.ASKeywordFreq
		if f.CustomerID != "" {
			cases, err = h.repo.KnowledgeSiteCases(f.CustomerID, 200)
			if err != nil {
				return err
			}
			attachKnowledgeLead(cases)
			org := f.CustomerID
			with := 0
			latest := ""
			for _, s := range sites {
				if s.CustomerID == f.CustomerID {
					org = s.OrgName
					break
				}
			}
			texts := make([]string, 0, len(cases))
			for _, it := range cases {
				if it.HasAction {
					with++
				}
				if latest == "" {
					latest = it.ReceiptDate
				}
				texts = append(texts, it.Symptom+" "+it.Action)
			}
			summary = &model.ASKnowledgeSummary{
				OrgName: org, Total: len(cases), WithAction: with,
				LatestReceipt: latest, AvgLeadLabel: knowledgeAvgLead(cases),
			}
			if h.kwRepo != nil {
				freqs = h.kwRepo.CountInTexts(texts, 8)
			}
		}
		data["SiteCases"] = cases
		data["SiteSummary"] = summary
		data["KeywordFreq"] = freqs
		return c.Render(http.StatusOK, "as/knowledge.html", data)
	}

	var items []model.ASSearchHit
	var total int
	if strings.TrimSpace(f.Query) != "" {
		items, total, err = h.repo.SearchKnowledge(f)
		if err != nil {
			return err
		}
		repository.DecorateKnowledgeHits(items, f.Query)
		attachKnowledgeLead(items)
		ids := make([]string, len(items))
		for i := range items {
			ids[i] = items[i].ASID
		}
		voted := h.repo.CaseVotedSet(ctxString(c, "user_id"), ids)
		for i := range items {
			if items[i].ASID != "" {
				items[i].Voted = voted[items[i].ASID]
			}
		}
		h.attachKBFiles(items)
		if total == 0 {
			_, _ = h.repo.RecordKBGap(f.Query, ctxString(c, "user_name"), asRef)
		}
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + 19) / 20
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	base := cloneURLValues(c.QueryParams())
	base.Del("page")
	data["Items"] = items
	data["Total"] = total
	data["Page"] = page
	data["TotalPages"] = totalPages
	data["PrevURL"] = asPageURL("/as/knowledge", base, page-1)
	data["NextURL"] = asPageURL("/as/knowledge", base, page+1)
	return c.Render(http.StatusOK, "as/knowledge.html", data)
}

// KnowledgeExcel 검색 결과 xlsx. §41.3.3
func (h *ASHandler) KnowledgeExcel(c echo.Context) error {
	tab := strings.TrimSpace(c.QueryParam("tab"))
	f := knowledgeFilter(c)
	f.Page = 1
	f.PageSize = 2000
	var items []model.ASSearchHit
	var err error
	if tab == "site" && f.CustomerID != "" {
		items, err = h.repo.KnowledgeSiteCases(f.CustomerID, 2000)
	} else if strings.TrimSpace(f.Query) != "" {
		items, _, err = h.repo.SearchKnowledge(f)
	}
	if err != nil {
		return err
	}
	attachKnowledgeLead(items)
	file, err := buildKnowledgeExcel(items)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	buf, err := file.WriteToBuffer()
	if err != nil {
		return err
	}
	return writeExcelDownload(c, buf.Bytes(), "as_knowledge")
}

func buildKnowledgeExcel(items []model.ASSearchHit) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}
	if err := f.SetSheetName(sheet, "AS사례"); err != nil {
		return nil, err
	}
	sheet = "AS사례"
	headers := []string{"접수일", "사이트", "증상", "조치", "소요(영업일)", "담당자", "AS번호", "출처", "작성자", "작성일", "원본작성자", "원본작성일"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for r, it := range items {
		action := it.Action
		if !it.HasAction {
			action = ""
		}
		org := it.OrgName
		if org == "" {
			org = "—"
		}
		vals := []interface{}{
			it.ReceiptDate, org, it.Symptom, action, it.LeadLabel, it.AssignedTo, it.ASNumber,
			it.OriginLabel, it.AuthorName, it.AuthorDate, it.SourceName, it.SourceDate,
		}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	return f, nil
}

func (h *ASHandler) attachKBFiles(items []model.ASSearchHit) {
	if h == nil || h.attachRepo == nil {
		return
	}
	for i := range items {
		id := strings.TrimSpace(items[i].KBID)
		if id == "" {
			continue
		}
		atts, err := h.attachRepo.ListByRef(model.RefTypeASKB, id)
		if err != nil || len(atts) == 0 {
			continue
		}
		items[i].Attachments = atts
	}
}

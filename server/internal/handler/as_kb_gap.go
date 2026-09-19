package handler

import (
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// KnowledgeGaps 답 없는 검색어와 조치 없는 AS. §41.13
func (h *ASHandler) KnowledgeGaps(c echo.Context) error {
	if !canProcessAS(c) && !canWriteMaster(c) {
		return echo.ErrForbidden
	}
	gaps, err := h.repo.ListOpenKBGaps()
	if err != nil {
		return err
	}
	groups, err := h.repo.ListGapKeywordGroups()
	if err != nil {
		return err
	}
	filled, openQ := h.repo.KBGapProgress()
	return c.Render(http.StatusOK, "as/kb_gaps.html", map[string]interface{}{
		"Title": "지식 보완 목록", "Active": NavASKnowledge,
		"Gaps": gaps, "Groups": groups,
		"MissingCount": h.repo.MissingActionCount(),
		"FilledMonth":  filled,
		"OpenQueries":  openQ,
		"CanProcess":   canProcessAS(c),
		"DisplayName":  ctxString(c, "user_name"),
		"Err":          c.QueryParam("err"),
		"OK":           c.QueryParam("ok"),
	})
}

type gapGroupQuery struct {
	other           bool
	keywordID, site string
	sort, dir       string
	offset          int
}

func gapGroupFromRequest(c echo.Context) gapGroupQuery {
	get := func(k string) string {
		if v := strings.TrimSpace(c.FormValue(k)); v != "" {
			return v
		}
		return strings.TrimSpace(c.QueryParam(k))
	}
	offset, _ := strconv.Atoi(get("offset"))
	if offset < 0 {
		offset = 0
	}
	sort, dir := parseOptionalSort(get("sort"), get("dir"), "receipt_date,as_number,customer,symptom")
	if sort == "" {
		sort, dir = "receipt_date", "desc"
	}
	return gapGroupQuery{
		other:     get("other") == "1",
		keywordID: get("keyword_id"),
		site:      get("site"),
		sort:      sort,
		dir:       dir,
		offset:    offset,
	}
}

const gapReceiptsLimit = 20

// KnowledgeGapGroup 묶음을 펼친 접수 목록. htmx 부분 응답. §41.16.3
func (h *ASHandler) KnowledgeGapGroup(c echo.Context) error {
	if !canProcessAS(c) && !canWriteMaster(c) {
		return echo.ErrForbidden
	}
	q := gapGroupFromRequest(c)
	if !q.other && q.keywordID == "" {
		return echo.ErrBadRequest
	}
	return h.renderGapGroup(c, q)
}

func (h *ASHandler) renderGapGroup(c echo.Context, q gapGroupQuery) error {
	items, total, err := h.repo.ListGapReceipts(q.keywordID, q.other, q.site, q.sort, q.dir, q.offset, gapReceiptsLimit)
	if err != nil {
		return err
	}
	sites, _ := h.repo.ListGapReceiptSites(q.keywordID, q.other)
	groupKey := q.keywordID
	if q.other {
		groupKey = "other"
	}
	from, to := 0, 0
	if total > 0 && len(items) > 0 {
		from = q.offset + 1
		to = q.offset + len(items)
	}
	filter := url.Values{}
	if q.other {
		filter.Set("other", "1")
	} else {
		filter.Set("keyword_id", q.keywordID)
	}
	if q.site != "" {
		filter.Set("site", q.site)
	}
	cols := []string{"as_number", "receipt_date", "customer", "symptom"}
	hrefs := sortLinkHrefs("/as/knowledge/gaps/group", filter, cols, q.sort, q.dir)
	next := cloneURLValues(filter)
	next.Set("sort", q.sort)
	next.Set("dir", q.dir)
	next.Set("offset", strconv.Itoa(q.offset+gapReceiptsLimit))
	data := map[string]interface{}{
		"Items":      items,
		"Total":      total,
		"From":       from,
		"To":         to,
		"HasMore":    q.offset+len(items) < total,
		"MoreURL":    "/as/knowledge/gaps/group?" + next.Encode(),
		"RowsOnly":   q.offset > 0,
		"GroupKey":   groupKey,
		"KeywordID":  q.keywordID,
		"Other":      q.other,
		"Site":       q.site,
		"Sites":      sites,
		"Sort":       q.sort,
		"Dir":        q.dir,
		"SortLinks":  hrefs,
		"SortSelect": sortSelectOptions(gapReceiptSortCols(), hrefs, q.sort, q.dir),
		"CanProcess": canProcessAS(c),
		"IndexEmpty": h.repo.KeywordLinkCount() == 0,
	}
	return renderGapReceipts(c, data)
}

func gapReceiptSortCols() []sortCol {
	return []sortCol{
		{Key: "as_number", Label: "접수번호"},
		{Key: "receipt_date", Label: "접수일"},
		{Key: "customer", Label: "사이트"},
		{Key: "symptom", Label: "증상"},
	}
}

func renderGapReceipts(c echo.Context, data interface{}) error {
	tmpl, err := template.New("_gap_receipts.html").Funcs(funcMap()).ParseFiles(
		filepath.Join("web/templates/sort/_th.html"),
		filepath.Join("web/templates/as/_gap_receipts.html"),
	)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Response().Header().Set("Cache-Control", "no-cache")
	return tmpl.ExecuteTemplate(c.Response().Writer, "_gap_receipts.html", data)
}

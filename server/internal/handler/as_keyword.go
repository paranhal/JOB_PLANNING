package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func (h *ASHandler) saveKeywordChecks(c echo.Context, asID, field string) {
	if h.kwRepo == nil || asID == "" {
		return
	}
	params, _ := c.FormParams()
	checked := params["kw_"+field]
	suggested := model.SplitCommaList(c.FormValue("kw_suggested_" + field))
	_ = h.kwRepo.ReplaceLinks(asID, field, checked, suggested)
}

func (h *ASHandler) rebuildDictKeywordLinks(asID string) {
	if h == nil || h.repo == nil || strings.TrimSpace(asID) == "" {
		return
	}
	if _, err := h.repo.RebuildKeywordLinks([]string{asID}); err != nil {
		log.Printf("as_keyword_links rebuild %s: %v", asID, err)
	}
}

func (h *ASHandler) keywordChecks(asID, field, text string) []model.ASKeyword {
	if h.kwRepo == nil {
		return nil
	}
	sug, _ := h.kwRepo.Suggest(text)
	linked := map[string]bool{}
	if asID != "" {
		links, _ := h.kwRepo.LinksByAS(asID)
		for _, l := range links {
			if l.Field == field {
				linked[l.KeywordID] = true
			}
		}
	}
	seen := map[string]bool{}
	var out []model.ASKeyword
	for _, k := range sug {
		k.Suggested = true
		k.Linked = linked[k.KeywordID]
		seen[k.KeywordID] = true
		out = append(out, k)
	}
	if asID == "" {
		return out
	}
	links, _ := h.kwRepo.LinksByAS(asID)
	all, _ := h.kwRepo.List(true)
	byID := map[string]model.ASKeyword{}
	for _, k := range all {
		byID[k.KeywordID] = k
	}
	for _, l := range links {
		if l.Field != field || seen[l.KeywordID] {
			continue
		}
		k := byID[l.KeywordID]
		if k.KeywordID == "" {
			continue
		}
		k.Linked = true
		out = append(out, k)
	}
	return out
}

// KeywordList 사전 관리 + 본문 빈도 후보. 후보는 제안만. §12.11.6
func (h *ASHandler) KeywordList(c echo.Context) error {
	if !canReceiveAS(c) && !canProcessAS(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	items, err := h.kwRepo.List(false)
	if err != nil {
		return err
	}
	cands, _ := h.kwRepo.FrequencyCandidates(40)
	linkN, receiptN := 0, 0
	if h.repo != nil {
		linkN = h.repo.KeywordLinkCount()
		receiptN = h.repo.ReceiptCount()
	}
	return c.Render(http.StatusOK, "as/keywords.html", map[string]interface{}{
		"Title": "AS 키워드 사전", "Active": NavAS,
		"Items": items, "Candidates": cands, "Groups": model.KWGroups(),
		"LinkTotal": linkN, "ReceiptTotal": receiptN,
		"CanEdit": canReceiveAS(c) || canProcessAS(c) || isAdminRole(c),
		"Err":     c.QueryParam("err"), "OK": c.QueryParam("ok"),
	})
}

func (h *ASHandler) KeywordCreate(c echo.Context) error {
	if !canReceiveAS(c) && !canProcessAS(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	k := &model.ASKeyword{
		Keyword:  strings.TrimSpace(c.FormValue("keyword")),
		Group:    strings.TrimSpace(c.FormValue("kw_group")),
		Synonyms: strings.TrimSpace(c.FormValue("synonyms")),
		Note:     strings.TrimSpace(c.FormValue("note")),
	}
	k.SortOrder, _ = strconv.Atoi(c.FormValue("sort_order"))
	if err := h.kwRepo.Create(k); err != nil {
		code := "keyword"
		if err.Error() == "stopword" {
			code = "stopword"
		}
		return c.Redirect(http.StatusSeeOther, "/as/keywords?err="+code)
	}
	return c.Redirect(http.StatusSeeOther, "/as/keywords?ok=1")
}

func (h *ASHandler) KeywordUpdate(c echo.Context) error {
	if !canReceiveAS(c) && !canProcessAS(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	cur, err := h.kwRepo.Get(id)
	if err != nil || cur == nil {
		return echo.ErrNotFound
	}
	cur.Keyword = strings.TrimSpace(c.FormValue("keyword"))
	cur.Group = strings.TrimSpace(c.FormValue("kw_group"))
	cur.Synonyms = strings.TrimSpace(c.FormValue("synonyms"))
	cur.Note = strings.TrimSpace(c.FormValue("note"))
	cur.SortOrder, _ = strconv.Atoi(c.FormValue("sort_order"))
	cur.IsActive = c.FormValue("is_active") == "1"
	if err := h.kwRepo.Update(cur); err != nil {
		return c.Redirect(http.StatusSeeOther, "/as/keywords?err=keyword")
	}
	return c.Redirect(http.StatusSeeOther, "/as/keywords?ok=1")
}

func (h *ASHandler) KeywordDelete(c echo.Context) error {
	if !isAdminRole(c) && !canReceiveAS(c) && !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	if err := h.kwRepo.Delete(id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/keywords?ok=1")
}

func (h *ASHandler) KeywordRebuild(c echo.Context) error {
	if !canReceiveAS(c) && !canProcessAS(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	if _, err := h.repo.RebuildKeywordLinks(nil); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/as/keywords?ok=relink")
}

// KeywordSuggest 본문 대조 후보 JSON. 쓰지 않으면 붙지 않는다.
func (h *ASHandler) KeywordSuggest(c echo.Context) error {
	items, err := h.kwRepo.Suggest(c.QueryParam("q"))
	if err != nil {
		return err
	}
	if items == nil {
		items = []model.ASKeyword{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"items": items})
}

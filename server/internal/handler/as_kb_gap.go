package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
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
	rows, err := h.repo.ListMissingActionSymptoms()
	if err != nil {
		return err
	}
	var dict []model.ASKeyword
	if h.kwRepo != nil {
		dict, _ = h.kwRepo.List(true)
	}
	groups := repository.GroupMissingActionByKeyword(dict, rows)
	filled, openQ := h.repo.KBGapProgress()
	return c.Render(http.StatusOK, "as/kb_gaps.html", map[string]interface{}{
		"Title": "지식 보완 목록", "Active": NavASKnowledge,
		"Gaps": gaps, "Groups": groups,
		"MissingCount": h.repo.MissingActionCount(),
		"FilledMonth":  filled,
		"OpenQueries":  openQ,
		"CanProcess":   canProcessAS(c),
		"DisplayName":  ctxString(c, "user_name"),
	})
}

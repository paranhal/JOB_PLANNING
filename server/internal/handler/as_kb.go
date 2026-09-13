package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *ASHandler) kbWriteFrom(c echo.Context) repository.KBWrite {
	return repository.KBWrite{
		ASID:       strings.TrimSpace(c.FormValue("as_id")),
		ActionText: strings.TrimSpace(c.FormValue("action_text")),
		Symptom:    strings.TrimSpace(c.FormValue("symptom_text")),
		ChangeNote: strings.TrimSpace(c.FormValue("change_note")),
		AuthorID:   ctxString(c, "user_id"),
		AuthorName: ctxString(c, "user_name"),
	}
}

func kbBack(c echo.Context, asID string) string {
	if ref := strings.TrimSpace(c.Request().Header.Get("Referer")); ref != "" {
		return ref
	}
	if q := strings.TrimSpace(c.FormValue("q")); q != "" {
		return "/as/knowledge?q=" + url.QueryEscape(q)
	}
	if asID != "" {
		return "/as/" + asID + "/action"
	}
	return "/as/knowledge"
}

func (h *ASHandler) CreateKnowledge(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	in := h.kbWriteFrom(c)
	if strings.TrimSpace(in.ActionText) == "" {
		return c.Redirect(http.StatusSeeOther, kbBack(c, in.ASID))
	}
	if _, err := h.repo.PublishKB(in); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, kbBack(c, in.ASID))
}

func (h *ASHandler) ReviseKnowledge(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("kb_id"))
	in := h.kbWriteFrom(c)
	if _, err := h.repo.ReviseKB(id, in); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, kbBack(c, in.ASID))
}

func (h *ASHandler) ArchiveKnowledge(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.Param("kb_id"))
	if err := h.repo.ArchiveKB(id); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, kbBack(c, c.FormValue("as_id")))
}

func (h *ASHandler) KnowledgeHistory(c echo.Context) error {
	id := strings.TrimSpace(c.Param("kb_id"))
	cur, err := h.repo.GetKB(id)
	if err != nil || cur == nil {
		return echo.ErrNotFound
	}
	past := h.repo.KBHistory(id)
	return c.Render(http.StatusOK, "as/kb_history.html", map[string]interface{}{
		"Title": "수정 이력", "Active": NavASKnowledge,
		"Current": cur, "Past": past,
		"AuthorName": model.DisplayPerson(cur.AuthorName),
		"CanReceive": canReceiveAS(c),
		"DisplayName": ctxString(c, "user_name"),
	})
}

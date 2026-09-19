package handler

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const maxKnowledgeBulk = 50

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
	if strings.TrimSpace(c.FormValue("from")) == "gaps" {
		return "/as/knowledge/gaps"
	}
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

func hxRequest(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

func (h *ASHandler) CreateKnowledge(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	in := h.kbWriteFrom(c)
	if strings.TrimSpace(in.ActionText) == "" {
		if hxRequest(c) {
			return c.HTML(http.StatusUnprocessableEntity, `<p class="text-xs text-red-700">조치 내용을 입력하세요.</p>`)
		}
		return c.Redirect(http.StatusSeeOther, kbBack(c, in.ASID))
	}
	kb, err := h.repo.PublishKB(in)
	if err != nil {
		return err
	}
	if kb != nil {
		_ = h.repo.ResolveKBGap(strings.TrimSpace(c.FormValue("gap_id")), kb.KBID)
		h.attachKBPhotos(c, kb.KBID)
	}
	if hxRequest(c) {
		key := strings.TrimSpace(c.FormValue("group_key"))
		setGapFilledTrigger(c, key, 1)
		return c.HTML(http.StatusOK, kbGapFilledHTML(in.ASID))
	}
	return c.Redirect(http.StatusSeeOther, kbBack(c, in.ASID))
}

func (h *ASHandler) attachKBPhotos(c echo.Context, kbID string) {
	if h.attach == nil || strings.TrimSpace(kbID) == "" {
		return
	}
	files, err := uploadFileHeaders(c)
	if err != nil || len(files) == 0 {
		return
	}
	_ = h.attach.saveKBFiles(kbID, strings.TrimSpace(c.FormValue("photo_memo")), files)
}

func kbGapFilledHTML(asID string) string {
	id := html.EscapeString(asID)
	return fmt.Sprintf(`<tbody id="gap-as-%s" class="bg-emerald-50"><tr><td colspan="7" class="px-2 py-2 text-xs text-emerald-800">지식 있음</td></tr></tbody>`, id)
}

func setGapFilledTrigger(c echo.Context, key string, n int) {
	payload, err := json.Marshal(map[string]any{"gapFilled": map[string]any{"key": key, "n": n}})
	if err != nil {
		return
	}
	c.Response().Header().Set("HX-Trigger", string(payload))
}

func uniqueFormIDs(vals []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range vals {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func kbBulkLimitMessage(n int) string {
	if n == 0 {
		return "접수를 선택하세요."
	}
	if n > maxKnowledgeBulk {
		return "한 번에 50건까지만 넣을 수 있습니다. 전체가 아니라 증상을 확인한 건만 고르세요."
	}
	return ""
}

// CreateKnowledgeBulk 고른 접수마다 지식 한 줄씩 저장한다. 한 줄을 공유하지 않는다. §41.16.4
func (h *ASHandler) CreateKnowledgeBulk(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	ids := uniqueFormIDs(c.Request().Form["as_id"])
	if msg := kbBulkLimitMessage(len(ids)); msg != "" {
		return h.kbBulkReject(c, msg, len(ids))
	}
	action := strings.TrimSpace(c.FormValue("action_text"))
	if action == "" {
		return h.kbBulkReject(c, "조치 내용을 입력하세요.", 0)
	}
	authorID := ctxString(c, "user_id")
	authorName := ctxString(c, "user_name")
	for _, id := range ids {
		as, err := h.repo.GetByID(id)
		if err != nil || as == nil {
			return fmt.Errorf("접수를 찾을 수 없습니다")
		}
		if _, err := h.repo.PublishKB(repository.KBWrite{
			ASID:       as.ASID,
			ActionText: action,
			Symptom:    as.Symptom,
			AuthorID:   authorID,
			AuthorName: authorName,
		}); err != nil {
			return err
		}
	}
	if hxRequest(c) {
		key := strings.TrimSpace(c.FormValue("group_key"))
		if key == "" {
			key = strings.TrimSpace(c.FormValue("keyword_id"))
			if c.FormValue("other") == "1" {
				key = "other"
			}
		}
		setGapFilledTrigger(c, key, len(ids))
		c.Response().Header().Set("HX-Retarget", "#grp-"+key)
		c.Response().Header().Set("HX-Reswap", "innerHTML")
		return h.renderGapGroup(c, gapGroupFromRequest(c))
	}
	return c.Redirect(http.StatusSeeOther, "/as/knowledge/gaps?ok=bulk")
}

func (h *ASHandler) kbBulkReject(c echo.Context, msg string, n int) error {
	if hxRequest(c) {
		code := http.StatusUnprocessableEntity
		if n > maxKnowledgeBulk {
			code = http.StatusBadRequest
		}
		return c.HTML(code, `<p class="text-xs text-red-700">`+html.EscapeString(msg)+`</p>`)
	}
	q := "bulk_none"
	if n > maxKnowledgeBulk {
		q = "bulk_limit"
	}
	if strings.Contains(msg, "조치") {
		q = "bulk_action"
	}
	return c.Redirect(http.StatusSeeOther, "/as/knowledge/gaps?err="+q)
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
	var atts []model.Attachment
	if h.attachRepo != nil {
		atts, _ = h.attachRepo.ListByRef(model.RefTypeASKB, id)
	}
	return c.Render(http.StatusOK, "as/kb_history.html", map[string]interface{}{
		"Title": "수정 이력", "Active": NavASKnowledge,
		"Current": cur, "Past": past, "Attachments": atts,
		"AuthorName":   model.DisplayPerson(cur.AuthorName),
		"CanReceive":   canReceiveAS(c),
		"CanProcess":   canProcessAS(c),
		"KBID":         id,
		"AttachErrMsg": attachErrMessage(c.QueryParam("err")),
		"DisplayName":  ctxString(c, "user_name"),
	})
}

package handler

import (
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

const (
	flashOKCookie  = "jp_flash_ok"
	flashOKSaved   = "saved"
	flashOKDeleted = "deleted"
)

type asSavePopupItem struct {
	ASID       string
	ASNumber   string
	OrgName    string
	Snippet    string
	ActionHref string
	CanDelete  bool
}

func setFlashOKCookie(c echo.Context, kind string, values []string) {
	if c == nil || len(values) == 0 {
		return
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return
	}
	raw := kind + ":" + strings.Join(values, ",")
	c.SetCookie(sessionCookie(flashOKCookie, url.QueryEscape(raw), 120))
}

func readFlashOKCookie(c echo.Context) (kind string, values []string) {
	if c == nil || c.Request() == nil {
		return "", nil
	}
	ck, err := c.Cookie(flashOKCookie)
	if err != nil || ck == nil {
		return "", nil
	}
	raw, err := url.QueryUnescape(ck.Value)
	if err != nil {
		raw = ck.Value
	}
	raw = strings.TrimSpace(raw)
	kind, rest, ok := strings.Cut(raw, ":")
	if !ok {
		return "", nil
	}
	for _, p := range strings.Split(rest, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			values = append(values, p)
		}
	}
	return strings.TrimSpace(kind), values
}

func clearFlashOKCookie(c echo.Context) {
	if c == nil || c.Response().Committed {
		return
	}
	c.SetCookie(sessionCookie(flashOKCookie, "", -1))
}

func asSymptomSnippet(s string) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	r := []rune(s)
	if len(r) <= 10 {
		return s
	}
	return string(r[:10]) + "…"
}

func (h *ASHandler) asSavePopupItems(c echo.Context, ids []string) []asSavePopupItem {
	if h == nil || h.repo == nil {
		return nil
	}
	var out []asSavePopupItem
	for _, id := range ids {
		as, err := h.repo.GetByID(repository.OrgAll, id)
		if err != nil || as == nil {
			continue
		}
		if !CanViewRecord(c, as) {
			continue
		}
		n := 0
		if h.processRepo != nil {
			if procs, err := h.processRepo.ListByAS(as.ASID); err == nil {
				n = len(procs)
			}
		}
		out = append(out, asSavePopupItem{
			ASID:       as.ASID,
			ASNumber:   as.ASNumber,
			OrgName:    as.OrgName,
			Snippet:    asSymptomSnippet(as.Symptom),
			ActionHref: asOpenHref(as.ASID),
			CanDelete:  canDeleteAS(c, as, n, h.canModifyAS(c, as)),
		})
	}
	return out
}

var asFlashHandler *ASHandler

func bindASFlashHandler(h *ASHandler) {
	asFlashHandler = h
}

func injectASSaveFlash(c echo.Context, data map[string]interface{}) {
	kind, values := readFlashOKCookie(c)
	if kind == "" || len(values) == 0 {
		return
	}
	if kind != flashOKSaved && kind != flashOKDeleted {
		return
	}
	clearFlashOKCookie(c)
	switch kind {
	case flashOKSaved:
		if asFlashHandler != nil {
			items := asFlashHandler.asSavePopupItems(c, values)
			if len(items) > 0 {
				data["ASSavePopup"] = items
			}
		}
	case flashOKDeleted:
		data["ASDeletedPopup"] = values[0]
	}
}

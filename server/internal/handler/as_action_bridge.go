package handler

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

var actionSetOwnedFields = map[string]bool{
	"cause_cat1":          true,
	"cause_cat1_custom":   true,
	"cause_cat2":          true,
	"cause_cat3":          true,
	"cause_detail":        true,
	"process_type_reason": true,
	"action_na":           true,
	"action_na_reason":    true,
}

type actionFormView struct {
	c    echo.Context
	last *model.ASActionSet
}

func newActionFormView(c echo.Context, posted []model.ASActionSet) *actionFormView {
	for i := range posted {
		normalizeCauseFromSet(&posted[i])
	}
	return &actionFormView{c: c, last: model.LastCommittedActionSet(posted)}
}

func (v *actionFormView) Get(name string) string {
	if v == nil || v.c == nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if v.last != nil && actionSetOwnedFields[name] {
		return actionSetField(v.last, name)
	}
	return v.c.FormValue(name)
}

func (v *actionFormView) seq() int {
	if v == nil || v.last == nil {
		return 0
	}
	return v.last.Seq
}

func actionSetField(s *model.ASActionSet, name string) string {
	if s == nil {
		return ""
	}
	switch name {
	case "cause_cat1":
		return strings.TrimSpace(s.CauseCat1)
	case "cause_cat1_custom":
		return strings.TrimSpace(s.CauseCat1Custom)
	case "cause_cat2":
		return strings.TrimSpace(s.CauseCat2)
	case "cause_cat3":
		return strings.TrimSpace(s.CauseCat3)
	case "cause_detail":
		return strings.TrimSpace(s.CauseDetail)
	case "process_type_reason":
		return strings.TrimSpace(s.ProcessTypeReason)
	case "action_na":
		if s.ActionNA {
			return "1"
		}
		return ""
	case "action_na_reason":
		return strings.TrimSpace(s.ActionNAReason)
	default:
		return ""
	}
}

func normalizeCauseFromSet(s *model.ASActionSet) {
	if s == nil {
		return
	}
	s.CauseCat1 = strings.TrimSpace(s.CauseCat1)
	s.CauseCat1Custom = model.NormalizeCauseCatCustom(s.CauseCat1Custom)
	s.CauseCat2 = strings.TrimSpace(s.CauseCat2)
	s.CauseCat3 = strings.TrimSpace(s.CauseCat3)
	if s.CauseCat1 == "" && s.CauseCat1Custom != "" {
		s.CauseCat1 = model.CauseCatCustom
	}
	if s.CauseCat1 == model.CauseCatCustom {
		s.CauseCat2 = ""
		s.CauseCat3 = ""
		return
	}
	s.CauseCat1Custom = ""
}

func (h *ASHandler) keepPostedAsDraft(c echo.Context, as *model.ASReceipt, posted []model.ASActionSet) {
	if h == nil || h.setRepo == nil || as == nil || len(posted) == 0 {
		return
	}
	_ = c
	for i := range posted {
		s := posted[i]
		s.ASID = as.ASID
		if strings.TrimSpace(s.ResultCode) == "" {
			s.State = model.ActionSetStateDraft
		} else if s.State == "" || s.State == model.ActionSetStateFinal {
			s.State = model.ActionSetStateSaved
		}
		_ = h.setRepo.Upsert(&s)
	}
}

func actionSetErrWithSeq(code string, seq int) string {
	code = strings.TrimSpace(code)
	if strings.HasPrefix(code, "set_draft_") {
		n := strings.TrimPrefix(code, "set_draft_")
		if seq < 1 {
			seq, _ = strconv.Atoi(n)
		}
		if seq < 1 {
			seq = 1
		}
		return "조치 " + strconv.Itoa(seq) + "에 결과가 선택되지 않았습니다. 결과를 선택하고 저장해 주세요"
	}
	msg := actionErrMessage(code)
	if seq < 1 {
		return msg
	}
	switch code {
	case "cause_cat1", "cause_cat2", "cause_cat3", "cause_cat1_custom", "cause_cat1_custom_len", "cause_cat1_input":
		return "조치 " + strconv.Itoa(seq) + ": " + msg
	default:
		return msg
	}
}

func focusSetQuery(v *actionFormView) string {
	if v == nil || v.seq() < 1 {
		return ""
	}
	return "&focus_set=" + strconv.Itoa(v.seq())
}

func actionErrMessagePrefixed(c echo.Context) string {
	if c == nil {
		return ""
	}
	code := strings.TrimSpace(c.QueryParam("err"))
	seq, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam("focus_set")))
	if strings.HasPrefix(code, "set_draft_") || code == "set_draft" {
		return actionSetErrWithSeq(code, seq)
	}
	if seq > 0 {
		if msg := actionSetErrWithSeq(code, seq); msg != actionErrMessage(code) {
			return msg
		}
	}
	return actionErrMessageWithSupport(code, parseOpenSupportN(c))
}

func (h *ASHandler) listCauseCategoriesForAS(as *model.ASReceipt) ([]model.CauseCategory, error) {
	if h == nil || h.repo == nil {
		return nil, nil
	}
	org := ""
	if as != nil {
		org = as.OrgID
	}
	return h.repo.ListCauseCategoriesForOrg(org)
}

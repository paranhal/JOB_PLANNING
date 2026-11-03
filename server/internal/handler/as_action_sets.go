package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

var asActionSideEffectRuns int

func (h *ASHandler) actionSetsOrVirtual(as *model.ASReceipt) []model.ASActionSet {
	if as == nil {
		return nil
	}
	var sets []model.ASActionSet
	if h.setRepo != nil {
		sets, _ = h.setRepo.ListByAS(as.ASID)
	}
	if len(sets) == 0 {
		s := model.VirtualActionSetFromReceipt(as)
		return []model.ASActionSet{s}
	}
	photos := map[string]int{}
	var all []model.Attachment
	if h.attachRepo != nil {
		all, _ = h.attachRepo.ListByRef(model.RefTypeASActionPhoto, as.ASID)
	}
	for _, p := range all {
		key := strings.TrimSpace(p.ActionSetID)
		photos[key]++
	}
	for i := range sets {
		id := strings.TrimSpace(sets[i].SetID)
		sets[i].PhotoN = photos[id]
		if sets[i].Seq == 1 {
			sets[i].PhotoN += photos[""]
		}
		sets[i].Open = i == len(sets)-1
	}
	return model.ActionSetOverlapWarning(sets)
}

func (h *ASHandler) collectPostedActionSets(c echo.Context, as *model.ASReceipt) []model.ASActionSet {
	var out []model.ASActionSet
	for seq := 1; seq <= model.MaxASActionSets; seq++ {
		p := "s" + strconv.Itoa(seq) + "_"
		taken := strings.TrimSpace(c.FormValue(p + "action_taken"))
		result := strings.TrimSpace(c.FormValue(p + "result_code"))
		setID := strings.TrimSpace(c.FormValue(p + "set_id"))
		if taken == "" && result == "" && setID == "" && c.FormValue(p+"work_place") == "" {
			continue
		}
		s := h.parsePostedSet(c, p, as, seq)
		out = append(out, s)
	}
	return out
}

func (h *ASHandler) parsePostedSet(c echo.Context, p string, as *model.ASReceipt, seq int) model.ASActionSet {
	dur, _ := strconv.Atoi(strings.TrimSpace(c.FormValue(p + "duration_min")))
	s := model.ASActionSet{
		SetID:             strings.TrimSpace(c.FormValue(p + "set_id")),
		ASID:              as.ASID,
		Seq:               seq,
		AssignedTo:        firstNonEmpty(c.FormValue(p+"assigned_to"), as.AssignedTo),
		AssignedUserID:    firstNonEmpty(c.FormValue(p+"assigned_user_id"), as.AssignedUserID),
		WorkPlace:         model.NormalizeWorkPlace(c.FormValue(p + "work_place")),
		ProcessType:       strings.TrimSpace(c.FormValue(p + "process_type")),
		ProcessTypeReason: strings.TrimSpace(c.FormValue(p + "process_type_reason")),
		CauseCat1:         strings.TrimSpace(c.FormValue(p + "cause_cat1")),
		CauseCat1Custom:   strings.TrimSpace(c.FormValue(p + "cause_cat1_custom")),
		CauseCat2:         strings.TrimSpace(c.FormValue(p + "cause_cat2")),
		CauseCat3:         strings.TrimSpace(c.FormValue(p + "cause_cat3")),
		CauseDetail:       strings.TrimSpace(c.FormValue(p + "cause_detail")),
		WorkDate:          strings.TrimSpace(c.FormValue(p + "work_date")),
		StartTime:         model.ClockHHMMStrict(c.FormValue(p + "start_time")),
		DurationMin:       dur,
		LeaveSame:         c.FormValue(p+"leave_same") != "0",
		LeaveTime:         model.ClockHHMMStrict(c.FormValue(p + "leave_time")),
		ActionTaken:       strings.TrimSpace(c.FormValue(p + "action_taken")),
		ActionNA:          c.FormValue(p+"action_na") == "1",
		ActionNAReason:    strings.TrimSpace(c.FormValue(p + "action_na_reason")),
		ResultCode:        strings.TrimSpace(c.FormValue(p + "result_code")),
		CreatedBy:         ctxString(c, "user_id"),
	}
	model.ApplyVisitDateFromWork(&s, c.FormValue(p+"visit_date"))
	if uid := strings.TrimSpace(c.FormValue(p + "assigned_user_id")); uid != "" && h.userRepo != nil {
		if u, _ := h.userRepo.GetByID(uid); u != nil {
			s.AssignedUserID = u.UserID
			s.AssignedTo = u.FullName
		}
	}
	_ = c.Request().ParseForm()
	vals := c.Request().PostForm[p+"leave_same"]
	if len(vals) == 0 {
		s.LeaveSame = true
	} else {
		s.LeaveSame = false
		for _, v := range vals {
			if v == "1" {
				s.LeaveSame = true
			}
		}
	}
	na := s.ActionNA
	naReason := s.ActionNAReason
	if resolved, _ := model.ResolveActionContent(s.ActionTaken, na, naReason); resolved != "" {
		s.ActionTaken = resolved
	}
	model.FillActionSetComputedTimes(&s)
	return s
}

func setFromReceiptFields(as *model.ASReceipt) model.ASActionSet {
	s := model.VirtualActionSetFromReceipt(as)
	s.Virtual = false
	s.State = model.ActionSetStateFinal
	s.LeaveSame = true
	return s
}

func validateActionSetFormat(s model.ASActionSet) string {
	if strings.TrimSpace(s.ResultCode) != "" && !model.IsASActionResultAllowed(s.ResultCode) {
		return "result_not_allowed"
	}
	return model.ValidateActionSetWorkTime(s)
}

func validateActionSetDraft(s model.ASActionSet, requireResult bool) string {
	if requireResult && strings.TrimSpace(s.ResultCode) == "" {
		return "result_code"
	}
	if code := validateActionSetFormat(s); code != "" {
		return code
	}
	if !requireResult {
		return ""
	}
	resolved, code := model.ResolveActionContent(s.ActionTaken, s.ActionNA, s.ActionNAReason)
	if code != "" {
		return code
	}
	s.ActionTaken = resolved
	tmp := model.ASReceipt{
		ActionTaken:       s.ActionTaken,
		WorkPlace:         s.WorkPlace,
		ProcessType:       s.ProcessType,
		ProcessTypeReason: s.ProcessTypeReason,
		VisitDate:         s.VisitDate,
		ResultCode:        s.ResultCode,
	}
	if code := actionSaveMissingErr(&tmp, s.ResultCode); code != "" {
		return code
	}
	return ""
}

func actionSetTimeErrMessage(code string) string {
	switch code {
	case "work_time_pair":
		return "업무 시작 시간과 작업 시간을 함께 입력하세요"
	case "work_time_midnight":
		return "작업이 자정을 넘을 수 없습니다"
	case "leave_before_end":
		return "철수 시간은 완료 시간 이후여야 합니다"
	case "leave_time_step", "start_time_step", "duration_step":
		return "시간은 10분 단위로 입력하세요"
	case "leave_time_required":
		return "외근에서 철수 시간을 직접 입력하세요"
	default:
		return actionErrMessage(code)
	}
}

func (h *ASHandler) persistPostedSets(c echo.Context, as *model.ASReceipt, final bool) ([]model.ASActionSet, string) {
	sets := h.collectPostedActionSets(c, as)
	if len(sets) == 0 {
		if !final {
			return nil, ""
		}
		s := setFromReceiptFields(as)
		if code := validateActionSetDraft(s, strings.TrimSpace(as.ResultCode) != ""); code != "" && strings.TrimSpace(as.ResultCode) != "" {
			return nil, code
		}
		if strings.TrimSpace(as.ResultCode) == "" && strings.TrimSpace(as.ActionTaken) == "" {
			return nil, ""
		}
		s.State = model.ActionSetStateFinal
		if h.setRepo != nil {
			_ = h.setRepo.ReplaceFinal(as.ASID, []model.ASActionSet{s})
		}
		return []model.ASActionSet{s}, ""
	}
	for i := range sets {
		if final && (strings.TrimSpace(sets[i].ResultCode) == "" || sets[i].State == model.ActionSetStateDraft && strings.TrimSpace(sets[i].ResultCode) == "") {
			return sets, "set_draft_" + strconv.Itoa(sets[i].Seq)
		}
		if code := validateActionSetDraft(sets[i], final); code != "" {
			return sets, code
		}
		if final {
			sets[i].State = model.ActionSetStateFinal
		} else if sets[i].State == "" {
			sets[i].State = model.ActionSetStateSaved
		}
	}
	if h.setRepo == nil {
		return sets, ""
	}
	if final {
		if err := h.setRepo.ReplaceFinal(as.ASID, sets); err != nil {
			return sets, "action"
		}
	}
	return sets, ""
}

// SaveActionSet 임시 저장. 부수효과 없음. POST /as/:id/action-sets
func (h *ASHandler) SaveActionSet(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(currentOrg(c), id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if isASClosedStatus(as.Status) && !h.canModifyAS(c, as) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "읽기 전용입니다"})
	}
	seq, _ := strconv.Atoi(firstNonEmpty(c.FormValue("seq"), c.FormValue("s_seq")))
	if seq < 1 {
		seq = 1
	}
	if seq > model.MaxASActionSets {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "조치는 20개까지입니다"})
	}
	p := "s" + strconv.Itoa(seq) + "_"
	if c.FormValue(p+"action_taken") == "" && c.FormValue("action_taken") != "" {
		p = ""
	}
	s := h.parsePostedSet(c, p, as, seq)
	if p == "" {
		s = h.parsePostedSet(c, "", as, seq)
		if s.ActionTaken == "" {
			s.ActionTaken = strings.TrimSpace(c.FormValue("action_taken"))
		}
		if s.ResultCode == "" {
			s.ResultCode = strings.TrimSpace(c.FormValue("result_code"))
		}
		if s.WorkPlace == "" {
			s.WorkPlace = model.NormalizeWorkPlace(c.FormValue("work_place"))
		}
		if s.ProcessType == "" {
			s.ProcessType = strings.TrimSpace(c.FormValue("process_type"))
		}
	}
	draft := c.FormValue("draft") == "1"
	if strings.TrimSpace(s.ResultCode) == "" && !draft {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": actionErrMessage("result_code"), "code": "result_code"})
	}
	if code := validateActionSetDraft(s, !draft); code != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": actionSetTimeErrMessage(code), "code": code})
	}
	if draft {
		s.State = model.ActionSetStateDraft
	} else {
		s.State = model.ActionSetStateSaved
	}
	beforeStatus := as.Status
	if h.setRepo != nil {
		if err := h.setRepo.Upsert(&s); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	}
	got, _ := h.repo.GetByID(currentOrg(c), id)
	if got != nil && got.Status != beforeStatus {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "상태가 바뀌었습니다"})
	}
	missing := model.ActionSetMissingFields(s)
	msg := "조치 " + strconv.Itoa(s.Seq) + " 임시 저장됨"
	if draft {
		msg = "조치 " + strconv.Itoa(s.Seq) + " 임시 저장됨 (결과 미선택 — 최종 저장 전에 결과를 선택해 주세요)"
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok": true, "seq": s.Seq, "set_id": s.SetID, "state": s.State,
		"missing": missing, "message": msg,
	})
}

func (h *ASHandler) KeywordSuggestPost(c echo.Context) error {
	if !canReceiveAS(c) && !canProcessAS(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	text := strings.TrimSpace(c.FormValue("text"))
	if text == "" {
		text = strings.TrimSpace(c.FormValue("q"))
	}
	if text == "" && c.Request().Method == http.MethodGet {
		text = strings.TrimSpace(c.QueryParam("q"))
	}
	items, err := h.kwRepo.Suggest(text)
	if err != nil {
		return err
	}
	if items == nil {
		items = []model.ASKeyword{}
	}
	news := model.SuggestNewKeywords(text, model.DictMatchTerms(items))
	if news == nil {
		news = []model.KeywordCand{}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"items": items, "new": news})
}

func (h *ASHandler) applyManualKeywords(c echo.Context, asID string) string {
	raw := strings.TrimSpace(c.FormValue("kw_manual"))
	if raw == "" {
		return ""
	}
	parsed := model.ParseKeywordTokens(raw)
	if len(parsed.Err) > 0 {
		return "keyword_manual"
	}
	if h.kwRepo == nil {
		return ""
	}
	var ids []string
	for _, cand := range parsed.OK {
		k := &model.ASKeyword{Keyword: cand.Keyword, Group: cand.Group, IsActive: true}
		if err := h.kwRepo.Create(k); err != nil {
			if existing := h.findKeywordByName(cand.Keyword, cand.Group); existing != "" {
				ids = append(ids, existing)
				continue
			}
			return "keyword"
		}
		ids = append(ids, k.KeywordID)
	}
	if len(ids) == 0 {
		return ""
	}
	checked := append([]string{}, c.Request().Form["kw_"+model.KWFieldAction]...)
	checked = append(checked, ids...)
	_ = h.kwRepo.ReplaceLinks(asID, model.KWFieldAction, checked, model.SplitCommaList(c.FormValue("kw_suggested_"+model.KWFieldAction)))
	return ""
}

func (h *ASHandler) findKeywordByName(name, group string) string {
	all, _ := h.kwRepo.List(true)
	for _, k := range all {
		if strings.EqualFold(k.Keyword, name) && (group == "" || k.Group == group) {
			return k.KeywordID
		}
	}
	return ""
}

func (h *ASHandler) applyNewKeywordChips(c echo.Context, asID string) {
	if h.kwRepo == nil {
		return
	}
	params, _ := c.FormParams()
	var ids []string
	for _, v := range params["kw_"+model.KWFieldAction] {
		v = strings.TrimSpace(v)
		if !strings.HasPrefix(v, "new:") {
			continue
		}
		rest := strings.TrimPrefix(v, "new:")
		name, g := rest, model.KWGroupAction
		if i := strings.Index(rest, "|"); i >= 0 {
			name = strings.TrimSpace(rest[:i])
			g = strings.TrimSpace(rest[i+1:])
		}
		if name == "" {
			continue
		}
		k := &model.ASKeyword{Keyword: name, Group: g, IsActive: true}
		if err := h.kwRepo.Create(k); err != nil {
			if id := h.findKeywordByName(name, g); id != "" {
				ids = append(ids, id)
			}
			continue
		}
		ids = append(ids, k.KeywordID)
	}
	if len(ids) == 0 {
		return
	}
	checked := params["kw_"+model.KWFieldAction]
	var keep []string
	for _, v := range checked {
		if strings.HasPrefix(v, "new:") {
			continue
		}
		keep = append(keep, v)
	}
	keep = append(keep, ids...)
	_ = h.kwRepo.ReplaceLinks(asID, model.KWFieldAction, keep, model.SplitCommaList(c.FormValue("kw_suggested_"+model.KWFieldAction)))
}

func countSavedActionSets(sets []model.ASActionSet) int {
	n := 0
	for _, s := range sets {
		if s.State == model.ActionSetStateSaved || s.State == model.ActionSetStateFinal {
			if !s.Virtual {
				n++
			}
		}
	}
	return n
}

func historyFromActionSets(sets []model.ASActionSet) []model.ASProcess {
	var out []model.ASProcess
	for _, s := range sets {
		if s.Virtual || s.State == model.ActionSetStateDraft || strings.TrimSpace(s.ResultCode) == "" {
			continue
		}
		p := model.ASProcess{
			WorkContent: s.ActionTaken,
			ResultCode:  s.ResultCode,
			Worker:      s.AssignedTo,
			WorkType:    s.ProcessType,
		}
		if s.LeaveDisplay() != "" {
			p.Notes = s.LeaveDisplay()
			if tw := s.TimeSummary(); tw != "" {
				p.Notes = tw
			}
		}
		out = append(out, p)
	}
	return out
}

func filterPhotosForSet(all []model.Attachment, set model.ASActionSet) []model.Attachment {
	var out []model.Attachment
	for _, a := range all {
		id := strings.TrimSpace(a.ActionSetID)
		if id == strings.TrimSpace(set.SetID) || (set.Seq == 1 && id == "") {
			out = append(out, a)
		}
	}
	return out
}

package handler

import (
	"strings"

	"customer-support/internal/model"
	"customer-support/internal/notify"
)

func clipCoopTitle(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > 30 {
		return string(r[:30])
	}
	return string(r)
}

func coopMailBody(kind, reqOrg, want string) string {
	return "요청 팀: " + reqOrg + "\n구분: " + model.CoopKindLabel(kind) + "\n희망 일정: " + want + "\n\n업무관리시스템 > 사업관리 > 사업·업무관리 > 받은/보낸 협조에서 확인하세요."
}

func (h *CoopHandler) notifyCoop(purpose, orgID, userID, title, body, actor string, item *model.Cooperation) {
	if h == nil || h.db == nil {
		return
	}
	u, _ := h.users.GetByID(userID)
	if u == nil || strings.TrimSpace(u.Email) == "" {
		if item != nil {
			h.repo.RecordEvent(item.CoopID, actor, "메일 미발송(주소 없음)", userID)
		}
		return
	}
	err := notify.Notify(h.db, h.settings, h.ncfg, notify.Message{
		Purpose: purpose, OrgID: orgID, ToUserID: u.UserID, ToEmail: u.Email,
		Title: title, Body: body, CreatedBy: actor,
	})
	if err != nil {
		if item != nil {
			h.repo.RecordEvent(item.CoopID, actor, "메일 큐 실패", "")
		}
	}
}

func (h *CoopHandler) mailRequested(item *model.Cooperation, actor string) {
	title := "[업무 협조] 요청이 도착했습니다 · " + clipCoopTitle(item.Title)
	body := coopMailBody(item.Kind, item.ReqOrgName, item.WantStart+" ~ "+item.WantEnd)
	seen := map[string]bool{}
	cands, _ := h.users.ListAssignable(item.TargetOrgID)
	for _, u := range cands {
		if model.NormalizeRole(u.Role) == model.RoleOrgAdmin || model.NormalizeRole(u.Role) == model.RoleAdmin {
			if seen[u.UserID] {
				continue
			}
			seen[u.UserID] = true
			h.notifyCoop(notify.PurposeCoopRequested, item.TargetOrgID, u.UserID, title, body, actor, item)
		}
	}
	if dep := h.repo.OrgDeputy(item.TargetOrgID); dep != "" && !seen[dep] {
		h.notifyCoop(notify.PurposeCoopRequested, item.TargetOrgID, dep, title, body, actor, item)
	}
}

func (h *CoopHandler) mailDecided(item *model.Cooperation, actor, verb string) {
	title := "[업무 협조] " + verb + " · " + clipCoopTitle(item.Title)
	body := coopMailBody(item.Kind, item.ReqOrgName, item.WantStart+" ~ "+item.WantEnd)
	if verb == "거부되었습니다" {
		body += "\n거부되었습니다. 사유는 시스템에서 확인하세요."
	}
	h.notifyCoop(notify.PurposeCoopDecided, item.ReqOrgID, item.ReqUserID, title, body, actor, item)
}

func (h *CoopHandler) mailAssigned(item *model.Cooperation, actor string, extra []model.CoopAssignee) {
	title := "[업무 협조] 업무가 배정되었습니다 · " + clipCoopTitle(item.Title)
	body := coopMailBody(item.Kind, item.ReqOrgName, item.PlanStart+" ~ "+item.PlanEnd)
	seen := map[string]bool{}
	for _, a := range extra {
		if seen[a.UserID] {
			continue
		}
		seen[a.UserID] = true
		b := body
		if a.Role == model.CoopAssignLead && item.WorkGroupID != "" {
			b += "\n그룹장"
		}
		h.notifyCoop(notify.PurposeCoopAssigned, item.TargetOrgID, a.UserID, title, b, actor, item)
	}
}

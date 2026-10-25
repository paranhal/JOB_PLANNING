package handler

import (
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func asReceiverIsSelf(c echo.Context, as *model.ASReceipt) bool {
	if as == nil {
		return false
	}
	uid := strings.TrimSpace(identityUserID(c))
	if uid != "" && uid == strings.TrimSpace(as.CreatedByUserID) {
		return true
	}
	name := currentUserDisplayName(c)
	if name != "" && name == strings.TrimSpace(as.ReceivedBy) {
		return true
	}
	return false
}

func asUntreatedForReceiver(as *model.ASReceipt, processN int) bool {
	if as == nil || isASClosedStatus(as.Status) || processN > 0 {
		return false
	}
	switch as.Status {
	case "received", "assigned":
		return true
	default:
		return false
	}
}

func canDeleteAS(c echo.Context, as *model.ASReceipt, processN int, canMod bool) bool {
	if as == nil || isReadOnly(c) || viewAsActive(c) {
		return false
	}
	if isVisionRole(c) {
		return true
	}
	if model.NormalizeRole(currentRole(c)) == model.RoleOrgAdmin && orgAdminInHome(c) {
		return true
	}
	if asReceiverIsSelf(c, as) {
		return asUntreatedForReceiver(as, processN)
	}
	return false
}

func (h *ASHandler) processCount(asID string) int {
	if h == nil || h.processRepo == nil {
		return 0
	}
	procs, err := h.processRepo.ListByAS(asID)
	if err != nil {
		return 0
	}
	return len(procs)
}

func (h *ASHandler) canDeleteThis(c echo.Context, as *model.ASReceipt) bool {
	if as == nil {
		return false
	}
	return canDeleteAS(c, as, h.processCount(as.ASID), h.canModifyAS(c, as))
}

func (h *ASHandler) markListCanDelete(c echo.Context, items []model.ASListItem) {
	if h == nil || h.repo == nil || len(items) == 0 {
		return
	}
	for i := range items {
		as, err := h.repo.GetByID(repository.OrgAll, items[i].ASID)
		if err != nil || as == nil {
			items[i].CanDelete = false
			items[i].DeleteDisabledReason = msgASDeleteGone
			continue
		}
		items[i].DeleteKey = bulkKeyAS + as.ASID
		reason := h.asListDeleteReason(c, as)
		if reason != "" {
			items[i].CanDelete = false
			items[i].DeleteDisabledReason = reason
			continue
		}
		items[i].CanDelete = true
	}
}

func (h *ASHandler) asListDeleteReason(c echo.Context, as *model.ASReceipt) string {
	if as == nil {
		return msgASDeleteGone
	}
	if isReadOnly(c) || viewAsActive(c) {
		return msgBulkReadOnly
	}
	org := currentOrg(c)
	if org != repository.OrgAll && strings.TrimSpace(as.OrgID) != "" && as.OrgID != org {
		return otherOrgReason(h.orgName(as.OrgID), as.OrgID)
	}
	if !h.canDeleteThis(c, as) {
		if isASClosedStatus(as.Status) && isAdminRole(c) && !h.canModifyAS(c, as) {
			return msgASDeleteLock
		}
		return msgASDeletePerm
	}
	if linked, _ := h.repo.HasLinkedAS(as.ASID); linked {
		return linkedASReason(h.repo.LinkedASNumbers(as.ASID))
	}
	return ""
}

func (h *ASHandler) orgName(orgID string) string {
	if h == nil || h.wbRepo == nil {
		return ""
	}
	return h.wbRepo.OrgName(orgID)
}

func linkedASReason(nums []string) string {
	if len(nums) == 0 {
		return msgASDeleteLinked
	}
	return msgASDeleteLinked + " (" + strings.Join(nums, ", ") + ")"
}

func otherOrgReason(orgName, orgID string) string {
	label := strings.TrimSpace(orgName)
	if label == "" {
		label = strings.TrimSpace(orgID)
	}
	if label == "" {
		return msgBulkOtherOrg
	}
	return msgBulkOtherOrg + " (" + label + ")"
}

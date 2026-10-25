package handler

import (
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
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
	closed := isASClosedStatus(as.Status)
	if isVisionRole(c) {
		return !closed || canMod
	}
	if model.NormalizeRole(currentRole(c)) == model.RoleOrgAdmin && orgAdminInHome(c) {
		return !closed || canMod
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
		as, err := h.repo.GetByID(currentOrg(c), items[i].ASID)
		if err != nil || as == nil {
			items[i].CanDelete = false
			continue
		}
		items[i].CanDelete = h.canDeleteThis(c, as)
	}
}

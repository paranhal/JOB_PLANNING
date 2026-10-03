package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func classifyHint(codes *repository.CodeRepo) string {
	if codes == nil {
		return ""
	}
	items, err := codes.ActiveByGroup(model.CodeGroupClassifyHint)
	if err != nil {
		return ""
	}
	for _, c := range items {
		if c.CodeValue == model.CodeValueClassifyBanner && strings.TrimSpace(c.CodeName) != "" {
			return c.CodeName
		}
	}
	return ""
}

func formAssetIDs(c echo.Context) []string {
	vals, err := c.FormParams()
	if err != nil {
		_ = c.Request().ParseForm()
		vals = c.Request().PostForm
	}
	seen := map[string]bool{}
	var out []string
	for _, id := range vals["asset_id"] {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func canMoveASToAdminWork(c echo.Context, as *model.ASReceipt) bool {
	if as == nil {
		return false
	}
	if as.Status == "cancelled" || as.Status == model.StatusAdminWork || strings.TrimSpace(as.MovedTaskID) != "" {
		return false
	}
	return canProcessAS(c) || isAdminRole(c)
}

func (h *ASHandler) ToAdminWork(c echo.Context) error {
	if !(canProcessAS(c) || isAdminRole(c)) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(currentOrg(c), id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if !canMoveASToAdminWork(c, as) {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err=move_admin")
	}
	reason := strings.TrimSpace(c.FormValue("reason"))
	_, err = h.repo.MoveToAdminWork(id, reason)
	if err != nil {
		code := "move_admin"
		if errors.Is(err, repository.ErrASMoveReason) {
			code = "move_reason"
		} else if errors.Is(err, repository.ErrASAlreadyMoved) {
			code = "move_done"
		}
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err="+code)
	}
	h.syncASPlannedDailyTask(id)
	return c.Redirect(http.StatusSeeOther, "/as/"+id+"?ok=moved")
}

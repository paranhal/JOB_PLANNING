package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *WorkboardHandler) persistNewTask(c echo.Context, t *model.WorkTask) (bool, error) {
	if t == nil {
		return false, echo.NewHTTPError(http.StatusBadRequest, "업무가 없습니다")
	}
	if strings.TrimSpace(c.FormValue("split_mode")) != "each" || !canSplitWork(t) {
		return false, h.repo.CreateTask(t)
	}
	targets, names, inherit := splitRequest(c)
	if len(targets) < 2 {
		return false, h.repo.CreateTask(t)
	}
	made, err := h.repo.SplitByTargets(t, targets, names, inherit)
	if err != nil {
		return false, err
	}
	if len(made) > 0 && made[0] != nil {
		*t = *made[0]
	}
	return true, nil
}

func canSplitWork(t *model.WorkTask) bool {
	if t == nil {
		return false
	}
	switch t.WorkType {
	case model.WBWorkAS, model.WBWorkMaintenance:
		return false
	}
	switch strings.TrimSpace(t.SourceType) {
	case model.WBSourceAS, model.WBSourceMaintenance:
		return false
	}
	if t.Status == model.WBTaskComplete || t.RecurrenceRole == model.RecurrenceRoleParent {
		return false
	}
	return true
}

func splitRequest(c echo.Context) ([]repository.SplitTarget, map[string]string, repository.SplitLinks) {
	_ = c.Request().ParseForm()
	form := c.Request().Form
	assets := formAssetIDs(c)
	var sites []string
	seenSite := map[string]bool{}
	for _, id := range form["site_id"] {
		id = strings.TrimSpace(id)
		if id == "" || seenSite[id] {
			continue
		}
		seenSite[id] = true
		sites = append(sites, id)
	}
	kind := strings.TrimSpace(c.FormValue("split_kind"))
	if kind != "asset" && kind != "site" {
		if len(assets) >= 2 {
			kind = "asset"
		} else if len(sites) >= 2 {
			kind = "site"
		}
	}
	var ids []string
	if kind == "site" {
		ids = sites
	} else {
		kind = "asset"
		ids = assets
	}
	names := map[string]string{}
	var targets []repository.SplitTarget
	for _, id := range ids {
		label := strings.TrimSpace(c.FormValue("split_label_" + kind + "_" + id))
		targets = append(targets, repository.SplitTarget{Kind: kind, ID: id, Name: label})
		if title := strings.TrimSpace(c.FormValue("split_title_" + kind + "_" + id)); title != "" {
			names[kind+":"+id] = title
		}
	}
	return targets, names, repository.SplitLinks{Assets: assets, Sites: sites}
}

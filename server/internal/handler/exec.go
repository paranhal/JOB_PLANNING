package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type ExecHandler struct {
	orgs *repository.OrgRepo
}

func NewExecHandler(orgs *repository.OrgRepo) *ExecHandler {
	return &ExecHandler{orgs: orgs}
}

func canViewExec(c echo.Context) bool {
	r := currentRole(c)
	return r == model.RoleCLevel || r == model.RoleVisionAdmin
}

func (h *AuthHandler) RequireExecView(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !canViewExec(c) {
			return h.forbidden(c)
		}
		return next(c)
	}
}

func (h *AuthHandler) GuardCLevelHidden(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if currentRole(c) != model.RoleCLevel {
			return next(c)
		}
		path := c.Request().URL.Path
		method := c.Request().Method
		if method != http.MethodGet && method != http.MethodHead {
			if cLevelWriteExempt(path) {
				return next(c)
			}
			return h.forbidden(c)
		}
		if cLevelHiddenPath(path) {
			return h.forbidden(c)
		}
		return next(c)
	}
}

func (h *ExecHandler) listOrgs() []model.Org {
	if h == nil || h.orgs == nil {
		return nil
	}
	list, err := h.orgs.ListActive()
	if err != nil {
		return nil
	}
	return list
}

func (h *ExecHandler) Dashboard(c echo.Context) error {
	return c.Render(http.StatusOK, "exec/dashboard.html", map[string]interface{}{
		"Title":  "경영 통합 대시보드",
		"Active": NavExec,
		"Orgs":   h.listOrgs(),
	})
}

func (h *ExecHandler) Work(c echo.Context) error {
	biz, _ := splitExecOrgs(h.listOrgs())
	orgID := strings.TrimSpace(c.QueryParam("org"))
	var cur *model.Org
	for i := range biz {
		if orgID == "" {
			cur = &biz[0]
			break
		}
		if biz[i].OrgID == orgID {
			cur = &biz[i]
			break
		}
	}
	if orgID != "" && cur == nil {
		return echo.ErrNotFound
	}
	if cur == nil {
		return c.Render(http.StatusOK, "exec/work.html", map[string]interface{}{
			"Title":  "업무 관리",
			"Active": NavExecWork,
			"Orgs":   biz,
		})
	}
	return c.Render(http.StatusOK, "exec/work.html", map[string]interface{}{
		"Title":   "업무 관리",
		"Active":  NavExecWork,
		"Orgs":    biz,
		"Org":     cur,
		"OrgID":   cur.OrgID,
		"OrgName": cur.OrgName,
	})
}

func (h *ExecHandler) OrgSoon(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if h.orgs == nil {
		return echo.ErrNotFound
	}
	o, err := h.orgs.Get(id)
	if err != nil || o == nil || !o.IsActive {
		return echo.ErrNotFound
	}
	if model.NormalizeOrgKind(o.Kind) != model.OrgKindSupport {
		return echo.ErrNotFound
	}
	return c.Render(http.StatusOK, "exec/org_soon.html", map[string]interface{}{
		"Title":  o.NavLabel() + " 대시보드",
		"Active": NavExecOrg,
		"Org":    o,
	})
}

package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type ExecHandler struct {
	orgs *repository.OrgRepo
	dash *repository.ExecDashRepo
}

func NewExecHandler(orgs *repository.OrgRepo, dash *repository.ExecDashRepo) *ExecHandler {
	return &ExecHandler{orgs: orgs, dash: dash}
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
		if cLevelWriteBlocked(method, path) {
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

func execDashDate(c echo.Context) string {
	if d := strings.TrimSpace(c.QueryParam("date")); d != "" {
		if t, err := time.ParseInLocation("2006-01-02", d, time.Local); err == nil {
			return t.Format("2006-01-02")
		}
	}
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location()).Format("2006-01-02")
}

func execScopeOrgIDs(c echo.Context) []string {
	s := DataScopeFor(c)
	if s.All {
		return nil
	}
	return s.OrgIDs
}

func (h *ExecHandler) dashData(c echo.Context) (map[string]interface{}, error) {
	date := execDashDate(c)
	var orgs []repository.ExecOrgToday
	var tot repository.ExecTotals
	var err error
	if h != nil && h.dash != nil {
		orgs, tot, err = h.dash.Today(date, execScopeOrgIDs(c))
		if err != nil {
			return nil, err
		}
	}
	wd := []string{"일", "월", "화", "수", "목", "금", "토"}
	t, _ := time.ParseInLocation("2006-01-02", date, time.Local)
	dateLabel := date
	if !t.IsZero() {
		dateLabel = t.Format("2006.01.02") + " (" + wd[int(t.Weekday())] + ")"
	}
	return map[string]interface{}{
		"Title":      "경영 통합 대시보드",
		"Active":     NavExec,
		"Date":       date,
		"DateLabel":  dateLabel,
		"RoleLabel":  model.RoleLabel(currentRole(c)),
		"Orgs":   orgs,
		"Totals": tot,
	}, nil
}

func (h *ExecHandler) Dashboard(c echo.Context) error {
	data, err := h.dashData(c)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "exec/dashboard.html", data)
}

func (h *ExecHandler) Cards(c echo.Context) error {
	data, err := h.dashData(c)
	if err != nil {
		return err
	}
	return RenderPartial(c, "exec/_org_cards.html", data)
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

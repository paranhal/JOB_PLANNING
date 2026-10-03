package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type OrgHandler struct {
	repo     *repository.OrgRepo
	auth     *AuthHandler
	db       *sql.DB
	dataDir  string
	settings *repository.SettingsRepo
	users    *repository.UserRepo
}

func NewOrgHandler(db *sql.DB, repo *repository.OrgRepo, auth *AuthHandler, settings *repository.SettingsRepo, users *repository.UserRepo, dataDir string) *OrgHandler {
	return &OrgHandler{db: db, repo: repo, auth: auth, settings: settings, users: users, dataDir: dataDir}
}

func canSwitchOrg(c echo.Context) bool {
	role := loginRole(c)
	return role == model.RoleOrgAdmin || role == model.RoleVisionAdmin
}

func currentOrg(c echo.Context) string {
	if strings.TrimSpace(ctxString(c, "sim_role")) != "" {
		if currentRole(c) == model.RoleVisionAdmin {
			return repository.OrgAll
		}
		if sim := strings.TrimSpace(ctxString(c, "sim_org_id")); sim != "" {
			return sim
		}
		return model.OrgIDLibrary
	}
	if canSwitchOrg(c) {
		view := strings.TrimSpace(ctxString(c, "view_org_id"))
		if view == repository.OrgAll {
			return repository.OrgAll
		}
		if view != "" {
			return view
		}
	}
	if loginRole(c) == model.RoleVisionAdmin {
		return repository.OrgAll
	}
	org := strings.TrimSpace(ctxString(c, "org_id"))
	if org == "" {
		return model.OrgIDLibrary
	}
	return org
}

func orgViewLabel(c echo.Context, orgs []model.Org) string {
	id := currentOrg(c)
	if id == repository.OrgAll {
		return "전 조직"
	}
	for _, o := range orgs {
		if o.OrgID == id {
			if strings.TrimSpace(o.ShortName) != "" {
				return o.ShortName
			}
			return o.OrgName
		}
	}
	if id == model.OrgIDLibrary {
		return model.OrgNameLibrary
	}
	return id
}

func (h *OrgHandler) List(c echo.Context) error {
	items, err := h.repo.List()
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "admin/orgs.html", map[string]interface{}{
		"Title":    "조직 관리",
		"Active":   NavOrgs,
		"Items":    items,
		"FlashOK":  strings.TrimSpace(c.QueryParam("ok")),
		"FlashErr": strings.TrimSpace(c.QueryParam("err")),
		"EditID":   strings.TrimSpace(c.QueryParam("edit")),
	})
}

func (h *OrgHandler) Create(c echo.Context) error {
	o, err := h.repo.Create(c.FormValue("org_name"), c.FormValue("short_name"), c.FormValue("note"), ctxString(c, "user_name"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs?err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/orgs?ok="+url.QueryEscape(o.OrgNo+" "+o.OrgName+"을 만들었습니다."))
}

func (h *OrgHandler) Update(c echo.Context) error {
	id := strings.TrimSpace(c.FormValue("org_id"))
	active := c.FormValue("is_active") == "1" || c.FormValue("is_active") == "on"
	if err := h.repo.Update(id, c.FormValue("org_name"), c.FormValue("short_name"), c.FormValue("note"), active); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs?err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/orgs?ok="+url.QueryEscape("조직을 수정했습니다."))
}

func (h *OrgHandler) Switch(c echo.Context) error {
	if !canSwitchOrg(c) {
		return echo.ErrForbidden
	}
	view := strings.TrimSpace(c.FormValue("view_org_id"))
	if view == "" {
		view = repository.OrgAll
	}
	if view != repository.OrgAll {
		got, err := h.repo.Get(view)
		if err != nil || got == nil || !got.IsActive {
			return c.Redirect(http.StatusSeeOther, "/admin/orgs?err="+url.QueryEscape("없는 조직입니다"))
		}
	}
	c.Set("view_org_id", view)
	unconfirmed := false
	if v := c.Get("auth_unconfirmed"); v != nil {
		if b, ok := v.(bool); ok {
			unconfirmed = b
		}
	}
	if h.auth != nil {
		h.auth.writeSessionCookie(c, sessionClaimsFromContext(c, unconfirmed))
	}
	back := strings.TrimSpace(c.FormValue("return"))
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		back = "/admin/orgs"
	}
	return c.Redirect(http.StatusSeeOther, back)
}

// RequireVisionOnly 비젼관리자만.
func (h *AuthHandler) RequireVisionOnly(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if currentRole(c) != model.RoleVisionAdmin {
			return echo.ErrForbidden
		}
		return next(c)
	}
}

package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
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

func canViewOrgs(c echo.Context) bool {
	return hasPerm(c, model.PermOrgView)
}

func canWriteOrgs(c echo.Context) bool {
	return currentRole(c) == model.RoleVisionAdmin
}

func orgListHomeID(c echo.Context) string {
	if currentRole(c) != model.RoleOrgAdmin {
		return ""
	}
	id := strings.TrimSpace(ctxString(c, "org_id"))
	if id == "" {
		return model.OrgIDLibrary
	}
	return id
}

func canSwitchOrg(c echo.Context) bool {
	return currentRole(c) == model.RoleVisionAdmin
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
	if !canViewOrgs(c) {
		return echo.ErrForbidden
	}
	items, err := h.repo.List()
	if err != nil {
		return err
	}
	if home := orgListHomeID(c); home != "" {
		own := items[:0]
		for _, it := range items {
			if it.OrgID == home {
				own = append(own, it)
			}
		}
		items = own
	}
	users, _ := h.users.ListAll()
	managers := map[string][]model.User{}
	candidates := map[string][]model.User{}
	activeOrgs, totalUsers := 0, 0
	for _, it := range items {
		if it.IsActive {
			activeOrgs++
		}
		totalUsers += it.UserCount
	}
	for _, u := range users {
		if !u.IsActive {
			continue
		}
		if u.Role == model.RoleOrgAdmin || u.Role == "admin" {
			managers[u.OrgID] = append(managers[u.OrgID], u)
		}
		if !model.IsAdminGrade(u.Role) {
			candidates[u.OrgID] = append(candidates[u.OrgID], u)
		}
	}
	return c.Render(http.StatusOK, "admin/orgs.html", map[string]interface{}{
		"Title":        "전체 조직 관리",
		"Active":       NavOrgs,
		"Items":        items,
		"Managers":     managers,
		"Candidates":   candidates,
		"OrgCount":     len(items),
		"ActiveCount":  activeOrgs,
		"UserCount":    totalUsers,
		"FlashOK":      strings.TrimSpace(c.QueryParam("ok")),
		"FlashErr":     strings.TrimSpace(c.QueryParam("err")),
		"EditID":       strings.TrimSpace(c.QueryParam("edit")),
		"AssignID":     strings.TrimSpace(c.QueryParam("assign")),
		"RevokeID":     strings.TrimSpace(c.QueryParam("revoke")),
		"CanWriteOrgs": canWriteOrgs(c),
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

func (h *OrgHandler) AssignManager(c echo.Context) error {
	if !canWriteOrgs(c) {
		return echo.ErrForbidden
	}
	orgID := strings.TrimSpace(c.Param("id"))
	uid := strings.TrimSpace(c.FormValue("user_id"))
	u, _ := h.users.GetByID(uid)
	if u == nil || u.OrgID != orgID || !u.IsActive || model.IsAdminGrade(u.Role) {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs?err="+url.QueryEscape("이 조직의 활성 일반 이용자만 지정할 수 있습니다."))
	}
	before := u.Role
	u.Role = model.RoleOrgAdmin
	if err := h.users.Update(u); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs?err="+url.QueryEscape("지정하지 못했습니다."))
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
		TargetTable: "users", TargetID: u.UserID, SubjectType: "user",
		SubjectID: u.UserID, SubjectName: u.FullName,
		Detail: "조직관리자 지정 " + orgID + " · 업무 " + model.RoleLabel(before) + " → 없음",
	})
	return c.Redirect(http.StatusSeeOther, "/admin/orgs?ok="+url.QueryEscape(u.FullName+"을 조직관리자로 지정했습니다."))
}

func (h *OrgHandler) RevokeManager(c echo.Context) error {
	if !canWriteOrgs(c) {
		return echo.ErrForbidden
	}
	orgID := strings.TrimSpace(c.Param("id"))
	job := model.NormalizeRole(c.FormValue("job"))
	if job != model.RoleSales && job != model.RoleTech && job != model.RoleSupport {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs?err="+url.QueryEscape("박탈 후 업무를 고르세요."))
	}
	uid := strings.TrimSpace(c.FormValue("user_id"))
	all, _ := h.users.ListAll()
	var n int
	var names []string
	for i := range all {
		u := all[i]
		if u.OrgID != orgID || (u.Role != model.RoleOrgAdmin && u.Role != "admin") {
			continue
		}
		if uid != "" && u.UserID != uid {
			continue
		}
		u.Role = job
		if err := h.users.Update(&u); err != nil {
			continue
		}
		n++
		names = append(names, u.FullName)
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
			TargetTable: "users", TargetID: u.UserID, SubjectType: "user",
			SubjectID: u.UserID, SubjectName: u.FullName,
			Detail: "조직관리자 박탈 " + orgID + " → " + model.RoleLabel(job),
		})
	}
	if n == 0 {
		return c.Redirect(http.StatusSeeOther, "/admin/orgs?err="+url.QueryEscape("박탈할 조직관리자가 없습니다."))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/orgs?ok="+url.QueryEscape(strings.Join(names, ", ")+"의 권한을 박탈했습니다."))
}

// RequireCanViewOrgs 조직 목록 조회. 권한표 org.view (§53.4.1 · §52.5.1).
func (h *AuthHandler) RequireCanViewOrgs(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !canViewOrgs(c) {
			return echo.ErrForbidden
		}
		return next(c)
	}
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

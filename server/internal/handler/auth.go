package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

type AuthHandler struct {
	userRepo     *repository.UserRepo
	orgRepo      *repository.OrgRepo
	settingsRepo *repository.SettingsRepo
	adminUnlock  *repository.AdminUnlockRepo
	jwtSecret    []byte
}

func (h *AuthHandler) LoginPage(c echo.Context) error {
	return c.Render(http.StatusOK, "auth/login.html", map[string]interface{}{
		"Title": "로그인", "Active": NavLogin, "HideNav": true,
	})
}

func (h *AuthHandler) Login(c echo.Context) error {
	username := strings.TrimSpace(c.FormValue("username"))
	password := c.FormValue("password")

	fail := func(user *model.User, reason string) error {
		rec := auditlog.Record{
			Action:   auditlog.ActionLoginFail,
			Result:   auditlog.ResultDeny,
			Reason:   reason,
			Detail:   "로그인 실패",
			Username: username,
		}
		if user != nil {
			rec.UserID = user.UserID
			rec.Username = user.Username
			rec.FullName = user.FullName
			rec.Role = user.Role
		}
		accessLog(c, rec)
		return c.Render(http.StatusOK, "auth/login.html", map[string]interface{}{
			"Title": "로그인", "Active": NavLogin, "HideNav": true,
			"Error": "아이디 또는 비밀번호가 잘못되었습니다.",
		})
	}

	user, err := h.userRepo.GetByUsername(username)
	if err != nil {
		log.Printf("로그인 사용자 조회 실패 username=%q: %v", username, err)
		accessLog(c, auditlog.Record{
			Action:   auditlog.ActionLoginFail,
			Result:   auditlog.ResultDeny,
			Reason:   "시스템오류",
			Detail:   "로그인 조회 실패",
			Username: username,
		})
		return c.Render(http.StatusOK, "auth/login.html", map[string]interface{}{
			"Title": "로그인", "Active": NavLogin, "HideNav": true,
			"Error": "일시적으로 로그인할 수 없습니다. 서버를 재시작한 뒤 다시 시도하세요.",
		})
	}
	if user == nil {
		return fail(nil, "비밀번호오류")
	}
	if !user.IsActive {
		return fail(user, "권한없음")
	}
	if !verifyPassword(user.PasswordHash, password) {
		return fail(user, "비밀번호오류")
	}
	upgradeLegacyPassword(user.PasswordHash, password, func(nh string) error {
		return h.userRepo.UpdatePassword(user.UserID, nh)
	})

	tokenStr := h.writeSessionCookie(c, sessionClaims(user, false))
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionLogin,
		Result:      auditlog.ResultOK,
		UserID:      user.UserID,
		Username:    user.Username,
		FullName:    user.FullName,
		Role:        user.Role,
		SessionID:   auditlog.SessionFingerprint(tokenStr),
		Detail:      "로그인",
		TargetTable: "users",
		TargetID:    user.UserID,
	})
	if user.NeedsProfileFill() {
		return c.Redirect(http.StatusSeeOther, "/account/complete")
	}
	return c.Redirect(http.StatusSeeOther, "/")
}

func (h *AuthHandler) Logout(c echo.Context) error {
	rec := auditlog.Record{
		Action: auditlog.ActionLogout,
		Result: auditlog.ResultOK,
		Detail: "로그아웃",
	}
	h.fillActorFromTokenCookie(c, &rec)
	accessLog(c, rec)
	c.SetCookie(&http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	return c.Redirect(http.StatusSeeOther, "/login")
}

// AccountPage 내 계정(프로필·비밀번호 변경)
func (h *AuthHandler) AccountPage(c echo.Context) error {
	uid := ctxString(c, "user_id")
	u, _ := h.userRepo.GetByID(uid)
	if u == nil {
		return c.Redirect(http.StatusSeeOther, "/logout")
	}
	msg := ""
	if c.QueryParam("ok") == "password" {
		msg = "비밀번호가 변경되었습니다."
	} else if c.QueryParam("ok") == "profile" {
		msg = "계정 정보가 저장되었습니다."
	}
	var orgs []model.Org
	if h.orgRepo != nil {
		orgs, _ = h.orgRepo.ListActive()
	}
	return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
		"Title": "내 계정", "Active": NavAccount, "User": u, "OK": msg, "Orgs": orgs,
	})
}

func (h *AuthHandler) AccountUpdateProfile(c echo.Context) error {
	uid := ctxString(c, "user_id")
	u, _ := h.userRepo.GetByID(uid)
	if u == nil {
		return c.Redirect(http.StatusSeeOther, "/logout")
	}
	name := strings.TrimSpace(c.FormValue("full_name"))
	if name == "" {
		return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
			"Title": "내 계정", "Active": NavAccount, "User": u,
			"Error": "이름을 입력하세요.",
		})
	}
	u.FullName = name
	bindUserContact(c, u)
	if err := u.ProfileFieldError(); err != "" {
		return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
			"Title": "내 계정", "Active": NavAccount, "User": u, "Error": err,
		})
	}
	h.userRepo.Update(u)
	h.refreshSession(c, u)
	return c.Redirect(http.StatusSeeOther, "/account?ok=profile")
}

func (h *AuthHandler) AccountCompleteForm(c echo.Context) error {
	u, err := h.currentAccountUser(c)
	if err != nil {
		return err
	}
	if !u.NeedsProfileFill() {
		return c.Redirect(http.StatusSeeOther, "/")
	}
	return h.renderProfileComplete(c, u, "")
}

func (h *AuthHandler) AccountComplete(c echo.Context) error {
	u, err := h.currentAccountUser(c)
	if err != nil {
		return err
	}
	miss := u.MissingProfileFields()
	for _, f := range miss {
		switch f {
		case "mobile":
			u.Mobile = strings.TrimSpace(c.FormValue("mobile"))
		case "email":
			u.Email = strings.TrimSpace(c.FormValue("email"))
		case "org":
			u.OrgID = strings.TrimSpace(c.FormValue("org_id"))
		}
	}
	if msg := u.ProfileFieldError(); msg != "" {
		return h.renderProfileComplete(c, u, msg)
	}
	u.ProfileDone = true
	if err := h.userRepo.Update(u); err != nil {
		return err
	}
	h.refreshSession(c, u)
	return c.Redirect(http.StatusSeeOther, "/")
}

func (h *AuthHandler) currentAccountUser(c echo.Context) (*model.User, error) {
	uid := ctxString(c, "user_id")
	u, _ := h.userRepo.GetByID(uid)
	if u == nil {
		return nil, c.Redirect(http.StatusSeeOther, "/logout")
	}
	return u, nil
}

func (h *AuthHandler) renderProfileComplete(c echo.Context, u *model.User, errMsg string) error {
	var orgs []model.Org
	if h.orgRepo != nil {
		orgs, _ = h.orgRepo.ListActive()
	}
	miss := map[string]bool{}
	for _, f := range u.MissingProfileFields() {
		miss[f] = true
	}
	return c.Render(http.StatusOK, "auth/profile_complete.html", map[string]interface{}{
		"Title": "내 정보 채우기", "Active": NavAccount, "HideNav": true, "User": u,
		"NeedMobile": miss["mobile"], "NeedEmail": miss["email"], "NeedOrg": miss["org"],
		"Orgs": orgs, "Error": errMsg,
	})
}

func bindUserContact(c echo.Context, u *model.User) {
	if u == nil {
		return
	}
	u.Mobile = strings.TrimSpace(c.FormValue("mobile"))
	u.Tel = strings.TrimSpace(c.FormValue("tel"))
	u.Email = strings.TrimSpace(c.FormValue("email"))
}

func (h *AuthHandler) AccountChangePassword(c echo.Context) error {
	uid := ctxString(c, "user_id")
	u, _ := h.userRepo.GetByID(uid)
	if u == nil {
		return c.Redirect(http.StatusSeeOther, "/logout")
	}
	current := c.FormValue("current_password")
	pw := strings.TrimSpace(c.FormValue("password"))
	confirm := strings.TrimSpace(c.FormValue("password_confirm"))
	renderErr := func(msg string) error {
		return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
			"Title": "내 계정", "Active": NavAccount, "User": u, "Error": msg,
		})
	}
	if !verifyPassword(u.PasswordHash, current) {
		return renderErr("현재 비밀번호가 올바르지 않습니다.")
	}
	if pw == "" {
		return renderErr("새 비밀번호를 입력하세요.")
	}
	if len(pw) < 4 {
		return renderErr("비밀번호는 4자 이상이어야 합니다.")
	}
	if pw != confirm {
		return renderErr("새 비밀번호와 확인 입력이 일치하지 않습니다.")
	}
	if verifyPassword(u.PasswordHash, pw) {
		return renderErr("새 비밀번호는 현재 비밀번호와 달라야 합니다.")
	}
	nh := HashPassword(pw)
	if nh == "" {
		return renderErr("비밀번호를 저장하지 못했습니다.")
	}
	h.userRepo.UpdatePassword(u.UserID, nh)
	u.PasswordHash = nh
	h.refreshSession(c, u)
	return c.Redirect(http.StatusSeeOther, "/account?ok=password")
}

func (h *AuthHandler) refreshSession(c echo.Context, user *model.User) {
	claims := sessionClaims(user, false)
	if v := strings.TrimSpace(ctxString(c, "view_org_id")); v != "" {
		claims["view_org_id"] = v
	}
	h.writeSessionCookie(c, claims)
}

func (h *AuthHandler) isAdmin(c echo.Context) bool {
	return isAdminRole(c)
}

func (h *AuthHandler) forbidden(c echo.Context) error {
	return c.Render(http.StatusForbidden, "auth/forbidden.html", map[string]interface{}{
		"Title": "접근 권한 없음", "Active": NavNone,
	})
}

// UserList 사용자 관리 화면 (옵저버는 조회만)
func (h *AuthHandler) UserList(c echo.Context) error {
	if !h.isAdmin(c) && !isObserverRole(c) && !hasPerm(c, model.PermCodesUsers) {
		return h.forbidden(c)
	}
	users, _ := h.userRepo.ListAll()
	msg := c.QueryParam("ok")
	switch msg {
	case "password":
		msg = "비밀번호가 변경되었습니다."
	case "saved":
		msg = "사용자 정보가 저장되었습니다."
	case "reset_perms":
		msg = "직급 기본 권한으로 되돌렸습니다."
	}
	errMsg := c.QueryParam("err")
	canEdit := h.isAdmin(c) || hasPerm(c, model.PermCodesUsers)
	var orgs []model.Org
	if h.orgRepo != nil {
		orgs, _ = h.orgRepo.ListActive()
	}
	data := map[string]interface{}{
		"Title": "사용자 관리", "Active": NavUsers, "Users": users, "OK": msg, "Error": errMsg,
		"PermDefs": model.AllPermissions, "CanEditUsers": canEdit,
		"RoleGroups": model.GroupUsersByRole(users), "Orgs": orgs,
	}
	if canEdit {
		data["HashMig"] = h.passwordMigrationStatus()
	}
	return c.Render(http.StatusOK, "auth/users.html", data)
}

func (h *AuthHandler) passwordMigrationStatus() map[string]interface{} {
	total, bcryptN := 0, 0
	if h.userRepo != nil {
		total, bcryptN, _ = h.userRepo.PasswordHashStats()
	}
	asHash, delHash := "", ""
	if h.settingsRepo != nil {
		asHash, _ = h.settingsRepo.Get(repository.SettingASCompletedEditPassword)
		delHash, _ = h.settingsRepo.Get(repository.SettingMaintenanceDeletePassword)
	}
	asBcrypt := passwd.IsBcrypt(asHash)
	delBcrypt := passwd.IsBcrypt(delHash)
	legacy := total - bcryptN
	return map[string]interface{}{
		"UserTotal":  total,
		"UserBcrypt": bcryptN,
		"UserLegacy": legacy,
		"ASSet":      strings.TrimSpace(asHash) != "",
		"ASBcrypt":   asBcrypt,
		"DelSet":     strings.TrimSpace(delHash) != "",
		"DelBcrypt":  delBcrypt,
		"Complete":   total > 0 && legacy == 0 && asBcrypt && delBcrypt,
	}
}

func (h *AuthHandler) UserCreate(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	role := model.NormalizeRole(c.FormValue("role"))
	baseRole := model.NormalizeRole(c.FormValue("base_role"))
	if role != model.RoleTester {
		baseRole = ""
	}
	form, _ := c.FormParams()
	var selected []string
	if form != nil {
		selected = form["perm"]
	}
	perms := model.CompactStoredPermissions(model.EffectiveRole(role, baseRole), selected)
	u := &model.User{
		Username:     strings.TrimSpace(c.FormValue("username")),
		PasswordHash: HashPassword(c.FormValue("password")),
		FullName:     strings.TrimSpace(c.FormValue("full_name")),
		Role:         role,
		BaseRole:     baseRole,
		Permissions:  perms,
		IsActive:     true,
		OrgID:        strings.TrimSpace(c.FormValue("org_id")),
	}
	bindUserContact(c, u)
	if msg := u.ProfileFieldError(); msg != "" {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(msg))
	}
	if err := h.userRepo.Create(u); err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("계정을 만들지 못했습니다."))
	}
	return c.Redirect(http.StatusSeeOther, "/users")
}

func (h *AuthHandler) UserUpdate(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	u, _ := h.userRepo.GetByID(c.Param("id"))
	if u == nil {
		return echo.ErrNotFound
	}
	before := userPublic(u)
	wasActive := u.IsActive
	u.FullName = c.FormValue("full_name")
	prevRole := model.NormalizeRole(c.FormValue("prev_role"))
	u.Role = model.NormalizeRole(c.FormValue("role"))
	if u.Role == model.RoleTester {
		u.BaseRole = model.NormalizeRole(c.FormValue("base_role"))
	} else {
		u.BaseRole = ""
	}
	u.OrgID = strings.TrimSpace(c.FormValue("org_id"))
	bindUserContact(c, u)
	if msg := u.ProfileFieldError(); msg != "" {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(msg))
	}
	if prevRole != "" && prevRole != u.Role && c.FormValue("clear_custom_perms") == "1" {
		u.Permissions = ""
	} else {
		form, _ := c.FormParams()
		var selected []string
		if form != nil {
			selected = form["perm"]
		}
		u.Permissions = model.CompactStoredPermissions(model.EffectiveRole(u.Role, u.BaseRole), selected)
	}
	u.IsActive = c.FormValue("is_active") != "0"
	h.userRepo.Update(u)
	if wasActive && !u.IsActive {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionDelete,
			TargetTable: "users",
			TargetID:    u.UserID,
			SubjectType: "user",
			SubjectID:   u.UserID,
			SubjectName: u.FullName,
			Detail:      "사용자 비활성",
			Reason:      accessReason(c, "사용자 비활성"),
			Result:      auditlog.ResultOK,
			BeforeJSON:  toJSON(before),
			AfterJSON:   toJSON(userPublic(u)),
		})
	}

	if pw := strings.TrimSpace(c.FormValue("password")); pw != "" {
		confirm := strings.TrimSpace(c.FormValue("password_confirm"))
		if confirm != "" && pw != confirm {
			users, _ := h.userRepo.ListAll()
			return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
				"Title": "사용자 관리", "Active": NavUsers, "Users": users,
				"Error": "비밀번호와 확인 입력이 일치하지 않습니다.", "PermDefs": model.AllPermissions, "CanEditUsers": true,
				"RoleGroups": model.GroupUsersByRole(users),
			})
		}
		if len(pw) < 4 {
			users, _ := h.userRepo.ListAll()
			return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
				"Title": "사용자 관리", "Active": NavUsers, "Users": users,
				"Error": "비밀번호는 4자 이상이어야 합니다.", "PermDefs": model.AllPermissions, "CanEditUsers": true,
				"RoleGroups": model.GroupUsersByRole(users),
			})
		}
		h.userRepo.UpdatePassword(u.UserID, HashPassword(pw))
		return c.Redirect(http.StatusSeeOther, "/users?ok=password")
	}
	return c.Redirect(http.StatusSeeOther, "/users?ok=saved")
}

func (h *AuthHandler) UserResetPermissions(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	u, _ := h.userRepo.GetByID(c.Param("id"))
	if u == nil {
		return echo.ErrNotFound
	}
	u.Permissions = ""
	if err := h.userRepo.Update(u); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/users?ok=reset_perms")
}

// UserChangePassword 관리자: 다른 사용자 비밀번호 변경
func (h *AuthHandler) UserChangePassword(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	u, _ := h.userRepo.GetByID(c.Param("id"))
	if u == nil {
		return echo.ErrNotFound
	}
	pw := strings.TrimSpace(c.FormValue("password"))
	confirm := strings.TrimSpace(c.FormValue("password_confirm"))
	users, _ := h.userRepo.ListAll()
	if pw == "" {
		return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
			"Title": "사용자 관리", "Active": NavUsers, "Users": users,
			"Error": "새 비밀번호를 입력하세요.", "FocusUser": u.UserID, "PermDefs": model.AllPermissions, "CanEditUsers": true,
			"RoleGroups": model.GroupUsersByRole(users),
		})
	}
	if pw != confirm {
		return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
			"Title": "사용자 관리", "Active": NavUsers, "Users": users,
			"Error": "비밀번호와 확인 입력이 일치하지 않습니다.", "FocusUser": u.UserID, "PermDefs": model.AllPermissions, "CanEditUsers": true,
			"RoleGroups": model.GroupUsersByRole(users),
		})
	}
	if len(pw) < 4 {
		return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
			"Title": "사용자 관리", "Active": NavUsers, "Users": users,
			"Error": "비밀번호는 4자 이상이어야 합니다.", "FocusUser": u.UserID, "PermDefs": model.AllPermissions, "CanEditUsers": true,
			"RoleGroups": model.GroupUsersByRole(users),
		})
	}
	h.userRepo.UpdatePassword(u.UserID, HashPassword(pw))
	return c.Redirect(http.StatusSeeOther, "/users?ok=password")
}

// AuthMiddleware JWT 인증 미들웨어
func (h *AuthHandler) AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		path := c.Request().URL.Path
		if path == "/login" || path == "/version" || strings.HasPrefix(path, "/static") {
			return next(c)
		}

		cookie, err := c.Cookie("token")
		if err != nil || cookie.Value == "" {
			return c.Redirect(http.StatusSeeOther, "/login")
		}

		token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return h.jwtSecret, nil
		})
		if err != nil {
			log.Printf("JWT 파싱 오류: %v", err)
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		if !token.Valid {
			return c.Redirect(http.StatusSeeOther, "/login")
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		role := applySessionClaims(c, claims)
		if !model.IsKnownRole(role) {
			return c.Redirect(http.StatusSeeOther, "/login")
		}

		uid := ctxString(c, "user_id")
		if uid != "" && needRevalidate(claims) && h.userRepo != nil {
			u, err := h.userRepo.GetByID(uid)
			if err != nil {
				log.Printf("[auth] 사용자 조회 실패 uid=%s: %v", uid, err)
				c.Set("auth_unconfirmed", true)
				h.writeSessionCookie(c, sessionClaimsFromContext(c, true))
			} else if u != nil {
				if !u.IsActive {
					c.SetCookie(&http.Cookie{Name: "token", Value: "", Path: "/", MaxAge: -1})
					return c.Redirect(http.StatusSeeOther, "/login")
				}
				viewOrg := ctxString(c, "view_org_id")
				applyUserSession(c, u)
				c.Set("view_org_id", viewOrg)
				h.refreshSession(c, u)
			}
		}
		if canSwitchOrg(c) && h.orgRepo != nil {
			if orgs, err := h.orgRepo.ListActive(); err == nil {
				c.Set("org_switch_list", orgs)
			}
		}
		if _, has := claims["profile_ok"]; has && !claimBool(claims, "profile_ok") {
			if !profileCompleteExempt(path) {
				return c.Redirect(http.StatusSeeOther, "/account/complete")
			}
		}
		pop := audit.Push(audit.Actor{
			UserID:   ctxString(c, "user_id"),
			Username: ctxString(c, "username"),
			Name:     ctxString(c, "user_name"),
			Role:     loginRole(c),
		})
		defer pop()
		return next(c)
	}
}

func profileCompleteExempt(path string) bool {
	p := strings.TrimSpace(path)
	return p == "/account/complete" || p == "/logout" || strings.HasPrefix(p, "/static")
}

const authRevalidateInterval = 5 * time.Minute

func sessionClaims(user *model.User, unconfirmed bool) jwt.MapClaims {
	role := model.NormalizeRole(user.Role)
	claims := jwt.MapClaims{
		"user_id":     user.UserID,
		"username":    user.Username,
		"role":        role,
		"name":        user.FullName,
		"org_id":      strings.TrimSpace(user.OrgID),
		"base_role":   strings.TrimSpace(user.BaseRole),
		"permissions": model.FormatPermissions(user.PermList()),
		"verified_at": time.Now().Unix(),
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
		"profile_ok":  !user.NeedsProfileFill(),
	}
	if unconfirmed {
		claims["auth_unconfirmed"] = true
	}
	return claims
}

func sessionClaimsFromContext(c echo.Context, unconfirmed bool) jwt.MapClaims {
	role := model.NormalizeRole(ctxString(c, "role"))
	claims := jwt.MapClaims{
		"user_id":     ctxString(c, "user_id"),
		"username":    ctxString(c, "username"),
		"role":        role,
		"name":        ctxString(c, "user_name"),
		"org_id":      ctxString(c, "org_id"),
		"view_org_id": ctxString(c, "view_org_id"),
		"base_role":   ctxString(c, "base_role"),
		"permissions": model.FormatPermissions(currentPerms(c)),
		"verified_at": time.Now().Unix(),
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
	}
	if unconfirmed {
		claims["auth_unconfirmed"] = true
	}
	return claims
}

func (h *AuthHandler) writeSessionCookie(c echo.Context, claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(h.jwtSecret)
	if err != nil {
		return ""
	}
	c.SetCookie(&http.Cookie{
		Name:     "token",
		Value:    tokenStr,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	return tokenStr
}

func applySessionClaims(c echo.Context, claims jwt.MapClaims) string {
	role := model.NormalizeRole(claimString(claims, "role"))
	c.Set("user_id", claimString(claims, "user_id"))
	c.Set("username", claimString(claims, "username"))
	c.Set("role", role)
	c.Set("user_name", claimString(claims, "name"))
	c.Set("org_id", claimString(claims, "org_id"))
	c.Set("view_org_id", claimString(claims, "view_org_id"))
	c.Set("base_role", model.NormalizeRole(claimString(claims, "base_role")))
	c.Set("permissions", model.EffectivePermissions(model.EffectiveRole(role, claimString(claims, "base_role")), claimString(claims, "permissions")))
	if claimBool(claims, "auth_unconfirmed") {
		c.Set("auth_unconfirmed", true)
	}
	return role
}

func applyUserSession(c echo.Context, u *model.User) {
	c.Set("username", u.Username)
	c.Set("role", model.NormalizeRole(u.Role))
	c.Set("user_name", u.FullName)
	c.Set("org_id", strings.TrimSpace(u.OrgID))
	c.Set("base_role", strings.TrimSpace(u.BaseRole))
	c.Set("permissions", u.PermList())
	c.Set("auth_unconfirmed", false)
}

func needRevalidate(claims jwt.MapClaims) bool {
	at := claimInt64(claims, "verified_at")
	if at <= 0 {
		return true
	}
	return time.Since(time.Unix(at, 0)) >= authRevalidateInterval
}

func claimString(claims jwt.MapClaims, key string) string {
	v, ok := claims[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func claimInt64(claims jwt.MapClaims, key string) int64 {
	v, ok := claims[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func claimBool(claims jwt.MapClaims, key string) bool {
	v, ok := claims[key]
	if !ok || v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	return s == "1" || strings.EqualFold(s, "true")
}

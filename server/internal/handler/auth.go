package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	settingsRepo *repository.SettingsRepo
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
		msg = "이름이 저장되었습니다."
	}
	return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
		"Title": "내 계정", "Active": NavAccount, "User": u, "OK": msg,
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
	h.userRepo.Update(u)
	// JWT에 이름이 남아 있으므로 재로그인 권장 — 쿠키를 갱신해 즉시 반영
	h.refreshSession(c, u)
	return c.Redirect(http.StatusSeeOther, "/account?ok=profile")
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
	h.writeSessionCookie(c, sessionClaims(user, false))
}

func (h *AuthHandler) isAdmin(c echo.Context) bool {
	return currentRole(c) == model.RoleAdmin
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
	}
	errMsg := c.QueryParam("err")
	canEdit := h.isAdmin(c) || hasPerm(c, model.PermCodesUsers)
	data := map[string]interface{}{
		"Title": "사용자 관리", "Active": NavUsers, "Users": users, "OK": msg, "Error": errMsg,
		"PermDefs": model.AllPermissions, "CanEditUsers": canEdit,
		"RoleGroups": model.GroupUsersByRole(users),
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
	perms := parsePermForm(c)
	if perms == "" {
		perms = model.FormatPermissions(model.DefaultPermissions(role))
	}
	u := &model.User{
		Username:     c.FormValue("username"),
		PasswordHash: HashPassword(c.FormValue("password")),
		FullName:     c.FormValue("full_name"),
		Role:         role,
		Permissions:  perms,
		IsActive:     true,
	}
	h.userRepo.Create(u)
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
	u.Role = model.NormalizeRole(c.FormValue("role"))
	u.Permissions = parsePermForm(c)
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
		if path == "/login" || strings.HasPrefix(path, "/static") {
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
				applyUserSession(c, u)
				h.refreshSession(c, u)
			}
		}
		pop := audit.Push(audit.Actor{
			UserID:   ctxString(c, "user_id"),
			Username: ctxString(c, "username"),
			Name:     ctxString(c, "user_name"),
		})
		defer pop()
		return next(c)
	}
}

const authRevalidateInterval = 5 * time.Minute

func sessionClaims(user *model.User, unconfirmed bool) jwt.MapClaims {
	role := model.NormalizeRole(user.Role)
	claims := jwt.MapClaims{
		"user_id":     user.UserID,
		"username":    user.Username,
		"role":        role,
		"name":        user.FullName,
		"permissions": model.FormatPermissions(user.PermList()),
		"verified_at": time.Now().Unix(),
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
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
	c.Set("permissions", model.EffectivePermissions(role, claimString(claims, "permissions")))
	if claimBool(claims, "auth_unconfirmed") {
		c.Set("auth_unconfirmed", true)
	}
	return role
}

func applyUserSession(c echo.Context, u *model.User) {
	c.Set("username", u.Username)
	c.Set("role", model.NormalizeRole(u.Role))
	c.Set("user_name", u.FullName)
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

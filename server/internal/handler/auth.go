package handler

import (
	"database/sql"
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
	"customer-support/internal/config"
	"customer-support/internal/model"
	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

type AuthHandler struct {
	userRepo     *repository.UserRepo
	orgRepo      *repository.OrgRepo
	settingsRepo *repository.SettingsRepo
	adminUnlock  *repository.AdminUnlockRepo
	helpRepo     *repository.LoginHelpRepo
	wbRepo       *repository.WBRepo
	db           *sql.DB
	cfg          *config.Config
	jwtSecret    []byte
	uploadDir    string
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
		return c.Render(http.StatusOK, "auth/login.html", h.loginPageData(map[string]interface{}{
			"Error": "아이디 또는 비밀번호가 잘못되었습니다.",
		}))
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
		return c.Render(http.StatusOK, "auth/login.html", h.loginPageData(map[string]interface{}{
			"Error": "일시적으로 로그인할 수 없습니다. 서버를 재시작한 뒤 다시 시도하세요.",
		}))
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
	h.clearViewAsCookie(c)
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
		return c.Redirect(http.StatusSeeOther, "/account/complete?ok=1")
	}
	if h.visionPasswordMustChange(user) {
		return c.Redirect(http.StatusSeeOther, "/account/vision-password?ok=1")
	}
	return c.Redirect(http.StatusSeeOther, "/?ok=1")
}

func (h *AuthHandler) Logout(c echo.Context) error {
	rec := auditlog.Record{
		Action: auditlog.ActionLogout,
		Result: auditlog.ResultOK,
		Detail: "로그아웃",
	}
	h.fillActorFromTokenCookie(c, &rec)
	accessLog(c, rec)
	h.clearTokenCookie(c)
	h.clearViewAsCookie(c)
	return c.Redirect(http.StatusSeeOther, "/login")
}

func (h *AuthHandler) clearTokenCookie(c echo.Context) {
	c.SetCookie(tokenCookie("", -1))
}

func (h *AuthHandler) clearViewAsCookie(c echo.Context) {
	c.SetCookie(sessionCookie(viewAsCookie, "", -1))
}

func tokenCookie(value string, maxAge int) *http.Cookie {
	return sessionCookie("token", value, maxAge)
}

func sessionCookie(name, value string, maxAge int) *http.Cookie {
	ck := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}
	// HTTPS 도입 시 COOKIE_SECURE=true. HTTP 에서 켜면 로그인이 불가능해진다(§24.2).
	// APP_ENV / appEnvProduction() 으로 판단하지 않는다 — 그 값은 데이터 초기화 보호용이다.
	if config.Load().CookieSecure {
		ck.Secure = true
	}
	return ck
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
		if n := strings.TrimSpace(c.QueryParam("n")); n != "" && n != "0" {
			msg = fmt.Sprintf("계정 정보가 저장되었습니다. 담당 글자 %s건을 함께 고쳤습니다.", n)
		}
	} else if c.QueryParam("ok") == "signature" {
		msg = "사인을 저장했습니다."
	} else if c.QueryParam("ok") == "signature_cleared" {
		msg = "사인을 지웠습니다."
	} else if c.QueryParam("ok") == "vision" {
		msg = "비젼관리자 비밀번호가 변경되었습니다."
	}
	var orgs []model.Org
	if h.orgRepo != nil {
		orgs, _ = h.orgRepo.ListActive()
	}
	return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
		"Title": "내 계정", "Active": NavAccount, "User": u, "OK": msg, "Orgs": orgs,
		"HasSignature":  strings.TrimSpace(u.SignaturePath) != "" && signatureFileExists(u.SignaturePath),
		"HasSignature2": strings.TrimSpace(u.SignaturePath2) != "" && signatureFileExists(u.SignaturePath2),
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
	oldName := strings.TrimSpace(u.FullName)
	u.FullName = name
	bindUserContact(c, u)
	if err := u.ProfileFieldError(); err != "" {
		return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
			"Title": "내 계정", "Active": NavAccount, "User": u, "Error": err,
		})
	}
	h.userRepo.Update(u)
	n, _, _ := h.rewriteNamesIfChanged(u.UserID, oldName, u.FullName)
	h.refreshSession(c, u)
	loc := "/account?ok=profile"
	if oldName != strings.TrimSpace(u.FullName) {
		loc = fmt.Sprintf("/account?ok=profile&n=%d", n)
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

func (h *AuthHandler) AccountRenameUsername(c echo.Context) error {
	uid := ctxString(c, "user_id")
	newUsername := strings.TrimSpace(c.FormValue("username"))
	if newUsername == "" {
		u, _ := h.userRepo.GetByID(uid)
		return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
			"Title": "내 계정", "Active": NavAccount, "User": u, "Error": "아이디를 입력하세요.",
		})
	}
	if err := h.userRepo.RenameUsername(uid, newUsername); err != nil {
		u, _ := h.userRepo.GetByID(uid)
		return c.Render(http.StatusOK, "auth/account.html", map[string]interface{}{
			"Title": "내 계정", "Active": NavAccount, "User": u, "Error": err.Error(),
		})
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionUpdate,
		Result:      auditlog.ResultOK,
		TargetTable: "users",
		TargetID:    uid,
		SubjectType: "user",
		SubjectID:   uid,
		Detail:      "아이디 변경",
	})
	h.clearTokenCookie(c)
	return c.Redirect(http.StatusSeeOther, "/login")
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

// resolveAdminRole 관리 화면 저장. 업무·조직·연락처가 비어도 기존 값을 유지하거나 지원으로 둔다.
func resolveAdminRole(c echo.Context, fallback string) string {
	role, err := model.RoleFromAdminGrade(c.FormValue("admin_grade"), c.FormValue("job"))
	if err == nil && model.IsKnownRole(role) {
		return role
	}
	if r := model.NormalizeRole(c.FormValue("role")); model.IsKnownRole(r) {
		return r
	}
	if model.IsKnownRole(fallback) {
		return fallback
	}
	return model.RoleSupport
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
	if passwordTooShort(pw) {
		return renderErr(passwordMinLenMsg())
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
	if !h.isAdmin(c) && !isReadOnly(c) && !hasPerm(c, model.PermCodesUsers) {
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
	case "username":
		msg = "아이디를 바꿨습니다. 그 사람은 다시 로그인해야 합니다."
	case "signature":
		msg = "사인을 저장했습니다."
	case "signature_cleared":
		msg = "사인을 지웠습니다."
	case "renamed":
		msg = fmt.Sprintf("이름을 바꿨습니다. 담당 글자 %s건을 함께 고쳤습니다.", strings.TrimSpace(c.QueryParam("n")))
	case "deleted":
		msg = "계정을 지웠습니다."
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
		"PasswordMinLen": passwd.MinLen, "VisionAdminCount": 0,
		"WorkCounts": map[string]int{},
	}
	orgNames := map[string]string{}
	for _, o := range orgs {
		orgNames[o.OrgID] = o.OrgName
	}
	data["OrgNames"] = orgNames
	if h.userRepo != nil {
		data["VisionAdminCount"] = h.userRepo.CountRole(model.RoleVisionAdmin)
		wc := map[string]int{}
		for _, u := range users {
			wc[u.UserID] = h.userRepo.AssignedWorkCount(u.UserID)
		}
		data["WorkCounts"] = wc
	}
	if canEdit {
		data["HashMig"] = h.passwordMigrationStatus()
	}
	if old := strings.TrimSpace(c.QueryParam("old")); old != "" && h.userRepo != nil {
		data["Unmatched"] = h.userRepo.UnmatchedAfterNameChange(old)
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
	role := resolveAdminRole(c, "")
	if passwordTooShort(c.FormValue("password")) {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(passwordMinLenMsg()))
	}
	form, _ := c.FormParams()
	var selected []string
	if form != nil {
		selected = form["perm"]
	}
	perms := model.CompactStoredPermissions(role, selected)
	orgID := strings.TrimSpace(c.FormValue("org_id"))
	if role == model.RoleVisionAdmin {
		orgID = ""
	}
	u := &model.User{
		Username:     strings.TrimSpace(c.FormValue("username")),
		PasswordHash: HashPassword(c.FormValue("password")),
		FullName:     strings.TrimSpace(c.FormValue("full_name")),
		Role:         role,
		Permissions:  perms,
		IsActive:     true,
		OrgID:        orgID,
		IsReadOnly:   c.FormValue("is_readonly") == "1",
		IsTest:       c.FormValue("is_test") == "1" && !model.IsAdminGrade(role),
	}
	bindUserContact(c, u)
	if strings.TrimSpace(u.Username) == "" || strings.TrimSpace(u.FullName) == "" {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("이름과 아이디를 입력하세요."))
	}
	if taken, err := h.userRepo.UsernameTaken(u.Username, ""); err != nil || taken {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("이미 있는 아이디입니다."))
	}
	if currentRole(c) == model.RoleOrgAdmin {
		if home := strings.TrimSpace(ctxString(c, "org_id")); home != "" {
			u.OrgID = home
		}
	}
	if err := h.userRepo.Create(u); err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("계정을 만들지 못했습니다."))
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionCreate,
		Result:      auditlog.ResultOK,
		TargetTable: "users",
		TargetID:    u.UserID,
		SubjectType: "user",
		SubjectID:   u.UserID,
		SubjectName: u.FullName,
		Detail:      "계정 생성",
		AfterJSON:   toJSON(userPublic(u)),
	})
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
	oldName := strings.TrimSpace(u.FullName)
	if name := strings.TrimSpace(c.FormValue("full_name")); name != "" {
		u.FullName = name
	}
	prevRole := model.NormalizeRole(c.FormValue("prev_role"))
	u.Role = resolveAdminRole(c, u.Role)
	u.BaseRole = ""
	u.OrgID = strings.TrimSpace(c.FormValue("org_id"))
	if currentRole(c) == model.RoleOrgAdmin {
		if home := strings.TrimSpace(ctxString(c, "org_id")); home != "" {
			u.OrgID = home
		}
	}
	if u.Role == model.RoleVisionAdmin {
		u.OrgID = ""
	}
	u.IsReadOnly = c.FormValue("is_readonly") == "1"
	u.IsTest = c.FormValue("is_test") == "1" && !model.IsAdminGrade(u.Role)
	if v := c.FormValue("is_active"); v != "" {
		u.IsActive = v == "1"
	}
	bindUserContact(c, u)
	if prevRole != "" && prevRole != u.Role && c.FormValue("clear_custom_perms") == "1" {
		u.Permissions = ""
	} else {
		form, _ := c.FormParams()
		var selected []string
		if form != nil {
			selected = form["perm"]
		}
		if len(selected) > 0 {
			u.Permissions = model.CompactStoredPermissions(u.Role, selected)
		}
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
			return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("비밀번호와 확인 입력이 일치하지 않습니다."))
		}
		if passwordTooShort(pw) {
			return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(passwordMinLenMsg()))
		}
		h.userRepo.UpdatePassword(u.UserID, HashPassword(pw))
		return c.Redirect(http.StatusSeeOther, "/users?ok=password")
	}
	n, _, _ := h.rewriteNamesIfChanged(u.UserID, oldName, u.FullName)
	if oldName != strings.TrimSpace(u.FullName) {
		loc := fmt.Sprintf("/users?ok=renamed&n=%d&old=%s", n, url.QueryEscape(oldName))
		return c.Redirect(http.StatusSeeOther, loc)
	}
	return c.Redirect(http.StatusSeeOther, "/users?ok=saved")
}

func (h *AuthHandler) rewriteNamesIfChanged(userID, oldName, newName string) (int, []repository.UnmatchedAssignee, error) {
	if h == nil || h.userRepo == nil {
		return 0, nil, nil
	}
	return h.userRepo.RewriteAssigneeNames(userID, oldName, newName)
}

func (h *AuthHandler) UserRenameUsername(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	id := strings.TrimSpace(c.Param("id"))
	newUsername := strings.TrimSpace(c.FormValue("username"))
	if newUsername == "" {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("아이디를 입력하세요."))
	}
	if err := h.userRepo.RenameUsername(id, newUsername); err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(err.Error()))
	}
	u, _ := h.userRepo.GetByID(id)
	name := newUsername
	if u != nil {
		name = u.FullName
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionUpdate,
		Result:      auditlog.ResultOK,
		TargetTable: "users",
		TargetID:    id,
		SubjectType: "user",
		SubjectID:   id,
		SubjectName: name,
		Detail:      "아이디 변경",
	})
	if ctxString(c, "user_id") == id {
		h.clearTokenCookie(c)
		return c.Redirect(http.StatusSeeOther, "/login")
	}
	return c.Redirect(http.StatusSeeOther, "/users?ok=username")
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
	if pw == "" {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("새 비밀번호를 입력하세요."))
	}
	if pw != confirm {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("비밀번호와 확인 입력이 일치하지 않습니다."))
	}
	if passwordTooShort(pw) {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(passwordMinLenMsg()))
	}
	h.userRepo.UpdatePassword(u.UserID, HashPassword(pw))
	return c.Redirect(http.StatusSeeOther, "/users?ok=password")
}

func (h *AuthHandler) UserDelete(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" || id == ctxString(c, "user_id") {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("이 계정은 지울 수 없습니다."))
	}
	u, _ := h.userRepo.GetByID(id)
	if u == nil {
		return echo.ErrNotFound
	}
	if !h.verifyVisionAdminPassword(c, c.FormValue("vision_password")) {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("비젼관리자 비밀번호가 올바르지 않습니다."))
	}
	n := h.userRepo.AssignedWorkCount(id)
	if n > 0 {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(fmt.Sprintf("담당 업무 %d건이 있습니다. 이관 후에 지우세요.", n)))
	}
	before := userPublic(u)
	if err := h.userRepo.Delete(id); err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("계정을 지우지 못했습니다."))
	}
	accessLog(c, auditlog.Record{
		Action:      auditlog.ActionDelete,
		Result:      auditlog.ResultOK,
		TargetTable: "users",
		TargetID:    id,
		SubjectType: "user",
		SubjectID:   id,
		SubjectName: u.FullName,
		Detail:      "계정 삭제",
		Reason:      accessReason(c, "계정 삭제"),
		BeforeJSON:  toJSON(before),
	})
	return c.Redirect(http.StatusSeeOther, "/users?ok=deleted")
}

func redirectLogin(c echo.Context) error {
	if c.QueryParam("ok") == "1" {
		return c.Redirect(http.StatusSeeOther, "/login?err=cookie")
	}
	return c.Redirect(http.StatusSeeOther, "/login")
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
			return redirectLogin(c)
		}

		token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return h.jwtSecret, nil
		})
		if err != nil {
			log.Printf("JWT 파싱 오류: %v", err)
			return redirectLogin(c)
		}
		if !token.Valid {
			return redirectLogin(c)
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return redirectLogin(c)
		}
		role := applySessionClaims(c, claims)
		if !model.IsKnownRole(role) {
			return redirectLogin(c)
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
					c.SetCookie(tokenCookie("", -1))
					return redirectLogin(c)
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
		if h.settingsRepo != nil && h.settingsRepo.VisionAdminMustChange() && loginRole(c) == model.RoleVisionAdmin {
			if !visionPasswordExempt(path) {
				return c.Redirect(http.StatusSeeOther, "/account/vision-password")
			}
		}
		pop := audit.Push(audit.Actor{
			UserID:   ctxString(c, "user_id"),
			Username: ctxString(c, "username"),
			Name:     ctxString(c, "user_name"),
			Role:     loginRole(c),
			IsTest:   ctxBool(c, "is_test"),
		})
		defer pop()
		return next(c)
	}
}

func profileCompleteExempt(path string) bool {
	p := strings.TrimSpace(path)
	return p == "/account/complete" || p == "/logout" || strings.HasPrefix(p, "/static") || p == "/account/vision-password"
}

func visionPasswordExempt(path string) bool {
	p := strings.TrimSpace(path)
	return p == "/account/vision-password" || p == "/logout" || strings.HasPrefix(p, "/static") || p == "/account/complete"
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
		"permissions": model.FormatPermissions(user.PermList()),
		"is_readonly": user.IsReadOnly,
		"is_test":     user.IsTest,
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
		"permissions": model.FormatPermissions(currentPerms(c)),
		"is_readonly": isReadOnly(c),
		"is_test":     ctxBool(c, "is_test"),
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
	c.SetCookie(tokenCookie(tokenStr, 86400))
	return tokenStr
}

func applySessionClaims(c echo.Context, claims jwt.MapClaims) string {
	role := model.NormalizeRole(claimString(claims, "role"))
	role = interpretLegacyRoleFlags(c, claims, role)
	c.Set("user_id", claimString(claims, "user_id"))
	c.Set("username", claimString(claims, "username"))
	c.Set("role", role)
	c.Set("user_name", claimString(claims, "name"))
	c.Set("org_id", claimString(claims, "org_id"))
	c.Set("view_org_id", claimString(claims, "view_org_id"))
	c.Set("permissions", model.EffectivePermissions(role, claimString(claims, "permissions")))
	if claimBool(claims, "auth_unconfirmed") {
		c.Set("auth_unconfirmed", true)
	}
	return role
}

func interpretLegacyRoleFlags(c echo.Context, claims jwt.MapClaims, role string) string {
	_, hasRO := claims["is_readonly"]
	ro := claimBool(claims, "is_readonly")
	test := claimBool(claims, "is_test")
	base := model.NormalizeRole(claimString(claims, "base_role"))
	job := func() string {
		if base == model.RoleSales || base == model.RoleTech || base == model.RoleSupport {
			return base
		}
		return ""
	}
	if role == model.RoleObserver {
		if !hasRO {
			log.Printf("[v57] 옛 observer 토큰 username=%s — 읽기전용으로 해석", claimString(claims, "username"))
		}
		ro = true
		if j := job(); j != "" {
			role = j
		} else {
			role = model.RoleOrgAdmin
		}
	}
	if role == model.RoleTester {
		test = true
		if j := job(); j != "" {
			role = j
		} else {
			role = model.RoleSupport
		}
	}
	c.Set("is_readonly", ro)
	c.Set("is_test", test)
	return role
}

func applyUserSession(c echo.Context, u *model.User) {
	c.Set("username", u.Username)
	c.Set("role", model.NormalizeRole(u.Role))
	c.Set("user_name", u.FullName)
	c.Set("org_id", strings.TrimSpace(u.OrgID))
	c.Set("permissions", u.PermList())
	c.Set("is_readonly", u.IsReadOnly)
	c.Set("is_test", u.IsTest)
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

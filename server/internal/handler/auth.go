package handler

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type AuthHandler struct {
	userRepo  *repository.UserRepo
	jwtSecret []byte
}

func HashPassword(pw string) string {
	h := sha256.Sum256([]byte(pw))
	return fmt.Sprintf("%x", h)
}

func (h *AuthHandler) LoginPage(c echo.Context) error {
	return c.Render(http.StatusOK, "auth/login.html", map[string]interface{}{
		"Title": "로그인", "Active": "login", "HideNav": true,
	})
}

func (h *AuthHandler) Login(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	user, err := h.userRepo.GetByUsername(username)
	if err != nil || user == nil || user.PasswordHash != HashPassword(password) || !user.IsActive {
		return c.Render(http.StatusOK, "auth/login.html", map[string]interface{}{
			"Title": "로그인", "Active": "login", "HideNav": true,
			"Error": "아이디 또는 비밀번호가 잘못되었습니다.",
		})
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.UserID,
		"username": user.Username,
		"role":     user.Role,
		"name":     user.FullName,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString(h.jwtSecret)

	c.SetCookie(&http.Cookie{
		Name:     "token",
		Value:    tokenStr,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	return c.Redirect(http.StatusSeeOther, "/")
}

func (h *AuthHandler) Logout(c echo.Context) error {
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
		"Title": "내 계정", "Active": "account", "User": u, "OK": msg,
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
			"Title": "내 계정", "Active": "account", "User": u,
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
			"Title": "내 계정", "Active": "account", "User": u, "Error": msg,
		})
	}
	if u.PasswordHash != HashPassword(current) {
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
	if HashPassword(pw) == u.PasswordHash {
		return renderErr("새 비밀번호는 현재 비밀번호와 달라야 합니다.")
	}
	h.userRepo.UpdatePassword(u.UserID, HashPassword(pw))
	u.PasswordHash = HashPassword(pw)
	h.refreshSession(c, u)
	return c.Redirect(http.StatusSeeOther, "/account?ok=password")
}

func (h *AuthHandler) refreshSession(c echo.Context, user *model.User) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.UserID,
		"username": user.Username,
		"role":     user.Role,
		"name":     user.FullName,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(h.jwtSecret)
	if err != nil {
		return
	}
	c.SetCookie(&http.Cookie{
		Name:     "token",
		Value:    tokenStr,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

func (h *AuthHandler) isAdmin(c echo.Context) bool {
	return ctxString(c, "role") == "admin"
}

func (h *AuthHandler) forbidden(c echo.Context) error {
	return c.Render(http.StatusForbidden, "auth/forbidden.html", map[string]interface{}{
		"Title": "접근 권한 없음", "Active": "",
	})
}

// UserList 사용자 관리 화면
func (h *AuthHandler) UserList(c echo.Context) error {
	if !h.isAdmin(c) {
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
	return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
		"Title": "사용자 관리", "Active": "users", "Users": users, "OK": msg, "Error": errMsg,
	})
}

func (h *AuthHandler) UserCreate(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	u := &model.User{
		Username:     c.FormValue("username"),
		PasswordHash: HashPassword(c.FormValue("password")),
		FullName:     c.FormValue("full_name"),
		Role:         c.FormValue("role"),
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
	u.FullName = c.FormValue("full_name")
	u.Role = c.FormValue("role")
	u.IsActive = c.FormValue("is_active") != "0"
	h.userRepo.Update(u)

	if pw := strings.TrimSpace(c.FormValue("password")); pw != "" {
		confirm := strings.TrimSpace(c.FormValue("password_confirm"))
		if confirm != "" && pw != confirm {
			users, _ := h.userRepo.ListAll()
			return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
				"Title": "사용자 관리", "Active": "users", "Users": users,
				"Error": "비밀번호와 확인 입력이 일치하지 않습니다.",
			})
		}
		if len(pw) < 4 {
			users, _ := h.userRepo.ListAll()
			return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
				"Title": "사용자 관리", "Active": "users", "Users": users,
				"Error": "비밀번호는 4자 이상이어야 합니다.",
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
			"Title": "사용자 관리", "Active": "users", "Users": users,
			"Error": "새 비밀번호를 입력하세요.", "FocusUser": u.UserID,
		})
	}
	if pw != confirm {
		return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
			"Title": "사용자 관리", "Active": "users", "Users": users,
			"Error": "비밀번호와 확인 입력이 일치하지 않습니다.", "FocusUser": u.UserID,
		})
	}
	if len(pw) < 4 {
		return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
			"Title": "사용자 관리", "Active": "users", "Users": users,
			"Error": "비밀번호는 4자 이상이어야 합니다.", "FocusUser": u.UserID,
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

		claims := token.Claims.(jwt.MapClaims)
		uid := claimString(claims, "user_id")
		c.Set("user_id", uid)
		c.Set("username", claimString(claims, "username"))
		c.Set("role", claimString(claims, "role"))
		c.Set("user_name", claimString(claims, "name"))

		// 역할·이름 변경이 JWT에 남아 있어도 DB 기준으로 즉시 반영
		if uid != "" {
			if u, err := h.userRepo.GetByID(uid); err == nil && u != nil {
				if !u.IsActive {
					c.SetCookie(&http.Cookie{Name: "token", Value: "", Path: "/", MaxAge: -1})
					return c.Redirect(http.StatusSeeOther, "/login")
				}
				c.Set("username", u.Username)
				c.Set("role", u.Role)
				c.Set("user_name", u.FullName)
			}
		}
		return next(c)
	}
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

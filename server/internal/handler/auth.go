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
		MaxAge:   86400,
	})
	return c.Redirect(http.StatusSeeOther, "/")
}

func (h *AuthHandler) Logout(c echo.Context) error {
	c.SetCookie(&http.Cookie{
		Name:   "token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	return c.Redirect(http.StatusSeeOther, "/login")
}

// UserList 사용자 관리 화면
func (h *AuthHandler) UserList(c echo.Context) error {
	users, _ := h.userRepo.ListAll()
	msg := ""
	switch c.QueryParam("ok") {
	case "password":
		msg = "비밀번호가 변경되었습니다."
	case "saved":
		msg = "사용자 정보가 저장되었습니다."
	}
	return c.Render(http.StatusOK, "auth/users.html", map[string]interface{}{
		"Title": "사용자 관리", "Active": "users", "Users": users, "OK": msg,
	})
}

func (h *AuthHandler) UserCreate(c echo.Context) error {
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

// UserChangePassword 비밀번호만 변경
func (h *AuthHandler) UserChangePassword(c echo.Context) error {
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
		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])
		c.Set("user_name", claims["name"])
		return next(c)
	}
}

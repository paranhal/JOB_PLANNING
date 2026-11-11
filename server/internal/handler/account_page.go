package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/service/approval"
)

func (h *AuthHandler) renderAccount(c echo.Context, u *model.User, errMsg string) error {
	if u == nil {
		return c.Redirect(http.StatusSeeOther, "/logout")
	}
	data := h.accountView(c, u, errMsg)
	return c.Render(http.StatusOK, "auth/account.html", data)
}

func (h *AuthHandler) accountView(c echo.Context, u *model.User, errMsg string) map[string]interface{} {
	ok := ""
	if errMsg == "" {
		switch c.QueryParam("ok") {
		case "password":
			ok = "비밀번호가 변경되었습니다."
		case "profile":
			ok = "계정 정보가 저장되었습니다."
			if n := strings.TrimSpace(c.QueryParam("n")); n != "" && n != "0" {
				ok = "계정 정보가 저장되었습니다. 담당 글자 " + n + "건을 함께 고쳤습니다."
			}
		case "signature":
			ok = "사인을 저장했습니다."
		case "signature_cleared":
			ok = "사인을 지웠습니다."
		case "vision":
			ok = "비젼관리자 비밀번호가 변경되었습니다."
		}
	}
	must := u.MustChangePassword || c.QueryParam("must") == "1" || c.FormValue("must") == "1"
	if must && ok == "" && errMsg == "" {
		ok = "임시 비밀번호입니다. 지금 바꿔 주세요."
	}
	var orgs []model.Org
	if h.orgRepo != nil {
		orgs, _ = h.orgRepo.ListActive()
	}
	orgName := "—"
	if h.orgRepo != nil && strings.TrimSpace(u.OrgID) != "" {
		if org, err := h.orgRepo.Get(u.OrgID); err == nil && org != nil && strings.TrimSpace(org.OrgName) != "" {
			orgName = org.OrgName
		}
	}
	hire := "—"
	if h.db != nil && strings.TrimSpace(u.UserID) != "" {
		var d string
		_ = h.db.QueryRow(`SELECT COALESCE(hire_date,'') FROM employee_profiles WHERE user_id=?`, u.UserID).Scan(&d)
		if strings.TrimSpace(d) != "" {
			hire = strings.TrimSpace(d)
		}
	}
	headLabel := "미지정 — 조직관리자가 결재"
	if h.db != nil && h.userRepo != nil && strings.TrimSpace(u.OrgID) != "" {
		if id, err := (&approval.Engine{DB: h.db}).OrgHead(u.OrgID); err == nil && strings.TrimSpace(id) != "" {
			if head, _ := h.userRepo.GetByID(id); head != nil {
				name := strings.TrimSpace(head.FullName)
				if name == "" {
					name = strings.TrimSpace(head.Username)
				}
				if name != "" {
					headLabel = name
				}
			}
		}
	}
	usernameInput := u.Username
	if c.Get("account_username_keep") == true {
		usernameInput = c.FormValue("username")
	}
	data := map[string]interface{}{
		"Title": "내 계정", "Active": NavAccount, "User": u, "OK": ok, "Error": errMsg, "Orgs": orgs,
		"MustChange":    must,
		"HasSignature":  strings.TrimSpace(u.SignaturePath) != "" && signatureFileExists(u.SignaturePath),
		"HasSignature2": strings.TrimSpace(u.SignaturePath2) != "" && signatureFileExists(u.SignaturePath2),
		"OrgName":       orgName,
		"RoleLabel":     model.RoleLabel(u.Role),
		"HireDate":      hire,
		"HeadLabel":     headLabel,
		"UsernameInput": usernameInput,
	}
	h.attachDupUsername(c, data)
	return data
}

func (h *AuthHandler) keepPostedUsername(c echo.Context) {
	c.Set("account_username_keep", true)
}

package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

const usernameDupDefaultReturn = "/admin/users/deleted"

var usernameDupReturnAllow = map[string]bool{
	"/admin/users":          true,
	"/users":                true,
	"/admin/my-org/members": true,
	"/admin/users/requests": true,
	"/admin/users/deleted":  true,
	"/account":              true,
}

type dupUsernameView struct {
	Kind         string
	UserID       string
	Username     string
	FullName     string
	Mobile       string
	DeletedAt    string
	DeleteReason string
	OrgID        string
	Return       string
	ApplyUserID  string
	CanRestore   bool
	CanRelease   bool
	Title        string
	Body         string
	Warning      string
	Orgs         []model.Org
}

func safeUsernameDupReturn(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return usernameDupDefaultReturn
	}
	u, err := url.Parse(raw)
	if err != nil {
		return usernameDupDefaultReturn
	}
	if u.Host != "" || u.Scheme != "" || strings.HasPrefix(raw, "//") {
		return usernameDupDefaultReturn
	}
	path := u.Path
	if path == "" {
		path = raw
	}
	if usernameDupReturnAllow[path] {
		return path
	}
	return usernameDupDefaultReturn
}

func (h *AuthHandler) usernameConflict(username, exceptUserID string) (model.UsernameConflict, error) {
	if h == nil || h.userRepo == nil {
		return model.UsernameConflict{}, echo.NewHTTPError(http.StatusInternalServerError, "계정을 확인할 수 없습니다")
	}
	return h.userRepo.LookupUsernameConflict(username, exceptUserID)
}

func (h *AuthHandler) redirectIfUsernameConflict(c echo.Context, fallback, username, exceptUserID string) (bool, error) {
	cf, err := h.usernameConflict(username, exceptUserID)
	if err != nil {
		return true, c.Redirect(http.StatusSeeOther, fallback+"?err="+url.QueryEscape(model.UsernameReservedMsg))
	}
	if cf.Empty() {
		return false, nil
	}
	return true, h.redirectUsernameConflict(c, fallback, username, exceptUserID, cf)
}

func (h *AuthHandler) redirectUsernameConflict(c echo.Context, fallback, username, exceptUserID string, cf model.UsernameConflict) error {
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		fallback = "/admin/users"
	}
	if dup := cf.DialogDup(); dup != "" {
		ret := safeUsernameDupReturn(firstNonEmpty(c.FormValue("return"), fallback))
		q := url.Values{}
		q.Set("dup", dup)
		q.Set("return", ret)
		if dup == model.UsernameConflictDeleted && strings.TrimSpace(cf.UserID) != "" {
			q.Set("id", cf.UserID)
		} else {
			q.Set("username", firstNonEmpty(strings.TrimSpace(username), cf.Username))
			if strings.TrimSpace(exceptUserID) != "" {
				q.Set("apply_user_id", exceptUserID)
			}
		}
		return c.Redirect(http.StatusSeeOther, fallback+"?"+q.Encode())
	}
	msg := cf.Message()
	if msg == "" {
		msg = model.UsernameReservedMsg
	}
	return c.Redirect(http.StatusSeeOther, fallback+"?err="+url.QueryEscape(msg))
}

func (h *AuthHandler) attachDupUsername(c echo.Context, data map[string]interface{}) {
	if data == nil || h == nil || h.userRepo == nil || c == nil {
		return
	}
	dup := strings.TrimSpace(c.QueryParam("dup"))
	if dup == "" {
		return
	}
	vision := currentRole(c) == model.RoleVisionAdmin
	canAct := vision && !isReadOnly(c)
	ret := safeUsernameDupReturn(c.QueryParam("return"))
	applyID := strings.TrimSpace(c.QueryParam("apply_user_id"))

	if dup == model.UsernameConflictDeleted {
		id := strings.TrimSpace(c.QueryParam("id"))
		u, _ := h.userRepo.GetByID(id)
		if u == nil || !u.IsDeleted() {
			return
		}
		view := dupUsernameView{
			Kind: dup, UserID: u.UserID, Username: u.Username, Return: ret,
			Title:      "삭제된 아이디입니다",
			Body:       "아이디 " + u.Username + " 는 삭제된 계정입니다.",
			CanRestore: canAct,
		}
		if vision {
			view.FullName = u.FullName
			view.Mobile = u.Mobile
			view.DeletedAt = u.DeletedAt
			view.DeleteReason = u.DeleteReason
			view.OrgID = u.OrgID
			if h.orgRepo != nil {
				view.Orgs, _ = h.orgRepo.ListActive()
			}
		} else {
			view.Body = "비젼관리자에게 복구를 요청하세요."
		}
		data["DupUsername"] = view
		return
	}

	uname := strings.TrimSpace(c.QueryParam("username"))
	cf, err := h.userRepo.LookupUsernameConflict(uname, applyID)
	if err != nil || cf.Empty() || cf.DialogDup() == "" {
		return
	}
	view := dupUsernameView{
		Kind: cf.Kind, UserID: cf.UserID, Username: firstNonEmpty(uname, cf.Username),
		Return: ret, ApplyUserID: applyID,
		Title:      "예약된 아이디입니다",
		CanRelease: canAct,
		Warning:    "예약 해제는 되돌릴 수 없습니다. 같은 아이디를 다른 사람이 쓰게 됩니다.",
	}
	if vision {
		view.Body = cf.VisionMessage()
		view.FullName = cf.OwnerName
	} else {
		view.Body = cf.PublicMessage()
	}
	data["DupUsername"] = view
}

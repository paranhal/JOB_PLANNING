package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/config"
	"customer-support/internal/mailer"
	"customer-support/internal/model"
	"customer-support/internal/notify"
	"customer-support/internal/repository"
)

const loginHelpOK = "접수했습니다. 관리자가 연락드립니다."

func (h *AuthHandler) loginPageData(extra map[string]interface{}) map[string]interface{} {
	data := map[string]interface{}{
		"Title": "로그인", "Active": NavLogin, "HideNav": true,
	}
	if h.orgRepo != nil {
		if orgs, err := h.orgRepo.ListActive(); err == nil {
			data["Orgs"] = orgs
		}
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

func (h *AuthHandler) LoginPage(c echo.Context) error {
	return c.Render(http.StatusOK, "auth/login.html", h.loginPageData(nil))
}

func (h *AuthHandler) LoginHelp(c echo.Context) error {
	ok := func() error {
		return c.Render(http.StatusOK, "auth/login.html", h.loginPageData(map[string]interface{}{
			"HelpOK": loginHelpOK, "HelpOpen": true,
		}))
	}
	if strings.TrimSpace(c.FormValue("website")) != "" {
		return ok()
	}
	name := strings.TrimSpace(c.FormValue("name"))
	mobile := strings.TrimSpace(c.FormValue("mobile"))
	orgID := strings.TrimSpace(c.FormValue("org_id"))
	kind := strings.TrimSpace(c.FormValue("kind"))
	message := strings.TrimSpace(c.FormValue("message"))
	if name == "" || mobile == "" {
		return c.Render(http.StatusOK, "auth/login.html", h.loginPageData(map[string]interface{}{
			"HelpError": "이름과 핸드폰번호를 입력하세요.", "HelpOpen": true,
		}))
	}
	switch kind {
	case "unknown_id", "reset_pw", "other":
	default:
		kind = "other"
	}
	if h.helpRepo == nil {
		return ok()
	}
	now := time.Now()
	if n, err := h.helpRepo.CountMobileSince(mobile, now.Add(-10*time.Minute)); err == nil && n >= 1 {
		return c.Render(http.StatusOK, "auth/login.html", h.loginPageData(map[string]interface{}{
			"HelpError": "같은 번호로는 잠시 뒤에 다시 문의할 수 있습니다.", "HelpOpen": true,
		}))
	}
	if n, err := h.helpRepo.CountNameOnDay(name, now.Format("2006-01-02")); err == nil && n >= 3 {
		return c.Render(http.StatusOK, "auth/login.html", h.loginPageData(map[string]interface{}{
			"HelpError": "같은 이름으로 오늘은 더 이상 문의할 수 없습니다.", "HelpOpen": true,
		}))
	}

	matched := ""
	if h.userRepo != nil {
		if users, err := h.userRepo.ListAll(); err == nil {
			matched = repository.MatchUserByNameMobile(users, name, mobile)
		}
	}
	row := &repository.LoginHelpRequest{
		OrgID: orgID, Name: name, Mobile: mobile, Kind: kind, Message: message,
		ClientIP: c.RealIP(), UserAgent: c.Request().UserAgent(), MatchedUserID: matched,
	}
	if err := h.helpRepo.Create(row); err != nil {
		return c.Render(http.StatusOK, "auth/login.html", h.loginPageData(map[string]interface{}{
			"HelpError": "잠시 후 다시 시도하세요.", "HelpOpen": true,
		}))
	}

	accessLog(c, auditlog.Record{
		Action: auditlog.ActionLoginHelp, Result: auditlog.ResultOK,
		Detail: "계정 문의", Username: name, Reason: kind,
	})

	today := now.Format("2006-01-02")
	task := &model.WorkTask{
		WorkType:    model.WBWorkAdmin,
		Title:       "계정 문의: " + name,
		Description: fmt.Sprintf("이름 %s · 전화 %s · 조직 %s\n%s", name, mobile, orgID, message),
		DueDate:     today,
		WorkDate:    today,
		Status:      model.WBTaskWaiting,
		Priority:    model.WBPriorityHigh,
		SourceType:  "login_help",
		SourceID:    row.RequestID,
		OrgID:       orgID,
	}
	if task.OrgID == "" {
		task.OrgID = model.OrgIDLibrary
	}
	if h.wbRepo != nil {
		if orgAdmins := h.orgAdminNames(orgID); len(orgAdmins) > 0 {
			task.Assignee = orgAdmins[0]
		}
		if err := h.wbRepo.CreateTask(task); err == nil {
			_ = h.helpRepo.SetTaskID(row.RequestID, task.TaskID)
		}
	}

	mailCfg := mailer.FromApp(h.cfg)
	to := h.helpMailRecipients(orgID)
	body := fmt.Sprintf("계정 문의가 접수되었습니다.\n이름: %s\n전화: %s\n조직: %s\n구분: %s\n내용: %s\n",
		name, mobile, orgID, kind, message)
	if mailCfg.Enabled && strings.TrimSpace(to) != "" {
		_ = h.helpRepo.SetMailStatus(row.RequestID, "queued")
	}
	_ = notify.Notify(h.db, h.settingsRepo, notify.FromApp(h.cfg), notify.Message{
		Purpose: notify.PurposeLoginHelp, OrgID: orgID, ToEmail: to,
		Title: "계정 문의: " + name, Body: body, CreatedBy: "login_help",
	})
	return ok()
}

func (h *AuthHandler) orgAdminNames(orgID string) []string {
	if h.userRepo == nil {
		return nil
	}
	users, err := h.userRepo.ListAll()
	if err != nil {
		return nil
	}
	var names []string
	for i := range users {
		u := users[i]
		if !u.IsActive {
			continue
		}
		role := model.NormalizeRole(u.Role)
		if role != model.RoleOrgAdmin && role != model.RoleVisionAdmin {
			continue
		}
		if orgID != "" && role == model.RoleOrgAdmin && u.OrgID != orgID {
			continue
		}
		if strings.TrimSpace(u.FullName) != "" {
			names = append(names, u.FullName)
		}
	}
	return names
}

func (h *AuthHandler) helpMailRecipients(orgID string) string {
	var addrs []string
	cfg := h.cfg
	if cfg == nil {
		cfg = config.Load()
	}
	if s := strings.TrimSpace(cfg.MailAdminTo); s != "" {
		addrs = append(addrs, s)
	}
	if h.userRepo == nil {
		return strings.Join(addrs, ",")
	}
	users, err := h.userRepo.ListAll()
	if err != nil {
		return strings.Join(addrs, ",")
	}
	for i := range users {
		u := users[i]
		if !u.IsActive || strings.TrimSpace(u.Email) == "" {
			continue
		}
		role := model.NormalizeRole(u.Role)
		if role == model.RoleVisionAdmin {
			addrs = append(addrs, u.Email)
			continue
		}
		if role == model.RoleOrgAdmin && (orgID == "" || u.OrgID == orgID) {
			addrs = append(addrs, u.Email)
		}
	}
	seen := map[string]bool{}
	var uniq []string
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		uniq = append(uniq, a)
	}
	return strings.Join(uniq, ",")
}

func (h *Handler) HandleLoginHelp(c echo.Context) error {
	if !canSwitchOrg(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	note := strings.TrimSpace(c.FormValue("handle_note"))
	if note == "" {
		note = "계정 문의 처리"
	}
	st := strings.TrimSpace(c.FormValue("status"))
	if st != "ignored" {
		st = "done"
	}
	by := strings.TrimSpace(ctxString(c, "user_name"))
	if by == "" {
		by = strings.TrimSpace(ctxString(c, "username"))
	}
	row, err := h.help.Get(id)
	if err != nil || row == nil {
		return c.Redirect(http.StatusSeeOther, "/")
	}
	if err := h.help.Handle(id, by, note, st); err != nil {
		return err
	}
	if row.TaskID != "" && h.Workboard != nil && h.Workboard.repo != nil {
		if t, e := h.Workboard.repo.GetTask(row.TaskID); e == nil && t != nil {
			t.Status = model.WBTaskComplete
			t.CompleteNote = note
			t.CompleteDate = time.Now().Format("2006-01-02")
			_ = h.Workboard.repo.UpdateTask(t)
		}
	}
	if row.MatchedUserID != "" && h.Auth != nil && h.Auth.userRepo != nil {
		if u, e := h.Auth.userRepo.GetByID(row.MatchedUserID); e == nil && u != nil {
			_ = notify.Notify(h.Auth.db, h.Auth.settingsRepo, notify.FromApp(h.Auth.cfg), notify.Message{
				Purpose: notify.PurposeLoginHelp, OrgID: row.OrgID, ToUserID: u.UserID,
				ToEmail: u.Email, ToMobile: u.Mobile,
				Title: "계정 문의 처리 안내", Body: "문의하신 계정 건을 처리했습니다. " + note,
				CreatedBy: by,
			})
		}
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
		Detail: "계정 문의 처리", Reason: st,
	})
	back := strings.TrimSpace(c.FormValue("back"))
	if back == "" {
		back = "/"
	}
	return c.Redirect(http.StatusSeeOther, back)
}

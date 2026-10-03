package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/config"
	"customer-support/internal/mailer"
	"customer-support/internal/notify"
	"customer-support/internal/repository"
)

type MailAdminHandler struct {
	db  *sql.DB
	cfg *config.Config
}

func (h *MailAdminHandler) Page(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	cfg := mailer.FromApp(h.cfg)
	items, err := mailer.List(h.db, 100)
	if err != nil {
		items = nil
	}
	return c.Render(http.StatusOK, "admin/mail.html", map[string]interface{}{
		"Title": "메일 보낸 함", "Active": NavMail, "MailEnabled": cfg.Enabled, "Items": items,
	})
}

func (h *MailAdminHandler) Test(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	cfg := mailer.FromApp(h.cfg)
	if !cfg.Enabled {
		return c.Redirect(http.StatusSeeOther, "/admin/mail")
	}
	to := strings.TrimSpace(c.FormValue("to"))
	if to == "" {
		to = strings.TrimSpace(cfg.AdminTo)
	}
	by := strings.TrimSpace(ctxString(c, "username"))
	_, _ = mailer.Enqueue(h.db, cfg, mailer.Item{
		ToAddr: to, Subject: "테스트 메일", Body: "고객지원시스템 테스트 메일입니다.",
		Purpose: notify.PurposeTest, CreatedBy: by,
	})
	return c.Redirect(http.StatusSeeOther, "/admin/mail")
}

func (h *MailAdminHandler) Retry(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	_ = mailer.Requeue(h.db, c.Param("id"))
	return c.Redirect(http.StatusSeeOther, "/admin/mail")
}

type NotifyAdminHandler struct {
	db       *sql.DB
	cfg      *config.Config
	settings *repository.SettingsRepo
}

func (h *NotifyAdminHandler) Page(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	org := currentOrg(c)
	ncfg := notify.FromApp(h.cfg)
	ch := notify.ActiveChannels(ncfg, h.settings, org)
	sms, _ := notify.ListSMS(h.db, 100)
	sample := strings.TrimSpace(c.QueryParam("sample"))
	kind := ""
	if sample != "" {
		kind = notify.SMSKind(sample)
	}
	return c.Render(http.StatusOK, "admin/notify.html", map[string]interface{}{
		"Title":      "알림 설정",
		"Active":     NavNotify,
		"EnvMail":    ncfg.Mail.Enabled,
		"EnvSMS":     ncfg.SMSOn,
		"EnvKakao":   ncfg.Kakao,
		"MailOn":     ch.Mail,
		"SMSOn":      ch.SMS,
		"KakaoOn":    ch.Kakao,
		"OrgMail":    checkboxOn(h.settings, "notify.mail.org."+orgKey(org), true),
		"OrgSMS":     checkboxOn(h.settings, "notify.sms.org."+orgKey(org), false),
		"OrgKakao":   checkboxOn(h.settings, "notify.kakao.org."+orgKey(org), false),
		"PLogin":     checkboxOn(h.settings, "notify.purpose."+notify.PurposeLoginHelp, true),
		"PAS":        checkboxOn(h.settings, "notify.purpose."+notify.PurposeASUrgent, true),
		"PDay":       checkboxOn(h.settings, "notify.purpose."+notify.PurposeSameDay, true),
		"Items":      sms,
		"Sample":     sample,
		"SampleKind": kind,
	})
}

func (h *NotifyAdminHandler) Save(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	org := orgKey(currentOrg(c))
	put := func(key, form string) {
		v := "0"
		if c.FormValue(form) == "1" {
			v = "1"
		}
		_ = h.settings.Set(key, v)
	}
	put("notify.mail.org."+org, "mail_org")
	put("notify.sms.org."+org, "sms_org")
	put("notify.kakao.org."+org, "kakao_org")
	put("notify.purpose."+notify.PurposeLoginHelp, "p_login")
	put("notify.purpose."+notify.PurposeASUrgent, "p_as")
	put("notify.purpose."+notify.PurposeSameDay, "p_day")
	return c.Redirect(http.StatusSeeOther, "/admin/notify")
}

func (h *NotifyAdminHandler) Retry(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	_ = notify.RequeueSMS(h.db, c.Param("id"))
	return c.Redirect(http.StatusSeeOther, "/admin/notify")
}

func orgKey(org string) string {
	if strings.TrimSpace(org) == "" {
		return "*"
	}
	return org
}

func checkboxOn(s *repository.SettingsRepo, key string, def bool) bool {
	if s == nil {
		return def
	}
	v, _ := s.Get(key)
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true") || v == "on"
}

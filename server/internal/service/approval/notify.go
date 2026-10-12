package approval

import (
	"strings"

	"customer-support/internal/mailer"
)

func (e *Engine) enqueue(orgID, toUserID, by, title, body string) {
	if e == nil || e.DB == nil {
		return
	}
	email := ""
	_ = e.DB.QueryRow(`SELECT COALESCE(email,'') FROM users WHERE user_id=?`, toUserID).Scan(&email)
	if strings.TrimSpace(email) == "" {
		email = toUserID + "@queue.local"
	}
	_, _ = mailer.Enqueue(e.DB, mailer.Config{Enabled: true}, mailer.Item{
		OrgID:     orgID,
		ToAddr:    email,
		Subject:   title,
		Body:      body,
		Purpose:   "approval",
		CreatedBy: by,
	})
}

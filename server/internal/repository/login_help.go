package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

type LoginHelpRequest struct {
	RequestID     string
	OrgID         string
	Name          string
	Mobile        string
	Kind          string
	Message       string
	ClientIP      string
	UserAgent     string
	MatchedUserID string
	TaskID        string
	MailStatus    string
	Status        string
	HandledBy     string
	HandledAt     string
	HandleNote    string
	CreatedAt     string
}

type LoginHelpRepo struct {
	db *sql.DB
}

func NewLoginHelpRepo(db *sql.DB) *LoginHelpRepo {
	return &LoginHelpRepo{db: db}
}

func (r *LoginHelpRepo) Create(row *LoginHelpRequest) error {
	if r == nil || r.db == nil || row == nil {
		return fmt.Errorf("login_help 없음")
	}
	if strings.TrimSpace(row.RequestID) == "" {
		row.RequestID = "LH-" + uuid.NewString()
	}
	if strings.TrimSpace(row.Status) == "" {
		row.Status = "open"
	}
	_, err := r.db.Exec(`
		INSERT INTO login_help_requests (
			request_id, org_id, name, mobile, kind, message, client_ip, user_agent,
			matched_user_id, task_id, mail_status, status, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
		row.RequestID, row.OrgID, row.Name, row.Mobile, row.Kind, row.Message,
		row.ClientIP, row.UserAgent, row.MatchedUserID, row.TaskID, row.MailStatus, row.Status)
	return err
}

func (r *LoginHelpRepo) CountMobileSince(mobile string, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM login_help_requests WHERE mobile=? AND created_at >= datetime('now', '-10 minutes')`,
		mobile,
	).Scan(&n)
	return n, err
}

func (r *LoginHelpRepo) CountNameOnDay(name, day string) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM login_help_requests WHERE name=? AND date(created_at,'localtime') = ?`,
		name, day,
	).Scan(&n)
	return n, err
}

func (r *LoginHelpRepo) ListOpen(orgID string) ([]LoginHelpRequest, error) {
	q := `SELECT request_id, COALESCE(org_id,''), name, mobile, COALESCE(kind,''), COALESCE(message,''),
		COALESCE(matched_user_id,''), COALESCE(task_id,''), COALESCE(mail_status,''), COALESCE(status,''),
		COALESCE(handled_by,''), COALESCE(handled_at,''), COALESCE(handle_note,''), COALESCE(created_at,'')
		FROM login_help_requests WHERE status='open'`
	args := []any{}
	if orgID != "" && orgID != OrgAll {
		q += ` AND (org_id='' OR org_id=?)`
		args = append(args, orgID)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LoginHelpRequest
	for rows.Next() {
		var it LoginHelpRequest
		if err := rows.Scan(&it.RequestID, &it.OrgID, &it.Name, &it.Mobile, &it.Kind, &it.Message,
			&it.MatchedUserID, &it.TaskID, &it.MailStatus, &it.Status, &it.HandledBy, &it.HandledAt, &it.HandleNote, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *LoginHelpRepo) Get(id string) (*LoginHelpRequest, error) {
	it := &LoginHelpRequest{}
	err := r.db.QueryRow(`
		SELECT request_id, COALESCE(org_id,''), name, mobile, COALESCE(kind,''), COALESCE(message,''),
			COALESCE(client_ip,''), COALESCE(user_agent,''), COALESCE(matched_user_id,''), COALESCE(task_id,''),
			COALESCE(mail_status,''), COALESCE(status,''), COALESCE(handled_by,''), COALESCE(handled_at,''),
			COALESCE(handle_note,''), COALESCE(created_at,'')
		FROM login_help_requests WHERE request_id=?`, id).Scan(
		&it.RequestID, &it.OrgID, &it.Name, &it.Mobile, &it.Kind, &it.Message,
		&it.ClientIP, &it.UserAgent, &it.MatchedUserID, &it.TaskID, &it.MailStatus, &it.Status,
		&it.HandledBy, &it.HandledAt, &it.HandleNote, &it.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return it, err
}

func (r *LoginHelpRepo) Handle(id, by, note, status string) error {
	if status != "done" && status != "ignored" {
		status = "done"
	}
	_, err := r.db.Exec(`
		UPDATE login_help_requests SET status=?, handled_by=?, handled_at=CURRENT_TIMESTAMP, handle_note=?
		WHERE request_id=?`, status, by, note, id)
	return err
}

func (r *LoginHelpRepo) SetMailStatus(id, status string) error {
	_, err := r.db.Exec(`UPDATE login_help_requests SET mail_status=? WHERE request_id=?`, status, id)
	return err
}

func (r *LoginHelpRepo) SetTaskID(id, taskID string) error {
	_, err := r.db.Exec(`UPDATE login_help_requests SET task_id=? WHERE request_id=?`, taskID, id)
	return err
}

// PurgeHandledOlderThan 완료·무시 후 90일이 지난 건을 지우고 건수만 남긴다. §60.4
func PurgeHandledLoginHelp(db *sql.DB, olderThan time.Duration) (int, error) {
	if db == nil {
		return 0, nil
	}
	cut := time.Now().Add(-olderThan).Format("2006-01-02 15:04:05")
	res, err := db.Exec(`
		DELETE FROM login_help_requests
		WHERE status IN ('done','ignored') AND handled_at IS NOT NULL AND handled_at <= ?`, cut)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		audit.Use(db)
		audit.Log(audit.ActionDelete, "login_help_requests", "purge", "90d",
			fmt.Sprintf("계정문의 %d건 삭제", n), "", fmt.Sprintf(`{"count":%d}`, n))
	}
	return int(n), nil
}

func MatchUserByNameMobile(users []model.User, name, mobile string) string {
	wantName := strings.TrimSpace(name)
	wantMob := digitsOnly(mobile)
	if wantName == "" {
		return ""
	}
	for i := range users {
		u := users[i]
		if !u.IsActive {
			continue
		}
		if strings.TrimSpace(u.FullName) != wantName {
			continue
		}
		if wantMob != "" && digitsOnly(u.Mobile) != wantMob {
			continue
		}
		return u.UserID
	}
	return ""
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

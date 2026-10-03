package mailer

import (
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"customer-support/internal/config"
)

const MaxAttachBytes = 10 * 1024 * 1024

var ErrDisabled = errors.New("mail disabled")
var ErrAttachTooLarge = errors.New("첨부 10MB 초과")

type Config struct {
	Enabled  bool
	Host     string
	Port     string
	User     string
	Pass     string
	FromName string
	AdminTo  string
	Send     func(Config, Item) error
	Now      func() time.Time
}

func FromApp(cfg *config.Config) Config {
	if cfg == nil {
		return Config{}
	}
	return Config{
		Enabled:  cfg.MailEnabled,
		Host:     cfg.MailHost,
		Port:     cfg.MailPort,
		User:     cfg.MailUser,
		Pass:     cfg.MailPass,
		FromName: cfg.MailFromName,
		AdminTo:  cfg.MailAdminTo,
	}
}

type Item struct {
	MailID     string
	OrgID      string
	ToAddr     string
	CCAddr     string
	Subject    string
	Body       string
	BodyHTML   string
	AttachPath string
	Purpose    string
	Status     string
	TryCount   int
	LastError  string
	CreatedBy  string
	CreatedAt  string
	SentAt     string
	NextTryAt  string
}

func Enqueue(db *sql.DB, cfg Config, it Item) (string, error) {
	if !cfg.Enabled {
		return "", ErrDisabled
	}
	if db == nil {
		return "", fmt.Errorf("db 없음")
	}
	it.ToAddr = strings.TrimSpace(it.ToAddr)
	if it.ToAddr == "" {
		return "", fmt.Errorf("받는 주소 없음")
	}
	if strings.TrimSpace(it.AttachPath) != "" {
		st, err := os.Stat(it.AttachPath)
		if err != nil {
			return "", err
		}
		if st.Size() > MaxAttachBytes {
			return "", ErrAttachTooLarge
		}
	}
	if it.MailID == "" {
		it.MailID = "M-" + uuid.NewString()
	}
	if it.Status == "" {
		it.Status = "queued"
	}
	_, err := db.Exec(`
		INSERT INTO mail_outbox (mail_id, org_id, to_addr, cc_addr, subject, body, body_html, attach_path,
			purpose, status, try_count, last_error, created_by, created_at, next_try_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,0,'',?,CURRENT_TIMESTAMP,NULL)`,
		it.MailID, it.OrgID, it.ToAddr, it.CCAddr, it.Subject, it.Body, it.BodyHTML, it.AttachPath,
		it.Purpose, it.Status, it.CreatedBy)
	return it.MailID, err
}

func RetryWait(tryCount int) time.Duration {
	switch tryCount {
	case 0, 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 30 * time.Minute
	}
}

func Start(db *sql.DB, cfg Config) {
	go func() {
		_, _ = ProcessOnce(db, cfg)
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for range tick.C {
			if _, err := ProcessOnce(db, cfg); err != nil {
				log.Printf("메일 큐: %v", err)
			}
		}
	}()
}

func ProcessOnce(db *sql.DB, cfg Config) (int, error) {
	if db == nil {
		return 0, nil
	}
	if _, err := PurgeHandledLoginHelpBridge(db); err != nil {
		log.Printf("계정문의 정리: %v", err)
	}
	if !cfg.Enabled {
		return 0, nil
	}
	now := time.Now()
	if cfg.Now != nil {
		now = cfg.Now()
	}
	rows, err := db.Query(`
		SELECT mail_id, COALESCE(org_id,''), to_addr, COALESCE(cc_addr,''), subject, body, COALESCE(body_html,''),
			COALESCE(attach_path,''), COALESCE(purpose,''), COALESCE(status,''), COALESCE(try_count,0),
			COALESCE(created_by,''), COALESCE(next_try_at,'')
		FROM mail_outbox
		WHERE status='queued' AND COALESCE(try_count,0) < 3
			AND (next_try_at IS NULL OR TRIM(COALESCE(next_try_at,''))='' OR next_try_at <= ?)
		ORDER BY created_at ASC
		LIMIT 20`, now.Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.MailID, &it.OrgID, &it.ToAddr, &it.CCAddr, &it.Subject, &it.Body, &it.BodyHTML,
			&it.AttachPath, &it.Purpose, &it.Status, &it.TryCount, &it.CreatedBy, &it.NextTryAt); err != nil {
			return 0, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	send := cfg.Send
	if send == nil {
		send = SendSMTP
	}
	n := 0
	for _, it := range items {
		err := send(cfg, it)
		if err == nil {
			_, _ = db.Exec(`UPDATE mail_outbox SET status='sent', last_error='', sent_at=? WHERE mail_id=?`,
				now.Format("2006-01-02 15:04:05"), it.MailID)
			n++
			continue
		}
		msg := sanitizeErr(err)
		try := it.TryCount + 1
		st := "queued"
		next := now.Add(RetryWait(try)).Format("2006-01-02 15:04:05")
		if try >= 3 {
			st = "failed"
			next = ""
		}
		_, _ = db.Exec(`UPDATE mail_outbox SET status=?, try_count=?, last_error=?, next_try_at=? WHERE mail_id=?`,
			st, try, msg, nullEmpty(next), it.MailID)
	}
	return n, nil
}

func List(db *sql.DB, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT mail_id, COALESCE(org_id,''), to_addr, COALESCE(cc_addr,''), subject, body, COALESCE(body_html,''),
			COALESCE(attach_path,''), COALESCE(purpose,''), COALESCE(status,''), COALESCE(try_count,0),
			COALESCE(last_error,''), COALESCE(created_by,''), COALESCE(created_at,''), COALESCE(sent_at,'')
		FROM mail_outbox ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.MailID, &it.OrgID, &it.ToAddr, &it.CCAddr, &it.Subject, &it.Body, &it.BodyHTML,
			&it.AttachPath, &it.Purpose, &it.Status, &it.TryCount, &it.LastError, &it.CreatedBy, &it.CreatedAt, &it.SentAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func Requeue(db *sql.DB, id string) error {
	_, err := db.Exec(`UPDATE mail_outbox SET status='queued', try_count=0, last_error='', next_try_at=NULL WHERE mail_id=?`, id)
	return err
}

func SendSMTP(cfg Config, it Item) error {
	host := strings.TrimSpace(cfg.Host)
	port := strings.TrimSpace(cfg.Port)
	if host == "" {
		host = "smtp.gmail.com"
	}
	if port == "" {
		port = "587"
	}
	from := strings.TrimSpace(cfg.User)
	if from == "" {
		return fmt.Errorf("MAIL_USER 없음")
	}
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, 20*time.Second)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if cfg.User != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.User, cfg.Pass, host)); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, to := range splitAddr(it.ToAddr) {
		if err := client.Rcpt(to); err != nil {
			return err
		}
	}
	for _, to := range splitAddr(it.CCAddr) {
		if err := client.Rcpt(to); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if err := writeMessage(w, cfg, it); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func writeMessage(w io.Writer, cfg Config, it Item) error {
	fromName := strings.TrimSpace(cfg.FromName)
	if fromName == "" {
		fromName = "고객지원시스템"
	}
	fmt.Fprintf(w, "From: %s <%s>\r\n", fromName, cfg.User)
	fmt.Fprintf(w, "To: %s\r\n", it.ToAddr)
	if strings.TrimSpace(it.CCAddr) != "" {
		fmt.Fprintf(w, "Cc: %s\r\n", it.CCAddr)
	}
	fmt.Fprintf(w, "Subject: %s\r\n", it.Subject)
	fmt.Fprintf(w, "MIME-Version: 1.0\r\n")
	if strings.TrimSpace(it.AttachPath) == "" {
		fmt.Fprintf(w, "Content-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", it.Body)
		return nil
	}
	mw := multipart.NewWriter(w)
	fmt.Fprintf(w, "Content-Type: multipart/mixed; boundary=%s\r\n\r\n", mw.Boundary())
	pw, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=utf-8"}})
	if err != nil {
		return err
	}
	if _, err := io.WriteString(pw, it.Body); err != nil {
		return err
	}
	name := filepath.Base(it.AttachPath)
	f, err := os.Open(it.AttachPath)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Type", mime.TypeByExtension(filepath.Ext(name))+"; name=\""+name+"\"")
	hdr.Set("Content-Disposition", `attachment; filename="`+name+`"`)
	aw, err := mw.CreatePart(hdr)
	if err != nil {
		return err
	}
	if _, err := io.Copy(aw, io.LimitReader(f, MaxAttachBytes+1)); err != nil {
		return err
	}
	return mw.Close()
}

func splitAddr(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' })
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sanitizeErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

func nullEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// PurgeHandledLoginHelpBridge 순환 import 없이 repository 호출. mailer 고리에서 같이 돈다.
var PurgeHandledLoginHelpBridge = func(db *sql.DB) (int, error) { return 0, nil }

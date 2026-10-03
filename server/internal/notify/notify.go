package notify

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"customer-support/internal/config"
	"customer-support/internal/mailer"
	"customer-support/internal/repository"
)

const (
	PurposeLoginHelp = "login_help"
	PurposeASUrgent  = "as_urgent"
	PurposeSameDay   = "same_day"
	PurposeTest      = "test"
)

type Config struct {
	Mail    mailer.Config
	SMSOn   bool
	APIURL  string
	APIKey  string
	From    string
	Kakao   bool
	SendSMS func(Config, Item) error
	Now     func() time.Time
	Loc     *time.Location
}

func FromApp(cfg *config.Config) Config {
	if cfg == nil {
		return Config{}
	}
	return Config{
		Mail:   mailer.FromApp(cfg),
		SMSOn:  cfg.SMSEnabled,
		APIURL: cfg.SMSAPIURL,
		APIKey: cfg.SMSAPIKey,
		From:   cfg.SMSFrom,
		Kakao:  cfg.KakaoEnabled,
	}
}

type Message struct {
	Purpose    string
	OrgID      string
	ToUserID   string
	ToEmail    string
	ToMobile   string
	Title      string
	Body       string
	AttachPath string
	CreatedBy  string
}

type Item struct {
	SMSID         string
	OrgID         string
	ToAddr        string
	Subject       string
	Body          string
	Purpose       string
	Status        string
	TryCount      int
	LastError     string
	CreatedBy     string
	CreatedAt     string
	SentAt        string
	NextTryAt     string
	SMSKind       string
	KakaoFallback int
}

type Channels struct {
	Mail  bool
	SMS   bool
	Kakao bool
}

func ChannelOn(cfg Config, settings *repository.SettingsRepo, orgID, channel string) bool {
	orgID = strings.TrimSpace(orgID)
	key := "notify." + channel + ".org."
	if orgID == "" {
		orgID = "*"
	}
	val := ""
	if settings != nil {
		v, _ := settings.Get(key + orgID)
		val = strings.TrimSpace(v)
		if val == "" && orgID != "*" {
			v, _ = settings.Get(key + "*")
			val = strings.TrimSpace(v)
		}
	}
	envOn := false
	switch channel {
	case "mail":
		envOn = cfg.Mail.Enabled
		if val == "" {
			return envOn
		}
	case "sms":
		envOn = cfg.SMSOn
		if val == "" {
			return false
		}
	case "kakao":
		envOn = cfg.Kakao
		if val == "" {
			return false
		}
	}
	return envOn && (val == "1" || strings.EqualFold(val, "true") || val == "on")
}

func PurposeOn(settings *repository.SettingsRepo, purpose string) bool {
	defaults := map[string]bool{
		PurposeLoginHelp: true,
		PurposeASUrgent:  true,
		PurposeSameDay:   true,
	}
	on := defaults[purpose]
	if settings == nil {
		return on
	}
	v, _ := settings.Get("notify.purpose." + purpose)
	v = strings.TrimSpace(v)
	if v == "" {
		return on
	}
	return v == "1" || strings.EqualFold(v, "true") || v == "on"
}

func ActiveChannels(cfg Config, settings *repository.SettingsRepo, orgID string) Channels {
	return Channels{
		Mail:  ChannelOn(cfg, settings, orgID, "mail"),
		SMS:   ChannelOn(cfg, settings, orgID, "sms"),
		Kakao: ChannelOn(cfg, settings, orgID, "kakao"),
	}
}

func SMSByteLen(s string) int {
	n := 0
	for _, r := range s {
		if r <= 0x7F {
			n++
		} else {
			n += 2
		}
	}
	return n
}

func SMSKind(s string) string {
	if SMSByteLen(s) <= 90 {
		return "SMS"
	}
	return "LMS"
}

func NightHold(now time.Time) bool {
	h := now.Hour()
	return h >= 21 || h < 8
}

func MorningAt(now time.Time) time.Time {
	loc := now.Location()
	d := now
	if now.Hour() >= 21 {
		d = now.Add(24 * time.Hour)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 8, 0, 0, 0, loc)
}

func Notify(db *sql.DB, settings *repository.SettingsRepo, cfg Config, msg Message) error {
	if db == nil {
		return fmt.Errorf("db 없음")
	}
	if !PurposeOn(settings, msg.Purpose) && msg.Purpose != PurposeTest {
		return nil
	}
	ch := ActiveChannels(cfg, settings, msg.OrgID)
	if ch.Mail && strings.TrimSpace(msg.ToEmail) != "" {
		_, err := mailer.Enqueue(db, cfg.Mail, mailer.Item{
			OrgID:      msg.OrgID,
			ToAddr:     msg.ToEmail,
			Subject:    msg.Title,
			Body:       msg.Body,
			AttachPath: msg.AttachPath,
			Purpose:    msg.Purpose,
			CreatedBy:  msg.CreatedBy,
		})
		if err != nil && err != mailer.ErrDisabled {
			return err
		}
	}
	kakaoFailed := false
	if ch.Kakao {
		if err := sendKakao(msg); err != nil {
			kakaoFailed = true
		}
	}
	wantSMS := ch.SMS || (ch.Kakao && kakaoFallbackOn(ch) && kakaoFailed)
	if wantSMS {
		to := strings.TrimSpace(msg.ToMobile)
		it := Item{
			OrgID:         msg.OrgID,
			ToAddr:        to,
			Subject:       msg.Title,
			Body:          msg.Body,
			Purpose:       msg.Purpose,
			CreatedBy:     msg.CreatedBy,
			SMSKind:       SMSKind(msg.Body),
			KakaoFallback: 0,
		}
		if ch.Kakao && kakaoFailed {
			it.KakaoFallback = 1
		}
		if to == "" {
			it.Status = "failed"
			it.LastError = "연락처 없음"
			return insertSMS(db, it)
		}
		if !ch.SMS && it.KakaoFallback == 1 {
			it.Status = "failed"
			it.LastError = "알림톡 실패·문자 꺼짐"
			return insertSMS(db, it)
		}
		it.Status = "queued"
		return insertSMS(db, it)
	}
	return nil
}

func kakaoFallbackOn(ch Channels) bool {
	return true
}

func sendKakao(msg Message) error {
	return fmt.Errorf("알림톡 미연동")
}

func insertSMS(db *sql.DB, it Item) error {
	if it.SMSID == "" {
		it.SMSID = "S-" + uuid.NewString()
	}
	_, err := db.Exec(`
		INSERT INTO sms_outbox (sms_id, org_id, to_addr, cc_addr, subject, body, body_html, attach_path,
			purpose, status, try_count, last_error, created_by, created_at, next_try_at, sms_kind, kakao_fallback)
		VALUES (?,?,?,?,?,?,?,?,?,?,0,?,?,CURRENT_TIMESTAMP,NULL,?,?)`,
		it.SMSID, it.OrgID, it.ToAddr, "", it.Subject, it.Body, "", "",
		it.Purpose, it.Status, it.LastError, it.CreatedBy, it.SMSKind, it.KakaoFallback)
	return err
}

func Start(db *sql.DB, cfg Config) {
	go func() {
		_, _ = ProcessSMS(db, cfg)
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for range tick.C {
			if _, err := ProcessSMS(db, cfg); err != nil {
				log.Printf("문자 큐: %v", err)
			}
		}
	}()
}

func ProcessSMS(db *sql.DB, cfg Config) (int, error) {
	if db == nil || !cfg.SMSOn {
		return 0, nil
	}
	now := time.Now()
	if cfg.Now != nil {
		now = cfg.Now()
	}
	if cfg.Loc != nil {
		now = now.In(cfg.Loc)
	}
	rows, err := db.Query(`
		SELECT sms_id, COALESCE(org_id,''), to_addr, subject, body, COALESCE(purpose,''), COALESCE(try_count,0),
			COALESCE(created_by,''), COALESCE(next_try_at,''), COALESCE(sms_kind,''), COALESCE(kakao_fallback,0)
		FROM sms_outbox
		WHERE status='queued' AND COALESCE(try_count,0) < 3
			AND (next_try_at IS NULL OR TRIM(COALESCE(next_try_at,''))='' OR next_try_at <= ?)
		ORDER BY created_at ASC LIMIT 20`, now.Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.SMSID, &it.OrgID, &it.ToAddr, &it.Subject, &it.Body, &it.Purpose, &it.TryCount,
			&it.CreatedBy, &it.NextTryAt, &it.SMSKind, &it.KakaoFallback); err != nil {
			return 0, err
		}
		items = append(items, it)
	}
	send := cfg.SendSMS
	if send == nil {
		send = SendHTTP
	}
	n := 0
	for _, it := range items {
		if NightHold(now) {
			next := MorningAt(now).Format("2006-01-02 15:04:05")
			_, _ = db.Exec(`UPDATE sms_outbox SET next_try_at=? WHERE sms_id=?`, next, it.SMSID)
			continue
		}
		if strings.TrimSpace(it.ToAddr) == "" {
			_, _ = db.Exec(`UPDATE sms_outbox SET status='failed', last_error=? WHERE sms_id=?`, "연락처 없음", it.SMSID)
			continue
		}
		err := send(cfg, it)
		if err == nil {
			_, _ = db.Exec(`UPDATE sms_outbox SET status='sent', last_error='', sent_at=? WHERE sms_id=?`,
				now.Format("2006-01-02 15:04:05"), it.SMSID)
			n++
			continue
		}
		msg := err.Error()
		if utf8.RuneCountInString(msg) > 200 {
			msg = string([]rune(msg)[:200])
		}
		try := it.TryCount + 1
		st := "queued"
		next := now.Add(mailer.RetryWait(try)).Format("2006-01-02 15:04:05")
		if try >= 3 {
			st = "failed"
			next = ""
		}
		_, _ = db.Exec(`UPDATE sms_outbox SET status=?, try_count=?, last_error=?, next_try_at=? WHERE sms_id=?`,
			st, try, msg, nextOrNil(next), it.SMSID)
	}
	return n, rows.Err()
}

func nextOrNil(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func SendHTTP(cfg Config, it Item) error {
	if strings.TrimSpace(cfg.APIURL) == "" {
		return fmt.Errorf("SMS_API_URL 없음")
	}
	payload, _ := json.Marshal(map[string]string{
		"to": it.ToAddr, "from": cfg.From, "text": it.Body, "kind": SMSKind(it.Body),
	})
	req, err := http.NewRequest(http.MethodPost, cfg.APIURL, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("SMS HTTP %d %s", resp.StatusCode, string(b))
	}
	return nil
}

func ListSMS(db *sql.DB, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT sms_id, COALESCE(org_id,''), to_addr, subject, body, COALESCE(purpose,''), COALESCE(status,''),
			COALESCE(try_count,0), COALESCE(last_error,''), COALESCE(created_by,''), COALESCE(created_at,''),
			COALESCE(sent_at,''), COALESCE(sms_kind,''), COALESCE(kakao_fallback,0)
		FROM sms_outbox ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.SMSID, &it.OrgID, &it.ToAddr, &it.Subject, &it.Body, &it.Purpose, &it.Status,
			&it.TryCount, &it.LastError, &it.CreatedBy, &it.CreatedAt, &it.SentAt, &it.SMSKind, &it.KakaoFallback); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func RequeueSMS(db *sql.DB, id string) error {
	_, err := db.Exec(`UPDATE sms_outbox SET status='queued', try_count=0, last_error='', next_try_at=NULL WHERE sms_id=?`, id)
	return err
}

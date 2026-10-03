package notify

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/mailer"
	"customer-support/internal/repository"
)

func TestSMSKindLMS(t *testing.T) {
	short := strings.Repeat("가", 45)
	if SMSKind(short) != "SMS" {
		t.Fatal(SMSKind(short))
	}
	long := strings.Repeat("가", 50)
	if SMSKind(long) != "LMS" {
		t.Fatal(SMSKind(long), SMSByteLen(long))
	}
}

func TestEmptyMobileRecords(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sms.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	settings := repository.NewSettingsRepo(db)
	_ = settings.Set("notify.sms.org.*", "1")
	cfg := Config{SMSOn: true, Mail: mailer.Config{Enabled: false}}
	if err := Notify(db, settings, cfg, Message{Purpose: PurposeASUrgent, ToMobile: "", Title: "t", Body: "b", CreatedBy: "x"}); err != nil {
		t.Fatal(err)
	}
	var st, last string
	if err := db.QueryRow(`SELECT status, last_error FROM sms_outbox`).Scan(&st, &last); err != nil {
		t.Fatal(err)
	}
	if st != "failed" || last != "연락처 없음" {
		t.Fatalf("%s %s", st, last)
	}
}

func TestNightHold(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sms2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	settings := repository.NewSettingsRepo(db)
	_ = settings.Set("notify.sms.org.*", "1")
	cfg := Config{SMSOn: true, SendSMS: func(Config, Item) error { t.Fatal("야간에 보내면 안 됨"); return nil }}
	if err := Notify(db, settings, cfg, Message{Purpose: PurposeSameDay, ToMobile: "01012345678", Title: "t", Body: "b", CreatedBy: "x"}); err != nil {
		t.Fatal(err)
	}
	night := time.Date(2026, 10, 4, 22, 0, 0, 0, time.Local)
	cfg.Now = func() time.Time { return night }
	n, err := ProcessSMS(db, cfg)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var st, next string
	if err := db.QueryRow(`SELECT status, COALESCE(next_try_at,'') FROM sms_outbox`).Scan(&st, &next); err != nil {
		t.Fatal(err)
	}
	if st != "queued" || !strings.Contains(next, "08:00") {
		t.Fatalf("st=%s next=%s", st, next)
	}
}

func TestKakaoFallbackSMS(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sms3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	settings := repository.NewSettingsRepo(db)
	_ = settings.Set("notify.kakao.org.*", "1")
	_ = settings.Set("notify.sms.org.*", "1")
	cfg := Config{Kakao: true, SMSOn: true}
	if err := Notify(db, settings, cfg, Message{Purpose: PurposeLoginHelp, ToMobile: "01000001111", Title: "t", Body: "hello", CreatedBy: "x"}); err != nil {
		t.Fatal(err)
	}
	var fb int
	if err := db.QueryRow(`SELECT kakao_fallback FROM sms_outbox`).Scan(&fb); err != nil {
		t.Fatal(err)
	}
	if fb != 1 {
		t.Fatalf("fallback=%d", fb)
	}
}

func TestChannelRequiresBoth(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sms4.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	settings := repository.NewSettingsRepo(db)
	cfg := Config{SMSOn: true}
	if ChannelOn(cfg, settings, "O01", "sms") {
		t.Fatal("체크박스 없이 켜지면 안 됨")
	}
}

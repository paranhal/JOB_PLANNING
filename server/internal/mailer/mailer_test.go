package mailer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/repository"
)

func TestEnqueueDisabled(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = Enqueue(db, Config{Enabled: false}, Item{ToAddr: "a@b.c", Subject: "s", Body: "b", CreatedBy: "t"})
	if err != ErrDisabled {
		t.Fatalf("err=%v", err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM mail_outbox`).Scan(&n)
	if n != 0 {
		t.Fatalf("queued=%d", n)
	}
}

func TestEnqueueAndRetry(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "mail2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := Config{Enabled: true, Send: func(Config, Item) error { return os.ErrInvalid }}
	id, err := Enqueue(db, cfg, Item{ToAddr: "a@b.c", Subject: "s", Body: "hello", CreatedBy: "tester"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 4, 10, 0, 0, 0, time.Local)
	cfg.Now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		cfg.Now = func() time.Time { return now.Add(time.Duration(i) * time.Hour) }
		_, _ = ProcessOnce(db, cfg)
	}
	var st, by string
	var tries int
	if err := db.QueryRow(`SELECT status, try_count, created_by FROM mail_outbox WHERE mail_id=?`, id).Scan(&st, &tries, &by); err != nil {
		t.Fatal(err)
	}
	if st != "failed" || tries < 3 {
		t.Fatalf("status=%s tries=%d", st, tries)
	}
	if by != "tester" {
		t.Fatalf("created_by=%s", by)
	}
}

func TestWriteMessageEncodesKoreanHeaders(t *testing.T) {
	var buf strings.Builder
	cfg := Config{User: "hjlee@visionit.co.kr", FromName: "비젼아이티 고객지원시스템"}
	if err := writeMessage(&buf, cfg, Item{MailID: "M-test", ToAddr: "a@b.c", Subject: "테스트 메일", Body: "본문"}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "From: 비젼") || strings.Contains(got, "Subject: 테스트") {
		t.Fatalf("한글 헤더가 인코딩되지 않았다:\n%s", got)
	}
	if !strings.Contains(got, "=?utf-8?b?") || !strings.Contains(got, "Message-ID: <M-test@visionit.co.kr>") {
		t.Fatalf("헤더 부족:\n%s", got)
	}
}

func TestSMTPImplicitTLSPort(t *testing.T) {
	if !smtpImplicitTLS("465") || smtpImplicitTLS("587") || smtpImplicitTLS("") {
		t.Fatal("465 만 암시적 SSL, 587 은 STARTTLS")
	}
}

func TestRetryWait(t *testing.T) {
	if RetryWait(1) != time.Minute || RetryWait(2) != 5*time.Minute || RetryWait(3) != 30*time.Minute {
		t.Fatal(RetryWait(1), RetryWait(2), RetryWait(3))
	}
}

func TestAttachTooLarge(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "mail3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	p := filepath.Join(t.TempDir(), "big.bin")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxAttachBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	_, err = Enqueue(db, Config{Enabled: true}, Item{ToAddr: "a@b.c", Subject: "s", Body: "b", AttachPath: p, CreatedBy: "t"})
	if err != ErrAttachTooLarge {
		t.Fatalf("err=%v", err)
	}
}

func TestProcessSends(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "mail4.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	sent := 0
	cfg := Config{Enabled: true, Send: func(Config, Item) error { sent++; return nil }}
	if _, err := Enqueue(db, cfg, Item{ToAddr: "a@b.c", Subject: "s", Body: "ok", CreatedBy: "t"}); err != nil {
		t.Fatal(err)
	}
	n, err := ProcessOnce(db, cfg)
	if err != nil || n != 1 || sent != 1 {
		t.Fatalf("n=%d sent=%d err=%v", n, sent, err)
	}
}

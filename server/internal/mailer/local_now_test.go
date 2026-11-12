package mailer

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_133MailOutboxLocalNow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 6, 16, 48, 45, 0, loc)
	restore := model.SetNowForTest(func() time.Time { return fixed })
	defer restore()

	db, err := repository.InitDB(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := Enqueue(db, Config{Enabled: true}, Item{
		ToAddr: "a@example.com", Subject: "s", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	var created string
	if err := db.QueryRow(`SELECT COALESCE(created_at,'') FROM mail_outbox WHERE mail_id=?`, id).Scan(&created); err != nil {
		t.Fatal(err)
	}
	if created != "2026-11-06 16:48:45" {
		t.Fatalf("mail created_at=%q", created)
	}
}

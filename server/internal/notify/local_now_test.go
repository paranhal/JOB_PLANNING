package notify

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_133SMSOutboxLocalNow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 6, 16, 48, 45, 0, loc)
	restore := model.SetNowForTest(func() time.Time { return fixed })
	defer restore()

	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sms.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := insertSMS(db, Item{ToAddr: "01012345678", Subject: "s", Body: "b", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	var created string
	if err := db.QueryRow(`SELECT COALESCE(created_at,'') FROM sms_outbox LIMIT 1`).Scan(&created); err != nil {
		t.Fatal(err)
	}
	if created != "2026-11-06 16:48:45" {
		t.Fatalf("sms created_at=%q", created)
	}
}

package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_133UsernameReservationLocalNow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 6, 16, 48, 45, 0, loc)
	restore := model.SetNowForTest(func() time.Time { return fixed })
	defer restore()

	db, err := InitDB(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	acc := NewAccountRepo(db)
	if err := acc.Reserve("foo-user", "USR-1", model.ReserveCurrent); err != nil {
		t.Fatal(err)
	}
	var created string
	if err := db.QueryRow(`SELECT COALESCE(created_at,'') FROM username_reservations WHERE username_norm=?`, "foo-user").Scan(&created); err != nil {
		t.Fatal(err)
	}
	if created != "2026-11-06 16:48:45" {
		t.Fatalf("reservation created_at=%q", created)
	}
}

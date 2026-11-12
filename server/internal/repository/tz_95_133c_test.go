package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

// 「오늘」의 경계는 서버 TZ 와 무관하게 한국 날짜다(UTC 15:30 = KST 다음 날 00:30).
func TestTodayBoundsFollowKST(t *testing.T) {
	prev := time.Local
	defer func() { time.Local = prev }()
	restore := model.SetNowForTest(func() time.Time { return time.Date(2026, 11, 5, 15, 30, 0, 0, time.UTC) })
	defer restore()
	for _, local := range []*time.Location{time.UTC, model.KST(), time.FixedZone("X", -5*3600)} {
		time.Local = local
		if got := todayTimeStart(); got != "2026-11-06 00:00:00" {
			t.Fatalf("Local=%s todayTimeStart=%s", local, got)
		}
		if got := tomorrowTimeStart(); got != "2026-11-07 00:00:00" {
			t.Fatalf("Local=%s tomorrowTimeStart=%s", local, got)
		}
	}
}

// 로그인 문의 「같은 이름 하루 3건」은 UTC 로 저장된 created_at 을 한국 날짜로 센다(서버 TZ 무관).
func TestLoginHelpCountNameOnDayUsesKSTDay(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "lh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewLoginHelpRepo(db)
	if err := r.Create(&LoginHelpRequest{RequestID: "LH-1", Name: "홍길동", Mobile: "01012345678", Kind: "forgot", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	// UTC 2026-11-05 15:30 = KST 2026-11-06 00:30
	if _, err := db.Exec(`UPDATE login_help_requests SET created_at='2026-11-05 15:30:00' WHERE request_id='LH-1'`); err != nil {
		t.Fatal(err)
	}
	prev := time.Local
	defer func() { time.Local = prev }()
	for _, local := range []*time.Location{time.UTC, model.KST(), time.FixedZone("X", -5*3600)} {
		time.Local = local
		if n, err := r.CountNameOnDay("홍길동", "2026-11-06"); err != nil || n != 1 {
			t.Fatalf("Local=%s 한국 날짜 11-06: n=%d err=%v", local, n, err)
		}
		if n, err := r.CountNameOnDay("홍길동", "2026-11-05"); err != nil || n != 0 {
			t.Fatalf("Local=%s UTC 날짜 11-05 로 세면 안 된다: n=%d err=%v", local, n, err)
		}
	}
}

package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

// 95-133: 시간대 없는 저장 문자열은 서버 TZ 와 무관하게 한국 시각으로 읽는다.
func TestParseTimeNaiveIsKST(t *testing.T) {
	prev := time.Local
	defer func() { time.Local = prev }()
	for _, local := range []*time.Location{time.UTC, model.KST(), time.FixedZone("X", -5*3600)} {
		time.Local = local
		for _, in := range []string{"2026-11-06 16:48:45", "2026-11-06T16:48:45", "2026-11-06 16:48:45.123456"} {
			got := parseTime(in)
			if got.IsZero() {
				t.Fatalf("Local=%s %q 못 읽음", local, in)
			}
			if f := got.Format("2006-01-02 15:04:05"); f[:19] != "2026-11-06 16:48:45" {
				t.Fatalf("Local=%s %q → %s", local, in, f)
			}
			if got.Unix() != time.Date(2026, 11, 6, 7, 48, 45, 0, time.UTC).Unix() {
				t.Fatalf("Local=%s %q 순간이 KST 16:48:45 와 다름: %s", local, in, got.UTC())
			}
		}
		if g := parseTime("2026-11-06 16:48"); g.Format("15:04") != "16:48" {
			t.Fatalf("분 단위 %s", g)
		}
		if g := parseTime("2026-11-06"); g.Format("2006-01-02 15:04") != "2026-11-06 00:00" {
			t.Fatalf("날짜만 %s", g)
		}
	}
}

// 드라이버가 붙인 가짜 Z(·+00:00)는 숫자를 그대로 한국 시각으로 읽고,
// 0이 아닌 오프셋은 진짜 시간대이므로 순간을 지켜 한국 시각으로 바꾼다.
func TestParseTimeAwareKeepsDriverZDigits(t *testing.T) {
	prev := time.Local
	defer func() { time.Local = prev }()
	for _, local := range []*time.Location{time.UTC, model.KST()} {
		time.Local = local
		for _, in := range []string{"2026-11-06T16:48:45Z", "2026-11-06 16:48:45+00:00"} {
			g := parseTime(in)
			if g.IsZero() || g.Format("2006-01-02 15:04:05") != "2026-11-06 16:48:45" {
				t.Fatalf("Local=%s %q → %s (숫자가 바뀌면 안 된다)", local, in, g)
			}
			if g.Unix() != time.Date(2026, 11, 6, 7, 48, 45, 0, time.UTC).Unix() {
				t.Fatalf("Local=%s %q 순간이 KST 16:48:45 와 다름", local, in)
			}
		}
	}
	g := parseTime("2026-11-06T07:48:45-02:00") // 진짜 오프셋: 09:48:45Z → KST 18:48:45
	if g.IsZero() || g.Format("2006-01-02 15:04:05") != "2026-11-06 18:48:45" {
		t.Fatalf("-02:00 → %s", g)
	}
	g = parseTime("2026-11-06T16:48:45+09:00")
	if g.IsZero() || g.Format("15:04") != "16:48" {
		t.Fatalf("+09:00 → %s", g)
	}
	if !parseTime("").IsZero() || !parseTime("쓰레기").IsZero() {
		t.Fatal("빈 값·쓰레기는 zero")
	}
}

// 실제 드라이버 경로: DATETIME 컬럼에 현지 시각을 넣고 문자열로 읽어 parseTime 해도 숫자가 그대로다.
func TestParseTimeRoundTripThroughDriver(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "rt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO mail_outbox(mail_id, to_addr, subject, body, created_at) VALUES ('RT','a@b.c','s','b','2026-11-06 16:48:45')`); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.QueryRow(`SELECT created_at FROM mail_outbox WHERE mail_id='RT'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if g := parseTime(raw); g.Format("2006-01-02 15:04:05") != "2026-11-06 16:48:45" {
		t.Fatalf("드라이버가 준 %q → %s", raw, g)
	}
}

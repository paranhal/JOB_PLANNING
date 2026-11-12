package model

import (
	"testing"
	"time"
)

func TestLocalNowStringAndDisplay(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 6, 16, 48, 45, 0, loc)
	restore := SetNowForTest(func() time.Time { return fixed })
	defer restore()

	got := LocalNowString()
	if got != "2026-11-06 16:48:45" {
		t.Fatalf("LocalNowString=%q", got)
	}
	if FormatLocalDateTime(got) != "2026-11-06 16:48" {
		t.Fatalf("표시=%q", FormatLocalDateTime(got))
	}
	if FormatLocalDateTime("2026-11-06T07:48:45") != "2026-11-06 07:48" {
		t.Fatalf("T 구분 표시")
	}
	if s := FormatLocalDateTime("2026-11-06 16:48"); s != "2026-11-06 16:48" {
		t.Fatalf("이미 분까지=%q", s)
	}
}

package model

import (
	"testing"
	"time"
)

// 95-133: 서버 TZ(time.Local)가 UTC 여도 시각 문자열은 한국 시각으로 읽고 보여 준다.
func TestKSTIsSeoul(t *testing.T) {
	loc := KST()
	got := time.Date(2026, 11, 6, 7, 48, 45, 0, time.UTC).In(loc).Format("2006-01-02 15:04:05")
	if got != "2026-11-06 16:48:45" {
		t.Fatalf("UTC 07:48:45 → KST 기대 16:48:45, got %s", got)
	}
}

// 완료 일시를 조치 세트로 옮길 때 +9시간이 붙지 않는다. 예전엔 UTC 로 읽은 값을 .In(time.Local) 해서 밀렸다.
func TestVirtualActionSetCompleteTimeNotShifted(t *testing.T) {
	prev := time.Local
	defer func() { time.Local = prev }()
	for _, local := range []*time.Location{time.UTC, KST(), time.FixedZone("X", -5*3600)} {
		time.Local = local
		done := time.Date(2026, 11, 6, 16, 48, 0, 0, KST())
		s := VirtualActionSetFromReceipt(&ASReceipt{CompleteDatetime: &done})
		if s.EndTime != "16:48" || s.WorkDate != "2026-11-06" || s.CompleteDatetime != "2026-11-06T16:48" {
			t.Fatalf("Local=%s: end=%s date=%s complete=%s", local, s.EndTime, s.WorkDate, s.CompleteDatetime)
		}
	}
}

// LocalNowString 은 서버 TZ(time.Local)와 무관하게 한국 시각이다.
func TestLocalNowStringIgnoresProcessTZ(t *testing.T) {
	prev := time.Local
	defer func() { time.Local = prev }()
	fixed := time.Date(2026, 11, 6, 7, 48, 45, 0, time.UTC)
	restore := SetNowForTest(func() time.Time { return fixed })
	defer restore()
	for _, local := range []*time.Location{time.UTC, KST(), time.FixedZone("X", -5*3600)} {
		time.Local = local
		if got := LocalNowString(); got != "2026-11-06 16:48:45" {
			t.Fatalf("Local=%s: %s", local, got)
		}
	}
}

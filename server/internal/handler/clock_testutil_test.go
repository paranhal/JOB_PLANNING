package handler

import (
	"os"
	"testing"
	"time"

	"customer-support/internal/appclock"
)

// pinAppClock 테스트 시각을 고정한다. 해넘이·당월 삭제 보호에 쓴다.
func pinAppClock(t *testing.T, tm time.Time) {
	t.Helper()
	t.Cleanup(appclock.SetFixed(tm))
}

func pinClockBeforeNov2026(t *testing.T) {
	t.Helper()
	pinAppClock(t, time.Date(2026, 10, 15, 9, 0, 0, 0, time.Local))
}

func skipIfNoPlaywrightBrowser(t *testing.T) {
	t.Helper()
	// 사유: Playwright 채널 msedge 없음. 담당 95-93. 만료 2026-11-17.
	if _, err := os.Stat("/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"); err != nil {
		t.Skip("msedge 없음 — Playwright 격리. 95-93 / 2026-11-17")
	}
}

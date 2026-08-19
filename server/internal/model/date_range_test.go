package model

import (
	"strings"
	"testing"
)

func TestNormalizeAppDateYearRange(t *testing.T) {
	if NormalizeAppDate("") != "" {
		t.Fatal("빈 값")
	}
	if got := NormalizeAppDate("2026-08-12"); got != "2026-08-12" {
		t.Fatalf("정상: %q", got)
	}
	if NormalizeAppDate("0206-08-12") != "" {
		t.Fatal("0206 은 거절해야 한다")
	}
	if NormalizeAppDate("1999-12-31") != "" {
		t.Fatal("1999 거절")
	}
	if NormalizeAppDate("2101-01-01") != "" {
		t.Fatal("2101 거절")
	}
	if _, err := ParseAppDate("0206-08-12"); err != ErrAppDateYear {
		t.Fatalf("ParseAppDate: %v", err)
	}
	if err := RequireAppDateYear("0206-08-12 10:00:00"); err != ErrAppDateYear {
		t.Fatalf("datetime: %v", err)
	}
	if err := RequireAppDateYear("2026-08-12 10:00:00"); err != nil {
		t.Fatal(err)
	}
}

func TestUnplannedDelayedBadgeStarted(t *testing.T) {
	red := UnplannedBadgeOfStarted(UnplannedDelayed, 3, false)
	if red.Label != "D+3" || !strings.Contains(red.Class, "red") {
		t.Fatalf("미착수: %+v", red)
	}
	yel := UnplannedBadgeOfStarted(UnplannedDelayed, 3, true)
	if yel.Label != "진행중 D+3" || !strings.Contains(yel.Class, "amber") {
		t.Fatalf("착수: %+v", yel)
	}
}

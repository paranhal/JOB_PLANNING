package leavecalc

import (
	"testing"
	"time"
)

func TestFormatAndNextDocNo(t *testing.T) {
	day := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	if got := FormatDocNo("VI", day, 1); got != "VI-근태-260807-001" {
		t.Fatalf("format=%s", got)
	}
	if got := NextDocNo("VI", day, nil); got != "VI-근태-260807-001" {
		t.Fatalf("empty next=%s", got)
	}
	existing := []string{"VI-근태-260807-001", "VI-근태-260807-002", "other-근태-260807-009", "VI-근태-260808-001"}
	if got := NextDocNo("VI", day, existing); got != "VI-근태-260807-003" {
		t.Fatalf("next=%s", got)
	}
	nextDay := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	if got := NextDocNo("VI", nextDay, existing); got != "VI-근태-260808-002" {
		t.Fatalf("date rollover=%s", got)
	}
}

func TestNextDocSeqIgnoresMalformed(t *testing.T) {
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	n := NextDocSeq("VI", day, []string{"not-a-number", "VI-근태-260102-010", "VI-근태-260102-x"})
	if n != 11 {
		t.Fatalf("seq=%d", n)
	}
}

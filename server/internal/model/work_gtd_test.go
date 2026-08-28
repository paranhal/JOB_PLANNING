package model

import (
	"strings"
	"testing"
	"time"
)

func TestIsAdminGTDTask(t *testing.T) {
	if !IsAdminGTDTask(WorkTask{WorkType: WBWorkAdmin}) {
		t.Fatal("admin")
	}
	if !IsAdminGTDTask(WorkTask{WorkType: WBWorkSupport}) {
		t.Fatal("support")
	}
	if IsAdminGTDTask(WorkTask{WorkType: WBWorkAdmin, SourceType: WBSourceAS}) {
		t.Fatal("AS source must be excluded")
	}
	if IsAdminGTDTask(WorkTask{WorkType: WBWorkAS, SourceType: WBSourceAS}) {
		t.Fatal("AS work type")
	}
}

func TestWBAdminStatusLabel(t *testing.T) {
	if WBAdminStatusLabel(WBTaskWaiting) != "할 일" {
		t.Fatal("waiting")
	}
	if WBAdminStatusLabel(WBTaskWaitingFor) != "회신 대기" {
		t.Fatal("waiting_for")
	}
	if WBAdminStatusLabel(WBTaskInbox) != "수집함" {
		t.Fatal("inbox leftover label")
	}
	sort, dir := NormalizeAdminWorkSort("", "")
	if sort != "due_date" || dir != "asc" {
		t.Fatalf("default sort=%s dir=%s", sort, dir)
	}
	sort, dir = NormalizeAdminWorkSort("title", "desc")
	if sort != "title" || dir != "desc" {
		t.Fatalf("title sort=%s dir=%s", sort, dir)
	}
	if !strings.Contains(AdminWorkOrderSQL("due_date", "asc"), "due_date") {
		t.Fatal("order sql")
	}
}

func TestAdminGTDErr(t *testing.T) {
	if got := AdminGTDErr(WBTaskHold, "", "2026-08-20", "", "", "", "", "", "", 0, 0, false, false, ""); got != "hold_required" {
		t.Fatalf("hold empty reason: %s", got)
	}
	if got := AdminGTDErr(WBTaskWaitingFor, "", "", "", "업체", "견적 요청", "", "", "", 0, 0, false, false, ""); got != "waiting_for" {
		t.Fatalf("waiting_for no date: %s", got)
	}
	if got := AdminGTDErr(WBTaskWaitingFor, "", "", "", "업체", "견적 요청", "2026-08-20", "", "", 0, 0, false, false, ""); got != "" {
		t.Fatalf("waiting_for ok: %s", got)
	}
	if got := AdminGTDErr(WBTaskComplete, "", "", "", "", "", "", "", "", 1, 0, false, false, ""); got != "complete_note" {
		t.Fatalf("complete no note: %s", got)
	}
	if got := AdminGTDErr(WBTaskComplete, "", "", "", "", "", "", "", "초안 발송", 1, 0, false, false, ""); got != "complete_block" {
		t.Fatalf("complete blocked: %s", got)
	}
	if got := AdminGTDErr(WBTaskComplete, "", "", "", "", "", "", "", "초안 발송", 1, 0, true, true, ""); got != "force_reason" {
		t.Fatalf("force no reason: %s", got)
	}
	if got := AdminGTDErr(WBTaskComplete, "", "", "", "", "", "", "", "초안 발송", 1, 0, true, true, "긴급 마감"); got != "" {
		t.Fatalf("force ok: %s", got)
	}
	if got := AdminGTDErr(WBTaskCancelled, "", "", "", "", "", "", "", "", 0, 0, false, false, ""); got != "cancel_reason" {
		t.Fatalf("cancel: %s", got)
	}
}

func TestActionProgressPct(t *testing.T) {
	if _, ok := ActionProgressPct(0, 0); ok {
		t.Fatal("분모 0이면 표시하지 않는다")
	}
	pct, ok := ActionProgressPct(1, 3)
	if !ok || pct < 33 || pct > 34 {
		t.Fatalf("1/3=%v ok=%v", pct, ok)
	}
	pct, ok = ActionProgressPct(3, 3)
	if !ok || pct != 100 {
		t.Fatalf("3/3=%v", pct)
	}
}

func TestAdminLeadTimeDays(t *testing.T) {
	d, ok := AdminLeadTimeDays("2026-08-13", "2026-08-14")
	if !ok || d != 1 {
		t.Fatalf("lead=%d ok=%v want 1", d, ok)
	}
	if _, ok := AdminLeadTimeDays("2026-08-13", ""); ok {
		t.Fatal("완료일 없으면 표시하지 않는다")
	}
}

func TestMergeDateIntervalsOverlapOnce(t *testing.T) {
	day := func(h int) time.Time {
		return time.Date(2026, 8, 13, h, 0, 0, 0, time.Local)
	}
	// 3개사 동시 요청 08-13 09시 → 08-14 09시
	spans := []DateInterval{
		{Start: day(9), End: day(9).Add(24 * time.Hour)},
		{Start: day(9), End: day(9).Add(24 * time.Hour)},
		{Start: day(10), End: day(9).Add(26 * time.Hour)},
	}
	got := IntervalDays(spans)
	if got < 1.08 || got > 1.12 { // 09:00→익일 11:00 = 26h = 1.083d
		t.Fatalf("merged days=%v want ~1.08 (3개사를 3일로 세면 안 됨)", got)
	}
	if IntervalDays(nil) != 0 {
		t.Fatal("empty")
	}
}

func TestPairActionWaitIntervals(t *testing.T) {
	acts := []WorkActivity{
		{ActivityType: WBActivitySend, CreatedAt: "2026-08-13 09:00:00"},
		{ActivityType: WBActivityReply, CreatedAt: "2026-08-13 18:00:00"},
		{ActivityType: WBActivityFollow, CreatedAt: "2026-08-14 09:00:00"},
		{ActivityType: WBActivityReply, CreatedAt: "2026-08-14 15:00:00"},
	}
	iv := PairActionWaitIntervals(acts, time.Time{}, time.Time{})
	days := IntervalDays(iv)
	// 9h + 6h = 15h = 0.625d
	if days < 0.62 || days > 0.63 {
		t.Fatalf("pair days=%v want 0.625", days)
	}
}

func TestExternalWaitSharePct(t *testing.T) {
	if _, ok := ExternalWaitSharePct(1, 0); ok {
		t.Fatal("분모 0")
	}
	pct, ok := ExternalWaitSharePct(1, 2)
	if !ok || pct != 50 {
		t.Fatalf("1/2 days share=%v ok=%v", pct, ok)
	}
	pct, ok = ExternalWaitSharePct(3, 2)
	if !ok || pct != 100 {
		t.Fatalf("대기>리드타임 cap=%v", pct)
	}
}

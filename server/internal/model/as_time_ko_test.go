package model

import (
	"strings"
	"testing"
)

func TestFormatKoreanTimeTable(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"00:00", "오전 12:00"},
		{"09:00", "오전 09:00"},
		{"12:00", "오후 12:00"},
		{"13:30", "오후 01:30"},
		{"23:50", "오후 11:50"},
	}
	for _, c := range cases {
		if got := FormatKoreanTime(c.in); got != c.want {
			t.Errorf("%s → %s want %s", c.in, got, c.want)
		}
	}
}

func TestAddMinutesClockBounds(t *testing.T) {
	t.Parallel()
	end, ok := AddMinutesClock("09:00", 90)
	if !ok || end != "10:30" {
		t.Fatalf("09:00+90 → %s ok=%v", end, ok)
	}
	if _, ok := AddMinutesClock("23:50", 10); ok {
		t.Fatal("23:50+10분은 거부되어야 한다")
	}
}

func TestTimeSlotAndDurationCounts(t *testing.T) {
	t.Parallel()
	if n := len(TimeSlotOptions()); n != 144 {
		t.Fatalf("시작 슬롯 %d", n)
	}
	if n := len(DurationMinOptions()); n != 72 {
		t.Fatalf("작업 시간 %d", n)
	}
	for _, o := range TimeSlotOptions() {
		if o.Value == "" || !IsTenMinuteClock(o.Value) {
			t.Fatalf("슬롯 value %q", o.Value)
		}
		if got := FormatKoreanTime(o.Value); got != o.Label {
			t.Fatalf("라벨 %s ≠ %s", o.Label, got)
		}
		if !strings.HasPrefix(o.Label, "오전 ") && !strings.HasPrefix(o.Label, "오후 ") {
			t.Fatalf("오전/오후 아님 %q", o.Label)
		}
	}
}

func TestValidateActionSetWorkTime(t *testing.T) {
	if code := ValidateActionSetWorkTime(ASActionSet{StartTime: "09:00"}); code != "work_time_pair" {
		t.Fatalf("시작만: %s", code)
	}
	if code := ValidateActionSetWorkTime(ASActionSet{DurationMin: 30}); code != "work_time_pair" {
		t.Fatalf("시간만: %s", code)
	}
	if code := ValidateActionSetWorkTime(ASActionSet{StartTime: "23:50", DurationMin: 10}); code != "work_time_midnight" {
		t.Fatalf("자정: %s", code)
	}
	s := ASActionSet{StartTime: "09:00", DurationMin: 60, LeaveSame: false, LeaveTime: "09:30", WorkPlace: WorkPlaceField}
	if code := ValidateActionSetWorkTime(s); code != "leave_before_end" {
		t.Fatalf("철수<완료: %s", code)
	}
	s.LeaveTime = "10:07"
	if code := ValidateActionSetWorkTime(s); code != "leave_time_step" {
		t.Fatalf("10분: %s", code)
	}
	s.LeaveTime = ""
	if code := ValidateActionSetWorkTime(s); code != "leave_time_required" {
		t.Fatalf("외근 철수 필수: %s", code)
	}
}

func TestActionResultsForASExactlyThree(t *testing.T) {
	opts := ActionResultsForAS()
	if len(opts) != 3 {
		t.Fatalf("%d", len(opts))
	}
	if opts[0].Value != ResultDone || opts[1].Value != ResultPartial || opts[2].Value != ResultTransfer {
		t.Fatalf("%+v", opts)
	}
	if opts[1].Label != "추가 조치 필요" {
		t.Fatal(opts[1].Label)
	}
	if IsSelectableActionResult(ResultHold) || IsSelectableActionResult(ResultTemporary) {
		t.Fatal("비허용")
	}
}

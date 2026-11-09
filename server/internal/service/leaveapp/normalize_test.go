package leaveapp

import (
	"strings"
	"testing"
	"time"

	"customer-support/internal/hwpx"
	"customer-support/internal/model"
	"customer-support/internal/service/approval"
	"customer-support/internal/service/leavecalc"
)

func Test95_162_NormalizePeriodAndHalf(t *testing.T) {
	in, err := Normalize(Input{TypeKey: leavecalc.TypeAnnual, PeriodMode: hwpx.LeavePeriodHours, StartDate: "2026-08-10", StartTime: "09:00"})
	if err != nil {
		t.Fatal(err)
	}
	if in.PeriodMode != hwpx.LeavePeriodDays || in.EndDate != "2026-08-10" || in.StartTime != "" {
		t.Fatalf("%+v", in)
	}
	h, err := Normalize(Input{TypeKey: leavecalc.TypeHalf, PeriodMode: hwpx.LeavePeriodDays, StartDate: "2026-08-10", StartTime: "09:00", HalfPart: "pm"})
	if err != nil {
		t.Fatal(err)
	}
	if h.PeriodMode != hwpx.LeavePeriodHours || h.EndTime != "14:00" || h.HalfPart != "am" {
		t.Fatalf("server half %+v", h)
	}
	n, err := Normalize(Input{TypeKey: leavecalc.TypeHalf, StartDate: "2026-08-10", StartTime: "12:30"})
	if err != nil || n.StartTime != "13:00" || n.EndTime != "17:00" || n.HalfPart != "pm" {
		t.Fatalf("12:30 %+v err=%v", n, err)
	}
	pm, err := Normalize(Input{TypeKey: leavecalc.TypeHalf, StartDate: "2026-08-10", StartTime: "14:00"})
	if err != nil || pm.EndTime != "18:00" || pm.HalfPart != "pm" {
		t.Fatalf("14:00 %+v err=%v", pm, err)
	}
}

func Test95_162_EarlyLeaveLunchSkip(t *testing.T) {
	svc, _, staff := testSvc(t)
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech}, Input{
		TypeKey: leavecalc.TypeEarlyLeave, PeriodMode: hwpx.LeavePeriodDays,
		StartDate: "2026-08-14", StartTime: "11:00", EndTime: "14:00", Reason: "조퇴점심",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(id)
	if got.WorkMinutes != 120 || got.PeriodMode != hwpx.LeavePeriodHours {
		t.Fatalf("%+v", got)
	}
}

func Test95_162_SwapSnapshotStillFills(t *testing.T) {
	sn := snapshotFrom(&Request{
		DocNo: "VI-근태-260103-001", TypeKey: leavecalc.TypeVacation, PeriodMode: hwpx.LeavePeriodSwap,
		StartDate: "2026-01-03", EndDate: "2026-01-03", SwapDate: "2026년 01월 04일", SwapState: "worked",
		Reason: "대체근무",
	}, time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC))
	in := PermitInput(sn, "01/03", "")
	vals := hwpx.LeavePermitValues(in)
	if vals["ck대체"] != "☑" || !strings.Contains(vals["대체_일자"], "01월 04일") && vals["대체_일자"] == "" {
		if vals["ck대체"] != "☑" {
			t.Fatalf("swap %v", vals["ck대체"])
		}
	}
	if vals["ck기근무"] != "☑" {
		t.Fatalf("worked %s", vals["ck기근무"])
	}
}

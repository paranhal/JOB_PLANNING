package model

import "testing"

func TestWorkKindLabelSupport(t *testing.T) {
	if WorkKindLabel(WorkKindSupport) != "지원" {
		t.Fatal(WorkKindLabel(WorkKindSupport))
	}
	if !IsSupportType(SupportFactoryInbound) || IsSupportType("unknown") {
		t.Fatal("types")
	}
}

func TestValidateSupportDetailRequired(t *testing.T) {
	if err := ValidateSupportDetail(ASSupportDetail{SupportType: "nope"}); err == nil {
		t.Fatal("unknown type")
	}
	if err := ValidateSupportDetail(ASSupportDetail{SupportType: SupportFactoryInbound}); err == nil {
		t.Fatal("vendor+due")
	}
	if err := ValidateSupportDetail(ASSupportDetail{SupportType: SupportCourier}); err == nil {
		t.Fatal("tracking")
	}
	if err := ValidateSupportDetail(ASSupportDetail{SupportType: SupportLoanerSend}); err == nil {
		t.Fatal("asset")
	}
	if err := ValidateSupportDetail(ASSupportDetail{SupportType: SupportOther}); err == nil {
		t.Fatal("memo")
	}
	if err := ValidateSupportDetail(ASSupportDetail{
		SupportType: SupportFactoryInbound, VendorName: "한빛", DueDate: "2026-10-20",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeriveSupportStepper(t *testing.T) {
	in := ASSupportItem{Work: ASWorkItem{Status: "open"}, Detail: ASSupportDetail{SupportType: SupportFactoryInbound}}
	ret := ASSupportItem{Work: ASWorkItem{Status: "open", ScheduledDate: "2026-10-22"}, Detail: ASSupportDetail{SupportType: SupportFactoryReturn, DueDate: "2026-10-22"}}
	if DeriveSupportStepper([]ASSupportItem{in}).Show {
		t.Fatal("입고만이면 스테퍼 없음")
	}
	st := DeriveSupportStepper([]ASSupportItem{in, ret})
	if !st.Show || st.Stage != 0 {
		t.Fatalf("입고 예정: %+v", st)
	}
	in.Detail.ActionDate = "2026-10-13"
	st = DeriveSupportStepper([]ASSupportItem{in, ret})
	if st.Stage != 2 {
		t.Fatalf("회수 예정: %+v", st)
	}
	ret.Detail.ActionDate = "2026-10-20"
	st = DeriveSupportStepper([]ASSupportItem{in, ret})
	if st.Stage != 3 {
		t.Fatalf("회수 완료: %+v", st)
	}
}

func TestFactoryWaitFromDates(t *testing.T) {
	days := func(a, b string) (int, bool) {
		if a == "" || b == "" {
			return 0, false
		}
		if a == b {
			return 0, true
		}
		return 3, true
	}
	none := FactoryWaitFromDates("", "", "2026-10-20", days)
	if none.Label != "—" {
		t.Fatalf("none=%s", none.Label)
	}
	prog := FactoryWaitFromDates("2026-10-13", "", "2026-10-20", days)
	if prog.Mode != FactoryWaitInProgress || prog.Label != "진행 중" {
		t.Fatalf("prog=%+v", prog)
	}
	done := FactoryWaitFromDates("2026-10-13", "2026-10-16", "2026-10-20", days)
	if done.Mode != FactoryWaitDays || done.Days != 3 {
		t.Fatalf("done=%+v", done)
	}
}

package model

import "testing"

// 수량만 하부 업무의 진행 보고 문장. 95-176 질문 1.
func TestQtyPackageSentence(t *testing.T) {
	cases := []struct {
		q    ReportPackageQty
		want string
	}{
		{ReportPackageQty{Name: "환경조사", Unit: "식", Done: 0, Total: 120}, "환경조사는 아직 착수 전입니다. 대상은 120식입니다."},
		{ReportPackageQty{Name: "환경조사", Unit: "식", Done: 30, Total: 120}, "환경조사는 120식 중 30식 완료(25%)했습니다."},
		{ReportPackageQty{Name: "물품산정", Unit: "", Done: 1200, Total: 3000}, "물품산정은 3,000식 중 1,200식 완료(40%)했습니다."},
		{ReportPackageQty{Name: "설치", Unit: "대", Done: 50, Total: 50}, "설치는 목표 50대를 모두 완료했습니다."},
	}
	for _, c := range cases {
		if got := qtyPackageSentence(c.q); got != c.want {
			t.Fatalf("%+v\n got=%q\nwant=%q", c.q, got, c.want)
		}
	}
}

// 목표 수량이 있는 수량만 하부 업무만 진행 보고 문장에 한 줄을 더하고, 분모 0 이면 만들지 않는다.
func TestBuildReportAddsQtyOnlyWhenTotalSet(t *testing.T) {
	base := ProgressReport{Has: true, PeriodLabel: "오늘", Phase: "전처리 업무", Package: "환경조사"}
	if got := BuildProjectProgressReport(base); len(got) != 1 {
		t.Fatalf("수량 없으면 위치 문장만: %v", got)
	}
	base.PackageQty = &ReportPackageQty{Name: "환경조사", Unit: "식", Done: 30, Total: 0}
	if got := BuildProjectProgressReport(base); len(got) != 1 {
		t.Fatalf("분모 0 이면 수량 문장 없음: %v", got)
	}
	base.PackageQty.Total = 120
	got := BuildProjectProgressReport(base)
	if len(got) != 2 || got[1] != "환경조사는 120식 중 30식 완료(25%)했습니다." {
		t.Fatalf("수량 문장: %v", got)
	}
}
